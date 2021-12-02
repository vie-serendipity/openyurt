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

package imagecustomization

import (
	"io"
	"net/http"

	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/openyurtio/openyurt/pkg/yurthub/filter"
	"github.com/openyurtio/openyurt/pkg/yurthub/kubernetes/serializer"
)

// Register registers a filter
func Register(filters *filter.Filters, sm *serializer.SerializerManager) {
	filters.Register(filter.ImageCustomizationFilterName, func() (filter.Runner, error) {
		return NewFilter(sm), nil
	})
}

func NewFilter(sm *serializer.SerializerManager) *imageCustomizationFilter {
	return &imageCustomizationFilter{
		serializerManager: sm,
	}
}

type imageCustomizationFilter struct {
	serializerManager *serializer.SerializerManager
	region            string
}

func (icf *imageCustomizationFilter) Name() string {
	return filter.ImageCustomizationFilterName
}

func (icf *imageCustomizationFilter) SupportedResourceAndVerbs() map[string]sets.String {
	return map[string]sets.String{
		"pods": sets.NewString("list", "watch"),
	}
}

func (icf *imageCustomizationFilter) SetImageRegion(r string) error {
	icf.region = r
	return nil
}

func (icf *imageCustomizationFilter) Filter(req *http.Request, rc io.ReadCloser, stopCh <-chan struct{}) (int, io.ReadCloser, error) {
	handler := NewImageCustomizationFilterHandler(icf.region)
	return filter.NewFilterReadCloser(req, icf.serializerManager, rc, handler, icf.Name(), stopCh)
}
