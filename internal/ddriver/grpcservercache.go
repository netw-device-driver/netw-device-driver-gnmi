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
	"encoding/json"
	"sort"
	"strings"
	"sync"

	"github.com/netw-device-driver/ndd-runtime/pkg/logging"
	"github.com/netw-device-driver/netwdevpb"
	"github.com/openconfig/gnmi/proto/gnmi"
)

/*
type Cache interface {
	netwdevpb.UnimplementedCacheStatusServer
	netwdevpb.UnimplementedCacheUpdateServer

	Request(ctx context.Context, req *netwdevpb.CacheStatusRequest) (*netwdevpb.CacheStatusReply, error)

	Update(ctx context.Context, req *netwdevpb.CacheUpdateRequest) (*netwdevpb.CacheUpdateReply, error)

	Lock()

	Unlock()

	SetOnChangeCacheUpdates(autoPilot bool, onChangeDeviations map[string]*Deviation)

	SetNewK8sOperatorUpdates(b bool)

	GetNewK8sOperatorUpdates() bool

	SetNewOnChangeUpdates(b bool)

	GetNewOnChangeUpdates() bool

	GetOnChangeDeviations() map[string]*Deviation

	SetOnChangeReApplyCache(b bool)

	GetOnChangeReApplyCache() bool

	GetOnChangeDeletes() []string

	GetOnChangeUpdates() []string

	SetStatus(l int, o string, s netwdevpb.CacheStatusReply_CacheResourceStatus) error

	ShowCacheStatus()

	CheckCache(dep string) (int, string, bool)

	CheckMissingDependency(dependencies []string) bool

	GetParentDependencyDeleteStatus(dependencies []string) bool

	UpdateLeafRefDependency(leafRefDep string)

	GetCurrentConfig() []byte

	SetCurrentConfig(b []byte)

	LocatePathInCache(pel []*gnmi.PathElem) (interface{}, bool, error)
}
*/

// Cache contains the
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
	log                   logging.Logger
}

type ResourceData struct {
	Config      *netwdevpb.CacheUpdateRequest
	CacheStatus netwdevpb.CacheStatusReply_CacheResourceStatus // Status of the resource
}

