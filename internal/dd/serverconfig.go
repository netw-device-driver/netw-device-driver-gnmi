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
	"fmt"
	"strings"
	"sync"

	jsonpatch "github.com/evanphx/json-patch"
	"github.com/karimra/gnmic/utils"
	config "github.com/netw-device-driver/ndd-grpc/config/configpb"
	nddv1 "github.com/netw-device-driver/ndd-runtime/apis/common/v1"
	"github.com/netw-device-driver/ndd-runtime/pkg/gext"
	"github.com/netw-device-driver/ndd-runtime/pkg/logging"
	"github.com/netw-device-driver/netw-device-driver-gnmi/internal/devices"
	"github.com/openconfig/gnmi/cache"
	"github.com/openconfig/gnmi/ctree"
	"github.com/openconfig/gnmi/match"
	"github.com/openconfig/gnmi/path"
	"github.com/openconfig/gnmi/proto/gnmi"
	"github.com/openconfig/gnmi/subscribe"
	"github.com/pkg/errors"
	ynddparser "github.com/yndd/ndd-yang/pkg/parser"
)

const (
	unmanagedResource = "Unmanaged resource"
)

// CacheOption can be used to manipulate Options.
type CacheOption func(*Cache)

// WithCacheLogger specifies how the cache should log messages.
func WithCacheLogger(log logging.Logger) CacheOption {
	return func(o *Cache) {
		o.log = log
	}
}

// WithParser specifies how the Reconciler should log messages.
func WithParser(log logging.Logger) CacheOption {
	return func(o *Cache) {
		o.parser = ynddparser.NewParser(ynddparser.WithLogger(log))
	}
}

func WithTarget(target string) CacheOption {
	return func(o *Cache) {
		o.target = target
	}
}

/*
func WithK8sClient(c client.Client) CacheOption {
	return func(o *Cache) {
		o.client = c
	}
}
*/

/*
func WithGNMICache(c *cache.Cache) CacheOption {
	return func(o *Cache) {
		o.c = c
	}
}
*/

type Cache struct {
	config.UnimplementedConfigurationServer

	Mutex              sync.RWMutex
	target             string
	ready              bool
	Device             devices.Device // handles all gnmi interaction based on the specific deviceexcept the capabilities
	NewProviderUpdates bool
	Levels             []int
	Data               map[int]map[string]*config.Status // old solution
	Data2              map[int]map[string]*resourceData  // new data cache using gnmi
	RegisteredDevices  map[nddv1.DeviceType]*nddv1.Register
	CurrentConfig      interface{}
	CurrentPath        *config.Path
	log                logging.Logger
	parser             *ynddparser.Parser
	//record             event.Recorder
	//client             client.Client
	m   *match.Match
	c   *cache.Cache
	ctx context.Context
}

func NewCache(target string, opts ...CacheOption) *Cache {
	var x1 interface{}
	empty, err := json.Marshal(x1)
	if err != nil {
		// TODO should this be better error handled
		return nil
	}

	c := &Cache{
		ready:             false,
		Data:              make(map[int]map[string]*config.Status),
		Data2:             make(map[int]map[string]*resourceData),
		RegisteredDevices: make(map[nddv1.DeviceType]*nddv1.Register),
		CurrentConfig:     empty,
		m:                 match.New(),
		c:                 cache.New(nil),
		ctx:               context.Background(),
	}
	for _, opt := range opts {
		opt(c)
	}

	if !c.c.HasTarget(target) {
		c.c.Add(target)
		c.log.Debug("NewCache target added to the local cache", "Target", target)
	}

	return c
}

func (c *Cache) UpdateResourceInGNMICache(resourceName string) {
	prefix, err := utils.CreatePrefix("", c.target)
	if err != nil {
		c.log.Debug("GNMI CreatePrefix Error", "Resource", resourceName, "Error", err)
	}
	c.c.GnmiUpdate(&gnmi.Notification{
		Prefix: prefix,
		Update: []*gnmi.Update{
			{
				Path: &gnmi.Path{
					Elem: []*gnmi.PathElem{
						{Name: "provider-resource-update"},
					},
				},
				Val: &gnmi.TypedValue{Value: &gnmi.TypedValue_StringVal{StringVal: resourceName}},
			},
		},
	})
	if err != nil {
		c.log.Debug("GNMI Update Error", "Resource", resourceName, "Error", err)
	}
	c.log.Debug("GNMI Update Successfull", "Resource", resourceName)

}

