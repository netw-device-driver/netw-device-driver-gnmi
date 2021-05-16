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
	"sort"

	"github.com/netw-device-driver/netwdevpb"
	log "github.com/sirupsen/logrus"
)

// Request is a GRPC service that provides the cache status
func (c *Cache) Request(ctx context.Context, req *netwdevpb.CacheStatusRequest) (*netwdevpb.CacheStatusReply, error) {
	log.Infof("Cache Status Request: Object: %v, Level: %v", req.Resource, req.Level)
	reply := &netwdevpb.CacheStatusReply{}
	if d, ok := c.Data[int(req.Level)][req.Resource]; ok {
		//reply := &netwdevpb.CacheStatusReply{}
		reply.Exists = true
		reply.Status = d.CacheStatus
		reply.Data = d.Config
		log.Debugf("Cache Status Reply: %v", reply)
		return reply, nil
	}
	reply.Exists = false
	return reply, nil
}

// Update is a GRPC service that updates the cache with new information
func (c *Cache) Update(ctx context.Context, req *netwdevpb.CacheUpdateRequest) (*netwdevpb.CacheUpdateReply, error) {
	log.Debugf("Cache Update Request: Object: %v, Level: %v", req.Resource, req.Level)
	log.Debugf("Cache Update Request: %v", req)
	c.Mutex.Lock()

	level := int(req.Level)
	if !contains(c.Levels, level) {
		c.Levels = append(c.Levels, level)
		c.Data[level] = make(map[string]*ResourceData)
	}
	sort.Ints(c.Levels)

	c.Data[level][req.Resource] = &ResourceData{
		Config:      req,
		CacheStatus: netwdevpb.CacheStatusReply_ToBeProcessed,
	}
	c.Mutex.Unlock()
	reply := &netwdevpb.CacheUpdateReply{}
	return reply, nil
}
