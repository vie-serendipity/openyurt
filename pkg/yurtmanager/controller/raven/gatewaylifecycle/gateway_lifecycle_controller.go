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

package gatewaylifecycle

import (
	"context"
	"fmt"
	"math"
	"net"
	"sort"
	"strconv"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	appconfig "github.com/openyurtio/openyurt/cmd/yurt-manager/app/config"
	"github.com/openyurtio/openyurt/cmd/yurt-manager/names"
	nodepoolv1beta1 "github.com/openyurtio/openyurt/pkg/apis/apps/v1beta1"
	"github.com/openyurtio/openyurt/pkg/apis/raven"
	ravenv1beta1 "github.com/openyurtio/openyurt/pkg/apis/raven/v1beta1"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/raven/util"
)

const (
	InterconnectionModeAnnotationKey = "alibabacloud.com/interconnection-mode"
	PoolNodesConnectedAnnotationKey  = "alibabacloud.com/pool-nodes-connected"
	SpecifiedGateway                 = "alibabacloud.com/belong-to-gateway"
	GatewayExcludeNodePool           = "alibabacloud.com/gateway-exclude-nodepool"
	AddressConflict                  = "alibabacloud.com/address-conflict"
	BelongToNodePool                 = "alibabacloud.com/belong-to-nodepool"

	GatewayEndpoint = "alibabacloud.com/cross-domain-gateway-endpoint"
	UnderNAT        = "alibabacloud.com/under-nat"

	EdgeNodeKey = "alibabacloud.com/is-edge-worker"
	NodePoolKey = "alibabacloud.com/nodepool-id"

	GatewayPrefix     = "gw-"
	CloudNodePoolName = "cloud"
	GatewayPrivateIP  = "internal-ip"
	DedicatedLine     = "private"
	PublicInternet    = "public"

	DefaultEndpointsProportion = 0.3
)

func Format(format string, args ...interface{}) string {
	s := fmt.Sprintf(format, args...)
	return fmt.Sprintf("%s: %s", names.GatewayLifeCycleController, s)
}

// Add creates a new Service Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(ctx context.Context, c *appconfig.CompletedConfig, mgr manager.Manager) error {
	return add(mgr, newReconciler(c, mgr))
}

var _ reconcile.Reconciler = &ReconcileGatewayLifeCycle{}

// ReconcileGatewayLifeCycle reconciles a Gateway object
type ReconcileGatewayLifeCycle struct {
	client.Client
	scheme             *runtime.Scheme
	recorder           record.EventRecorder
	centreExposedPorts string
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(c *appconfig.CompletedConfig, mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileGatewayLifeCycle{
		Client:             mgr.GetClient(),
		scheme:             mgr.GetScheme(),
		recorder:           mgr.GetEventRecorderFor(names.GatewayLifeCycleController),
		centreExposedPorts: c.ComponentConfig.GatewayLifecycleController.CentreExposedPorts,
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New(names.GatewayLifeCycleController, mgr, controller.Options{
		Reconciler: r, MaxConcurrentReconciles: util.ConcurrentReconciles,
	})
	if err != nil {
		return err
	}

	// Watch for changes to NodePool
	err = c.Watch(&source.Kind{Type: &corev1.ConfigMap{}}, &EnqueueRequestForRavenConfigEvent{mgr.GetClient()}, predicate.NewPredicateFuncs(
		func(object client.Object) bool {
			cm, ok := object.(*corev1.ConfigMap)
			if !ok {
				return false
			}
			if cm.GetNamespace() != util.WorkingNamespace {
				return false
			}
			if cm.GetName() != util.RavenGlobalConfig {
				return false
			}
			return true
		}))
	if err != nil {
		return err
	}

	err = c.Watch(&source.Kind{Type: &nodepoolv1beta1.NodePool{}}, &EnqueueRequestForNodePoolEvent{})
	if err != nil {
		return err
	}

	err = c.Watch(&source.Kind{Type: &corev1.Node{}}, &EnqueueRequestForNodeEvent{})
	if err != nil {
		return err
	}

	return nil
}

// Reconcile reads that state of the cluster for a Gateway object and makes changes based on the state read
// and what is in the Gateway.Spec
func (r *ReconcileGatewayLifeCycle) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {

	np, clear, err := needDeleteGateway(ctx, r.Client, req.Name)
	if err != nil {
		klog.Error(Format("check whether the gateway for nodepool %s needs to be deleted, error %s", req.Name, err.Error()))
		return reconcile.Result{Requeue: true, RequeueAfter: 2 * time.Second}, err
	}

	if isExclude(np) {
		klog.Info(Format("ignore nodepool %s, skip reconcile it", np.GetName()))
		return reconcile.Result{}, nil
	}

	err = r.reconcileCloud(ctx, np)
	if err != nil {
		klog.Error(Format("reconcile cloud gateway for nodepool %s, error %s", np.GetName(), err.Error()))
		return reconcile.Result{Requeue: true, RequeueAfter: 2 * time.Second}, fmt.Errorf("reconcile cloud gateway, error %s", err.Error())
	}

	err = r.reconcileEdge(ctx, np, clear)
	if err != nil {
		klog.Error(Format("reconcile edge gateway for nodepool %s, error %s", np.GetName(), err.Error()))
		return reconcile.Result{Requeue: true, RequeueAfter: 2 * time.Second}, fmt.Errorf("reconcile edge %s gateway, error %s", np.GetName(), err.Error())
	}
	return reconcile.Result{}, nil
}

func needDeleteGateway(ctx context.Context, client client.Client, nodePoolName string) (*nodepoolv1beta1.NodePool, bool, error) {
	var clear bool
	np := nodepoolv1beta1.NodePool{ObjectMeta: metav1.ObjectMeta{Name: nodePoolName}}
	err := client.Get(ctx, types.NamespacedName{Name: nodePoolName}, &np)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return np.DeepCopy(), clear, err
		} else {
			clear = true
			return np.DeepCopy(), clear, nil
		}
	}

	if np.GetDeletionTimestamp() != nil {
		clear = true
		return np.DeepCopy(), clear, nil
	}

	enableProxy, enableTunnel := util.CheckServer(ctx, client)
	if !(enableProxy && enableTunnel) {
		clear = true
		return np.DeepCopy(), clear, nil
	}
	return np.DeepCopy(), clear, nil
}

