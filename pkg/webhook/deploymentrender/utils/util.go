/*
Copyright 2023 The OpenYurt Authors.

Licensed under the Apache License, Version 2.0 (the License);
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an AS IS BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package utils

import (
	"reflect"
	"strings"
)

func FlattenYAML(data map[string]interface{}, parentKey string, sep string) (map[string]interface{}, error) {
	flattenedData := make(map[string]interface{})
	for key, value := range data {
		newKey := strings.Join([]string{parentKey, key}, sep)
		if reflect.ValueOf(value).Kind() == reflect.Map {
			flattenedMap, err := FlattenYAML(value.(map[string]interface{}), newKey, sep)
			if err != nil {
				return flattenedData, err
			}
			for k, v := range flattenedMap {
				flattenedData[k] = v
			}
		} else {
			flattenedData[newKey] = value
		}
	}
	return flattenedData, nil
}
