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
	"encoding/json"
	"sort"

	config "github.com/netw-device-driver/ndd-grpc/config/configpb"
	"github.com/netw-device-driver/netw-device-driver-gnmi/internal/devices"
)

func (c *Cache) Reconcile(ctx context.Context, Device devices.Device) {
	c.log.Debug("Reconcile start...")
	// sort the levels
	c.log.Debug("Data", "data", c.Data)
	levels := make([]int, 0, len(c.Data))
	for level := range c.Data {
		c.log.Debug("Level", "level", level)
		levels = append(levels, level)
	}
	sort.Ints(levels)

	c.log.Debug("Levels", "levels", levels)

	// process deletes first
	delObjects := make([]*config.Status, 0)
	for _, level := range levels {
		for _, resourceData := range c.Data[level] {
			// all the dependencies should be taken care of with the leafref validations
			// in the provider
			// maybe aggregating some deletes if they have a parent dependency might be needed
			if resourceData.GetStatus() == config.Status_DeletePending {
				delObjects = append(delObjects, resourceData)
			}
		}
	}

	for _, delObject := range delObjects {
		c.log.Debug("Delete Object", "delObject", delObject)

		if delObject.GetPath() != nil {
			c.log.Debug("Delete Object", "Path", delObject.GetPath())
			_, err := Device.Delete(ctx, gnmiPathToXPath(delObject.GetPath()))
			if err != nil {
				// TODO we should fail
				c.log.Debug("GNMI delete process failed", "ResourceName", delObject.Name, "Path", delObject.GetPath(), "Error", err)
			}
			c.DeleteResource(int(delObject.GetLevel()), delObject.GetName())
		} else {
			c.log.Debug("Delete Object with empty patn is suicide so we cannot let this happen", "Path", delObject.GetPath())
		}

	}

	// process creates/updates
	// Here we merge all the resources to optimize the transactions to the device
	var mergedPath = &config.Path{}
	var mergedData interface{}
	updateObjects := make([]*config.Status, 0)
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
			}
		}
	}
	if !c.IsEqual(mergedData) {
		d, err := json.Marshal(mergedData)
		if err != nil {
			c.log.Debug("Marshal failed", "Error", err)
			// TODO we should fail
		}
		// if the data is empty, there is no need for an update
		if string(d) != "null" {
			c.log.Debug("Gnmi Update", "Path", mergedPath, "Data", mergedData)
			_, err = Device.Update(ctx, gnmiPathToXPath(mergedPath), d)
			if err != nil {
				c.log.Debug("GNMI update process failed", "Error", err)
				// TODO we should fail
				for _, updateObject := range updateObjects {
					c.SetStatus(int(updateObject.GetLevel()), updateObject.GetName(), config.Status_Failed)
				}
			}
		} else {
			c.log.Debug("Gnmi Data emptyUpdate")
		}
		c.SetCurrentConfig(mergedData)

	}
	for _, updateObject := range updateObjects {
		c.SetStatus(int(updateObject.GetLevel()), updateObject.GetName(), config.Status_Success)
	}

	c.SetNewProviderUpdates(false)
	c.log.Debug("Show CACHE STATUS after UPDATE")
	c.ShowStatus()
	c.log.Debug("Current Config", "config", c.GetCurrentConfig())
}
