package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type BackendData struct {
	VNI     int    `json:"VNI,omitempty"`
	VtepMAC string `json:"VtepMAC,omitempty"`
}

type Flannel struct {
	SubnetKubeManaged bool        `json:"subnetKubeManaged,omitempty"`
	BackendData       BackendData `json:"backendData,omitempty"`
	BackendType       string      `json:"backendType,omitempty"`
	PublicIP          string      `json:"publicIP,omitempty"`
	PublicIPOverwrite string      `json:"publicIPOverwrite,omitempty"`
	PodCidr           string      `json:"podCidr,omitempty"`
}

// NodeNetworkConfigurationSpec defines the desired state of NodeNetworkConfiguration
type NodeNetworkConfigurationSpec struct {
	Flannel Flannel `json:"flannel,omitempty"`
}

// NodeNetworkConfigurationStatus defines the observed state of NodeNetworkConfiguration
type NodeNetworkConfigurationStatus struct {
}

// +genclient
// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// NodeNetworkConfiguration is the Schema for the NodeNetworkConfigurations API
type NodeNetworkConfiguration struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NodeNetworkConfigurationSpec   `json:"spec,omitempty"`
	Status NodeNetworkConfigurationStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// NodeNetworkConfigurationList contains a list of NodeNetworkConfiguration
type NodeNetworkConfigurationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NodeNetworkConfiguration `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NodeNetworkConfiguration{}, &NodeNetworkConfigurationList{})
}
