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
	"fmt"
	"strings"

	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

func (d *DeviceDriver) InitSubscriptionExceptionPaths() error {
	// get ConfigMap information -> subscription and subscription exceptions
	var namespace string
	var cmName string
	switch *d.NetworkNodeKind {
	case "nokia_srl":
		namespace = "nddriver-system"
		cmName = "srl-k8s-subscription-config"
	}
	log.Infof("kind: %s", *d.NetworkNodeKind)
	log.Infof("namespace: %s", namespace)
	log.Infof("cmName: %s", cmName)
	cmKey := types.NamespacedName{
		Namespace: namespace,
		Name:      cmName,
	}
	cm := &corev1.ConfigMap{}
	if err := (*d.K8sClient).Get(d.Ctx, cmKey, cm); err != nil {
		return fmt.Errorf("Failed to get ConfigMap, error: %v", err)
	}

	// configmap with subscriptions and exception paths
	log.Infof("ConfigMap Data: %v", cm.Data)
	if ep, ok := cm.Data["exception-paths"]; ok {
		eps := strings.Split(ep, " ")
		//log.Infof("ConfigMap excption-paths data: %v", eps)
		d.ExceptionPaths = &eps
	}
	if d.ExceptionPaths != nil {
		log.Infof("ConfigMap exception-paths data: %v", *d.ExceptionPaths)
	}

	if eep, ok := cm.Data["explicit-exception-paths"]; ok {
		eeps := strings.Split(eep, " ")
		//log.Infof("ConfigMap explicit-exception-paths: data: %v", eeps)
		d.ExplicitExceptionPaths = &eeps
	}
	if d.ExplicitExceptionPaths != nil {
		log.Infof("ConfigMap explicit-exception-paths data: %v", *d.ExplicitExceptionPaths)
	}

	if sub, ok := cm.Data["subscriptions"]; ok {
		subs := strings.Split(sub, " ")
		//log.Infof("ConfigMap subscriptions data: %v", sps)
		d.Subscriptions = &subs
	}
	if d.Subscriptions != nil {
		log.Infof("ConfigMap subscriptions data: %v", *d.Subscriptions)
	}

	return nil
}
