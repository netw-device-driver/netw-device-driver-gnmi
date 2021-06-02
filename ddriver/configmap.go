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
	//nddv1 "github.com/netw-device-driver/netw-device-controller/api/v1"

	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

func (d *DeviceDriver) ConfigMapUpdate(jsonStr *string) error {

	cmKey := types.NamespacedName{
		Namespace: "nddriver-system",
		Name:      "nddriver-cm-" + *d.DeviceName,
	}
	cm := &corev1.ConfigMap{}
	if err := (*d.K8sClient).Get(d.Ctx, cmKey, cm); err != nil {
		log.WithError(err).Error("Failed to get configmap")
		return err
	}
	if cm.Data == nil {
		cm.Data = make(map[string]string)
	}

	cm.Data["config.json"] = *jsonStr

	err := (*d.K8sClient).Update(d.Ctx, cm)
	if err != nil {
		log.WithError(err).Error("Failed to update configmap")
		return err
	}
	log.Infof("updated configmap...")

	return nil
}
