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
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	jsonpatch "github.com/evanphx/json-patch"
	log "github.com/sirupsen/logrus"

	"github.com/netw-device-driver/netwdevpb"
)

// Cache contains the
type Cache struct {
	Mutex         sync.RWMutex
	Data          map[int]map[string]*Data // int represnts the level, string represents the path
	Levels        []int
	CurrentConfig []byte
}

// Data contains the driver data information
type Data struct {
	AggregateActionPath  string
	IndividualActionPath []string
	Action               netwdevpb.ConfigMessage_ActionType
	Data                 [][]byte // content of the data
	Dependencies         []string // dependencies the object has on other objects
	LastUpdated          time.Time
	Status               *DataStatus // Status of the data object
}

// DeletePath struct
type DeletePath struct {
	Object string
	Path   string
	Level  int
}

// DataStatus is an enum
type DataStatus string

const (
	// ToBeProcessed -> new data, not processed
	ToBeProcessed DataStatus = "ToBeProcessed"
	// Processed -> data, processed
	Processed DataStatus = "Processed"
	// DependencyMissing -> data with missing dependency
	DependencyMissing DataStatus = "DependencyMissing"
	// DeletePending -> data with pending delete
	DeletePending DataStatus = "DeletePending"
	// UpdatePending -> data with pending update
	UpdatePending DataStatus = "UpdatePending"
)

// UpdateCacheEntry updates the driver cache
func (c *Cache) UpdateCacheEntry(o string, netwCfgMsg *netwdevpb.ConfigMessage) error {
	c.Mutex.Lock()

	level := int(netwCfgMsg.Level)
	if !contains(c.Levels, level) {
		c.Levels = append(c.Levels, level)
		c.Data[level] = make(map[string]*Data)
	}
	sort.Ints(c.Levels)

	switch netwCfgMsg.Action {
	case netwdevpb.ConfigMessage_Update:
		c.Data[level][o] = &Data{
			Action:               netwCfgMsg.Action,
			AggregateActionPath:  netwCfgMsg.AggregateActionPath,
			IndividualActionPath: netwCfgMsg.IndividualActionPath,
			Data:                 netwCfgMsg.Data,
			Dependencies:         netwCfgMsg.Dependencies,
			LastUpdated:          time.Now(),
			Status:               new(DataStatus),
		}
	case netwdevpb.ConfigMessage_Replace:
		c.Data[level][o] = &Data{
			Action:               netwCfgMsg.Action,
			AggregateActionPath:  netwCfgMsg.AggregateActionPath,
			IndividualActionPath: netwCfgMsg.IndividualActionPath,
			Data:                 netwCfgMsg.Data,
			Dependencies:         netwCfgMsg.Dependencies,
			LastUpdated:          time.Now(),
			Status:               new(DataStatus),
		}
	case netwdevpb.ConfigMessage_Delete:
		c.Data[level][o] = &Data{
			Action:               netwCfgMsg.Action,
			IndividualActionPath: netwCfgMsg.IndividualActionPath,
			Data:                 netwCfgMsg.Data,
			Dependencies:         netwCfgMsg.Dependencies,
			LastUpdated:          time.Now(),
			Status:               new(DataStatus),
		}
	default:
	}
	*c.Data[level][o].Status = ToBeProcessed

	c.Mutex.Unlock()
	return nil
}

// DeleteCacheEntry updates the driver cache
// do = deleteObject
func (c *Cache) DeleteCacheEntry(do string) error {
	c.Mutex.Lock()
	for _, l := range c.Levels {
		for o, data := range c.Data[l] {
			if strings.Contains(o, do) {
				if *data.Status == DeletePending {
					delete(c.Data[l], o)
				}
			}
		}
	}
	c.Mutex.Unlock()
	return nil
}

// DeleteCacheEntryFailed updates the driver cache
// do = deleteObject, l = level, o = object
func (c *Cache) DeleteCacheEntryFailed(do string) error {
	c.Mutex.Lock()
	for _, l := range c.Levels {
		for o, data := range c.Data[l] {
			if strings.Contains(o, do) {
				if *data.Status == DeletePending {
					*data.Status = ToBeProcessed
				}
			}
		}
	}
	c.Mutex.Unlock()
	return nil
}

