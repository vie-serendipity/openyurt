package elb

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"

	prvd "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/util/cloudprovider"
	elbmodel "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/util/cloudprovider/model/elb"
)

func NewELBManager(cloud prvd.Provider) *ELBManager {
	return &ELBManager{
		cloud: cloud,
	}
}

type ELBManager struct {
	cloud prvd.Provider
}

func (mgr *ELBManager) BuildLocalModel(reqCtx *RequestContext, pool *elbmodel.PoolIdentity) (*elbmodel.EdgeLoadBalancer, error) {
	if pool.GetNetwork() == "" || pool.GetVSwitch() == "" {
		return nil, fmt.Errorf("%s lacks annotation about edge private network or switch", Key(reqCtx.Service))
	}
	lModel := &elbmodel.EdgeLoadBalancer{}

	lModel.LoadBalancerAttribute.NetworkId = pool.GetNetwork()
	lModel.LoadBalancerAttribute.VSwitchId = pool.GetVSwitch()
	lModel.LoadBalancerAttribute.EnsRegionId = pool.GetRegion()

	if err := mgr.setELBFromDefaultConfig(reqCtx, lModel); err != nil {
		return nil, err
	}
	if err := mgr.setELBFromAnnotation(reqCtx, lModel, pool.GetNetwork()); err != nil {
		return nil, err
	}

	return lModel, nil
}

func (mgr *ELBManager) BuildRemoteModel(reqCtx *RequestContext, pool *elbmodel.PoolIdentity) (*elbmodel.EdgeLoadBalancer, error) {
	rModel := &elbmodel.EdgeLoadBalancer{}
	rModel.NamespacedName = NamespacedName(reqCtx.Service)
	rModel.LoadBalancerAttribute.LoadBalancerId = getLoadBalancerId(pool.GetNetwork(), reqCtx.AnnoCtx.Get(LoadBalancerId))
	rModel.LoadBalancerAttribute.LoadBalancerName = GetDefaultLoadBalancerName(reqCtx)
	rModel.LoadBalancerAttribute.NetworkId = pool.GetNetwork()
	if rModel.GetLoadBalancerId() != "" {
		rModel.LoadBalancerAttribute.IsUserManaged = true
	} else {
		rModel.LoadBalancerAttribute.IsUserManaged = false
	}
	err := mgr.Find(reqCtx, rModel)
	if err != nil {
		return rModel, err
	}
	return rModel, nil
}

func (mgr *ELBManager) Find(reqCtx *RequestContext, mdl *elbmodel.EdgeLoadBalancer) error {
	err := mgr.cloud.FindEdgeLoadBalancer(reqCtx.Ctx, mdl)
	if err != nil {
		// load balancer id is set empty if it can not find load balancer instance by id or name
		mdl.LoadBalancerAttribute.LoadBalancerId = ""
		return err
	}
	// active load balancer
	if mdl.GetLoadBalancerId() != "" && mdl.LoadBalancerAttribute.LoadBalancerStatus != ELBActive {
		if err := mgr.waitFindLoadBalancer(reqCtx, mdl.GetLoadBalancerId(), mdl); err != nil {
			return fmt.Errorf("describe elb %s error: %s", mdl.GetLoadBalancerId(), err.Error())
		}
		if mdl.LoadBalancerAttribute.LoadBalancerStatus == ELBInActive {
			err := mgr.cloud.SetEdgeLoadBalancerStatus(reqCtx.Ctx, ELBActive, mdl)
			if err != nil {
				return fmt.Errorf("active elb error: %s", err.Error())
			}
		}
		// update remote model
		klog.InfoS(fmt.Sprintf("successfully active elb %s", mdl.GetLoadBalancerId()), "service", Key(reqCtx.Service))
	}
	return nil
}

func (mgr *ELBManager) Create(reqCtx *RequestContext, mdl *elbmodel.EdgeLoadBalancer) error {
	if err := mgr.checkBeforeCreate(reqCtx, mdl); err != nil {
		return err
	}
	if err := mgr.cloud.CreateEdgeLoadBalancer(reqCtx.Ctx, mdl); err != nil {
		return err
	}
	return nil
}

func (mgr *ELBManager) checkBeforeCreate(reqCtx *RequestContext, mdl *elbmodel.EdgeLoadBalancer) error {
	if mdl.GetNetworkId() == "" {
		return fmt.Errorf("check error, lock network id")
	}
	if mdl.GetVSwitchId() == "" {
		return fmt.Errorf("check error, lock vswitch id")
	}
	if mdl.LoadBalancerAttribute.EnsRegionId == "" {
		err := mgr.cloud.DescribeNetwork(reqCtx.Ctx, mdl)
		if err != nil {
			return fmt.Errorf("check error, lock region id")
		}
	}
	if mdl.LoadBalancerAttribute.LoadBalancerSpec == "" {
		mdl.LoadBalancerAttribute.LoadBalancerSpec = elbmodel.ELBDefaultSpec
	}

	if mdl.LoadBalancerAttribute.PayType == "" {
		mdl.LoadBalancerAttribute.PayType = elbmodel.ELBDefaultPayType
	}

	if mdl.LoadBalancerAttribute.LoadBalancerName == "" {
		mdl.LoadBalancerAttribute.LoadBalancerName = GetDefaultLoadBalancerName(reqCtx)
	}
	return nil
}

