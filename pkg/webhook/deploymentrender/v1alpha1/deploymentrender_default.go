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
	"context"
	"encoding/json"
	"fmt"
	"strings"

	jsonpatch "github.com/evanphx/json-patch"
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	"github.com/openyurtio/openyurt/pkg/apis/apps/v1alpha1"
	"github.com/openyurtio/openyurt/pkg/webhook/deploymentrender/utils"
)

var (
	resources = []string{"YurtAppSet", "YurtAppDaemon"}
)

func contain(kind string, resources []string) bool {
	for _, v := range resources {
		if kind == v {
			return true
		}
	}
	return false
}

// Default satisfies the defaulting webhook interface.
func (webhook *DeploymentRenderHandler) Default(ctx context.Context, obj runtime.Object) error {

	deployment, ok := obj.(*v1.Deployment)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a YurtAppConfigRender but got a %T", obj))
	}
	if deployment.OwnerReferences == nil {
		return nil
	}
	if !contain(deployment.OwnerReferences[0].Kind, resources) {
		return nil
	}

	klog.Info("start to validate deployment")
	// Get YurtAppSet/YurtAppDaemon resource of this deployment
	app := deployment.OwnerReferences[0]
	var instance client.Object
	switch app.Kind {
	case "YurtAppSet":
		instance = &v1alpha1.YurtAppSet{}
	case "YurtAppDaemon":
		instance = &v1alpha1.YurtAppDaemon{}
	default:
		return nil
	}
	if err := webhook.Client.Get(ctx, client.ObjectKey{
		Namespace: deployment.Namespace,
		Name:      app.Name,
	}, instance); err != nil {
		return err
	}

	// Get YurtAppConfigRender resource of app(1 to 1)
	var configRenderList v1alpha1.YurtAppConfigRenderList
	//listOptions := client.MatchingFields{"subject.Kind": app.Kind, "subject.Name": app.Name, "subject.APIVersion": app.APIVersion}
	if err := webhook.Client.List(ctx, &configRenderList, client.InNamespace(deployment.Namespace)); err != nil {
		klog.Info("error in listing YurtAppConfigRender")
		return err
	}
	if len(configRenderList.Items) == 0 {
		return nil
	}
	render := configRenderList.Items[0]

	// Get nodepool of deployment
	nodepool := deployment.Labels["apps.openyurt.io/pool-name"]

	klog.Info("start to render deployment")
	for _, entry := range render.Entries {
		pools := entry.Pools
		for _, pool := range pools {
			if pool == nodepool || pool == "*" {
				// Get the corresponding config  of this deployment
				// reference to the volumeSource implementation
				items := entry.Items
				// replace {{nodepool}} into real pool name
				for _, item := range items {
					if strings.Contains(item.ConfigMap.ConfigMapTarget, "{{nodepool}}") {
						item.ConfigMap.ConfigMapTarget = strings.ReplaceAll(item.ConfigMap.ConfigMapTarget, "{{nodepool}}", nodepool)
					}
					if strings.Contains(item.Secret.SecretTarget, "{{nodepool}}") {
						item.Secret.SecretTarget = strings.ReplaceAll(item.Secret.SecretTarget, "{{nodepool}}", nodepool)
					}
					if strings.Contains(item.PersistentVolumeClaim.PVCTarget, "{{nodepool}}") {
						item.PersistentVolumeClaim.PVCTarget = strings.ReplaceAll(item.PersistentVolumeClaim.PVCTarget, "{{nodepool}}", nodepool)
					}
				}
				// Replace items
				if err := replaceItems(deployment, items); err != nil {
					return err
				}
				// json patch and strategic merge
				patches := entry.Patches
				// Implement injection
				newDeployment := &v1.Deployment{}
				if err := updatePatches(deployment, patches, newDeployment); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func strategicMergePatch(oldObj interface{}, patch v1alpha1.Patch, newObj interface{}) error {
	patchMap := make(map[string]interface{})
	if err := json.Unmarshal(patch.Extensions.Raw, &patchMap); err != nil {
		return err
	}

	original, err := runtime.DefaultUnstructuredConverter.ToUnstructured(oldObj)
	if err != nil {
		return err
	}

	patchedMap, err := strategicpatch.StrategicMergeMapPatch(original, patchMap, newObj)
	if err != nil {
		return err
	}

	return runtime.DefaultUnstructuredConverter.FromUnstructured(patchedMap, newObj)
}

// implement json patch
func jsonMergePatch(oldObj interface{}, patch v1alpha1.Patch, newObj interface{}) error {

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
	patchOperations := make([]map[string]string, len(flattenedData))
	for key, value := range flattenedData {
		patchOperations = append(patchOperations, map[string]string{
			"Op":    string(patch.Type),
			"Path":  key,
			"Value": value.(string),
		})
	}
	patchBytes, err := json.Marshal(patchOperations)
	if err != nil {
		return err
	}
	patchedData, err := yaml.YAMLToJSON(oldObj.([]byte))
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
	return json.Unmarshal(patchedData, &newObj)
}

func updatePatches(oldObj interface{}, patches []v1alpha1.Patch, newObj interface{}) error {
	for _, patch := range patches {
		// go func
		// SlowStartBatch tools.go
		switch patch.Type {
		case v1alpha1.Default:
			if err := strategicMergePatch(oldObj, patch, newObj); err != nil {
				return err
			}
		case v1alpha1.ADD, v1alpha1.REMOVE, v1alpha1.REPLACE:

			if err := jsonMergePatch(oldObj, patch, newObj); err != nil {
				return nil
			}

		default:
			return fmt.Errorf("unknown patch type: %v", patch.Type)
		}
	}
	return nil
}

func replaceItems(deployment *v1.Deployment, items []v1alpha1.Item) error {
	for _, item := range items {
		// go func
		switch {
		case item.Replicas != nil:
			deployment.Spec.Replicas = item.Replicas
		case item.Env != nil:
			var envVars []corev1.EnvVar
			for key, val := range item.Env.EnvClaim {
				envVar := corev1.EnvVar{
					Name:  key,
					Value: val,
				}
				envVars = append(envVars, envVar)
			}

			for _, v := range deployment.Spec.Template.Spec.Containers {
				if v.Name == item.Env.ContainerName {
					v.Env = envVars
				}
			}
		case item.UpgradeStrategy != nil:
			if v1.DeploymentStrategyType(*item.UpgradeStrategy) == v1.RecreateDeploymentStrategyType {
				if deployment.Spec.Strategy.RollingUpdate != nil {
					deployment.Spec.Strategy.RollingUpdate = nil
				}
				deployment.Spec.Strategy.Type = v1.RecreateDeploymentStrategyType
			} else if v1.DeploymentStrategyType(*item.UpgradeStrategy) == v1.RollingUpdateDeploymentStrategyType {
				deployment.Spec.Strategy.Type = v1.RollingUpdateDeploymentStrategyType
			}
		case item.Image != nil:
			for _, v := range deployment.Spec.Template.Spec.Containers {
				if v.Name == item.Image.ContainerName {
					v.Image = item.Image.ImageClaim
				}
			}
		case item.ConfigMap != nil:
			for _, container := range deployment.Spec.Template.Spec.Containers {
				for _, env := range container.Env {
					if env.ValueFrom.ConfigMapKeyRef != nil {
						if env.ValueFrom.ConfigMapKeyRef.Name == item.ConfigMap.ConfigMapSource {
							env.ValueFrom.ConfigMapKeyRef.Name = item.ConfigMap.ConfigMapTarget
						}
					}
				}
			}
			for _, volume := range deployment.Spec.Template.Spec.Volumes {
				if volume.VolumeSource.ConfigMap != nil && volume.VolumeSource.ConfigMap.Name == item.ConfigMap.ConfigMapSource {
					volume.VolumeSource.ConfigMap.Name = item.ConfigMap.ConfigMapTarget
				}
			}
		case item.PersistentVolumeClaim != nil:
			for _, volume := range deployment.Spec.Template.Spec.Volumes {
				if volume.VolumeSource.PersistentVolumeClaim != nil &&
					volume.VolumeSource.PersistentVolumeClaim.ClaimName == item.PersistentVolumeClaim.PVCSource {
					volume.VolumeSource.PersistentVolumeClaim.ClaimName = item.PersistentVolumeClaim.PVCTarget
				}
			}
		case item.Secret != nil:
			for _, volume := range deployment.Spec.Template.Spec.Volumes {
				if volume.VolumeSource.Secret != nil && volume.VolumeSource.Secret.SecretName == item.Secret.SecretSource {
					volume.VolumeSource.Secret.SecretName = item.Secret.SecretTarget
				}
			}
		}
	}
	return nil
}
