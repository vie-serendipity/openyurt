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
	"strings"

	"github.com/spf13/pflag"

	vsagconfig "github.com/openyurtio/openyurt/pkg/controller/vsag/config"
)

// VsagControllerOptions contains elements describing VsagController.
type VsagControllerOptions struct {
	*vsagconfig.VsagControllerConfiguration
}

// AddFlags adds flags related to VsagController for controller manager to the specified FlagSet.
func (o *VsagControllerOptions) AddFlags(fs *pflag.FlagSet) {
	if o == nil {
		return
	}

	fs.StringVar(&o.VsagCoreImage, "vsag-core-image", o.VsagCoreImage, "The vsag core image.")
	fs.StringVar(&o.VsagSideCarImage, "vsag-sidecar-image", o.VsagSideCarImage, "The vsag sidecar image.")
	fs.StringVar(&o.VsagHelperImage, "vsag-helper-image", o.VsagHelperImage, "The vsag helper image.")
	fs.IntVar(&o.ConcurrentVsagWorkers, "concurrent-vsag-workers", o.ConcurrentVsagWorkers, "The numbers of vsag syncing workers.")
	fs.StringVar(&o.HostAkCredSecretName, "resource-ak-secret-name", o.HostAkCredSecretName, "The secret contains resource ak credential info.")
	fs.StringVar(&o.CloudConfigFile, "cloud-config", o.CloudConfigFile, "The path to the cloud provider configuration file.")
}

// ApplyTo fills up VsagController config with options.
func (o *VsagControllerOptions) ApplyTo(cfg *vsagconfig.VsagControllerConfiguration) error {
	if o == nil {
		return nil
	}

	cfg.HostAkCredSecretName = o.HostAkCredSecretName
	cfg.CloudConfigFile = o.CloudConfigFile
	// edge node works in internet
	cfg.VsagCoreImage = convertVpcImageToPublicImage(o.VsagCoreImage)
	cfg.VsagSideCarImage = convertVpcImageToPublicImage(o.VsagSideCarImage)
	cfg.VsagHelperImage = convertVpcImageToPublicImage(o.VsagHelperImage)

	cfg.ConcurrentVsagWorkers = o.ConcurrentVsagWorkers

	return nil
}

// Validate checks validation of VsagController.
func (o *VsagControllerOptions) Validate() []error {
	if o == nil {
		return nil
	}

	errs := []error{}
	return errs
}

func convertVpcImageToPublicImage(image string) string {
	if strings.Contains(image, "registry-vpc") {
		return strings.Replace(image, "registry-vpc", "registry", -1)
	}
	if strings.Contains(image, "-vpc.ack.aliyuncs") {
		return strings.Replace(image, "-vpc.ack.aliyuncs", ".ack.aliyuncs", -1)
	}
	return image
}
