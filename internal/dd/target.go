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

	"github.com/karimra/gnmic/target"
	"github.com/karimra/gnmic/types"
	"github.com/netw-device-driver/ndd-runtime/pkg/logging"
	"github.com/openconfig/gnmi/proto/gnmi"
	"github.com/openconfig/gnmi/proto/gnmi_ext"
	"github.com/pkg/errors"
)

const (
	//errors
	errGetGnmiCapabilities = "cannot get gnmi capabilities"
)

// target is a struct to hold the target configuration
// used to handle gnmi capabilities
type Target struct {
	targetConfig *types.TargetConfig
	target       *target.Target
	log          logging.Logger
}

// TargetOption can be used to manipulate Options.
type TargetOption func(*Target)

// WithTargetLogger specifies how the object should log messages.
func WithTargetLogger(log logging.Logger) TargetOption {
	return func(o *Target) {
		o.log = log
	}
}

func WithTargetConfig(tc *types.TargetConfig) TargetOption {
	return func(o *Target) {
		o.targetConfig = tc
	}
}

func NewTarget(target *target.Target, opts ...TargetOption) *Target {
	t := &Target{
		target: target,
	}
	for _, opt := range opts {
		opt(t)
	}

	return t
}

func (t *Target) DeviceCapabilities(ctx context.Context) ([]*gnmi.ModelData, error) {
	t.log.Debug("verifying gnmi capabilities...")

	ext := new(gnmi_ext.Extension)
	resp, err := t.target.Capabilities(ctx, ext)
	if err != nil {
		return nil, errors.Wrap(err, errGetGnmiCapabilities)
	}
	//t.log.Debug("Gnmi Capability", "response", resp)

	return resp.SupportedModels, nil
}

func (t *Target) GetTarget() *target.Target {
	return t.target
}
