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

	register "github.com/netw-device-driver/ndd-grpc/register/registerpb"
	nddv1 "github.com/netw-device-driver/ndd-runtime/apis/common/v1"
	"github.com/netw-device-driver/ndd-runtime/pkg/logging"
)

type Register struct {
	register.UnimplementedRegistrationServer

	RegisteredDevices map[nddv1.DeviceType]*register.RegistrationInfo
	log               logging.Logger
}

// RegisterOption can be used to manipulate Options.
type RegisterOption func(*Register)

// WithRegisterLogger specifies how the Reconciler should log messages.
func WithRegisterLogger(log logging.Logger) RegisterOption {
	return func(o *Register) {
		o.log = log
	}
}

func NewRegister(opts ...RegisterOption) *Register {
	r := &Register{
		RegisteredDevices: make(map[nddv1.DeviceType]*register.RegistrationInfo),
	}
	for _, opt := range opts {
		opt(r)
	}

	return r
}

func (r *Register) Create(ctx context.Context, req *register.RegistrationInfo) (*register.DeviceType, error) {
	r.log.Debug("Register Create...")

	r.RegisteredDevices[nddv1.DeviceType(req.DeviceType)] = req

	//r.subCh <- true

	reply := &register.DeviceType{
		DeviceType: req.GetDeviceType(),
	}
	r.log.Debug("Register Create reply...")
	return reply, nil
}

func (r *Register) Read(ctx context.Context, req *register.DeviceType) (*register.RegistrationInfo, error) {
	r.log.Debug("Register Read...")

	reply := &register.RegistrationInfo{}

	// check if the device type was registered
	if _, ok := r.RegisteredDevices[nddv1.DeviceType(req.GetDeviceType())]; ok {
		reply.DeviceType = r.RegisteredDevices[nddv1.DeviceType(req.GetDeviceType())].DeviceType
	}

	r.log.Debug("Registration Read reply...")
	return reply, nil
}

func (r *Register) Update(ctx context.Context, req *register.RegistrationInfo) (*register.DeviceType, error) {
	r.log.Debug("Register Update...")

	r.RegisteredDevices[nddv1.DeviceType(req.DeviceType)] = req

	//r.subCh <- true

	reply := &register.DeviceType{
		DeviceType: req.GetDeviceType(),
	}
	r.log.Debug("Register Update reply...")
	return reply, nil
}

// DeRegister is a GRPC service that deregisters the device type
func (r *Register) Delete(ctx context.Context, req *register.DeviceType) (*register.DeviceType, error) {
	r.log.Debug("Registration Delete...")

	delete(r.RegisteredDevices, nddv1.DeviceType(req.GetDeviceType()))

	//r.subCh <- true

	reply := &register.DeviceType{
		DeviceType: req.GetDeviceType(),
	}
	r.log.Debug("Registration Delete reply...")
	return reply, nil
}

// GetDeviceTypes returns all devicetypes that are registered
func (r *Register) GetDeviceTypes() []nddv1.DeviceType {
	l := make([]nddv1.DeviceType, 0)
	for d := range r.RegisteredDevices {
		l = append(l, d)
	}
	return l
}

// GetDeviceMatches returns a map indexed by matchstring with element devicetype
func (r *Register) GetDeviceMatches() map[string]nddv1.DeviceType {
	l := make(map[string]nddv1.DeviceType)
	for d, o := range r.RegisteredDevices {
		l[o.MatchString] = d
	}
	return l
}

func (r *Register) GetSubscriptions(d nddv1.DeviceType) []string {
	if _, ok := r.RegisteredDevices[d]; !ok {
		return nil
	}
	return r.RegisteredDevices[d].Subscriptions
}

func (r *Register) GetExceptionPaths(d nddv1.DeviceType) []string {
	if _, ok := r.RegisteredDevices[d]; !ok {
		return nil
	}
	return r.RegisteredDevices[d].ExceptionPaths
}

func (r *Register) GetExplicitExceptionPaths(d nddv1.DeviceType) []string {
	if _, ok := r.RegisteredDevices[d]; !ok {
		return nil
	}
	return r.RegisteredDevices[d].ExplicitExceptionPaths
}
