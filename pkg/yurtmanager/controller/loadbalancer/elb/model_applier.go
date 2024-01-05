package elb

import (
	"fmt"

	"k8s.io/klog/v2"

	elbmodel "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/util/cloudprovider/model/elb"
)

func NewModelApplier(elbMgr *ELBManager, eipMgr *EIPManager, lisMgr *ListenerManager, sgMgr *ServerGroupManager) *ModelApplier {
	return &ModelApplier{
		ELBMgr: elbMgr,
		EIPMgr: eipMgr,
		LisMgr: lisMgr,
		SGMgr:  sgMgr,
	}
}

type ModelApplier struct {
	ELBMgr *ELBManager
	EIPMgr *EIPManager
	LisMgr *ListenerManager
	SGMgr  *ServerGroupManager
}

func (applier *ModelApplier) Apply(reqCtx *RequestContext, pool *elbmodel.PoolIdentity, local *elbmodel.EdgeLoadBalancer) (*elbmodel.EdgeLoadBalancer, error) {

	remote, err := applier.ELBMgr.BuildRemoteModel(reqCtx, pool)
	if err != nil {
		return remote, fmt.Errorf("build rmote model, error %s", err.Error())
	}
	if err = applier.applyLoadBalancerAttribute(reqCtx, local, remote, pool.GetAction()); err != nil {
		return remote, fmt.Errorf("reconcile elb attribute error: %s", err.Error())
	}

	err = applier.EIPMgr.BuildRemoteModel(reqCtx, remote)
	if err != nil {
		return nil, fmt.Errorf("build remote model eip attribute from cloud, error %s", err.Error())
	}

	if err = applier.applyEipAttribute(reqCtx, local, remote, pool.GetAction()); err != nil {
		return remote, fmt.Errorf("reconcile eip attribute error: %s", err.Error())
	}

	// delete event has released load balancer, return
	if pool.GetAction() == elbmodel.Delete {
		return remote, nil
	}

	err = applier.SGMgr.BuildRemoteModel(reqCtx, remote)
	if err != nil {
		return nil, fmt.Errorf("build remote model server group from cloud, error %s", err.Error())
	}
	if err = applier.applyServerGroupAttribute(reqCtx, local, remote); err != nil {
		return remote, fmt.Errorf("reconcile server group error %s", err.Error())
	}

	err = applier.LisMgr.BuildRemoteModel(reqCtx, remote)
	if err != nil {
		return remote, fmt.Errorf("build remote model listeners from cloud, error %s", err.Error())
	}
	if err = applier.applyListenersAttribute(reqCtx, local, remote); err != nil {
		return remote, fmt.Errorf("reconcile listener error %s", err.Error())
	}
	return remote, nil
}

func (applier *ModelApplier) applyLoadBalancerAttribute(reqCtx *RequestContext, localModel, remoteModel *elbmodel.EdgeLoadBalancer, action elbmodel.ActionType) error {
	if localModel.NamespacedName.String() != remoteModel.NamespacedName.String() {
		return fmt.Errorf("models for different svc, local [%s], remote [%s]",
			localModel.NamespacedName.String(), remoteModel.NamespacedName.String())
	}
	switch action {
	case elbmodel.Delete:
		if remoteModel.GetLoadBalancerId() == "" {
			klog.Info(Format("elb does not exist for service %s", Key(reqCtx.Service)))
			return nil
		}
		if !remoteModel.LoadBalancerAttribute.IsUserManaged {
			err := applier.ELBMgr.DeleteELB(reqCtx, remoteModel)
			if err != nil {
				return err
			}
			klog.InfoS(fmt.Sprintf("successfully delete elb %s", remoteModel.GetLoadBalancerId()), "service", Key(reqCtx.Service))
			return nil
		}
		klog.InfoS(fmt.Sprintf("elb %s is manageed by user, skip delete it", remoteModel.GetLoadBalancerId()), "service", Key(reqCtx.Service))
	case elbmodel.Create:
		if localModel.GetLoadBalancerId() != "" {
			klog.Infof(Format("elb %s has been created for service", Key(reqCtx.Service)))
			return nil
		}
		if err := applier.ELBMgr.Create(reqCtx, localModel); err != nil {
			return fmt.Errorf("create elb error: %s", err.Error())
		}
		if err := applier.ELBMgr.waitFindLoadBalancer(reqCtx, localModel.GetLoadBalancerId(), remoteModel); err != nil {
			return fmt.Errorf("describe elb %s error: %s", localModel.GetLoadBalancerId(), err.Error())
		}
		// active load balancer
		if remoteModel.LoadBalancerAttribute.LoadBalancerStatus == ELBInActive {
			err := applier.ELBMgr.cloud.SetEdgeLoadBalancerStatus(reqCtx.Ctx, ELBActive, remoteModel)
			if err != nil {
				return fmt.Errorf("active elb error: %s", err.Error())
			}
		}
		klog.InfoS(fmt.Sprintf("successfully create elb %s", remoteModel.GetLoadBalancerId()), "service", Key(reqCtx.Service))
	case elbmodel.Update:
		// update loadbalancer
	default:

	}
	return nil
}