func (mgr *ELBManager) setELBFromDefaultConfig(reqCtx *RequestContext, mdl *elbmodel.EdgeLoadBalancer) error {
	mdl.NamespacedName = NamespacedName(reqCtx.Service)
	mdl.LoadBalancerAttribute.PayType = elbmodel.ELBDefaultPayType
	mdl.LoadBalancerAttribute.LoadBalancerSpec = elbmodel.ELBDefaultSpec
	mdl.LoadBalancerAttribute.IsUserManaged = false
	mdl.LoadBalancerAttribute.IsReUse = false
	return nil
}
func (mgr *ELBManager) setELBFromAnnotation(reqCtx *RequestContext, mdl *elbmodel.EdgeLoadBalancer, vpcId string) error {
	lbId := getLoadBalancerId(vpcId, reqCtx.AnnoCtx.Get(LoadBalancerId))
	if lbId != "" {
		serverOverride, err := strconv.ParseBool(reqCtx.AnnoCtx.Get(BackendOverride))
		if err != nil || !serverOverride {
			return fmt.Errorf("%s has incorrect annotation, you must specify %s=true",
				Key(reqCtx.Service), composite(AnnotationPrefix, BackendOverride))
		}
		listenerOverride, err := strconv.ParseBool(reqCtx.AnnoCtx.Get(ListenerOverride))
		if err == nil && listenerOverride {
			if reqCtx.Service.Spec.ExternalTrafficPolicy != v1.ServiceExternalTrafficPolicyTypeCluster {
				return fmt.Errorf("%s has incorrect annotation, reused loadbalancer only support service.spce.externalTrafficPolicy=%v",
					Key(reqCtx.Service), v1.ServiceExternalTrafficPolicyTypeCluster)
			}
			mdl.LoadBalancerAttribute.IsReUse = true
		}
		mdl.LoadBalancerAttribute.LoadBalancerId = lbId
		ret, err := mgr.cloud.FindNetWorkAndVSwitchByLoadBalancerId(reqCtx.Ctx, mdl.LoadBalancerAttribute.LoadBalancerId)
		if err != nil {
			return fmt.Errorf("%s has incorrect annotation about %s to find network and vswitch, err: %s", Key(reqCtx.Service), lbId, err.Error())
		}
		mdl.LoadBalancerAttribute.NetworkId = ret[0]
		mdl.LoadBalancerAttribute.VSwitchId = ret[1]
		mdl.LoadBalancerAttribute.IsUserManaged = true

	} else {
		mdl.LoadBalancerAttribute.IsUserManaged = false
		err := mgr.cloud.DescribeNetwork(reqCtx.Ctx, mdl)
		if err != nil {
			return fmt.Errorf("%s has incorrect annotation about network id %s and vswitch id %s to get region by network, err: %s",
				Key(reqCtx.Service), mdl.GetNetworkId(), mdl.GetVSwitchId(), err.Error())
		}
		if spec := reqCtx.AnnoCtx.Get(Spec); spec != "" {
			mdl.LoadBalancerAttribute.LoadBalancerSpec = spec
		}
		if payType := reqCtx.AnnoCtx.Get(PayType); payType != "" {
			mdl.LoadBalancerAttribute.PayType = payType
		}
	}
	return nil
}

func getLoadBalancerId(vpcId, lbIds string) string {
	lbIdSlice := strings.Split(lbIds, ",")
	if lbIdSlice == nil {
		return ""
	}
	for idx := range lbIdSlice {
		keyAndValue := strings.Split(lbIdSlice[idx], "=")
		if len(keyAndValue) == 2 {
			if keyAndValue[0] == vpcId {
				return keyAndValue[1]
			}
		}
	}
	return ""
}

func (mgr *ELBManager) waitFindLoadBalancer(reqCtx *RequestContext, lbId string, mdl *elbmodel.EdgeLoadBalancer) error {
	waitErr := RetryImmediateOnError(10*time.Second, 2*time.Minute, canSkipError, func() error {
		err := mgr.cloud.DescribeEdgeLoadBalancerById(reqCtx.Ctx, lbId, mdl)
		if err != nil {
			return err
		}
		if mdl.GetLoadBalancerId() == "" {
			return fmt.Errorf("service %s %s load balancer", InstanceNotFound, mdl.NamespacedName.String())
		}
		if mdl.LoadBalancerAttribute.LoadBalancerStatus == ELBActive || mdl.LoadBalancerAttribute.LoadBalancerStatus == ELBInActive {
			return nil
		}
		return fmt.Errorf("elb %s status is %s", mdl.GetLoadBalancerId(), StatusAberrant)
	})
	if waitErr != nil {
		return fmt.Errorf("failed to find elb %s", mdl.GetLoadBalancerId())
	}
	return nil
}

func (mgr *ELBManager) DeleteELB(reqCtx *RequestContext, mdl *elbmodel.EdgeLoadBalancer) error {
	err := mgr.cloud.DeleteEdgeLoadBalancer(reqCtx.Ctx, mdl)
	if err != nil {
		return fmt.Errorf("delete elb [%s] error: %s", mdl.GetLoadBalancerId(), err.Error())
	}
	klog.InfoS(fmt.Sprintf("successfully delete elb %s", mdl.GetLoadBalancerId()), "service", Key(reqCtx.Service))
	return nil
}

func canSkipError(err error) bool {
	return strings.Contains(err.Error(), InstanceNotFound) || strings.Contains(err.Error(), StatusAberrant)
}

func isEqual(s1, s2 interface{}) bool {
	if reflect.TypeOf(s1) != reflect.TypeOf(s2) {
		return false
	}
	return reflect.DeepEqual(s1, s2)
}
