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

	nddv1 "github.com/netw-device-driver/netw-device-controller/api/v1"
	log "github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func (d *DeviceDriver) NetworkDeviceUpdate(dDetails *nddv1.DeviceDetails, status nddv1.DiscoveryStatus) error {

	ndKey := types.NamespacedName{
		Namespace: "default",
		Name:      *d.DeviceName,
	}
	nd := &nddv1.NetworkDevice{}
	if err := (*d.K8sClient).Get(d.Ctx, ndKey, nd); err != nil {
		log.WithError(err).Error("Failed to get NetworkNode")
		return err
	}

	nd.Status = nddv1.NetworkDeviceStatus{
		DiscoveryStatus: &status,
		DeviceDetails:   dDetails,
	}

	if err := d.saveNetworkDeviceStatus(d.Ctx, nd); err != nil {
		return err
	}

	return nil
}

func (d *DeviceDriver) saveNetworkDeviceStatus(ctx context.Context, nd *nddv1.NetworkDevice) error {
	t := metav1.Now()
	nd.Status.DeepCopy()
	nd.Status.LastUpdated = &t

	log.Info("Network Node status",
		"status", nd.Status)

	if err := (*d.K8sClient).Status().Update(ctx, nd); err != nil {
		log.WithError(err).Error("Failed to update network device status ")
		return err
	}
	return nil
}
