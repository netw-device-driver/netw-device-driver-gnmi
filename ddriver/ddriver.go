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
	InitialConfig      map[string]interface{}
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
		InitialConfig:      make(map[string]interface{}),
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
		d.InitialConfig, err = d.GetInitialConfig()
		if err != nil {
			return err
		}
		// trim the first map
		for _, v := range d.InitialConfig {
			switch v.(type) {
			case map[string]interface{}:
				d.InitialConfig = cleanConfig(v.(map[string]interface{}))
			}
		}
		log.Infof("Latest config cleaned: %v", d.InitialConfig)
	default:
		// TODO add other devices
	}

	log.Infof("Device details: %v", d.DeviceDetails)
	log.Infof("LatestConfig: %v", d.InitialConfig)

	jsonConfigStr, err := json.Marshal(d.InitialConfig)
	if err != nil {
		return err
	}
	if err := d.ConfigMapUpdate(StringPtr(string(jsonConfigStr))); err != nil {
		return err
	}

	if err := d.NetworkNodeUpdate(d.DeviceDetails, nddv1.DiscoveryStatusReady); err != nil {
		return err
	}

	return nil
}

func cleanConfig(x1 map[string]interface{}) map[string]interface{} {
	x2 := make(map[string]interface{})
	for k1, v1 := range x1 {
		log.Infof("cleanConfig Key: %s", k1)
		log.Infof("cleanConfig Value: %v", v1)
		switch x3 := v1.(type) {
		case []interface{}:
			x := make([]interface{}, 0)
			for _, v3 := range x3 {
				switch x3 := v3.(type) {
				case map[string]interface{}:
					x4 := cleanConfig(x3)
					x = append(x, x4)
				default:
					x = append(x, v3)
				}
			}
			x2[strings.Split(k1, ":")[len(strings.Split(k1, ":"))-1]] = x
		case map[string]interface{}:
			x4 := cleanConfig(x3)
			x2[strings.Split(k1, ":")[len(strings.Split(k1, ":"))-1]] = x4
		default:
			x2[strings.Split(k1, ":")[len(strings.Split(k1, ":"))-1]] = v1
		}
	}
	return x2
}

func addElement2ExpceptionTree(et map[string]interface{}, idx int, split []string) map[string]interface{} {
	// entry does not exist
	log.Infof("addElement2ExpceptionTree Idx: %d, Split: %v Exception Tree: %v", idx, split, et)
	if _, ok := et[split[idx]]; !ok {
		// if there is no more elements in the exception path dont add more
		// if not initialize for the additional elements in the exception path
		if idx < len(split)-1 {
			e := make(map[string]interface{})
			idx++
			et[split[idx-1]] = addElement2ExpceptionTree(e, idx, split)
			return et
		} else {
			et[split[idx]] = nil
			return et
		}
	} else {
		if idx < len(split)-1 {
			idx++
			e := et[split[idx-1]]
			switch e.(type) {
			case map[string]interface{}:
				et[split[idx-1]] = addElement2ExpceptionTree(e.(map[string]interface{}), idx, split)
			default:
				log.Errorf("addElement2ExpceptionTree: We should never come here")
			}
			return et
		} else {
			et[split[idx]] = nil
			return et
		}
	}
}

// ValidateInitalConfig validates the retrieved config and checks if the
/*
func (d *DeviceDriver) ValidateInitalConfig() (err error) {
	// creates an exception tree
	et := make(map[string]interface{})
	for _, ep := range *d.ExceptionPaths {
		split := strings.Split(ep, "/")
		et = addElement2ExpceptionTree(et, 0, split)
	}

	log.Infof("Exception Tree: %v", et)

	lc := make(map[string]interface{})
	for _, v := range d.InitialConfig {
		switch v.(type) {
		case map[string]interface{}:
			lc = v.(map[string]interface{})
		}
	}

	f := compareLatestConfigTree(lc, et)
	log.Infof("compareLatestConfigTree with exception tree %v", f)

	d.DeletedUnwantedConfiguration(f, "/")

	d.InitialConfig, err = d.GetInitialConfig()
	if err != nil {
		return err
	}

	return nil
}
*/