func (r *ReconcileGatewayLifeCycle) reconcileCloud(ctx context.Context, np *nodepoolv1beta1.NodePool) error {
	if !isCloudNodePool(np) {
		return nil
	}
	enableProxy, enableTunnel := util.CheckServer(ctx, r.Client)
	if enableProxy || enableTunnel {
		err := r.updateCloudGateway(ctx)
		if err != nil {
			return fmt.Errorf("failed to update cloud gateway for nodepool %s, error %s", np.GetName(), err.Error())
		}
		err = r.manageNodesLabelByNodePool(ctx, np, fmt.Sprintf("%s%s", GatewayPrefix, CloudNodePoolName), false)
		if err != nil {
			return fmt.Errorf("failed to update cloud gateway label for nodepool %s,, error %s", np.GetName(), err.Error())
		}
	} else {
		err := r.clearCloudGateway(ctx)
		if err != nil {
			return fmt.Errorf("failed to delete cloud gateway, error %s", err.Error())
		}
	}
	return nil
}

func (r *ReconcileGatewayLifeCycle) updateCloudGateway(ctx context.Context) error {
	var cloudGw ravenv1beta1.Gateway
	gwName := fmt.Sprintf("%s%s", GatewayPrefix, CloudNodePoolName)
	publicAddr, privateAddr := r.getLoadBalancerIP(ctx)
	endpoints := r.getCloudGatewayEndpoints(ctx, publicAddr, privateAddr)
	err := r.Client.Get(ctx, types.NamespacedName{Name: gwName}, &cloudGw)
	if err != nil {
		if apierrors.IsNotFound(err) {
			anno := map[string]string{"proxy-server-exposed-ports": r.centreExposedPorts}
			err = r.Client.Create(ctx, &ravenv1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: gwName, Annotations: anno},
				Spec: ravenv1beta1.GatewaySpec{
					ProxyConfig: ravenv1beta1.ProxyConfiguration{
						Replicas:       len(endpoints) / 2,
						ProxyHTTPPort:  fmt.Sprintf("%d,%d", util.KubeletInsecurePort, util.PrometheusInsecurePort),
						ProxyHTTPSPort: fmt.Sprintf("%d,%d", util.KubeletSecurePort, util.PrometheusSecurePort),
					},
					TunnelConfig: ravenv1beta1.TunnelConfiguration{
						Replicas: 1,
					},
					ExposeType: ravenv1beta1.ExposeTypeLoadBalancer,
					Endpoints:  endpoints,
				},
			})
			if err != nil {
				if apierrors.IsAlreadyExists(err) {
					klog.V(4).Info(Format("gateway %s has already exist, ignore creating it", gwName))
					return nil
				}
				return fmt.Errorf("failed to create gateway %s, error %s", gwName, err.Error())
			}
			return nil
		}
		return fmt.Errorf("failed to create gateway %s, error %s", gwName, err.Error())
	}
	cloudGw.Spec.ProxyConfig.Replicas = len(endpoints) / 2
	cloudGw.Spec.TunnelConfig.Replicas = 1
	cloudGw.Spec.Endpoints = endpoints
	err = r.Client.Update(ctx, cloudGw.DeepCopy())
	if err != nil {
		return fmt.Errorf("failed to update gateway %s, error %s", gwName, err.Error())
	}
	return nil
}

