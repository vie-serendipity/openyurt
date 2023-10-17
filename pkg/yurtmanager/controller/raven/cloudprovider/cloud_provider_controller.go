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

package cloudprovider

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	appconfig "github.com/openyurtio/openyurt/cmd/yurt-manager/app/config"
	"github.com/openyurtio/openyurt/cmd/yurt-manager/names"
	"github.com/openyurtio/openyurt/pkg/apis/raven"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/raven/util"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/raven/util/model"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/raven/util/provider/base"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/raven/util/provider/eip"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/raven/util/provider/slb"
)

func Format(format string, args ...interface{}) string {
	s := fmt.Sprintf(format, args...)
	return fmt.Sprintf("%s: %s", names.RavenCloudProviderController, s)
}

// Add creates a new Service Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(ctx context.Context, c *appconfig.CompletedConfig, mgr manager.Manager) error {
	r, err := newReconciler(c, mgr)
	if err != nil {
		klog.Error(Format("new reconcile error: %s", err.Error()))
		return err
	}
	return add(mgr, r)
}

func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New(names.RavenCloudProviderController, mgr, controller.Options{
		Reconciler: r, MaxConcurrentReconciles: 1,
	})
	if err != nil {
		klog.Error(Format("new controller error: %s", err.Error()))
		return err
	}

	err = c.Watch(&source.Kind{Type: &corev1.Service{}}, &handler.EnqueueRequestForObject{}, predicate.NewPredicateFuncs(
		func(object client.Object) bool {
			svc, ok := object.(*corev1.Service)
			if !ok {
				return false
			}
			return isFocusedObject(svc)
		}))
	if err != nil {
		klog.Error(Format("watch resource corev1.Service error: %s", err.Error()))
		return err
	}

	return nil
}

func isFocusedObject(svc *corev1.Service) bool {
	if svc.Namespace != util.WorkingNamespace {
		return false
	}
	if svc.Spec.Type != corev1.ServiceTypeLoadBalancer {
		return false
	}
	if _, ok := svc.Labels[raven.LabelCurrentGateway]; !ok {
		return false
	}
	return true
}

var _ reconcile.Reconciler = &ReconcileResource{}

type ReconcileResource struct {
	client.Client
	scheme   *runtime.Scheme
	recorder record.EventRecorder
	queue    workqueue.RateLimitingInterface

	slbClient   *slb.SLBProvider
	eipClient   *eip.VPCProvider
	cloudClient *base.ClientMgr

	localModel  *model.Model
	remoteModel *model.Model
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(c *appconfig.CompletedConfig, mgr manager.Manager) (*ReconcileResource, error) {
	cloudClient, err := base.NewClientMgr(c.ComponentConfig.RavenCloudProviderController.CloudConfigPath)
	if err != nil {
		return nil, err
	}

	return &ReconcileResource{
		Client:      mgr.GetClient(),
		scheme:      mgr.GetScheme(),
		cloudClient: cloudClient,
		slbClient:   slb.NewLBProvider(cloudClient),
		eipClient:   eip.NewEIPProvider(cloudClient),
		queue:       workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "RavenServiceQueue"),
		recorder:    mgr.GetEventRecorderFor(names.RavenCloudProviderController),
	}, nil
}

