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
	"sync"

	jsonpatch "github.com/evanphx/json-patch"
	config "github.com/netw-device-driver/ndd-grpc/config/configpb"
	"github.com/netw-device-driver/ndd-runtime/pkg/logging"
	"github.com/pkg/errors"
)

// CacheOption can be used to manipulate Options.
type CacheOption func(*Cache)

// WithCacheLogger specifies how the Reconciler should log messages.
func WithCacheLogger(log logging.Logger) CacheOption {
	return func(o *Cache) {
		o.log = log
	}
}

type Cache struct {
	config.UnimplementedConfigurationServer

	Mutex              sync.RWMutex
	NewProviderUpdates bool
	Levels             []int
	Data               map[int]map[string]*config.Status
	CurrentConfig      interface{}
	CurrentPath        *config.Path
	log                logging.Logger
}

func NewCache(opts ...CacheOption) *Cache {
	var x1 interface{}
	empty, err := json.Marshal(x1)
	if err != nil {
		// TODO should this be better error handled
		return nil
	}

	c := &Cache{
		Data:          make(map[int]map[string]*config.Status),
		CurrentConfig: empty,
	}
	for _, opt := range opts {
		opt(c)
	}

	return c
}

func (c *Cache) GetConfig(ctx context.Context, req *config.ConfigRequest) (*config.ConfigReply, error) {
	c.log.Debug("Config GetConfig...")

	d, err := json.Marshal(c.CurrentConfig)
	if err != nil {
		return nil, errors.Wrap(err, "Marshal failed")
	}

	return &config.ConfigReply{
		Data: d,
	}, nil

}

func (c *Cache) Create(ctx context.Context, req *config.Request) (*config.Reply, error) {
	c.log.Debug("Config Create...")

	c.Lock()
	if _, ok := c.Data[int(req.GetLevel())]; !ok {
		c.Data[int(req.GetLevel())] = make(map[string]*config.Status)
	}
	c.Data[int(req.GetLevel())][req.GetName()] = &config.Status{
		Name:      req.GetName(),
		Level:     req.GetLevel(),
		Data:      req.GetData(),
		Path:      req.GetPath(),
		Status:    config.Status_CreatePending,
		Deviation: make([]*config.Deviation, 0),
	}
	c.SetNewProviderUpdates(true)
	c.Unlock()

	c.ShowStatus()

	c.log.Debug("Config Create reply...")
	return &config.Reply{}, nil
}

func (c *Cache) Get(ctx context.Context, req *config.ResourceKey) (*config.Status, error) {
	c.log.Debug("Config Get...")

	if x, ok := c.Data[int(req.GetLevel())][req.GetName()]; ok {
		// resource exists
		c.log.Debug("Config Get reply, resource exists", "Level", req.GetLevel(), "Name", req.GetName())
		x.Exists = true
		return x, nil
	}
	// resource does not exist
	c.log.Debug("Config Get reply, resource does not exist", "Level", req.GetLevel(), "Name", req.GetName())
	return &config.Status{
		Exists: false,
	}, nil
}

func (c *Cache) Update(ctx context.Context, req *config.Request) (*config.Reply, error) {
	c.log.Debug("Config Update...")

	c.Lock()
	if _, ok := c.Data[int(req.GetLevel())]; !ok {
		c.Data[int(req.GetLevel())] = make(map[string]*config.Status)
	}
	c.Data[int(req.GetLevel())][req.GetName()] = &config.Status{
		Name:      req.GetName(),
		Level:     req.GetLevel(),
		Data:      req.GetData(),
		Path:      req.GetPath(),
		Status:    config.Status_UpdatePending,
		Deviation: make([]*config.Deviation, 0),
	}

	c.SetNewProviderUpdates(true)
	c.Unlock()

	c.log.Debug("Config Update reply...")
	return &config.Reply{}, nil
}

func (c *Cache) Delete(ctx context.Context, req *config.ResourceKey) (*config.Reply, error) {
	c.log.Debug("Config Delete...")

	c.Data[int(req.GetLevel())][req.GetName()].Status = config.Status_DeletePending
	c.Lock()
	c.SetNewProviderUpdates(true)
	c.Unlock()

	c.log.Debug("Config Delete reply...")
	return &config.Reply{}, nil
}

func (c *Cache) Lock() {
	c.Mutex.RLock()
}

func (c *Cache) Unlock() {
	c.Mutex.RUnlock()
}

func (c *Cache) SetNewProviderUpdates(b bool) {
	c.NewProviderUpdates = b
}

func (c *Cache) GetNewProviderUpdates() bool {
	return c.NewProviderUpdates
}

func (c *Cache) DeleteResource(level int, name string) {
	delete(c.Data[level], name)
}

func (c *Cache) SetStatus(level int, name string, s config.Status_ResourceStatus) {
	c.Data[level][name].Status = s
}

func (c *Cache) ShowStatus() {
	c.log.Debug("show cache status start")
	for l, d1 := range c.Data {
		for n, d2 := range d1 {
			c.log.Debug("cache resource", "level", l, "resourceName", n, "path", d2.Path, "status", d2.GetStatus())
		}
	}
	c.log.Debug("show cache status end")
}

func (c *Cache) IsEqual(mergedData interface{}) bool {
	j1, err := json.Marshal(c.CurrentConfig)
	if err != nil {
		return false
	}

	j2, err := json.Marshal(mergedData)
	if err != nil {
		return false
	}

	if jsonpatch.Equal(j1, j2) {
		return true
	}
	return false
}

func (c *Cache) SetCurrentConfig(d interface{}) error {
	c.CurrentConfig = d
	return nil
}

func (c *Cache) GetCurrentConfig() interface{} {
	return c.CurrentConfig
}
