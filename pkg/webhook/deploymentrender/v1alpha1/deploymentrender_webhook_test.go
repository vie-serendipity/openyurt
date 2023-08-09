/*
Copyright 2023 The OpenYurt Authors.

Licensed under the Apache License, Version 2.0 (the License);
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an AS IS BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package v1alpha1

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/openyurtio/openyurt/pkg/apis/apps/v1alpha1"
)

var (
	replica int32 = 3
)

var defaultAppSet = &v1alpha1.YurtAppSet{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "yurtappset-patch",
		Namespace: "default",
	},
	Spec: v1alpha1.YurtAppSetSpec{
		Topology: v1alpha1.Topology{
			Pools: []v1alpha1.Pool{{
				Name:     "nodepool-test",
				Replicas: &replica}},
		},
		Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "test"}},
		WorkloadTemplate: v1alpha1.WorkloadTemplate{
			DeploymentTemplate: &v1alpha1.DeploymentTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": "test"},
				},
				Spec: appsv1.DeploymentSpec{
					Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "test"}},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{"app": "test"},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{Name: "nginx", Image: "nginx"},
							},
						},
					},
				},
			},
		},
	},
}

var defaultDeployment = &appsv1.Deployment{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "test",
		Namespace: "default",
		OwnerReferences: []metav1.OwnerReference{{
			APIVersion: "apps.openyurt.io/v1alpha1",
			Kind:       "YurtAppSet",
			Name:       "yurtappset-patch",
		}},
		Labels: map[string]string{
			"apps.openyurt.io/pool-name": "nodepool-test",
		},
	},
	Status: appsv1.DeploymentStatus{},
	Spec: appsv1.DeploymentSpec{
		Replicas: &replica,
		Selector: &metav1.LabelSelector{
			MatchLabels: map[string]string{
				"app": "test",
			},
		},
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{
					"app": "test",
				},
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name:  "nginx",
						Image: "nginx",
					},
				},
				Volumes: []corev1.Volume{
					{
						Name: "config",
						VolumeSource: corev1.VolumeSource{
							ConfigMap: &corev1.ConfigMapVolumeSource{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: "configMapSource-nodepool-test",
								},
							},
						},
					},
				},
			},
		},
	},
}

var defaultConfigRender = &v1alpha1.YurtAppConfigRender{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "demo",
		Namespace: "default",
	},
	Spec: v1alpha1.YurtAppConfigRenderSpec{
		Subject: v1alpha1.Subject{
			Name: "foo",
		},
		Entries: []v1alpha1.Entry{
			{
				Pools: []string{"*"},
				Items: []v1alpha1.Item{
					{
						ConfigMap: &v1alpha1.ConfigMapItem{
							ConfigMapSource: "configMapSource-{{nodepool}}",
							ConfigMapTarget: "configMapTarget-{{nodepool}}",
						},
					},
				},
			},
			{
				Pools: []string{"nodepool-test"},
				Items: []v1alpha1.Item{
					{
						Image: &v1alpha1.ImageItem{
							ContainerName: "nginx",
							ImageClaim:    "nginx:1.18",
						},
					},
				},
				Patches: []v1alpha1.Patch{
					{
						Type:       v1alpha1.Default,
						Extensions: &runtime.RawExtension{Raw: []byte(`{"spec":{"replicas":3,"template":{"spec":{"containers":[{"image":"nginx:2.20","name":"nginx"}]}}}}`)},
					},
				},
			},
		},
	},
}

func TestDeploymentRenderHandler_Default(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Logf("failed to add yurt custom resource")
		return
	}
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Logf("failed to add kubernetes clint-go custom resource")
		return
	}
	webhook := &DeploymentRenderHandler{
		Client: fakeclient.NewClientBuilder().WithScheme(scheme).WithObjects(defaultAppSet, defaultDeployment, defaultConfigRender).Build(),
	}
	if err := webhook.Default(context.TODO(), defaultDeployment); err != nil {
		t.Fatal(err)
	}
	for _, volume := range defaultDeployment.Spec.Template.Spec.Volumes {
		if volume.VolumeSource.ConfigMap != nil {
			if volume.VolumeSource.ConfigMap.Name != "configMapTarget-nodepool-test" {
				t.Fatalf("fail to update configMap")
			}
		}
	}
}
