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

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// ImageItem specifies the corresponding container and the claimed image
type ImageItem struct {
	ContainerName string `json:"containerName"`
	// ImageClaim represents the image name which is used by container above.
	ImageClaim string `json:"imageClaim"`
}

// EnvItem specifies the corresponding pool, containerName, and claimed env
type EnvItem struct {
	ContainerName string `json:"containerName"`
	// EnvClaim represents the detailed enviroment variables that container contains.
	EnvClaim map[string]string `json:"envClaim"`
}

// PersistentVolumeClaimItem
type PersistentVolumeClaimItem struct {
	ContainerName string `json:"containerName"`
	// PVCSource represents volume name.
	PVCSource string `json:"pvcSource"`
	// PVCTarget represents the PVC corresponding to the volume above.
	PVCTarget string `json:"pvcTarget"`
}

type ConfigMapItem struct {
	// ContainerName represents name of the container.
	ContainerName string `json:"containerName"`
	// ConfigMapSource represents volume name.
	ConfigMapSource string `json:"configMapClaim"`
	// ConfigMapTarget represents the ConfigMap corresponding to the volume above.
	ConfigMapTarget string `json:"configMapTarget"`
}

type SecretItem struct {
	// ContainerName represents name of the container.
	ContainerName string `json:"containerName"`
	// SecretSource represents volume name.
	SecretSource string `json:"secretClaim"`
	// SecretTarget represents the Secret corresponding to the volume above.
	SecretTarget string `json:"secretTarget"`
}

type Replacement struct {
	Pools []string `json:"pools"`
	Items []Item   `json:"items"`
}

// Item represents configuration to be injected.
// Only one of its members may be specified.
type Item struct {
	Image                 *ImageItem                 `json:"image"`
	ConfigMap             *ConfigMapItem             `json:"configMap"`
	Secret                *SecretItem                `json:"secret"`
	Env                   *EnvItem                   `json:"env"`
	PersistentVolumeClaim *PersistentVolumeClaimItem `json:"persistentVolumeClaim"`
	Replicas              *int                       `json:"replicas"`
	UpgradeStrategy       *string                    `json:"upgradeStrategy"`
}

type Subject struct {
	metav1.TypeMeta `json:",inline"`
	// Name is the name of YurtAppSet or YurtAppDaemon
	NameSpace string `json:"nameSpace"`
	Name      string `json:"name"`
}

type YurtAppConfigurationReplacement struct {
	metav1.TypeMeta `json:",inline"`

	// Standard object's metadata
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Describe the object to which this replacement belongs
	Subject Subject `json:"subject"`

	// Describe detailed multi-region configuration of the subject above
	Replacements []Replacement `json:"replacements"`
}
