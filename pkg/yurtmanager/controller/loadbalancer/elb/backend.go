package elb

import (
	"context"
	"fmt"
	"strings"

	v1 "k8s.io/api/core/v1"
	discovery "k8s.io/api/discovery/v1"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	elbmodel "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/util/cloudprovider/model/elb"
)

// Edge Backend
const (
	EdgeNodeLabel   = "alibabacloud.com/is-edge-worker"
	NodePoolLabel   = "alibabacloud.com/nodepool-id"
	EdgeNodeValue   = "true"
	ENSLabel        = "alibabacloud.com/ens-instance-id"
	LabelNodeTypeVK = "virtual-kubelet"
)

type TrafficPolicy string

const (
	// LocalTrafficPolicy externalTrafficPolicy=Local
	LocalTrafficPolicy = TrafficPolicy("Local")
	// ClusterTrafficPolicy externalTrafficPolicy=Cluster
	ClusterTrafficPolicy = TrafficPolicy("Cluster")
)

// EndpointWithENI
// Currently, EndpointWithENI accept two kind of backend
// normal nodes of type []*v1.Node, and endpoints of type *v1.Endpoints
type EdgeEnsEndpoint struct {
	// TrafficPolicy
	// external traffic policy.
	TrafficPolicy TrafficPolicy
	// AddressIPVersion
	// it indicates the address ip version type of the backends attached to the LoadBalancer
	AddressIPVersion string
	// Nodes
	// contains all the candidate nodes consider of LoadBalance Backends.
	// Cloud implementation has the right to make any filter on it.
	Nodes []v1.Node

	// Ens
	// record the ens id of nodes
	Ens map[string]string
	// Endpoints
	// It is the direct pod location information which cloud implementation
	// may needed for some kind of filtering. eg. direct ENI attach.
	Endpoints *v1.Endpoints
	// EndpointSlices
	// contains all the endpointslices of a service
	EndpointSlices []discovery.EndpointSlice
}

// NewEdgeEndpoints Collect nodes that meet the conditions as the back end of the ELB Service
func NewEdgeEndpoints(reqCtx *RequestContext, kubeClient client.Client, nodepool string) (edgeBackend *EdgeEnsEndpoint, err error) {
	edgeBackend = new(EdgeEnsEndpoint)
	edgeBackend.setTrafficPolicy(reqCtx)
	edgeBackend.setAddressIpVersion(reqCtx)
	if edgeBackend.AddressIPVersion == elbmodel.IPv6 {
		return edgeBackend, fmt.Errorf("the backend of elb can not adopt IPv6 address")
	}

	edgeBackend.Nodes, err = getEdgeENSNodes(reqCtx, kubeClient, nodepool)
	if err != nil {
		return nil, fmt.Errorf("get nodes error: %s", err.Error())
	}

	edgeBackend.EndpointSlices, err = getEndpointByEndpointSlice(reqCtx, kubeClient, edgeBackend.AddressIPVersion)
	if err != nil {
		return nil, fmt.Errorf("get endpointslice error: %s", err.Error())
	}
	klog.InfoS("backend details", "endpointslices", LogEndpointSliceList(edgeBackend.EndpointSlices), "service", Key(reqCtx.Service))

	return edgeBackend, nil
}

func (e *EdgeEnsEndpoint) setTrafficPolicy(reqCtx *RequestContext) {
	if reqCtx.Service.Spec.ExternalTrafficPolicy == v1.ServiceExternalTrafficPolicyTypeLocal {
		e.TrafficPolicy = LocalTrafficPolicy
		return
	}
	e.TrafficPolicy = ClusterTrafficPolicy
}

func (e *EdgeEnsEndpoint) setAddressIpVersion(reqCtx *RequestContext) {
	if reqCtx.AnnoCtx.Get(IPVersion) == elbmodel.IPv6 {
		e.AddressIPVersion = elbmodel.IPv6
	}
	e.AddressIPVersion = elbmodel.IPv4
}

