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

package jsonutils

import (
	"encoding/json"
	"strings"

	"github.com/netw-device-driver/ndd-runtime/pkg/utils"
)

func CleanConfig2String(cfg map[string]interface{}) (map[string]interface{}, *string, error) {
	// trim the first map
	for _, v := range cfg {
		switch v.(type) {
		case map[string]interface{}:
			cfg = cleanConfig(v.(map[string]interface{}))
		}
	}

	jsonConfigStr, err := json.Marshal(cfg)
	if err != nil {
		return nil, nil, err
	}
	return cfg, utils.StringPtr(string(jsonConfigStr)), nil
}

func cleanConfig(x1 map[string]interface{}) map[string]interface{} {
	x2 := make(map[string]interface{})
	for k1, v1 := range x1 {
		//log.Infof("cleanConfig Key: %s", k1)
		//log.Infof("cleanConfig Value: %v", v1)
		switch x3 := v1.(type) {
		case []interface{}:
			x := make([]interface{}, 0)
			for _, v3 := range x3 {
				switch x3 := v3.(type) {
				case map[string]interface{}:
					x4 := cleanConfig(x3)
					x = append(x, x4)
				default:
					x = append(x, v3)
				}
			}
			x2[strings.Split(k1, ":")[len(strings.Split(k1, ":"))-1]] = x
		case map[string]interface{}:
			x4 := cleanConfig(x3)
			x2[strings.Split(k1, ":")[len(strings.Split(k1, ":"))-1]] = x4
		default:
			x2[strings.Split(k1, ":")[len(strings.Split(k1, ":"))-1]] = v1
		}
	}
	return x2
}
