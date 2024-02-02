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

func TestRuntimeObjectFilter(t *testing.T) {
	testcases := map[string]struct {
		region        string
		imageRepoType string
		inputObj      runtime.Object
		expectObj     runtime.Object
	}{
		"pod list pod with vpc image": {
			region:        "cn-beijing",
			imageRepoType: PublicImageRepo,
			inputObj: &v1.PodList{
				Items: []v1.Pod{
					{
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
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "coredns",
							Namespace: "kube-system",
						},
						Spec: v1.PodSpec{
							Containers: []v1.Container{
								{
									Name:  "foo",
									Image: "registry-cn-hangzhou-vpc.ack.aliyuncs.com/acs/coredns:v1.0",
								},
							},
						},
					},
				},
			},
			expectObj: &v1.PodList{
				Items: []v1.Pod{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "logtail-ds",
							Namespace: "kube-system",
						},
						Spec: v1.PodSpec{
							Containers: []v1.Container{
								{
									Name:  "foo",
									Image: "registry.cn-beijing.aliyuncs.com/acs/logtail-ds:v1.0",
								},
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "coredns",
							Namespace: "kube-system",
						},
						Spec: v1.PodSpec{
							Containers: []v1.Container{
								{
									Name:  "foo",
									Image: "registry-cn-beijing.ack.aliyuncs.com/acs/coredns:v1.0",
								},
							},
						},
					},
				},
			},
		},
		"pod list without region": {
			imageRepoType: PublicImageRepo,
			inputObj: &v1.PodList{
				Items: []v1.Pod{
					{
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
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "coredns",
							Namespace: "kube-system",
						},
						Spec: v1.PodSpec{
							Containers: []v1.Container{
								{
									Name:  "foo",
									Image: "registry-cn-hangzhou-vpc.ack.aliyuncs.com/acs/coredns:v1.0",
								},
							},
						},
					},
				},
			},
			expectObj: &v1.PodList{
				Items: []v1.Pod{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "logtail-ds",
							Namespace: "kube-system",
						},
						Spec: v1.PodSpec{
							Containers: []v1.Container{
								{
									Name:  "foo",
									Image: "registry.cn-beijing.aliyuncs.com/acs/logtail-ds:v1.0",
								},
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "coredns",
							Namespace: "kube-system",
						},
						Spec: v1.PodSpec{
							Containers: []v1.Container{
								{
									Name:  "foo",
									Image: "registry-cn-hangzhou.ack.aliyuncs.com/acs/coredns:v1.0",
								},
							},
						},
					},
				},
			},
		},
		"private repo: pod with vpc image": {
			region:        "cn-beijing",
			imageRepoType: PrivateImageRepo,
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
		"private repo: pod with public image": {
			region:        "cn-beijing",
			imageRepoType: PrivateImageRepo,
			inputObj: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "logtail-ds",
					Namespace: "kube-system",
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name:  "foo",
							Image: "registry.cn-beijing.aliyuncs.com/acs/logtail-ds:v1.0",
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
		"private repo: public image with different region": {
			region:        "cn-hangzhou",
			imageRepoType: PrivateImageRepo,
			inputObj: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "logtail-ds",
					Namespace: "kube-system",
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name:  "foo",
							Image: "registry.cn-beijing.aliyuncs.com/acs/logtail-ds:v1.0",
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
							Image: "registry-vpc.cn-hangzhou.aliyuncs.com/acs/logtail-ds:v1.0",
						},
					},
				},
			},
		},
		"private repo: vpc image with different region": {
			region:        "cn-hangzhou",
			imageRepoType: PrivateImageRepo,
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
							Image: "registry-vpc.cn-hangzhou.aliyuncs.com/acs/logtail-ds:v1.0",
						},
					},
				},
			},
		},
		"private repo: pod with acr ee vpc image": {
			region:        "cn-beijing",
			imageRepoType: PrivateImageRepo,
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
		"private repo: pod with acr ee public image": {
			region:        "cn-beijing",
			imageRepoType: PrivateImageRepo,
			inputObj: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "coredns",
					Namespace: "kube-system",
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name:  "foo",
							Image: "registry-cn-beijing.ack.aliyuncs.com/acs/coredns:v1.0",
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
		"private repo: acr ee public image with different region": {
			region:        "cn-hangzhou",
			imageRepoType: PrivateImageRepo,
			inputObj: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "coredns",
					Namespace: "kube-system",
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name:  "foo",
							Image: "registry-cn-beijing.ack.aliyuncs.com/acs/coredns:v1.0",
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
							Image: "registry-cn-hangzhou-vpc.ack.aliyuncs.com/acs/coredns:v1.0",
						},
					},
				},
			},
		},
		"private repo: acr ee vpc image with different region": {
			region:        "cn-hangzhou",
			imageRepoType: PrivateImageRepo,
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
							Image: "registry-cn-hangzhou-vpc.ack.aliyuncs.com/acs/coredns:v1.0",
						},
					},
				},
			},
		},
		"public repo: pod with vpc image": {
			region:        "cn-beijing",
			imageRepoType: PublicImageRepo,
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
							Image: "registry.cn-beijing.aliyuncs.com/acs/logtail-ds:v1.0",
						},
					},
				},
			},
		},
		"public repo: pod with public image": {
			region:        "cn-beijing",
			imageRepoType: PublicImageRepo,
			inputObj: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "logtail-ds",
					Namespace: "kube-system",
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name:  "foo",
							Image: "registry.cn-beijing.aliyuncs.com/acs/logtail-ds:v1.0",
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
							Image: "registry.cn-beijing.aliyuncs.com/acs/logtail-ds:v1.0",
						},
					},
				},
			},
		},
		"public repo: public image with different region": {
			region:        "cn-hangzhou",
			imageRepoType: PublicImageRepo,
			inputObj: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "logtail-ds",
					Namespace: "kube-system",
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name:  "foo",
							Image: "registry.cn-beijing.aliyuncs.com/acs/logtail-ds:v1.0",
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
							Image: "registry.cn-hangzhou.aliyuncs.com/acs/logtail-ds:v1.0",
						},
					},
				},
			},
		},
		"public repo: vpc image with different region": {
			region:        "cn-hangzhou",
			imageRepoType: PublicImageRepo,
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
							Image: "registry.cn-hangzhou.aliyuncs.com/acs/logtail-ds:v1.0",
						},
					},
				},
			},
		},
		"public repo: pod with acr ee vpc image": {
			region:        "cn-beijing",
			imageRepoType: PublicImageRepo,
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
							Image: "registry-cn-beijing.ack.aliyuncs.com/acs/coredns:v1.0",
						},
					},
				},
			},
		},
		"public repo: pod with acr ee public image": {
			region:        "cn-beijing",
			imageRepoType: PublicImageRepo,
			inputObj: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "coredns",
					Namespace: "kube-system",
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name:  "foo",
							Image: "registry-cn-beijing.ack.aliyuncs.com/acs/coredns:v1.0",
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
							Image: "registry-cn-beijing.ack.aliyuncs.com/acs/coredns:v1.0",
						},
					},
				},
			},
		},
		"public repo: acr ee public image with different region": {
			region:        "cn-hangzhou",
			imageRepoType: PublicImageRepo,
			inputObj: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "coredns",
					Namespace: "kube-system",
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name:  "foo",
							Image: "registry-cn-beijing.ack.aliyuncs.com/acs/coredns:v1.0",
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
							Image: "registry-cn-hangzhou.ack.aliyuncs.com/acs/coredns:v1.0",
						},
					},
				},
			},
		},
		"public repo: acr ee vpc image with different region": {
			region:        "cn-hangzhou",
			imageRepoType: PublicImageRepo,
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
							Image: "registry-cn-hangzhou.ack.aliyuncs.com/acs/coredns:v1.0",
						},
					},
				},
			},
		},
		"public repo: acr ee vpc image with financial region": {
			imageRepoType: PublicImageRepo,
			inputObj: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "csi-plugin",
					Namespace: "kube-system",
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name:  "foo",
							Image: "registry-cn-shanghai-finance-1-vpc.ack.aliyuncs.com/acs/csi-plugin:v1.0",
						},
					},
				},
			},
			expectObj: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "csi-plugin",
					Namespace: "kube-system",
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name:  "foo",
							Image: "registry-cn-shanghai-finance-1.ack.aliyuncs.com/acs/csi-plugin:v1.0",
						},
					},
				},
			},
		},
		"public repo: acr ee vpc image with strange region": {
			imageRepoType: PublicImageRepo,
			inputObj: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "csi-plugin",
					Namespace: "kube-system",
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name:  "foo",
							Image: "registry-cn-north-2-gov-1-vpc.ack.aliyuncs.com/acs/csi-plugin:v1.0",
						},
					},
				},
			},
			expectObj: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "csi-plugin",
					Namespace: "kube-system",
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name:  "foo",
							Image: "registry-cn-north-2-gov-1.ack.aliyuncs.com/acs/csi-plugin:v1.0",
						},
					},
				},
			},
		},
		"public repo: acr ee vpc image with foreign region": {
			imageRepoType: PublicImageRepo,
			inputObj: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "csi-plugin",
					Namespace: "kube-system",
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name:  "foo",
							Image: "registry-ap-south-1-vpc.ack.aliyuncs.com/acs/csi-plugin:v1.0",
						},
					},
				},
			},
			expectObj: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "csi-plugin",
					Namespace: "kube-system",
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name:  "foo",
							Image: "registry-ap-south-1.ack.aliyuncs.com/acs/csi-plugin:v1.0",
						},
					},
				},
			},
		},
		"not ack image": {
			region:        "cn-hangzhou",
			imageRepoType: PublicImageRepo,
			inputObj: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "coredns",
					Namespace: "kube-system",
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name:  "foo",
							Image: "openyurt/coredns:v1.3.0-rc1-c41dc01",
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
							Image: "openyurt/coredns:v1.3.0-rc1-c41dc01",
						},
					},
				},
			},
		},
		"not system component": {
			region:        "cn-hangzhou",
			imageRepoType: PublicImageRepo,
			inputObj: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "yurthub",
					Namespace: "kube-system",
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name:  "foo",
							Image: "openyurt/yurthub:v1.3.0-rc1-c41dc01",
						},
					},
				},
			},
			expectObj: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "yurthub",
					Namespace: "kube-system",
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name:  "foo",
							Image: "openyurt/yurthub:v1.3.0-rc1-c41dc01",
						},
					},
				},
			},
		},
	}

	stopCh := make(chan struct{}, 0)
	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			f := NewFilter()
			f.SetImageInfo(tc.region, tc.imageRepoType)
			obj := f.Filter(tc.inputObj, stopCh)
			if !reflect.DeepEqual(obj, tc.expectObj) {
				t.Errorf("expect obj:\n %v, \nbut got %v\n", tc.expectObj, obj)
			}
		})
	}
}
