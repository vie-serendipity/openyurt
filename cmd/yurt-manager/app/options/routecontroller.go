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

package options

import (
	"errors"
	"fmt"
	"time"

	"github.com/spf13/pflag"
	"k8s.io/utils/net"

	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/route/config"
)

type RouteControllerOptions struct {
	*config.RouteControllerConfiguration
}

func NewRouteControllerOptions() *RouteControllerOptions {
	return &RouteControllerOptions{
		&config.RouteControllerConfiguration{},
	}
}

// AddFlags adds flags related to route for yurt-manager to the specified FlagSet.
func (n *RouteControllerOptions) AddFlags(fs *pflag.FlagSet) {
	if n == nil {
		return
	}

	fs.StringVar(&n.ClusterCIDR, "cluster-cidr", "", "CIDR Range for Pods in cluster.")
	fs.DurationVar(&n.RouteReconciliationPeriod.Duration, "route-reconciliation-period", time.Hour,
		"The period for reconciling routes created for nodes by cloud provider. The minimum value is 1 minute")
	fs.BoolVar(&n.SyncRoutes, "sync-edge-route", true, "Whether to synchronize routes regularly")
	fs.StringVar(&n.RouteTableIDs, "route-table-ids", "", "Specify the rout table IDs to use")
}

// ApplyTo fills up route config with options.
func (o *RouteControllerOptions) ApplyTo(cfg *config.RouteControllerConfiguration) error {
	if o == nil {
		return nil
	}
	if o.RouteReconciliationPeriod.Duration < time.Minute {
		o.RouteReconciliationPeriod.Duration = time.Minute
	}
	if o.RouteReconciliationPeriod.Duration > 24*time.Hour {
		o.RouteReconciliationPeriod.Duration = 24 * time.Hour
	}
	cfg.SyncRoutes = o.SyncRoutes
	cfg.RouteReconciliationPeriod = o.RouteReconciliationPeriod
	cfg.ClusterCIDR = o.ClusterCIDR
	cfg.RouteTableIDs = o.RouteTableIDs

	return nil
}

// Validate checks validation of RouteControllerOptions.
func (o *RouteControllerOptions) Validate() []error {
	if o == nil {
		return nil
	}
	errs := []error{}
	if o.ClusterCIDR == "" {
		errs = append(errs, errors.New("cluster CIDR is empty"))
	} else {
		_, _, err := net.ParseCIDRSloppy(o.ClusterCIDR)
		if err != nil {
			errs = append(errs, errors.New(fmt.Sprintf("cluster CIDR is incorrect, %s", err.Error())))
		}
	}
	return errs
}
