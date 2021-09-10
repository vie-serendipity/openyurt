/*
Copyright 2021 The OpenYurt Authors.

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

package filter

const (
	// masterservice filter is used to mutate the ClusterIP and https port of default/kubernetes service
	// in order to pods on edge nodes can access kube-apiserver directly by inClusterConfig.
	MasterServiceFilterName = "masterservice"

	// servicetopology filter is used to reassemble endpointslice in order to make the service traffic
	// under the topology that defined by service.Annotation["openyurt.io/topologyKeys"]
	ServiceTopologyFilterName = "servicetopology"

	// discardcloudservice filter is used to discard cloud service(like loadBalancer service)
	// on kube-proxy list/watch service request from edge nodes.
	DiscardCloudServiceFilterName = "discardcloudservice"
)