func getEndpointByEndpointSlice(reqCtx *RequestContext, kubeClient client.Client, ipVersion string) ([]discovery.EndpointSlice, error) {
	epsList := &discovery.EndpointSliceList{}
	err := kubeClient.List(context.Background(), epsList, client.MatchingLabels{
		discovery.LabelServiceName: reqCtx.Service.Name,
	}, client.InNamespace(reqCtx.Service.Namespace))
	if err != nil {
		return nil, err
	}

	addressType := discovery.AddressTypeIPv4
	if ipVersion == elbmodel.IPv6 {
		addressType = discovery.AddressTypeIPv6
	}

	var ret []discovery.EndpointSlice
	for _, es := range epsList.Items {
		if es.AddressType == addressType {
			ret = append(ret, es)
		}
	}

	return ret, nil
}

func getEdgeENSNodes(reqCtx *RequestContext, kubeClient client.Client, nodepool string) ([]v1.Node, error) {
	matchLabels := make(client.MatchingLabels)
	if reqCtx.AnnoCtx.Get(BackendLabel) != "" {
		var err error
		matchLabels, err = splitBackendLabel(reqCtx.AnnoCtx.Get(BackendLabel))
		if err != nil {
			return nil, fmt.Errorf("filter nodes by label %s error: %s", reqCtx.AnnoCtx.Get(BackendLabel), err.Error())
		}
	}
	matchLabels[EdgeNodeLabel] = EdgeNodeValue
	matchLabels[NodePoolLabel] = nodepool
	nodeList := v1.NodeList{}
	err := kubeClient.List(reqCtx.Ctx, &nodeList, client.HasLabels{ENSLabel}, matchLabels)
	if err != nil {
		return nil, fmt.Errorf("get edge ens node error: %s", err.Error())
	}
	return filterOutNodes(reqCtx, nodeList.Items), nil
}

func splitBackendLabel(labels string) (map[string]string, error) {
	if labels == "" {
		return nil, nil
	}
	ret := make(map[string]string)
	labelSlice := strings.Split(labels, ",")
	for _, v := range labelSlice {
		lb := strings.Split(v, "=")
		if len(lb) != 2 {
			return ret, fmt.Errorf("parse backend label: %s, [k1=v1,k2=v2]", v)
		}
		ret[lb[0]] = lb[1]
	}
	return ret, nil
}

func filterOutNodes(reqCtx *RequestContext, nodes []v1.Node) []v1.Node {
	condidateNodes := make([]v1.Node, 0)
	for _, node := range nodes {
		if shouldSkipNode(reqCtx, &node) {
			continue
		}
		condidateNodes = append(condidateNodes, node)
	}
	return condidateNodes
}

// skip cloud, master, vk, unscheduled(when enabled), notReady nodes
func shouldSkipNode(reqCtx *RequestContext, node *v1.Node) bool {
	// need to keep the node who has exclude label in order to be compatible with vk node
	// It's safe because these nodes will be filtered in build backends func

	if isNodeExcludeFromEdgeLoadBalancer(node) {
		klog.Info("[%s] node %s is exclude node, skip adding it to lb", Key(reqCtx.Service), node.Name)
		return true
	}

	// filter unscheduled node
	if node.Spec.Unschedulable && reqCtx.AnnoCtx.Get(RemoveUnscheduled) != "" {
		if reqCtx.AnnoCtx.Get(RemoveUnscheduled) == string(elbmodel.OnFlag) {
			klog.Infof("[%s] node %s is unschedulable, skip adding to lb", Key(reqCtx.Service), node.Name)
			return true
		}
	}

	// ignore vk node condition check.
	// Even if the vk node is NotReady, it still can be added to lb. Because the eci pod that actually joins the lb, not a vk node
	if label, ok := node.Labels["type"]; ok && label == LabelNodeTypeVK {
		return true
	}

	// If we have no info, don't accept
	if len(node.Status.Conditions) == 0 {
		return true
	}

	for _, cond := range node.Status.Conditions {
		// We consider the node for load balancing only when its NodeReady
		// condition status is ConditionTrue
		if cond.Type == v1.NodeReady &&
			cond.Status != v1.ConditionTrue {
			klog.Infof("[%s] node %v with %v condition "+
				"status %v", Key(reqCtx.Service), node.Name, cond.Type, cond.Status)
			return true
		}
	}

	return false
}

func isNodeExcludeFromEdgeLoadBalancer(node *v1.Node) bool {
	if _, exclude := node.Labels[ExcludeBackendLabel]; exclude {
		return true
	}
	return false
}
