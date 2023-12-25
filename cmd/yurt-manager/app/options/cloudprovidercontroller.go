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
	"github.com/spf13/pflag"

	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/raven/util/config"
)

type CloudProviderControllerOptions struct {
	*config.RavenCloudProviderConfiguration
}

func NewCloudProviderControllerOptions() *CloudProviderControllerOptions {
	return &CloudProviderControllerOptions{
		&config.RavenCloudProviderConfiguration{},
	}
}

// AddFlags adds flags related to nodepool for yurt-manager to the specified FlagSet.
func (g *CloudProviderControllerOptions) AddFlags(fs *pflag.FlagSet) {
	if g == nil {
		return
	}
	fs.StringVar(&g.CloudConfigPath, "cloud-config", g.CloudConfigPath, "The path to the cloud provider configuration file.")

}

// ApplyTo fills up nodepool config with options.
func (g *CloudProviderControllerOptions) ApplyTo(cfg *config.RavenCloudProviderConfiguration) error {
	if g == nil {
		return nil
	}
	cfg.CloudConfigPath = g.CloudConfigPath

	return nil
}

// Validate checks validation of GatewayControllerOptions.
func (g *CloudProviderControllerOptions) Validate() []error {
	if g == nil {
		return nil
	}
	var errs []error
	return errs
}
