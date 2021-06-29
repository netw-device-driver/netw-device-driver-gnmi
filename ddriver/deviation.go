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
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/google/go-cmp/cmp"
	"github.com/netw-device-driver/netw-device-driver-gnmi/pkg/gnmic"
	"github.com/netw-device-driver/netwdevpb"
	"github.com/openconfig/gnmi/proto/gnmi"
	"github.com/r3labs/diff/v2"
	log "github.com/sirupsen/logrus"
)

func gnmiPathToXPath(pElems []*gnmi.PathElem) ([]ElementKeyValue, string) {
	var ekvl []ElementKeyValue
	sb := strings.Builder{}
	for i, pElem := range pElems {
		pes := strings.Split(pElem.GetName(), ":")
		var pe string
		if len(pes) > 1 {
			pe = pes[1]
		} else {
			pe = pes[0]
		}
		ekv := ElementKeyValue{
			Element:  pe,
			KeyName:  "",
			KeyValue: "",
		}
		sb.WriteString(pe)
		for k, v := range pElem.GetKey() {
			sb.WriteString("[")
			sb.WriteString(k)
			sb.WriteString("=")
			sb.WriteString(v)
			sb.WriteString("]")
			log.Debugf("pElem %s Key %s Value: %s", pe, k, v)
			ekv.KeyName = k
			ekv.KeyValue = v
		}
		if i+1 != len(pElems) {
			sb.WriteString("/")
		}
		ekvl = append(ekvl, ekv)
		//log.Infof("pElemKeyValue: %s", sb.String())
	}
	return ekvl, "/" + sb.String()

}

/*
type SubscriptionDelta struct {
	SubAction          *SubcriptionAction
	Value              *interface{}
	Ekvl               *[]ElementKeyValue
	Xpath              *string
	DataSubDeltaDelete *[]string
	DataSubDeltaUpdate *[]string
	ReApplyCacheData   *bool
}

type SubcriptionAction string

const (
	// ConfigStatusNone means the subscription action is update
	SubcriptionActionDelete SubcriptionAction = "delete"

	// SubcriptionActionUpdate means the subscription action is update
	SubcriptionActionUpdate SubcriptionAction = "update"
)

type SubcriptionKind string

const (
	// SubcriptionDelete means the subscription kind is delete
	SubcriptionDelete SubcriptionKind = "delete"

	// SubcriptionUpdate means the subscription kind is update
	SubcriptionUpdate SubcriptionKind = "update"
)

func SubscriptionActionPtr(s SubcriptionAction) *SubcriptionAction { return &s }
*/

func (c *Cache) locatePathInCache(pel []*gnmi.PathElem) (interface{}, bool, error) {
	var x1 interface{}
	err := json.Unmarshal(c.CurrentConfig, &x1)
	if err != nil {
		return nil, false, err
	}
	_, x1, f := findPathInTree(x1, pel, 0)
	// TODO validate data
	return x1, f, nil
}

