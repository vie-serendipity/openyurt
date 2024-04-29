/*
Copyright 2023 The OpenYurt Authors.

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

package names

const (
	CsrApproverController                  = "csr-approver-controller"
	DaemonPodUpdaterController             = "daemon-pod-updater-controller"
	NodePoolController                     = "nodepool-controller"
	PlatformAdminController                = "platform-admin-controller"
	ServiceTopologyEndpointsController     = "service-topology-endpoints-controller"
	ServiceTopologyEndpointSliceController = "service-topology-endpointslice-controller"
	YurtAppSetController                   = "yurt-app-set-controller"
	YurtStaticSetController                = "yurt-static-set-controller"
	YurtCoordinatorCertController          = "yurt-coordinator-cert-controller"
	DelegateLeaseController                = "delegate-lease-controller"
	PodBindingController                   = "pod-binding-controller"
	GatewayPickupController                = "gateway-pickup-controller"
	GatewayLifeCycleController             = "gateway-life-cycle-controller"
	GatewayInternalServiceController       = "gateway-internal-service-controller"
	GatewayPublicServiceController         = "gateway-public-service-controller"
	GatewayDNSController                   = "gateway-dns-controller"
	RavenCloudProviderController           = "raven-cloud-provider-controller"
	RavenACLEntryController                = "raven-acl-entry-controller"
	NodeLifeCycleController                = "node-life-cycle-controller"
	UnitedDeploymentController             = "uniteddeployment-controller"
	CloudNodepoolLifecycleController       = "cloud-nodepool-lifecycle-controller"
	NodeBucketController                   = "node-bucket-controller"
	EnsLoadBalancerController              = "ens-loadbalancer-controller"
	CloudNodesLabelsController             = "cloud-nodes-labels-controller"
	LoadBalancerSetController              = "load-balancer-set-controller"
	EdgeRouteController                    = "edge-route-controller"
)

func YurtManagerControllerAliases() map[string]string {
	// return a new reference to achieve immutability of the mapping
	return map[string]string{
		"csrapprover":                   CsrApproverController,
		"daemonpodupdater":              DaemonPodUpdaterController,
		"nodepool":                      NodePoolController,
		"platformadmin":                 PlatformAdminController,
		"servicetopologyendpoints":      ServiceTopologyEndpointsController,
		"servicetopologyendpointslices": ServiceTopologyEndpointSliceController,
		"yurtappset":                    YurtAppSetController,
		"yurtstaticset":                 YurtStaticSetController,
		"yurtcoordinatorcert":           YurtCoordinatorCertController,
		"delegatelease":                 DelegateLeaseController,
		"podbinding":                    PodBindingController,
		"gatewaypickup":                 GatewayPickupController,
		"gatewayinternalservice":        GatewayInternalServiceController,
		"gatewaypublicservice":          GatewayPublicServiceController,
		"gatewaydns":                    GatewayDNSController,
		"gatewaylifecycle":              GatewayLifeCycleController,
		"nodelifecycle":                 NodeLifeCycleController,
		"cloudprovider":                 RavenCloudProviderController,
		"aclentry":                      RavenACLEntryController,
		"uniteddeployment":              UnitedDeploymentController,
		"cloudnodepoollifecycle":        CloudNodepoolLifecycleController,
		"nodebucket":                    NodeBucketController,
		"ensloadbalancer":               EnsLoadBalancerController,
		"cloudnodeslabels":              CloudNodesLabelsController,
		"loadbalancerset":               LoadBalancerSetController,
		"edgeroute":                     EdgeRouteController,
	}
}
