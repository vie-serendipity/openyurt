package cloudprovider

import (
	"context"
	"fmt"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/util/cloudprovider/alibaba/raven"
	ravenmodel "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/util/cloudprovider/model/raven"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
)

func (r *ReconcileResource) DescribeSLBByName() error {
	err := r.provider.DescribeLoadBalancers(context.TODO(), r.remoteModel.SLBModel)
	if err != nil {
		return err
	}
	return nil
}

func (r *ReconcileResource) DescribeACLByName() error {
	err := r.provider.DescribeAccessControlLists(context.TODO(), r.remoteModel.ACLModel)
	if err != nil {
		return err
	}
	return nil
}

func (r *ReconcileResource) DescribeEIPByName() error {
	err := r.provider.DescribeEipAddresses(context.TODO(), r.remoteModel.EIPModel)
	if err != nil {
		return err
	}
	return nil
}

func (r *ReconcileResource) DescribeSLBById() error {
	err := r.provider.DescribeLoadBalancer(context.TODO(), r.localModel.SLBModel)
	if err != nil {
		return err
	}
	return nil
}

func (r *ReconcileResource) DescribeACLById() error {
	err := r.provider.DescribeAccessControlListAttribute(context.TODO(), r.localModel.ACLModel)
	if err != nil {
		return err
	}
	return nil
}

func (r *ReconcileResource) DescribeEIPById() error {
	err := r.provider.DescribeEipAddresses(context.TODO(), r.localModel.EIPModel)
	if err != nil {
		return err
	}
	return nil
}

func (r *ReconcileResource) CreateSLB() error {
	waitErr := wait.PollImmediate(5*time.Second, time.Minute, func() (done bool, err error) {
		err = r.provider.CreateLoadBalancer(context.TODO(), r.localModel.SLBModel)
		if err != nil {
			klog.Errorf(Format("create slb error %s", err.Error()))
			return false, nil
		}
		return true, nil
	})
	if waitErr != nil {
		return fmt.Errorf("create slb error %s", waitErr.Error())
	}
	waitErr = wait.PollImmediate(5*time.Second, time.Minute, func() (done bool, err error) {
		err = r.provider.TagResource(context.TODO(), &r.localModel.SLBModel.Tags, &ravenmodel.Instance{
			InstanceId:     r.localModel.SLBModel.LoadBalancerId,
			InstanceType:   raven.LoadBalancerResourceType,
			InstanceRegion: r.localModel.SLBModel.Region,
		})
		if err != nil {
			klog.Errorf(Format("tag slb error %s", err.Error()))
			return false, nil
		}
		return true, nil
	})
	if waitErr != nil {
		return fmt.Errorf("tag slb error %s", waitErr.Error())
	}
	return nil
}

func (r *ReconcileResource) CreateACL() error {
	waitErr := wait.PollImmediate(5*time.Second, time.Minute, func() (done bool, err error) {
		err = r.provider.CreateAccessControlList(context.TODO(), r.localModel.ACLModel)
		if err != nil {
			klog.Errorf(Format("create acl error %s", err.Error()))
			return true, nil
		}
		return true, nil
	})
	if waitErr != nil {
		return fmt.Errorf("create acl error %s", waitErr.Error())
	}

	waitErr = wait.PollImmediate(5*time.Second, time.Minute, func() (done bool, err error) {
		err = r.provider.TagResource(context.TODO(), &r.localModel.ACLModel.Tags, &ravenmodel.Instance{
			InstanceId:     r.localModel.ACLModel.AccessControlListId,
			InstanceType:   raven.AccessControlListResourceType,
			InstanceRegion: r.localModel.ACLModel.Region,
		})
		if err != nil {
			klog.Errorf(Format("tag acl error %s", err.Error()))
			return true, nil
		}
		return true, nil
	})
	if waitErr != nil {
		return fmt.Errorf("tag acl error %s", waitErr.Error())
	}
	return nil
}

func (r *ReconcileResource) CreateEIP() error {
	waitErr := wait.PollImmediate(5*time.Second, time.Minute, func() (done bool, err error) {
		err = r.provider.AllocateEipAddress(context.TODO(), r.localModel.EIPModel)
		if err != nil {
			klog.Errorf(Format("create eip error %s", err.Error()))
			return false, nil
		}
		return true, nil
	})
	if waitErr != nil {
		return fmt.Errorf("create eip error %s", waitErr.Error())
	}
	return nil
}

func (r *ReconcileResource) AssociateInstance() error {
	waitErr := wait.PollImmediate(5*time.Second, time.Minute, func() (done bool, err error) {
		err = r.provider.AssociateEipAddress(context.TODO(), r.localModel.EIPModel, r.localModel.SLBModel.LoadBalancerId)
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

func (r *ReconcileResource) CleanupSLB(model *ravenmodel.LoadBalancerAttribute) error {
	if model.NamedKey.String() != model.Name {
		klog.Infoln(Format("slb %s is not created by ecm, skip cleanup it", r.localModel.SLBModel.LoadBalancerId))
		return nil
	}
	waitErr := wait.PollImmediate(5*time.Second, time.Minute, func() (done bool, err error) {
		err = r.provider.DeleteLoadBalancer(context.TODO(), model)
		if err != nil && !raven.IsNotFound(err) {
			klog.Error(Format("delete slb error %s", err.Error()))
			return false, nil
		}
		return true, nil
	})
	if waitErr != nil {
		return fmt.Errorf("cleanup slb error %s", waitErr.Error())
	}
	return nil
}

func (r *ReconcileResource) CleanupACL(model *ravenmodel.AccessControlListAttribute) error {
	if model.NamedKey.String() != model.Name {
		klog.Infoln(Format("acl %s is not created by ecm, skip cleanup it", r.localModel.ACLModel.AccessControlListId))
		return nil
	}
	waitErr := wait.PollImmediate(5*time.Second, time.Minute, func() (done bool, err error) {
		err = r.provider.DeleteAccessControlList(context.TODO(), model)
		if err != nil && !raven.IsNotFound(err) {
			klog.Error(Format("delete acl error %s", err.Error()))
			return false, nil
		}
		return true, nil
	})
	if waitErr != nil {
		return fmt.Errorf("cleanup acl error %s", waitErr.Error())
	}
	return nil
}

func (r *ReconcileResource) CleanupEIP(model *ravenmodel.ElasticIPAttribute) error {
	if model.NamedKey.String() != model.Name {
		klog.Infoln(Format("eip %s is not created by ecm, skip cleanup it", r.localModel.EIPModel.AllocationId))
		return nil
	}
	waitErr := wait.PollImmediate(10*time.Second, time.Minute, func() (done bool, err error) {
		err = r.provider.DescribeEipAddresses(context.TODO(), r.localModel.EIPModel)
		if err != nil {
			if raven.IsNotFound(err) {
				return true, nil
			}
			klog.Error(Format("delete eip error %s", err.Error()))
			return false, nil
		}
		if r.localModel.EIPModel.InstanceId != "" || r.localModel.EIPModel.Status == "InUse" {
			err = r.provider.UnassociateEipAddress(context.TODO(), model)
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

	waitErr = wait.PollImmediate(10*time.Second, time.Minute, func() (done bool, err error) {
		err = r.provider.ReleaseEipAddress(context.TODO(), r.localModel.EIPModel)
		if err != nil {
			klog.Error(Format("delete eip error %s", err.Error()))
			return false, nil
		}
		return true, nil
	})
	if waitErr != nil {
		return fmt.Errorf("cleanup eip error %s", waitErr.Error())
	}

	return nil
}
