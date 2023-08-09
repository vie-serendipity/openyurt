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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ImageItem specifies the corresponding container and the claimed image
type ImageItem struct {
	// ContainerName represents name of the container
	// in which the Image will be replaced
	ContainerName string `json:"containerName"`
	// ImageClaim represents the claimed image name
	//which is injected into the container above
	ImageClaim string `json:"imageClaim"`
}

// EnvItem specifies the corresponding container and the claimed env
type EnvItem struct {
	// ContainerName represents name of the container
	// in which the env will be replaced
	ContainerName string `json:"containerName"`
	// EnvClaim represents the detailed environment variables container contains
	EnvClaim map[string]string `json:"envClaim"`
}

// PersistentVolumeClaimItem specifies the corresponding container and the claimed pvc
type PersistentVolumeClaimItem struct {
	// PVCSource represents pvcClaim name.
	PVCSource string `json:"pvcSource"`
	// PVCTarget represents the PVC corresponding to the volume above.
	// PVCTarget supprot advanced features like wildcard.
	// By naming pvc as pvcName-{{nodepool}}, all pvc can be injected at once.
	PVCTarget string `json:"pvcTarget"`
}

// ConfigMapItem specifies the corresponding containerName and the claimed configMap
type ConfigMapItem struct {
	// ConfigMapSource represents configMap name
	ConfigMapSource string `json:"configMapSource"`
	// ConfigMapTarget represents the ConfigMap corresponding to the volume above.
	// ConfigMapTarget supprot advanced features like wildcard.
	// By naming configMap as configMapName-{{nodepool}}, all configMap can be injected at once.
	ConfigMapTarget string `json:"configMapTarget"`
}

type SecretItem struct {
	// SecretSource represents secret name.
	SecretSource string `json:"secretSource"`
	// SecretTarget represents the Secret corresponding to the volume above.
	// SecretTarget supprot advanced features like wildcard.
	// By naming secret as secretName-{{nodepool}}, all secret can be injected at once.
	SecretTarget string `json:"secretTarget"`
}

// Item represents configuration to be injected.
// Only one of its members may be specified.
type Item struct {
	// +optional
	Image *ImageItem `json:"image,omitempty"`
	// +optional
	ConfigMap *ConfigMapItem `json:"configMap,omitempty"`
	// +optional
	Secret *SecretItem `json:"secret,omitempty"`
	// +optional
	Env *EnvItem `json:"env,omitempty"`
	// +optional
	PersistentVolumeClaim *PersistentVolumeClaimItem `json:"persistentVolumeClaim,omitempty"`
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`
	// +optional
	UpgradeStrategy *string `json:"upgradeStrategy,omitempty"`
}

type Operation string

const (
	Default Operation = "default" // strategic merge patch
	ADD     Operation = "add"     // json patch
	REMOVE  Operation = "remove"  // json patch
	REPLACE Operation = "replace" // json patch
)

type Patch struct {
	// type represents the operation
	// default is strategic merge patch
	// +optional
	Type Operation `json:"type"`
	// Indicates the patch for the template
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	Extensions *runtime.RawExtension `json:"extensions"`
}

// Describe detailed multi-region configuration of the subject
// Entry describe a set of nodepools and their shared or identical configurations
type Entry struct {
	Pools []string `json:"pools"`
	// +optional
	Items []Item `json:"items,omitempty"`
	// Convert Patch struct into json patch operation
	// +optional
	Patches []Patch `json:"patches,omitempty"`
}

// Describe the object Entries belongs
type Subject struct {
	metav1.TypeMeta `json:",inline"`
	// Name is the name of YurtAppSet or YurtAppDaemon
	Name string `json:"name"`
}

// YurtAppConfigRenderStatus defines the observed state of YurtAppConfigRender.
type YurtAppConfigRenderStatus struct {
	// ObservedGeneration is the most recent generation observed for this YurtAppConfigRender. It corresponds to the
	// YurtAppConfigRender's generation, which is updated on mutation by the API Server.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Count of hash collisions for the YurtAppConfigRender. The YurtAppConfigRender controller
	// uses this field as a collision avoidance mechanism when it needs to
	// create the name for the newest ControllerRevision.
	// +optional
	CollisionCount *int32 `json:"collisionCount,omitempty"`

	// CurrentRevision, if not empty, indicates the current version of the YurtAppConfigRender.
	CurrentRevision string `json:"currentRevision"`
}

type YurtAppConfigRenderSpec struct {
	Subject Subject `json:"subject"`
	Entries []Entry `json:"entries"`
	// Indicates the number of histories to be conserved.
	// If unspecified, defaults to 10.
	// +optional
	RevisionHistoryLimit *int32 `json:"revisionHistoryLimit,omitempty"`
}

// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=yacr
// +kubebuilder:printcolumn:name="Subject",type="string",JSONPath=".subject.kind",description="The subject kind of this configrender."
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp",description="CreationTimestamp is a timestamp representing the server time when this object was created. It is not guaranteed to be set in happens-before order across separate operations. Clients may not set this value. It is represented in RFC3339 form and is in UTC."

type YurtAppConfigRender struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              YurtAppConfigRenderSpec   `json:"spec"`
	Status            YurtAppConfigRenderStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// YurtAppConfigRenderList contains a list of YurtAppConfigRender
type YurtAppConfigRenderList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []YurtAppConfigRender `json:"items"`
}

func init() {
	SchemeBuilder.Register(&YurtAppConfigRender{}, &YurtAppConfigRenderList{})
}