func findPathInTree(x1 interface{}, pel []*gnmi.PathElem, i int) (int, interface{}, bool) {
	log.Infof("pel entry: %d %v %v", i, cleanStr(pel[i].GetName()), pel[i].GetKey())
	keyName := make([]string, 0)
	keyValue := make([]string, 0)
	if pel[i].GetKey() != nil {
		keyName, keyValue = getKeyInfo(pel[i].GetKey())
	}
	switch x1 := x1.(type) {
	case map[string]interface{}:
		if _, ok := x1[cleanStr(pel[i].GetName())]; ok {
			if i == len(pel)-1 {
				if pel[i].GetKey() != nil {
					//log.Infof("LAST OBJECT FOUND WITH KEY CONTINUE: %d, PEL: %v, X1: %v", i, pel, x1[cleanStr(pel[i].GetName())])
					return findPathInTree(x1[cleanStr(pel[i].GetName())], pel, i)
				} else {
					//log.Infof("LAST OBJECT FOUND END: %d, PEL: %v, X1: %v", i, pel, x1[cleanStr(pel[i].GetName())])
					return i, x1[cleanStr(pel[i].GetName())], true
				}
			} else {
				if pel[i].GetKey() != nil {
					//log.Infof("OBJECT FOUND WITH KEY CONTINUE: %d, PEL: %v, X1: %v", i, pel, x1[cleanStr(pel[i].GetName())])
					return findPathInTree(x1[cleanStr(pel[i].GetName())], pel, i)
				} else {
					//log.Infof("OBJECT FOUND WITHOUT KEY CONTINUE: %d, PEL: %v, X1: %v", i, pel, x1[cleanStr(pel[i].GetName())])
					return findPathInTree(x1[cleanStr(pel[i].GetName())], pel, i+1)
				}
			}
		}
		//log.Infof("OBJECT ELEMENT NOT FOUND: %d, PEL: %v, X1: %v", i, pel[i], x1)
		return i, nil, false
	case []interface{}:
		for n, x := range x1 {
			switch x2 := x.(type) {
			case map[string]interface{}:
				// this loop checks if the keyNames and keyValues match
				f := false
				for idx, keyName := range keyName {
					// check if the keyName exists
					if x3, ok := x2[keyName]; ok {
						// if the keyName exists, check if the keyValue matches
						switch x3.(type) {
						case string:
							if x3 == keyValue[idx] {
								//log.Infof("OBJECT FOUND STRING IN LIST END: %d, PEL: %v, X1: %v", i, pel, x1[n])
								f = true
							}
						case uint32:
							var v int
							v, _ = strconv.Atoi(fmt.Sprintf("%v", keyValue[idx]))
							if x3 == uint32(v) {
								//log.Infof("OBJECT FOUND UINT32 IN LIST END: %d, PEL: %v, X1: %v", i, pel, x1[n])
								f = true
							}
						case float64:
							var v float64
							v, _ = strconv.ParseFloat(fmt.Sprintf("%v", keyValue[idx]), 8)
							if x3 == v {
								//log.Infof("OBJECT FOUND FLOAT64 IN LIST END: %d, PEL: %v, X1: %v", i, pel, x1[n])
								f = true
							}
						default:
							log.Infof("Missing type -> LOOK AT THIS %v", reflect.TypeOf(x3))
						}
						if !f {
							// stop the for loop if the keyValue of the keyName does not match
							break
						}
					} else {
						// if keyName not found return false and break the for loop, since the keyName is not present in the data structure
						f = false
						break
					}
				}
				// if f= true all KeyNames/Key Values match, so we can continue
				if f {
					if i == len(pel)-1 {
						return i, x2, true
					} else {
						//log.Infof("OBJECT FOUND IN LIST CONTINUE: %d, PEL: %v, X1: %v", i, pel, x1[n])
						return findPathInTree(x2, pel, i+1)
					}
				}
				// else we let go and we will return found false if no matches were found
			}
			// continue the for loop as there can be multiple list entries that could match
			log.Infof("multiple []interface{} objects: n: %d, x: %v", n, x1)
		}
		return i, nil, false
	case nil:
		return i, nil, false
	default:
		log.Infof("findPathInTree default WE SHOULD NOT COME HERE %v", reflect.TypeOf(x1))
		return i, nil, false
	}
	//return i, nil, false
}

func cleanSubscriptionValueForComparison(val *gnmi.TypedValue) (map[string]interface{}, error) {
	x2 := make(map[string]interface{})
	// get the value from gnmic
	value, err := gnmic.GetValue(val)
	if err != nil {
		return nil, err
	}
	switch x := value.(type) {
	case map[string]interface{}:
		for k, v := range x {
			// if a string contains a : we return the last string after the :
			sk := strings.Split(k, ":")[len(strings.Split(k, ":"))-1]
			if k != sk {
				delete(x, k)
				switch v.(type) {
				case string:
					v = strings.Split(fmt.Sprintf("%v", v), ":")[len(strings.Split(fmt.Sprintf("%v", v), ":"))-1]
				}
				x[sk] = v
			}
		}
		x2 = x
	}
	return x2, nil
}

