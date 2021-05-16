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
	"fmt"
	"strings"

	nddv1 "github.com/netw-device-driver/netw-device-controller/api/v1"
	"github.com/netw-device-driver/netw-device-driver-gnmi/pkg/gnmic"
	"github.com/openconfig/gnmi/proto/gnmi"
	log "github.com/sirupsen/logrus"
)

// DiscoverDeviceDetailsSRL discovers the device details
func (d *DeviceDriver) DiscoverDeviceDetailsSRL() (*nddv1.DeviceDetails, error) {
	var err error
	var p string
	var req *gnmi.GetRequest
	var rsp *gnmi.GetResponse
	dDetails := &nddv1.DeviceDetails{}
	// /system/name/host-name
	// /platform/control[slot="*"]

	log.Info("Discover SRL details ...")

	p = "/system/app-management/application[name=idb_server]"
	req, err = gnmic.CreateGetRequest(&p, stringPtr("STATE"), stringPtr("JSON_IETF"))
	if err != nil {
		return nil, err
	}
	rsp, err = d.Target.Get(d.Ctx, req)
	if err != nil {
		return nil, err
	}
	u, err := gnmic.HandleGetResponse(rsp)
	if err != nil {
		return nil, err
	}
	for _, update := range u {
		// we expect a single response in the get since we target the explicit resource
		switch x := update.Values["application"].(type) {
		case map[string]interface{}:
			for k, v := range x {
				sk := strings.Split(k, ":")[len(strings.Split(k, ":"))-1]
				switch sk {
				case "version":
					log.Info("set sw version type...")
					dDetails.SwVersion = &strings.Split(fmt.Sprintf("%v", v), "-")[0]
				}
			}

		}
		log.Infof("gnmi idb application information: update response %v", update)
	}

	log.Infof("Device details sw version %v", dDetails.SwVersion)

	p = "/platform/chassis"
	req, err = gnmic.CreateGetRequest(&p, stringPtr("STATE"), stringPtr("JSON_IETF"))
	if err != nil {
		return nil, err
	}
	rsp, err = d.Target.Get(d.Ctx, req)
	if err != nil {
		return nil, err
	}

	u, err = gnmic.HandleGetResponse(rsp)
	if err != nil {
		return nil, err
	}
	for _, update := range u {
		// we expect a single response in the get since we target the explicit resource
		switch x := update.Values["chassis"].(type) {
		case map[string]interface{}:
			for k, v := range x {
				sk := strings.Split(k, ":")[len(strings.Split(k, ":"))-1]
				switch sk {
				case "type":
					log.Info("set hardware type...")
					dDetails.Kind = stringPtr(fmt.Sprintf("%v", v))
					//g.nn.SetHardwareDetails("type", fmt.Sprintf("%v", v))
				case "serial-number":
					log.Info("set serial number...")
					dDetails.SerialNumber = stringPtr(fmt.Sprintf("%v", v))
					//g.nn.SetHardwareDetails("serial-number", fmt.Sprintf("%v", v))
				case "mac-address":
					log.Info("set mac address...")
					dDetails.MacAddress = stringPtr(fmt.Sprintf("%v", v))
					//g.nn.SetHardwareDetails("mac-address", fmt.Sprintf("%v", v))
				default:
				}
			}
		}

		log.Infof("gnmi platform information: update response %v", update)

	}
	log.Infof("Device details %v", dDetails)

	return dDetails, nil

}

func (d *DeviceDriver) GetLatestConfig() (map[string]interface{}, error) {
	var err error
	var p string
	var req *gnmi.GetRequest
	var rsp *gnmi.GetResponse

	p = "/"
	req, err = gnmic.CreateGetRequest(&p, stringPtr("CONFIG"), stringPtr("JSON_IETF"))
	if err != nil {
		return nil, err
	}
	rsp, err = d.Target.Get(d.Ctx, req)
	if err != nil {
		return nil, err
	}

	u, err := gnmic.HandleGetResponse(rsp)
	if err != nil {
		return nil, err
	}

	for _, update := range u {
		log.Infof("GetLatestConfig response: %v", update)
		return update.Values, nil
	}

	return nil, nil

}
