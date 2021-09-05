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
	"fmt"
	"strings"

	config "github.com/netw-device-driver/ndd-grpc/config/configpb"
	"github.com/openconfig/gnmi/proto/gnmi"
	"github.com/wI2L/jsondiff"
	yparser "github.com/yndd/ndd-yang/pkg/parser"
)

func (c *Cache) ReconcileOnChange(resp *gnmi.SubscribeResponse) error {
	switch resp.GetResponse().(type) {
	case *gnmi.SubscribeResponse_Update:
		// handle deletes
		du := resp.GetUpdate().Delete
		// subscription DELETE per xpath
		for _, del := range du {
			path := c.parser.GnmiPath2ConfigPath(del)
			xpath := c.parser.ConfigGnmiPathToXPath(path, true)
			// check if this is a managed resource or unmanged resource
			// name == unmanagedResource is an unmanaged resource
			_, resourceName := c.FindManagedResource(*xpath)
			log := c.log.WithValues(
				"ReconcileOnChangeUpdates", "delete",
				"Cache Resource", resourceName,
				"Xpath", xpath,
			)
			// 1. update the cache currentconfig for both MR and UMR
			// 2. if the resource was not found we can ignore the change
			// 3. if the resource was found we delete the entry in the currentconfig
			// initialize a tracecontext
			tc := &yparser.TraceCtxt{}
			c.Lock()
			c.CurrentConfig, tc = c.DeleteObjectInConfig(c.CurrentConfig, path)
			c.Unlock()
			if !tc.Found {
				// object not found in the current config tree
				// we ignore this for both MR and UMR
				log.Debug("ReconcileOnChangeUpdates Delete Not found in Cache", "tc.idx", tc.Idx, "tc.msg", tc.Msg)
			} else {
				// object found in the current config tree
				// if this is a MR we need to update the status with the deviation
				if resourceName != unmanagedResource {
					log.Debug("ReconcileOnChangeUpdates Delete report as MR", "tc.idx", tc.Idx, "tc.msg", tc.Msg)
					/*
						c.AddDeviation(resourceLevel,
							resourceName,
							&config.Deviation{
								Path:      path,
								Operation: config.Deviation_Delete,
							},
						)
					*/
					/*
						if err := c.PublishEvent(resourceName); err != nil {
							log.Debug("Publish Event error", "Error", err)
							return err
						}
					*/
					c.UpdateResourceInGNMICache(resourceName)
				}
			}
		}
		// handle updates
		u := resp.GetUpdate().Update
		// subscription UPDATE per xpath
		for _, upd := range u {
			// copy path to config.Path and clean the path (avoid prefixes)
			path := c.parser.GnmiPath2ConfigPath(upd.GetPath())
			// get the data value of the update
			value, err := c.parser.GetValue(upd.GetVal())
			if err != nil {
				c.log.Debug("ReconcileOnChangeUpdates Update gnmic.GetValue", "Error", err)
				return err
			}
			// used in the comparison to see which items we need to delete
			// since leaflists and leafs in the same container get send in 2 seperate messages
			// with on change notifications from SRL
			valueType := c.parser.GetValueType(value)

			// provide an xpath which is easier for comparison and finding the resource
			xpath := c.parser.ConfigGnmiPathToXPath(path, true)
			// check if this is a managed resource or unmanged resource
			// name == unmanagedResource is an unmanaged resource
			_, resourceName := c.FindManagedResource(*xpath)

			log := c.log.WithValues("Cache Resource", resourceName, "Xpath", xpath)
			log.Debug("ReconcileOnChangeUpdates Update", "Value", value)
			// compare the updates with the cached data and
			// update the currentconfig if there are changes

			cacheValue, tc := c.GetObjectValueInConfig(c.CurrentConfig, path)
			if tc.Found {
				// object found in the current config tree
				// we need to check the delta
				// valueType is used to deal with lists as value
				patch, err := c.parser.CompareValues(path, cacheValue, value, valueType)
				if err != nil {
					log.Debug("ReconcileOnChangeUpdates Update", "Error", err)
				}
				if len(patch) == 0 {
					// all good we can rest a sleep
					log.Debug("ReconcileOnChangeUpdates Update Up to Date", "Value", value)
				} else {
					log.Debug("ReconcileOnChangeUpdates Update Out of sync", "Diff", patch, "Value", value, "TC", tc)
					for _, operation := range patch {
						switch operation.Type {
						case jsondiff.OperationReplace:
							// happens when a leaf get changed: admin-state enabled -> disabled
							path.Elem = append(path.Elem, &config.PathElem{
								// remoe the
								Name: strings.ReplaceAll(operation.Path.String(), "/", ""),
							})
							c.Lock()
							c.CurrentConfig, _ = c.UpdateObjectInConfig(c.CurrentConfig, path, operation.Value)
							c.Unlock()
						case jsondiff.OperationAdd:
							// happens when a leaf gets added to a leaflist: add mtu: 2000 to interface list
							path.Elem = append(path.Elem, &config.PathElem{
								Name: strings.ReplaceAll(operation.Path.String(), "/", ""),
							})
							c.Lock()
							c.CurrentConfig, _ = c.CreateObjectInConfig(c.CurrentConfig, path, operation.Value)
							c.Unlock()
						default:
							log.Debug("Json Patch difference", "Operation", operation)
						}
					}

					// we just report the deviation for history reasons
					if resourceName != unmanagedResource {
						/*
							cacheByteValue, err := json.Marshal(cacheValue)
							if err != nil {
								log.Debug("ReconcileOnChangeUpdates Update Marshal error", "Error", err)
								return err
							}
							byteValue, err := json.Marshal(value)
							if err != nil {
								log.Debug("ReconcileOnChangeUpdates Update Marshal error", "Error", err)
								return err
							}
							c.AddDeviation(resourceLevel,
								resourceName,
								&config.Deviation{
									Path:      path,
									OldValue:  cacheByteValue,
									Value:     byteValue,
									Operation: config.Deviation_Replace,
								},
							)
						*/
						c.UpdateResourceInGNMICache(resourceName)
					}

				}
			} else {
				// create the entry in the tree
				// compare the updates with the cached data and
				// update the currentconfig if there are changes
				c.CurrentConfig, _ = c.CreateObjectInConfig(c.CurrentConfig, path, value)
				log.Debug("ReconcileOnChangeUpdates Update Resource Not Found in Cache -> Create in Tree", "tc.idx", tc.Idx, "tc.msg", tc.Msg)
				// if this is a MR we just report the deviation for history reasons
				if resourceName != unmanagedResource {
					// marshal the value before adding to the deviation
					/*
						if err := c.PublishEvent(resourceName); err != nil {
							log.Debug("Publish Event error", "Error", err)
							return err
						}
					*/
					/*
						byteValue, err := json.Marshal(value)
						if err != nil {
							log.Debug("ReconcileOnChangeUpdates Update Marshal error", "Error", err)
							return err
						}
						c.AddDeviation(resourceLevel,
							resourceName,
							&config.Deviation{
								Path:      path,
								Value:     byteValue,
								Operation: config.Deviation_Add,
							},
						)
					*/
					c.UpdateResourceInGNMICache(resourceName)
				}
			}
		}
	case *gnmi.SubscribeResponse_SyncResponse:
		// handled for ...
	}
	fmt.Printf("Config After On Change Reconcile: %v\n", c.GetCurrentConfig())
	//c.log.Debug("Config After On Change Reconcile", "config", c.GetCurrentConfig())
	return nil
}

