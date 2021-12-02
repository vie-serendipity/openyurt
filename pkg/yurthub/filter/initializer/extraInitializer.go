/*
Copyright 2021 The OpenYurt Authors.

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

package initializer

import "github.com/openyurtio/openyurt/pkg/yurthub/filter"

// WantsImageRegion is an interface for setting system component pod image registry
type WantsImageRegion interface {
	SetImageRegion(string) error
}

// imageCustomizationInitializer is responsible for initializing extra filters(except discardcloudservice, masterservice, servicetopology)
type extraInitializer struct {
	region string
}

// New creates an filterInitializer object
func NewExtraInitializer(region string) *extraInitializer {
	return &extraInitializer{
		region: region,
	}
}

func (ei *extraInitializer) Initialize(ins filter.Runner) error {
	if wants, ok := ins.(WantsImageRegion); ok {
		if err := wants.SetImageRegion(ei.region); err != nil {
			return err
		}
	}
	return nil
}
