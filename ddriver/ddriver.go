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
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/karimra/gnmic/collector"
	nddv1 "github.com/netw-device-driver/netw-device-controller/api/v1"
	"github.com/netw-device-driver/netwdevpb"
	"github.com/openconfig/gnmi/proto/gnmi_ext"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const timer = 10

// DeviceeDetails struct
/*
type DeviceDetails struct {
	// the Kind of hardware
	Kind *string `json:"kind,omitempty"`
	// the Mac address of the hardware
	MacAddress *string `json:"macAddress,omitempty"`
	// the Serial Number of the hardware
	SerialNumber *string `json:"serialNumber,omitempty"`
}
*/

// DeviceDriver contains the device driver information
type DeviceDriver struct {
	//NatsServer         *string
	CacheServerPort    *int
	CacheServerAddress *string
	DeviceName         *string
	TargetConfig       *collector.TargetConfig
	Target             *collector.Target
	TargetCollector    *Collector
	K8sClient          *client.Client
	NetworkNodeKind    *string
	DeviceDetails      *nddv1.DeviceDetails
	LatestConfig       map[string]interface{}
	Subscriptions      *[]string
	ExceptionPaths     *[]string
	Debug              *bool

	Cache  *Cache
	StopCh chan struct{}
	Ctx    context.Context
}

// Option is a function to initialize the options of the device driver
type Option func(d *DeviceDriver)