func (r *ReconcileGatewayLifeCycle) getCloudGatewayEndpoints(ctx context.Context, publicAddress, privateAddress string) []ravenv1beta1.Endpoint {
	endpoints := make([]ravenv1beta1.Endpoint, 0)
	defaultEndpoints := make([]ravenv1beta1.Endpoint, 0)
	var nodeList corev1.NodeList
	err := wait.PollImmediate(10*time.Second, time.Minute, func() (done bool, err error) {
		err = r.Client.List(ctx, &nodeList, &client.ListOptions{
			LabelSelector: labels.Set{EdgeNodeKey: "false"}.AsSelector()})
		if err != nil {
			return false, err
		}
		return true, nil
	})
	if err != nil {
		klog.Warning(Format("failed to list cloud nodes for gateway endpoints"))
		return endpoints
	}
	sort.Slice(nodeList.Items, func(i, j int) bool { return nodeList.Items[i].Name < nodeList.Items[j].Name })
	num := int(math.Max(float64(len(nodeList.Items))*DefaultEndpointsProportion, 3))
	for i := range nodeList.Items {
		node := nodeList.Items[i]
		if isGatewayEndpoint(&node) {
			endpoints = append(endpoints, r.generateEndpoints(node.Name, publicAddress, privateAddress, false)...)
		} else {
			if num == 0 {
				break
			}
			defaultEndpoints = append(defaultEndpoints, r.generateEndpoints(node.Name, publicAddress, privateAddress, false)...)
			num--
		}
	}
	if len(endpoints) > 0 {
		return endpoints
	}
	return defaultEndpoints
}

func (r *ReconcileGatewayLifeCycle) getEdgeGatewayEndpoints(ctx context.Context, np *nodepoolv1beta1.NodePool) []ravenv1beta1.Endpoint {
	endpoints := make([]ravenv1beta1.Endpoint, 0)
	defaultEndpoints := make([]ravenv1beta1.Endpoint, 0)
	var nodeList corev1.NodeList
	err := wait.PollImmediate(10*time.Second, time.Minute, func() (done bool, err error) {
		err = r.Client.List(ctx, &nodeList, &client.ListOptions{
			LabelSelector: labels.Set{NodePoolKey: np.GetName()}.AsSelector()})
		if err != nil {
			return false, err
		}
		return true, nil
	})
	if err != nil {
		klog.Warning(Format("failed to list cloud nodes for gateway endpoints"))
		return endpoints
	}
	sort.Slice(nodeList.Items, func(i, j int) bool { return nodeList.Items[i].Name < nodeList.Items[j].Name })
	num := int(math.Max(float64(len(nodeList.Items))*DefaultEndpointsProportion, 1))
	for i := range nodeList.Items {
		node := nodeList.Items[i]
		underNat := isUnderNAT(&node)
		if isGatewayEndpoint(&node) {
			endpoints = append(endpoints, r.generateEndpoints(node.Name, "", "", underNat)...)
		} else {
			if num == 0 {
				break
			}
			defaultEndpoints = append(defaultEndpoints, r.generateEndpoints(node.Name, "", "", underNat)...)
			num--
		}
	}
	if len(endpoints) > 0 {
		return endpoints
	}
	return defaultEndpoints

}

