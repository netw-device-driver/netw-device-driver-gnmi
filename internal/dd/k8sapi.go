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
	"os"
	"strings"

	ndrv1 "github.com/netw-device-driver/ndd-core/apis/dvr/v1"
	nddv1 "github.com/netw-device-driver/ndd-runtime/apis/common/v1"
	"github.com/netw-device-driver/ndd-runtime/pkg/logging"
	"github.com/netw-device-driver/ndd-runtime/pkg/utils"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	errSetNetworkNodeStatus = "cannot set network node status"
	errGetConfigMap         = "cannot get configmap"
	errUpdateConfigMap      = "cannot update configmap"
)

// K8sApi is a struct to hold the information to talk to the K8s api server
type K8sApi struct {
	client.Client
	NameSpaceResource string // this is the namespace the network node resource got deployed in, used for secrets, etc
	NameSpacePoD      string // this is the namespace on which the pod runs, which is used to get/update configmap
	DeviceName        string
	log               logging.Logger
}

// K8sApiOption can be used to manipulate Options.
type K8sApiOption func(*K8sApi)

// WithTargetLogger specifies how the object should log messages.
func WithK8sApiLogger(log logging.Logger) K8sApiOption {
	return func(o *K8sApi) {
		o.log = log
	}
}

// WithK8sApiClient initializes the client
func WithK8sApiClient(c client.Client) K8sApiOption {
	return func(o *K8sApi) {
		o.Client = c
	}
}

// WithClient initializes the client
func WithK8sApiNameSpace(s string) K8sApiOption {
	return func(o *K8sApi) {
		o.NameSpaceResource = s
	}
}

// WithClient initializes the client
func WithK8sApiDeviceName(s string) K8sApiOption {
	return func(o *K8sApi) {
		o.DeviceName = s
	}
}

func NewK8sApi(opts ...K8sApiOption) *K8sApi {
	a := &K8sApi{
		NameSpacePoD: os.Getenv("POD_NAMESPACE"),
	}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

func (a *K8sApi) GetNetworkNode(ctx context.Context) (*ndrv1.NetworkNode, error) {
	nnKey := types.NamespacedName{
		Namespace: a.NameSpaceResource,
		Name:      a.DeviceName,
	}
	nn := &ndrv1.NetworkNode{}
	if err := a.Get(ctx, nnKey, nn); err != nil {
		return nil, errors.Wrap(err, errGetNetworkNode)
	}
	return nn, nil
}

// SetNetworkNodeStatus updates the network device in not configured condition
func (a *K8sApi) SetNetworkNodeStatus(ctx context.Context, dDetails *ndrv1.DeviceDetails, c nddv1.Condition) error { // nolint:gocyclo
	nnKey := types.NamespacedName{
		Namespace: a.NameSpaceResource,
		Name:      a.DeviceName,
	}
	nn := &ndrv1.NetworkNode{}
	if err := a.Get(ctx, nnKey, nn); err != nil {
		return errors.Wrap(err, errSetNetworkNodeStatus)
	}
	a.log.Debug("Configmap info", "DeviceDetails", dDetails, "Conditions", c)
	nn.SetDeviceDetails(dDetails)
	nn.SetConditions(c)

	return errors.Wrap(a.Status().Update(ctx, nn), errSetNetworkNodeStatus)
}

// GetCredentials gets the username and apssword from the secret
func (a *K8sApi) GetCredentials(ctx context.Context, credName string) (*string, *string, error) { // nolint:gocyclo
	secretKey := types.NamespacedName{
		Namespace: a.NameSpaceResource,
		Name:      credName,
	}
	credsSecret := &corev1.Secret{}
	if err := a.Get(ctx, secretKey, credsSecret); err != nil {
		return nil, nil, errors.Wrap(err, errGetSecret)
	}

	return utils.StringPtr(strings.TrimSuffix(string(credsSecret.Data["username"]), "\n")),
		utils.StringPtr(strings.TrimSuffix(string(credsSecret.Data["password"]), "\n")),
		nil
}

// UpdateConfigMap updates the configmap with the respective data
func (a *K8sApi) UpdateConfigMap(ctx context.Context, cfg *string) error { // nolint:gocyclo
	cmKey := types.NamespacedName{
		Namespace: a.NameSpacePoD,
		Name:      strings.Join([]string{ndrv1.PrefixConfigmap, a.DeviceName}, "-"),
	}
	a.log.Debug("Configmap info", "cmKey", cmKey)

	cm := &corev1.ConfigMap{}
	if err := a.Get(ctx, cmKey, cm); err != nil {
		return errors.Wrap(err, errGetConfigMap)
	}
	if cm.Data == nil {
		cm.Data = make(map[string]string)
	}

	a.log.Debug("Configmap info", "configmap", cm)

	cm.Data[ndrv1.ConfigmapJsonConfig] = *cfg

	err := a.Update(ctx, cm)
	if err != nil {
		return errors.Wrap(err, errUpdateConfigMap)
	}
	a.log.Debug("updated configmap...")

	return nil
}
