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

package cloudnodepoollifecycle

import (
	"context"
	"flag"
	"fmt"

	"github.com/go-errors/errors"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
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
	"github.com/openyurtio/openyurt/pkg/apis/apps/v1beta1"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/cloudnodepoollifecycle/config"
)

func init() {
	flag.IntVar(&concurrentReconciles, "cloudnodepoollifecycle-workers", concurrentReconciles, "Max concurrent workers for Cloudnodepoollifecycle controller.")
}

var (
	concurrentReconciles = 3
	controllerResource   = v1beta1.SchemeGroupVersion.WithResource("nodepools")
)

var (
	cloudNodeDefaultLabels = map[string]string{
		"alibabacloud.com/cloud-worker-nodes":           "tools",
		"alibabacloud.com/edge-enable-addon-coredns":    "true",
		"alibabacloud.com/edge-enable-addon-flannel":    "true",
		"alibabacloud.com/edge-enable-addon-kube-proxy": "true",
		"alibabacloud.com/is-edge-worker":               "false",
	}
)

const (
	cloudNodepoolLabelKey            = "alibabacloud.com/nodepool-id"
	poolNodesConnectedAnnotationsKey = "alibabacloud.com/pool-nodes-connected"
)

func Format(format string, args ...interface{}) string {
	s := fmt.Sprintf(format, args...)
	return fmt.Sprintf("%s: %s", names.CloudNodepoolLifecycleController, s)
}

// Add creates a new Cloudnodepoollifecycle Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(ctx context.Context, cfg *appconfig.CompletedConfig, mgr manager.Manager) error {
	r := newReconciler(cfg, mgr)

	if _, err := r.mapper.KindFor(controllerResource); err != nil {
		return errors.Errorf("resource %s isn't exist", controllerResource.String())

	}

	ctrl, err := controller.New(names.CloudNodepoolLifecycleController, mgr, controller.Options{
		Reconciler: r, MaxConcurrentReconciles: concurrentReconciles,
	})
	if err != nil {
		return errors.Errorf("failed to new %s controller, error: %v", names.CloudNodepoolLifecycleController, err)
	}

	err = ctrl.Watch(&source.Kind{Type: &v1beta1.NodePool{}}, &handler.EnqueueRequestForObject{}, newPredNodepool())
	if err != nil {
		return errors.Errorf("failed to watch nodepool, error: %v", err)
	}

	err = ctrl.Watch(&source.Kind{Type: &v1.Node{}}, &EnqueueRequestNode{})
	if err != nil {
		return errors.Errorf("failed to watch node, error: %v", err)
	}

	return nil
}

func newPredNodepool() predicate.Funcs {
	return predicate.Funcs{
		// Create returns true if the Create event should be processed
		CreateFunc: func(e event.CreateEvent) bool {
			return filterNodepool(e.Object)
		},

		// Delete returns true if the Delete event should be processed
		DeleteFunc: func(e event.DeleteEvent) bool {
			return filterNodepool(e.Object)
		},

		// Update returns true if the Update event should be processed
		UpdateFunc: func(e event.UpdateEvent) bool {
			return filterNodepool(e.ObjectNew)
		},

		// Generic returns true if the Generic event should be processed
		GenericFunc: func(e event.GenericEvent) bool {
			return filterNodepool(e.Object)
		},
	}
}

func filterNodepool(o client.Object) bool {
	np, ok := o.(*v1beta1.NodePool)
	if !ok {
		return false
	}
	return np.Spec.Type == v1beta1.Cloud
}

var _ reconcile.Reconciler = &ReconcileCloudNodepoolLifecycle{}