func cleanCacheDataForComparison(x1 interface{}, pel []*gnmi.PathElem) (interface{}, error) {
	// delete all leaftlists and keys of the cache data for comparison
	keyNames := make([]string, 0)
	if pel[len(pel)-1].GetKey() != nil {
		keyNames, _ = getKeyInfo(pel[len(pel)-1].GetKey())
	}
	// loop over multiple keys
	for _, keyName := range keyNames {
		switch x := x1.(type) {
		case map[string]interface{}:
			for k, v := range x {
				switch v.(type) {
				// delete maps since they come with a different xpath if present
				case map[string]interface{}:
					delete(x, k)
				// delete lists since they come with a different xpath if present
				case []interface{}:
					delete(x, k)
				case string:
					if k == keyName {
						delete(x, k)
					}
				case float64:
					if k == keyName {
						delete(x, k)
					}
				default:
					log.Infof("%T", v)
				}
			}
			x1 = x
		}
	}
	return x1, nil
}

func findDeviationAction(x1 interface{}, pel []*gnmi.PathElem, val *gnmi.TypedValue) (netwdevpb.Deviation_DeviationAction, error) {
	// delete all leaftlists and keys of the cache data for comparison
	var err error
	var deviationAction netwdevpb.Deviation_DeviationAction
	x1, err = cleanCacheDataForComparison(x1, pel)
	if err != nil {
		return 0, err
	}
	log.Infof("On-Change subscription: Update Diff => Cleaned Cache Data for Comparison: %v", x1)

	// remove the long names ensure the keys only contain the information without the :
	x2, err := cleanSubscriptionValueForComparison(val)
	if err != nil {
		return 0, err
	}
	log.Infof("On-Change subscription: Update Diff => Cleaned On-Change Data for Comparison: %v", x2)
	if !cmp.Equal(x1, x2) {
		// change = true
		changelog, _ := diff.Diff(x2, x1)
		//log.Infof("On-Change subscription: Update Diff Data from Network Device and cache is not equal, changelog : %v", changelog)
		for _, change := range changelog {
			if change.Type == diff.CREATE || change.Type == diff.UPDATE {
				// something got deleted or a value changed, we should reapply this to the cache
				log.Infof("On-Change subscription: Update Diff Something got deleted or changed -> reapply the cache, change: %v", change)
				deviationAction = netwdevpb.Deviation_DeviationActionReApplyCache
			} else {
				// something got created wich was not there, we can delete it
				log.Infof("On-Change subscription: Update Diff -> Something got created which was not there, delete it, change: %v", change)
				deviationAction = netwdevpb.Deviation_DeviationActionDelete
			}
		}
	} else {
		log.Infof("On-Change subscription: Update Diff -> Data from on-change subscription and cache is equal, Do Nothing")
		deviationAction = netwdevpb.Deviation_DeviationActionIgnore
	}
	// TODO we need to return this information
	return deviationAction, nil
}

/*
type OnChangeAction string

const (
	// OnChangeDelete means the onChange action is delete
	OnChangeDelete OnChangeAction = "delete"

	// OnChangeUpdate means the onChange action is update
	OnChangeUpdate OnChangeAction = "update"
)

func OnChangeActionPtr(s OnChangeAction) *OnChangeAction { return &s }

type DeviationAction string

const (
	// DeviationActionIgnore means the deviation action is ignore
	DeviationActionIgnore DeviationAction = "ignore"

	// DeviationActionReApplyCache means the devation action is reApplyCache
	DeviationActionReApplyCache DeviationAction = "reApplyCache"

	// DeviationActionDelete means the deviation action is delete
	DeviationActionDelete DeviationAction = "delete"

	// DeviationActionDeleteIgnoreByParent means the deviation action is ignore, because a parent delete is present
	DeviationActionDeleteIgnoreByParent DeviationAction = "deleteIgnoreByParent"
)

func DeviationActionPtr(s DeviationAction) *DeviationAction { return &s }
*/

