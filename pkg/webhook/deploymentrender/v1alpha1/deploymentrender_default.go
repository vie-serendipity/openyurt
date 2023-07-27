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
	v1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	resources = []string{"YurtAppSet", "YurtAppDaemon"}
)

// Default satisfies the defaulting webhook interface.
func (webhook *DeploymentRenderHandler) Default(ctx context.Context, obj runtime.Object) error {

	deployment, ok := obj.(*v1.Deployment)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a YurtAppConfigRender but got a %T", obj))
	}
	if deployment.OwnerReferences == nil {
		return nil
	}
	for _, v := range resources {
		if deployment.OwnerReferences[0].Kind == v {
			return nil
		}
	}

	// Get YurtAppSet/YurtAppDaemon resource of this deployment
	app := deployment.OwnerReferences[0]
	var instance client.Object
	switch app.Kind {
	case "YurtAppSet":
		instance = &v1alpha1.YurtAppSet{}
	case "YurtAppDaemon":
		instance = &v1alpha1.YurtAppDaemon{}
	default:
		return nil
	}
	if err := webhook.Client.Get(ctx, client.ObjectKey{
		Namespace: deployment.Namespace,
		Name:      app.Name,
	}, instance); err != nil {
		return err
	}

	// Get YurtAppConfigRender resource of app(1 to 1)
	var configRenderList v1alpha1.YurtAppConfigRenderList
	listOptions := client.MatchingFields{"subject.Kind": app.Kind, "subject.Name": app.Name, "subject.APIVersion": app.APIVersion}
	if err := webhook.Client.List(ctx, &configRenderList, client.InNamespace(deployment.Namespace), listOptions); err != nil {
		return err
	}
	render := configRenderList.Items[0]

	// Get nodepool of deployment
	nodepool := deployment.Labels["apps.openyurt.io/pool-name"]

	for _, entry := range render.Entries {
		pools := entry.Pools
		for _, pool := range pools {
			if pool == nodepool {
				// Get the corresponding config  of this deployment
				// reference to the volumeSource implementation
				items := entry.Items
				replaceItems(items)
				// json patch and strategic merge
				patches := entry.Patches
				// Implement injection
				updatePatches(patches)
			}
		}
	}
	return nil
}

func updatePatches(patches []v1alpha1.Patch) {

}

func replaceItems(items []v1alpha1.Item) {

}
