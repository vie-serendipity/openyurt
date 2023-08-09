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
	"github.com/openyurtio/yurt-app-manager-api/pkg/yurtappmanager/apis/apps/v1alpha1"
)

const updateRetries = 5

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

// +kubebuilder:rbac:groups=apps.openyurt.io,resources=yurtappconfigrenders,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=deployments/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core,resources=events,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=controllerrevisions,verbs=get;list;watch;create;update;patch;delete

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
	oldStatus := instance.Status.DeepCopy()

	currentRevision, updatedRevision, collisionCount, err := r.constructYurtAppConfigRenderRevisions(instance)
	if err != nil {
		klog.Errorf("Fail to construct controller revision of YurtAppConfigRender %s/%s: %s", instance.Namespace, instance.Name, err)
		return reconcile.Result{}, err
	}

	expectedRevision := currentRevision
	if updatedRevision != nil {
		expectedRevision = updatedRevision
	}

	newStatus, err := r.updatePools(instance, expectedRevision.Name)
	if err != nil {
		klog.Errorf("Fail to update YurtAppConfigRender %s/%s: %s", instance.Namespace, instance.Name, err)
	}

	return r.updateStatus(instance, newStatus, oldStatus, currentRevision, collisionCount)
}

func (r *ReconcileYurtAppConfigRender) updateStatus(instance *appsv1alpha1.YurtAppConfigRender, newStatus, oldStatus *appsv1alpha1.YurtAppConfigRenderStatus,
	currentRevision *v1.ControllerRevision,
	collisionCount int32) (reconcile.Result, error) {

	newStatus = r.calculateStatus(newStatus, currentRevision, collisionCount)
	_, err := r.updateYurtAppConfigRender(instance, oldStatus, newStatus)

	return reconcile.Result{}, err
}

func (r *ReconcileYurtAppConfigRender) updatePools(yacr *appsv1alpha1.YurtAppConfigRender, revision string) (newStatus *appsv1alpha1.YurtAppConfigRenderStatus, updateErr error) {
	newStatus = yacr.Status.DeepCopy()

	pools := []string(nil)
	for _, entry := range yacr.Spec.Entries {
		pools = append(pools, entry.Pools...)
	}

	for _, pool := range pools {
		deployments := v1.DeploymentList{}
		listOptions := client.MatchingLabels{"apps.openyurt.io/pool-name": pool}
		updateErr = r.List(context.TODO(), &deployments, listOptions)
		for _, deployment := range deployments.Items {
			deployment.Annotations["resourceVersion"] = deployment.ResourceVersion
			deployment.Labels[v1alpha1.ControllerRevisionHashLabelKey] = revision
			updateErr = r.Update(context.TODO(), &deployment)
		}
	}
	return
}

func (r *ReconcileYurtAppConfigRender) calculateStatus(newStatus *appsv1alpha1.YurtAppConfigRenderStatus,
	currentRevision *v1.ControllerRevision, collisionCount int32) *appsv1alpha1.YurtAppConfigRenderStatus {

	newStatus.CollisionCount = &collisionCount

	if newStatus.CurrentRevision == "" {
		// init with current revision
		newStatus.CurrentRevision = currentRevision.Name
	}

	return newStatus
}

func (r *ReconcileYurtAppConfigRender) updateYurtAppConfigRender(yacr *appsv1alpha1.YurtAppConfigRender, oldStatus, newStatus *appsv1alpha1.YurtAppConfigRenderStatus) (*appsv1alpha1.YurtAppConfigRender, error) {
	if oldStatus.CurrentRevision == newStatus.CurrentRevision &&
		oldStatus.CollisionCount == newStatus.CollisionCount &&
		yacr.Generation == newStatus.ObservedGeneration {
		return yacr, nil
	}
	newStatus.ObservedGeneration = yacr.Generation

	var getErr, updateErr error
	for i, obj := 0, yacr; ; i++ {
		klog.V(4).Infof(fmt.Sprintf("The %d th time updating status for %v: %s/%s, ", i, obj.Kind, obj.Namespace, obj.Name) +
			fmt.Sprintf("sequence No: %v->%v", obj.Status.ObservedGeneration, newStatus.ObservedGeneration))

		obj.Status = *newStatus

		updateErr = r.Client.Status().Update(context.TODO(), obj)
		if updateErr == nil {
			return obj, nil
		}
		if i >= updateRetries {
			break
		}
		tmpObj := &appsv1alpha1.YurtAppConfigRender{}
		if getErr = r.Client.Get(context.TODO(), client.ObjectKey{Namespace: obj.Namespace, Name: obj.Name}, tmpObj); getErr != nil {
			return nil, getErr
		}
		obj = tmpObj
	}

	klog.Errorf("fail to update YurtAppConfigRender %s/%s status: %s", yacr.Namespace, yacr.Name, updateErr)
	return nil, updateErr
}