func (applier *ModelApplier) applyEipAttribute(reqCtx *RequestContext, localModel, remoteModel *elbmodel.EdgeLoadBalancer, action elbmodel.ActionType) error {
	if localModel.NamespacedName.String() != remoteModel.NamespacedName.String() {
		return fmt.Errorf("models for different svc, local [%s], remote [%s]",
			localModel.NamespacedName.String(), remoteModel.NamespacedName.String())
	}
	switch action {
	case elbmodel.Delete:
		if remoteModel.GetEIPId() == "" {
			klog.Info(Format("eip does not exist for service %s", Key(reqCtx.Service)))
			return nil
		}
		if !remoteModel.LoadBalancerAttribute.IsUserManaged {
			err := applier.EIPMgr.DeleteEIP(reqCtx, remoteModel)
			if err != nil {
				return err
			}
			klog.InfoS(fmt.Sprintf("successfully delete eip %s", remoteModel.GetEIPId()), "service", Key(reqCtx.Service))
			return nil
		}
		klog.InfoS(fmt.Sprintf("eip %s is manageed by user, skip delete it", remoteModel.GetEIPId()), "service", Key(reqCtx.Service))
		return nil
	case elbmodel.Create, elbmodel.Update:
		if localModel.GetAddressType() == elbmodel.IntranetAddressType {
			return nil
		}
		if localModel.LoadBalancerAttribute.IsUserManaged {
			klog.Info(Format("eip %s is managed by user, skip update it", remoteModel.GetEIPId()))
			return nil
		}
		if remoteModel.GetEIPId() == "" {
			err := applier.EIPMgr.cloud.CreateEip(reqCtx.Ctx, localModel)
			if err != nil {
				return fmt.Errorf("create elb error: %s", err.Error())
			}
			remoteModel.EipAttribute.AllocationId = localModel.GetEIPId()
			klog.InfoS(fmt.Sprintf("successfully create eip %s", remoteModel.GetEIPId()), "service", Key(reqCtx.Service))
		}
		if remoteModel.EipAttribute.InstanceId == "" {
			if err := applier.EIPMgr.AssociateEIP(reqCtx, remoteModel.GetEIPId(), remoteModel); err != nil {
				return fmt.Errorf("associate eip %s to elb %s error: %s", remoteModel.GetEIPId(), remoteModel.GetLoadBalancerId(), err.Error())
			}
			klog.InfoS(fmt.Sprintf("successfully associate eip %s to elb %s", remoteModel.GetEIPId(), remoteModel.GetLoadBalancerId()), "service", Key(reqCtx.Service))
		}
		if localModel.EipAttribute.Bandwidth != remoteModel.EipAttribute.Bandwidth {
			err := applier.EIPMgr.cloud.ModifyEipAttribute(reqCtx.Ctx, remoteModel.GetEIPId(), localModel)
			if err != nil {
				return fmt.Errorf("update eip %s attribute error: %s", remoteModel.GetEIPId(), err.Error())
			}
			klog.InfoS(fmt.Sprintf("successfully update eip %s bandwidth into %d ", remoteModel.GetEIPId(), localModel.EipAttribute.Bandwidth), "service", Key(reqCtx.Service))
		}
	default:

	}

	return nil
}

func (applier *ModelApplier) applyServerGroupAttribute(reqCtx *RequestContext, localModel, remoteModel *elbmodel.EdgeLoadBalancer) error {
	addServerGroup, removeServerGroup, updateServerGroup := getUpdateServerGroup(localModel, remoteModel)
	if err := applier.SGMgr.batchRemoveServerGroup(reqCtx, remoteModel.GetLoadBalancerId(), removeServerGroup); err != nil {
		return fmt.Errorf("batch remove server group error : %s", err.Error())
	}
	if err := applier.SGMgr.batchAddServerGroup(reqCtx, remoteModel.GetLoadBalancerId(), addServerGroup); err != nil {
		return fmt.Errorf("batch add server group error : %s", err.Error())
	}
	if err := applier.SGMgr.batchUpdateServerGroup(reqCtx, remoteModel.GetLoadBalancerId(), updateServerGroup); err != nil {
		return fmt.Errorf("batch update server group error : %s", err.Error())
	}
	return nil
}

func (applier *ModelApplier) applyListenersAttribute(reqCtx *RequestContext, localModel, remoteModel *elbmodel.EdgeLoadBalancer) error {
	addListener, removeListener, updateListener, err := getListeners(reqCtx, localModel, remoteModel)
	if err != nil {
		return fmt.Errorf("check for listener error %s", err.Error())
	}
	err = applier.LisMgr.batchRemoveListeners(reqCtx, remoteModel.GetLoadBalancerId(), &removeListener)
	if err != nil {
		return fmt.Errorf("batch remove listeners error : %s", err.Error())
	}

	err = applier.LisMgr.batchAddListeners(reqCtx, remoteModel.GetLoadBalancerId(), &addListener)
	if err != nil {
		return fmt.Errorf("batch add listeners error : %s", err.Error())
	}

	err = applier.LisMgr.batchUpdateListeners(reqCtx, remoteModel.GetLoadBalancerId(), &updateListener)
	if err != nil {
		return fmt.Errorf("batch update listeners error : %s", err.Error())
	}
	return nil
}
