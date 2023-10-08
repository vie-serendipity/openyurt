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

package mutetunnelnodes

import (
	"strings"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/yurthub/filter"
)

const (
	CorednsConfigMapNamespace = "kube-system"
	CorednsConfigMapName      = "coredns"
	CorednsDataKey            = "Corefile"
	CorednsMuteBlock          = `        hosts /etc/edge/tunnel-nodes {
            reload 300ms
            fallthrough
        }`
)

// Register registers a filter
func Register(filters *filter.Filters) {
	filters.Register(filter.MuteTunnelNodesFilterName, func() (filter.ObjectFilter, error) {
		return NewMutateTunnelNodesFilter()
	})
}

type muteTunnelNodesFilter struct{}

func NewMutateTunnelNodesFilter() (filter.ObjectFilter, error) {
	return &muteTunnelNodesFilter{}, nil
}

func (mtnf *muteTunnelNodesFilter) Name() string {
	return filter.MuteTunnelNodesFilterName
}

func (mtnf *muteTunnelNodesFilter) SupportedResourceAndVerbs() map[string]sets.String {
	return map[string]sets.String{
		"configmaps": sets.NewString("get", "list", "watch"),
	}
}

func (mtnf *muteTunnelNodesFilter) Filter(obj runtime.Object, _ <-chan struct{}) runtime.Object {
	switch v := obj.(type) {
	case *v1.ConfigMapList:
		for i := range v.Items {
			mutated := mutateCorednsConfigMap(&v.Items[i])
			if mutated {
				break
			}
		}
		return v
	case *v1.ConfigMap:
		mutateCorednsConfigMap(v)
		return v
	default:
		return v
	}
}

func mutateCorednsConfigMap(cm *v1.ConfigMap) bool {
	mutated := false
	if cm.Namespace == CorednsConfigMapNamespace && cm.Name == CorednsConfigMapName {
		if cm.Data != nil && len(cm.Data[CorednsDataKey]) != 0 {
			corefile := cm.Data[CorednsDataKey]
			newCorefile := strings.Replace(corefile, CorednsMuteBlock, "", 1)
			cm.Data[CorednsDataKey] = newCorefile
			klog.Infof("corefile in configmap(%s/%s) has been commented, new corefile: \n%s\n", cm.Namespace, cm.Name, cm.Data[CorednsDataKey])
			mutated = true
		}
	}
	return mutated
}
