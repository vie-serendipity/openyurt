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

// import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// // ImageReplacement specifies the corresponding container and the claimed image.
// type ImageReplacement struct {
// 	ContainerName string `json:"containerName"`
// 	// ImageClaim represents the image name which is used by container above.
// 	ImageClaim string `json:"imageClaim"`
// }

// // EnvReplacement specifies the corresponding container and
// type EnvReplacement struct {
// 	ContainerName string `json:"containerName"`
// 	// EnvClaim represents the detailed enviroment variables that container contains.
// 	EnvClaim map[string]string `json:"envClaim"`
// }

// type PersistentVolumeClaimReplacement struct {
// 	ContainerName string `json:"containerName"`
// 	// PVCSource represents volume name.
// 	PVCSource string `json:"pvcSource"`
// 	// PVCTarget represents the PVC corresponding to the volume above.
// 	PVCTarget string `json:"pvcTarget"`
// }

// type ConfigMapReplacement struct {
// 	// ContainerName represents name of the container.
// 	ContainerName string `json:"containerName"`
// 	// ConfigMapSource represents volume name.
// 	ConfigMapSource string `json:"configMapClaim"`
// 	// ConfigMapTarget represents the ConfigMap corresponding to the volume above.
// 	ConfigMapTarget string `json:"configMapTarget"`
// }

// type SecretReplacement struct {
// 	// ContainerName represents name of the container.
// 	ContainerName string `json:"containerName"`
// 	// SecretSource represents volume name.
// 	SecretSource string `json:"secretClaim"`
// 	// SecretTarget represents the Secret corresponding to the volume above.
// 	SecretTarget string `json:"secretTarget"`
// }

// // Replacement represents configuration to be injected.
// // Only one of its members may be specified.
// type Replacement struct {
// 	Image                 *ImageReplacement                 `json:"image"`
// 	ConfigMap             *ConfigMapReplacement             `json:"configMap"`
// 	Secret                *SecretReplacement                `json:"secret"`
// 	Env                   *EnvReplacement                   `json:"env"`
// 	PersistentVolumeClaim *PersistentVolumeClaimReplacement `json:"persistentVolumeClaim"`
// 	Replicas              *int                              `json:"replicas"`
// 	UpgradeStrategy       *string                           `json:"upgradeStrategy"`
// }

// type Subject struct {
// 	metav1.TypeMeta `json:",inline"`
// 	// Name is the name of YurtAppSet or YurtAppDaemon
// 	Name string `json:"name"`
// 	// Pools represent names of nodepool that replacements will be injected into.
// 	Pools []string `json:"pools"`
// }

// type YurtAppConfigBinding struct {
// 	metav1.TypeMeta `json:",inline"`

// 	// Standard object's metadata
// 	metav1.ObjectMeta `json:"metadata,omitempty"`

// 	// Describe the object to which this binding belongs
// 	Subject Subject `json:"subject"`
// 	// Describe detailed configuration to be injected of the subject above.
// 	Replacements []Replacement `json:"replacements"`
// }
