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

	"github.com/netw-device-driver/netw-device-driver-gnmi/pkg/gnmic"
	"github.com/netw-device-driver/netwdevpb"
)

// Cache contains the
type Cache struct {
	netwdevpb.UnimplementedCacheStatusServer
	netwdevpb.UnimplementedCacheUpdateServer

	Mutex                 sync.RWMutex
	NewK8sOperatorUpdates *bool
	Data                  map[int]map[string]*ResourceData
	Levels                []int
	NewOnChangeUpdates    *bool
	OnChangeReApplyCache  *bool
	OnChangeDeletes       *[]string
	OnChangeUpdates       *[]string
	OnChangeDeviations    *map[string]*Deviation
	CurrentConfig         []byte
}

type ResourceData struct {
	Config      *netwdevpb.CacheUpdateRequest
	CacheStatus netwdevpb.CacheStatusReply_CacheResourceStatus // Status of the resource
}

func (c *Cache) OnChangeCacheUpdates(autoPilot *bool, onChangeDeviations *map[string]*Deviation) {
	c.Mutex.Lock()

	for xpath, deviation := range *onChangeDeviations {
		// update the OnChangeDeviations in the cache
		(*c.OnChangeDeviations)[xpath] = deviation
		switch deviation.DeviationAction {
		case netwdevpb.Deviation_DeviationActionReApplyCache:
			*c.OnChangeReApplyCache = true
		case netwdevpb.Deviation_DeviationActionDelete:
			*c.NewOnChangeUpdates = true
			*c.OnChangeDeletes = append(*c.OnChangeDeletes, xpath)
		case netwdevpb.Deviation_DeviationActionIgnore:
			// do nothing
		case netwdevpb.Deviation_DeviationActionDeleteIgnoredByParent:
			// do nothing
		case netwdevpb.Deviation_DeviationActionIgnoreException:
			// do nothing
		default:
		}
	}
	c.Mutex.Unlock()
}

// SetStatus sets the status of the data
// l = level, o = object
func (c *Cache) SetStatus(l int, o string, s netwdevpb.CacheStatusReply_CacheResourceStatus) error {
	c.Mutex.Lock()

	log.Debugf("New Status for Level: %d, Object: %s, CacheStatus: %s", l, o, s)
	if d, ok := c.Data[l][o]; ok {
		d.CacheStatus = s
	}

	c.Mutex.Unlock()
	return nil
}

func (c *Cache) getParentDependencyDeleteStatus(dependencies []string) bool {
	log.Debugf("Check Parent dependency status: %s", dependencies)
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
	log.Debug("getParentDependencyDeleteSuccess: We come here since the parent got already deleted")
	return true
}