/*
func (c *Cache) PublishEvent(resourceName string) error {
	gvk, err := gvk.String2GVK(resourceName)
	if err != nil {
		c.log.Debug("Cannot get gvk", "error", err, "externalResourceName", resourceName, "gvk", gvk)
		return errors.Wrap(err, fmt.Sprintf("Cannot get gvk with externalResourceName %s, gvk: %v", resourceName, gvk))
	}

	mr := &unstructured.Unstructured{}
	mr.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   gvk.GetGroup(),
		Kind:    gvk.GetKind(),
		Version: gvk.GetVersion(),
	})

	key := types.NamespacedName{
		Namespace: gvk.GetNameSpace(),
		Name:      gvk.GetName(),
		//Name:      split[len(split)-1],
	}
	if err := c.client.Get(c.ctx, key, mr); err != nil {
		c.log.Debug("Cannot get external resource", "error", err, "externalResourceName", resourceName, "gvk", gvk)
		return errors.Wrap(err, fmt.Sprintf("Cannot get external resource with externalResourceName %s, gvk: %v", resourceName, gvk))

	}
	c.record.Event(mr, event.Normal("resource modified", "test"))

	return nil


}
*/

/*
	t := metav1.Now()
	c.client.Create(c.ctx, &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: reason + "-",
			Namespace:    o.ObjectMeta.Namespace,
		},
		InvolvedObject: corev1.ObjectReference{
			Kind:       gvk.GetKind(),
			Namespace:  o.Namespace,
			Name:       o.Name,
			UID:        o.UID,
			APIVersion: GroupVersion.String(),
		},
		Reason:  "reason",
		Message: message,
		Source: corev1.EventSource{
			Component: "srl-controller",
		},
		FirstTimestamp:      t,
		LastTimestamp:       t,
		Count:               1,
		Type:                corev1.EventTypeNormal,
		ReportingController: "srlinux.henderiw.be/srl-controller",
	})
*/
