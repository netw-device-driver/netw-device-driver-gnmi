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

	ndddvrv1 "github.com/netw-device-driver/ndd-core/apis/dvr/v1"
	"github.com/netw-device-driver/ndd-runtime/pkg/logging"
	"github.com/netw-device-driver/ndd-runtime/pkg/resource"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/types"
)

const (
	errGetNetworkNode = "cannot get network node"
	errUpdateStatus   = "cannot update network node status"
)

// An Establisher brings up or down a set of resources in the API server
type Establisher interface {
	// Update device status to ready state
	UpdateDeviceStatusReady(ctx context.Context, dd *ndddvrv1.DeviceDetails) error

	// Update device status to not ready state
	UpdateDeviceStatusNotReady(ctx context.Context, dd *ndddvrv1.DeviceDetails) error

	// Update config map
	UpdateConfigMap(ctx context.Context, cfg *string) error
}

// APIEstablisher establishes control or ownership of resources in the API
// server for a parent.
type APIEstablisher struct {
	client              resource.ClientApplicator
	log                 logging.Logger
	namespaceConfig     string
	namespaceDeployment string
	deviceName          string
}

// NewAPIEstablisher creates a new APIEstablisher.
func NewAPIEstablisher(client resource.ClientApplicator, log logging.Logger, deviceName, nsConfig, nsDeployment string) *APIEstablisher {
	return &APIEstablisher{
		client:              client,
		log:                 log,
		namespaceConfig:     nsConfig,
		namespaceDeployment: nsDeployment,
		deviceName:          deviceName,
	}
}

// UpdateDeviceStatusReady updates a set of resources
func (e *APIEstablisher) UpdateDeviceStatusReady(ctx context.Context, dDetails *ndddvrv1.DeviceDetails) error { // nolint:gocyclo
	nnKey := types.NamespacedName{
		Namespace: e.namespaceConfig,
		Name:      e.deviceName,
	}
	nn := &ndddvrv1.NetworkNode{}
	if err := e.client.Get(ctx, nnKey, nn); err != nil {
		return errors.Wrap(err, errGetNetworkNode)
	}
	nn.SetDeviceDetails(dDetails)
	nn.SetConditions(ndddvrv1.Discovered())

	return errors.Wrap(e.client.Status().Update(ctx, nn), errUpdateStatus)
}

// UpdateDeviceStatusReady updates a set of resources
func (e *APIEstablisher) UpdateDeviceStatusNotReady(ctx context.Context, dDetails *ndddvrv1.DeviceDetails) error { // nolint:gocyclo
	nnKey := types.NamespacedName{
		Namespace: e.namespaceConfig,
		Name:      e.deviceName,
	}
	nn := &ndddvrv1.NetworkNode{}
	if err := e.client.Get(ctx, nnKey, nn); err != nil {
		return errors.Wrap(err, errGetNetworkNode)
	}
	nn.SetDeviceDetails(dDetails)
	nn.SetConditions(ndddvrv1.NotDiscovered())

	return errors.Wrap(e.client.Status().Update(ctx, nn), errUpdateStatus)
}