func (c *Cache) UpdateGNMICache(n *ctree.Leaf) {
	c.log.Debug("UpdateGNMICache", "Notification", n)
	switch v := n.Value().(type) {
	case *gnmi.Notification:
		c.log.Debug("UpdateGNMICache", "Path", path.ToStrings(v.Prefix, true), "Notification", v)
		subscribe.UpdateNotification(c.m, n, v, path.ToStrings(v.Prefix, true))
	default:
		c.log.Debug("unexpected update type", "type", v)
	}
}

// GetResourceName returns the matched resource from the path, by checking the best match from the cache
func (c *Cache) GetResourceName(ctx context.Context, req *config.ResourceRequest) (*config.ResourceReply, error) {
	// if the cache is not ready we return an empty response
	if !c.GetReady() {
		return &config.ResourceReply{}, nil
	}

	c.log.Debug("Config GetResourceName...", "Path", req.GetPath())

	// provide a string from the gnmi Path
	reqPath := *c.parser.ConfigGnmiPathToXPath(req.GetPath(), true)
	// initialize the variables which are used to keep track of the matched strings
	matchedResourceName := ""
	matchedResourcePath := ""
	// loop over the cache
	for _, resourceData := range c.Data {
		for resourceName, resourceStatus := range resourceData {
			resourcePath := *c.parser.ConfigGnmiPathToXPath(resourceStatus.Path, true)
			// check if the string is contained in the path
			if strings.Contains(reqPath, resourcePath) {
				// if there is a better match we use the better match
				if len(resourcePath) > len(matchedResourcePath) {
					matchedResourcePath = resourcePath
					matchedResourceName = resourceName
				}
			}
		}
	}
	c.log.Debug("Config GetResourceName...", "ResourceName", matchedResourceName)
	return &config.ResourceReply{
		Name: matchedResourceName,
	}, nil
}

// GetConfig returns the current config from the cache
func (c *Cache) GetConfig(ctx context.Context, req *config.ConfigRequest) (*config.ConfigReply, error) {
	// if the cache is not ready we return an empty response
	if !c.GetReady() {
		return &config.ConfigReply{}, nil
	}

	c.log.Debug("Config GetConfig...")

	d, err := json.Marshal(c.CurrentConfig)
	if err != nil {
		return nil, errors.Wrap(err, "Marshal failed")
	}

	return &config.ConfigReply{
		Data: d,
	}, nil
}

// Create creates a managed resource from the provider
func (c *Cache) Create(ctx context.Context, req *config.Request) (*config.Reply, error) {
	c.log.Debug("Config Create...")
	// if the cache is not ready we return an empty response
	if !c.GetReady() {
		return &config.Reply{}, nil
	}

	c.Lock()
	if _, ok := c.Data[int(req.GetLevel())]; !ok {
		c.Data[int(req.GetLevel())] = make(map[string]*config.Status)
	}
	c.Data[int(req.GetLevel())][req.GetName()] = &config.Status{
		Name:      req.GetName(),
		Level:     req.GetLevel(),
		Data:      req.GetData(),
		Path:      req.GetPath(),
		Update:    req.GetUpdate(),
		Status:    config.Status_CreatePending,
		Deviation: make([]*config.Deviation, 0),
	}
	c.SetNewProviderUpdates(true)
	c.Unlock()

	c.ShowStatus2()

	c.log.Debug("Config Create reply...")
	return &config.Reply{}, nil
}

