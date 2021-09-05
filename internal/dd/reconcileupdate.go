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

	config "github.com/netw-device-driver/ndd-grpc/config/configpb"
	yparser "github.com/yndd/ndd-yang/pkg/parser"
)

func (c *Cache) ReconcileUpdate(ctx context.Context, req *config.Notification) error {
	// initialize a tracecontext
	tc := &yparser.TraceCtxt{}

	c.log.Debug("ReconcileUpdate", "DeletPaths", req.Delete, "UpdatePaths", req.Update)

	// we update the changes in a single transaction
	_, err := c.Device.Set(ctx, req.Update, req.Delete)
	if err != nil {
		// delete not successfull
		// TODO we should fail in certain conditions
		c.log.Debug("GNMI delete failed", "ResourceName", req.GetName(), "Paths", req.Delete, "Error", err)
		return err
	}
	// delete was successfull, update the cache
	for _, delPath := range req.Delete {
		c.log.Debug("GNMI delete success", "ResourceName", req.GetName(), "Path", delPath)
		c.Lock()
		c.CurrentConfig, tc = c.DeleteObjectInConfig(c.CurrentConfig, delPath)
		c.Unlock()
		if !tc.Found {
			c.log.WithValues(
				"Cache Resource", req.GetName(),
				"Xpath", delPath,
			).Debug("ReconcileUpdate Delete Not found", "tc", tc.Idx, "tc.msg", tc.Msg)
		}
	}

	// update was successfull, update the cache
	for _, update := range req.Update {
		c.log.Debug("GNMI update success", "ResourceName", req.GetName(), "Path", update.Path)

		var x1 interface{}
		json.Unmarshal(update.Value, &x1)
		c.Lock()
		c.CurrentConfig, tc = c.UpdateObjectInConfig(c.CurrentConfig, update.Path, x1)
		c.Unlock()
		if !tc.Found {
			c.log.WithValues(
				"Cache Resource", req.GetName(),
				"Xpath", update.Path,
			).Debug("ReconcileUpdate Not found", "tc", tc.Idx, "tc.msg", tc.Msg)
		}
	}
	return nil
}