func (r *ReconcileGatewayLifeCycle) clearCloudGateway(ctx context.Context) error {
	gwName := fmt.Sprintf("%s%s", GatewayPrefix, CloudNodePoolName)
	var gw ravenv1beta1.Gateway
	err := r.Client.Get(ctx, types.NamespacedName{Name: gwName}, &gw)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to get gateway %s, error %s", gw.GetName(), err.Error())
	}
	for _, node := range gw.Status.Nodes {
		err := r.manageNodeLabel(ctx, node.NodeName, "", true)
		if err != nil {
			return fmt.Errorf("failed to delete label for node %s, error %s", node.NodeName, err.Error())
		}
	}
	err = r.Client.Delete(ctx, &gw)
	if err != nil {
		return fmt.Errorf("failed to delete gateway %s, error %s", gw.GetName(), err.Error())
	}
	return nil
}

func (r *ReconcileGatewayLifeCycle) reconcileEdge(ctx context.Context, np *nodepoolv1beta1.NodePool, isClear bool) error {
	if !isEdgeNodePool(np) {
		return nil
	}

	if getInterconnectionMode(np) == DedicatedLine && !isAddressConflict(np) {
		np.Annotations[SpecifiedGateway] = fmt.Sprintf("%s%s", GatewayPrefix, CloudNodePoolName)
	}

	if isClear {
		return r.clearGateway(ctx, np, isSoloMode(np))
	}

	return r.updateEdgeGateway(ctx, np)
}

func (r *ReconcileGatewayLifeCycle) manageNodesLabelByNodePool(ctx context.Context, np *nodepoolv1beta1.NodePool, gwName string, isClear bool) error {
	for _, node := range np.Status.Nodes {
		err := r.manageNodeLabel(ctx, node, gwName, isClear)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *ReconcileGatewayLifeCycle) manageNodeLabel(ctx context.Context, nodeName, gwName string, isClear bool) error {
	var node corev1.Node
	err := r.Client.Get(ctx, types.NamespacedName{Name: nodeName}, &node)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed get nodes %s for gateway %s, error %s", nodeName, gwName, err.Error())
		} else {
			return nil
		}
	}
	if isClear {
		if _, ok := node.Labels[raven.LabelCurrentGateway]; ok {
			delete(node.Labels, raven.LabelCurrentGateway)
			err = r.Client.Update(ctx, node.DeepCopy())
			if err != nil {
				return fmt.Errorf("failed update node %s for gateway %s, error %s", nodeName, gwName, err.Error())
			}
		}
	} else {
		if node.Labels[raven.LabelCurrentGateway] != gwName {
			node.Labels[raven.LabelCurrentGateway] = gwName
			err = r.Client.Update(ctx, node.DeepCopy())
			if err != nil {
				return fmt.Errorf("failed update node %s for gateway %s, error %s", nodeName, gwName, err.Error())
			}
		}
	}
	return nil
}

func (r *ReconcileGatewayLifeCycle) getLoadBalancerIP(ctx context.Context) (publicAddr, privateAddr string) {
	var cm corev1.ConfigMap
	objKey := types.NamespacedName{Name: util.RavenGlobalConfig, Namespace: util.WorkingNamespace}
	err := r.Client.Get(ctx, objKey, &cm)
	if err != nil {
		return "", ""
	}
	if cm.Data == nil {
		return "", ""
	}
	privateAddr = cm.Data[util.LoadBalancerIP]
	publicAddr = cm.Data[util.ElasticIPIP]
	return
}

func (r *ReconcileGatewayLifeCycle) updateEdgeGateway(ctx context.Context, np *nodepoolv1beta1.NodePool) error {
	if len(np.Status.Nodes) == 0 {
		klog.Info(Format("nodepool %s has no node, skip manage it for gateway", np.GetName()))
		return nil
	}
	if gwName, ok := isSpecified(np); ok {
		err := r.updateSpecifiedGateways(ctx, np, gwName)
		if err != nil {
			return fmt.Errorf("failed to update specified gateway for nodepool %s, error %s", np.GetName(), err.Error())
		}
		return nil
	}

	if isSoloMode(np) {
		err := r.updateSoloGateways(ctx, np)
		if err != nil {
			return fmt.Errorf("failed to update edge solo gateway for nodepool %s, error %s", np.GetName(), err.Error())
		}
		return nil
	}

	if isPoolMode(np) {
		err := r.updatePoolGateways(ctx, np)
		if err != nil {
			return fmt.Errorf("failed update edge pool gateway for nodepool %s, error %s", np.GetName(), err.Error())
		}
		return nil
	}
	return nil
}

