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

	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/cloudnodeslabels/config"
)

type CloudNodesLabelsControllerOptions struct {
	*config.CloudNodesLabelsControllerConfiguration
}

func NewCloudnodeslabelsControllerOptions() *CloudNodesLabelsControllerOptions {
	return &CloudNodesLabelsControllerOptions{
		&config.CloudNodesLabelsControllerConfiguration{},
	}
}

// AddFlags adds flags related to cloudnodeslabels for yurt-manager to the specified FlagSet.
func (n *CloudNodesLabelsControllerOptions) AddFlags(fs *pflag.FlagSet) {
	if n == nil {
		return
	}

}

// ApplyTo fills up cloudnodeslabels config with options.
func (o *CloudNodesLabelsControllerOptions) ApplyTo(cfg *config.CloudNodesLabelsControllerConfiguration) error {
	if o == nil {
		return nil
	}

	return nil
}

// Validate checks validation of CloudnodeslabelsControllerOptions.
func (o *CloudNodesLabelsControllerOptions) Validate() []error {
	if o == nil {
		return nil
	}
	errs := []error{}
	return errs
}
