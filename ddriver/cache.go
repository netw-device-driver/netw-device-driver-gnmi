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
	"strings"
	"sync"

	jsonpatch "github.com/evanphx/json-patch"
	log "github.com/sirupsen/logrus"

	"github.com/netw-device-driver/netwdevpb"
)

// Cache contains the
type Cache struct {
	netwdevpb.UnimplementedCacheStatusServer
	netwdevpb.UnimplementedCacheUpdateServer

	Mutex         sync.RWMutex
	Data          map[int]map[string]*ResourceData
	Levels        []int
	CurrentConfig []byte
}

type ResourceData struct {
	Config      *netwdevpb.CacheUpdateRequest
	CacheStatus netwdevpb.CacheStatusReply_CacheResourceStatus // Status of the resource
}

// UpdateCacheEntry updates the driver cache
/*
func (c *Cache) UpdateCacheEntry(o string, netwCfgMsg *netwdevpb.CacheUpdateRequest) error {
	c.Mutex.Lock()

	level := int(netwCfgMsg.Level)
	if !contains(c.Levels, level) {
		c.Levels = append(c.Levels, level)
		c.Data[level] = make(map[string]*Data)
	}
	sort.Ints(c.Levels)

	c.Data[level][o] = &Data{
		Config: netwCfgMsg,
		CacheStatus:   netwdevpb.CacheStatusReply_ToBeProcessed,
	}

	c.Mutex.Unlock()
	return nil
}
*/

// SetStatus sets the status of the data
// l = level, o = object
func (c *Cache) SetStatus(l int, o string, s netwdevpb.CacheStatusReply_CacheResourceStatus) error {
	c.Mutex.Lock()

	log.Infof("New Status for Level: %d, Object: %s, CacheStatus: %s", l, o, s)
	if d, ok := c.Data[l][o]; ok {
		d.CacheStatus = s
	}

	c.Mutex.Unlock()
	return nil
}