// ReconcileCloudnodepoollifecycle reconciles a Cloudnodepoollifecycle object
type ReconcileCloudNodepoolLifecycle struct {
	client.Client
	mapper       meta.RESTMapper
	Configration config.CloudNodepoolLifeCycleControllerConfiguration
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(c *appconfig.CompletedConfig, mgr manager.Manager) *ReconcileCloudNodepoolLifecycle {
	return &ReconcileCloudNodepoolLifecycle{
		Client:       mgr.GetClient(),
		mapper:       mgr.GetRESTMapper(),
		Configration: c.ComponentConfig.CloudNodepoolLifeCycleController,
	}
}

// Reconcile reads that state of the cluster for a Cloudnodepoollifecycle object and makes changes based on the state read
func (r *ReconcileCloudNodepoolLifecycle) Reconcile(_ context.Context, request reconcile.Request) (reconcile.Result, error) {
	klog.Infof(Format("Reconcile CloudNodepoolLifecycle %s/%s", request.Namespace, request.Name))

	if err := r.checkCreateOrDeleteNodepool(request.Name); err != nil {
		return reconcile.Result{}, errors.Errorf("failed to check to create or delete nodepool, error: %v", err)
	}

	if err := r.ensureLabelsWithAllCloudNodes(request.Name); err != nil {
		return reconcile.Result{}, errors.Errorf("failed to ensure labels with all cloud nodes, error: %v", err)
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileCloudNodepoolLifecycle) checkCreateOrDeleteNodepool(nodepoolName string) error {
	needCreate, needDelete, err := r.isCreateOrDeleteNodepool(nodepoolName)
	if err != nil {
		return errors.Errorf("failed to judge create or delete nodepool %s, error: %v", nodepoolName, err)
	}

	if needCreate {
		if err := r.Create(context.Background(), newCloudNodepool(nodepoolName)); err != nil {
			return errors.Errorf("failed to create nodepool %v, error: %v", nodepoolName, err)
		}
	}

	if needDelete {
		if err := r.Delete(context.Background(), newCloudNodepool(nodepoolName)); err != nil {
			return errors.Errorf("failed to delete nodepool %s, error: %v", nodepoolName, err)
		}
	}
	return nil
}

func (r *ReconcileCloudNodepoolLifecycle) isCreateOrDeleteNodepool(nodepoolName string) (needCreate bool, needDelete bool, err error) {
	hasCloudNodes, err := r.hasCloudNodesInNodepool(nodepoolName)
	if err != nil {
		return false, false, errors.Errorf("failed to judge cloud nodes exist with nodepool %s, error: %v", nodepoolName, err)
	}

	exist, err := r.isExistNodepool(nodepoolName)
	if err != nil {
		return false, false, errors.Errorf("failed to judge nodepool %s is exist, error: %v", nodepoolName, err)
	}

	if hasCloudNodes && !exist {
		return true, false, nil
	}

	if !hasCloudNodes && exist {
		return false, true, nil
	}

	return false, false, nil
}

func (r *ReconcileCloudNodepoolLifecycle) hasCloudNodesInNodepool(nodepoolName string) (bool, error) {
	selector := client.MatchingLabels{cloudNodepoolLabelKey: nodepoolName}
	nodeList := &v1.NodeList{}
	if err := r.List(context.Background(), nodeList, selector); err != nil {
		return false, errors.Errorf("failed to list node with labels %v, error: %v", selector, err)
	}
	return len(nodeList.Items) != 0, nil
}

func (r *ReconcileCloudNodepoolLifecycle) isExistNodepool(nodepoolName string) (bool, error) {
	np := &v1beta1.NodePool{}
	err := r.Get(context.Background(), types.NamespacedName{Name: nodepoolName}, np)
	if err == nil {
		return true, nil
	}

	return false, client.IgnoreNotFound(err)
}

func newCloudNodepool(nodepoolName string) *v1beta1.NodePool {
	return &v1beta1.NodePool{
		ObjectMeta: metav1.ObjectMeta{
			Name:        nodepoolName,
			Annotations: map[string]string{poolNodesConnectedAnnotationsKey: "true"},
		},
		Spec: v1beta1.NodePoolSpec{
			Type:        v1beta1.Cloud,
			HostNetwork: false,
		},
	}
}

func (r *ReconcileCloudNodepoolLifecycle) ensureLabelsWithAllCloudNodes(nodepoolName string) error {
	selector := client.MatchingLabels{cloudNodepoolLabelKey: nodepoolName}
	nodeList := &v1.NodeList{}

	if err := r.List(context.Background(), nodeList, selector); err != nil {
		return errors.Errorf("failed to list node with labels: %v, error: %s", selector, err)
	}

	for _, node := range nodeList.Items {
		if err := r.labelsForCloudNode(&node); err != nil {
			return errors.Errorf("failed to ensure labels for cloud node %s, error: %v", node.Name, err)
		}
	}
	return nil
}

func (r *ReconcileCloudNodepoolLifecycle) labelsForCloudNode(node *v1.Node) error {
	if node.Labels == nil {
		node.Labels = make(map[string]string)
	}

	if r.isExistDefaultCloudLabels(node.Labels) {
		return nil
	}

	for key, value := range cloudNodeDefaultLabels {
		node.Labels[key] = value
	}

	if err := r.Update(context.Background(), node, &client.UpdateOptions{}); err != nil {
		return err
	}

	return nil
}

func (r *ReconcileCloudNodepoolLifecycle) isExistDefaultCloudLabels(nodeLabels map[string]string) bool {
	for key, value := range cloudNodeDefaultLabels {
		if v := nodeLabels[key]; v != value {
			return false
		}
	}
	return true
}
