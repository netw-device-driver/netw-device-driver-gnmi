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
	"strconv"
	"strings"
	"time"

	"github.com/karimra/gnmic/collector"
	ndddvrv1 "github.com/netw-device-driver/ndd-core/apis/dvr/v1"
	"github.com/netw-device-driver/ndd-runtime/pkg/logging"
	"github.com/netw-device-driver/ndd-runtime/pkg/utils"
	"github.com/netw-device-driver/netw-device-driver-gnmi/internal/devices"
	_ "github.com/netw-device-driver/netw-device-driver-gnmi/internal/devices/all"
	"github.com/netw-device-driver/netwdevpb"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// timers
	timer = 1

	// errors
	errSetupGnmi             = "cannot setup gnmi connection"
	errCreateTcpListener     = "cannot create tcp listener"
	errGrpcServer            = "error grpc server"
	errUpdateDeviationServer = "cannot update Deviation Server"
	errJsonMarshal           = "errors json marshal"
	errStartGRPCServer       = "error start grpc server"
)

// DeviceDriver contains the device driver information
type DeviceDriver struct {
	log               logging.Logger
	Device            devices.Device
	Objects           Establisher
	Discoverer        Discoverer
	Collector         Collector
	GrpcServerPort    *int
	GrpcServerAddress *string
	DeviceName        *string
	TargetConfig      *collector.TargetConfig
	Target            *collector.Target
	K8sClient         *client.Client
	NetworkNodeKind   *string
	DeviceDetails     *ndddvrv1.DeviceDetails
	InitialConfig     map[string]interface{}
	GrpcServer        *grpc.Server

	AutoPilot *bool
	Debug     *bool

	Registrator *Registrator
	Cache       *Cache
	SubCh       chan bool
	StopCh      chan struct{}
	Ctx         context.Context
}

// Option is a function to initialize the options of the device driver
type Option func(d *DeviceDriver)

func WithContext() Option {
	return func(d *DeviceDriver) {
		d.Ctx = context.Background()
	}
}

// WithLogger specifies how the Reconciler should log messages.
func WithLogger(log logging.Logger) Option {
	return func(d *DeviceDriver) {
		d.log = log
	}
}

// WithEstablisher specifies how the ddriver should create/delete/update
// resources through the k8s api
func WithEstablisher(er *APIEstablisher) Option {
	return func(d *DeviceDriver) {
		d.Objects = er
	}
}