// SetStatus sets the status of the data
// l = level, o = object
func (c *Cache) SetStatus(l int, o string, s DataStatus) error {
	c.Mutex.Lock()

	log.Infof("New Status for Level: %d, Object: %s, Status: %s", l, o, s)
	if d, ok := c.Data[l][o]; ok {
		*d.Status = s
	}

	c.Mutex.Unlock()
	return nil
}

// CheckMissingDependency validates the dependencies of the path
func (c *Cache) CheckMissingDependency(dependencies []string) bool {
	log.Infof("Check missing dependencies: %s", dependencies)
	for _, l := range c.Levels {
		for o, data := range c.Data[l] {
			if len(dependencies) == 0 {
				// no depedency for this object
				return false
			}
			for _, dep := range dependencies {
				log.Infof("Dependency: %s To be checked withing Object: %s", o, dep)
				for _, dp := range data.IndividualActionPath {
					log.Infof("Object: %s, ObjDependency: %s Global Dependency: %s", o, dep, dp)
					if strings.Contains(dp, dep) {
						// we can check the status since we processed the data before
						if *data.Status != DependencyMissing {
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

func (c *Cache) showCacheStatus() {
	for _, l := range c.Levels {
		for o, data := range c.Data[l] {
			log.Infof("CACHE DATA: Object %s, Level: %d, Status %s, Dependencies: %v, DeletePath: %v", o, l, *data.Status, data.Dependencies, data.IndividualActionPath)
		}
	}
}

// CheckCache checks if an object exists in the cache
func (c *Cache) CheckCache(dep string) (int, string, bool) {
	for _, l := range c.Levels {
		for o, data := range c.Data[l] {
			for _, dp := range data.Dependencies {
				if dp == dep {
					return l, o, true
				}
			}
		}
	}
	return 0, "", false
}

// ReconcileCache reconciles the cache
func (d *DeviceDriver) ReconcileCache() error {
	// SHOW CACHE STATUS
	log.Info("Show STATUS before DELETE PROCESSING")
	d.Cache.showCacheStatus()

	// PROCESS DELETE ACTIONS
	deletePaths := make([]DeletePath, 0)
	// walk over the cache sorted with level and paths per level
	for _, l := range d.Cache.Levels {
		for o, data := range d.Cache.Data[l] {
			if len(data.Dependencies) == 0 {
				// no object dependencies
				if *data.Status == ToBeProcessed && data.Action == netwdevpb.ConfigMessage_Delete {
					d.Cache.SetStatus(l, o, DeletePending)
					for _, p := range data.IndividualActionPath {
						dp := DeletePath{
							Path:   p,
							Level:  l,
							Object: o,
						}
						deletePaths = append(deletePaths, dp)
					}

				}
			} else {
				// check object dependencies
				for _, dep := range data.Dependencies {
					// object's parent dependency gets deleted
					log.Infof("Delete dependencies: object: %s, dep: %s, ", o, dep)
					// dl = dependency level, do = dependency object
					dl, do, b := d.Cache.CheckCache(dep)
					if b {
						// parent dependency object is -> d.Cache.Data[l][dep]
						if *d.Cache.Data[dl][do].Status == ToBeProcessed && d.Cache.Data[dl][do].Action == netwdevpb.ConfigMessage_Delete {
							// the parent is to be deleted, so the child object will be deleted
							// -> we just need to update the cache status only
							d.Cache.SetStatus(l, o, DeletePending)
						} else {
							// the parent object exists, but will not be deleted so we need to delete the child if the Action is delete
							if *d.Cache.Data[l][o].Status == ToBeProcessed && d.Cache.Data[l][o].Action == netwdevpb.ConfigMessage_Delete {
								d.Cache.SetStatus(l, o, DeletePending)
								for _, p := range data.IndividualActionPath {
									dp := DeletePath{
										Path:   p,
										Level:  l,
										Object: o,
									}
									deletePaths = append(deletePaths, dp)
								}
							}
						}
					} else {
						// no parent object exists
						d.Cache.SetStatus(l, o, DependencyMissing)
					}
				}
			}
		}
	}

	log.Info("Show STATUS before DELETE GNMI")
	d.Cache.showCacheStatus()
	log.Infof("DeletePaths: %v", deletePaths)
	for _, dp := range deletePaths {
		if err := d.deleteDeviceData(&dp.Path); err != nil {
			log.WithError(err).Errorf("delete process failed, object: %s, path: %s", dp.Object, dp.Path)
			// When the delete action failed we update the status to ToBeProcessed
			d.Cache.DeleteCacheEntryFailed(dp.Object)
		} else {
			d.Cache.DeleteCacheEntry(dp.Object)
			log.Infof("delete processed, object: %s, path: %s", dp.Object, dp.Path)
		}
	}
	// PROCESS UPDATE ACTIONS

	log.Info("Show STATUS before UPDATE PROCESSING")
	d.Cache.showCacheStatus()

	var mergedData interface{}
	var mergedPath string
	var err error

	// walk over the cache sorted with level and objects per level
	for _, l := range d.Cache.Levels {
		for o, data := range d.Cache.Data[l] {
			if data.Action == netwdevpb.ConfigMessage_Update {
				if d.Cache.CheckMissingDependency(data.Dependencies) {
					// parent object dependencies are missing
					d.Cache.SetStatus(l, o, DependencyMissing)
				} else {
					// merge the data based on the UpdateActionPath
					log.Infof("New Update Path: aggregate path %s, individual path %s, current Update Path: %s", data.AggregateActionPath, data.IndividualActionPath, mergedPath)
					// merge or insert the data object per object rather than through a full list
					for i, d := range data.Data {
						log.Infof("Start MERGE; individual path: %s, aggregate path: %s, mergedPath: %s", data.IndividualActionPath[i], data.AggregateActionPath, mergedPath)
						var d1 interface{}
						json.Unmarshal(d, &d1)
						log.Infof("Start MERGE; new data: %v", d1)
						log.Infof("Start MERGE; current merged data: %v", mergedData)
						mergedPath, mergedData, err = startMerge(mergedPath, mergedData, data.IndividualActionPath[i], data.AggregateActionPath, d)
						if err != nil {
							log.WithError(err).Error("merge error")
						}
					}
				}
			}
		}
	}

	d.Cache.showCacheStatus()

	// SHOW MERGED CONFIG
	newMergedConfig, err := json.Marshal(mergedData)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("MergedUpdatePath: %s \n", mergedPath)
	fmt.Printf("MergedData: %s \n", newMergedConfig)

	if jsonpatch.Equal(d.Cache.CurrentConfig, newMergedConfig) {
		log.Info("The new merged data is EQUAL to the current config. DO NOTHING")
	} else {
		log.Info("The new merged data is DIFFERENT, apply to the device")
		d.Cache.CurrentConfig = newMergedConfig

		// TODO update status
		if len(mergedPath) > 0 {
			if err := d.updateDeviceData(&mergedPath, newMergedConfig); err != nil {
				// TODO check failure status
				log.WithError(err).Errorf("Merged update process FAILED, path: %s", mergedPath)
			}
			log.Infof("Merged update process SUCCEEDED, path: %s", mergedPath)
		}
	}

	return nil
}

func startMerge(ap1 string, j1 interface{}, ip2, ap2 string, data2 []byte) (string, interface{}, error) {
	log.Infof("Start Merge: Path1: %s, Path2: %s", ap1, ap2)
	var j2, x1, x2 interface{}
	var newAggrPath, newIndivPath string

	err := json.Unmarshal(data2, &j2)
	if err != nil {
		return "", nil, err
	}

	contains := false
	if len(ap1) == 0 {
		newAggrPath = ap2
		newIndivPath = ip2
		log.Infof("merged Data Zero: new aggregate pathPath: %s", newAggrPath)
		return newAggrPath, j2, nil
	}
	if len(ap2) >= len(ap1) {
		x1 = j1
		x2 = j2
		if strings.Contains(ap2, ap1) {
			contains = true
		}
		newAggrPath = ap1
		newIndivPath = ip2
		log.Infof("merged Data P1 >= P2: new aggregate pathPath: %s", newAggrPath)
	} else {
		log.Error("We should never come here since we order the data per level")
		x1 = j2
		x2 = j1
		if strings.Contains(ap1, ap2) {
			contains = true
		}
		newAggrPath = ap2
		newIndivPath = ip2
		log.Infof("merged Data P2 > P1: new aggregate pathPath: %s", newAggrPath)
	}
	if contains {
		var m interface{}

		log.Infof("mergePath: %s", newAggrPath)
		log.Infof("merge individual path: %s", newIndivPath)
		// NEW CODE
		ekvl := getHierarchicalElements(newIndivPath)
		m, err = addObjectToTheTree(x1, x2, ekvl, 0)

		return newAggrPath, m, err
	}
	log.Error("We should never come here, since dependencies were checked before")
	return ap1, j1, nil
}

// ElementKeyValue struct
type ElementKeyValue struct {
	Element  string
	KeyName  string
	KeyValue interface{}
}

func getHierarchicalElements(p string) (ekv []ElementKeyValue) {
	skipElement := false

	s1 := strings.Split(p, "/")
	log.Infof("Split: %v", s1)
	for i, v := range s1 {
		if i > 0 && !skipElement {
			log.Infof("Element: %s", v)
			if strings.Contains(s1[i], "[") {
				s2 := strings.Split(s1[i], "[")
				s3 := strings.Split(s2[1], "=")
				var v string
				if strings.Contains(s3[1], "]") {
					v = strings.Trim(s3[1], "]")
				} else {
					v = s3[1] + "/" + strings.Trim(s1[i+1], "]")
					skipElement = true
				}
				e := ElementKeyValue{
					Element:  s2[0],
					KeyName:  s3[0],
					KeyValue: v,
				}
				ekv = append(ekv, e)
			} else {
				e := ElementKeyValue{
					Element:  s1[i],
					KeyName:  "",
					KeyValue: "",
				}
				ekv = append(ekv, e)
			}
		} else {
			skipElement = false
		}
	}
	return ekv
}

func addObjectToTheTree(x1, x2 interface{}, ekvl []ElementKeyValue, i int) (interface{}, error) {
	log.Infof("START ADDING OBJECT TO THE TREE Index:%d, EKV: %v", i, ekvl)
	log.Infof("START ADDING OBJECT TO THE TREE X1: %v", x1)
	log.Infof("START ADDING OBJECT TO THE TREE X2: %v", x2)
	/*
		for _, ekv := range ekvl {
			x1 = addObject(x1, x2, ekv)
		}*/
	x1 = addObject(x1, x2, ekvl, 0)
	log.Infof("FINISHED ADDING OBJECT TO THE TREE X1: %v", x1)
	return x1, nil
}

func addObject(x1, x2 interface{}, ekv []ElementKeyValue, i int) interface{} {
	log.Infof("ADD1 OBJECT EKV: %v", ekv)
	log.Infof("ADD1 OBJECT EKV INDEX: %v", i)
	log.Infof("ADD1 OBJECT X1: %v", x1)
	log.Infof("ADD1 OBJECT X2: %v", x2)
	switch x1 := x1.(type) {
	case map[string]interface{}:
		x2, ok := x2.(map[string]interface{})
		if !ok {
			log.Info("NOK SOMETHING WENT WRONG map[string]interface{}")
			return x1
		}
		if _, ok := x1[ekv[i].Element]; ok {
			// object exists, so we need to continue -> this is typically for lists
			log.Infof("Check NEXT ELEMENT")
			if i == len(ekv)-1 {
				// last element of the list
				x1[ekv[i].Element] = addObject(x1[ekv[i].Element], x2[ekv[i].Element], ekv, i)
			} else {
				// not last element of the list e.g. we are at interface of  interface[name=ethernet-1/1]/subinterface[index=100]
				x1[ekv[i].Element] = addObject(x1[ekv[i].Element], x2, ekv, i)
			}
			// after list are processed return
			return x1
		}
		// it is a new element so we return. E.g. network-instance get added to / or interfaces gets added to network-instance
		log.Infof("Added NEW ELEMENT BEFORE X1: %v", x1)
		log.Infof("Added NEW ELEMENT BEFORE X2: %v", x2)
		log.Infof("Added NEW ELEMENT BEFORE EKV element: %v", ekv)
		if ekv[i].KeyName != "" {
			// list -> interfaces or network-instances
			x1[ekv[i].Element] = x2[ekv[i].Element]
		} else {
			// add e.g. system of (system, ntp)
			x1[ekv[i].Element] = nil
		}
		log.Infof("Added NEW ELEMENT BEFORE X1[]: %v", x1[ekv[i].Element])
		log.Infof("Added NEW ELEMENT BEFORE X2[]: %v", x2[ekv[i].Element])
		log.Infof("Added NEW ELEMENT to X1: %v", x1)
		if i == len(ekv)-1 {
			log.Infof("ADDING ELEMENTS FINISHED")
			return x1
		} else {
			log.Infof("CONTINUE ADDING ELEMENTS")
			log.Infof("CONTINUE ADDING ELEMENTS X1: %v", x1)
			log.Infof("CONTINUE ADDING ELEMENTS X2: %v", x2)
			log.Infof("CONTINUE ADDING ELEMENTS EKV element: %v", ekv)
			log.Infof("CONTINUE ADDING ELEMENTS X1[]: %v", x1[ekv[i].Element])
			log.Infof("CONTINUE ADDING ELEMENTS X2: %v", x2)

			x1[ekv[i].Element] = addObject(x1[ekv[i].Element], x2, ekv, i+1)
		}

	case []interface{}:
		for n, v1 := range x1 {
			switch x3 := v1.(type) {
			case map[string]interface{}:
				for k3, v3 := range x3 {
					if k3 == ekv[i].KeyName {
						switch v3.(type) {
						case string, uint32:
							if v3 == ekv[i].KeyValue {
								if i == len(ekv)-1 {
									// last element in the ekv list
									log.Info("OBJECT FOUND In LIST OVERWRITE WITH NEW OBJECT")
									x1[n] = x2
									return x1
								} else {
									// not last element in the ekv list
									log.Info("OBJECT FOUND IN LIST CONTINUE")
									log.Infof("OBJECT FOUND IN LIST CONTINUE: X1[n]: %v", x1[n])
									log.Infof("OBJECT FOUND IN LIST CONTINUE: X2: %v", x2)
									x1[n] = addObject(x1[n], x2, ekv, i+1)
								}
							}
						}
					}
				}
			}
		}
		if i == len(ekv)-1 {
			x2, ok := x2.([]interface{})
			if !ok {
				log.Info("NOK SOMETHING WENT WRONG []interface")
				return x1
			}
			log.Info("OBJECT NOT FOUND In LIST APPEND NEW OBJECT")
			log.Infof("APPEND BEFORE X1: %v", x1)
			log.Infof("APPEND BEFORE X2[0]: %v", x2[0])
			x1 = append(x1, x2[0])
			log.Infof("APPEND AFTER X1: %v", x1)
			return x1
		}
	case nil:
		log.Info("OBJECT DOES NOT EXIST CREATE")
		x1, ok := x2.(map[string]interface{})
		log.Infof("OBJECT DOES NOT EXIST CREATE X1: %v", x1)
		log.Infof("OBJECT DOES NOT EXIST CREATE X2: %v", x2)
		if ok {
			return x1
		}
	}

	return x1
}

func (d *DeviceDriver) updateDeviceData(p *string, data []byte) error {
	req, err := d.GnmiClient.CreateSetRequest(p, data)
	if err != nil {
		log.WithError(err).Error("error creating set request")
		return err
	}
	resp, err := d.GnmiClient.Set(context.Background(), req)
	if err != nil {
		log.WithError(err).Error("error sending set request for update")
		return err
	}
	log.Infof("response: %v", resp)

	/*
		u, err := d.Client.HandleSetRespone(response)
		if err != nil {
			return  err
		}
	*/
	return nil
}

func (d *DeviceDriver) deleteDeviceData(p *string) error {
	req, err := d.GnmiClient.CreateDeleteRequest(p)
	if err != nil {
		log.WithError(err).Error("error creating set request")
		return err
	}
	resp, err := d.GnmiClient.Set(context.Background(), req)
	if err != nil {
		log.WithError(err).Error("error sending set request for delete")
		return err
	}
	log.Infof("response: %v", resp)

	/*
		u, err := d.Client.HandleSetRespone(response)
		if err != nil {
			return  err
		}
	*/
	return nil
}
