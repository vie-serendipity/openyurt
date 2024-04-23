package elb

import (
	"fmt"
	"reflect"
	"strings"
	"time"

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

func (mgr *ELBManager) BuildLocalModel(reqCtx *RequestContext) (*elbmodel.EdgeLoadBalancer, error) {
	lModel := &elbmodel.EdgeLoadBalancer{
		NamespacedName: NamespacedName(reqCtx.Service),
		NamedKey: elbmodel.NamedKey{
			Prefix:      elbmodel.DEFAULT_PREFIX,
			CID:         reqCtx.ClusterId,
			Namespace:   reqCtx.Service.Namespace,
			ServiceName: reqCtx.Service.Name,
			NodePool:    reqCtx.PoolAttribute.NodePoolID,
		},
		LoadBalancerAttribute: elbmodel.EdgeLoadBalancerAttribute{},
		EipAttribute:          elbmodel.EdgeEipAttribute{},
		ServerGroup:           elbmodel.EdgeServerGroup{},
		Listeners:             elbmodel.EdgeListeners{},
	}

	if err := mgr.setELBFromDefaultConfig(reqCtx, lModel); err != nil {
		return nil, err
	}
	if err := mgr.setELBFromAnnotation(reqCtx, lModel); err != nil {
		return nil, err
	}

	return lModel, nil
}

func (mgr *ELBManager) BuildRemoteModel(reqCtx *RequestContext) (*elbmodel.EdgeLoadBalancer, error) {
	rModel := &elbmodel.EdgeLoadBalancer{
		NamedKey: elbmodel.NamedKey{
			Prefix:      elbmodel.DEFAULT_PREFIX,
			CID:         reqCtx.ClusterId,
			Namespace:   reqCtx.Service.Namespace,
			ServiceName: reqCtx.Service.Name,
			NodePool:    reqCtx.PoolAttribute.NodePoolID,
		},
	}
	rModel.NamespacedName = NamespacedName(reqCtx.Service)
	rModel.LoadBalancerAttribute.LoadBalancerId = reqCtx.AnnoCtx.Get(LoadBalancerId)
	rModel.LoadBalancerAttribute.LoadBalancerName = GetDefaultLoadBalancerName(rModel)
	rModel.LoadBalancerAttribute.NetworkId = reqCtx.PoolAttribute.VPC()
	rModel.LoadBalancerAttribute.IsUserManaged = reqCtx.AnnoCtx.IsManageByUser()
	rModel.LoadBalancerAttribute.IsShared = reqCtx.AnnoCtx.IsShared()
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

	if mdl.LoadBalancerAttribute.LoadBalancerSpec == "" {
		mdl.LoadBalancerAttribute.LoadBalancerSpec = elbmodel.ELBDefaultSpec
	}

	if mdl.LoadBalancerAttribute.PayType == "" {
		mdl.LoadBalancerAttribute.PayType = elbmodel.ELBDefaultPayType
	}

	if mdl.LoadBalancerAttribute.LoadBalancerName == "" {
		mdl.LoadBalancerAttribute.LoadBalancerName = GetDefaultLoadBalancerName(mdl)
	}
	return nil
}

func (mgr *ELBManager) setELBFromDefaultConfig(reqCtx *RequestContext, mdl *elbmodel.EdgeLoadBalancer) error {
	mdl.LoadBalancerAttribute.LoadBalancerName = GetDefaultLoadBalancerName(mdl)
	mdl.LoadBalancerAttribute.NetworkId = reqCtx.PoolAttribute.VPC()
	mdl.LoadBalancerAttribute.VSwitchId = reqCtx.PoolAttribute.VSwitch()
	mdl.LoadBalancerAttribute.EnsRegionId = reqCtx.PoolAttribute.Region()
	mdl.LoadBalancerAttribute.PayType = elbmodel.ELBDefaultPayType
	mdl.LoadBalancerAttribute.LoadBalancerSpec = elbmodel.ELBDefaultSpec
	mdl.LoadBalancerAttribute.AddressType = elbmodel.InternetAddressType
	mdl.LoadBalancerAttribute.IsUserManaged = false
	mdl.LoadBalancerAttribute.IsShared = false
	err := mgr.cloud.DescribeNetwork(reqCtx.Ctx, mdl)
	if err != nil {
		return fmt.Errorf("incorrect network attribute, network: [%s], region: [%s], vswitch: [%s], error %s", mdl.GetNetworkId(), mdl.GetRegionId(), mdl.GetVSwitchId(), err.Error())
	}
	return nil
}

func (mgr *ELBManager) setELBFromAnnotation(reqCtx *RequestContext, mdl *elbmodel.EdgeLoadBalancer) error {
	mdl.LoadBalancerAttribute.IsUserManaged = reqCtx.AnnoCtx.IsManageByUser()
	mdl.LoadBalancerAttribute.IsShared = reqCtx.AnnoCtx.IsShared()
	lbId := reqCtx.AnnoCtx.Get(LoadBalancerId)
	if lbId != "" {
		mdl.LoadBalancerAttribute.LoadBalancerId = lbId
	} else {
		if vsw := reqCtx.AnnoCtx.Get(VSwitch); vsw != "" {
			mdl.LoadBalancerAttribute.VSwitchId = vsw
		}
		if spec := reqCtx.AnnoCtx.Get(Spec); spec != "" {
			mdl.LoadBalancerAttribute.LoadBalancerSpec = spec
		}
		if payType := reqCtx.AnnoCtx.Get(PayType); payType != "" {
			mdl.LoadBalancerAttribute.PayType = payType
		}
	}
	if addressType := reqCtx.AnnoCtx.Get(AddressType); addressType != "" {
		mdl.LoadBalancerAttribute.AddressType = addressType
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
	waitErr := RetryImmediateOnError(10*time.Second, time.Minute, canSkipError, func() error {
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
