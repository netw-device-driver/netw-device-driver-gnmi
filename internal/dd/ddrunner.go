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
	"fmt"
	"time"

	"github.com/netw-device-driver/netw-device-driver-gnmi/internal/devices"
	"github.com/netw-device-driver/netw-device-driver-gnmi/internal/jsonutils"
)

const (
	// timers
	reconcileTimer = 1 * time.Second
)

func (d *deviceDriver) Run() error {

	if err := d.Server.Run(d.ctx, d.Register, d.Cache); err != nil {
		return err
	}
	// set the network node condition to configured
	if err := d.Configured(); err != nil {
		d.log.Debug(errSetNetworkNodeStatus)
	}

	// discover the device type
	for {
		cap, err := d.Target.DeviceCapabilities(d.ctx)
		if err != nil {
			// set the network node condition to not ready
			d.NotReady(string(fmt.Sprintf("%s", err)))
			// retry in 60 sec
			time.Sleep(notReadyTimeout)
			continue
		} else {
			// get device type based on registered data
			deviceType := d.getDeviceType(cap)
			d.log.Debug("deviceType info", "deviceType", deviceType)

			// initialize the device driver based on the discovered device type
			deviceInitializer, ok := devices.Devices[deviceType]
			if !ok {
				// set the network node condition to not ready
				d.NotReady(errDeviceNotRegistered)
				// retry in 60 sec
				time.Sleep(notReadyTimeout)
				continue
			}
			d.Device = deviceInitializer()
			if err := d.Device.Init(
				devices.WithLogging(d.log.WithValues("device", d.DeviceName)),
				devices.WithTarget(d.Target.GetTarget()),
			); err != nil {
				// set the network node condition to not ready
				d.NotReady(errDeviceInitFailed)
				// retry in 60 sec
				time.Sleep(notReadyTimeout)
				continue
			}

			// get device details through gnmi
			d.DeviceDetails, err = d.Device.Discover(d.ctx)
			if err != nil {
				// set the network node condition to not ready
				d.NotReady(errDeviceDiscoveryFailed)
				// retry in 60 sec
				time.Sleep(notReadyTimeout)
				continue
			}
			d.log.Debug("DeviceDetails", "info", d.DeviceDetails)

			// get initial config through gnmi
			d.InitialConfig, err = d.Device.GetConfig(d.ctx)
			if err != nil {
				// set the network node condition to not ready
				d.NotReady(errDeviceDiscoveryFailed)
				// retry in 60 sec
				time.Sleep(notReadyTimeout)
				continue
			}

			// clean config and provide a string ptr to map in the configmap
			var cfgStringptr *string
			d.InitialConfig, cfgStringptr, err = jsonutils.CleanConfig2String(d.InitialConfig)
			if err != nil {
				d.log.Debug("CleanConfig2String", "error", err)
			}

			// update configmap with the initial config
			if err := d.K8sApi.UpdateConfigMap(d.ctx, cfgStringptr); err != nil {
				// set the network node condition to not ready
				d.NotReady(errUpdateConfigMap)
				// retry in 60 sec
				time.Sleep(notReadyTimeout)
				continue
			}
			// bring the device in ready status
			if err := d.Ready(); err != nil {
				// set the network node condition to not ready
				d.NotReady(errSetNetworkNodeStatus)
				// retry in 60 sec
				time.Sleep(notReadyTimeout)
				continue
			}
			break
		}
	}

	d.log.Debug("Device is discovered")

	d.StopCh = make(chan struct{})
	defer close(d.StopCh)

	// start reconcile process
	go func() {
		d.StartReconcileProcess()
	}()

	select {
	case <-d.ctx.Done():
		d.log.Debug("context cancelled")
	}
	close(d.StopCh)

	return nil

}

func (d *deviceDriver) StartReconcileProcess() error {
	d.log.Debug("Starting reconciliation process...")
	timeout := make(chan bool, 1)
	timeout <- true
	d.Cache.SetNewProviderUpdates(true)
	for {
		select {
		case <-timeout:
			time.Sleep(reconcileTimer)
			timeout <- true

			// reconcile cache when:
			// -> new updates from k8s operator are received
			// -> autopilot is on and new onChange information was reveived that requires device updates
			// -> autopilot is on and new onChange information was received that requires to repply the cache

			if d.Cache.GetNewProviderUpdates() {
				d.Cache.Reconcile(d.ctx, d.Device)
			} else {
				fmt.Printf(".")
			}

		case <-d.StopCh:
			d.log.Debug("Stopping timer reconciliation process")
			return nil
		}
	}
}
