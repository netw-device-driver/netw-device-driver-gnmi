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

package dd

import (
	"context"
	"strings"
	"time"

	ndrv1 "github.com/netw-device-driver/ndd-core/apis/dvr/v1"
	nddv1 "github.com/netw-device-driver/ndd-runtime/apis/common/v1"
	"github.com/netw-device-driver/ndd-runtime/pkg/logging"
	"github.com/netw-device-driver/netw-device-driver-gnmi/internal/devices"
	"github.com/openconfig/gnmi/proto/gnmi"
	"github.com/pkg/errors"
)

const (
	//timers
	notReadyTimeout = 10 * time.Second
	//errors

	errDeviceNotRegistered   = "the device type is not registered"
	errDeviceInitFailed      = "cannot initialize the device"
	errDeviceDiscoveryFailed = "cannot discover device"
	errDeviceGetConfigFailed = "cannot get device config"
	errGetNetworkNode        = "cannot get NetworkNode"
	errGetSecret             = "cannot get Secret"
)

type deviceDriver struct {
	ctx context.Context

	// startup data
	DeviceName string

	// runtime data
	DeviceType nddv1.DeviceType

	// k8sapi client
	//scheme *runtime.Scheme
	//client client.Client
	K8sApi *K8sApi

	// gnmi client
	Target    *Target          // used to interact via gnmi to the target to get capabilities
	Device    devices.Device   // handles all gnmi interaction based on the specific deviceexcept the capabilities
	Collector *DeviceCollector // handles the gnmi subscriptions

	// grpc server
	Server *Server // used for registration, cache update/status reporting
	//Register *Register // grpc service which handles registration
	//Cache    *Cache    // grpc service which handles the cache

	// dynamic discovered data
	DeviceDetails *ndrv1.DeviceDetails
	InitialConfig map[string]interface{}

	// logging
	log logging.Logger

	// stopCh
	StopCh chan struct{}
}

// getDeviceType returns the devicetype using the registered data from the provider
func (d *deviceDriver) getDeviceType(gnmiCap []*gnmi.ModelData) nddv1.DeviceType {
	for _, sm := range gnmiCap {
		for match, devicType := range d.Server.Cache.GetDeviceMatches() {
			d.log.Debug("Device info", "match", match, "deviceType", devicType, "sm.Name", sm.Name)
			if strings.Contains(sm.Name, match) {
				return devicType
			}
		}
	}
	return nddv1.DeviceTypeUnknown
}

func (d *deviceDriver) Ready() error {
	//d.DeviceDetails = d.initDeviceDetails()
	if err := d.K8sApi.SetNetworkNodeStatus(d.ctx, d.DeviceDetails, ndrv1.Discovered()); err != nil {
		d.log.Debug(errSetNetworkNodeStatus, "error", err)
		return errors.Wrap(err, errSetNetworkNodeStatus)
	}
	return nil
}

func (d *deviceDriver) NotReady(msg string) error {
	d.DeviceDetails = d.initDeviceDetails()
	if err := d.K8sApi.SetNetworkNodeStatus(d.ctx, d.DeviceDetails, ndrv1.NotDiscovered()); err != nil {
		d.log.Debug(errSetNetworkNodeStatus, "error", err)
		return errors.Wrap(err, errSetNetworkNodeStatus)
	}
	d.log.Debug(msg)
	return nil
}

func (d *deviceDriver) Configured() error {
	d.DeviceDetails = d.initDeviceDetails()
	if err := d.K8sApi.SetNetworkNodeStatus(d.ctx, d.DeviceDetails, ndrv1.Configured()); err != nil {
		d.log.Debug(errSetNetworkNodeStatus, "error", err)
		return errors.Wrap(err, errSetNetworkNodeStatus)
	}
	return nil
}

func (d *deviceDriver) NotConfigured(s string) error {
	d.DeviceDetails = d.initDeviceDetails()
	if err := d.K8sApi.SetNetworkNodeStatus(d.ctx, d.DeviceDetails, ndrv1.NotConfigured()); err != nil {
		d.log.Debug(errSetNetworkNodeStatus, "error", err)
		return errors.Wrap(err, errSetNetworkNodeStatus)
	}
	return nil
}

func (d *deviceDriver) initDeviceDetails() *ndrv1.DeviceDetails {
	return &ndrv1.DeviceDetails{
		HostName:     &d.DeviceName,
		Kind:         new(string),
		SwVersion:    new(string),
		MacAddress:   new(string),
		SerialNumber: new(string),
	}
}