// Reconcile reads that state of the cluster for a Gateway object and makes changes based on the state read
// and what is in the Gateway.Spec
func (r *ReconcileResource) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	klog.V(2).Info(Format("started reconciling cloud resource for configmap %s/%s", util.WorkingNamespace, util.RavenGlobalConfig))
	defer klog.V(2).Info(Format("finished reconciling cloud resource for configmap %s/%s", util.WorkingNamespace, util.RavenGlobalConfig))
	cm, err := r.getRavenConfig()
	if err != nil {
		klog.Error(Format("get configmap %s/%s, error %s", util.WorkingNamespace, util.RavenGlobalConfig, err.Error()))
		return reconcile.Result{Requeue: true, RequeueAfter: 2 * time.Second}, err
	}

	err = r.buildModel()
	if err != nil {
		klog.Error(Format("init model error: %s", err.Error()))
		return reconcile.Result{Requeue: true, RequeueAfter: 2 * time.Second}, err
	}
	err = r.loadLocalModel(cm)
	if err != nil {
		klog.Error(Format("load local model error: %s", err.Error()))
		return reconcile.Result{Requeue: true, RequeueAfter: 2 * time.Second}, err
	}

	cleanup, err := r.needCleanupResource()
	if err != nil {
		klog.Error(Format("check whether to cleanup resource error: %s", err.Error()))
		return reconcile.Result{Requeue: true, RequeueAfter: 2 * time.Second}, err
	}

	if cleanup {
		err = r.cleanupCloudResource()
		if err != nil {
			klog.Error(Format("cleanup cloud resource error %s", err.Error()))
			return reconcile.Result{Requeue: true, RequeueAfter: 2 * time.Second}, err
		}
	} else {
		err = r.createCloudResource()
		if err != nil {
			klog.Error(Format("create cloud resource error %s", err.Error()))
			return reconcile.Result{Requeue: true, RequeueAfter: 2 * time.Second}, err
		}
	}

	if cm != nil {
		err = r.updateRavenConfig(cm)
		if err != nil {
			klog.Error(Format("update configmap %s/%s error %s", cm.GetNamespace(), cm.GetName(), err.Error()))
			return reconcile.Result{Requeue: true, RequeueAfter: 2 * time.Second}, err
		}
	}
	return reconcile.Result{}, nil
}

func (r *ReconcileResource) buildModel() error {
	if r.cloudClient == nil {
		return fmt.Errorf("cloud client is not found")
	}
	clusterId, err := r.cloudClient.Meta.GetClusterID()
	if err != nil {
		return err
	}
	regionId, err := r.cloudClient.Meta.GetRegion()
	if err != nil {
		return err
	}
	vpcId, err := r.cloudClient.Meta.GetVpcID()
	if err != nil {
		return err
	}
	vswitch, err := r.cloudClient.Meta.GetVswitchID()
	if err != nil {
		return err
	}
	nameKey := model.NewNamedKey(clusterId)
	tag := map[string]string{util.ResourceUseForRavenComponentKey: util.ResourceUseForRavenComponentValue}
	r.localModel = model.NewModel(nameKey, regionId)
	r.localModel.ACLModel.Tags = model.NewTagList(tag)
	r.localModel.EIPModel.Tags = model.NewTagList(tag)
	r.localModel.SLBModel.Tags = model.NewTagList(tag)
	r.localModel.SLBModel.Spec = "slb.s2.medium"
	r.localModel.SLBModel.AddressType = "intranet"
	r.localModel.SLBModel.VpcId = vpcId
	r.localModel.SLBModel.VSwitchId = vswitch
	r.localModel.EIPModel.Bandwidth = "100"
	r.localModel.EIPModel.InstanceChargeType = "PostPaid"
	r.localModel.EIPModel.InternetChargeType = "PayByTraffic"

	r.remoteModel = model.NewModel(nameKey, regionId)
	r.remoteModel.SLBModel.VpcId = vpcId
	r.remoteModel.SLBModel.VSwitchId = vswitch
	return nil
}