type Deviation struct {
	OnChangeAction  netwdevpb.Deviation_OnChangeAction
	Pel             []*gnmi.PathElem
	Value           []byte
	DeviationAction netwdevpb.Deviation_DeviationAction
	Change          bool
}

func (d *DeviceDriver) processOnChangeUpdates(resp *gnmi.SubscribeResponse) error {
	switch resp.GetResponse().(type) {
	case *gnmi.SubscribeResponse_Update:
		// data structure to keep track of the onChange delta
		onChangeDeviations := make(map[string]*Deviation, 0)
		du := resp.GetUpdate().Delete
		// subscription DELETE per xpath
		for i, del := range du {
			// subscription DELETE per xpath
			_, xpath := gnmiPathToXPath(del.GetElem())
			// the cache data is not relevant in a on-change delete scenario so we ignore it
			x1, f, err := d.Cache.locatePathInCache(del.GetElem())
			if err != nil {
				return fmt.Errorf("On-Change subscription: locate path in cache error %v", err)
			}
			x1, err = cleanCacheDataForComparison(x1, del.GetElem())
			if err != nil {
				return err
			}
			data, err := json.Marshal(x1)
			if err != nil {
				return err
			}
			// fill out deviation, deviation action is filled in later
			deviation := &Deviation{
				OnChangeAction: netwdevpb.Deviation_OnChangeActionDelete,
				Pel:            del.GetElem(),
				Value:          data,
				Change:         true,
			}
			// if object was found or not
			if f {
				// xpath got deleted -> reapply to cache
				// k8s operator is no longer in sync
				log.Infof("On-Change subscription: delete %d: xpath: %s, found %t -> reapply the cache", i, xpath, f)
				deviation.DeviationAction = netwdevpb.Deviation_DeviationActionReApplyCache
			} else {
				// we delete something that was not managed by the k8s operator, so we can ignore it
				log.Infof("On-Change subscription: delete %d: xpath: %s, found %t -> ignore object was not managed by k8s operator", i, xpath, f)
				deviation.DeviationAction = netwdevpb.Deviation_DeviationActionIgnore
			}
			//log.Infof("Subscription delete %d: xpath: %s, found %t", i, xpath, f)
			onChangeDeviations[xpath] = deviation
		}
		u := resp.GetUpdate().Update
		// subscription UPDATE per xpath
		for i, upd := range u {
			// subscription UPDATE per xpath
			_, xpath := gnmiPathToXPath(upd.GetPath().GetElem())
			value, err := gnmic.GetValue(upd.GetVal())
			if err != nil {
				return err
			}
			x1, f, err := d.Cache.locatePathInCache(upd.GetPath().GetElem())
			if err != nil {
				return fmt.Errorf("On-Change subscription: locate path in cache error %v", err)
			}
			data, err := json.Marshal(value)
			if err != nil {
				return err
			}
			// fill out deviation, deviation action is filled in later
			deviation := &Deviation{
				OnChangeAction: netwdevpb.Deviation_OnChangeActionUpdate,
				Pel:            upd.GetPath().GetElem(),
				Value:          data,
				Change:         true,
			}
			// if object was found or not
			if f {
				// xpath got updated -> we need to find the diff/delta
				// deviation action
				deviation.DeviationAction, err = findDeviationAction(x1, upd.GetPath().GetElem(), upd.GetVal())
				if err != nil {
					return fmt.Errorf("diff error for subscription update %v", err)
				}
				log.Infof("Subscription update %d: xpath: %s, found %t -> did it change? %s", i, xpath, f, deviation.DeviationAction.String())

			} else {
				// xpath is/was not in the cache -> delete the object if not in exception list
				// we can also aggregate the path we delete since this is more scalable

				// in autopilot we need to ensure the exception path list is not deleted
				if *d.AutoPilot {
					if d.checkExceptionXpath(xpath) {
						deviation.DeviationAction = netwdevpb.Deviation_DeviationActionIgnoreException
					} else {
						aggregatedByOtherDelete := aggregateDeleteDeviations(&onChangeDeviations, &xpath)
						if aggregatedByOtherDelete {
							deviation.DeviationAction = netwdevpb.Deviation_DeviationActionDeleteIgnoredByParent
						} else {
							deviation.DeviationAction = netwdevpb.Deviation_DeviationActionDelete
						}
					}
				} else {
					deviation.DeviationAction = netwdevpb.Deviation_DeviationActionDelete
				}
				log.Infof("Subscription update %d: xpath: %s, found %t -> new data delete it final action: %s", i, xpath, f, deviation.DeviationAction.String())
			}

			onChangeDeviations[xpath] = deviation
			//log.Infof("Subscription update %d: xpath: %s, found %t", i, xpath, f)
		}
		d.Cache.OnChangeCacheUpdates(d.AutoPilot, &onChangeDeviations)

	case *gnmi.SubscribeResponse_SyncResponse:
	}
	return nil
}

