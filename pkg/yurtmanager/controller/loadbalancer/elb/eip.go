package elb

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"k8s.io/klog/v2"

	prvd "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/util/cloudprovider"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/util/cloudprovider/alibaba/elb"
	elbmodel "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/util/cloudprovider/model/elb"
)

func NewEIPManager(cloud prvd.Provider) *EIPManager {
	return &EIPManager{
		cloud: cloud,
	}
}

type EIPManager struct {
	cloud prvd.Provider
}

func (mgr *EIPManager) BuildLocalModel(reqCtx *RequestContext, lModel *elbmodel.EdgeLoadBalancer) error {
	if err := setEIPFromDefaultConfig(reqCtx, lModel); err != nil {
		return fmt.Errorf("set default eip attribute error, %s", err.Error())
	}

	if err := setEipFromAnnotation(reqCtx, lModel); err != nil {
		return fmt.Errorf("set default eip attribute error, %s", err.Error())
	}
	return nil
}

func setEIPFromDefaultConfig(reqCtx *RequestContext, mdl *elbmodel.EdgeLoadBalancer) error {
	mdl.EipAttribute.Name = GetDefaultLoadBalancerName(reqCtx)
	description := elbmodel.NamedKey{
		Prefix:      elbmodel.DEFAULT_PREFIX,
		CID:         GetCID(reqCtx),
		Namespace:   reqCtx.Service.Namespace,
		ServiceName: reqCtx.Service.Name,
	}
	mdl.EipAttribute.Description = description.String()
	if mdl.LoadBalancerAttribute.EnsRegionId == "" {
		return fmt.Errorf("eip lacks ens region id")
	}
	mdl.EipAttribute.EnsRegionId = mdl.LoadBalancerAttribute.EnsRegionId
	mdl.EipAttribute.Bandwidth = elbmodel.EipDefaultBandwidth
	mdl.EipAttribute.InstanceChargeType = elbmodel.EipDefaultInstanceChargeType
	mdl.EipAttribute.InternetChargeType = elbmodel.EipDefaultInternetChargeType
	return nil
}

func setEipFromAnnotation(reqCtx *RequestContext, mdl *elbmodel.EdgeLoadBalancer) error {
	if reqCtx.AnnoCtx.Get(EipBandwidth) != "" {
		bandwidth, err := strconv.Atoi(reqCtx.AnnoCtx.Get(EipBandwidth))
		if err != nil {
			return fmt.Errorf("Annotation eip bandwidth must be integer, but got [%s]. message=[%s] ",
				reqCtx.AnnoCtx.Get(EipBandwidth), err.Error())
		}
		mdl.EipAttribute.Bandwidth = bandwidth
	}
	if reqCtx.AnnoCtx.Get(EipInstanceChargeType) != "" {
		mdl.EipAttribute.InstanceChargeType = reqCtx.AnnoCtx.Get(EipInstanceChargeType)
	}
	if reqCtx.AnnoCtx.Get(EipInternetChargeType) != "" {
		mdl.EipAttribute.InternetChargeType = reqCtx.AnnoCtx.Get(EipInternetChargeType)
	}
	ISP := reqCtx.AnnoCtx.Get(EipInternetProviderService)
	if ISP != "" {
		if !strings.Contains(mdl.EipAttribute.EnsRegionId, ISP) {
			return fmt.Errorf("eip ISP must be support ens region, ISP=%s, EnsRegion=%s", ISP, mdl.EipAttribute.EnsRegionId)
		}
		mdl.EipAttribute.InternetProviderService = ISP
	}
	return nil
}

func (mgr *EIPManager) BuildRemoteModel(reqCtx *RequestContext, rModel *elbmodel.EdgeLoadBalancer) error {
	rModel.EipAttribute.Name = GetDefaultLoadBalancerName(reqCtx)
	rModel.EipAttribute.EnsRegionId = rModel.LoadBalancerAttribute.EnsRegionId
	if rModel.LoadBalancerAttribute.IsUserManaged {
		if rModel.GetLoadBalancerId() != "" {
			err := mgr.cloud.DescribeEnsEipByInstanceId(reqCtx.Ctx, rModel.GetLoadBalancerId(), rModel)
			if err != nil && !elb.IsNotFound(err) {
				return err
			}
		}
	} else {
		err := mgr.cloud.DescribeEnsEipByName(reqCtx.Ctx, GetDefaultLoadBalancerName(reqCtx), rModel)
		if err != nil && !elb.IsNotFound(err) {
			return err
		}
	}
	return nil
}

func (mgr *EIPManager) DeleteEIP(reqCtx *RequestContext, mdl *elbmodel.EdgeLoadBalancer) error {
	waitErr := RetryImmediateOnError(10*time.Second, 2*time.Minute, canSkipError, func() error {
		err := mgr.cloud.DescribeEnsEipById(reqCtx.Ctx, mdl.GetEIPId(), mdl)
		if err != nil {
			klog.Errorf("delete eip [%s] error: %s", mdl.GetEIPId(), err.Error())
		}
		if mdl.EipAttribute.Status != EipAvailable {
			return fmt.Errorf("eip [%s] status %s", mdl.GetEIPId(), StatusAberrant)
		} else {
			return nil
		}
	})
	if waitErr != nil {
		return fmt.Errorf("delete eip [%s] error: eip status aberrant, can not release it", mdl.GetEIPId())
	}
	err := mgr.cloud.ReleaseEip(reqCtx.Ctx, mdl)
	if err != nil {
		return fmt.Errorf("delete eip [%s] error: %s", mdl.GetEIPId(), err.Error())
	}
	return nil
}

func (mgr *EIPManager) AssociateEIP(reqCtx *RequestContext, eipId string, mdl *elbmodel.EdgeLoadBalancer) error {
	waitErr := RetryImmediateOnError(10*time.Second, 2*time.Minute, canSkipError, func() error {
		if eipId == "" {
			return fmt.Errorf("service %s %s eip", InstanceNotFound, mdl.NamespacedName.String())
		}
		err := mgr.cloud.DescribeEnsEipById(reqCtx.Ctx, eipId, mdl)
		if err != nil {
			return err
		}
		switch mdl.EipAttribute.Status {
		case EipAvailable:
			err = mgr.cloud.AssociateElbEipAddress(reqCtx.Ctx, eipId, mdl.GetLoadBalancerId())
			if err != nil {
				return fmt.Errorf("associate eip %s to elb  %s error: %s", eipId, mdl.GetLoadBalancerId(), err.Error())
			}
		case EipInUse:
			if mdl.EipAttribute.InstanceId != mdl.GetLoadBalancerId() {
				klog.Errorf("eip [id: %s] should associate to elb [id: %s], but it has been associated to elb [id: %s]",
					mdl.GetEIPId(), mdl.GetLoadBalancerId(), mdl.EipAttribute.InstanceId)
				return fmt.Errorf("eip [id: %s] associated incorrect elb %s", mdl.GetEIPId(), mdl.EipAttribute.InstanceId)
			}
			return nil
		default:
			klog.Warningf("[id: %s] unknown eip status %s", eipId, mdl.EipAttribute.Status)
		}
		return nil
	})
	if waitErr != nil {
		return fmt.Errorf("failed to find eip %s", mdl.GetLoadBalancerId())
	}
	return nil
}
