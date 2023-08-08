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

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openyurtio/openyurt/pkg/apis/apps/v1alpha1"
)

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *YurtAppConfigRenderHandler) ValidateCreate(ctx context.Context, obj runtime.Object) error {
	configRender, ok := obj.(*v1alpha1.YurtAppConfigRender)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a YurtAppConfigRender but got a %T", obj))
	}

	// validate
	if err := webhook.validateOneToOne(ctx, configRender); err != nil {
		return err
	}
	if err := webhook.validateStar(configRender); err != nil {
		return err
	}
	return nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *YurtAppConfigRenderHandler) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) error {
	_, ok := newObj.(*v1alpha1.YurtAppConfigRender)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a YurtAppConfigRender but got a %T", newObj))
	}
	newConfigRender, ok := oldObj.(*v1alpha1.YurtAppConfigRender)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a YurtAppConfigRender} but got a %T", oldObj))
	}

	// validate
	if err := webhook.validateOneToOne(ctx, newConfigRender); err != nil {
		return err
	}
	if err := webhook.validateStar(newConfigRender); err != nil {
		return err
	}
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

// YurtConfigRender and YurtAppSet are one-to-one relationship
func (webhook *YurtAppConfigRenderHandler) validateOneToOne(ctx context.Context, configRender *v1alpha1.YurtAppConfigRender) error {
	app := configRender.Subject
	var configRenderList v1alpha1.YurtAppConfigRenderList
	listOptions := client.MatchingFields{"subject.Kind": app.Kind, "subject.Name": app.Name, "subject.APIVersion": app.APIVersion}
	if err := webhook.Client.List(ctx, &configRenderList, client.InNamespace(configRender.Namespace), listOptions); err != nil {
		klog.Info("error in listing YurtAppConfigRender")
		return err
	}
	if len(configRenderList.Items) > 0 {
		return fmt.Errorf("only one YurtAppConfigRender can be bound into one YurtAppSet")
	}
	return nil
}

// Verify that * and other pools are not set at the same time
func (webhook *YurtAppConfigRenderHandler) validateStar(configRender *v1alpha1.YurtAppConfigRender) error {
	for _, entry := range configRender.Entries {
		for _, pool := range entry.Pools {
			if pool == "*" && len(entry.Pools) > 1 {
				return fmt.Errorf("pool can't be '*' when other pools are set")
			}
		}
	}
	return nil
}