func aggregateDeleteDeviations(onChangeDeviations *map[string]*Deviation, xpathNew *string) bool {
	var aggregate bool
	for xpath, deviation := range *onChangeDeviations {
		if strings.Contains(xpath, *xpathNew) {
			if deviation.DeviationAction == netwdevpb.Deviation_DeviationActionDelete {
				aggregate = true
			}
		}
		if strings.Contains(*xpathNew, xpath) {
			aggregate = false
			if deviation.DeviationAction == netwdevpb.Deviation_DeviationActionDelete {
				deviation.DeviationAction = netwdevpb.Deviation_DeviationActionDeleteIgnoredByParent
			}
		}
	}
	return aggregate
}

/*
func (d *DeviceDriver) validateDiffAutoPilot(resp *gnmi.SubscribeResponse) {
	var err error
	switch resp.GetResponse().(type) {
	case *gnmi.SubscribeResponse_Update:
		log.Info("SubscribeResponse_Update")

		subDeltaDelete := make([]string, 0)
		subDeltaUpdate := make([]string, 0)
		subDelta := &SubscriptionDelta{
			ReApplyCacheData:   BoolPtr(false),
			DataSubDeltaDelete: &subDeltaDelete,
			DataSubDeltaUpdate: &subDeltaUpdate,
		}

		du := resp.GetUpdate().Delete
		for i, del := range du {
			log.Infof("SubscribeResponse Delete Path %d", i)
			ekvl, xpath := gnmiPathToXPath(del.GetElem())

			subDelta.SubAction = SubscriptionActionPtr(SubcriptionActionDelete)
			subDelta.Ekvl = &ekvl
			subDelta.Xpath = &xpath

			subDelta, err = d.FindCacheObject(subDelta)
			if err != nil {
				log.WithError(err).Error("FindCacheObject error")
			}
		}

		u := resp.GetUpdate().Update
		for i, upd := range u {
			log.Infof("SubscribeResponse Update Path %d", i)
			ekvl, xpath := gnmiPathToXPath(upd.GetPath().GetElem())
			value, err := gnmic.GetValue(upd.GetVal())
			if err != nil {
				log.WithError(err).Error("validateDiff get Value error")
			}
			log.Infof("SubscribeResponse Update Path Nbr %d Ekvl %v Xpath %s", i, ekvl, xpath)
			subDelta.SubAction = SubscriptionActionPtr(SubcriptionActionUpdate)
			subDelta.Ekvl = &ekvl
			subDelta.Xpath = &xpath
			subDelta.Value = &value

			subDelta, err = d.FindCacheObject(subDelta)
			if err != nil {
				log.WithError(err).Error("FindCacheObject error")
			}
		}
		log.Infof("Subcribe delta SubAction: %v", *subDelta.SubAction)
		log.Infof("Subcribe delta DataSubDeltaDelete: %v", *subDelta.DataSubDeltaDelete)
		log.Infof("Subcribe delta DataSubDeltaUpdate: %v", *subDelta.DataSubDeltaUpdate)
		log.Infof("Subcribe delta ReApplyCacheData: %v", *subDelta.ReApplyCacheData)

		d.Cache.UpdateSubscriptionDelta(subDelta)
	case *gnmi.SubscribeResponse_SyncResponse:
		log.Infof("SubscribeResponse_SyncResponse")
	}
}
*/

