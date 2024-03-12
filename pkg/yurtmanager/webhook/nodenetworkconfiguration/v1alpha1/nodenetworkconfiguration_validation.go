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
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/openyurtio/openyurt/pkg/apis/network/v1alpha1"
)

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *NodeNetworkConfigurationHandler) ValidateCreate(ctx context.Context, obj runtime.Object) error {
	return nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *NodeNetworkConfigurationHandler) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) error {
	return nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *NodeNetworkConfigurationHandler) ValidateDelete(ctx context.Context, obj runtime.Object) error {
	nnc, ok := obj.(*v1alpha1.NodeNetworkConfiguration)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a NodeNetworkConfiguration but got a %T", obj))
	}
	var node v1.Node
	err := webhook.Client.Get(ctx, types.NamespacedName{Name: nnc.Name}, &node)
	if apierrors.IsNotFound(err) {
		klog.Infof("node %s has been deleted, can delete nodenetworkconfiguration", nnc.GetName())
		return nil
	}
	return fmt.Errorf("node %s is not deleted, can not delete nodenetworkconfiguration", nnc.GetName())
}
