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
	"github.com/openyurtio/openyurt/pkg/controller/yurtappconfigrender/config"
	"github.com/spf13/pflag"
)

type YurtAppConfigurationReplacementControllerOptions struct {
	*config.YurtAppConfigurationReplacementControllerConfiguration
}

func NewYurtAppConfigurationReplacementControllerOptions() *YurtAppConfigurationReplacementControllerOptions {
	return &YurtAppConfigurationReplacementControllerOptions{
		&config.YurtAppConfigurationReplacementControllerConfiguration{},
	}
}

// AddFlags adds flags related to nodepool for yurt-manager to the specified FlagSet.
func (n *YurtAppConfigurationReplacementControllerOptions) AddFlags(fs *pflag.FlagSet) {
	if n == nil {
		return
	}

	//fs.BoolVar(&n.CreateDefaultPool, "create-default-pool", n.CreateDefaultPool, "Create default cloud/edge pools if indicated.")
}

// ApplyTo fills up nodepool config with options.
func (o *YurtAppConfigurationReplacementControllerOptions) ApplyTo(cfg *config.YurtAppConfigurationReplacementControllerConfiguration) error {
	if o == nil {
		return nil
	}

	return nil
}

// Validate checks validation of YurtAppConfigurationReplacementControllerOptions.
func (o *YurtAppConfigurationReplacementControllerOptions) Validate() []error {
	if o == nil {
		return nil
	}
	errs := []error{}
	return errs
}
