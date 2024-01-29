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

package cloudnodeslabels

import (
	"context"
	"flag"
	"fmt"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	appconfig "github.com/openyurtio/openyurt/cmd/yurt-manager/app/config"
	"github.com/openyurtio/openyurt/cmd/yurt-manager/names"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/cloudnodeslabels/config"
)

func init() {
	flag.IntVar(&concurrentReconciles, "cloudnodeslabels-workers", concurrentReconciles, "Max concurrent workers for Cloudnodeslabels controller.")
}

var (
	concurrentReconciles = 3
)

var cloudNodeDefaultLabels = map[string]string{
	"alibabacloud.com/is-edge-worker": "false",
}

func Format(format string, args ...interface{}) string {
	s := fmt.Sprintf(format, args...)
	return fmt.Sprintf("%s: %s", names.CloudNodesLabelsController, s)
}

// Add creates a new Cloudnodeslabels Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(ctx context.Context, c *appconfig.CompletedConfig, mgr manager.Manager) error {
	klog.Infof(Format("cloudnodeslabels-controller add controller"))
	return add(mgr, newReconciler(c, mgr))
}

var _ reconcile.Reconciler = &ReconcileCloudNodesLabels{}

// ReconcileCloudnodeslabels reconciles a Cloudnodeslabels object
type ReconcileCloudNodesLabels struct {
	client.Client
	scheme       *runtime.Scheme
	recorder     record.EventRecorder
	Configration config.CloudNodesLabelsControllerConfiguration
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(c *appconfig.CompletedConfig, mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileCloudNodesLabels{
		Client:       mgr.GetClient(),
		scheme:       mgr.GetScheme(),
		recorder:     mgr.GetEventRecorderFor(names.CloudNodesLabelsController),
		Configration: c.ComponentConfig.CloudNodesLabelsController,
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New(names.CloudNodesLabelsController, mgr, controller.Options{
		Reconciler: r, MaxConcurrentReconciles: concurrentReconciles,
	})
	if err != nil {
		return err
	}

	// Watch for changes to Cloudnodeslabels
	err = c.Watch(&source.Kind{Type: &corev1.Node{}}, &handler.EnqueueRequestForObject{}, newPredNode())
	if err != nil {
		return err
	}

	return nil
}

func newPredNode() predicate.Funcs {
	return predicate.Funcs{
		// Create returns true if the Create event should be processed
		CreateFunc: func(e event.CreateEvent) bool {
			return filterCloudNode(e.Object)
		},

		// Delete returns true if the Delete event should be processed
		DeleteFunc: func(e event.DeleteEvent) bool {
			return false
		},

		// Update returns true if the Update event should be processed
		UpdateFunc: func(e event.UpdateEvent) bool {
			return filterCloudNode(e.ObjectNew)
		},

		// Generic returns true if the Generic event should be processed
		GenericFunc: func(e event.GenericEvent) bool {
			return filterCloudNode(e.Object)
		},
	}
}

func filterCloudNode(o client.Object) bool {
	node, ok := o.(*corev1.Node)
	if !ok {
		return false
	}

	if node.Spec.ProviderID == "" {
		return false
	}

	if isExistDefaultCloudLabels(node.Labels) {
		return false
	}

	return true
}

func isExistDefaultCloudLabels(nodeLabels map[string]string) bool {
	if nodeLabels == nil {
		return false
	}

	for key, value := range cloudNodeDefaultLabels {
		if v := nodeLabels[key]; v != value {
			return false
		}
	}
	return true
}

func (r *ReconcileCloudNodesLabels) Reconcile(_ context.Context, request reconcile.Request) (reconcile.Result, error) {
	klog.Infof(Format("Reconcile cloud node %s/%s", request.Namespace, request.Name))

	node := &corev1.Node{}
	err := r.Client.Get(context.Background(), request.NamespacedName, node)

	if apierrors.IsNotFound(err) {
		return reconcile.Result{}, nil
	}
	if err != nil {
		return reconcile.Result{}, errors.Wrapf(err, "failed to get node %s", request.Name)
	}

	if node.Spec.ProviderID == "" {
		return reconcile.Result{}, nil
	}

	if err := r.ensureCloudNodeDefaultLabels(node); err != nil {
		return reconcile.Result{}, errors.Wrapf(err, "failed to ensure cloud node default label")
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileCloudNodesLabels) ensureCloudNodeDefaultLabels(node *corev1.Node) error {

	if isExistDefaultCloudLabels(node.Labels) {
		return nil
	}

	if err := r.labelsForCloudNode(node); err != nil {
		return errors.Wrapf(err, "failed to label node %s", node.Name)
	}
	return nil
}

func (r *ReconcileCloudNodesLabels) labelsForCloudNode(node *corev1.Node) error {
	if node.Labels == nil {
		node.Labels = make(map[string]string)
	}

	for key, value := range cloudNodeDefaultLabels {
		node.Labels[key] = value
	}

	err := r.Update(context.Background(), node, &client.UpdateOptions{})

	return client.IgnoreNotFound(err)
}