func (c *Cache) Get(ctx context.Context, req *config.ResourceKey) (*config.Status, error) {
	c.log.Debug("Config Get/Observe...", "level", req.GetLevel(), "Name", req.GetName())

	// if the cache is not ready we return an empty response
	if !c.GetReady() {
		return &config.Status{
			Name:   "",
			Path:   req.Path,
			Data:   nil,
			Status: config.Status_None,
			Exists: false,
		}, nil
	}
	// Get data from currentconfig in cache

	in, tc := c.GetObjectValueInConfig(c.CurrentConfig, req.Path)
	if !tc.Found {
		c.log.Debug("Config Get Data not found", "Trace Context", tc)
		if _, ok := c.Data[int(req.GetLevel())]; ok {
			if x, ok := c.Data[int(req.GetLevel())][req.GetName()]; ok {
				// this is a managed resource and there is no data
				// this is a bit strange if we come here
				return &config.Status{
					Name:  x.Name,
					Level: x.Level,
					Path:  x.Path,
					//Data:      nil,
					Status:    x.Status,
					Deviation: x.Deviation,
					Exists:    true,
				}, nil
			} else {
				// there is no managed resource info and there is no data as an unmanaged resource
				return &config.Status{
					Name: "",
					Path: req.Path,
					//Data:   nil,
					Status: config.Status_None,
					Exists: false,
				}, nil
			}
		} else {
			// there is no managed resource info and there is no data as an unmanaged resource
			return &config.Status{
				Name: "",
				Path: req.Path,
				//Data:   nil,
				Status: config.Status_None,
				Exists: false,
			}, nil
		}
	} else {
		// data exists
		// copy the data to a new struct
		x1, err := c.parser.DeepCopy(in)
		if err != nil {
			c.log.Debug("Config Get/Observe cannot copy data", "error", err)
			return &config.Status{
				Exists: true,
			}, err
		}
		c.log.Debug("Config Get/Observe reply from cache", "data", x1)

		if _, ok := c.Data[int(req.GetLevel())]; ok {
			if x, ok := c.Data[int(req.GetLevel())][req.GetName()]; ok {
				// this is a managed resource and there is data, clean it before
				// sending the response to the provider:
				// Remove the elements of a hierarchical resource
				// find all subPaths that have a hierarchical dependency on the main resource
				d, err := c.ParseReturnObserveData(x, x1)
				if err != nil {
					return &config.Status{
						Name:      x.Name,
						Level:     x.Level,
						Path:      x.Path,
						Data:      x.Data,
						Status:    x.Status,
						Deviation: x.Deviation,
						Exists:    true,
					}, err
				}
				// return data
				return &config.Status{
					Name:      x.Name,
					Level:     x.Level,
					Path:      x.Path,
					Data:      d,
					Status:    x.Status,
					Deviation: x.Deviation,
					Exists:    true,
				}, nil
			} else {
				// there is no managed resource info and there is data as an unmanaged resource
				d, err := json.Marshal(x1)
				if err != nil {
					// error marshaling the data, return info we have with error
					return &config.Status{
						Name:   "",
						Path:   req.Path,
						Data:   nil,
						Status: config.Status_None,
						Exists: false,
					}, err
				}
				return &config.Status{
					Name:   "",
					Path:   req.Path,
					Data:   d,
					Status: config.Status_None,
					Exists: false,
				}, nil
			}
		} else {
			// there is no managed resource info and there is data as an unmanaged resource
			d, err := json.Marshal(x1)
			if err != nil {
				// error marshaling the data, return info we have with error
				return &config.Status{
					Name:   "",
					Path:   req.Path,
					Data:   nil,
					Status: config.Status_None,
					Exists: false,
				}, err
			}
			return &config.Status{
				Name:   "",
				Path:   req.Path,
				Data:   d,
				Status: config.Status_None,
				Exists: false,
			}, nil
		}
	}
}

func (c *Cache) Update(ctx context.Context, req *config.Notification) (*config.Reply, error) {
	c.log.Debug("Config Update...", "level", req.GetLevel(), "Name", req.GetName())

	// if the cache is not ready we return an empty response
	if !c.GetReady() {
		return &config.Reply{}, nil
	}

	c.ReconcileUpdate(ctx, req)

	c.log.Debug("Config Update reply...")
	return &config.Reply{}, nil
}

