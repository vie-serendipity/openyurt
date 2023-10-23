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
	"fmt"
	unitv1alpha1 "github.com/openyurtio/openyurt/pkg/apis/apps/v1alpha1"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/openyurtio/openyurt/pkg/apis/apps/v1alpha1"
)

// Default satisfies the defaulting webhook interface.
func (webhook *UnitedDeploymentHandler) Default(ctx context.Context, obj runtime.Object) error {
	ud, ok := obj.(*v1alpha1.UnitedDeployment)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a UnitedDeployment but got a %T", obj))
	}

	v1alpha1.SetDefaultsUnitedDeployment(ud)

	ud.Status = unitv1alpha1.UnitedDeploymentStatus{}

	statefulSetTemp := ud.Spec.WorkloadTemplate.StatefulSetTemplate
	deployTem := ud.Spec.WorkloadTemplate.DeploymentTemplate

	if statefulSetTemp != nil {
		statefulSetTemp.Spec.Selector = ud.Spec.Selector
	}
	if deployTem != nil {
		deployTem.Spec.Selector = ud.Spec.Selector
	}

	return nil
}