func (c *Cache) getParentDependencyDeleteStatus(dependencies []string) bool {
	log.Infof("Check Parent dependency status: %s", dependencies)
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
	log.Error("getParentDependencyDeleteSuccess: we should never come here since the parent dependency should be found")
	return false
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
				for _, dp := range data.Config.IndividualActionPath {
					log.Infof("Object: %s, ObjDependency: %s Global Dependency: %s", o, dep, dp)
					if strings.Contains(dp, dep) {
						// we can check the status since we processed the data before
						if data.CacheStatus != netwdevpb.CacheStatusReply_DependencyMissing {
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
			log.Infof("CACHE DATA: Object %s, Level: %d, Action: %s, CacheStatus %s, Dependencies: %v, IndividualPath: %v", o, l, data.Config.Action, data.CacheStatus, data.Config.Dependencies, data.Config.IndividualActionPath)
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

// ReconcileCache reconciles the cache
func (d *DeviceDriver) ReconcileCache() error {
	// SHOW CACHE STATUS
	log.Info("Show STATUS before DELETE PRE-PROCESSING")
	d.Cache.showCacheStatus()

	// PROCESS DELETE ACTIONS
	deletePaths := make(map[int]map[string]*ResourceData)
	// walk over the cache sorted with level and paths per level
	for _, l := range d.Cache.Levels {
		for o, data := range d.Cache.Data[l] {
			if len(data.Config.Dependencies) == 0 {
				// no object dependencies
				if data.Config.Action == netwdevpb.CacheUpdateRequest_Delete {
					log.Info("Delete pending Addition, no dependencies")
					d.Cache.SetStatus(l, o, netwdevpb.CacheStatusReply_DeletePending)
					// update deletePath; if the object did not exist initialize it and add the data or just add the data
					if _, ok := deletePaths[l]; !ok {
						deletePaths[l] = make(map[string]*ResourceData)
					}
					deletePaths[l][o] = data
				}
			} else {
				// check object dependencies
				for _, dep := range data.Config.Dependencies {
					// object's parent dependency gets deleted
					log.Infof("Delete dependencies: object: %s, dep: %s, ", o, dep)
					// dl = dependency level, do = dependency object
					dl, do, parentFound := d.Cache.CheckCache(dep)
					if parentFound {
						// parent dependency object is -> d.Cache.Data[l][dep]
						// parent object will be deleted
						if d.Cache.Data[dl][do].CacheStatus == netwdevpb.CacheStatusReply_DeletePending || d.Cache.Data[dl][do].CacheStatus == netwdevpb.CacheStatusReply_DeletePendingWithParentDependency {
							switch d.Cache.Data[l][o].Config.Action {
							case netwdevpb.CacheUpdateRequest_Delete:
								log.Info("Delete pending Addition with parent dependency, since parent object will be deleted")
								// the parent is to be deleted, so the child object will be deleted as well
								// -> we need to update the cache status but no gnmi delete
								d.Cache.SetStatus(l, o, netwdevpb.CacheStatusReply_DeletePendingWithParentDependency)
								// update deletePath; if the object did not exist initialize it and add the data or just add the data
								if _, ok := deletePaths[l]; !ok {
									deletePaths[l] = make(map[string]*ResourceData)
								}
								deletePaths[l][o] = data
							default:
								log.Info("No delete pending Addition, since parent object will be deleted, but child object remains")
								// dont delete the object since the parent just gets deleted and the object is still valid in the config
								d.Cache.SetStatus(l, o, netwdevpb.CacheStatusReply_DependencyMissing)
							}
						} else {
							// the parent object exists, but will not be deleted so we need to delete the child if the Action is delete
							if d.Cache.Data[l][o].Config.Action == netwdevpb.CacheUpdateRequest_Delete {
								log.Info("Delete pending Addition, parent object exists and not being deleted")
								d.Cache.SetStatus(l, o, netwdevpb.CacheStatusReply_DeletePending)
								// update deletePath; if the object did not exist initialize it and add the data or just add the data
								if _, ok := deletePaths[l]; !ok {
									deletePaths[l] = make(map[string]*ResourceData)
								}
								deletePaths[l][o] = data
							}
						}
					} else {
						// no parent object exists, we can just delete the object
						if d.Cache.Data[l][o].Config.Action == netwdevpb.CacheUpdateRequest_Delete {
							d.Cache.SetStatus(l, o, netwdevpb.CacheStatusReply_DeleteWithMissingDependency)
							if _, ok := deletePaths[l]; !ok {
								deletePaths[l] = make(map[string]*ResourceData)
							}
							deletePaths[l][o] = data
						} else {
							// just update the status with missing dependency
							d.Cache.SetStatus(l, o, netwdevpb.CacheStatusReply_DependencyMissing)
						}
					}
				}
			}
		}
	}

	log.Info("Show STATUS before DELETE GNMI")
	d.Cache.showCacheStatus()

	log.Infof("deletePaths: %v", deletePaths)

	// the delete should happen per level starting from the top since the delete hierarchy is respected
	// if interface and subinterfaace get delete, only delete interface via gnmi as the dependnt object
	//  will be deleted due to that
	for _, l := range d.Cache.Levels {
		for o, data := range deletePaths[l] {
			// we assume for a second that the deletion will be successful,
			// so that later if one delete fails we keep it in failed state
			// for thi object
			data.Config.AggregateActionPathSuccess = true
			for _, ip := range data.Config.IndividualActionPath {
				// - DependencyMissing -> child object is not deleted, but parent can get deleted or is not present
				// - DeletePending -> child object will be deleted through gnmi which can be successfull or not
				// - DeletePendingWithParentDependency -> child object and parent object gets deleted, but child status
				// is pending the success of the parent deletion
				// - DeleteWithMissingDependency -> child object gets deleted, but since there was no parent we can
				// just delete it w/o gnmi interaction
				switch data.CacheStatus {
				case netwdevpb.CacheStatusReply_DependencyMissing:
					log.Error("This stat should never be processed here, since the dependncy is missing")
				case netwdevpb.CacheStatusReply_DeletePending:
					// process the gnmi to delete the object
					if err := d.deleteDeviceDataGnmi(&ip); err != nil {
						log.WithError(err).Errorf("GNMI delete process failed, object: %s, path: %s", o, ip)
						// When the delete gnmi action failed we update the status back to ToBeProcessed
						// and indicate the failure in the status
						data.CacheStatus = netwdevpb.CacheStatusReply_ToBeProcessed // so in the next iteration we retry
						data.Config.AggregateActionPathSuccess = false
						data.Config.IndividualActionPathSuccess = append(data.Config.IndividualActionPathSuccess, false)
						//informK8sOperator(dp)
					} else {
						// only update the individual status since we assume the aggregate object is success
						data.Config.IndividualActionPathSuccess = append(data.Config.IndividualActionPathSuccess, true)
						log.Infof("GNMI delete processed successfully, object: %s, path: %s", o, ip)
					}
				case netwdevpb.CacheStatusReply_DeletePendingWithParentDependency:
					// check if the parent got deleted successfully and only than delete the object and cry success
					// E.g. delete interface and subinterfaces simultenously
					if d.Cache.getParentDependencyDeleteStatus(data.Config.Dependencies) {
						// parent delete was successfull
						data.Config.IndividualActionPathSuccess = append(data.Config.IndividualActionPathSuccess, true)
						log.Infof("Parent GNMI delete processed successfully, object: %s, path: %s", o, ip)
					} else {
						// parent delete was NOT successfull
						data.CacheStatus = netwdevpb.CacheStatusReply_ToBeProcessed // so in the next iteration we retry
						data.Config.AggregateActionPathSuccess = false
						data.Config.IndividualActionPathSuccess = append(data.Config.IndividualActionPathSuccess, false)
					}
				case netwdevpb.CacheStatusReply_DeleteWithMissingDependency:
					// dont delete through gnmi and the deletion will be successful since there is no parent dependency
					// e.g. delete subinterface object, which had no interface configured ever
					// since this object was never applied to the device we can just delete it w/o
					// deleting the object on the device through gnmi
					data.Config.IndividualActionPathSuccess = append(data.Config.IndividualActionPathSuccess, true)

				}
			}
			// DOING ACTUAL DELETE OPERATION On CACHE
			if data.Config.AggregateActionPathSuccess {
				// all deltions where positive -> update the status in nats, delete the object from the cache
				/*
					if err := d.updateK8sOperatorThroughNats(true, netwdevpb.Config_Delete, o, data); err != nil {
						log.WithError(err).Error("Failed to update nats")
					}
				*/
				// delete object from cache
				delete((*d.Cache).Data[l], o)
			} else {
				// one or all deletions failed -> update the status in nats, dont delete
				// the object from the cache since we will retry in the next reconciliation iteration
				/*
					if err := d.updateK8sOperatorThroughNats(false, netwdevpb.Config_Delete, o, data); err != nil {
						log.WithError(err).Error("Failed to update nats")
					}
				*/
			}
		}
	}

	// PREPROCESS UPDATE ACTIONS

	log.Info("Show STATUS BEFORE UPDATE PROCESSING AND AFTER DELETE PROCESSING")
	d.Cache.showCacheStatus()

	var mergedData interface{}
	var mergedPath string
	var err error

	// walk over the cache sorted with level and objects per level
	for _, l := range d.Cache.Levels {
		for o, data := range d.Cache.Data[l] {
			// only process Updates that are in the cache, no Deletes, etc
			if data.Config.Action == netwdevpb.CacheUpdateRequest_Update {
				if d.Cache.CheckMissingDependency(data.Config.Dependencies) {
					// parent object dependencies are missing
					d.Cache.SetStatus(l, o, netwdevpb.CacheStatusReply_DependencyMissing)
				} else {
					// merge the data based on the UpdateActionPath
					log.Infof("New Update Path: aggregate path %s, individual path %s, current Update Path: %s", data.Config.AggregateActionPath, data.Config.IndividualActionPath, mergedPath)
					// merge or insert the data object per object rather than through a full list
					// We merge per individual path the data
					for i, d := range data.Config.ConfigData {
						log.Infof("Start MERGE; individual path: %s, aggregate path: %s, mergedPath: %s", data.Config.IndividualActionPath[i], data.Config.AggregateActionPath, mergedPath)
						var d1 interface{}
						json.Unmarshal(d, &d1)
						log.Debugf("Start MERGE; new data: %v", d1)
						log.Debugf("Start MERGE; current merged data: %v", mergedData)
						mergedPath, mergedData, err = startMerge(mergedPath, mergedData, data.Config.IndividualActionPath[i], data.Config.AggregateActionPath, d)
						if err != nil {
							log.WithError(err).Error("merge error")
						}
					}
					d.Cache.SetStatus(l, o, netwdevpb.CacheStatusReply_UpdateBeingProcessed)
				}
			}
		}
	}

	// PROCESS UPDATE AND SHOW CACHE BEFORE UPDATE
	log.Info("Show STATUS before UPDATE")
	d.Cache.showCacheStatus()

	newMergedConfig, err := json.Marshal(mergedData)
	if err != nil {
		log.Fatal(err)
	}

	if jsonpatch.Equal(d.Cache.CurrentConfig, newMergedConfig) {
		log.Info("The new merged data is EQUAL to the current config. DO NOTHING")
		d.UpdateCacheAfterUpdate(true)
	} else {
		log.Info("The new merged data is DIFFERENT, apply to the device")
		log.Infof("MergedUpdatePath: %s \n", mergedPath)
		log.Infof("MergedData: %s \n", newMergedConfig)
		d.Cache.CurrentConfig = newMergedConfig

		// Only Update the device when the data is present
		if len(mergedPath) > 0 {
			updateSuccess := true
			if err := d.updateDeviceDataGnmi(&mergedPath, newMergedConfig); err != nil {
				// TODO check failure status
				log.WithError(err).Errorf("Merged update process FAILED, path: %s", mergedPath)
				updateSuccess = false
			}
			log.Infof("Merged update process SUCCEEDED, path: %s", mergedPath)
			// update Cache based on the result of the update
			d.UpdateCacheAfterUpdate(updateSuccess)
		}
	}
	log.Info("Show STATUS after UPDATE")
	d.Cache.showCacheStatus()

	return nil
}

func (d *DeviceDriver) UpdateCacheAfterUpdate(updateSuccess bool) error {
	for _, l := range d.Cache.Levels {
		for o, data := range d.Cache.Data[l] {
			if data.Config.Action == netwdevpb.CacheUpdateRequest_Update && data.CacheStatus == netwdevpb.CacheStatusReply_UpdateBeingProcessed {
				if updateSuccess {
					data.Config.AggregateActionPathSuccess = true
					for _, ip := range data.Config.IndividualActionPath {
						log.Infof("update processed successfully, object: %s, path: %s", o, ip)
						data.Config.IndividualActionPathSuccess = append(data.Config.IndividualActionPathSuccess, true)
					}
					d.Cache.SetStatus(l, o, netwdevpb.CacheStatusReply_UpdateProcessedSuccess)
				} else {
					data.Config.AggregateActionPathSuccess = false
					for _, ip := range data.Config.IndividualActionPath {
						log.Infof("update processed successfully, object: %s, path: %s", o, ip)
						data.Config.IndividualActionPathSuccess = append(data.Config.IndividualActionPathSuccess, false)
					}
					d.Cache.SetStatus(l, o, netwdevpb.CacheStatusReply_UpdateProcessedFailed)
				}
			}
		}
	}
	return nil
}

/*
func (d *DeviceDriver) updateK8sOperatorThroughNats(ok bool, op netwdevpb.Config_ActionType, o string, data *Data2) error {
	var resp *netwdevpb.Config
	switch op {
	case netwdevpb.Config_Delete:
		resp = &netwdevpb.Config{
			Kind:                        netwdevpb.Config_Response,
			ApiGroup:                    data.Config.ApiGroup,
			Resource:                    data.Config.Resource,
			ResourceName:                data.Config.ResourceName,
			Namespace:                   data.Config.Namespace,
			Action:                      op,
			AggregateActionPath:         data.Config.AggregateActionPath,
			AggregateActionPathSuccess:  data.Config.AggregateActionPathSuccess,
			IndividualActionPath:        data.Config.IndividualActionPath,
			IndividualActionPathSuccess: data.Config.IndividualActionPathSuccess,
			ConfigData:                  nil,
			StatusData:                  nil,
		}
	case netwdevpb.Config_Update:
		resp = &netwdevpb.Config{
			Kind:                        netwdevpb.Config_Response,
			ApiGroup:                    data.Config.ApiGroup,
			Resource:                    data.Config.Resource,
			ResourceName:                data.Config.ResourceName,
			Namespace:                   data.Config.Namespace,
			Action:                      op,
			AggregateActionPath:         data.Config.AggregateActionPath,
			AggregateActionPathSuccess:  data.Config.AggregateActionPathSuccess,
			IndividualActionPath:        data.Config.IndividualActionPath,
			IndividualActionPathSuccess: data.Config.IndividualActionPathSuccess,
			ConfigData:                  nil,
			StatusData:                  nil,
		}
	default:
		return fmt.Errorf("unknown action type: %s", op)
	}

	// response topic: ndd.<resource>.<resourcename>.<device>
	topic := data.Config.ApiGroup + "." + strcase.UpperCamelCase(data.Config.Resource) + "." + strcase.UpperCamelCase(data.Config.ResourceName)
	n := &natsc.Client{
		Server: *d.NatsServer,
		Topic:  topic,
	}
	log.Infof("Published Nats response: topic: %s response: %v", topic, resp)
	n.Publish(resp)
	return nil
}
*/

func (d *DeviceDriver) updateDeviceDataGnmi(p *string, data []byte) error {
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

func (d *DeviceDriver) deleteDeviceDataGnmi(p *string) error {
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
