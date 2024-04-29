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

package route

import (
	"context"
	"fmt"
	"k8s.io/apimachinery/pkg/util/wait"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"
	kubeconsts "k8s.io/kubernetes/cmd/kubeadm/app/constants"
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
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/route/config"
	prvd "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/util/cloudprovider"
	routemodel "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/util/cloudprovider/model/route"
	nodeutil "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/util/node"
)

const (
	GatewayNode = "node-role.alibabacloud.com/cloud-gateway"
	NodeTypeKey = "alibabacloud.com/is-edge-worker"
)

func Format(format string, args ...interface{}) string {
	s := fmt.Sprintf(format, args...)
	return fmt.Sprintf("%s: %s", names.EdgeRouteController, s)
}

// Add creates a new Route Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(ctx context.Context, c *appconfig.CompletedConfig, mgr manager.Manager) error {
	r, err := newReconciler(c, mgr)
	if err != nil {
		klog.Error(Format("new reconcile error: %s", err.Error()))
		return err
	}
	return add(mgr, r)
}

var _ reconcile.Reconciler = &ReconcileRoute{}

// ReconcileRoute reconciles a Route object
type ReconcileRoute struct {
	client.Client
	scheme   *runtime.Scheme
	provider prvd.Provider
	recorder record.EventRecorder
	cfg      config.RouteControllerConfiguration
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(c *appconfig.CompletedConfig, mgr manager.Manager) (*ReconcileRoute, error) {
	if c.Config.ComponentConfig.Generic.CloudProvider == nil {
		klog.Error(Format("can not get alibaba provider"))
		return nil, fmt.Errorf("can not get alibaba provider")
	}
	if c.Config.ComponentConfig.EdgeRouteController.ClusterCIDR == "" {
		klog.Error(Format("can not get cluster cidr"))
		return nil, fmt.Errorf("can not get cluster cidr")
	}
	return &ReconcileRoute{
		Client:   mgr.GetClient(),
		scheme:   mgr.GetScheme(),
		recorder: mgr.GetEventRecorderFor(names.EdgeRouteController),
		provider: c.Config.ComponentConfig.Generic.CloudProvider,
		cfg:      c.ComponentConfig.EdgeRouteController,
	}, nil
}

type routeController struct {
	c     controller.Controller
	recon *ReconcileRoute
}

// Start() function will not be called until the resource lock is acquired
func (controller routeController) Start(ctx context.Context) error {
	if controller.recon.cfg.SyncRoutes {
		controller.recon.periodicalSync()
	}
	return controller.c.Start(ctx)
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r *ReconcileRoute) error {
	// Create a new controller

	c, err := controller.NewUnmanaged(names.EdgeRouteController, mgr, controller.Options{
		Reconciler:              r,
		MaxConcurrentReconciles: 1,
		RecoverPanic:            true,
	})
	if err != nil {
		return err
	}

	// Watch for changes to Route
	err = c.Watch(&source.Kind{Type: &corev1.Node{}}, &handler.EnqueueRequestForObject{},
		predicate.Funcs{
			CreateFunc: func(event event.CreateEvent) bool {
				node, ok := event.Object.(*corev1.Node)
				if ok {
					return isGatewayNode(node)
				}
				return false
			},
			UpdateFunc: func(event event.UpdateEvent) bool {
				newNode, ok1 := event.ObjectOld.(*corev1.Node)
				oldNode, ok2 := event.ObjectNew.(*corev1.Node)
				if ok1 && ok2 {
					return isGatewayNode(newNode) || isGatewayNode(oldNode)
				}
				return false
			},
			DeleteFunc: func(event event.DeleteEvent) bool {
				node, ok := event.Object.(*corev1.Node)
				if ok {
					return isGatewayNode(node)
				}
				return false
			},
		})
	if err != nil {
		return err
	}

	return mgr.Add(&routeController{c: c, recon: r})
}

// Reconcile reads that state of the cluster for a Route object and makes changes based on the state read
// and what is in the Route.Spec
func (r *ReconcileRoute) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	klog.Infof(Format("Start reconcile route for gateway node %s", request.Name))
	err := r.reconcile(ctx)
	if err != nil {
		return reconcile.Result{Requeue: true, RequeueAfter: 5 * time.Second}, err
	}
	return reconcile.Result{}, nil
}

func (r *ReconcileRoute) reconcile(ctx context.Context) error {
	tables, err := r.getRouteTables(ctx)
	if err != nil {
		klog.Error(Format("can not find vpc route tables, skip reconcile it, error %s", err.Error()))
		return err
	}
	gatewayNodes, err := r.findGatewayNodes(ctx)
	if err != nil {
		klog.Error(Format("can not find gateway node in cloud, error %s", err.Error()))
		return err
	}
	if len(gatewayNodes) == 0 {
		err = r.cleanupRoute(tables)
	} else {
		err = r.ensureRoute(tables, &gatewayNodes[0])
	}
	if err != nil {
		klog.Error(Format("reconcile route error %s", err.Error()))
		return err
	}
	return nil
}

