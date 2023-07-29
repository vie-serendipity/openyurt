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

package yurtappconfigrender

import (
	"context"
	"flag"
	"fmt"

	v1 "k8s.io/api/apps/v1"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	appconfig "github.com/openyurtio/openyurt/cmd/yurt-manager/app/config"
	appsv1alpha1 "github.com/openyurtio/openyurt/pkg/apis/apps/v1alpha1"
	"github.com/openyurtio/openyurt/pkg/controller/yurtappconfigrender/config"
	utilclient "github.com/openyurtio/openyurt/pkg/util/client"
	utildiscovery "github.com/openyurtio/openyurt/pkg/util/discovery"
)

func init() {
	flag.IntVar(&concurrentReconciles, "yurtappconfigrender-workers", concurrentReconciles, "Max concurrent workers for YurtAppConfigRender controller.")
}

var (
	concurrentReconciles = 3
	controllerKind       = appsv1alpha1.SchemeGroupVersion.WithKind("YurtAppConfigRender")
)

const (
	ControllerName = "YurtAppConfigRender-controller"
)

func Format(format string, args ...interface{}) string {
	s := fmt.Sprintf(format, args...)
	return fmt.Sprintf("%s: %s", ControllerName, s)
}

// Add creates a new YurtAppConfigRender Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(c *appconfig.CompletedConfig, mgr manager.Manager) error {
	if !utildiscovery.DiscoverGVK(controllerKind) {
		return nil
	}
	return add(mgr, newReconciler(c, mgr))
}

var _ reconcile.Reconciler = &ReconcileYurtAppConfigRender{}

// ReconcileYurtAppConfigRender reconciles a YurtAppConfigRender object
type ReconcileYurtAppConfigRender struct {
	client.Client
	scheme       *runtime.Scheme
	recorder     record.EventRecorder
	Configration config.YurtAppConfigRenderControllerConfiguration
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(c *appconfig.CompletedConfig, mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileYurtAppConfigRender{
		Client:   utilclient.NewClientFromManager(mgr, ControllerName),
		scheme:   mgr.GetScheme(),
		recorder: mgr.GetEventRecorderFor(ControllerName),
		//Configration: c.ComponentConfig.YurtAppConfigRenderController,
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New(ControllerName, mgr, controller.Options{
		Reconciler: r, MaxConcurrentReconciles: concurrentReconciles,
	})
	if err != nil {
		return err
	}

	// Watch for changes to YurtAppConfigRender
	err = c.Watch(&source.Kind{Type: &appsv1alpha1.YurtAppConfigRender{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	return nil
}

// +kubebuilder:rbac:groups=apps.openyurt.io,resources=YurtAppConfigRenders,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps.openyurt.io,resources=YurtAppConfigRenders/status,verbs=get;update;patch

// Reconcile reads that state of the cluster for a YurtAppConfigRender object and makes changes based on the state read
// and what is in the YurtAppConfigRender.Spec
func (r *ReconcileYurtAppConfigRender) Reconcile(_ context.Context, request reconcile.Request) (reconcile.Result, error) {

	// Note !!!!!!!!!!
	// We strongly recommend use Format() to  encapsulation because Format() can print logs by module
	// @kadisi
	klog.Infof(Format("Reconcile YurtAppConfigRender %s/%s", request.Namespace, request.Name))

	// Fetch the YurtAppConfigRender instance
	instance := &appsv1alpha1.YurtAppConfigRender{}
	err := r.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	if instance.DeletionTimestamp != nil {
		return reconcile.Result{}, nil
	}

	// Update Status
	//if instance.Spec.Foo != instance.Status.Foo {
	//	instance.Status.Foo = instance.Spec.Foo
	//	if err = r.Status().Update(context.TODO(), instance); err != nil {
	//		klog.Errorf(Format("Update YurtAppConfigRender Status %s error %v", klog.KObj(instance), err))
	//		return reconcile.Result{Requeue: true}, err
	//	}
	//}
	// Update Instance
	// Update Deployment
	pools := []string(nil)
	for _, entry := range instance.Entries {
		pools = append(pools, entry.Pools...)
	}
	//client.Patch()
	for _, pool := range pools {
		deployments := v1.DeploymentList{}
		listOptions := client.MatchingLabels{"apps.openyurt.io/pool-name": pool}
		r.List(context.TODO(), &deployments, listOptions)
		for _, deployment := range deployments.Items {
			deployment.Annotations["modify"] = "modified"
		}
	}

	//if err = r.Update(context.TODO(), instance); err != nil {
	//	klog.Errorf(Format("Update YurtAppConfigRender %s error %v", klog.KObj(instance), err))
	//	return reconcile.Result{Requeue: true}, err
	//}

	return reconcile.Result{}, nil
}
