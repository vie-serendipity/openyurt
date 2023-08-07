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
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/openyurtio/openyurt/pkg/apis/apps/v1alpha1"
)

var (
	replica int32 = 3
)

var defaultAppSet = &v1alpha1.YurtAppSet{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "foo",
		Namespace: "default",
	},
	Spec: v1alpha1.YurtAppSetSpec{
		Topology: v1alpha1.Topology{Pools: []v1alpha1.Pool{{Name: "nodepool-test"}}},
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
			},
		},
	},
}

var defaultConfigRender = &v1alpha1.YurtAppConfigRender{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "demo",
		Namespace: "default",
	},
	Subject: v1alpha1.Subject{
		Name: "foo",
	},
}

func TestDeploymentRenderHandler_Default(t *testing.T) {
	webhook := &DeploymentRenderHandler{
		Client: fakeclient.NewClientBuilder().WithObjects(defaultAppSet, defaultDeployment, defaultConfigRender).Build(),
	}
	if err := webhook.Default(context.TODO(), defaultDeployment); err != nil {
		t.Fatal(err)
	}
}
