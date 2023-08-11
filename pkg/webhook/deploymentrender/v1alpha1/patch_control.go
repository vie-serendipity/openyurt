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

package v1alpha1

import (
	"encoding/json"
	"fmt"
	"strings"

	jsonpatch "github.com/evanphx/json-patch"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/strategicpatch"

	"github.com/openyurtio/openyurt/pkg/apis/apps/v1alpha1"
	"github.com/openyurtio/openyurt/pkg/webhook/deploymentrender/utils"
)

type PatchControl struct {
	patches     []v1alpha1.Patch
	patchObject interface{}
	// data structure
	dataStruct interface{}
}

func (pc *PatchControl) strategicMergePatch(patch v1alpha1.Patch) error {
	patchMap := make(map[string]interface{})
	if err := json.Unmarshal(patch.Extensions.Raw, &patchMap); err != nil {
		return err
	}

	original, err := runtime.DefaultUnstructuredConverter.ToUnstructured(pc.patchObject)
	if err != nil {
		return err
	}

	patchedMap, err := strategicpatch.StrategicMergeMapPatch(original, patchMap, pc.dataStruct)
	if err != nil {
		return err
	}

	return runtime.DefaultUnstructuredConverter.FromUnstructured(patchedMap, pc.patchObject)
}

// implement json patch
func (pc *PatchControl) jsonMergePatch(patch v1alpha1.Patch) error {

	patchMap := make(map[string]interface{})
	if err := json.Unmarshal(patch.Extensions.Raw, &patchMap); err != nil {
		return err
	}
	// convert patch yaml into path and value
	flattenedData, err := utils.FlattenYAML(patchMap, "", "/")
	if err != nil {
		return err
	}
	// convert into json patch format
	var patchOperations []string
	for key, value := range flattenedData {
		single, err := json.Marshal(map[string]interface{}{
			"op":    string(patch.Type),
			"path":  key,
			"value": value,
		})
		if err != nil {
			return err
		}
		patchOperations = append(patchOperations, string(single))
	}
	patchBytes := []byte("[" + strings.Join(patchOperations, ",") + "]")
	patchedData, err := json.Marshal(pc.patchObject.(*appsv1.Deployment))
	if err != nil {
		return err
	}
	// conduct json patch
	patchObj, err := jsonpatch.DecodePatch(patchBytes)
	if err != nil {
		return err
	}
	patchedData, err = patchObj.Apply(patchedData)
	if err != nil {
		return err
	}
	return json.Unmarshal(patchedData, &pc.patchObject)
}

func (pc *PatchControl) updatePatches() error {
	for _, patch := range pc.patches {
		switch patch.Type {
		case v1alpha1.Default, "":
			if err := pc.strategicMergePatch(patch); err != nil {
				return err
			}
		case v1alpha1.ADD, v1alpha1.REMOVE, v1alpha1.REPLACE:
			if err := pc.jsonMergePatch(patch); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unknown patch type: %v", patch.Type)
		}
	}
	return nil
}
