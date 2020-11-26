/*
Copyright 2019 The Kubernetes Authors.

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

// VsagControllerConfiguration contains elements describing VSAGController.
type VsagControllerConfiguration struct {
	// ConcurrentWorkers is the numbers of syncing workers.
	ConcurrentVsagWorkers int
	// HostAkCredSecretName is the secret contains resource ak credential info.
	HostAkCredSecretName string
	// CloudConfigFile is the path to cloud config file.
	CloudConfigFile string
	// The vsag core image
	VsagCoreImage string
	// The vsag sidecar image
	VsagSideCarImage string
	// The vsag helper image
	VsagHelperImage string
}
