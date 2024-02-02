package cloudprovider

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/util/cloudprovider/alibaba/raven"
	ravenmodel "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/util/cloudprovider/model/raven"
)

func (r *ReconcileResource) AssociateInstance(ctx context.Context, mdl *ravenmodel.ElasticIPAttribute, instanceId string) error {
	waitErr := wait.PollImmediate(5*time.Second, time.Minute, func() (done bool, err error) {
		err = r.provider.AssociateEipAddress(ctx, mdl, instanceId)
		if err != nil {
			klog.Errorf(Format("associate eip to slb error %s", err.Error()))
			return false, nil
		}
		return true, nil
	})
	if waitErr != nil {
		return fmt.Errorf("associate eip to slb error %s", waitErr.Error())
	}
	return nil
}

func (r *ReconcileResource) CleanupSLB(ctx context.Context, model *ravenmodel.LoadBalancerAttribute) error {
	if model.NamedKey.String() != model.Name {
		klog.Infoln(Format("slb %s is not created by ecm, skip cleanup it", model.LoadBalancerId))
		return nil
	}
	err := r.provider.DeleteLoadBalancer(ctx, model)
	if err != nil && !raven.IsNotFound(err) {
		return err
	}
	klog.Infoln(Format("successfully delete slb: %s", model.LoadBalancerId))
	model.LoadBalancerId = ""
	model.Address = ""
	return nil
}

func (r *ReconcileResource) CleanupACL(ctx context.Context, model *ravenmodel.AccessControlListAttribute) error {
	if model.NamedKey.String() != model.Name {
		klog.Infoln(Format("acl %s is not created by ecm, skip cleanup it", model.AccessControlListId))
		return nil
	}
	err := r.provider.DeleteAccessControlList(ctx, model)
	if err != nil && !raven.IsNotFound(err) {
		return err
	}
	klog.Infoln(Format("successfully delete acl: %s", model.AccessControlListId))
	model.AccessControlListId = ""
	return nil
}

func (r *ReconcileResource) CleanupEIP(ctx context.Context, model *ravenmodel.ElasticIPAttribute) error {
	if model.NamedKey.String() != model.Name {
		klog.Infoln(Format("eip %s is not created by ecm, skip cleanup it", model.AllocationId))
		return nil
	}
	waitErr := wait.PollImmediate(10*time.Second, time.Minute, func() (done bool, err error) {
		err = r.provider.DescribeEipAddresses(ctx, model)
		if err != nil {
			if raven.IsNotFound(err) {
				return true, nil
			}
			klog.Error(Format("describe eip error %s", err.Error()))
			return false, nil
		}
		if model.InstanceId != "" || model.Status == "InUse" {
			err = r.provider.UnassociateEipAddress(ctx, model)
			if err != nil {
				klog.Error(Format("unassociate eip error %s", err.Error()))
				return false, nil
			}
		}
		return true, nil
	})
	if waitErr != nil {
		return fmt.Errorf("unassociate eip error %s", waitErr.Error())
	}
	err := r.provider.ReleaseEipAddress(ctx, model)
	if err != nil {
		klog.Error(Format("delete eip error %s", err.Error()))
		return err
	}
	klog.Infoln(Format("successfully delete eip: %s", model.AllocationId))
	model.AllocationId = ""
	model.Address = ""
	return nil
}
