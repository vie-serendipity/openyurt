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

package options

import (
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	cliflag "k8s.io/component-base/cli/flag"

	"github.com/openyurtio/openyurt/cmd/yurt-manager/app/config"
)

// YurtManagerOptions is the main context object for the yurt-manager.
type YurtManagerOptions struct {
	Generic                      *GenericOptions
	NodePoolController           *NodePoolControllerOptions
	GatewayPickupController      *GatewayPickupControllerOptions
	GatewayLifecycleController   *GatewayLifecycleControllerOptions
	YurtStaticSetController      *YurtStaticSetControllerOptions
	YurtAppSetController         *YurtAppSetControllerOptions
	PlatformAdminController      *PlatformAdminControllerOptions
	NodeLifeCycleController      *NodeLifecycleControllerOptions
	UnitedDeploymentController   *UnitedDeploymentControllerOptions
	RavenCloudProviderController *CloudProviderControllerOptions
	NodeBucketController         *NodeBucketControllerOptions
	LoadBalancerSetController    *LoadBalancerSetControllerOptions
	EdgeRouteController          *RouteControllerOptions
}

// NewYurtManagerOptions creates a new YurtManagerOptions with a default config.
func NewYurtManagerOptions() (*YurtManagerOptions, error) {

	s := YurtManagerOptions{
		Generic:                      NewGenericOptions(),
		NodePoolController:           NewNodePoolControllerOptions(),
		GatewayPickupController:      NewGatewayPickupControllerOptions(),
		GatewayLifecycleController:   NewGatewayLifecycleControllerOptions(),
		YurtStaticSetController:      NewYurtStaticSetControllerOptions(),
		YurtAppSetController:         NewYurtAppSetControllerOptions(),
		PlatformAdminController:      NewPlatformAdminControllerOptions(),
		NodeLifeCycleController:      NewNodeLifecycleControllerOptions(),
		UnitedDeploymentController:   NewUnitedDeploymentControllerOptions(),
		RavenCloudProviderController: NewCloudProviderControllerOptions(),
		NodeBucketController:         NewNodeBucketControllerOptions(),
		LoadBalancerSetController:    NewLoadBalancerSetControllerOptions(),
		EdgeRouteController:          NewRouteControllerOptions(),
	}

	return &s, nil
}

func (y *YurtManagerOptions) Flags(allControllers, disabledByDefaultControllers []string) cliflag.NamedFlagSets {
	fss := cliflag.NamedFlagSets{}
	y.Generic.AddFlags(fss.FlagSet("generic"), allControllers, disabledByDefaultControllers)
	y.NodePoolController.AddFlags(fss.FlagSet("nodepool controller"))
	y.GatewayPickupController.AddFlags(fss.FlagSet("gateway controller"))
	y.GatewayLifecycleController.AddFlags(fss.FlagSet("gatewaylifecycle controller"))
	y.YurtStaticSetController.AddFlags(fss.FlagSet("yurtstaticset controller"))
	y.PlatformAdminController.AddFlags(fss.FlagSet("iot controller"))
	y.NodeLifeCycleController.AddFlags(fss.FlagSet("nodelifecycle controller"))
	y.RavenCloudProviderController.AddFlags(fss.FlagSet("cloudresource controller"))
	y.NodeBucketController.AddFlags(fss.FlagSet("nodebucket controller"))
	y.LoadBalancerSetController.AddFlags(fss.FlagSet("loadbalancerset controller"))
	y.EdgeRouteController.AddFlags(fss.FlagSet("edgeroute controller"))
	return fss
}

// Validate is used to validate the options and config before launching the yurt-manager
func (y *YurtManagerOptions) Validate(allControllers []string, controllerAliases map[string]string) error {
	var errs []error
	errs = append(errs, y.Generic.Validate(allControllers, controllerAliases)...)
	errs = append(errs, y.NodePoolController.Validate()...)
	errs = append(errs, y.GatewayPickupController.Validate()...)
	errs = append(errs, y.GatewayLifecycleController.Validate()...)
	errs = append(errs, y.YurtStaticSetController.Validate()...)
	errs = append(errs, y.PlatformAdminController.Validate()...)
	errs = append(errs, y.NodeLifeCycleController.Validate()...)
	errs = append(errs, y.RavenCloudProviderController.Validate()...)
	errs = append(errs, y.NodeBucketController.Validate()...)
	errs = append(errs, y.LoadBalancerSetController.Validate()...)
	errs = append(errs, y.EdgeRouteController.Validate()...)
	return utilerrors.NewAggregate(errs)
}

// ApplyTo fills up yurt manager config with options.
func (y *YurtManagerOptions) ApplyTo(c *config.Config, controllerAliases map[string]string) error {
	if err := y.Generic.ApplyTo(&c.ComponentConfig.Generic, controllerAliases); err != nil {
		return err
	}
	if err := y.NodePoolController.ApplyTo(&c.ComponentConfig.NodePoolController); err != nil {
		return err
	}
	if err := y.YurtStaticSetController.ApplyTo(&c.ComponentConfig.YurtStaticSetController); err != nil {
		return err
	}
	if err := y.PlatformAdminController.ApplyTo(&c.ComponentConfig.PlatformAdminController); err != nil {
		return err
	}
	if err := y.GatewayPickupController.ApplyTo(&c.ComponentConfig.GatewayPickupController); err != nil {
		return err
	}
	if err := y.NodeLifeCycleController.ApplyTo(&c.ComponentConfig.NodeLifeCycleController); err != nil {
		return err
	}
	if err := y.GatewayLifecycleController.ApplyTo(&c.ComponentConfig.GatewayLifecycleController); err != nil {
		return err
	}
	if err := y.RavenCloudProviderController.ApplyTo(&c.ComponentConfig.RavenCloudProviderController); err != nil {
		return err
	}
	if err := y.NodeBucketController.ApplyTo(&c.ComponentConfig.NodeBucketController); err != nil {
		return err
	}
	if err := y.LoadBalancerSetController.ApplyTo(&c.ComponentConfig.LoadBalancerSetController); err != nil {
		return err
	}
	if err := y.EdgeRouteController.ApplyTo(&c.ComponentConfig.EdgeRouteController); err != nil {
		return err
	}
	return nil
}

// Config return a yurt-manager config objective
func (y *YurtManagerOptions) Config(allControllers []string, controllerAliases map[string]string) (*config.Config, error) {
	if err := y.Validate(allControllers, controllerAliases); err != nil {
		return nil, err
	}

	c := &config.Config{}
	if err := y.ApplyTo(c, controllerAliases); err != nil {
		return nil, err
	}

	return c, nil
}
