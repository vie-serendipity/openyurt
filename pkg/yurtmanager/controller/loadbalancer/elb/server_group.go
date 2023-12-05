package elb

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	v1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	prvd "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/util/cloudprovider"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/util/cloudprovider/model/elb"
)

func NewServerGroupManager(kubeClient client.Client, cloud prvd.Provider) *ServerGroupManager {
	manager := &ServerGroupManager{
		kubeClient: kubeClient,
		cloud:      cloud,
	}
	return manager
}

type ServerGroupManager struct {
	kubeClient client.Client
	cloud      prvd.Provider
}

func (mgr *ServerGroupManager) BuildLocalModel(reqCtx *RequestContext, poolId string, lModel *elb.EdgeLoadBalancer) error {
	candidates, err := NewEdgeEndpoints(reqCtx, mgr.kubeClient, poolId)
	if err != nil {
		return fmt.Errorf("build edge endpoints error: %s", err.Error())
	}
	candidates.Nodes, candidates.Ens, err = mgr.getEdgeNetworkENSNode(reqCtx, candidates, lModel)
	if err != nil {
		return fmt.Errorf("get ens nodes error: %s", err.Error())
	}

	lModel.ServerGroup, err = mgr.buildLocalServerGroup(reqCtx, candidates)
	if err != nil {
		return fmt.Errorf("build local server group error: %s", err.Error())
	}

	return nil
}

func (mgr *ServerGroupManager) BuildRemoteModel(reqCtx *RequestContext, mdl *elb.EdgeLoadBalancer) error {
	return mgr.cloud.FindBackendFromLoadBalancer(reqCtx.Ctx, mdl.GetLoadBalancerId(), &mdl.ServerGroup)
}

func (mgr *ServerGroupManager) getEdgeNetworkENSNode(reqCtx *RequestContext, candidates *EdgeEnsEndpoint, mdl *elb.EdgeLoadBalancer) ([]v1.Node, map[string]string, error) {
	nodes := make([]v1.Node, 0)
	ens := make(map[string]string, 0)
	ensMap, err := mgr.cloud.FindEnsInstancesByNetwork(reqCtx.Ctx, mdl)
	if err != nil {
		return nodes, ens, fmt.Errorf("find ens instances by network %s error: %s", mdl.GetNetworkId(), err.Error())
	}
	for _, node := range candidates.Nodes {
		ensId := node.Labels[EnsNodeId]
		if ensId != "" {
			status, ok := ensMap[ensId]
			if ok && status == ENSRunning {
				nodes = append(nodes, node)
				ens[node.Name] = ensId
			}
		}
	}
	return nodes, ens, nil
}

func (mgr *ServerGroupManager) buildLocalServerGroup(reqCtx *RequestContext, candidates *EdgeEnsEndpoint) (elb.EdgeServerGroup, error) {
	var err error
	ret := new(elb.EdgeServerGroup)

	switch candidates.TrafficPolicy {
	case LocalTrafficPolicy:
		klog.InfoS("local mode, build backends for loadbalancer", "service", Key(reqCtx.Service))
		ret.Backends, err = mgr.buildLocalBackends(reqCtx, candidates)
		if err != nil {
			return *ret, fmt.Errorf("build local backends error: %s", err.Error())
		}
	case ClusterTrafficPolicy:
		klog.InfoS("cluster mode, build backends for loadbalancer", "service", Key(reqCtx.Service))
		ret.Backends, err = mgr.buildClusterBackends(reqCtx, candidates)
		if err != nil {
			return *ret, fmt.Errorf("build cluster backends error: %s", err.Error())
		}
	}
	return *ret, nil
}

func (mgr *ServerGroupManager) buildLocalBackends(reqCtx *RequestContext, candidates *EdgeEnsEndpoint) ([]elb.EdgeBackendAttribute, error) {
	ret := make([]elb.EdgeBackendAttribute, 0)
	var initBackends []elb.EdgeBackendAttribute
	initBackends = setBackendFromEndpointSlice(reqCtx, candidates)
	candidateBackends := getCandidateBackend(candidates)
	for _, backend := range initBackends {
		if _, ok := candidateBackends[backend.ServerId]; ok {
			ret = append(ret, backend)
		}
	}
	return ret, nil
}

