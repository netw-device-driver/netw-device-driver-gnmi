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
	"strings"

	log "github.com/sirupsen/logrus"
)

func contains(s []int, e int) bool {
	for _, a := range s {
		if a == e {
			return true
		}
	}
	return false
}

func stringPtr(s string) *string { return &s }

/*
type reversedMap struct {
	Index   int
	Element map[string]*Data
}

func reverseOrder(m map[int]map[string]*Data) map[int]map[string]*Data {
	var keys []int
	for k := range m {
		keys = append(keys, k)
	}

	sort.Sort(sort.Reverse(sort.IntSlice(keys)))

	var onerM reversedMap
	var rM []reversedMap

	for _, key := range keys {
		onerM.Index = key
		onerM.Element = m[key]
		rM = append(rM, onerM)
	}

	reversed_m := make(map[int]map[string]*Data) // empty map

	// put the reverse ordered elements into the new map

	for _, record := range rM {
		reversed_m[record.Index] = record.Element
	}
	return reversed_m
}
*/

func startMerge(ap1 string, j1 interface{}, ip2, ap2 string, data2 []byte) (string, interface{}, error) {
	log.Debugf("Start Merge: Path1: %s, Path2: %s", ap1, ap2)
	var j2, x1, x2 interface{}
	var newAggrPath, newIndivPath string

	err := json.Unmarshal(data2, &j2)
	if err != nil {
		return "", nil, err
	}

	contains := false
	if len(ap1) == 0 {
		// this is typically the start when you start from nothing
		newAggrPath = ap2
		newIndivPath = ip2
		log.Debugf("merged Data Zero: new aggregate pathPath: %s", newAggrPath)
		return newAggrPath, j2, nil
	}
	if len(ap2) >= len(ap1) {
		// this is the main branch we should enter given that the merge is happening per level and hence
		// the path should be received in order
		x1 = j1
		x2 = j2
		if strings.Contains(ap2, ap1) {
			contains = true
		}
		newAggrPath = ap1
		newIndivPath = ip2
		log.Debugf("merged Data P1 >= P2: new aggregate pathPath: %s", newAggrPath)
	} else {
		log.Error("We should never come here since we order the data per level")
		x1 = j2
		x2 = j1
		if strings.Contains(ap1, ap2) {
			contains = true
		}
		newAggrPath = ap2
		newIndivPath = ip2
		log.Debugf("merged Data P2 > P1: new aggregate pathPath: %s", newAggrPath)
	}
	if contains {
		// this is normally the case since we start from the root hierarchy in all cases
		// otherwise we have missing depenedencies
		var m interface{}

		log.Debugf("mergePath: %s", newAggrPath)
		log.Debugf("merge individual path: %s", newIndivPath)
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
	log.Debugf("Split: %v", s1)
	for i, v := range s1 {
		if i > 0 && !skipElement {
			log.Debugf("Element: %s", v)
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
	log.Debugf("START ADDING OBJECT TO THE TREE Index:%d, EKV: %v", i, ekvl)
	log.Debugf("START ADDING OBJECT TO THE TREE X1: %v", x1)
	log.Debugf("START ADDING OBJECT TO THE TREE X2: %v", x2)
	/*
		for _, ekv := range ekvl {
			x1 = addObject(x1, x2, ekv)
		}*/
	x1 = addObject(x1, x2, ekvl, 0)
	log.Debugf("FINISHED ADDING OBJECT TO THE TREE X1: %v", x1)
	return x1, nil
}

func addObject(x1, x2 interface{}, ekv []ElementKeyValue, i int) interface{} {
	log.Debugf("ADD1 OBJECT EKV: %v", ekv)
	log.Debugf("ADD1 OBJECT EKV INDEX: %v", i)
	log.Debugf("ADD1 OBJECT X1: %v", x1)
	log.Debugf("ADD1 OBJECT X2: %v", x2)
	switch x1 := x1.(type) {
	case map[string]interface{}:
		x2, ok := x2.(map[string]interface{})
		if !ok {
			log.Debugf("NOK SOMETHING WENT WRONG map[string]interface{}")
			return x1
		}
		if _, ok := x1[ekv[i].Element]; ok {
			// object exists, so we need to continue -> this is typically for lists
			log.Debugf("Check NEXT ELEMENT")
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
		log.Debugf("Added NEW ELEMENT BEFORE X1: %v", x1)
		log.Debugf("Added NEW ELEMENT BEFORE X2: %v", x2)
		log.Debugf("Added NEW ELEMENT BEFORE EKV element: %v", ekv)
		if ekv[i].KeyName != "" {
			// list -> interfaces or network-instances
			x1[ekv[i].Element] = x2[ekv[i].Element]
		} else {
			// add e.g. system of (system, ntp)
			x1[ekv[i].Element] = nil
		}
		log.Debugf("Added NEW ELEMENT BEFORE X1[]: %v", x1[ekv[i].Element])
		log.Debugf("Added NEW ELEMENT BEFORE X2[]: %v", x2[ekv[i].Element])
		log.Debugf("Added NEW ELEMENT to X1: %v", x1)
		if i == len(ekv)-1 {
			log.Debugf("ADDING ELEMENTS FINISHED")
			return x1
		} else {
			log.Debugf("CONTINUE ADDING ELEMENTS")
			log.Debugf("CONTINUE ADDING ELEMENTS X1: %v", x1)
			log.Debugf("CONTINUE ADDING ELEMENTS X2: %v", x2)
			log.Debugf("CONTINUE ADDING ELEMENTS EKV element: %v", ekv)
			log.Debugf("CONTINUE ADDING ELEMENTS X1[]: %v", x1[ekv[i].Element])
			log.Debugf("CONTINUE ADDING ELEMENTS X2: %v", x2)

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
									log.Debugf("OBJECT FOUND In LIST OVERWRITE WITH NEW OBJECT")
									x1[n] = x2
									return x1
								} else {
									// not last element in the ekv list
									log.Debug("OBJECT FOUND IN LIST CONTINUE")
									log.Debugf("OBJECT FOUND IN LIST CONTINUE: X1[n]: %v", x1[n])
									log.Debugf("OBJECT FOUND IN LIST CONTINUE: X2: %v", x2)
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
			log.Debug("OBJECT NOT FOUND In LIST APPEND NEW OBJECT")
			log.Debugf("APPEND BEFORE X1: %v", x1)
			log.Debugf("APPEND BEFORE X2[0]: %v", x2[0])
			x1 = append(x1, x2[0])
			log.Debugf("APPEND AFTER X1: %v", x1)
			return x1
		}
	case nil:
		log.Debug("OBJECT DOES NOT EXIST CREATE")
		x1, ok := x2.(map[string]interface{})
		log.Debugf("OBJECT DOES NOT EXIST CREATE X1: %v", x1)
		log.Debugf("OBJECT DOES NOT EXIST CREATE X2: %v", x2)
		if ok {
			return x1
		}
	}

	return x1
}
