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

	"github.com/openyurtio/openyurt/pkg/apis/apps/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
)

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *YurtAppConfigRenderHandler) ValidateCreate(ctx context.Context, obj runtime.Object) error {
	_, ok := obj.(*v1alpha1.YurtAppConfigRender)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a YurtAppConfigRender but got a %T", obj))
	}

	//validate

	return nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *YurtAppConfigRenderHandler) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) error {
	_, ok := newObj.(*v1alpha1.YurtAppConfigRender)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a YurtAppConfigRender but got a %T", newObj))
	}
	_, ok = oldObj.(*v1alpha1.YurtAppConfigRender)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a YurtAppConfigRender} but got a %T", oldObj))
	}

	// validate
	return nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *YurtAppConfigRenderHandler) ValidateDelete(_ context.Context, obj runtime.Object) error {
	_, ok := obj.(*v1alpha1.YurtAppConfigRender)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a YurtAppConfigRender but got a %T", obj))
	}
	// validate
	return nil
}
