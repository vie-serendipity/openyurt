package cloudprovider

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/raven/util/model"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/raven/util/provider/eip"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/raven/util/provider/slb"
)

func (r *ReconcileResource) DescribeSLBByName() error {
	err := r.slbClient.DescribeLoadBalancers(context.TODO(), r.remoteModel.SLBModel)
	if err != nil {
		return err
	}
	return nil
}

func (r *ReconcileResource) DescribeACLByName() error {
	err := r.slbClient.DescribeAccessControlLists(context.TODO(), r.remoteModel.ACLModel)
	if err != nil {
		return err
	}
	return nil
}

func (r *ReconcileResource) DescribeEIPByName() error {
	err := r.eipClient.DescribeEipAddresses(context.TODO(), r.remoteModel.EIPModel)
	if err != nil {
		return err
	}
	return nil
}

func (r *ReconcileResource) DescribeSLBById() error {
	err := r.slbClient.DescribeLoadBalancer(context.TODO(), r.localModel.SLBModel)
	if err != nil {
		return err
	}
	return nil
}

func (r *ReconcileResource) DescribeACLById() error {
	err := r.slbClient.DescribeAccessControlListAttribute(context.TODO(), r.localModel.ACLModel)
	if err != nil {
		return err
	}
	return nil
}

func (r *ReconcileResource) DescribeEIPById() error {
	err := r.eipClient.DescribeEipAddresses(context.TODO(), r.localModel.EIPModel)
	if err != nil {
		return err
	}
	return nil
}

func (r *ReconcileResource) CreateSLB() error {
	waitErr := wait.PollImmediate(5*time.Second, time.Minute, func() (done bool, err error) {
		err = r.slbClient.CreateLoadBalancer(context.TODO(), r.localModel.SLBModel)
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
		err = r.slbClient.TagResource(context.TODO(), &r.localModel.SLBModel.Tags, &model.Instance{
			InstanceId:     r.localModel.SLBModel.LoadBalancerId,
			InstanceType:   slb.LoadBalancerResourceType,
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
		err = r.slbClient.CreateAccessControlList(context.TODO(), r.localModel.ACLModel)
		if err != nil {
			klog.Errorf(Format("create acl error %s", err.Error()))
			return false, nil
		}
		return true, nil
	})
	if waitErr != nil {
		return fmt.Errorf("create acl error %s", waitErr.Error())
	}

	waitErr = wait.PollImmediate(5*time.Second, time.Minute, func() (done bool, err error) {
		err = r.slbClient.TagResource(context.TODO(), &r.localModel.ACLModel.Tags, &model.Instance{
			InstanceId:     r.localModel.ACLModel.AccessControlListId,
			InstanceType:   slb.AccessControlListResourceType,
			InstanceRegion: r.localModel.ACLModel.Region,
		})
		if err != nil {
			klog.Errorf(Format("tag acl error %s", err.Error()))
			return false, nil
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
		err = r.eipClient.AllocateEipAddress(context.TODO(), r.localModel.EIPModel)
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
		err = r.eipClient.AssociateEipAddress(context.TODO(), r.localModel.EIPModel, r.localModel.SLBModel.LoadBalancerId)
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

func (r *ReconcileResource) CleanupSLB(model *model.LoadBalancerAttribute) error {
	if model.NamedKey.String() != model.Name {
		klog.Infoln(Format("slb %s is not created by ecm, skip cleanup it", r.localModel.SLBModel.LoadBalancerId))
		return nil
	}
	waitErr := wait.PollImmediate(5*time.Second, time.Minute, func() (done bool, err error) {
		err = r.slbClient.DeleteLoadBalancer(context.TODO(), model)
		if err != nil && !slb.IsNotFound(err) {
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

func (r *ReconcileResource) CleanupACL(model *model.AccessControlListAttribute) error {
	if model.NamedKey.String() != model.Name {
		klog.Infoln(Format("acl %s is not created by ecm, skip cleanup it", r.localModel.ACLModel.AccessControlListId))
		return nil
	}
	waitErr := wait.PollImmediate(5*time.Second, time.Minute, func() (done bool, err error) {
		err = r.slbClient.DeleteAccessControlList(context.TODO(), model)
		if err != nil && !slb.IsNotFound(err) {
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

func (r *ReconcileResource) CleanupEIP(model *model.ElasticIPAttribute) error {
	if model.NamedKey.String() != model.Name {
		klog.Infoln(Format("eip %s is not created by ecm, skip cleanup it", r.localModel.EIPModel.AllocationId))
		return nil
	}
	waitErr := wait.PollImmediate(10*time.Second, time.Minute, func() (done bool, err error) {
		err = r.eipClient.DescribeEipAddresses(context.TODO(), r.localModel.EIPModel)
		if err != nil {
			if eip.IsNotFound(err) {
				return true, nil
			}
			klog.Error(Format("delete eip error %s", err.Error()))
			return false, nil
		}
		if r.localModel.EIPModel.InstanceId != "" || r.localModel.EIPModel.Status == "InUse" {
			err = r.eipClient.UnassociateEipAddress(context.TODO(), model)
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
		err = r.eipClient.ReleaseEipAddress(context.TODO(), r.localModel.EIPModel)
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
