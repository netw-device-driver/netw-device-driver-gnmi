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
	"strings"

	"github.com/karimra/gnmic/collector"
	nddv1 "github.com/netw-device-driver/ndd-runtime/apis/common/v1"
	"github.com/netw-device-driver/ndd-runtime/pkg/logging"
	"github.com/openconfig/gnmi/proto/gnmi_ext"
	"github.com/pkg/errors"
)

const (
	//errors
	errGetGnmiCapabilities = "cannot get gnmi capabilities"
)

// A Discoverer discovers device kinds
type Discoverer interface {
	// Discover discovers the device through gnmi
	Discover(ctx context.Context) (nddv1.DeviceType, error)
}

type DeviceDiscoverer struct {
	target *collector.Target
	log    logging.Logger
}

func NewDeviceDiscoverer(target *collector.Target, log logging.Logger) *DeviceDiscoverer {
	return &DeviceDiscoverer{
		target: target,
		log:    log,
	}
}

func (d *DeviceDiscoverer) Discover(ctx context.Context) (nddv1.DeviceType, error) {
	d.log.Debug("verifying gnmi capabilities...")

	ext := new(gnmi_ext.Extension)
	resp, err := d.target.Capabilities(ctx, ext)
	if err != nil {
		return nddv1.DeviceTypeUnknown, errors.Wrap(err, errGetGnmiCapabilities)
	}
	d.log.Debug("Gnmi Capability", "response", resp)

	for _, sm := range resp.SupportedModels {
		if strings.Contains(sm.Name, "srl_nokia") {
			return nddv1.DeviceTypeSRL, nil
		}
		if strings.Contains(sm.Name, "sros_nokia") {
			return nddv1.DeviceTypeSROS, nil
		}
	}
	return nddv1.DeviceTypeUnknown, nil
}