func (r *ReconcileRoute) syncRoute() {
	tables, err := r.getRouteTables(context.TODO())
	if err != nil {
		klog.Error(Format("can not find vpc route tables, skip reconcile it, error %s", err.Error()))
		return
	}
	gatewayNodes, err := r.findGatewayNodes(context.TODO())
	if err != nil {
		klog.Error(Format("can not find gateway node in cloud, error %s", err.Error()))
		return
	}
	if len(gatewayNodes) == 0 {
		err = r.cleanupRoute(tables)
	} else {
		err = r.ensureRoute(tables, &gatewayNodes[0])
	}
	if err != nil {
		klog.Error(Format("reconcile route error %s", err.Error()))
		return
	}
	return
}

func (r *ReconcileRoute) periodicalSync() {
	go wait.Until(r.syncRoute, r.cfg.RouteReconciliationPeriod.Duration, wait.NeverStop)
}

func (r *ReconcileRoute) cleanupRoute(tables []string) error {
	var tableErr []error
	for _, table := range tables {
		remote, err := r.buildRemoteModel(table)
		if err != nil {
			return fmt.Errorf("build remote model error %s", err.Error())
		}
		for _, mdl := range remote {
			if mdl.Status == routemodel.StatusDeleting {
				continue
			}
			if mdl.Name == routemodel.DefaultName && mdl.Description == routemodel.DefaultDescription {
				err = r.provider.DeleteRoute(context.TODO(), mdl.Id, mdl.NextHotId)
				if err != nil {
					err = fmt.Errorf("delete route entry id [%s], destinationCIDR [%s], nextHotId [%s], error %s", mdl.Id, mdl.DestinationCIDR, mdl.NextHotId, err.Error())
					tableErr = append(tableErr, err)
					continue
				}
				klog.Infof("Delete route for %s with %s - %s successfully", table, mdl.NextHotId, mdl.DestinationCIDR)
			}
		}

	}
	if utilerrors.NewAggregate(tableErr) != nil {
		return fmt.Errorf("cleanup route error: %s", utilerrors.NewAggregate(tableErr).Error())
	}
	return nil
}

func (r *ReconcileRoute) ensureRoute(tables []string, node *corev1.Node) error {
	prvdId := node.Spec.ProviderID
	if prvdId == "" {
		klog.Warningf("node %s provider id is not exist, skip creating route", node.Name)
		return nil
	}
	_, instanceId, err := NodeFromProviderID(prvdId)
	if err != nil {
		return fmt.Errorf("invalid provide id: %v, err: %v", prvdId, err)
	}
	var tableErr []error
	for _, table := range tables {
		remoteModel, err := r.buildRemoteModel(table)
		if err != nil {
			return fmt.Errorf("build remote model error %s", err.Error())
		}
		tableErr = append(tableErr, r.applyRouteEntry(table, r.buildLocalModel(instanceId, node.Name), remoteModel))
	}
	if utilerrors.NewAggregate(tableErr) != nil {
		return fmt.Errorf("ensure route error: %s", utilerrors.NewAggregate(tableErr).Error())
	}
	return nil
}

func (r *ReconcileRoute) buildLocalModel(instanceId, nodeName string) *routemodel.Route {
	mdl := routemodel.NewRouteModel()
	mdl.NextHotId = instanceId
	mdl.DestinationCIDR = r.cfg.ClusterCIDR
	mdl.NextNodeName = nodeName
	return mdl
}

func (r *ReconcileRoute) buildRemoteModel(tableId string) ([]*routemodel.Route, error) {
	routes, err := r.provider.FindRoute(context.TODO(), tableId, r.cfg.ClusterCIDR)
	if err != nil {
		return nil, fmt.Errorf("find route entry tableId [%s], destinationCIDR [%s], error: %s", tableId, r.cfg.ClusterCIDR, err.Error())
	}
	return routes, nil
}

func (r *ReconcileRoute) applyRouteEntry(tableId string, local *routemodel.Route, remote []*routemodel.Route) error {
	addRoutes := []*routemodel.Route{local}

	deleteRoutes := make([]*routemodel.Route, 0)
	for _, r := range remote {
		if r.NextHotId == local.NextHotId && r.DestinationCIDR == local.DestinationCIDR {
			addRoutes = []*routemodel.Route{}
			continue
		}
		if r.Name == routemodel.DefaultName && r.Description == routemodel.DefaultDescription {
			deleteRoutes = append(deleteRoutes, r)
		}
	}

	for i := range deleteRoutes {
		if deleteRoutes[i].Status == routemodel.StatusDeleting {
			continue
		}
		err := r.provider.DeleteRoute(context.TODO(), deleteRoutes[i].Id, deleteRoutes[i].NextHotId)
		if err != nil {
			return fmt.Errorf("delete route in routetable [%s]: nextHotId [%s], destinationCIDR [%s] , err: %s", tableId, deleteRoutes[i].NextHotId, deleteRoutes[i].DestinationCIDR, err.Error())
		}
		klog.Infof("Delete route for %s with %s - %s successfully", tableId, deleteRoutes[i].NextHotId, deleteRoutes[i].DestinationCIDR)
	}

	for i := range addRoutes {
		err := r.waitForDeleteRoute(tableId, addRoutes[i].DestinationCIDR)
		if err != nil {
			return fmt.Errorf("wait for routes to be deleted completely, error %s", err.Error())
		}
		err = r.provider.CreateRoute(context.TODO(), tableId, addRoutes[i])
		if err != nil {
			return fmt.Errorf("create route for node %s in routetable[%s]: nextHotId id [%s], destinationCIDR [%s], err: %s", addRoutes[i].NextNodeName, tableId, addRoutes[i].NextHotId, addRoutes[i].DestinationCIDR, err.Error())
		}
		klog.Infof("Created route in routetable %s with %s - %s successfully", tableId, addRoutes[i].NextHotId, addRoutes[i].DestinationCIDR)
	}

	return nil
}

