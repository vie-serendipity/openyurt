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

type imageCustomizationFilterHandler struct {
	region string
}

func NewImageCustomizationFilterHandler(
	region string) filter.ObjectHandler {
	return &imageCustomizationFilterHandler{
		region: region,
	}
}

// RuntimeObjectFilter mutate the image in the system component pod(like kube-proxy) from response object
func (fh *imageCustomizationFilterHandler) RuntimeObjectFilter(obj runtime.Object) (runtime.Object, bool) {
	if fh.region == "" {
		klog.Errorf("skip filter, image region of imageCustomizationFilterHandler is empty")
		return obj, false
	}

	switch v := obj.(type) {
	case *v1.PodList:
		for i := range v.Items {
			fh.replacePodImage(&v.Items[i])
		}
		return v, false
	case *v1.Pod:
		fh.replacePodImage(v)
		return v, false
	default:
		return obj, false
	}
}

func (fh *imageCustomizationFilterHandler) replacePodImage(pod *v1.Pod) {
	if pod.Namespace == systemNamespace && isSystemComponent(pod.Name) {
		for j := range pod.Spec.InitContainers {
			pod.Spec.InitContainers[j].Image = fh.replaceImageRegion(pod.Spec.InitContainers[j].Image)
		}
		for j := range pod.Spec.Containers {
			pod.Spec.Containers[j].Image = fh.replaceImageRegion(pod.Spec.Containers[j].Image)
		}
	}
}

func (fh *imageCustomizationFilterHandler) replaceImageRegion(image string) string {
	s := strings.Split(image, ".")
	if len(s) < 2 {
		return image
	}

	oldStr := s[0] + "." + s[1]
	newStr := mutateRegitstryPrefix + fh.region
	if strings.HasPrefix(oldStr, registryPrefix) {
		res := strings.Replace(image, oldStr, newStr, -1)
		klog.V(2).Infof("mutate pod image: %s into region image: %s", image, res)
		return res
	}

	// change image: registry-{{.Region}}.ack.aliyuncs.com
	// to registry-{{.Region}}-vpc.ack.aliyuncs.com
	if strings.Contains(image, ".ack.aliyuncs.com") {
		if strings.HasPrefix(s[0], "registry-") && !strings.HasSuffix(s[0], "-vpc") {
			newPrefix := fmt.Sprintf("registry-%s-vpc", fh.region)
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
