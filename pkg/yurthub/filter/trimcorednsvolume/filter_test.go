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
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/openyurtio/openyurt/pkg/yurthub/filter"
)

func TestName(t *testing.T) {
	tcvf, _ := NewTrimCorednsVolumeFilter()
	if tcvf.Name() != filter.TrimCorednsVolumeFilterName {
		t.Errorf("expect %s, but got %s", filter.TrimCorednsVolumeFilterName, tcvf.Name())
	}
}

func TestSupportedResourceAndVerbs(t *testing.T) {
	tcvf, _ := NewTrimCorednsVolumeFilter()
	rvs := tcvf.SupportedResourceAndVerbs()
	if len(rvs) != 1 {
		t.Errorf("supported not one resource, %v", rvs)
	}

	for resource, verbs := range rvs {
		if resource != "pods" {
			t.Errorf("expect resource is pods, but got %s", resource)
		}

		if !verbs.Equal(sets.NewString("get", "list", "watch")) {
			t.Errorf("expect verbs are list/watch, but got %v", verbs.UnsortedList())
		}
	}
}

func TestFilter(t *testing.T) {
	hostPathType := corev1.HostPathUnset

	testcases := map[string]struct {
		responseObject runtime.Object
		expectObject   runtime.Object
	}{
		"coredns pod with hosts volume only": {
			responseObject: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "coredns-xxx",
					Namespace: "kube-system",
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "coredns",
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "hosts",
									MountPath: "/etc/edge",
									ReadOnly:  true,
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "hosts",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "edge-tunnel-nodes",
									},
								},
							},
						},
					},
				},
			},
			expectObject: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "coredns-xxx",
					Namespace: "kube-system",
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "coredns",
						},
					},
				},
			},
		},
		"coredns pod with multiple volumes": {
			responseObject: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "coredns-xxx",
					Namespace: "kube-system",
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "coredns",
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "hosts",
									MountPath: "/etc/edge",
									ReadOnly:  true,
								},
								{
									Name:      "timezone",
									MountPath: "/etc/localtime",
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "hosts",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "edge-tunnel-nodes",
									},
								},
							},
						},
						{
							Name: "timezone",
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{
									Path: "/etc/localtime",
									Type: &hostPathType,
								},
							},
						},
					},
				},
			},
			expectObject: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "coredns-xxx",
					Namespace: "kube-system",
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "coredns",
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "timezone",
									MountPath: "/etc/localtime",
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "timezone",
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{
									Path: "/etc/localtime",
									Type: &hostPathType,
								},
							},
						},
					},
				},
			},
		},
		"kube-proxy pod with multiple volumes": {
			responseObject: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "kube-proxy-xxx",
					Namespace: "kube-system",
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "coredns",
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "hosts",
									MountPath: "/etc/edge",
									ReadOnly:  true,
								},
								{
									Name:      "timezone",
									MountPath: "/etc/localtime",
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "hosts",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "edge-tunnel-nodes",
									},
								},
							},
						},
						{
							Name: "timezone",
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{
									Path: "/etc/localtime",
									Type: &hostPathType,
								},
							},
						},
					},
				},
			},
			expectObject: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "kube-proxy-xxx",
					Namespace: "kube-system",
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "coredns",
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "hosts",
									MountPath: "/etc/edge",
									ReadOnly:  true,
								},
								{
									Name:      "timezone",
									MountPath: "/etc/localtime",
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "hosts",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "edge-tunnel-nodes",
									},
								},
							},
						},
						{
							Name: "timezone",
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{
									Path: "/etc/localtime",
									Type: &hostPathType,
								},
							},
						},
					},
				},
			},
		},
	}

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			tcvf, _ := NewTrimCorednsVolumeFilter()
			stopCh := make(<-chan struct{})
			newObj := tcvf.Filter(tc.responseObject, stopCh)
			if !reflect.DeepEqual(newObj, tc.expectObject) {
				t.Errorf("TrimCorednsVolumeFilter expect: \n%#+v\nbut got: \n%#+v\n", tc.expectObject, newObj)
			}
		})
	}
}