func (r *ReconcileGatewayLifeCycle) updateSpecifiedGateways(ctx context.Context, np *nodepoolv1beta1.NodePool, gwName string) error {
	var edgeGatewayList ravenv1beta1.GatewayList
	err := r.Client.List(ctx, &edgeGatewayList, &client.ListOptions{LabelSelector: labels.Set{BelongToNodePool: np.GetName()}.AsSelector()})
	if err != nil {
		return fmt.Errorf("failed to find gateways for nodepool %s, error %s", np.GetName(), err.Error())
	}
	for _, gw := range edgeGatewayList.Items {
		err = r.Client.Delete(ctx, &gw)
		if err != nil {
			klog.Errorf("failed to delete gateway %s, error %s", gw.GetName(), err.Error())
			continue
		}
	}
	err = r.manageNodesLabelByNodePool(ctx, np, gwName, false)
	if err != nil {
		return fmt.Errorf("failed to label nodes for gateway %s, error %s", gwName, err.Error())
	}
	return nil
}

func (r *ReconcileGatewayLifeCycle) updatePoolGateways(ctx context.Context, np *nodepoolv1beta1.NodePool) error {
	anno := map[string]string{InterconnectionModeAnnotationKey: getInterconnectionMode(np)}
	label := map[string]string{BelongToNodePool: np.GetName()}
	endpoints := r.getEdgeGatewayEndpoints(ctx, np)
	gwName := fmt.Sprintf("%s%s", GatewayPrefix, np.GetName())
	var edgeGateway ravenv1beta1.Gateway
	err := r.Client.Get(ctx, types.NamespacedName{Name: gwName}, &edgeGateway)
	if err != nil {
		if apierrors.IsNotFound(err) {
			err = r.Client.Create(ctx, &ravenv1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: gwName, Annotations: anno, Labels: label},
				Spec: ravenv1beta1.GatewaySpec{
					ProxyConfig: ravenv1beta1.ProxyConfiguration{
						Replicas: len(endpoints) / 2,
					},
					TunnelConfig: ravenv1beta1.TunnelConfiguration{
						Replicas: 1,
					},
					Endpoints: endpoints,
				},
			})
			if err != nil {
				return fmt.Errorf("create gateway %s for nodepool %s, error %s", gwName, np.GetName(), err.Error())
			}
		} else {
			return fmt.Errorf("create gateway %s, error %s", gwName, err.Error())
		}
	} else {
		edgeGateway.Spec.ProxyConfig.Replicas = len(endpoints) / 2
		edgeGateway.Spec.Endpoints = endpoints
		err = r.Client.Update(ctx, edgeGateway.DeepCopy())
		if err != nil {
			return fmt.Errorf("update gateway %s for nodepool %s, error %s", gwName, np.GetName(), err.Error())
		}
	}
	err = r.manageNodesLabelByNodePool(ctx, np, gwName, false)
	if err != nil {
		return fmt.Errorf("failed to label nodes for gateway %s, error %s", gwName, err.Error())
	}
	return nil
}

func (r *ReconcileGatewayLifeCycle) getSoloGateways(ctx context.Context, name string) (*ravenv1beta1.GatewayList, error) {
	var gwList ravenv1beta1.GatewayList
	err := r.Client.List(ctx, &gwList, &client.ListOptions{LabelSelector: labels.Set{BelongToNodePool: name}.AsSelector()})
	if err != nil {
		return nil, err
	}
	return gwList.DeepCopy(), nil
}

func (r *ReconcileGatewayLifeCycle) generateSoloGateways(ctx context.Context, np *nodepoolv1beta1.NodePool) (*ravenv1beta1.GatewayList, error) {
	var gwList ravenv1beta1.GatewayList
	gwSlice := make([]ravenv1beta1.Gateway, 0)
	anno := map[string]string{InterconnectionModeAnnotationKey: getInterconnectionMode(np)}
	label := map[string]string{BelongToNodePool: np.GetName()}
	for _, name := range np.Status.Nodes {
		var node corev1.Node
		err := r.Client.Get(ctx, types.NamespacedName{Name: name}, &node)
		if err != nil {
			return nil, err
		}
		underNat := isUnderNAT(&node)
		gwSlice = append(gwSlice, ravenv1beta1.Gateway{
			ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("%s%s", GatewayPrefix, name), Annotations: anno, Labels: label},
			Spec: ravenv1beta1.GatewaySpec{
				ProxyConfig: ravenv1beta1.ProxyConfiguration{
					Replicas: 1,
				},
				TunnelConfig: ravenv1beta1.TunnelConfiguration{
					Replicas: 1,
				},
				Endpoints: r.generateEndpoints(name, "", "", underNat),
			}})
	}
	gwList.Items = gwSlice
	return gwList.DeepCopy(), nil
}

