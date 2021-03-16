/*
Copyright 2021 Wim Henderickx.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package ddriver

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	nddv1 "github.com/netw-device-driver/netw-device-controller/api/v1"
	"github.com/netw-device-driver/netw-device-driver-gnmi/gnmic"
	"github.com/netw-device-driver/netwdevpb"
	"github.com/openconfig/gnmi/proto/gnmi_ext"
	log "github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const timer = 10

// HardwareDetails struct
type HardwareDetails struct {
	// the Kind of hardware
	Kind *string `json:"kind,omitempty"`
	// the Mac address of the hardware
	MacAddress *string `json:"macAddress,omitempty"`
	// the Serial Number of the hardware
	SerialNumber *string `json:"serialNumber,omitempty"`
}

// DeviceDriver contains the device driver information
type DeviceDriver struct {
	Server          *string
	DeviceName      *string
	GnmiClient      *gnmic.GnmiClient
	K8sClient       *client.Client
	NetworkNodeKind *string
	HardwareDetails *HardwareDetails
	LatestConfig    map[string]interface{}

	Cache  *Cache
	StopCh chan struct{}
	Ctx    context.Context
}

// Option is a function to initialize the options of the device driver
type Option func(d *DeviceDriver)

// WithServer initializes the server in the device driver
func WithServer(s *string) Option {
	return func(d *DeviceDriver) {
		d.Server = s
	}
}

// WithDeviceName initializes the device name in the device driver
func WithDeviceName(n *string) Option {
	return func(d *DeviceDriver) {
		d.DeviceName = n
	}
}

// WithDeviceName initializes the device name in the device driver
func WithK8sClient(c *client.Client) Option {
	return func(d *DeviceDriver) {
		d.K8sClient = c
	}
}

// GnmiProtocolOption is a function to initialize the options of the protocol
type GnmiProtocolOption func(g *gnmic.GnmiClient)

// WithTarget initializes the address in the protocol of the  device driver
func WithTarget(t *string) GnmiProtocolOption {
	return func(g *gnmic.GnmiClient) {
		g.Target = *t
	}
}

// WithUsername initializes the username in the protocol of the  device driver
func WithUsername(u *string) GnmiProtocolOption {
	return func(g *gnmic.GnmiClient) {
		g.Username = *u
	}
}

// WithPassword initializes the password in the protocol of the device driver
func WithPassword(p *string) GnmiProtocolOption {
	return func(g *gnmic.GnmiClient) {
		g.Password = *p
	}
}

// WithSkipVerify initializes the password in the protocol of the device driver
func WithSkipVerify(b bool) GnmiProtocolOption {
	return func(g *gnmic.GnmiClient) {
		g.SkipVerify = b
	}
}

// WithEncoding initializes the encoding in the protocol of the device driver
func WithEncoding(e *string) GnmiProtocolOption {
	return func(g *gnmic.GnmiClient) {
		g.Encoding = *e
	}
}

// NewDeviceDriver function defines a new device driver
func NewDeviceDriver(opts []Option, popts ...GnmiProtocolOption) *DeviceDriver {
	log.Info("initialize new device driver ...")
	d := &DeviceDriver{
		Server:       new(string),
		DeviceName:   new(string),
		LatestConfig: make(map[string]interface{}),
		K8sClient:    new(client.Client),
		Cache: &Cache{
			Data:   make(map[int]map[string]*Data),
			Levels: make([]int, 0),
		},
		StopCh: make(chan struct{}),
	}

	for _, o := range opts {
		o(d)
	}

	d.GnmiClient = gnmic.NewGnmiClient()
	for _, o := range popts {
		o(d.GnmiClient)
	}
	log.Infof("Client: %v", *d.GnmiClient)
	if err := d.GnmiClient.Initialize(); err != nil {
		log.WithError(err).Error("unable to setup the GNMI connection")
		os.Exit(1)
	}

	return d
}

// InitDeviceDriverControllers initializes the device driver controller
func (d *DeviceDriver) InitDeviceDriverControllers() error {
	log.Info("initialize device driver controllers ...")
	// stopCh to synchronize the finalization for a graceful shutdown
	d.StopCh = make(chan struct{})
	defer close(d.StopCh)

	// Create a context.
	ctx, cancel := context.WithCancel(context.Background())

	d.Ctx = ctx

	// start nats device driver
	go func() {
		d.StartNatsSubscription()
	}()

	// start reconcile device driver
	go func() {
		d.StartReconcileProcess()
	}()

	select {
	case <-d.Ctx.Done():
		log.Info("context cancelled")
	}
	close(d.StopCh)

	cancel()

	return nil
}

// StartReconcileProcess starts the driver reconciiation process
func (d *DeviceDriver) StartReconcileProcess() {
	log.Info("Starting reconciliation process...")
	timeout := make(chan bool, 1)
	timeout <- true
	log.Info("Timer reconciliation process is running...")
	for {
		select {
		case <-timeout:
			time.Sleep(timer * time.Second)
			timeout <- true

			log.Info("reconcile cache...")
			d.ReconcileCache()

		case <-d.StopCh:
			log.Info("Stopping timer reconciliation process")
			return
		}
	}
}

// StartNatsSubscription starts the nats subscription process
func (d *DeviceDriver) StartNatsSubscription() {
	log.Info("Starting nats subscribe...")
	topic := "ndd." + *d.DeviceName
	// Connect Options.
	opts := []nats.Option{nats.Name(fmt.Sprintf("NATS Subscriber %s", topic))}
	opts = d.SetupNatsConnOptions(opts)

	// Connect to NATS
	nc, err := nats.Connect(*d.Server, opts...)
	if err != nil {
		log.Error("Nats connect error", "Error", err)
		os.Exit(0)
	}

	nc.Subscribe(topic, func(msg *nats.Msg) {
		netwCfgMsg := &netwdevpb.ConfigMessage{}
		if err = proto.Unmarshal(msg.Data, netwCfgMsg); err != nil {
			log.WithError(err).Error("unmarchal error")
		}
		if err = d.Cache.UpdateCacheEntry(msg.Subject, netwCfgMsg); err != nil {
			log.WithError(err).Error("update cache error")
		}
		log.Infof("Nats subscription data path %s, %s", msg.Subject, netwCfgMsg.Action)
	})
	nc.Flush()

	if err := nc.LastError(); err != nil {
		log.WithError(err).Error("Nats subscribe error")
		os.Exit(0)
	} else {
		log.Info("Subscribe info",
			"Subject", topic)
	}
}

// SetupNatsConnOptions defines the nats connection options
func (d *DeviceDriver) SetupNatsConnOptions(opts []nats.Option) []nats.Option {
	totalWait := 10 * time.Minute
	reconnectDelay := time.Second

	opts = append(opts, nats.ReconnectWait(reconnectDelay))
	opts = append(opts, nats.MaxReconnects(int(totalWait/reconnectDelay)))
	opts = append(opts, nats.DisconnectHandler(func(nc *nats.Conn) {
		log.Infof("Disconnected: will attempt reconnects for %.0fm", totalWait.Minutes())
	}))
	opts = append(opts, nats.ReconnectHandler(func(nc *nats.Conn) {
		log.Infof("Reconnected [%s] url", nc.ConnectedUrl())
	}))
	opts = append(opts, nats.ClosedHandler(func(nc *nats.Conn) {
		log.WithError(nc.LastError()).Error("Exiting")
	}))
	return opts
}

// DiscoverDeviceDetails discovers the device details
func (d *DeviceDriver) DiscoverDeviceDetails() error {
	log.Info("verifying gnmi capabilities...")

	d.Ctx = context.Background()

	ext := new(gnmi_ext.Extension)
	resp, err := d.GnmiClient.Capabilities(d.Ctx, ext)
	if err != nil {
		return fmt.Errorf("failed sending capabilities request: %v", err)
	}
	log.Infof("response: %v", resp)

	for _, sm := range resp.SupportedModels {
		if strings.Contains(sm.Name, "srl_nokia") {
			d.NetworkNodeKind = stringPtr("nokia_srl")
			break
		}
		// TODO add other devices
	}

	log.Infof("gnmi connectivity verified; response: %s, networkNodeInfo %s", resp.GNMIVersion, *d.NetworkNodeKind)

	dDetails := &nddv1.DeviceDetails{}
	switch *d.NetworkNodeKind {
	case "nokia_srl":
		dDetails, err = d.DiscoverDeviceDetailsSRL()
		if err != nil {
			return err
		}
		d.LatestConfig, err = d.GetLatestConfig()
		if err != nil {
			return err
		}
	default:
		// TODO add other devices
	}

	log.Infof("Device details: %v", dDetails)
	log.Infof("LatestConfig: %v", d.LatestConfig)

	if err := d.NetworkDeviceUpdate(dDetails, nddv1.DiscoveryStatusReady); err != nil {
		return err
	}

	return nil

}
