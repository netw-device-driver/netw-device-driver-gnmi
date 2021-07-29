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

	"github.com/netw-device-driver/ndd-runtime/pkg/logging"
	"github.com/netw-device-driver/netwdevpb"
)

type Registrator struct {
	netwdevpb.UnimplementedRegistrationServer

	DeviceKind             string
	DeviceMatch            string
	Subscriptions          []string
	ExceptionPaths         []string
	ExplicitExceptionPaths []string
	subCh                  chan bool
	log                    logging.Logger
}

func NewRegistrator(log logging.Logger, subCh chan bool) *Registrator {
	return &Registrator{
		subCh: subCh,
		log:   log,
	}
}

// Request is a GRPC service that provides the cache status
func (r *Registrator) Register(ctx context.Context, req *netwdevpb.RegistrationRequest) (*netwdevpb.RegistrationReply, error) {
	r.log.WithValues("register request", req)
	r.log.Debug("Registration...")

	r.Subscriptions = req.Subscriptions
	r.ExceptionPaths = req.ExcpetionPaths
	r.ExplicitExceptionPaths = req.ExplicitExceptionPaths

	//r.subCh <- true

	reply := &netwdevpb.RegistrationReply{}
	r.log.Debug("Registration reply...")
	return reply, nil
}

func (r *Registrator) GetDeviceKind() string {
	return r.DeviceKind
}

func (r *Registrator) GetDeviceMatch() string {
	return r.DeviceMatch
}

func (r *Registrator) GetSubscriptions() []string {
	return r.Subscriptions
}

func (r *Registrator) GetExceptionPaths() []string {
	return r.ExceptionPaths
}

func (r *Registrator) GetExplicitExceptionPaths() []string {
	return r.ExplicitExceptionPaths
}
