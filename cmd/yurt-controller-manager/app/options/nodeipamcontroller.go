/*
Copyright 2018 The Kubernetes Authors.

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
	"fmt"
	"strings"

	"github.com/spf13/pflag"

	nodeipamconfig "github.com/openyurtio/openyurt/pkg/controller/nodeipam/config"
)

// NodeIPAMControllerConfiguration contains elements describing NodeIPAMController.
type NodeIPAMControllerOptions struct {
	*nodeipamconfig.NodeIPAMControllerConfiguration
}

// AddFlags adds flags related to NodeIPAMController for controller manager to the specified FlagSet.
func (o *NodeIPAMControllerOptions) AddFlags(fs *pflag.FlagSet) {
	if o == nil {
		return
	}

	fs.StringVar(&o.ClusterCIDR, "cluster-cidr", o.ClusterCIDR, "CIDR Range for Pods in cluster.")
	fs.StringVar(&o.ServiceCIDR, "service-cluster-ip-range", o.ServiceCIDR, "CIDR Range for Services in cluster.")
	fs.Int32Var(&o.NodeCIDRMaskSize, "node-cidr-mask-size", o.NodeCIDRMaskSize, "Mask size for node cidr in cluster.")
	fs.BoolVar(&o.LegacyIpam, "legacy-ipam", o.LegacyIpam, "Whether to use original node ipam logic or the new logic regarding to nodepool size.")
}

// ApplyTo fills up NodeIPAMController config with options.
func (o *NodeIPAMControllerOptions) ApplyTo(cfg *nodeipamconfig.NodeIPAMControllerConfiguration) error {
	if o == nil {
		return nil
	}

	// split the cidrs list and assign primary and secondary
	serviceCIDRList := strings.Split(o.ServiceCIDR, ",")
	if len(serviceCIDRList) > 0 {
		cfg.ServiceCIDR = serviceCIDRList[0]
	}
	if len(serviceCIDRList) > 1 {
		cfg.SecondaryServiceCIDR = serviceCIDRList[1]
	}

	cfg.ClusterCIDR = o.ClusterCIDR
	cfg.NodeCIDRMaskSize = o.NodeCIDRMaskSize
	cfg.LegacyIpam = o.LegacyIpam

	return nil
}

// Validate checks validation of NodeIPAMControllerOptions.
func (o *NodeIPAMControllerOptions) Validate() []error {
	if o == nil {
		return nil
	}
	errs := make([]error, 0)

	serviceCIDRList := strings.Split(o.ServiceCIDR, ",")
	if len(serviceCIDRList) > 2 {
		errs = append(errs, fmt.Errorf("--service-cluster-ip-range can not contain more than two entries"))
	}

	return errs
}