func (c *Cache) Delete(ctx context.Context, req *config.ResourceKey) (*config.Reply, error) {
	c.log.Debug("Config Delete...", "level", req.GetLevel(), "Name", req.GetName())

	// if the cache is not ready we return an empty response
	if !c.GetReady() {
		return &config.Reply{}, nil
	}
	if _, ok := c.Data[int(req.GetLevel())]; ok {
		if _, ok := c.Data[int(req.GetLevel())][req.GetName()]; ok {
			c.Data[int(req.GetLevel())][req.GetName()].Status = config.Status_DeletePending
			c.Lock()
			c.SetNewProviderUpdates(true)
			c.Unlock()
			c.log.Debug("Config Delete reply, resource found...", "level", req.GetLevel(), "Name", req.GetName())
		} else {
			c.log.Debug("Config Delete reply resourcename not found", "level", req.GetLevel(), "Name", req.GetName())
		}
	} else {
		c.log.Debug("Config Delete reply level not found", "level", req.GetLevel(), "Name", req.GetName())
	}
	return &config.Reply{}, nil
}

func (c *Cache) GetReady() bool {
	return c.ready
}

func (c *Cache) Ready() {
	c.ready = true
}

func (c *Cache) NotReady() {
	c.ready = false
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

func (c *Cache) DeleteResource2(level int, name string) {
	delete(c.Data2[level], name)
}

func (c *Cache) SetStatus(level int, name string, s config.Status_ResourceStatus) {
	c.Data[level][name].Status = s
}

func (c *Cache) SetStatus2(level int, name string, s gext.ResourceStatus) {
	c.Data2[level][name].Status = s
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

func (c *Cache) ShowStatus2() {
	c.log.Debug("show cache status start")
	for l, d1 := range c.Data2 {
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

func (c *Cache) DeleteObjectInConfig(cfg interface{}, path *config.Path) (interface{}, *ynddparser.TraceCtxt) {
	tc := &ynddparser.TraceCtxt{
		Path:   path,
		Idx:    0,
		Msg:    make([]string, 0),
		Action: ynddparser.ConfigTreeActionDelete,
	}
	//c.log.Debug("DeletePathInConfig Before", "Config", cfg)
	cfg = c.parser.ParseTreeWithAction(cfg, tc, 0, 0)
	//fmt.Printf("DeletePathInConfig finished: %v, path: %v\n", tc, tc.Path)
	//c.log.Debug("DeletePathInConfig After", "Config", cfg)

	return cfg, tc
}

func (c *Cache) DeleteObjectGnmi(cfg interface{}, path *gnmi.Path) (interface{}, *ynddparser.TraceCtxtGnmi) {
	tc := &ynddparser.TraceCtxtGnmi{
		Path:   path,
		Idx:    0,
		Msg:    make([]string, 0),
		Action: ynddparser.ConfigTreeActionDelete,
	}
	//c.log.Debug("DeletePathInConfig Before", "Config", cfg)
	cfg = c.parser.ParseTreeWithActionGnmi(cfg, tc, 0, 0)
	//fmt.Printf("DeletePathInConfig finished: %v, path: %v\n", tc, tc.Path)
	//c.log.Debug("DeletePathInConfig After", "Config", cfg)

	return cfg, tc
}

func (c *Cache) GetObjectValueInConfig(cfg interface{}, path *config.Path) (interface{}, *ynddparser.TraceCtxt) {
	tc := &ynddparser.TraceCtxt{
		Path:   path,
		Idx:    0,
		Msg:    make([]string, 0),
		Action: ynddparser.ConfigTreeActionGet,
	}
	//fmt.Printf("FindValueInCurrentConfig: %v path %v\n", tc, tc.path)
	x := c.parser.ParseTreeWithAction(cfg, tc, 0, 0)
	//fmt.Printf("parseTreeWithAction finished: %v, path: %v\n", tc, tc.Path)
	return x, tc
	//return findObjectInTree(c.CurrentConfig, tc)
}

func (c *Cache) GetObjectValueGnmi(cfg interface{}, path *gnmi.Path) (interface{}, *ynddparser.TraceCtxtGnmi) {
	tc := &ynddparser.TraceCtxtGnmi{
		Path:   path,
		Idx:    0,
		Msg:    make([]string, 0),
		Action: ynddparser.ConfigTreeActionGet,
	}
	//fmt.Printf("FindValueInCurrentConfig: %v path %v\n", tc, tc.path)
	x := c.parser.ParseTreeWithActionGnmi(cfg, tc, 0, 0)
	//fmt.Printf("parseTreeWithAction finished: %v, path: %v\n", tc, tc.Path)
	return x, tc
	//return findObjectInTree(c.CurrentConfig, tc)
}

func (c *Cache) UpdateObjectInConfig(cfg interface{}, path *config.Path, value interface{}) (interface{}, *ynddparser.TraceCtxt) {
	tc := &ynddparser.TraceCtxt{
		Path:   path,
		Value:  value,
		Idx:    0,
		Msg:    make([]string, 0),
		Action: ynddparser.ConfigTreeActionUpdate,
	}
	//c.log.Debug("UpdateObjectInConfig Before", "Config", cfg)
	cfg = c.parser.ParseTreeWithAction(cfg, tc, 0, 0)
	//fmt.Printf("parseTreeWithAction finished: %v, path: %v\n", tc, tc.Path)
	//c.log.Debug("UpdateObjectInConfig After", "Config", cfg)
	return cfg, tc
}

func (c *Cache) UpdateObjectGnmi(cfg interface{}, path *gnmi.Path, value interface{}) (interface{}, *ynddparser.TraceCtxtGnmi) {
	tc := &ynddparser.TraceCtxtGnmi{
		Path:   path,
		Value:  value,
		Idx:    0,
		Msg:    make([]string, 0),
		Action: ynddparser.ConfigTreeActionUpdate,
	}
	//c.log.Debug("UpdateObjectInConfig Before", "Config", cfg)
	cfg = c.parser.ParseTreeWithActionGnmi(cfg, tc, 0, 0)
	//fmt.Printf("parseTreeWithAction finished: %v, path: %v\n", tc, tc.Path)
	//c.log.Debug("UpdateObjectInConfig After", "Config", cfg)
	return cfg, tc
}

func (c *Cache) CreateObjectInConfig(cfg interface{}, path *config.Path, value interface{}) (interface{}, *ynddparser.TraceCtxt) {
	tc := &ynddparser.TraceCtxt{
		Path:   path,
		Value:  value,
		Idx:    0,
		Msg:    make([]string, 0),
		Action: ynddparser.ConfigTreeActionCreate,
	}
	//c.log.Debug("CreateValueInConfig Before", "Config", cfg)
	cfg = c.parser.ParseTreeWithAction(cfg, tc, 0, 0)
	//fmt.Printf("parseTreeWithAction finished: %v, path: %v\n", tc, tc.Path)
	//c.log.Debug("CreateValueInConfig After", "Config", cfg)
	return cfg, tc
}

func (c *Cache) CreateObjectGnmi(cfg interface{}, path *gnmi.Path, value interface{}) (interface{}, *ynddparser.TraceCtxtGnmi) {
	tc := &ynddparser.TraceCtxtGnmi{
		Path:   path,
		Value:  value,
		Idx:    0,
		Msg:    make([]string, 0),
		Action: ynddparser.ConfigTreeActionCreate,
	}
	//c.log.Debug("CreateValueInConfig Before", "Config", cfg)
	cfg = c.parser.ParseTreeWithActionGnmi(cfg, tc, 0, 0)
	//fmt.Printf("parseTreeWithAction finished: %v, path: %v\n", tc, tc.Path)
	//c.log.Debug("CreateValueInConfig After", "Config", cfg)
	return cfg, tc
}

func (c *Cache) FindManagedResource(path string) (int, string) {
	matchedResourceName := unmanagedResource
	matchedResourcePath := ""
	matchedLevel := 0
	// loop over the cache
	for level, resourceData := range c.Data {
		for resourceName, resourceStatus := range resourceData {
			resourcePath := *c.parser.ConfigGnmiPathToXPath(resourceStatus.Path, true)
			// check if the string is contained in the path
			if strings.Contains(path, resourcePath) {
				// if there is a better match we use the better match
				if len(resourcePath) > len(matchedResourcePath) {
					matchedResourcePath = resourcePath
					matchedResourceName = resourceName
					matchedLevel = level
				}
			}
		}
	}
	return matchedLevel, matchedResourceName
}

func (c *Cache) FindManagedResourceGnmi(path string) (int, string) {
	matchedResourceName := unmanagedResource
	matchedResourcePath := ""
	matchedLevel := 0
	// loop over the cache
	for level, resources := range c.Data2 {
		for resourceName, resourceStatus := range resources {
			resourcePath := *c.parser.GnmiPathToXPath(resourceStatus.Path, true)
			// check if the string is contained in the path
			if strings.Contains(path, resourcePath) {
				// if there is a better match we use the better match
				if len(resourcePath) > len(matchedResourcePath) {
					matchedResourcePath = resourcePath
					matchedResourceName = resourceName
					matchedLevel = level
				}
			}
		}
	}
	return matchedLevel, matchedResourceName
}

func (c *Cache) AddDeviation(l int, r string, d *config.Deviation) error {
	if _, ok := c.Data[l]; !ok {
		return errors.New(fmt.Sprintf("level: %d does not exist in cache", l))
	}
	if _, ok := c.Data[l][r]; !ok {
		return errors.New(fmt.Sprintf("resourcename: %s does not exist in cache", r))
	}
	c.Data[l][r].Deviation = append(c.Data[l][r].Deviation, d)
	return nil
}

func (c *Cache) DeleteDeviation(l int, r string, d *config.Deviation) error {
	if _, ok := c.Data[l]; !ok {
		return errors.New(fmt.Sprintf("level: %d does not exist in cache", l))
	}
	if _, ok := c.Data[l][r]; !ok {
		return errors.New(fmt.Sprintf("resourcename: %s does not exist in cache", r))
	}
	// TODO, not sure we need this
	return nil
}

func (c *Cache) GetDeviations(l int, r string) ([]*config.Deviation, error) {
	if _, ok := c.Data[l]; !ok {
		return nil, errors.New(fmt.Sprintf("level: %d does not exist in cache", l))
	}
	if _, ok := c.Data[l][r]; !ok {
		return nil, errors.New(fmt.Sprintf("resourcename: %s does not exist in cache", r))
	}
	return c.Data[l][r].Deviation, nil
}

// ParseReturnObserveData cleans the data which is returned, by removing the hierarchical elements
// from the data
func (c *Cache) ParseReturnObserveData(x *config.Status, x1 interface{}) ([]byte, error) {
	// Remove the elements of a hierarchical resource
	// find all subPaths that have a hierarchical dependency on the main resource
	xPath := *c.parser.ConfigGnmiPathToXPath(x.Path, true)
	subPaths := make([]string, 0)
	// loop over the cache to check if a hierarchical resource exists
	for _, resourceData := range c.Data {
		for _, resourceStatus := range resourceData {
			resourcePath := *c.parser.ConfigGnmiPathToXPath(resourceStatus.Path, true)
			c.log.Debug("ParseReturnObserveData", "ResourcePath", xPath, "Other ResourcePath", resourcePath)
			if strings.Contains(resourcePath, xPath) {
				if len(resourcePath) > len(xPath) {
					subPaths = append(subPaths, resourcePath)
				}
			}
		}
	}
	pathElems := make([]string, 0)
	for _, subPath := range subPaths {
		// trim xpath prefix
		p := strings.TrimPrefix(subPath, xPath+"/")
		// split in "/"
		split := strings.Split(p, "/")
		// split in "[" to remove the key
		s := strings.Split(split[0], "[")
		pathElem := s[0]
		f := false
		for _, e := range pathElems {
			if e == pathElem {
				f = true
			}
		}
		if !f {
			pathElems = append(pathElems, pathElem)
		}
	}
	c.log.Debug("Config Get/Observe hierarchical elements", "pathElements", pathElems)
	for _, pathElem := range pathElems {
		delete(x1.(map[string]interface{}), pathElem)
	}
	// Add the last element of the path back to the data
	// the data we return from findObjectInTree returns the data w/o the interface element e.g. interface[name=system0]
	x2 := make(map[string]interface{})
	x2[x.Path.GetElem()[len(x.Path.GetElem())-1].GetName()] = x1
	c.log.Debug("Config Get/Observe reply final", "ResourceName", x.Name, "Status", x.Status, "data", x2)
	d, err := json.Marshal(x2)
	if err != nil {
		return nil, err
	}
	return d, nil
}

// ParseReturnObserveData cleans the data which is returned, by removing the hierarchical elements
// from the data
func (c *Cache) ParseReturnObserveDataGnmi(x *resourceData, x1 interface{}) ([]byte, error) {
	// Remove the elements of a hierarchical resource
	// find all subPaths that have a hierarchical dependency on the main resource
	xPath := *c.parser.GnmiPathToXPath(x.GetPath(), true)
	subPaths := make([]string, 0)
	// loop over the cache to check if a hierarchical resource exists
	for _, resources := range c.Data2 {
		for _, resData := range resources {
			resourcePath := *c.parser.GnmiPathToXPath(resData.GetPath(), true)
			c.log.Debug("ParseReturnObserveData", "ResourcePath", xPath, "Other ResourcePath", resourcePath)
			if strings.Contains(resourcePath, xPath) {
				if len(resourcePath) > len(xPath) {
					subPaths = append(subPaths, resourcePath)
				}
			}
		}
	}
	pathElems := make([]string, 0)
	for _, subPath := range subPaths {
		// trim xpath prefix
		p := strings.TrimPrefix(subPath, xPath+"/")
		// split in "/"
		split := strings.Split(p, "/")
		// split in "[" to remove the key
		s := strings.Split(split[0], "[")
		pathElem := s[0]
		f := false
		for _, e := range pathElems {
			if e == pathElem {
				f = true
			}
		}
		if !f {
			pathElems = append(pathElems, pathElem)
		}
	}
	c.log.Debug("Config Get/Observe hierarchical elements", "pathElements", pathElems)
	for _, pathElem := range pathElems {
		delete(x1.(map[string]interface{}), pathElem)
	}
	// Add the last element of the path back to the data
	// the data we return from findObjectInTree returns the data w/o the interface element e.g. interface[name=system0]
	x2 := make(map[string]interface{})
	x2[x.Path.GetElem()[len(x.Path.GetElem())-1].GetName()] = x1
	c.log.Debug("Last Element", "Element name", x.Path.GetElem()[len(x.Path.GetElem())-1].GetName())
	c.log.Debug("Config Get/Observe reply final", "ResourceName", x.Name, "Status", x.Status, "Path", c.parser.GnmiPathToXPath(x.Path, true), "data", x2)
	d, err := json.Marshal(x2)
	if err != nil {
		return nil, err
	}
	return d, nil
}

// GetDeviceTypes returns all devicetypes that are registered
func (c *Cache) GetDeviceTypes() []nddv1.DeviceType {
	l := make([]nddv1.DeviceType, 0)
	for d := range c.RegisteredDevices {
		l = append(l, d)
	}
	return l
}

// GetDeviceMatches returns a map indexed by matchstring with element devicetype
func (c *Cache) GetDeviceMatches() map[string]nddv1.DeviceType {
	l := make(map[string]nddv1.DeviceType)
	for d, o := range c.RegisteredDevices {
		l[o.MatchString] = d
	}
	return l
}

func (c *Cache) GetSubscriptions(d nddv1.DeviceType) []string {
	if _, ok := c.RegisteredDevices[d]; !ok {
		return nil
	}
	return c.RegisteredDevices[d].Subscriptions
}
