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
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apivalidation "k8s.io/apimachinery/pkg/api/validation"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openyurtio/openyurt/pkg/apis/apps/v1alpha1"
)

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *YurtAppConfigurationReplacementHandler) ValidateCreate(ctx context.Context, obj runtime.Object) error {
	np, ok := obj.(*v1alpha1.YurtAppConfigurationReplacement)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a YurtAppConfigurationReplacement but got a %T", obj))
	}

	//validate

	return nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *YurtAppConfigurationReplacementHandler) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) error {
	newNp, ok := newObj.(*v1alpha1.YurtAppConfigurationReplacement)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a YurtAppConfigurationReplacement but got a %T", newObj))
	}
	oldNp, ok := oldObj.(*v1alpha1.YurtAppConfigurationReplacement)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a YurtAppConfigurationReplacement} but got a %T", oldObj))
	}

	// validate
	return nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *YurtAppConfigurationReplacementHandler) ValidateDelete(_ context.Context, obj runtime.Object) error {
	np, ok := obj.(*v1alpha1.YurtAppConfigurationReplacement)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a YurtAppConfigurationReplacement but got a %T", obj))
	}
	// validate
	return nil
}
