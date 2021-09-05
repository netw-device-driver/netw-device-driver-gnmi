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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/netw-device-driver/ndd-runtime/pkg/gext"
	"github.com/netw-device-driver/netw-device-driver-gnmi/internal/devices"
	"github.com/openconfig/gnmi/proto/gnmi"
	yparser "github.com/yndd/ndd-yang/pkg/parser"
)

func (c *Cache) Reconcile(ctx context.Context, Device devices.Device) {
	c.log.Debug("Reconcile start...")
	fmt.Printf("Current Config : %v \n", c.GetCurrentConfig())
	// sort the levels
	c.log.Debug("Data", "data", c.Data2)
	levels := make([]int, 0, len(c.Data2))
	for level := range c.Data2 {
		c.log.Debug("Level", "level", level)
		levels = append(levels, level)
	}
	sort.Ints(levels)

	c.log.Debug("Levels", "levels", levels)

	// process deletes first
	delObjects := make([]*resourceData, 0)
	delPaths := make([]*gnmi.Path, 0)
	for _, level := range levels {
		for _, resourceData := range c.Data2[level] {
			// all the dependencies should be taken care of with the leafref validations
			// in the provider
			// maybe aggregating some deletes if they have a parent dependency might be needed
			if resourceData.GetStatus() == gext.ResourceStatusDeletePending {
				delObjects = append(delObjects, resourceData)
				delPaths = append(delPaths, resourceData.GetPath())
			}
		}
	}
	// delete the paths if there are present in a single transaction
	if len(delPaths) != 0 {
		murder := false
		for _, delPath := range delPaths {
			if delPath == nil {
				murder = true
			}
		}
		if !murder {
			_, err := Device.DeleteGnmi(ctx, delPaths)
			if err != nil {
				// TODO we should fail in certain consitions
				// we keep the status in DeletePending to retry
				c.log.Debug("GNMI delete process failed", "Paths", delPaths, "Error", err)
			} else {
				c.log.Debug("GNMI delete process success", "Paths", delPaths)
				for _, delObject := range delObjects {
					// delete the resource in the cache
					c.DeleteResource2(int(delObject.GetLevel()), delObject.GetName())
					// delete the data in the current config
					tc := &yparser.TraceCtxtGnmi{}
					c.Lock()
					c.log.Debug("Delete Object", "Path", delObject.GetPath())
					c.CurrentConfig, tc = c.DeleteObjectGnmi(c.CurrentConfig, delObject.GetPath())
					c.Unlock()
					if !tc.Found {
						c.log.WithValues(
							"Cache Resource", delObject.Name,
							"Xpath", delObject.GetPath(),
						).Debug("Reconcile Delete Not found", "tc", tc.Idx, "tc.msg", tc.Msg)
					}
				}
			}
		}
	}

	/*
		for _, delObject := range delObjects {
			c.log.Debug("Delete Object", "delObject", delObject)

			if delObject.GetPath() != nil {
				c.log.Debug("Delete Object", "Path", delObject.GetPath())
				delpaths := make([]*config.Path, 0)
				delpaths = append(delpaths, delObject.GetPath())
				_, err := Device.Delete(ctx, delpaths)
				if err != nil {
					// TODO we should fail in certain consitions
					// we keep the status in DeletePending to retry
					c.log.Debug("GNMI delete process failed", "ResourceName", delObject.Name, "Path", delObject.GetPath(), "Error", err)
				} else {
					// delete was successfull
					c.log.Debug("GNMI delete process success", "ResourceName", delObject.Name, "Path", delObject.GetPath())
					// delete the resource in the cache
					c.DeleteResource(int(delObject.GetLevel()), delObject.GetName())
					// delete the data in the current config
					tc := &yparser.TraceCtxt{}
					c.CurrentConfig, tc = c.DeleteObjectInConfig(c.CurrentConfig, delObject.GetPath())
					if !tc.Found {
						c.log.WithValues(
							"Cache Resource", delObject.Name,
							"Xpath", delObject.GetPath(),
						).Debug("Reconcile Delete Not found", "tc", tc.Idx, "tc.msg", tc.Msg)
					}
				}
			} else {
				c.log.Debug("Delete Object with empty patn is suicide so we cannot let this happen", "Path", delObject.GetPath())
			}
		}
	*/

	// process creates/updates
	// Here we merge all the resources to optimize the transactions to the device
	/* OLD APPROACH
	var mergedPath = &config.Path{}
	mergedData := c.GetCurrentConfig()
	updateObjects := make([]*config.Status, 0)
	doUpdate := false
	for _, level := range levels {
		c.log.Debug("Level", "level", level)
		for _, resourceData := range c.Data[level] {
			c.log.Debug("resource info", "resourcePath", resourceData.Path, "status", resourceData.Status)
			if resourceData.GetStatus() != config.Status_DeletePending {
				updateObjects = append(updateObjects, resourceData)
				var x1 interface{}
				json.Unmarshal(resourceData.Data, &x1)

				c.log.Debug("Path", "path", resourceData.GetPath(), "data", x1)
				mergedPath, mergedData = startMerge(mergedPath, resourceData.GetPath(), mergedData, x1)

				c.log.Debug("Merge Info", "mergedPath", mergedPath, "mergedData", mergedData)
				doUpdate = true
			}
		}
	}
	OLD APPROACH */
	// NEW APPROACH with individual paths per leaflist, list, etc; rather than applying a big blob
	var mergedPath = &gnmi.Path{}
	doUpdate := false
	//mergedData := c.GetCurrentConfig()
	mergedData, err := c.parser.DeepCopy(c.CurrentConfig)
	if err != nil {
		c.log.Debug("Error copying currentconfig -> mergedData")
	}
	updateObjects := make([]*resourceData, 0)

	fmt.Printf("Current Config after delete merged Data: %v \n", mergedData)

	// we merge are the data of the verious resources in the config, since we
	// can handle this in a single transaction to the device
	for _, level := range levels {
		for _, resData := range c.Data2[level] {
			c.log.Debug("resource info", "resourcePath", resData.Path, "status", resData.Status)
			// only merge the data that is create pending since the other resource should have been
			if resData.GetStatus() != gext.ResourceStatusDeletePending {
				c.log.Debug("resource info", "resourcePath", resData.Path, "updates", resData.GetUpdate())
				for _, update := range resData.GetUpdate() {
					c.log.Debug("resource info", "resourcePath", resData.Path, "update", update)
					// append the resource for updating the status later when we configure the device
					// with the status of the result of the transaction
					updateObjects = append(updateObjects, resData)
					// unmarshal the data for insertion in the JSON Tree
					x1, err := c.parser.GetValue(update.Val)
					if err != nil {
						// TODO better error handling
						c.log.Debug("Error getiing value")
					}
					// initialize a tracecontext
					tc := &yparser.TraceCtxtGnmi{}
					c.Lock()
					mergedData, tc = c.CreateObjectGnmi(mergedData, update.Path, x1)
					c.Unlock()
					c.log.Debug("Update Current Config", "TraceCtxt", tc)
					doUpdate = true
				}
			}
		}
	}

	fmt.Printf("Current Config after update merged Data: %v \n", mergedData)

	//if !c.IsEqual(mergedData) {
	if doUpdate {
		d, err := json.Marshal(mergedData)
		if err != nil {
			c.log.Debug("Marshal failed", "Error", err)
			// TODO we should fail
		}
		// if the data is empty, there is no need for an update
		if string(d) != "null" {
			c.log.Debug("Gnmi Update", "Path", mergedPath, "Data", mergedData)

			updates := make([]*gnmi.Update, 0)
			updates = append(updates, &gnmi.Update{
				Path: &gnmi.Path{}, // this is the roo path
				//Val:  &gnmi.TypedValue{Value: &gnmi.TypedValue_JsonIetfVal{JsonIetfVal: d}},
				Val: &gnmi.TypedValue{
					Value: &gnmi.TypedValue_JsonIetfVal{
						JsonIetfVal: bytes.Trim(d, " \r\n\t"),
					},
				},
			})
			if err != nil {
				c.log.Debug("Error copying mergedData -> currentconfig")
			}
			_, err = Device.UpdateGnmi(ctx, updates)
			if err != nil {
				c.log.Debug("GNMI update process failed", "Error", err)
				// TODO we should fail
				for _, updateObject := range updateObjects {
					c.SetStatus2(int(updateObject.GetLevel()), updateObject.GetName(), gext.ResourceStatusFailed)
				}
				c.CurrentConfig, err = c.parser.DeepCopy(mergedData)
				if err != nil {
					c.log.Debug("Error copying mergedData -> currentconfig")
				}
			} else {
				// update was successfull
				for _, updateObject := range updateObjects {
					c.SetStatus2(int(updateObject.GetLevel()), updateObject.GetName(), gext.ResourceStatusSuccess)
					//c.CurrentConfig = mergedData
				}
				// update the current config with the merged data
				c.CurrentConfig, err = c.parser.DeepCopy(mergedData)
			}
		} else {
			c.log.Debug("Gnmi Data emptyUpdate")
		}
	}

	// set the status to false if all succeeded, if not we keep retrying for the resource where the delete or update failed
	c.SetNewProviderUpdates(false)
	for _, level := range levels {
		for _, resourceData := range c.Data2[level] {
			if resourceData.GetStatus() != gext.ResourceStatusSuccess {
				c.SetNewProviderUpdates(true)
			}
		}
	}
	c.log.Debug("Show CACHE STATUS after UPDATE")
	c.ShowStatus2()
	fmt.Printf("Config After Config Reconcile: %v\n", c.GetCurrentConfig())
	//c.log.Debug("Config After Config Reconcile", "config", c.GetCurrentConfig())
}