func (r *ReconcileResource) loadLocalModel(cm *corev1.ConfigMap) error {
	if cm != nil && cm.Data != nil {
		r.localModel.SLBModel.LoadBalancerId = cm.Data[util.LoadBalancerId]
		r.localModel.SLBModel.Address = cm.Data[util.LoadBalancerIP]
		r.localModel.EIPModel.AllocationId = cm.Data[util.ElasticIPId]
		r.localModel.EIPModel.Address = cm.Data[util.ElasticIPIP]
		r.localModel.ACLModel.AccessControlListId = cm.Data[util.ACLId]
	}

	if r.localModel.SLBModel.LoadBalancerId != "" {
		err := r.DescribeSLBById()
		if err != nil {
			if !slb.IsNotFound(err) {
				return fmt.Errorf("describe slb %s, error %s", r.localModel.SLBModel.LoadBalancerId, err.Error())
			} else {
				klog.Warningf(Format("slb id %s is not found recreate a slb", r.localModel.SLBModel.LoadBalancerId))
				r.localModel.SLBModel.LoadBalancerId = ""
			}
		}
	}
	if r.localModel.EIPModel.AllocationId != "" {
		err := r.DescribeEIPById()
		if err != nil {
			if !eip.IsNotFound(err) {
				return fmt.Errorf("describe eip %s, error %s", r.localModel.EIPModel.AllocationId, err.Error())
			} else {
				klog.Warningf(Format("eip id %s is not found recreate a eip", r.localModel.EIPModel.AllocationId))
				r.localModel.EIPModel.AllocationId = ""
			}
		}
	}

	if r.localModel.ACLModel.AccessControlListId != "" {
		err := r.DescribeACLById()
		if err != nil {
			if !slb.IsNotFound(err) {
				return fmt.Errorf("describe acl %s, error %s", r.localModel.ACLModel.AccessControlListId, err.Error())
			} else {
				klog.Warningf(Format("acl id %s is not found recreate a slb", r.localModel.ACLModel.AccessControlListId))
				r.localModel.ACLModel.AccessControlListId = ""
			}
		}
	}
	return nil
}

func (r *ReconcileResource) updateRavenConfig(cm *corev1.ConfigMap) error {
	if cm.Data == nil {
		cm.Data = make(map[string]string, 0)
	}
	cm.Data[util.LoadBalancerId] = r.localModel.SLBModel.LoadBalancerId
	cm.Data[util.LoadBalancerIP] = r.localModel.SLBModel.Address
	cm.Data[util.ElasticIPId] = r.localModel.EIPModel.AllocationId
	cm.Data[util.ElasticIPIP] = r.localModel.EIPModel.Address
	cm.Data[util.ACLId] = r.localModel.ACLModel.AccessControlListId
	if r.localModel.SLBModel.LoadBalancerId == "" && r.localModel.EIPModel.AllocationId == "" && r.localModel.ACLModel.AccessControlListId == "" {
		cm.Data[util.RavenEnableTunnel] = "false"
		cm.Data[util.RavenEnableProxy] = "false"
	}
	err := r.Client.Update(context.TODO(), cm)
	if err != nil {
		return err
	}
	return err
}

func (r *ReconcileResource) needCleanupResource() (bool, error) {
	svcList := &corev1.ServiceList{}
	listOpt := &client.ListOptions{Namespace: util.WorkingNamespace}
	client.HasLabels{raven.LabelCurrentGateway, raven.LabelCurrentGatewayType}.ApplyToList(listOpt)
	err := r.Client.List(context.TODO(), svcList, listOpt)
	if err != nil {
		return false, err
	}
	newSvc := make([]corev1.Service, 0)
	for idx := range svcList.Items {
		if svcList.Items[idx].Spec.Type == corev1.ServiceTypeLoadBalancer {
			newSvc = append(newSvc, *svcList.Items[idx].DeepCopy())
		}
	}
	if len(newSvc) == 0 {
		return true, nil
	}
	return false, nil
}

