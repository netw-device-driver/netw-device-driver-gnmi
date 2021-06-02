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

func (d *DeviceDriver) NetworkNodeUpdate(dDetails *nddv1.DeviceDetails, status nddv1.DiscoveryStatus) error {

	nnKey := types.NamespacedName{
		Namespace: "default",
		Name:      *d.DeviceName,
	}
	nn := &nddv1.NetworkNode{}
	if err := (*d.K8sClient).Get(d.Ctx, nnKey, nn); err != nil {
		log.WithError(err).Error("Failed to get NetworkNode")
		return err
	}

	grpcServer := &nddv1.GrpcServerDetails{
		Port: d.CacheServerPort,
	}

	log.Infof("DiscoveryStatus: %v", status)
	nn.SetDiscoveryStatus(status)
	nn.Status.DeviceDetails = dDetails
	nn.Status.GrpcServer = grpcServer

	if err := d.saveNetworkNodeStatus(d.Ctx, nn); err != nil {
		return err
	}

	return nil
}

func (d *DeviceDriver) saveNetworkNodeStatus(ctx context.Context, nn *nddv1.NetworkNode) error {
	t := metav1.Now()
	nn.Status.DeepCopy()
	nn.Status.LastUpdated = &t

	log.Infof("Network Node status, discovery status %v", *nn.Status.DiscoveryStatus)

	if err := (*d.K8sClient).Status().Update(ctx, nn); err != nil {
		log.WithError(err).Error("Failed to update network node status ")
		return err
	}
	return nil
}
