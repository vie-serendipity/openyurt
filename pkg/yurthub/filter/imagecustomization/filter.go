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
	"fmt"
	"strings"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/yurthub/filter"
)

const (
	systemNamespace       = "kube-system"
	registryPrefix        = "registry.cn-"
	mutateRegitstryPrefix = "registry-vpc."
)

var (
	systemComponents = []string{"logtail-ds", "edge-tunnel-agent", "coredns", "kube-flannel-ds",
		"kube-proxy", "csi-local-plugin", "node-resource-manager"}
)

// Register registers a filter
func Register(filters *filter.Filters) {
	filters.Register(filter.ImageCustomizationFilterName, func() (filter.ObjectFilter, error) {
		return &imageCustomizationFilter{}, nil
	})
}

type imageCustomizationFilter struct {
	region string
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

// Filter mutate the image in the system component pod(like kube-proxy) from response object
func (icf *imageCustomizationFilter) Filter(obj runtime.Object, _ <-chan struct{}) runtime.Object {
	if icf.region == "" {
		klog.Errorf("skip filter, image region of imageCustomizationFilterHandler is empty")
		return obj
	}

	switch v := obj.(type) {
	case *v1.PodList:
		for i := range v.Items {
			icf.replacePodImage(&v.Items[i])
		}
		return v
	case *v1.Pod:
		icf.replacePodImage(v)
		return v
	default:
		return obj
	}
}

func (icf *imageCustomizationFilter) replacePodImage(pod *v1.Pod) {
	if pod.Namespace == systemNamespace && isSystemComponent(pod.Name) {
		for j := range pod.Spec.InitContainers {
			pod.Spec.InitContainers[j].Image = icf.replaceImageRegion(pod.Spec.InitContainers[j].Image)
		}
		for j := range pod.Spec.Containers {
			pod.Spec.Containers[j].Image = icf.replaceImageRegion(pod.Spec.Containers[j].Image)
		}
	}
}

func (icf *imageCustomizationFilter) replaceImageRegion(image string) string {
	s := strings.Split(image, ".")
	if len(s) < 2 {
		return image
	}

	oldStr := s[0] + "." + s[1]
	newStr := mutateRegitstryPrefix + icf.region
	if strings.HasPrefix(oldStr, registryPrefix) {
		res := strings.Replace(image, oldStr, newStr, -1)
		klog.V(2).Infof("mutate pod image: %s into region image: %s", image, res)
		return res
	}

	// change image: registry-{{.Region}}.ack.aliyuncs.com
	// to registry-{{.Region}}-vpc.ack.aliyuncs.com
	if strings.Contains(image, ".ack.aliyuncs.com") {
		if strings.HasPrefix(s[0], "registry-") && !strings.HasSuffix(s[0], "-vpc") {
			newPrefix := fmt.Sprintf("registry-%s-vpc", icf.region)
			res := strings.Replace(image, s[0], newPrefix, -1)
			klog.V(2).Infof("mutate pod image: %s into region image: %s", image, res)
			return res
		}
	}
	return image
}

func isSystemComponent(podName string) bool {
	if podName == "" {
		return false
	}

	for i := range systemComponents {
		if strings.HasPrefix(podName, systemComponents[i]) {
			return true
		}
	}
	return false
}
