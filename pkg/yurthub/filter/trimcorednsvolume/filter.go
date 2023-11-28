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

package trimcorednsvolume

import (
	"strings"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/openyurtio/openyurt/pkg/yurthub/filter"
)

const (
	CorednsPodNamespace  = "kube-system"
	CorednsPodNamePrefix = "coredns-"
)

// Register registers a filter
func Register(filters *filter.Filters) {
	filters.Register(filter.TrimCorednsVolumeFilterName, func() (filter.ObjectFilter, error) {
		return NewTrimCorednsVolumeFilter()
	})
}

func NewTrimCorednsVolumeFilter() (filter.ObjectFilter, error) {
	return &trimCorednsVolumeFilter{}, nil
}

type trimCorednsVolumeFilter struct{}

func (tcvf *trimCorednsVolumeFilter) Name() string {
	return filter.TrimCorednsVolumeFilterName
}

func (tcvf *trimCorednsVolumeFilter) SupportedResourceAndVerbs() map[string]sets.String {
	return map[string]sets.String{
		"pods": sets.NewString("get", "list", "watch"),
	}
}

func (tcvf *trimCorednsVolumeFilter) Filter(obj runtime.Object, stopCh <-chan struct{}) runtime.Object {
	switch v := obj.(type) {
	case *v1.PodList:
		for i := range v.Items {
			if v.Items[i].Namespace == CorednsPodNamespace && strings.HasPrefix(v.Items[i].Name, CorednsPodNamePrefix) {
				trimCorednsVolume(&v.Items[i])
				break
			}
		}
		return v
	case *v1.Pod:
		if v.Namespace == CorednsPodNamespace && strings.HasPrefix(v.Name, CorednsPodNamePrefix) {
			trimCorednsVolume(v)
		}
		return v
	default:
		return obj
	}
}

func trimCorednsVolume(pod *v1.Pod) {
	var newVolumes []v1.Volume
	for _, volume := range pod.Spec.Volumes {
		if volume.Name != "hosts" {
			newVolumes = append(newVolumes, volume)
		}
	}
	pod.Spec.Volumes = newVolumes

	for i, container := range pod.Spec.Containers {
		var newVolumeMounts []v1.VolumeMount
		for _, volumeMount := range container.VolumeMounts {
			if volumeMount.Name != "hosts" {
				newVolumeMounts = append(newVolumeMounts, volumeMount)
			}
		}
		pod.Spec.Containers[i].VolumeMounts = newVolumeMounts
	}
}