// CheckMissingDependency validates the dependencies of the path
func (c *Cache) CheckMissingDependency(dependencies []string) bool {
	log.Debugf("Check missing dependencies: %s", dependencies)
	for _, l := range c.Levels {
		for o, data := range c.Data[l] {
			if len(dependencies) == 0 {
				// no depedency for this object
				return false
			}
			for _, dep := range dependencies {
				log.Debugf("Dependency: %s To be checked within Object: %s", o, dep)
				for _, dp := range data.Config.IndividualActionPath {
					log.Debugf("Object: %s, ObjDependency: %s Global Dependency: %s", o, dep, dp)
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

func (c *Cache) showCacheStatus() {
	for _, l := range c.Levels {
		for o, data := range c.Data[l] {
			log.Infof("CACHE DATA: Object %s, Level: %d, Action: %s, CacheStatus %s, Dependencies: %v, LeafRefDependencies: %v, IndividualPath: %v", o, l, data.Config.Action, data.CacheStatus, data.Config.Dependencies, data.Config.LeafRefDependencies, data.Config.IndividualActionPath)
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

// ReconcileCache reconciles the cache
func (d *DeviceDriver) ReconcileCache() error {
	log.Infof("reconcile cache...")

	// show subdelta
	log.Infof("K8sOperator cache updates: %t", *d.Cache.NewK8sOperatorUpdates)
	log.Infof("OnChange ReApplyCacheData: %t", *d.Cache.OnChangeReApplyCache)
	log.Infof("OnChange Delete: %v", *d.Cache.OnChangeDeletes)
	log.Infof("OnChange Update: %v", *d.Cache.OnChangeUpdates)

	// show initial config
	//log.Infof("Initial Config: %v", d.InitialConfig)

	for _, ip := range *d.Cache.OnChangeDeletes {
		// process to delete the object via gnmi
		if err := d.deleteDeviceDataGnmi(&ip); err != nil {
			log.WithError(err).Errorf("GNMI delete process failed for subscription delta, path: %s", ip)
		} else {
			// only update the individual status since we assume the aggregate object is success
			log.Infof("GNMI delete processed successfully for subscription delta, path: %s", ip)
		}
	}

	for _, ip := range *d.Cache.OnChangeUpdates {
		// process to delete the object via gnmi
		if err := d.deleteDeviceDataGnmi(&ip); err != nil {
			log.WithError(err).Errorf("GNMI delete process failed for subscription delta, path: %s", ip)
		} else {
			// only update the individual status since we assume the aggregate object is success
			log.Infof("GNMI delete processed successfully for subscription delta, path: %s", ip)
		}
	}

	// SHOW CACHE STATUS
	log.Debugf("Show STATUS before DELETE PRE-PROCESSING")
	if *d.Debug {
		d.Cache.showCacheStatus()
	}

	if *d.Cache.NewK8sOperatorUpdates || (*d.AutoPilot && *d.Cache.OnChangeReApplyCache) {

		// PROCESS DELETE ACTIONS
		deletePaths := make(map[int]map[string]*ResourceData)
		// walk over the cache sorted with level and paths per level
		for _, l := range d.Cache.Levels {
			for o, data := range d.Cache.Data[l] {
				// check/update leafref dependencies
				//for _, leafRefDep := range data.Config.LeafRefDependencies {
				//	d.Cache.UpdateLeafRefDependency(leafRefDep)
				//}
				if len(data.Config.Dependencies) == 0 {
					// no object dependencies
					if data.Config.Action == netwdevpb.CacheUpdateRequest_ActionDelete {
						log.Debugf("Delete pending Addition, no dependencies")
						d.Cache.SetStatus(l, o, netwdevpb.CacheStatusReply_CacheResourceStatusDeletePending)
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
						log.Debugf("Delete dependencies: object: %s, dep: %s, ", o, dep)
						// dl = dependency level, do = dependency object
						dl, do, parentFound := d.Cache.CheckCache(dep)
						if parentFound {
							// parent dependency object is -> d.Cache.Data[l][dep]
							// parent object will be deleted
							if d.Cache.Data[dl][do].CacheStatus == netwdevpb.CacheStatusReply_CacheResourceStatusDeletePending || d.Cache.Data[dl][do].CacheStatus == netwdevpb.CacheStatusReply_CacheResourceStatusDeletePendingWithParentDependency {
								switch d.Cache.Data[l][o].Config.Action {
								case netwdevpb.CacheUpdateRequest_ActionDelete:
									log.Debugf("Delete pending Addition with parent dependency, since parent object will be deleted")
									// the parent is to be deleted, so the child object will be deleted as well
									// -> we need to update the cache status but no gnmi delete
									d.Cache.SetStatus(l, o, netwdevpb.CacheStatusReply_CacheResourceStatusDeletePendingWithParentDependency)
									// update deletePath; if the object did not exist initialize it and add the data or just add the data
									if _, ok := deletePaths[l]; !ok {
										deletePaths[l] = make(map[string]*ResourceData)
									}
									deletePaths[l][o] = data
								default:
									log.Debugf("No delete pending Addition, since parent object will be deleted, but child object remains")
									// dont delete the object since the parent just gets deleted and the object is still valid in the config
									d.Cache.SetStatus(l, o, netwdevpb.CacheStatusReply_CacheResourceStatusDependencyMissing)
								}
							} else {
								// the parent object exists, but will not be deleted so we need to delete the child if the Action is delete
								if d.Cache.Data[l][o].Config.Action == netwdevpb.CacheUpdateRequest_ActionDelete {
									log.Debugf("Delete pending Addition, parent object exists and not being deleted")
									d.Cache.SetStatus(l, o, netwdevpb.CacheStatusReply_CacheResourceStatusDeletePending)
									// update deletePath; if the object did not exist initialize it and add the data or just add the data
									if _, ok := deletePaths[l]; !ok {
										deletePaths[l] = make(map[string]*ResourceData)
									}
									deletePaths[l][o] = data
								}
							}
						} else {
							// no parent object exists, we can just delete the object
							if d.Cache.Data[l][o].Config.Action == netwdevpb.CacheUpdateRequest_ActionDelete {
								d.Cache.SetStatus(l, o, netwdevpb.CacheStatusReply_CacheResourceStatusDeleteWithMissingDependency)
								if _, ok := deletePaths[l]; !ok {
									deletePaths[l] = make(map[string]*ResourceData)
								}
								deletePaths[l][o] = data
							} else {
								// just update the status with missing dependency
								d.Cache.SetStatus(l, o, netwdevpb.CacheStatusReply_CacheResourceStatusDependencyMissing)
							}
						}
					}
				}
			}
		}

		// update the local cache before the delete to ensure the dynamic subscription notification take into
		// account the latest cache that is aligned with the delete actions
		log.Debugf("Show STATUS before DELETE PRE-PROCESSING")
		var mergedData interface{}
		var mergedPath string
		//mergedData = d.InitialConfig
		mergedPath = "/"
		var mergedConfigIsEqual bool
		var err error

		// walk over the cache sorted with level and objects per level
		for _, l := range d.Cache.Levels {
			for _, data := range d.Cache.Data[l] {
				// only process Updates that are in the cache, no Deletes, etc
				if data.Config.Action == netwdevpb.CacheUpdateRequest_ActionUpdate {
					if d.Cache.CheckMissingDependency(data.Config.Dependencies) {
						// parent object dependencies are missing
						//// d.Cache.SetStatus(l, o, netwdevpb.CacheStatusReply_DependencyMissing)
					} else {
						// merge the data based on the UpdateActionPath
						log.Debugf("New Update Path: aggregate path %s, individual path %s, current Update Path: %s", data.Config.AggregateActionPath, data.Config.IndividualActionPath, mergedPath)
						// merge or insert the data object per object rather than through a full list
						// We merge per individual path the data
						for i, d := range data.Config.ConfigData {
							log.Debugf("Start MERGE; individual path: %s, aggregate path: %s, mergedPath: %s", data.Config.IndividualActionPath[i], data.Config.AggregateActionPath, mergedPath)
							var d1 interface{}
							json.Unmarshal(d, &d1)
							log.Debugf("Start MERGE; new data: %v", d1)
							log.Debugf("Start MERGE; current merged data: %v", mergedData)
							mergedPath, mergedData, err = startMerge(mergedPath, mergedData, data.Config.IndividualActionPath[i], data.Config.AggregateActionPath, d)
							if err != nil {
								log.WithError(err).Error("merge error")
							}
						}
						//// d.Cache.SetStatus(l, o, netwdevpb.CacheStatusReply_UpdateBeingProcessed)
					}
				}
			}
		}

		newMergedConfig, err := json.Marshal(mergedData)
		if err != nil {
			log.Fatal(err)
		}
		if jsonpatch.Equal(d.Cache.CurrentConfig, newMergedConfig) {
			mergedConfigIsEqual = true
		} else {
			mergedConfigIsEqual = false
			d.Cache.CurrentConfig = newMergedConfig
		}

		// show initial config
		log.Infof("Initial Config: %v", d.InitialConfig)
		// show initial config
		log.Infof("Merge is Equal: %t", mergedConfigIsEqual)
		log.Infof("Merged Config: %v", mergedData)

		log.Infof("Show STATUS before DELETE GNMI")
		d.Cache.showCacheStatus()
		if *d.Debug {
			d.Cache.showCacheStatus()
		}

		for k1, v1 := range deletePaths {
			for k2, v2 := range v1 {
				log.Infof("deletePaths Level %d, path: %s: %v", k1, k2, *v2)
			}
		}
		log.Infof("deletePaths: %v", deletePaths)

		// the delete should happen per level starting from the top since the delete hierarchy is respected
		// if interface and subinterfaace get delete, only delete interface via gnmi as the dependent object
		// will be deleted due to that
		for _, l := range d.Cache.Levels {
			for o, data := range deletePaths[l] {
				// we assume for a second that the deletion will be successful,
				// so that later if one delete fails we keep it in failed state
				// for thi object
				data.Config.AggregateActionPathSuccess = true
				// clean IndividualActionPathSuccess
				data.Config.IndividualActionPathSuccess = make([]bool, 0)
				for i, ip := range data.Config.IndividualActionPath {
					// append the IndividualActionPathSuccess and initialize them
					data.Config.IndividualActionPathSuccess = append(data.Config.IndividualActionPathSuccess, false)

					// - DependencyMissing -> child object is not deleted, but parent can get deleted or is not present
					// - DeletePending -> child object will be deleted through gnmi which can be successfull or not
					// - DeletePendingWithParentDependency -> child object and parent object gets deleted, but child status
					// is pending the success of the parent deletion
					// - DeleteWithMissingDependency -> child object gets deleted, but since there was no parent we can
					// just delete it w/o gnmi interaction
					switch data.CacheStatus {
					case netwdevpb.CacheStatusReply_CacheResourceStatusDependencyMissing:
						log.Error("This state should never be processed here, since the dependency is missing")
					case netwdevpb.CacheStatusReply_CacheResourceStatusDeletePending, netwdevpb.CacheStatusReply_CacheResourceStatusUpdateBeingProcessed:
						// process the gnmi to delete the object
						if err := d.deleteDeviceDataGnmi(&ip); err != nil {
							log.WithError(err).Errorf("GNMI delete process failed, object: %s, path: %s", o, ip)
							// When the delete gnmi action failed we update the status back to ToBeProcessed
							// and indicate the failure in the status
							data.CacheStatus = netwdevpb.CacheStatusReply_CacheResourceStatusToBeProcessed // so in the next iteration we retry
							data.Config.AggregateActionPathSuccess = false
							data.Config.IndividualActionPathSuccess[i] = false
							//informK8sOperator(dp)
						} else {
							// only update the individual status since we assume the aggregate object is success
							data.Config.IndividualActionPathSuccess[i] = true
							log.Infof("GNMI delete processed successfully, object: %s, path: %s", o, ip)
						}
					case netwdevpb.CacheStatusReply_CacheResourceStatusDeletePendingWithParentDependency:
						// check if the parent got deleted successfully and only than delete the object and cry success
						// E.g. delete interface and subinterfaces simultenously
						if d.Cache.getParentDependencyDeleteStatus(data.Config.Dependencies) {
							// parent delete was successfull
							data.Config.IndividualActionPathSuccess[i] = true
							log.Debugf("Parent GNMI delete processed successfully, object: %s, path: %s", o, ip)
						} else {
							// parent delete was NOT successfull
							data.CacheStatus = netwdevpb.CacheStatusReply_CacheResourceStatusToBeProcessed // so in the next iteration we retry
							data.Config.AggregateActionPathSuccess = false
							data.Config.IndividualActionPathSuccess[i] = false
						}
					case netwdevpb.CacheStatusReply_CacheResourceStatusDeleteWithMissingDependency:
						// dont delete through gnmi and the deletion will be successful since there is no parent dependency
						// e.g. delete subinterface object, which had no interface configured ever
						// since this object was never applied to the device we can just delete it w/o
						// deleting the object on the device through gnmi
						data.Config.IndividualActionPathSuccess[i] = true

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

		log.Infof("Show STATUS BEFORE UPDATE PROCESSING AND AFTER DELETE PROCESSING")
		d.Cache.showCacheStatus()
		if *d.Debug {
			d.Cache.showCacheStatus()
		}

		//var mergedData interface{}
		//var mergedPath string
		//mergedData = d.InitialConfig
		mergedPath = "/"
		//var err error

		// walk over the cache sorted with level and objects per level
		for _, l := range d.Cache.Levels {
			for o, data := range d.Cache.Data[l] {
				// only process Updates that are in the cache, no Deletes, etc
				if data.Config.Action == netwdevpb.CacheUpdateRequest_ActionUpdate {
					if d.Cache.CheckMissingDependency(data.Config.Dependencies) {
						// parent object dependencies are missing
						d.Cache.SetStatus(l, o, netwdevpb.CacheStatusReply_CacheResourceStatusDependencyMissing)
					} else {
						// merge the data based on the UpdateActionPath
						log.Debugf("New Update Path: aggregate path %s, individual path %s, current Update Path: %s", data.Config.AggregateActionPath, data.Config.IndividualActionPath, mergedPath)
						// merge or insert the data object per object rather than through a full list
						// We merge per individual path the data
						for i, d := range data.Config.ConfigData {
							log.Debugf("Start MERGE; individual path: %s, aggregate path: %s, mergedPath: %s", data.Config.IndividualActionPath[i], data.Config.AggregateActionPath, mergedPath)
							var d1 interface{}
							json.Unmarshal(d, &d1)
							log.Debugf("Start MERGE; new data: %v", d1)
							log.Debugf("Start MERGE; current merged data: %v", mergedData)
							mergedPath, mergedData, err = startMerge(mergedPath, mergedData, data.Config.IndividualActionPath[i], data.Config.AggregateActionPath, d)
							if err != nil {
								log.WithError(err).Error("merge error")
							}
						}
						d.Cache.SetStatus(l, o, netwdevpb.CacheStatusReply_CacheResourceStatusUpdateBeingProcessed)
					}
				}
			}
		}

		// PROCESS UPDATE AND SHOW CACHE BEFORE UPDATE
		log.Infof("Show STATUS before UPDATE")
		d.Cache.showCacheStatus()
		if *d.Debug {
			d.Cache.showCacheStatus()
		}

		newMergedConfig, err = json.Marshal(mergedData)
		if err != nil {
			log.Fatal(err)
		}

		if jsonpatch.Equal(d.Cache.CurrentConfig, newMergedConfig) {
			if *d.Cache.OnChangeReApplyCache || !mergedConfigIsEqual {
				log.Info("The new merged data is EQUAL to the current config, but an object got deleted -> REAPPLY CACHE")

				// update the configmap
				if err := d.ConfigMapUpdate(StringPtr(string(d.Cache.CurrentConfig))); err != nil {
					log.WithError(err).Error("Update configmap failed")
				}

				//if len(mergedPath) > 0 {
				log.Infof("MergedUpdatePath: %s", mergedPath)
				log.Infof("MergedData: %s", newMergedConfig)
				log.Infof("MergedData length: %d", len(newMergedConfig))
				if string(newMergedConfig) != "null" && len(newMergedConfig) > 0 {
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
			} else {
				log.Info("The new merged data is EQUAL to the current config. DO NOTHING")
				d.UpdateCacheAfterUpdate(true)
			}
		} else {
			log.Info("The new merged data is DIFFERENT, apply to the device")
			log.Infof("MergedUpdatePath: %s ", mergedPath)
			log.Infof("MergedData: %s ", newMergedConfig)
			log.Infof("MergedData length: %d", len(newMergedConfig))
			d.Cache.CurrentConfig = newMergedConfig

			// update the configmap
			if err := d.ConfigMapUpdate(StringPtr(string(d.Cache.CurrentConfig))); err != nil {
				log.WithError(err).Error("Update configmap failed")
			}

			// Only Update the device when the data is present
			//if len(mergedPath) > 0 {
			log.Infof("New Merged Config %v", newMergedConfig)
			if string(newMergedConfig) != "null" && len(newMergedConfig) > 0 {
				updateSuccess := true
				if err := d.updateDeviceDataGnmi(&mergedPath, newMergedConfig); err != nil {
					// TODO check failure status
					log.WithError(err).Errorf("Merged update process FAILED, path: %s", mergedPath)
					updateSuccess = false
				} else {
					log.Infof("Merged update process SUCCEEDED, path: %s", mergedPath)
					// update Cache based on the result of the update
					d.UpdateCacheAfterUpdate(updateSuccess)
				}
			}
		}
	}
	d.Cache.Mutex.Lock()
	*d.Cache.OnChangeReApplyCache = false
	*d.Cache.OnChangeDeletes = make([]string, 0)
	*d.Cache.OnChangeUpdates = make([]string, 0)
	*d.Cache.NewK8sOperatorUpdates = false
	*d.Cache.NewOnChangeUpdates = false
	d.Cache.Mutex.Unlock()
	log.Info("Show CACHE STATUS after UPDATE")
	d.Cache.showCacheStatus()

	return nil
}

func (d *DeviceDriver) UpdateCacheAfterUpdate(updateSuccess bool) error {
	for _, l := range d.Cache.Levels {
		for o, data := range d.Cache.Data[l] {
			if data.Config.Action == netwdevpb.CacheUpdateRequest_ActionUpdate && data.CacheStatus == netwdevpb.CacheStatusReply_CacheResourceStatusUpdateBeingProcessed {
				if updateSuccess {
					data.Config.AggregateActionPathSuccess = true
					// reiniliaze the individualPath success
					data.Config.IndividualActionPathSuccess = make([]bool, 0)
					for _, ip := range data.Config.IndividualActionPath {
						log.Debugf("update processed successfully, object: %s, path: %s", o, ip)
						data.Config.IndividualActionPathSuccess = append(data.Config.IndividualActionPathSuccess, true)
					}
					d.Cache.SetStatus(l, o, netwdevpb.CacheStatusReply_CacheResourceStatusUpdateProcessedSuccess)
				} else {
					data.Config.AggregateActionPathSuccess = false
					// reiniliaze the individualPath success
					data.Config.IndividualActionPathSuccess = make([]bool, 0)
					for _, ip := range data.Config.IndividualActionPath {
						log.Debugf("update processed successfully, object: %s, path: %s", o, ip)
						data.Config.IndividualActionPathSuccess = append(data.Config.IndividualActionPathSuccess, false)
					}
					d.Cache.SetStatus(l, o, netwdevpb.CacheStatusReply_CacheResourceStatusUpdateProcessedFailed)
				}
			}
		}
	}
	return nil
}

func (d *DeviceDriver) updateDeviceDataGnmi(p *string, data []byte) error {

	req, err := gnmic.CreateSetRequest(p, data)
	if err != nil {
		log.WithError(err).Error("error creating set request")
		return err
	}
	resp, err := d.Target.Set(context.Background(), req)
	if err != nil {
		log.WithError(err).Error("error sending set request for update")
		return err
	}
	log.Debugf("response: %v", resp)

	return nil
}

func (d *DeviceDriver) deleteDeviceDataGnmi(p *string) error {

	req, err := gnmic.CreateDeleteRequest(p)
	if err != nil {
		log.WithError(err).Error("error creating set request")
		return err
	}
	resp, err := d.Target.Set(context.Background(), req)
	if err != nil {
		log.WithError(err).Error("error sending set request for delete")
		return err
	}
	log.Debugf("response: %v", resp)

	return nil
}