func compareLatestConfigTree(lc, et map[string]interface{}) map[string]interface{} {
	log.Infof("Latest    Config: %v", lc)
	log.Infof("Exception Tree  : %v", et)
	found := make(map[string]interface{})
	for k1, x1 := range lc {
		k1 := strings.Split(k1, ":")[len(strings.Split(k1, ":"))-1]
		found[k1] = false
		for k2, v2 := range et {
			// getHierarchicalElements assumes the string starts with a /
			ekvl := getHierarchicalElements("/" + k2)
			log.Infof("ekvl: %v", ekvl)
			if k1 == ekvl[0].Element {
				if ekvl[0].KeyName != "" {
					// when a keyname exists, we should delete the found entry w/o the key, since this name will not be used
					delete(found, k1)
					switch x3 := x1.(type) {
					case []interface{}:
						for _, v1 := range x3 {
							switch x3 := v1.(type) {
							case map[string]interface{}:
								for k3, v3 := range x3 {
									log.Infof("k3: %v, ekvl[0].KeyName: %v", k3, ekvl[0].KeyName)
									if k3 == ekvl[0].KeyName {
										switch v3.(type) {
										case string, uint32:
											if v3 == ekvl[0].KeyValue {
												switch x2 := v2.(type) {
												case nil:
													// we are at the end of the exception tree
													found[fmt.Sprintf("%s[%s=%s]", ekvl[0].Element, ekvl[0].KeyName, v3)] = true
												case map[string]interface{}:
													// we are not yet at the end of the exception tree
													found[fmt.Sprintf("%s[%s=%s]", ekvl[0].Element, ekvl[0].KeyName, v3)] = compareLatestConfigTree(x1.(map[string]interface{}), x2)
												}
											} else {
												found[fmt.Sprintf("%s[%s=%s]", ekvl[0].Element, ekvl[0].KeyName, v3)] = false
											}
										}
									}
									// ignore other elements since they are not keys
								}
							}
						}
					}
				} else {
					// element without a key, like /system
					switch x2 := v2.(type) {
					case nil:
						// we are at the end of the exception tree
						found[k1] = true
					case map[string]interface{}:
						// we are not yet at the end of the exception tree
						found[k1] = compareLatestConfigTree(x1.(map[string]interface{}), x2)
					}
				}
			}
		}
	}
	//log.Infof("compareLatestConfigTree: %v", found)
	return found
}

func (d *DeviceDriver) DeletedUnwantedConfiguration(f map[string]interface{}, prefix string) {
	for k, v := range f {
		switch b := v.(type) {
		case bool:
			// if b is false
			if !b {
				log.Infof("Delete Path: %s", prefix+k)
				ip := prefix + k
				if err := d.deleteDeviceDataGnmi(&ip); err != nil {
					// individual path delete process failure
					log.WithError(err).Errorf("GNMI delete process failed for subscription delta, path: %s", ip)
				} else {
					// individual path delete process success
					log.Infof("GNMI delete processed successfully for subscription delta, path: %s", ip)
				}

			}
		case map[string]interface{}:
			d.DeletedUnwantedConfiguration(v.(map[string]interface{}), prefix+k+"/")
		}
	}
}

func (d *DeviceDriver) UpdateLatestConfigWithGnmi() {
	mergedPath := "/"
	newMergedConfig, err := json.Marshal(d.InitialConfig)
	if err != nil {
		log.Fatal(err)
	}
	if err := d.updateDeviceDataGnmi(&mergedPath, newMergedConfig); err != nil {
		// TODO check failure status
		log.WithError(err).Errorf("Merged update process FAILED, path: %s", mergedPath)
	} else {
		log.Infof("Merged update process SUCCEEDED, path: %s", mergedPath)
	}

}