func (r *ReconcileResource) cleanupCloudResource() error {
	if r.localModel.SLBModel.LoadBalancerId != "" {
		err := r.CleanupSLB(r.localModel.SLBModel)
		if err != nil {
			return fmt.Errorf("cleanup slb error %s", err.Error())
		}
	} else {
		err := r.DescribeSLBByName()
		if err != nil {
			return fmt.Errorf("describe slb by name error %s", err.Error())
		}
		if r.remoteModel.SLBModel.LoadBalancerId != "" {
			err = r.CleanupSLB(r.remoteModel.SLBModel)
			if err != nil {
				return fmt.Errorf("cleanup slb error %s", err.Error())
			}
		} else {
			klog.Infoln(Format("can not find slb id, skip cleanup it"))
		}
	}
	r.localModel.SLBModel.LoadBalancerId = ""
	r.localModel.SLBModel.Address = ""

	if r.localModel.ACLModel.AccessControlListId != "" {
		err := r.CleanupACL(r.localModel.ACLModel)
		if err != nil {
			return fmt.Errorf("cleanup acl error %s", err.Error())
		}
	} else {
		err := r.DescribeACLByName()
		if err != nil {
			return fmt.Errorf("describe acl by name error %s", err.Error())
		}
		if r.remoteModel.ACLModel.AccessControlListId != "" {
			err := r.CleanupACL(r.remoteModel.ACLModel)
			if err != nil {
				return fmt.Errorf("cleanup acl error %s", err.Error())
			}
		} else {
			klog.Infoln(Format("can not find acl id, skip cleanup it"))
		}
	}
	r.localModel.ACLModel.AccessControlListId = ""

	if r.localModel.EIPModel.AllocationId != "" {
		err := r.CleanupEIP(r.localModel.EIPModel)
		if err != nil {
			return fmt.Errorf("cleanup eip error %s", err.Error())
		}
	} else {
		err := r.DescribeEIPByName()
		if err != nil && !eip.IsNotFound(err) {
			return fmt.Errorf("describe eip by name error %s", err.Error())
		}
		if r.remoteModel.EIPModel.AllocationId != "" {
			err = r.CleanupEIP(r.remoteModel.EIPModel)
			if err != nil {
				return fmt.Errorf("cleanup eip error %s", err.Error())
			}
		} else {
			klog.Infoln(Format("can not find eip id, skip cleanup it"))
		}
	}
	r.localModel.EIPModel.AllocationId = ""
	r.localModel.EIPModel.Address = ""
	return nil
}

func (r *ReconcileResource) createCloudResource() error {
	if r.localModel.SLBModel.LoadBalancerId == "" {
		err := r.DescribeSLBByName()
		if err != nil {
			return fmt.Errorf("describe slb error %s", err.Error())
		}
		if r.remoteModel.SLBModel.LoadBalancerId == "" {
			err = r.CreateSLB()
			if err != nil {
				return fmt.Errorf("create slb error %s", err.Error())
			}
		} else {
			r.localModel.SLBModel.LoadBalancerId = r.remoteModel.SLBModel.LoadBalancerId
			r.localModel.SLBModel.Address = r.remoteModel.SLBModel.Address
		}
	}

	if r.localModel.ACLModel.AccessControlListId == "" {
		err := r.DescribeACLByName()
		if err != nil {
			return fmt.Errorf("describe acl error %s", err.Error())
		}
		if r.remoteModel.ACLModel.AccessControlListId == "" {
			err = r.CreateACL()
			if err != nil {
				return fmt.Errorf("create acl error %s", err.Error())
			}
		} else {
			r.localModel.ACLModel.AccessControlListId = r.remoteModel.ACLModel.AccessControlListId
		}
	}

	if r.localModel.EIPModel.AllocationId == "" {
		err := r.DescribeEIPByName()
		if err != nil && !eip.IsNotFound(err) {
			return fmt.Errorf("describe eip error %s", err.Error())
		}
		if r.remoteModel.EIPModel.AllocationId == "" {
			err = r.CreateEIP()
			if err != nil {
				return fmt.Errorf("create eip error %s", err.Error())
			}
		} else {
			r.localModel.EIPModel.AllocationId = r.remoteModel.EIPModel.AllocationId
			r.localModel.EIPModel.Address = r.remoteModel.EIPModel.Address
			r.localModel.EIPModel.InstanceId = r.remoteModel.EIPModel.InstanceId
		}
	}

	if r.localModel.EIPModel.InstanceId == "" && r.localModel.SLBModel.LoadBalancerId != "" {
		err := r.AssociateInstance()
		if err != nil {
			return fmt.Errorf("associate eip to slb error %s", err.Error())
		}
		r.localModel.EIPModel.InstanceId = r.localModel.SLBModel.LoadBalancerId
	}
	return nil
}

func (r *ReconcileResource) getRavenConfig() (*corev1.ConfigMap, error) {
	var cm corev1.ConfigMap
	objKey := types.NamespacedName{Namespace: util.WorkingNamespace, Name: util.RavenGlobalConfig}
	err := r.Client.Get(context.TODO(), objKey, &cm)
	if err != nil && !apierrors.IsNotFound(err) {
		return nil, err
	}
	if apierrors.IsNotFound(err) || cm.DeletionTimestamp != nil {
		return nil, nil
	}
	return cm.DeepCopy(), nil
}
