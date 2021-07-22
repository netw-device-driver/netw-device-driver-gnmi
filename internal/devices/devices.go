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

package devices

import (
	"context"

	"github.com/karimra/gnmic/collector"
	ndddvrv1 "github.com/netw-device-driver/ndd-core/apis/dvr/v1"
	nddv1 "github.com/netw-device-driver/ndd-runtime/apis/common/v1"
	"github.com/netw-device-driver/ndd-runtime/pkg/logging"
	"github.com/openconfig/gnmi/proto/gnmi"
)

/*
const (
	DeviceKindSRL  nddv1.DeviceKind = "nokia-srl"
	DeviceKindSROS nddv1.DeviceKind = "nokia-sros"
)
*/

type Device interface {
	// Init initializes the device
	Init(...DeviceOption) error
	// WithTarget, initializes the device target
	WithTarget(target *collector.Target)
	// WithLogging initializes the device logging
	WithLogging(log logging.Logger)
	// Discover, discovers the device and its respective data
	Discover(ctx context.Context) (*ndddvrv1.DeviceDetails, error)
	// GetConfig, gets the config from the device
	GetConfig(ctx context.Context) (map[string]interface{}, error)
	// Get, gets the gnmi path from the tree
	Get(ctx context.Context, p *string) (map[string]interface{}, error)
	// Update, updates the gnmi path from the tree with the respective data
	Update(ctx context.Context, p *string, data []byte) (*gnmi.SetResponse, error)
	// Delete, deletes the gnmi path from the tree
	Delete(ctx context.Context, p *string) (*gnmi.SetResponse, error)
}

var Devices = map[nddv1.DeviceType]Initializer{}

type Initializer func() Device

func Register(name nddv1.DeviceType, initFn Initializer) {
	Devices[name] = initFn
}

type DeviceOption func(Device)

func WithTarget(target *collector.Target) DeviceOption {
	return func(d Device) {
		d.WithTarget(target)
	}
}

func WithLogging(log logging.Logger) DeviceOption {
	return func(d Device) {
		d.WithLogging(log)
	}
}