/*
func (d *DeviceDriver) FindCacheObject(subDelta *SubscriptionDelta) (*SubscriptionDelta, error) {
	var x1 interface{}
	err := json.Unmarshal(d.Cache.CurrentConfig, &x1)
	if err != nil {
		return nil, err
	}
	log.Debugf("Cache Config: %v", x1)
	log.Infof("EKVL : %v", *subDelta.Ekvl)
	i, x1, f := findObject(x1, *subDelta.Ekvl, 0)
	if f {
		if *subDelta.SubAction == SubcriptionActionUpdate {
			log.Infof("OBJECT FINALLY FOUND: %v", x1)
			log.Infof("Update: ok - %s", *subDelta.Xpath)
			// delete all leaftlists of the data for comparison
			// delete the key as well
			switch x := x1.(type) {
			case map[string]interface{}:
				for k, v := range x {
					switch v.(type) {
					case map[string]interface{}:
						delete(x, k)
					case []interface{}:
						delete(x, k)
					case string:
						if k == (*subDelta.Ekvl)[i].KeyName {
							delete(x, k)
						}
					case float64:
						if k == (*subDelta.Ekvl)[i].KeyName {
							delete(x, k)
						}
					default:
						log.Infof("%T", v)
					}
				}
				x1 = x
			}
			log.Infof("Update: Data in Cache    : %v", x1)
			x2 := make(map[string]interface{})
			switch x := (*subDelta.Value).(type) {
			case map[string]interface{}:
				for k, v := range x {
					sk := strings.Split(k, ":")[len(strings.Split(k, ":"))-1]
					if k != sk {
						delete(x, k)
						x[sk] = v
					}
				}
				x2 = x
			}
			log.Infof("Update: Data from Network Device : %v", x2)

			if !cmp.Equal(x1, x2) {
				// change = true
				changelog, _ := diff.Diff(x2, x1)
				log.Infof("Update: Data from Network Device and cache is not equal, changelog : %v", changelog)
				for _, change := range changelog {
					if change.Type == diff.CREATE || change.Type == diff.UPDATE {
						subDelta.ReApplyCacheData = BoolPtr(true)
					} else {
						// delete
						updateSubDeltaUpdate(subDelta, &change)
					}
				}
			} else {
				log.Infof("Update: Data from Network Device and cache is equal, Do Nothing")
			}
		} else {
			// Delete subscription update
			// create the data again since it was in the cache -
			// add a flag to indicate even if the cache is equal reapply to the network
			log.Infof("Delete: to be recreated xpath - %s", *subDelta.Xpath)
			log.Infof("Delete: to be recreated data  - %v", x1)
			subDelta.ReApplyCacheData = BoolPtr(true)
		}
	} else {
		if *subDelta.SubAction == SubcriptionActionUpdate {
			log.Infof("OBJECT FINALLY NOT FOUND IN CACHE: INDEX %d EKV %v", i, *subDelta.Ekvl)
			// delete the object from the device since the data does not exist in the cache
			// delete only the objects that are not in the exception list
			// delete the object from the root of the tree - aggregate the paths since this is more scalable
			if d.checkExceptionXpath(*subDelta.Xpath) {
				// add in an exception
				log.Infof("Update: exception: + %s", *subDelta.Xpath)
			} else {
				log.Infof("Update: + %s", *subDelta.Xpath)
				subDelta.DataSubDeltaDelete = updateSubDeltaDelete(subDelta)
			}
		} else {
			// do nothing as the data was not supposed to be there
			// the data is not in the cache
			log.Debugf("Delete: do nothing as the config was not supposed to be here %s", *subDelta.Xpath)
		}
	}
	return subDelta, nil
}
*/

