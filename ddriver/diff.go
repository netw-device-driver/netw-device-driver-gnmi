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
	return ekvl, sb.String()

}

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

func SubscriptionActionPtr(s SubcriptionAction) *SubcriptionAction { return &s }

func (d *DeviceDriver) validateDiff(resp *gnmi.SubscribeResponse) {
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
			subDelta.SubAction = SubscriptionActionPtr(SubcriptionActionUpdate)
			subDelta.Ekvl = &ekvl
			subDelta.Xpath = &xpath
			subDelta.Value = &value

			if err != nil {
				log.WithError(err).Error("validateDiff get Value error")
			}

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

func (d *DeviceDriver) FindCacheObject(subDelta *SubscriptionDelta) (*SubscriptionDelta, error) {
	var x1 interface{}
	err := json.Unmarshal(d.Cache.CurrentConfig, &x1)
	if err != nil {
		return nil, err
	}
	log.Infof("Cache Config: %v", x1)
	log.Infof("EKVL : %v", *subDelta.Ekvl)
	i, x1, f := findObject(x1, *subDelta.Ekvl, 0)
	if f {
		if *subDelta.SubAction == SubcriptionActionUpdate {
			log.Debugf("OBJECT FINALLY FOUND: %v", x1)
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
			log.Infof("Update: Data from System : %v", x2)

			if !cmp.Equal(x1, x2) {
				// change = true
				changelog, _ := diff.Diff(x2, x1)
				log.Infof("Update: Data from system and cache is not equal, changelog : %v", changelog)
				for _, change := range changelog {
					if change.Type == diff.CREATE || change.Type == diff.UPDATE {
						subDelta.ReApplyCacheData = BoolPtr(true)
					} else {
						// delete
						updateSubDeltaUpdate(subDelta, &change)
					}
				}
			} else {
				log.Infof("Update: Data from system and cache is equal, Do Nothing")
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

func updateSubDeltaUpdate(subDelta *SubscriptionDelta, change *diff.Change) {
	for _, p := range change.Path {
		*subDelta.DataSubDeltaUpdate = append(*subDelta.DataSubDeltaUpdate, "/"+*subDelta.Xpath+"/"+p)
	}

}

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

/*
func getExceptionPaths() []string {
	return []string{"interface[name=mgmt0]", "network-instance[name=mgmt]", "system/gnmi-server"}
}
*/

func (d *DeviceDriver) checkExceptionXpath(xpath string) bool {
	for _, ep := range *d.ExceptionPaths {
		if strings.Contains(xpath, ep) {
			return true
		}
	}
	return false
}

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
		log.Error("WE SHOULD NEVER COME HERE NIL")
		log.Debugf("OBJECT DOES NOT EXIST CREATE %d, EKV: %v, X1: %v", i, ekvl[i], x1)
		return i, nil, false
	default:
		return i, nil, false
	}
	//log.Errorf("OBJECT DOES NOT EXIST WE SHOULD NEVER COME HERE %d, EKV: %v, X1: %v", i, ekvl[i], x1)
	//return i, nil, false
}