func (mgr *ServerGroupManager) buildClusterBackends(reqCtx *RequestContext, candidates *EdgeEnsEndpoint) ([]elb.EdgeBackendAttribute, error) {
	ret := make([]elb.EdgeBackendAttribute, 0)
	weightMap := splitBackendWeight(reqCtx)
	for _, node := range candidates.Nodes {
		if node.Name == "" || candidates.Ens[node.Name] == "" {
			klog.Warningf("[%s], Invalid node without ens id", Key(reqCtx.Service))
			continue
		}
		ret = append(ret, elb.EdgeBackendAttribute{
			ServerId: candidates.Ens[node.Name],
			Type:     elb.ServerGroupDefaultType,
			Weight:   getBackendWeight(node.Name, weightMap),
			Port:     elb.ServerGroupDefaultPort,
		})
	}
	return ret, nil
}

func (mgr *ServerGroupManager) batchAddServerGroup(reqCtx *RequestContext, lbId string, sg *elb.EdgeServerGroup) error {
	iter := int(math.Ceil(float64(len(sg.Backends)) / float64(ENSBatchAddMaxNumber)))
	for i := 0; i < iter; i++ {
		subSg := new(elb.EdgeServerGroup)
		if i == iter-1 {
			subSg.Backends = sg.Backends[i*ENSBatchAddMaxNumber:]
		} else {
			subSg.Backends = sg.Backends[i*ENSBatchAddMaxNumber : (i+1)*ENSBatchAddMaxNumber]
		}
		err := mgr.cloud.AddBackendToEdgeServerGroup(reqCtx.Ctx, lbId, subSg)
		if err != nil {
			return err
		}
	}

	return nil
}

func (mgr *ServerGroupManager) batchUpdateServerGroup(reqCtx *RequestContext, lbId string, sg *elb.EdgeServerGroup) error {

	iter := int(math.Ceil(float64(len(sg.Backends)) / float64(ENSBatchAddMaxNumber)))
	for i := 0; i < iter; i++ {
		subSg := new(elb.EdgeServerGroup)
		if i == iter-1 {
			subSg.Backends = sg.Backends[i*ENSBatchAddMaxNumber:]
		} else {
			subSg.Backends = sg.Backends[i*ENSBatchAddMaxNumber : (i+1)*ENSBatchAddMaxNumber]
		}
		err := mgr.cloud.UpdateEdgeServerGroup(reqCtx.Ctx, lbId, subSg)
		if err != nil {
			return err
		}
	}
	return nil
}

func (mgr *ServerGroupManager) batchRemoveServerGroup(reqCtx *RequestContext, lbId string, sg *elb.EdgeServerGroup) error {
	iter := int(math.Ceil(float64(len(sg.Backends)) / float64(ENSBatchAddMaxNumber)))
	for i := 0; i < iter; i++ {
		subSg := new(elb.EdgeServerGroup)
		if i == iter-1 {
			subSg.Backends = sg.Backends[i*ENSBatchAddMaxNumber:]
		} else {
			subSg.Backends = sg.Backends[i*ENSBatchAddMaxNumber : (i+1)*ENSBatchAddMaxNumber]
		}
		err := mgr.cloud.RemoveBackendFromEdgeServerGroup(reqCtx.Ctx, lbId, subSg)
		if err != nil {
			return err
		}
	}
	return nil
}

