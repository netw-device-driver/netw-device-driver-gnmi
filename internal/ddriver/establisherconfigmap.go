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

	ndddvrv1 "github.com/netw-device-driver/ndd-core/apis/dvr/v1"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	errGetConfigMap    = "cannot get configmap"
	errUpdateConfigMap = "cannot update configmap"
)

// UpdateDeviceStatusReady updates a set of resources
func (e *APIEstablisher) UpdateConfigMap(ctx context.Context, cfg *string) error { // nolint:gocyclo
	cmKey := types.NamespacedName{
		Namespace: e.namespaceDeployment,
		Name:      strings.Join([]string{ndddvrv1.PrefixConfigmap, e.deviceName}, "-"),
	}
	cm := &corev1.ConfigMap{}
	if err := e.client.Get(ctx, cmKey, cm); err != nil {
		return errors.Wrap(err, errGetConfigMap)
	}
	if cm.Data == nil {
		cm.Data = make(map[string]string)
	}

	cm.Data[ndddvrv1.ConfigmapJsonConfig] = *cfg

	err := e.client.Update(ctx, cm)
	if err != nil {
		return errors.Wrap(err, errUpdateConfigMap)
	}
	e.log.Debug("updated configmap...")

	return nil
}
