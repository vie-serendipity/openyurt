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
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/raven/util"
	"github.com/spf13/pflag"

	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/raven/gatewaylifecycle/config"
)

type GatewayLifecycleControllerOptions struct {
	*config.GatewayLifecycleControllerConfiguration
}

func NewGatewayLifecycleControllerOptions() *GatewayLifecycleControllerOptions {
	return &GatewayLifecycleControllerOptions{
		&config.GatewayLifecycleControllerConfiguration{CentreExposedPorts: util.CentreGatewayExposedPorts},
	}
}

// AddFlags adds flags related to nodepool for yurt-manager to the specified FlagSet.
func (g *GatewayLifecycleControllerOptions) AddFlags(fs *pflag.FlagSet) {
	if g == nil {
		return
	}
	fs.StringVar(&g.CentreExposedPorts, "centre-exposed-ports", g.CentreExposedPorts, "The ports are used by central gateway to exposed.")

}

// ApplyTo fills up nodepool config with options.
func (g *GatewayLifecycleControllerOptions) ApplyTo(cfg *config.GatewayLifecycleControllerConfiguration) error {
	if g == nil {
		return nil
	}
	cfg.CentreExposedPorts = g.CentreExposedPorts
	return nil
}

// Validate checks validation of GatewayControllerOptions.
func (g *GatewayLifecycleControllerOptions) Validate() []error {
	if g == nil {
		return nil
	}
	var errs []error
	return errs
}
