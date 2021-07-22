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
	log "github.com/sirupsen/logrus"
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
func (c *Registrator) Register(ctx context.Context, req *netwdevpb.RegistrationRequest) (*netwdevpb.RegistrationReply, error) {
	log.Debug("Registration...")

	c.Subscriptions = req.Subscriptions
	c.ExceptionPaths = req.ExcpetionPaths
	c.ExplicitExceptionPaths = req.ExplicitExceptionPaths

	c.subCh <- true

	reply := &netwdevpb.RegistrationReply{}
	return reply, nil
}

func (c *Registrator) GetDeviceKind() string {
	return c.DeviceKind
}

func (c *Registrator) GetDeviceMatch() string {
	return c.DeviceMatch
}

func (c *Registrator) GetSubscriptions() []string {
	return c.Subscriptions
}

func (c *Registrator) GetExceptionPaths() []string {
	return c.ExceptionPaths
}

func (c *Registrator) GetExplicitExceptionPaths() []string {
	return c.ExplicitExceptionPaths
}
