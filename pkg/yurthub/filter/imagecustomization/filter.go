/*
Copyright 2023 The OpenYurt Authors.

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
	"regexp"
	"strings"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/yurthub/filter"
)

const (
	PublicImageRepo  = "public"
	PrivateImageRepo = "private"
)

var (
	systemComponents = map[string][]string{
		"kube-system": {
			"logtail-ds",
			"edge-tunnel-agent",
			"coredns",
			"kube-flannel-ds",
			"kube-proxy",
			"csi-local-plugin",
			"csi-ens-plugin",
			"csi-plugin",
			"node-resource-manager",
			"raven-agent-ds",
			"yss-upgrade-worker",
			"gpushare-core-device-plugin-ds",
			"gpushare-device-plugin-ds",
			"gpushare-mps-device-plugin-ds",
			"gputopo-device-plugin-ds",
			"migparted-device-plugin",
			"nvidia-device-plugin-recover",
			"ack-node-problem-detector-daemonset",
		},
		"fluid-system": {
			"csi-nodeplugin-fluid",
		},
		"arms-prom": {
			"ack-prometheus-gpu-exporter",
			"node-exporter",
		},
	}
	matchForOldRegistry = regexp.MustCompile(`^registry(-vpc)?\.(cn-\w+)\.aliyuncs\.com(.*)`)
	matchForEERegistry  = regexp.MustCompile(`^registry-(cn-\w+)(-vpc)?\.ack\.aliyuncs\.com(.*)`)
)

const (
	conversionToOldPrivateRegistry              = "registry-vpc.%s.aliyuncs.com$3"
	conversionToOldPrivateRegistryWithoutRegion = "registry-vpc.$2.aliyuncs.com$3"
	conversionToOldPublicRegistry               = "registry.%s.aliyuncs.com$3"
	conversionToOldPublicRegistryWithoutRegion  = "registry.$2.aliyuncs.com$3"
	conversionToEEPrivateRegistry               = "registry-%s-vpc.ack.aliyuncs.com$3"
	conversionToEEPrivateRegistryWithoutRegion  = "registry-$1-vpc.ack.aliyuncs.com$3"
	conversionToEEPublicRegistry                = "registry-%s.ack.aliyuncs.com$3"
	conversionToEEPublicRegistryWithoutRegion   = "registry-$1.ack.aliyuncs.com$3"
)

// Register registers a filter
func Register(filters *filter.Filters) {
	filters.Register(filter.ImageCustomizationFilterName, func() (filter.ObjectFilter, error) {
		return NewFilter(), nil
	})
}

func NewFilter() *imageCustomizationFilter {
	return &imageCustomizationFilter{}
}

type imageCustomizationFilter struct {
	region        string
	imageRepoType string
}

func (icf *imageCustomizationFilter) Name() string {
	return filter.ImageCustomizationFilterName
}

func (icf *imageCustomizationFilter) SupportedResourceAndVerbs() map[string]sets.String {
	return map[string]sets.String{
		"pods": sets.NewString("get", "list", "watch", "patch"),
	}
}

func (icf *imageCustomizationFilter) SetImageInfo(r, repoType string) error {
	icf.region = r
	icf.imageRepoType = repoType
	return nil
}

func (icf *imageCustomizationFilter) Filter(obj runtime.Object, stopCh <-chan struct{}) runtime.Object {
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
	if isSystemComponent(pod.Namespace, pod.Name) {
		for j := range pod.Spec.InitContainers {
			newImage := icf.replaceImageRegion(pod.Spec.InitContainers[j].Image)
			pod.Spec.InitContainers[j].Image = newImage
		}
		for j := range pod.Spec.Containers {
			newImage := icf.replaceImageRegion(pod.Spec.Containers[j].Image)
			pod.Spec.Containers[j].Image = newImage
		}
	}
}

func (icf *imageCustomizationFilter) replaceImageRegion(image string) string {
	if matchForOldRegistry.MatchString(image) {
		var newImage string
		if icf.imageRepoType == PrivateImageRepo {
			newImage = conversionToOldPrivateRegistryWithoutRegion
			if len(icf.region) != 0 {
				newImage = fmt.Sprintf(conversionToOldPrivateRegistry, icf.region)
			}
		} else {
			newImage = conversionToOldPublicRegistryWithoutRegion
			if len(icf.region) != 0 {
				newImage = fmt.Sprintf(conversionToOldPublicRegistry, icf.region)
			}
		}
		klog.Infof("imageCustomizationFilter: convert image %s to %s", image, matchForOldRegistry.ReplaceAllString(image, newImage))
		return matchForOldRegistry.ReplaceAllString(image, newImage)
	}

	if matchForEERegistry.MatchString(image) {
		var newImage string
		if icf.imageRepoType == PrivateImageRepo {
			newImage = conversionToEEPrivateRegistryWithoutRegion
			if len(icf.region) != 0 {
				newImage = fmt.Sprintf(conversionToEEPrivateRegistry, icf.region)
			}
		} else {
			newImage = conversionToEEPublicRegistryWithoutRegion
			if len(icf.region) != 0 {
				newImage = fmt.Sprintf(conversionToEEPublicRegistry, icf.region)
			}
		}
		klog.Infof("imageCustomizationFilter: convert image %s to %s", image, matchForEERegistry.ReplaceAllString(image, newImage))
		return matchForEERegistry.ReplaceAllString(image, newImage)
	}

	return image
}

func isSystemComponent(ns, name string) bool {
	if ns == "" || name == "" {
		return false
	}

	componentNames := systemComponents[ns]
	for i := range componentNames {
		if strings.HasPrefix(name, componentNames[i]) {
			return true
		}
	}
	return false
}
