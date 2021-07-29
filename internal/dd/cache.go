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
	"sync"

	"github.com/netw-device-driver/ndd-runtime/pkg/logging"
	"github.com/netw-device-driver/netwdevpb"
	"github.com/openconfig/gnmi/proto/gnmi"
)

type Cache struct {
	netwdevpb.UnimplementedCacheStatusServer
	netwdevpb.UnimplementedCacheUpdateServer

	Mutex                 sync.RWMutex
	NewK8sOperatorUpdates bool
	Data                  map[int]map[string]*ResourceData
	Levels                []int
	NewOnChangeUpdates    bool
	OnChangeReApplyCache  bool
	OnChangeDeletes       []string
	OnChangeUpdates       []string
	OnChangeDeviations    map[string]*Deviation
	CurrentConfig         []byte

	log logging.Logger
}

type ResourceData struct {
	Config      *netwdevpb.CacheUpdateRequest
	CacheStatus netwdevpb.CacheStatusReply_CacheResourceStatus // Status of the resource
}

type Deviation struct {
	OnChangeAction  netwdevpb.Deviation_OnChangeAction
	Pel             []*gnmi.PathElem
	Value           []byte
	DeviationAction netwdevpb.Deviation_DeviationAction
	Change          bool
}

// CacheOption can be used to manipulate Options.
type CacheOption func(*Cache)

// WithCacheLogger specifies how the Reconciler should log messages.
func WithCacheLogger(log logging.Logger) CacheOption {
	return func(o *Cache) {
		o.log = log
	}
}

func NewCache(opts ...CacheOption) *Cache {
	var x1 interface{}
	empty, err := json.Marshal(x1)
	if err != nil {
		// TODO should this be better error handled
		return nil
	}

	c := &Cache{
		Data:               make(map[int]map[string]*ResourceData),
		Levels:             make([]int, 0),
		OnChangeDeletes:    make([]string, 0),
		OnChangeUpdates:    make([]string, 0),
		OnChangeDeviations: make(map[string]*Deviation),
		CurrentConfig:      empty,
	}
	for _, opt := range opts {
		opt(c)
	}

	return c
}

// Request is a GRPC service that provides the cache status
func (c *Cache) Request(ctx context.Context, req *netwdevpb.CacheStatusRequest) (*netwdevpb.CacheStatusReply, error) {
	c.log.WithValues("Resource", req.Resource, "Level", req.Level)
	c.log.Debug("Cache Status Request")
	reply := &netwdevpb.CacheStatusReply{}
	if d, ok := c.Data[int(req.Level)][req.Resource]; ok {
		//reply := &netwdevpb.CacheStatusReply{}
		reply.Exists = true
		reply.Status = d.CacheStatus
		reply.Data = d.Config
		c.log.Debug("Cache Status", "reply", reply)
		return reply, nil
	}
	reply.Exists = false
	c.log.Debug("Cache Status", "reply", reply)
	return reply, nil
}

// Update is a GRPC service that updates the cache with new information
func (c *Cache) Update(ctx context.Context, req *netwdevpb.CacheUpdateRequest) (*netwdevpb.CacheUpdateReply, error) {
	c.log.WithValues("Resource", req.Resource, "Level", req.Level)
	c.log.Debug("Cache Update Request")
	c.Mutex.Lock()

	level := int(req.Level)
	if !contains(c.Levels, level) {
		c.Levels = append(c.Levels, level)
		c.Data[level] = make(map[string]*ResourceData)
	}
	sort.Ints(c.Levels)

	c.Data[level][req.Resource] = &ResourceData{
		Config:      req,
		CacheStatus: netwdevpb.CacheStatusReply_CacheResourceStatusToBeProcessed,
	}
	c.NewK8sOperatorUpdates = true
	c.Mutex.Unlock()
	reply := &netwdevpb.CacheUpdateReply{}
	return reply, nil
}

func (c *Cache) Lock() {
	c.Mutex.RLock()
}

func (c *Cache) Unlock() {
	c.Mutex.RUnlock()
}