func (r *ReconcileGatewayLifeCycle) updateSoloGateways(ctx context.Context, np *nodepoolv1beta1.NodePool) error {
	curr, err := r.getSoloGateways(ctx, np.GetName())
	if err != nil {
		return fmt.Errorf("list current solo gateways error %s", err.Error())
	}
	spec := &ravenv1beta1.GatewayList{Items: []ravenv1beta1.Gateway{}}
	_, enableTunnel := util.CheckServer(ctx, r.Client)
	if enableTunnel {
		spec, err = r.generateSoloGateways(ctx, np)
		if err != nil {
			return fmt.Errorf("generate spec solo gateways error %s", err.Error())
		}
	}

	addedGateway := make([]ravenv1beta1.Gateway, 0)
	updatedGateway := make([]ravenv1beta1.Gateway, 0)
	deletedGateway := make([]ravenv1beta1.Gateway, 0)

	currMap := make(map[string]int)
	specMap := make(map[string]int)
	for idx, gw := range curr.Items {
		currMap[gw.GetName()] = idx
	}
	for idx, gw := range spec.Items {
		specMap[gw.GetName()] = idx
	}
	for key, val := range specMap {
		if idx, ok := currMap[key]; ok {
			gw := curr.Items[idx]
			gw.Spec = spec.Items[val].Spec
			updatedGateway = append(updatedGateway, gw)
			delete(currMap, key)
		} else {
			addedGateway = append(addedGateway, spec.Items[val])
		}
	}
	for _, val := range currMap {
		deletedGateway = append(deletedGateway, curr.Items[val])
	}

	for i := range addedGateway {
		gwName := addedGateway[i].GetName()
		nodeName := strings.TrimPrefix(gwName, GatewayPrefix)
		err = r.Client.Create(ctx, addedGateway[i].DeepCopy())
		if err != nil {
			return fmt.Errorf("create gateway %s, error %s", gwName, err.Error())
		}
		err = r.manageNodeLabel(ctx, nodeName, gwName, false)
		if err != nil {
			return fmt.Errorf("label node %s gateway=%s, error %s", nodeName, gwName, err.Error())
		}
	}
	for i := range updatedGateway {
		gwName := updatedGateway[i].GetName()
		nodeName := strings.TrimPrefix(gwName, GatewayPrefix)
		err = r.Client.Update(ctx, &updatedGateway[i])
		if err != nil {
			return fmt.Errorf("create gateway %s, error %s", gwName, err.Error())
		}
		err = r.manageNodeLabel(ctx, nodeName, gwName, false)
		if err != nil {
			return fmt.Errorf("label node %s gateway=%s, error %s", nodeName, gwName, err.Error())
		}
	}
	for i := range deletedGateway {
		err = r.Client.Delete(ctx, &deletedGateway[i])
		if err != nil {
			return fmt.Errorf("create gateway %s, error %s", deletedGateway[i].GetName(), err.Error())
		}
	}
	return nil
}

func (r *ReconcileGatewayLifeCycle) clearGateway(ctx context.Context, np *nodepoolv1beta1.NodePool, isSoloMode bool) error {
	if isSoloMode {
		for _, nodeName := range np.Status.Nodes {
			gwName := fmt.Sprintf("%s%s", GatewayPrefix, nodeName)
			err := r.Client.Delete(ctx, &ravenv1beta1.Gateway{ObjectMeta: metav1.ObjectMeta{Name: gwName}})
			if err != nil && !apierrors.IsNotFound(err) {
				return fmt.Errorf("failed to delete gateway %s, error %s", gwName, err.Error())
			}
			err = r.manageNodeLabel(ctx, nodeName, gwName, true)
			if err != nil {
				return fmt.Errorf("failed to clear label for node %s, error %s", nodeName, err.Error())
			}
		}
	} else {
		gwName := fmt.Sprintf("%s%s", GatewayPrefix, np.GetName())
		err := r.Client.Delete(ctx, &ravenv1beta1.Gateway{ObjectMeta: metav1.ObjectMeta{Name: gwName}})
		if err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to delete gateway %s, error %s", gwName, err.Error())
		}
		err = r.manageNodesLabelByNodePool(ctx, np, "", true)
		if err != nil {
			return fmt.Errorf("failed to clear label for nodepool %s, error %s", np.GetName(), err)
		}
	}
	return nil
}