func (r *ReconcileRoute) waitForDeleteRoute(tableId, destinationCIDR string) error {
	return wait.PollImmediate(5*time.Second, 30*time.Second, func() (done bool, err error) {
		routes, err := r.provider.FindRoute(context.TODO(), tableId, destinationCIDR)
		if err != nil {
			return true, err
		}
		if len(routes) == 0 {
			return true, nil
		}
		klog.Infof("[%s] wait for route enties to be deleted completely", tableId)
		return false, nil
	})
}

func (r *ReconcileRoute) getRouteTables(ctx context.Context) ([]string, error) {
	vpcId, err := r.provider.GetVpcID()
	if err != nil {
		return nil, fmt.Errorf("get vpc id from metadata error: %s", err.Error())
	}

	if strings.TrimSpace(r.cfg.RouteTableIDs) != "" {
		return strings.Split(strings.TrimSpace(r.cfg.RouteTableIDs), ","), nil
	}

	tables, err := r.provider.ListRouteTables(ctx, vpcId)
	if err != nil {
		return nil, fmt.Errorf("can not found route table by id [%s], error: %v", vpcId, err)
	}

	if len(tables) > 1 {
		return nil, fmt.Errorf("multiple route tables found by vpc id [%s], length(tables)=%d", vpcId, len(tables))
	}

	if len(tables) == 0 {
		return nil, fmt.Errorf("no route tables found by vpc id [%s]", vpcId)
	}
	klog.Infof("find route tables %s", tables)
	return tables, nil
}

func (r *ReconcileRoute) findGatewayNodes(ctx context.Context) ([]corev1.Node, error) {
	var nodeList corev1.NodeList
	listOpt := &client.ListOptions{LabelSelector: labels.SelectorFromSet(labels.Set{GatewayNode: "", NodeTypeKey: "false"})}
	err := r.Client.List(ctx, &nodeList, listOpt)
	if err != nil {
		return nil, err
	}
	candidateNodes := make([]corev1.Node, 0)
	for _, node := range nodeList.Items {
		if canSkipNode(&node) {
			continue
		}
		candidateNodes = append(candidateNodes, node)
	}
	sort.Slice(candidateNodes, func(i, j int) bool { return candidateNodes[i].Name < candidateNodes[j].Name })
	return candidateNodes, nil
}

func canSkipNode(node *corev1.Node) bool {
	if _, ok := node.Labels[kubeconsts.LabelNodeRoleControlPlane]; ok {
		return true
	}
	if _, ok := node.Labels[kubeconsts.LabelNodeRoleOldControlPlane]; ok {
		return true
	}

	if !isNodeReady(node) {
		return true
	}

	if !node.DeletionTimestamp.IsZero() {
		return true
	}

	_, id, err := NodeFromProviderID(node.Spec.ProviderID)
	if err != nil || id == "" {
		return true
	}
	return false
}

func isNodeReady(node *corev1.Node) bool {
	_, nc := nodeutil.GetNodeCondition(&node.Status, corev1.NodeReady)
	// GetNodeCondition will return nil and -1 if the condition is not present
	return nc != nil && nc.Status == corev1.ConditionTrue
}

func isGatewayNode(node *corev1.Node) bool {
	if node.Labels != nil {
		_, ok := node.Labels[GatewayNode]
		if ok && strings.ToLower(node.Labels[NodeTypeKey]) == "false" {
			return true
		}
	}
	return false
}

func NodeFromProviderID(providerID string) (string, string, error) {
	if strings.HasPrefix(providerID, "alicloud://") {
		k8sName := strings.Split(providerID, "://")
		if len(k8sName) < 2 {
			return "", "", fmt.Errorf("alicloud: unable to split instanceid and region from providerID, error unexpected providerID=%s", providerID)
		} else {
			providerID = k8sName[1]
		}
	}

	name := strings.Split(providerID, ".")
	if len(name) < 2 {
		return "", "", fmt.Errorf("alicloud: unable to split instanceid and region from providerID, error unexpected providerID=%s", providerID)
	}
	return name[0], name[1], nil
}