// WithGrpcServer initializes the cache server in the device driver
func WithGrpcServer(s *string) Option {
	return func(d *DeviceDriver) {
		d.GrpcServerAddress = s
		p, _ := strconv.Atoi(strings.Split(*s, ":")[1])
		d.GrpcServerPort = &p
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

func WithAutoPilot(b *bool) Option {
	return func(d *DeviceDriver) {
		d.AutoPilot = b
	}
}

func WithDebug(b *bool) Option {
	return func(d *DeviceDriver) {
		d.Debug = b
	}
}

// NewDeviceDriver function defines a new device driver
func NewDeviceDriver(opts ...Option) (*DeviceDriver, error) {
	d := &DeviceDriver{
		GrpcServerAddress: new(string),
		DeviceName:        new(string),
		DeviceDetails:     new(ndddvrv1.DeviceDetails),
		TargetConfig:      new(collector.TargetConfig),
		InitialConfig:     make(map[string]interface{}),
		K8sClient:         new(client.Client),

		//SubCh:     make(chan struct{}),
		//StopCh:    make(chan struct{}),
		AutoPilot: new(bool),
		Debug:     new(bool),
		//Subscriptions:  new([]string),
		//ExceptionPaths: new([]string),
	}

	for _, o := range opts {
		o(d)
	}

	d.Target = collector.NewTarget(d.TargetConfig)
	err := d.Target.CreateGNMIClient(d.Ctx, grpc.WithBlock()) // TODO add dialopts
	if err != nil {
		return nil, errors.Wrap(err, errSetupGnmi)
	}
	// initialize a discoverer to discover device kinds
	d.Discoverer = NewDeviceDiscoverer(d.Target, d.log)

	// initialize a collector as a telemtry collector
	d.Collector = NewDeviceCollector(d.Target, d.log)

	// initialize a subscriber as a grpc server to retreive the registration
	d.SubCh = make(chan bool)
	d.Registrator = NewRegistrator(d.log, d.SubCh)

	// initialize the cache
	d.Cache = NewCache(d.log)

	return d, nil
}

func (d *DeviceDriver) Start() error {
	d.log.Debug("start device driver...")
	errChannel := make(chan error)
	go func() {
		if err := d.StartGrpcServer(*d.GrpcServerAddress); err != nil {
			errChannel <- errors.Wrap(err, errStartGRPCServer)
		}
		errChannel <- nil
	}()
	return <-errChannel
}

func (d *DeviceDriver) InitGrpcServer() error {
	d.log.Debug("init grpc server ...")
	errChannel := make(chan error)
	go func() {
		if err := d.StartGrpcServer(*d.GrpcServerAddress); err != nil {
			errChannel <- errors.Wrap(err, errStartGRPCServer)
		}
		errChannel <- nil
	}()
	return <-errChannel
}

// StartGRPCServer starts the grpcs server
func (d *DeviceDriver) StartGrpcServer(s string) error {
	d.log.Debug("starting GRPC server...")

	// create a listener on a specific address:port
	l, err := net.Listen("tcp", s)
	if err != nil {
		return errors.Wrap(err, errCreateTcpListener)
	}

	// create a gRPC server object
	d.GrpcServer = grpc.NewServer()

	// attach the gRPC service to the server
	netwdevpb.RegisterCacheStatusServer(d.GrpcServer, d.Cache)
	netwdevpb.RegisterCacheUpdateServer(d.GrpcServer, d.Cache)
	netwdevpb.RegisterRegistrationServer(d.GrpcServer, d.Registrator)

	// start the server
	d.log.Debug("starting GRPC server...")
	if err := d.GrpcServer.Serve(l); err != nil {
		d.log.Debug("Errors", "error", err)
		return errors.Wrap(err, errGrpcServer)
	}
	return nil
}

func (d *DeviceDriver) StopGrpcServer() {
	d.GrpcServer.Stop()
}

// InitDeviceDriverControllers initializes the device driver controller
func (d *DeviceDriver) InitDeviceDriverControllers() error {
	d.log.Debug("initialize device driver controllers ...")
	// stopCh to synchronize the finalization for a graceful shutdown
	d.StopCh = make(chan struct{})
	defer close(d.StopCh)

	// start reconcile device driver
	go func() {
		d.StartReconcileProcess()
	}()

	// start gnmi subscription handler
	go func() {
		d.StartGnmiSubscriptionHandler()
	}()

	select {
	case <-d.Ctx.Done():
		d.log.Debug("context cancelled")
	}
	close(d.StopCh)

	return nil
}

// StartReconcileProcess starts the driver reconciiation process
func (d *DeviceDriver) StartReconcileProcess() error {
	d.log.Debug("Starting reconciliation process...")
	timeout := make(chan bool, 1)
	timeout <- true
	//log.Info("Timer reconciliation process is running...")
	// run the reconcile process on startup
	d.Cache.SetNewK8sOperatorUpdates(true)
	for {
		select {
		case <-timeout:
			time.Sleep(timer * time.Second)
			timeout <- true

			// reconcile cache when:
			// -> new updates from k8s operator are received
			// -> autopilot is on and new onChange information was reveived that requires device updates
			// -> autopilot is on and new onChange information was received that requires to repply the cache
			if d.Cache.GetNewK8sOperatorUpdates() || (*d.AutoPilot && (d.Cache.GetNewOnChangeUpdates() || d.Cache.GetOnChangeReApplyCache())) {
				d.log.Debug("........ Started  reconciliation ........")
				//d.ReconcileCache()
				d.log.Debug("........ Finished reconciliation ........")
			} else {
				//fmt.Printf(".")
			}
			// report changes to the deviation server
			target := "srl-k8s-operator-controller-manager-deviation-service" + "." + "srl-k8s-operator-system" + "." + "svc.cluster.local" + ":" + "9998"
			deviations := make([]*netwdevpb.Deviation, 0)
			d.Cache.Lock()
			for xpath, deviation := range d.Cache.GetOnChangeDeviations() {
				if deviation.Change {
					//var x1 interface{}
					//data := json.Unmarshal(deviation.Value, x1)
					// In auto-pilot mode, only report changes that are ignored by Exception paths
					if *d.AutoPilot && deviation.DeviationAction == netwdevpb.Deviation_DeviationActionIgnoreException {
						d.log.Debug("Deviation", "deviation", xpath, "Change", deviation.Change, "OnChangeAction", deviation.OnChangeAction.String(), "DeviationAction", deviation.DeviationAction.String(), "Data", string(deviation.Value))
						dev := &netwdevpb.Deviation{
							OnChange:       deviation.OnChangeAction,
							DevationResult: deviation.DeviationAction,
							Xpath:          xpath,
							Value:          deviation.Value,
						}
						deviations = append(deviations, dev)
					}
					// in operator controlled mode we report all changes, except the once we should ignore
					if !*d.AutoPilot && deviation.DeviationAction != netwdevpb.Deviation_DeviationActionIgnore {
						d.log.Debug("Deviation", "deviation", xpath, "Change", deviation.Change, "OnChangeAction", deviation.OnChangeAction.String(), "DeviationAction", deviation.DeviationAction.String(), "Data", string(deviation.Value))
						dev := &netwdevpb.Deviation{
							OnChange:       deviation.OnChangeAction,
							DevationResult: deviation.DeviationAction,
							Xpath:          xpath,
							Value:          deviation.Value,
						}
						deviations = append(deviations, dev)
					}
				}
				// update the change status to ensure we dont report it next time
				deviation.Change = false
			}
			d.Cache.Unlock()
			// only report deviations if there are changes
			if len(deviations) > 0 {
				if _, err := deviationUpdate(d.Ctx, utils.StringPtr(target), deviations); err != nil {
					return errors.Wrap(err, errUpdateDeviationServer)
				}
			}
		case <-d.StopCh:
			d.log.Debug("Stopping timer reconciliation process")
			return nil
		}
	}
}

func (d *DeviceDriver) StartGnmiSubscriptionHandler() {
	d.log.Debug("Starting cache GNMI subscription...")

	d.Collector.Lock()
	if d.Collector.GetSubscription("ConfigChangesubscription") {
		go d.Collector.StartSubscription(d.Ctx, "ConfigChangesubscription", d.Registrator.GetSubscriptions())
	}
	d.Collector.Unlock()

	chanSubResp, chanSubErr := d.Target.ReadSubscriptions()

	for {
		select {
		case resp := <-chanSubResp:
			//log.Infof("SubRsp Response %v", resp)
			d.processOnChangeUpdates(resp.Response)
		case tErr := <-chanSubErr:
			d.log.Debug("subscribe", "error", tErr)
			time.Sleep(60 * time.Second)
		case <-d.StopCh:
			//log.Info("Stopping subscription process")
			return
		}
	}
}

func addElement2ExpceptionTree(et map[string]interface{}, idx int, split []string) map[string]interface{} {
	// entry does not exist
	//d.log.Debug("addElement2ExpceptionTree", "Index", idx, "Split", split, "Exception Tree", et)
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
				//log.Errorf("addElement2ExpceptionTree: We should never come here")
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
	//log.Infof("Latest    Config: %v", lc)
	//log.Infof("Exception Tree  : %v", et)
	found := make(map[string]interface{})
	for k1, x1 := range lc {
		k1 := strings.Split(k1, ":")[len(strings.Split(k1, ":"))-1]
		found[k1] = false
		for k2, v2 := range et {
			// getHierarchicalElements assumes the string starts with a /
			ekvl := getHierarchicalElements("/" + k2)
			//log.Infof("ekvl: %v", ekvl)
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
									//log.Infof("k3: %v, ekvl[0].KeyName: %v", k3, ekvl[0].KeyName)
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
				d.log.Debug("Delete", "Path", prefix+k)
				ip := prefix + k
				_, err := d.Device.Delete(d.Ctx, &ip)
				if err != nil {
					// individual path delete process failure
					d.log.Debug("GNMI delete process failed for subscription delta", "path", ip)
				} else {
					// individual path delete process success
					d.log.Debug("GNMI delete processed successfully for subscription delta", "path", ip)
				}

			}
		case map[string]interface{}:
			d.DeletedUnwantedConfiguration(v.(map[string]interface{}), prefix+k+"/")
		}
	}
}

func (d *DeviceDriver) UpdateLatestConfigWithGnmi() error {
	mergedPath := "/"
	newMergedConfig, err := json.Marshal(d.InitialConfig)
	if err != nil {
		return errors.Wrap(err, errJsonMarshal)
	}
	_, err = d.Device.Update(d.Ctx, &mergedPath, newMergedConfig)
	if err != nil {
		// TODO check failure status
		return errors.Wrap(err, fmt.Sprintf("Merged update process FAILED, path: %s", mergedPath))
	} else {
		d.log.Debug("Merged update process SUCCEEDED, path: %s", mergedPath)
	}
	return nil
}
