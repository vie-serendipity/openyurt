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
	"net"
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

	EdgeNodeKey       = "alibabacloud.com/is-edge-worker"
	BelongToNodePool  = "alibabacloud.com/belong-to-nodepool"
	GatewayPrefix     = "gw-"
	CloudNodePoolName = "cloud"
	GatewayPrivateIP  = "internal-ip"
	DedicatedLine     = "private"
	PublicInternet    = "public"
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

var _ reconcile.Reconciler = &ReconcileResource{}

// ReconcileService reconciles a Gateway object
type ReconcileResource struct {
	client.Client
	scheme   *runtime.Scheme
	recorder record.EventRecorder
	option   util.Option
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(c *appconfig.CompletedConfig, mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileResource{
		Client:   mgr.GetClient(),
		scheme:   mgr.GetScheme(),
		recorder: mgr.GetEventRecorderFor(names.GatewayLifeCycleController),
		option:   util.NewOption(),
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

	return nil
}

// Reconcile reads that state of the cluster for a Gateway object and makes changes based on the state read
// and what is in the Gateway.Spec
func (r *ReconcileResource) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	klog.V(2).Info(Format("started reconciling nodepool %s", req.Name))
	defer func() {
		klog.V(2).Info(Format("finished reconciling nodepool %s", req.Name))
	}()

	enableProxy, enableTunnel := util.CheckServer(ctx, r.Client)
	r.option.SetProxyOption(enableProxy)
	r.option.SetTunnelOption(enableTunnel)

	np, err := r.getNodePool(ctx, req.Name)
	if err != nil {
		return reconcile.Result{Requeue: true, RequeueAfter: 2 * time.Second}, fmt.Errorf("failed to get np %s, error %s", req.Name, err.Error())
	}
	if isExclude(np) {
		klog.V(4).Info(Format("skip reconcile nodepool %s", np.GetName()))
		return reconcile.Result{}, nil
	}

	err = r.reconcileCloud(ctx, np)
	if err != nil {
		return reconcile.Result{Requeue: true, RequeueAfter: 2 * time.Second}, fmt.Errorf("failed to reconcile cloud gateway, error %s", err.Error())
	}

	err = r.reconcileEdge(ctx, np)
	if err != nil {
		return reconcile.Result{Requeue: true, RequeueAfter: 2 * time.Second}, fmt.Errorf("failed to reconcile edge %s gateway, error %s", np.GetName(), err.Error())
	}
	return reconcile.Result{}, nil
}

func (r *ReconcileResource) getNodePool(ctx context.Context, name string) (*nodepoolv1beta1.NodePool, error) {
	var np nodepoolv1beta1.NodePool
	err := r.Client.Get(ctx, types.NamespacedName{Name: name}, &np)
	if err != nil {
		return nil, err
	}
	return np.DeepCopy(), nil
}

func (r *ReconcileResource) reconcileCloud(ctx context.Context, np *nodepoolv1beta1.NodePool) error {
	if !isCloudNodePool(np) {
		return nil
	}
	enableTunnel := r.option.GetTunnelOption()
	enableProxy := r.option.GetProxyOption()
	if enableProxy || enableTunnel {
		err := r.updateCloudGateway(ctx)
		if err != nil {
			return fmt.Errorf("failed to update cloud gateway, error %s", err.Error())
		}
		err = r.labelNodesByNodePool(ctx, np, fmt.Sprintf("%s%s", GatewayPrefix, CloudNodePoolName), false)
		if err != nil {
			return fmt.Errorf("failed to update cloud gateway label, error %s", err.Error())
		}
	} else {
		err := r.clearCloudGateway(ctx)
		if err != nil {
			return fmt.Errorf("failed to delete cloud gateway, error %s", err.Error())
		}
	}
	return nil
}

func (r *ReconcileResource) updateCloudGateway(ctx context.Context) error {
	var cloudGw ravenv1beta1.Gateway
	gwName := fmt.Sprintf("%s%s", GatewayPrefix, CloudNodePoolName)
	err := r.Client.Get(ctx, types.NamespacedName{Name: gwName}, &cloudGw)
	publicAddr, privateAddr, addrErr := r.getLoadBalancerIP(ctx)
	if addrErr != nil {
		r.recorder.Event(cloudGw.DeepCopy(), corev1.EventTypeNormal, "AcquiredEndpointsPublicIP",
			fmt.Sprintf("The gateway %s has no publicIP, ExposedType %s", gwName, cloudGw.Spec.ExposeType))
	}
	if err != nil {
		if apierrors.IsNotFound(err) {
			var nodeList corev1.NodeList
			err = wait.PollImmediate(10*time.Second, time.Minute, func() (done bool, err error) {
				err = r.Client.List(ctx, &nodeList, &client.ListOptions{
					LabelSelector: labels.Set{EdgeNodeKey: "false"}.AsSelector()})
				if err != nil {
					return false, err
				}
				return true, nil
			})
			if err != nil {
				klog.Warning(Format("failed to list cloud nodes for generate gateway %s", gwName))
			}
			endpoints := make([]ravenv1beta1.Endpoint, 0)
			for idx, node := range nodeList.Items {
				if idx > 2 {
					break
				}
				endpoints = append(endpoints, r.generateEndpoints(node.GetName(), publicAddr, false)...)
			}
			for idx := 0; idx < len(endpoints); idx++ {
				endpoints[idx].Config[GatewayPrivateIP] = privateAddr
			}
			anno := map[string]string{"proxy-server-exposed-ports": "10280,10281,10282"}
			err = r.Client.Create(ctx, &ravenv1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: gwName, Annotations: anno},
				Spec: ravenv1beta1.GatewaySpec{
					ProxyConfig: ravenv1beta1.ProxyConfiguration{
						Replicas:       1,
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
	var needUpdate bool
	for idx, ep := range cloudGw.Spec.Endpoints {
		if ep.PublicIP != publicAddr {
			needUpdate = true
			cloudGw.Spec.Endpoints[idx].PublicIP = publicAddr
		}
		if privateIP := ep.Config[GatewayPrivateIP]; privateIP != privateAddr {
			needUpdate = true
			cloudGw.Spec.Endpoints[idx].Config[GatewayPrivateIP] = privateAddr
		}
	}
	if needUpdate {
		err = r.Client.Update(ctx, cloudGw.DeepCopy())
		if err != nil {
			return fmt.Errorf("failed to update gateway %s, error %s", gwName, err.Error())
		}
	}

	return nil
}

func (r *ReconcileResource) clearCloudGateway(ctx context.Context) error {
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
		err := r.labelNode(ctx, node.NodeName, "", true)
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

func (r *ReconcileResource) clearCloudGatewayLabel(ctx context.Context, np *nodepoolv1beta1.NodePool) error {
	gwName := fmt.Sprintf("%s%s", GatewayPrefix, CloudNodePoolName)
	for _, nodeName := range np.Status.Nodes {
		err := r.labelNode(ctx, nodeName, gwName, false)
		if err != nil {
			return fmt.Errorf("failed to label node %s, error %s", nodeName, err.Error())
		}
	}
	return nil
}

func (r *ReconcileResource) reconcileEdge(ctx context.Context, np *nodepoolv1beta1.NodePool) error {
	if !isEdgeNodePool(np) {
		return nil
	}
	if getInterconnectionMode(np) == DedicatedLine && !isAddressConflict(np) {
		np.Annotations[SpecifiedGateway] = fmt.Sprintf("%s%s", GatewayPrefix, CloudNodePoolName)
	}
	enableTunnel := r.option.GetTunnelOption()
	enableProxy := r.option.GetProxyOption()
	if enableProxy || enableTunnel {
		err := r.updateEdgeGateway(ctx, np)
		if err != nil {
			return err
		}
	} else {
		return r.clearGateway(ctx, np, isSoloMode(np))
	}
	if !enableTunnel && isSoloMode(np) {
		return r.clearGateway(ctx, np, true)
	}
	return nil
}

func (r *ReconcileResource) labelNodesByNodePool(ctx context.Context, np *nodepoolv1beta1.NodePool, gwName string, isClear bool) error {
	for _, node := range np.Status.Nodes {
		err := r.labelNode(ctx, node, gwName, isClear)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *ReconcileResource) labelNode(ctx context.Context, nodeName, gwName string, isClear bool) error {
	var node corev1.Node
	err := r.Client.Get(ctx, types.NamespacedName{Name: nodeName}, &node)
	if err != nil {
		return fmt.Errorf("failed get nodes %s for gateway %s, error %s", nodeName, gwName, err.Error())
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

func (r *ReconcileResource) getLoadBalancerIP(ctx context.Context) (publicAddr, privateAddr string, err error) {
	var cm corev1.ConfigMap
	objKey := types.NamespacedName{Name: util.RavenGlobalConfig, Namespace: util.WorkingNamespace}
	err = r.Client.Get(ctx, objKey, &cm)
	if err != nil {
		return "", "", fmt.Errorf("failed to get configmap %s, error %s", objKey.String(), err.Error())
	}
	if cm.Data == nil {
		return "", "", fmt.Errorf("failed to get loadbalancer public ip")
	}
	var ok bool
	privateAddr, ok = cm.Data[util.LoadBalancerIP]
	if !ok {
		return "", "", fmt.Errorf("failed to get loadbalancer public ip")
	}
	publicAddr, ok = cm.Data[util.ElasticIPIP]
	if !ok {
		return "", "", fmt.Errorf("failed to get loadbalancer public ip")
	}
	return
}

func (r *ReconcileResource) updateEdgeGateway(ctx context.Context, np *nodepoolv1beta1.NodePool) error {
	if len(np.Status.Nodes) == 0 {
		klog.Info(Format("nodepool %s has no node, skip manage it for gateway", np.GetName()))
		return nil
	}
	if gwName, ok := isSpecified(np); ok {
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
		err = r.labelNodesByNodePool(ctx, np, gwName, false)
		if err != nil {
			return fmt.Errorf("failed to label nodes for gateway %s, error %s", gwName, err.Error())
		}
		return nil
	}
	anno := map[string]string{InterconnectionModeAnnotationKey: getInterconnectionMode(np)}
	label := map[string]string{BelongToNodePool: np.GetName()}
	if isSoloMode(np) && r.option.GetTunnelOption() {
		for _, nodeName := range np.Status.Nodes {
			gwName := fmt.Sprintf("%s%s", GatewayPrefix, nodeName)
			var edgeGateway ravenv1beta1.Gateway
			err := r.Client.Get(ctx, types.NamespacedName{Name: gwName}, &edgeGateway)
			if err != nil && !apierrors.IsNotFound(err) {
				klog.Error(Format("failed to find gateway %s, error %s", gwName, err.Error()))
			}
			if apierrors.IsNotFound(err) {
				err = r.Client.Create(ctx, &ravenv1beta1.Gateway{
					ObjectMeta: metav1.ObjectMeta{Name: gwName, Annotations: anno, Labels: label},
					Spec: ravenv1beta1.GatewaySpec{
						ProxyConfig: ravenv1beta1.ProxyConfiguration{
							Replicas: 1,
						},
						TunnelConfig: ravenv1beta1.TunnelConfiguration{
							Replicas: 1,
						},
						Endpoints: r.generateEndpoints(nodeName, "", true),
					},
				})
				if err != nil {
					return fmt.Errorf("failed to create gateway %s, error %s", gwName, err.Error())
				}
			}
			err = r.labelNode(ctx, nodeName, gwName, false)
			if err != nil {
				return fmt.Errorf("failed to label nodes for gateway %s, error %s", gwName, err.Error())
			}
		}
	}

	if isPoolMode(np) {
		gwName := fmt.Sprintf("%s%s", GatewayPrefix, np.GetName())
		var edgeGateway ravenv1beta1.Gateway
		err := r.Client.Get(ctx, types.NamespacedName{Name: gwName}, &edgeGateway)
		if err != nil && !apierrors.IsNotFound(err) {
			klog.Error(Format("failed to create gateway %s, error %s", gwName, err.Error()))
		}
		if apierrors.IsNotFound(err) {
			endpoints := make([]ravenv1beta1.Endpoint, 0)
			endpoints = append(endpoints, r.generateEndpoints(np.Status.Nodes[0], "", true)...)
			err = r.Client.Create(ctx, &ravenv1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: gwName, Annotations: anno, Labels: label},
				Spec: ravenv1beta1.GatewaySpec{
					ProxyConfig: ravenv1beta1.ProxyConfiguration{
						Replicas: 1,
					},
					TunnelConfig: ravenv1beta1.TunnelConfiguration{
						Replicas: 1,
					},
					Endpoints: endpoints,
				},
			})
			if err != nil {
				return fmt.Errorf("failed to create gateway %s for nodepool %s, error %s", gwName, np.GetName(), err.Error())
			}
		}
		err = r.labelNodesByNodePool(ctx, np, gwName, false)
		if err != nil {
			return fmt.Errorf("failed to label nodes for gateway %s, error %s", gwName, err.Error())
		}
	}
	return nil

}

func (r *ReconcileResource) clearGateway(ctx context.Context, np *nodepoolv1beta1.NodePool, isSoloMode bool) error {
	if isSoloMode {
		for _, nodeName := range np.Status.Nodes {
			gwName := fmt.Sprintf("%s%s", GatewayPrefix, nodeName)
			err := r.Client.Delete(ctx, &ravenv1beta1.Gateway{ObjectMeta: metav1.ObjectMeta{Name: gwName}})
			if err != nil && !apierrors.IsNotFound(err) {
				return fmt.Errorf("failed to delete gateway %s, error %s", gwName, err.Error())
			}
			err = r.labelNode(ctx, nodeName, gwName, true)
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
		err = r.labelNodesByNodePool(ctx, np, "", true)
		if err != nil {
			return fmt.Errorf("failed to clear label for nodepool %s, error %s", np.GetName(), err)
		}
	}
	return nil
}

func (r *ReconcileResource) getTargetPort() (proxyPort, tunnelPort int32) {
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

func (r *ReconcileResource) generateEndpoints(nodeName, publicIP string, underNAT bool) []ravenv1beta1.Endpoint {
	proxyPort, tunnelPort := r.getTargetPort()
	endpoints := make([]ravenv1beta1.Endpoint, 0)
	endpoints = append(endpoints, ravenv1beta1.Endpoint{
		NodeName: nodeName,
		Type:     ravenv1beta1.Tunnel,
		Port:     int(tunnelPort),
		UnderNAT: underNAT,
		PublicIP: publicIP,
		Config:   map[string]string{},
	})
	endpoints = append(endpoints, ravenv1beta1.Endpoint{
		NodeName: nodeName,
		Type:     ravenv1beta1.Proxy,
		Port:     int(proxyPort),
		UnderNAT: underNAT,
		PublicIP: publicIP,
		Config:   map[string]string{},
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
