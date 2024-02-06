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
	"fmt"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/raven/util"
	"github.com/spf13/pflag"
	"strconv"
	"strings"

	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/raven/gatewaylifecycle/config"
)

type GatewayLifecycleControllerOptions struct {
	CentreExposedProxyPorts string
	CentreExposedTunnelPort int
}

func NewGatewayLifecycleControllerOptions() *GatewayLifecycleControllerOptions {
	return &GatewayLifecycleControllerOptions{
		CentreExposedProxyPorts: util.CentreGatewayExposedPorts,
		CentreExposedTunnelPort: 4500,
	}
}

// AddFlags adds flags related to nodepool for yurt-manager to the specified FlagSet.
func (g *GatewayLifecycleControllerOptions) AddFlags(fs *pflag.FlagSet) {
	if g == nil {
		return
	}
	fs.StringVar(&g.CentreExposedProxyPorts, "centre-exposed-proxy-ports", g.CentreExposedProxyPorts, "The proxy ports are used by central gateway to exposed.")
	fs.IntVar(&g.CentreExposedTunnelPort, "centre-exposed-tunnel-port", g.CentreExposedTunnelPort, "The tunnel ports are used by central gateway to exposed.")

}

// ApplyTo fills up nodepool config with options.
func (g *GatewayLifecycleControllerOptions) ApplyTo(cfg *config.GatewayLifecycleControllerConfiguration) error {
	if g == nil {
		return nil
	}
	exposedProxyPorts := make([]int, 0)
	proxyPorts := strings.Split(g.CentreExposedProxyPorts, ",")
	for i := range proxyPorts {
		p, err := strconv.Atoi(proxyPorts[i])
		if err != nil {
			return err
		}
		if p > 65535 || p < 1 {
			return fmt.Errorf("%d is not a compliant port", p)
		}
		exposedProxyPorts = append(exposedProxyPorts, p)
	}
	cfg.CentreExposedProxyPorts = exposedProxyPorts
	if g.CentreExposedTunnelPort < 65535 && g.CentreExposedTunnelPort > 0 {
		cfg.CentreExposedTunnelPort = g.CentreExposedTunnelPort
	} else {
		return fmt.Errorf("%d is not a compliant port", g.CentreExposedTunnelPort)
	}
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