/*
func updateSubDeltaUpdate(subDelta *SubscriptionDelta, change *diff.Change) {
	for _, p := range change.Path {
		*subDelta.DataSubDeltaUpdate = append(*subDelta.DataSubDeltaUpdate, "/"+*subDelta.Xpath+"/"+p)
	}

}
*/

/*
func updateSubDeltaDelete(subDelta *SubscriptionDelta) *[]string {
	log.Infof("DataSubDeltaDelete 1 %v", *subDelta.DataSubDeltaDelete)
	log.Infof("DataSubDeltaDelete 1 xpath %s", *subDelta.Xpath)
	f := false
	for i, dp := range *subDelta.DataSubDeltaDelete {
		if strings.Contains(dp, *subDelta.Xpath) {
			// replace the entry in the list since this item aggregates the delete
			(*subDelta.DataSubDeltaDelete)[i] = *subDelta.Xpath
			f = true
		}
		if strings.Contains(*subDelta.Xpath, dp) {
			// dont append the entry since there is an aggregate path already in the list
			f = true
		}
	}
	if !f {
		// add the xpath to the list if it was not initialied so far
		*subDelta.DataSubDeltaDelete = append(*subDelta.DataSubDeltaDelete, *subDelta.Xpath)

	}
	log.Infof("DataSubDeltaDelete 2 %v", *subDelta.DataSubDeltaDelete)
	return subDelta.DataSubDeltaDelete
}
*/

/*
func getExceptionPaths() []string {
	return []string{"interface[name=mgmt0]", "network-instance[name=mgmt]", "system/gnmi-server"}
}
*/

func (d *DeviceDriver) checkExceptionXpath(xpath string) bool {
	// when the xpath is part of an exception path we can ignore it -> lpm match
	for _, ep := range *d.ExceptionPaths {
		if strings.Contains(xpath, ep) {
			return true
		}
	}
	// in case there is a match for the explicit exception path -> should be exact match
	for _, eep := range *d.ExplicitExceptionPaths {
		if xpath == eep {
			return true
		}
	}
	return false
}

