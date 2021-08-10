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
	"fmt"
	"strings"

	config "github.com/netw-device-driver/ndd-grpc/config/configpb"
	"github.com/netw-device-driver/ndd-runtime/pkg/utils"
	"github.com/pkg/errors"
)

func gnmiPathToXPath(path *config.Path) *string {
	sb := strings.Builder{}
	for i, pElem := range path.GetElem() {
		pes := strings.Split(pElem.GetName(), ":")
		var pe string
		if len(pes) > 1 {
			pe = pes[1]
		} else {
			pe = pes[0]
		}

		sb.WriteString(pe)
		for k, v := range pElem.GetKey() {
			sb.WriteString("[")
			sb.WriteString(k)
			sb.WriteString("=")
			sb.WriteString(v)
			sb.WriteString("]")
		}
		if i+1 != len(path.GetElem()) {
			sb.WriteString("/")
		}
		//log.Infof("pElemKeyValue: %s", sb.String())
	}
	return utils.StringPtr("/" + sb.String())
}

func startMerge(p1, p2 *config.Path, x1, x2 interface{}) (*config.Path, interface{}) {
	// we always start merging from root, so we just have to initialize the x1 if the data is empty
	if x1 == nil {
		// this is the start when you start from empty data
		x1 = make(map[string]interface{})
	}

	// transform data input when object is a list with a key from
	// map[string]interface{} to map[string][]interface{}
	// e.g. interfaces or subinterfaces or network-instances
	if len(p2.GetElem()) > 0 {
		if len(p2.GetElem()[0].GetKey()) != 0 {
			x3 := make(map[string]interface{})
			switch x2 := x2.(type) {
			case map[string]interface{}:
				for k2, v2 := range x2 {
					x4 := make([]interface{}, 0)
					x4 = append(x4, v2)
					x3[k2] = x4
				}
			default:
				// wrong data input
				fmt.Println(errors.New(fmt.Sprintf("data tarnsformation, wrong data input %v", x2)))
			}
			fmt.Printf("data transformation, new data: %v\n", x3)
			m := addObjectToTree(x1, x3, p2, 0)
			return p1, m
		}
	}
	// when the data is not a list with a key
	m := addObjectToTree(x1, x2, p2, 0)
	return p1, m
}

func addObjectToTree(x1, x2 interface{}, path *config.Path, idx int) interface{} {
	fmt.Printf("addObjectToTree entry, idx: %d, path length %d, path: %v\n data1: %v\n data2: %v\n", idx, len(path.GetElem()), path.GetElem(), x1, x2)
	switch x1 := x1.(type) {
	case map[string]interface{}:
		fmt.Printf("addObjectToTree map[string]interface{}, idx: %d, path length %d, path: %v\n data1: %v\n data2: %v\n", idx, len(path.GetElem()), path.GetElem(), x1, x2)
		x2, ok := x2.(map[string]interface{})
		if !ok {
			fmt.Println(errors.New("The object to be merged is not of type  map[string]interface{}"))
		}
		if _, ok := x1[path.GetElem()[idx].GetName()]; ok {
			// object exists, so we need to continue -> this is typically for lists
			if idx == len(path.GetElem())-1 {
				if len(path.GetElem()[idx].GetKey()) != 0 {
					// not last element of the list e.g. we are at interface of interface[name=ethernet-1/1]
					x1[path.GetElem()[idx].GetName()] = addObjectToTree(x1[path.GetElem()[idx].GetName()], x2[path.GetElem()[idx].GetName()], path, idx)
				} else {
					// system/ntp
					x1[path.GetElem()[idx].GetName()] = x2[path.GetElem()[idx].GetName()]
				}
			} else {
				if len(path.GetElem()[idx].GetKey()) != 0 {
					// not last element of the list e.g. we are at interface of interface[name=ethernet-1/1]/subinterface[index=100]
					x1[path.GetElem()[idx].GetName()] = addObjectToTree(x1[path.GetElem()[idx].GetName()], x2, path, idx)
				} else {
					// not last element of interface[name=ethernet-1/1]/protocol/bgp-vpn; we are at protocol level
					x1[path.GetElem()[idx].GetName()] = addObjectToTree(x1[path.GetElem()[idx].GetName()], x2, path, idx+1)
				}
			}
			return x1
		}
		// it is a new element so we return. E.g. network-instance get added to / or interfaces gets added to network-instance
		if len(path.GetElem()[idx].GetKey()) != 0 {
			// list -> interfaces or network-instances
			x1[path.GetElem()[idx].GetName()] = x2[path.GetElem()[idx].GetName()]
		}
		if idx == len(path.GetElem())-1 {
			x1[path.GetElem()[idx].GetName()] = x2[path.GetElem()[idx].GetName()]
			return x1
		} else {
			x1[path.GetElem()[idx].GetName()] = addObjectToTree(x1[path.GetElem()[idx].GetName()], x2, path, idx+1)
		}
	case []interface{}:
		fmt.Printf("addObjectToTree []interface{}, idx: %d, path length %d, path: %v\n data1: %v\n data2: %v\n", idx, len(path.GetElem()), path.GetElem(), x1, x2)
		for n, v1 := range x1 {
			switch x3 := v1.(type) {
			case map[string]interface{}:
				for k3, v3 := range x3 {
					if len(path.GetElem()[idx].GetKey()) != 0 {
						for k, v := range path.GetElem()[idx].GetKey() {
							fmt.Printf("addObjectToTree []interface{} k: %v, k3: %v, v: %v\n", k, k3, v3)
							if k3 == k {
								switch v3.(type) {
								case string, uint32:
									if v3 == v {
										if idx == len(path.GetElem())-1 {
											// last element in the pathElem list
											if x2, ok := x2.([]interface{}); !ok {
												return x1
											} else {
												x1[n] = x2[0]
												return x1
											}
										} else {
											// not last element in the ekv list
											x1[n] = addObjectToTree(x1[n], x2, path, idx+1)
										}
									}
								}
							}
						}
					}
				}
			}
		}
		if idx == len(path.GetElem())-1 {
			if x2, ok := x2.([]interface{}); !ok {
				return x1
			} else {
				x1 = append(x1, x2[0])
				return x1
			}
		}
	case nil:
		// we need to add all elements of the ekv in the tree if they dont exist
		// e.g. system/network-instance/protocols/evpn
		fmt.Printf("nil interface{}, path length %d, data1: %v, data2: %v\n", len(path.GetElem()), x1, x2)
		if idx == len(path.GetElem())-1 {
			x1, ok := x2.(map[string]interface{})
			if ok {
				return x1
			}
		} else {
			x1 := make(map[string]interface{})
			x1[path.GetElem()[idx].GetName()] = addObjectToTree(x1[path.GetElem()[idx].GetName()], x2, path, idx+1)
			return x1
		}
	default:
	}
	return x1
}