func (r *ReconcileGatewayLifeCycle) getTargetPort() (proxyPort, tunnelPort int32) {
	proxyPort = ravenv1beta1.DefaultProxyServerExposedPort
	tunnelPort = ravenv1beta1.DefaultTunnelServerExposedPort
	var cm corev1.ConfigMap
	err := r.Get(context.TODO(), types.NamespacedName{Namespace: util.WorkingNamespace, Name: util.RavenAgentConfig}, &cm)
	if err != nil {
		return
	}
	_, proxyExposedPort, err := net.SplitHostPort(cm.Data[util.ProxyServerExposedPortKey])
	if err == nil {
		proxy, _ := strconv.Atoi(proxyExposedPort)
		proxyPort = int32(proxy)
	}
	_, tunnelExposedPort, err := net.SplitHostPort(cm.Data[util.VPNServerExposedPortKey])
	if err == nil {
		tunnel, _ := strconv.Atoi(tunnelExposedPort)
		tunnelPort = int32(tunnel)
	}
	return
}

func (r *ReconcileGatewayLifeCycle) generateEndpoints(nodeName, publicIP, privateIP string, underNAT bool) []ravenv1beta1.Endpoint {
	proxyPort, tunnelPort := r.getTargetPort()
	endpoints := make([]ravenv1beta1.Endpoint, 0)
	endpoints = append(endpoints, ravenv1beta1.Endpoint{
		NodeName: nodeName,
		Type:     ravenv1beta1.Tunnel,
		Port:     int(tunnelPort),
		UnderNAT: underNAT,
		PublicIP: publicIP,
		Config:   map[string]string{GatewayPrivateIP: privateIP},
	})
	endpoints = append(endpoints, ravenv1beta1.Endpoint{
		NodeName: nodeName,
		Type:     ravenv1beta1.Proxy,
		Port:     int(proxyPort),
		UnderNAT: underNAT,
		PublicIP: publicIP,
		Config:   map[string]string{GatewayPrivateIP: privateIP},
	})
	return endpoints
}

func isEdgeNodePool(np *nodepoolv1beta1.NodePool) bool {
	return np.Spec.Type == nodepoolv1beta1.Edge
}

func isCloudNodePool(np *nodepoolv1beta1.NodePool) bool {
	return np.Spec.Type == nodepoolv1beta1.Cloud
}

func isSoloMode(np *nodepoolv1beta1.NodePool) bool {
	return strings.ToLower(np.Annotations[PoolNodesConnectedAnnotationKey]) != "true"
}

func isPoolMode(np *nodepoolv1beta1.NodePool) bool {
	return strings.ToLower(np.Annotations[PoolNodesConnectedAnnotationKey]) == "true"
}

func getInterconnectionMode(np *nodepoolv1beta1.NodePool) string {
	return strings.ToLower(np.Annotations[InterconnectionModeAnnotationKey])
}

func isAddressConflict(np *nodepoolv1beta1.NodePool) bool {
	return strings.ToLower(np.Annotations[AddressConflict]) == "true"
}

func isSpecified(np *nodepoolv1beta1.NodePool) (string, bool) {
	gwName, ok := np.Annotations[SpecifiedGateway]
	return gwName, ok
}

func isExclude(np *nodepoolv1beta1.NodePool) bool {
	return strings.ToLower(np.Annotations[GatewayExcludeNodePool]) == "true"
}

func isGatewayEndpoint(node *corev1.Node) bool {
	ret, err := strconv.ParseBool(node.Labels[GatewayEndpoint])
	if err != nil {
		return false
	}
	return ret
}

func isUnderNAT(node *corev1.Node) bool {
	ret, err := strconv.ParseBool(node.Labels[UnderNAT])
	if err != nil {
		return true
	}
	return ret
}