func NewCache(log logging.Logger) *Cache {
	//onChangeDeletes := make([]string, 0)
	//onChangeUpdates := make([]string, 0)
	//OnChangeDeviations := make(map[string]*Deviation)
	var x1 interface{}
	empty, err := json.Marshal(x1)
	if err != nil {
		return nil
	}
	return &Cache{
		//NewK8sOperatorUpdates: new(bool),
		//NewOnChangeUpdates:    new(bool),
		//OnChangeReApplyCache:  new(bool),
		Data:               make(map[int]map[string]*ResourceData),
		Levels:             make([]int, 0),
		OnChangeDeletes:    make([]string, 0),
		OnChangeUpdates:    make([]string, 0),
		OnChangeDeviations: make(map[string]*Deviation),

		CurrentConfig: empty,
		log:           log,
	}
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

func (c *Cache) SetNewK8sOperatorUpdates(b bool) {
	c.NewK8sOperatorUpdates = b
}

func (c *Cache) GetNewK8sOperatorUpdates() bool {
	return c.NewK8sOperatorUpdates
}

func (c *Cache) SetNewOnChangeUpdates(b bool) {
	c.NewOnChangeUpdates = b
}

func (c *Cache) GetNewOnChangeUpdates() bool {
	return c.NewOnChangeUpdates
}

func (c *Cache) GetOnChangeDeviations() map[string]*Deviation {
	return c.OnChangeDeviations
}

func (c *Cache) SetOnChangeReApplyCache(b bool) {
	c.OnChangeReApplyCache = b
}

func (c *Cache) GetOnChangeReApplyCache() bool {
	return c.OnChangeReApplyCache
}

func (c *Cache) GetOnChangeDeletes() []string {
	return c.OnChangeDeletes
}

func (c *Cache) GetOnChangeUpdates() []string {
	return c.OnChangeUpdates
}

func (c *Cache) SetOnChangeCacheUpdates(autoPilot bool, onChangeDeviations map[string]*Deviation) {
	c.Lock()

	for xpath, deviation := range onChangeDeviations {
		// update the OnChangeDeviations in the cache
		(c.OnChangeDeviations)[xpath] = deviation
		switch deviation.DeviationAction {
		case netwdevpb.Deviation_DeviationActionReApplyCache:
			c.OnChangeReApplyCache = true
		case netwdevpb.Deviation_DeviationActionDelete:
			c.NewOnChangeUpdates = true
			c.OnChangeDeletes = append(c.OnChangeDeletes, xpath)
		case netwdevpb.Deviation_DeviationActionIgnore:
			// do nothing
		case netwdevpb.Deviation_DeviationActionDeleteIgnoredByParent:
			// do nothing
		case netwdevpb.Deviation_DeviationActionIgnoreException:
			// do nothing
		default:
		}
	}
	c.Unlock()
}

// SetStatus sets the status of the data
// l = level, o = object
func (c *Cache) SetStatus(l int, o string, s netwdevpb.CacheStatusReply_CacheResourceStatus) error {
	c.Mutex.Lock()

	c.log.Debug("New Status", "Level", l, "Object", o, "CacheStatus", s)
	if d, ok := c.Data[l][o]; ok {
		d.CacheStatus = s
	}

	c.Mutex.Unlock()
	return nil
}

func (c *Cache) GetParentDependencyDeleteStatus(dependencies []string) bool {
	c.log.Debug("Check Parent dependency", "status", dependencies)
	for _, l := range c.Levels {
		for _, data := range c.Data[l] {
			for i, ip := range data.Config.IndividualActionPath {
				for _, dep := range dependencies {
					if dep == ip {
						return data.Config.IndividualActionPathSuccess[i]
					}
				}
			}
		}
	}
	c.log.Debug("getParentDependencyDeleteSuccess: We come here since the parent got already deleted")
	return true
}

// CheckMissingDependency validates the dependencies of the path
func (c *Cache) CheckMissingDependency(dependencies []string) bool {
	c.log.Debug("Check missing dependencies", "depdendencies", dependencies)
	for _, l := range c.Levels {
		for o, data := range c.Data[l] {
			if len(dependencies) == 0 {
				// no depedency for this object
				return false
			}
			for _, dep := range dependencies {
				c.log.Debug("Dependency", "Object", o, "Dependency", dep)
				for _, dp := range data.Config.IndividualActionPath {
					c.log.Debug("object", "Object", o, "ObjDependency", dep, "Global Dependency", dp)
					if strings.Contains(dp, dep) {
						// we can check the status since we processed the data before
						if data.CacheStatus != netwdevpb.CacheStatusReply_CacheResourceStatusDependencyMissing {
							// no depedency for this object
							return false
						}
					}
				}
			}
		}
	}
	// a dependency is missing for this object
	return true
}

func (c *Cache) ShowCacheStatus() {
	for _, l := range c.Levels {
		for o, data := range c.Data[l] {
			c.log.Debug("cache data", "Object", o, "Level", l, "Action", data.Config.Action, "CacheStatus", data.CacheStatus, "Dependencies", data.Config.Dependencies, "LeafRefDependencies", data.Config.LeafRefDependencies, "IndividualPath", data.Config.IndividualActionPath)
		}
	}
}

// CheckCache checks if an object exists in the cache
func (c *Cache) CheckCache(dep string) (int, string, bool) {
	for _, l := range c.Levels {
		for o, data := range c.Data[l] {
			for _, dp := range data.Config.IndividualActionPath {
				if dp == dep {
					return l, o, true
				}
			}
		}
	}
	return 0, "", false
}

// UpdateLeafRefDependency validates and updates the leafref dependency status
func (c *Cache) UpdateLeafRefDependency(leafRefDep string) {
	for _, l := range c.Levels {
		for o, data := range c.Data[l] {
			for _, dp := range data.Config.IndividualActionPath {
				if dp == leafRefDep {
					c.SetStatus(l, o, netwdevpb.CacheStatusReply_CacheResourceStatusLeafRefDependency)
				}
			}
		}
	}
}

func (c *Cache) GetCurrentConfig() []byte {
	return c.CurrentConfig
}

func (c *Cache) SetCurrentConfig(b []byte) {
	c.CurrentConfig = b
}

func (c *Cache) LocatePathInCache(pel []*gnmi.PathElem) (interface{}, bool, error) {
	var x1 interface{}
	err := json.Unmarshal(c.GetCurrentConfig(), &x1)
	if err != nil {
		return nil, false, err
	}
	_, x1, f := findPathInTree(x1, pel, 0)
	// TODO validate data
	return x1, f, nil
}
