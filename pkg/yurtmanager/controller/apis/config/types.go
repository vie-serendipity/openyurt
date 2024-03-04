/*
Copyright 2020 The OpenYurt Authors.

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

package config

import (
	prvd "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/util/cloudprovider"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	componentbaseconfig "k8s.io/component-base/config"
	"k8s.io/kube-controller-manager/config/v1alpha1"

	cloudnodepoollifecycleconfig "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/cloudnodepoollifecycle/config"
	cloudnodeslabels "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/cloudnodeslabels/config"
	nodebucketconfig "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/nodebucket/config"
	nodepoolconfig "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/nodepool/config"
	platformadminconfig "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/platformadmin/config"
	poolserviceconfig "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/poolservice/config"
	gatewaylifecycleconfig "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/raven/gatewaylifecycle/config"
	gatewaypickupconfig "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/raven/gatewaypickup/config"
	ravencloudproviderconfig "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/raven/util/config"
	uniteddeploymentconfig "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/uniteddeployment/config"
	yurtappsetconfig "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/yurtappset/config"
	yurtstaticsetconfig "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/yurtstaticset/config"
)

// YurtManagerConfiguration contains elements describing yurt-manager.
type YurtManagerConfiguration struct {
	metav1.TypeMeta
	Generic GenericConfiguration
	// NodePoolControllerConfiguration holds configuration for NodePoolController related features.
	NodePoolController nodepoolconfig.NodePoolControllerConfiguration

	// UnitedDeploymentControllerConfiguration holds configuration for UnitDeploymentController related features.
	UnitDeploymentController uniteddeploymentconfig.UnitedDeploymentControllerConfiguration

	// GatewayPickupControllerConfiguration holds configuration for GatewayController related features.
	GatewayPickupController gatewaypickupconfig.GatewayPickupControllerConfiguration

	// GatewayLifecycleControllerConfiguration holds configuration for GatewayController related features.
	GatewayLifecycleController gatewaylifecycleconfig.GatewayLifecycleControllerConfiguration

	// YurtAppSetControllerConfiguration holds configuration for YurtAppSetController related features.
	YurtAppSetController yurtappsetconfig.YurtAppSetControllerConfiguration

	// YurtStaticSetControllerConfiguration holds configuration for YurtStaticSetController related features.
	YurtStaticSetController yurtstaticsetconfig.YurtStaticSetControllerConfiguration

	// PlatformAdminControllerConfiguration holds configuration for PlatformAdminController related features.
	PlatformAdminController platformadminconfig.PlatformAdminControllerConfiguration

	//RavenCloudProviderController holds configuration for RavenCloudProviderController related features
	RavenCloudProviderController ravencloudproviderconfig.RavenCloudProviderConfiguration

	NodeLifeCycleController v1alpha1.NodeLifecycleControllerConfiguration

	// CloudNodepoolLifeCycleController holds configuration for CloudNodepoolLifeCycleController related features.
	CloudNodepoolLifeCycleController cloudnodepoollifecycleconfig.CloudNodepoolLifeCycleControllerConfiguration

	//  NodeBucketController holds configuration for NodeBucketController related features.
	NodeBucketController nodebucketconfig.NodeBucketControllerConfiguration

	// CloudNodepoolLifeCycleController holds configuration for CloudNodepoolLifeCycleController related features.
	CloudNodesLabelsController cloudnodeslabels.CloudNodesLabelsControllerConfiguration
	//  PoolServiceController holds configuration for PoolServiceController related features.
	PoolServiceController poolserviceconfig.PoolServiceControllerConfiguration
}

type GenericConfiguration struct {
	Version          bool
	MetricsAddr      string
	HealthProbeAddr  string
	WebhookPort      int
	LeaderElection   componentbaseconfig.LeaderElectionConfiguration
	RestConfigQPS    int
	RestConfigBurst  int
	WorkingNamespace string
	Kubeconfig       string
	Cloudconfig      string
	CloudProvider    prvd.Provider
	// Controllers is the list of controllers to enable or disable
	// '*' means "all enabled by default controllers"
	// 'foo' means "enable 'foo'"
	// '-foo' means "disable 'foo'"
	// first item for a particular name wins
	Controllers []string
	// DisabledWebhooks is used to specify the disabled webhooks
	// Only care about controller-independent webhooks
	DisabledWebhooks []string
}