func setBackendFromEndpointSlice(reqCtx *RequestContext, candidates *EdgeEnsEndpoint) []elb.EdgeBackendAttribute {
	ret := make([]elb.EdgeBackendAttribute, 0)
	endpointMap := make(map[string]struct{})
	weightMap := splitBackendWeight(reqCtx)

	for _, es := range candidates.EndpointSlices {
		for _, ep := range es.Endpoints {
			if ep.Conditions.Ready == nil || !*ep.Conditions.Ready {
				continue
			}
			if *ep.NodeName == "" {
				klog.Warningf("[%s], Invalid endpoints without node name and ens id", Key(reqCtx.Service))
				continue
			}
			if _, ok := endpointMap[*ep.NodeName]; ok {
				continue
			}
			if candidates.Ens[*ep.NodeName] == "" {
				continue
			}
			endpointMap[*ep.NodeName] = struct{}{}
			ret = append(ret, elb.EdgeBackendAttribute{
				ServerId: candidates.Ens[*ep.NodeName],
				Type:     elb.ServerGroupDefaultType,
				Weight:   getBackendWeight(*ep.NodeName, weightMap),
				Port:     elb.ServerGroupDefaultPort,
			})
		}
	}
	return ret
}

func getCandidateBackend(candidates *EdgeEnsEndpoint) map[string]struct{} {
	ret := make(map[string]struct{})
	for _, node := range candidates.Nodes {
		ret[candidates.Ens[node.Name]] = struct{}{}
	}
	return ret
}

func getUpdateServerGroup(localModel, remoteModel *elb.EdgeLoadBalancer) (addServerGroup, removeServerGroup, updateServerGroup *elb.EdgeServerGroup) {
	addServerGroup = new(elb.EdgeServerGroup)
	removeServerGroup = new(elb.EdgeServerGroup)
	updateServerGroup = new(elb.EdgeServerGroup)
	addServerGroup.Backends = make([]elb.EdgeBackendAttribute, 0)
	removeServerGroup.Backends = make([]elb.EdgeBackendAttribute, 0)
	updateServerGroup.Backends = make([]elb.EdgeBackendAttribute, 0)

	if len(remoteModel.ServerGroup.Backends) == 0 {
		addServerGroup.Backends = append(addServerGroup.Backends, localModel.ServerGroup.Backends...)
		return
	}
	remoteServerMap := make(map[string]int)
	localServerMap := make(map[string]int)
	for idx, remoteServer := range remoteModel.ServerGroup.Backends {
		remoteServerMap[remoteServer.ServerId] = idx
	}
	for idx, localServer := range localModel.ServerGroup.Backends {
		localServerMap[localServer.ServerId] = idx
	}

	for _, localBackend := range localModel.ServerGroup.Backends {
		if _, ok := remoteServerMap[localBackend.ServerId]; !ok {
			addServerGroup.Backends = append(addServerGroup.Backends, localBackend)
			continue
		}
		if idx, ok := remoteServerMap[localBackend.ServerId]; ok {
			if localBackend.Weight != remoteModel.ServerGroup.Backends[idx].Weight {
				updateServerGroup.Backends = append(updateServerGroup.Backends, localBackend)
			}
		}
	}
	for _, remoteBackend := range remoteModel.ServerGroup.Backends {
		if _, ok := localServerMap[remoteBackend.ServerId]; !ok {
			removeServerGroup.Backends = append(removeServerGroup.Backends, remoteBackend)
		}
	}
	return
}

func splitBackendWeight(reqCtx *RequestContext) map[string]int {
	str := reqCtx.AnnoCtx.Get(EdgeServerWeight)
	ret := make(map[string]int)
	if str == "" {
		return ret
	}
	kv := strings.Split(str, ",")
	for _, e := range kv {
		r := strings.Split(e, "=")
		v, err := strconv.Atoi(r[1])
		if err != nil {
			klog.ErrorS(err, fmt.Sprintf("parse backend weight [%s] error %s", e, err.Error()), "service", Key(reqCtx.Service))
			v = elb.ServerGroupDefaultServerWeight
		}
		if v < 0 || v > 100 {
			v = elb.ServerGroupDefaultServerWeight
		}
		ret[r[0]] = v
	}
	return ret
}

func getBackendWeight(name string, weights map[string]int) int {
	ret := elb.ServerGroupDefaultServerWeight
	if w, ok := weights[BaseBackendWeight]; ok {
		ret = w
	}
	if w, ok := weights[name]; ok {
		ret = w
	}
	return ret
}