// WithCacheServer initializes the cache server in the device driver
func WithCacheServer(s *string) Option {
	return func(d *DeviceDriver) {
		d.CacheServerAddress = s
		p, _ := strconv.Atoi(strings.Split(*s, ":")[1])
		d.CacheServerPort = &p
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

// WithTargetName initializes the name in the gnmi target
func WithTargetName(n *string) Option {
	return func(d *DeviceDriver) {
		d.TargetConfig.Name = *n
	}
}

// WithTargetAddress initializes the address in the gnmi target
func WithTargetAddress(t *string) Option {
	return func(d *DeviceDriver) {
		d.TargetConfig.Address = *t
	}
}

// WithUsername initializes the username
func WithUsername(u *string) Option {
	return func(d *DeviceDriver) {
		d.TargetConfig.Username = u
	}
}

// WithPassword initializes the password
func WithPassword(p *string) Option {
	return func(d *DeviceDriver) {
		d.TargetConfig.Password = p
	}
}

// WithSkipVerify initializes skipVerify
func WithSkipVerify(b *bool) Option {
	return func(d *DeviceDriver) {
		d.TargetConfig.SkipVerify = b
	}
}

// WithInsecure initializes insecure
func WithInsecure(b *bool) Option {
	return func(d *DeviceDriver) {
		d.TargetConfig.Insecure = b
	}
}

// WithTLSCA initializes TLSCA
func WithTLSCA(t *string) Option {
	return func(d *DeviceDriver) {
		d.TargetConfig.TLSCA = t
	}
}

// WithTLSCert initializes TLSCert
func WithTLSCert(t *string) Option {
	return func(d *DeviceDriver) {
		d.TargetConfig.TLSCert = t
	}
}

// WithTLSKey initializes TLSKey
func WithTLSKey(t *string) Option {
	return func(d *DeviceDriver) {
		d.TargetConfig.TLSKey = t
	}
}

// WithGzip initializes targetconfig
func WithGzip(b *bool) Option {
	return func(d *DeviceDriver) {
		d.TargetConfig.Gzip = b
	}
}

// WithTimeout initializes the timeout in the protocol of the device driver
func WithTimeout(t time.Duration) Option {
	return func(d *DeviceDriver) {
		d.TargetConfig.Timeout = t
	}
}

// WithEncoding initializes the encoding in the protocol of the device driver
/*
func WithEncoding(e *string) Option {
	return func(d *DeviceDriver) {
		d.TargetConfig.Encoding = *e
	}
}
*/

func WithDebug(b *bool) Option {
	return func(d *DeviceDriver) {
		d.Debug = b
	}
}

// NewDeviceDriver function defines a new device driver
func NewDeviceDriver(opts ...Option) *DeviceDriver {
	log.Info("initialize new device driver ...")
	dataSubDeltaDelete := make([]string, 0)
	dataSubDeltaUpdate := make([]string, 0)
	var x1 interface{}
	empty, err := json.Marshal(x1)
	d := &DeviceDriver{
		CacheServerAddress: new(string),
		DeviceName:         new(string),
		DeviceDetails:      new(nddv1.DeviceDetails),
		TargetConfig:       new(collector.TargetConfig),
		LatestConfig:       make(map[string]interface{}),
		K8sClient:          new(client.Client),
		Cache: &Cache{
			Data:               make(map[int]map[string]*ResourceData),
			Levels:             make([]int, 0),
			DataSubDeltaDelete: &dataSubDeltaDelete,
			DataSubDeltaUpdate: &dataSubDeltaUpdate,
			ReApplyCacheData:   new(bool),
			CurrentConfig:      empty,
		},
		StopCh: make(chan struct{}),
		Debug:  new(bool),
		//Subscriptions:  new([]string),
		//ExceptionPaths: new([]string),
	}

	for _, o := range opts {
		o(d)
	}

	d.Target = collector.NewTarget(d.TargetConfig)

	d.Ctx = context.Background()
	err = d.Target.CreateGNMIClient(d.Ctx, grpc.WithBlock()) // TODO add dialopts
	if err != nil {
		log.WithError(err).Error("unable to setup the GNMI connection")
		os.Exit(1)
	}

	d.TargetCollector = NewCollector(d.Target)

	return d
}

func (d *DeviceDriver) InitExceptionPaths(eps *[]string) {
	d.ExceptionPaths = eps
}

func (d *DeviceDriver) InitSubscriptions(subs *[]string) {
	d.Subscriptions = subs
}

// InitDeviceDriverControllers initializes the device driver controller
func (d *DeviceDriver) InitDeviceDriverControllers() error {
	log.Info("initialize device driver controllers ...")
	// stopCh to synchronize the finalization for a graceful shutdown
	d.StopCh = make(chan struct{})
	defer close(d.StopCh)

	// Create a context.
	//ctx, cancel := context.WithCancel(context.Background())

	d.Ctx = context.Background()

	// start reconcile device driver
	go func() {
		d.StartReconcileProcess()
	}()

	// start grpc server
	go func() {
		d.StartCacheGRPCServer()
	}()

	// start gnmi subscription handler
	go func() {
		d.StartGnmiSubscriptionHandler()
	}()

	select {
	case <-d.Ctx.Done():
		log.Info("context cancelled")
	}
	close(d.StopCh)

	//cancel()

	return nil
}

// StartCacheGRPCServer function
func (d *DeviceDriver) StartCacheGRPCServer() {
	log.Info("Starting cache GRPC server...")

	// create a listener on a specific address:port
	lis, err := net.Listen("tcp", *d.CacheServerAddress)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	// create a gRPC server object
	grpcServer := grpc.NewServer()

	// attach the gRPC service to the server
	netwdevpb.RegisterCacheStatusServer(grpcServer, d.Cache)
	netwdevpb.RegisterCacheUpdateServer(grpcServer, d.Cache)

	// start the server
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %s", err)
	}
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

func (d *DeviceDriver) StartGnmiSubscriptionHandler() {
	log.Info("Starting cache GNMI subscription...")

	d.TargetCollector.SubscriptionsMutex.RLock()
	if _, ok := d.TargetCollector.Subscriptions["ConfigChangesubscription"]; !ok {
		go d.TargetCollector.StartSubscription(d.Ctx, StringPtr("ConfigChangesubscription"), d.Subscriptions)
	}
	d.TargetCollector.SubscriptionsMutex.RUnlock()

	chanSubResp, chanSubErr := d.Target.ReadSubscriptions()

	for {
		select {
		case resp := <-chanSubResp:
			log.Infof("SubRsp Response %v", resp)
			d.validateDiff(resp.Response)
		case tErr := <-chanSubErr:
			log.Errorf("subscribe error: %v", tErr)
			time.Sleep(60 * time.Second)
		case <-d.StopCh:
			log.Info("Stopping subscription process")
			return
		}
	}
}

// DiscoverDeviceDetails discovers the device details
func (d *DeviceDriver) DiscoverDeviceDetails() error {
	log.Info("verifying gnmi capabilities...")

	ext := new(gnmi_ext.Extension)
	resp, err := d.Target.Capabilities(d.Ctx, ext)
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

	//dDetails := &nddv1.DeviceDetails{}
	switch *d.NetworkNodeKind {
	case "nokia_srl":
		d.DeviceDetails, err = d.DiscoverDeviceDetailsSRL()
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

	log.Infof("Device details: %v", d.DeviceDetails)
	log.Infof("LatestConfig: %v", d.LatestConfig)

	if err := d.NetworkDeviceUpdate(d.DeviceDetails, nddv1.DiscoveryStatusReady); err != nil {
		return err
	}

	return nil
}
