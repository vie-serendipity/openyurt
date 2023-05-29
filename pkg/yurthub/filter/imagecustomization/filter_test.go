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
	"reflect"
	"testing"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestFilter(t *testing.T) {
	testcases := map[string]struct {
		region    string
		inputObj  runtime.Object
		expectObj runtime.Object
	}{
		"pod with vpc image": {
			region: "cn-hangzhou",
			inputObj: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "logtail-ds",
					Namespace: "kube-system",
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name:  "foo",
							Image: "registry-vpc.cn-beijing.aliyuncs.com/acs/logtail-ds:v1.0",
						},
					},
				},
			},
			expectObj: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "logtail-ds",
					Namespace: "kube-system",
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name:  "foo",
							Image: "registry-vpc.cn-beijing.aliyuncs.com/acs/logtail-ds:v1.0",
						},
					},
				},
			},
		},
		"pod with internet image": {
			region: "cn-hangzhou",
			inputObj: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "edge-tunnel-agent",
					Namespace: "kube-system",
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name:  "foo",
							Image: "registry.cn-beijing.aliyuncs.com/acs/edge-tunnel-agent:v1.0",
						},
					},
				},
			},
			expectObj: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "edge-tunnel-agent",
					Namespace: "kube-system",
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name:  "foo",
							Image: "registry-vpc.cn-hangzhou.aliyuncs.com/acs/edge-tunnel-agent:v1.0",
						},
					},
				},
			},
		},
		"pod with acr ee vpc image": {
			region: "cn-hangzhou",
			inputObj: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "coredns",
					Namespace: "kube-system",
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name:  "foo",
							Image: "registry-cn-beijing-vpc.ack.aliyuncs.com/acs/coredns:v1.0",
						},
					},
				},
			},
			expectObj: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "coredns",
					Namespace: "kube-system",
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name:  "foo",
							Image: "registry-cn-beijing-vpc.ack.aliyuncs.com/acs/coredns:v1.0",
						},
					},
				},
			},
		},
		"pod witch acr ee internet image": {
			region: "cn-hangzhou",
			inputObj: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "kube-flannel-ds",
					Namespace: "kube-system",
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name:  "foo",
							Image: "registry-cn-beijing.ack.aliyuncs.com/acs/kube-flannel-ds:v1.0",
						},
					},
				},
			},
			expectObj: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "kube-flannel-ds",
					Namespace: "kube-system",
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name:  "foo",
							Image: "registry-cn-hangzhou-vpc.ack.aliyuncs.com/acs/kube-flannel-ds:v1.0",
						},
					},
				},
			},
		},
	}

	stopCh := make(<-chan struct{})
	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			h := &imageCustomizationFilter{
				region: tc.region,
			}
			obj := h.Filter(tc.inputObj, stopCh)
			if !reflect.DeepEqual(obj, tc.expectObj) {
				t.Errorf("expect obj:\n %v, \nbut got %v\n", tc.expectObj, obj)
			}
		})
	}
}