/*
func findObject(x1 interface{}, ekvl []ElementKeyValue, i int) (int, interface{}, bool) {
	log.Debugf("FIND OBJECT INDEX: %d, EKV: %v, X1: %v", i, ekvl, x1)
	switch x1 := x1.(type) {
	case map[string]interface{}:
		if _, ok := x1[ekvl[i].Element]; ok {
			if i == len(ekvl)-1 {
				// e.g. interface[name=mgmt0] should continue since we still need to process the key/values
				if ekvl[i].KeyName != "" {
					log.Debugf("LAST OBJECT FOUND WITH KEY CONTINUE: %d, EKV: %v, X1: %v", i, ekvl, x1[ekvl[i].Element])
					return findObject(x1[ekvl[i].Element], ekvl, i)
				}
				log.Debugf("LAST OBJECT FOUND END: %d, EKV: %v, X1: %v", i, ekvl, x1[ekvl[i].Element])
				return i, x1[ekvl[i].Element], true
			} else {
				if ekvl[i].KeyName != "" {
					// e.g. interface[name=mgmt0] should continue since we still need to process the key/values,
					// As such we dont increment i
					log.Debugf("OBJECT FOUND WITH KEY CONTINUE: %d, EKV: %v, X1: %v", i, ekvl, x1[ekvl[i].Element])
					return findObject(x1[ekvl[i].Element], ekvl, i)
				}
				log.Debugf("OBJECT FOUND WITHOUT KEY CONTINUE: %d, EKV: %v, X1: %v", i, ekvl, x1[ekvl[i].Element])
				return findObject(x1[ekvl[i].Element], ekvl, i+1)
			}
		}
		// e.g. interface[name=lag1]/qos/output/scheduler -> qos object not found
		log.Debugf("OBJECT ELEMENT NOT FOUND: %d, EKV: %v, X1: %v", i, ekvl[i], x1)
		return i, nil, false

	case []interface{}:
		for n, v1 := range x1 {
			switch x3 := v1.(type) {
			case map[string]interface{}:
				for k3, v3 := range x3 {
					if k3 == ekvl[i].KeyName {
						switch v3.(type) {
						case string:
							log.Debugf("STRING: %s, %v", v3, ekvl[i].KeyValue)
							if v3 == ekvl[i].KeyValue {
								if i == len(ekvl)-1 {
									// last element in the ekv list
									log.Debugf("OBJECT FOUND IN LIST END: %d, EKV: %v, X1: %v", i, ekvl, x1[n])
									return i, x1[n], true
								} else {
									// not last element in the ekv list
									log.Debugf("OBJECT FOUND IN LIST CONTINUE: %d, EKV: %v, X1: %v", i, ekvl, x1[n])
									return findObject(x1[n], ekvl, i+1)
								}
							}
						case uint32:
							var v int
							switch ekvl[i].KeyValue.(type) {
							case string:
								v, _ = strconv.Atoi(fmt.Sprintf("%v", ekvl[i].KeyValue))
							}
							log.Debugf("UINT32: %d, %d", v3, v)
							if v3 == uint32(v) {
								if i == len(ekvl)-1 {
									// last element in the ekv list
									log.Debugf("OBJECT FOUND IN LIST END: %d, EKV: %v, X1: %v", i, ekvl, x1[n])
									return i, x1[n], true
								} else {
									// not last element in the ekv list
									log.Debugf("OBJECT FOUND IN LIST CONTINUE: %d, EKV: %v, X1: %v", i, ekvl, x1[n])
									return findObject(x1[n], ekvl, i+1)
								}
							}
						case float64:
							var v float64
							switch ekvl[i].KeyValue.(type) {
							case string:
								v, _ = strconv.ParseFloat(fmt.Sprintf("%v", ekvl[i].KeyValue), 8)
							}
							log.Debugf("FLOAT64: %v, %v", v3, v)
							if v3 == v {
								if i == len(ekvl)-1 {
									// last element in the ekv list
									log.Debugf("OBJECT FOUND IN LIST END: %d, EKV: %v, X1: %v", i, ekvl, x1[n])
									return i, x1[n], true
								} else {
									// not last element in the ekv list
									log.Debugf("OBJECT FOUND IN LIST CONTINUE: %d, EKV: %v, X1: %v", i, ekvl, x1[n])
									return findObject(x1[n], ekvl, i+1)
								}
							}
						default:
							log.Debugf("%v", reflect.TypeOf(v3))
						}
					}
				}
			}
		}
		//if i == len(ekvl)-1 {
		//e.g. interface[name=mgmt0] and mgmt0 key does not exist
		log.Debugf("OBJECT NOT FOUND IN LIST KEY NOT FOUND: %d, EKV: %v, X1: %v", i, ekvl, x1)
		return i, nil, false
		//}
	case nil:
		//log.Error("WE SHOULD NEVER COME HERE NIL")
		log.Debugf("OBJECT DOES NOT EXIST CREATE %d, EKV: %v, X1: %v", i, ekvl[i], x1)
		return i, nil, false
	default:
		return i, nil, false
	}
	//log.Errorf("OBJECT DOES NOT EXIST WE SHOULD NEVER COME HERE %d, EKV: %v, X1: %v", i, ekvl[i], x1)
	//return i, nil, false
}
*/
