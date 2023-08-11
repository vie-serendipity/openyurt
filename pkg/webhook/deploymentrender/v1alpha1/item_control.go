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
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"

	"github.com/openyurtio/openyurt/pkg/apis/apps/v1alpha1"
)

func replaceItems(deployment *v1.Deployment, items []v1alpha1.Item) error {
	for _, item := range items {
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

			for i := range deployment.Spec.Template.Spec.Containers {
				if deployment.Spec.Template.Spec.Containers[i].Name == item.Env.ContainerName {
					deployment.Spec.Template.Spec.Containers[i].Env = envVars
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
			for i := range deployment.Spec.Template.Spec.Containers {
				if deployment.Spec.Template.Spec.Containers[i].Name == item.Image.ContainerName {
					deployment.Spec.Template.Spec.Containers[i].Image = item.Image.ImageClaim
				}
			}
		case item.ConfigMap != nil:
			for i := range deployment.Spec.Template.Spec.Containers {
				for j := range deployment.Spec.Template.Spec.Containers[i].Env {
					if deployment.Spec.Template.Spec.Containers[i].Env[j].ValueFrom.ConfigMapKeyRef != nil {
						if deployment.Spec.Template.Spec.Containers[i].Env[j].ValueFrom.ConfigMapKeyRef.Name == item.ConfigMap.ConfigMapSource {
							deployment.Spec.Template.Spec.Containers[i].Env[j].ValueFrom.ConfigMapKeyRef.Name = item.ConfigMap.ConfigMapTarget
						}
					}
				}
			}
			for i := range deployment.Spec.Template.Spec.Volumes {
				if deployment.Spec.Template.Spec.Volumes[i].VolumeSource.ConfigMap != nil && deployment.Spec.Template.Spec.Volumes[i].VolumeSource.ConfigMap.Name == item.ConfigMap.ConfigMapSource {
					deployment.Spec.Template.Spec.Volumes[i].VolumeSource.ConfigMap.Name = item.ConfigMap.ConfigMapTarget
				}
			}
		case item.PersistentVolumeClaim != nil:
			for i := range deployment.Spec.Template.Spec.Volumes {
				if deployment.Spec.Template.Spec.Volumes[i].VolumeSource.PersistentVolumeClaim != nil &&
					deployment.Spec.Template.Spec.Volumes[i].VolumeSource.PersistentVolumeClaim.ClaimName == item.PersistentVolumeClaim.PVCSource {
					deployment.Spec.Template.Spec.Volumes[i].VolumeSource.PersistentVolumeClaim.ClaimName = item.PersistentVolumeClaim.PVCTarget
				}
			}
		case item.Secret != nil:
			for i := range deployment.Spec.Template.Spec.Volumes {
				if deployment.Spec.Template.Spec.Volumes[i].VolumeSource.Secret != nil && deployment.Spec.Template.Spec.Volumes[i].VolumeSource.Secret.SecretName == item.Secret.SecretSource {
					deployment.Spec.Template.Spec.Volumes[i].VolumeSource.Secret.SecretName = item.Secret.SecretTarget
				}
			}
		}
	}
	return nil
}
