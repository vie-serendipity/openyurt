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
	"encoding/json"
	"fmt"
	"strings"
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
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/raven/util"
	prvd "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/util/cloudprovider"
	ravenprvd "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/util/cloudprovider/alibaba/raven"
	ravenmodel "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/util/cloudprovider/model/raven"
)

func Format(format string, args ...interface{}) string {
	s := fmt.Sprintf(format, args...)
	return fmt.Sprintf("%s: %s", names.RavenCloudProviderController, s)
}

// Add creates a new Service Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(ctx context.Context, c *appconfig.CompletedConfig, mgr manager.Manager) error {
	r, err := newReconciler(ctx, c, mgr)
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

	err = c.Watch(&source.Kind{Type: &corev1.ConfigMap{}}, &handler.EnqueueRequestForObject{}, predicate.NewPredicateFuncs(
		func(object client.Object) bool {
			cm, ok := object.(*corev1.ConfigMap)
			if !ok {
				return false
			}
			return isFocusedObject(cm)
		}))
	if err != nil {
		klog.Error(Format("watch resource corev1.Configmap error: %s", err.Error()))
		return err
	}

	return nil
}

func isFocusedObject(cm *corev1.ConfigMap) bool {
	if cm.Namespace != util.WorkingNamespace {
		return false
	}
	if cm.Name != util.RavenGlobalConfig {
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
	provider prvd.Provider
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(ctx context.Context, c *appconfig.CompletedConfig, mgr manager.Manager) (*ReconcileResource, error) {
	if c.Config.ComponentConfig.Generic.CloudProvider == nil {
		klog.Error("can not get alibaba provider")
		return nil, fmt.Errorf("can not get alibaba provider")
	}
	return &ReconcileResource{
		Client:   mgr.GetClient(),
		scheme:   mgr.GetScheme(),
		provider: c.Config.ComponentConfig.Generic.CloudProvider,
		queue:    workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "RavenServiceQueue"),
		recorder: mgr.GetEventRecorderFor(names.RavenCloudProviderController),
	}, nil
}

// Reconcile reads that state of the cluster for a Gateway object and makes changes based on the state read
// and what is in the Gateway.Spec
func (r *ReconcileResource) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	klog.V(2).Info(Format("started reconciling cloud resource for configmap %s ", req.NamespacedName.String()))
	cm, err := r.getRavenConfig(req.NamespacedName)
	if err != nil {
		klog.Error(Format("get configmap %s/%s, error %s", util.WorkingNamespace, util.RavenGlobalConfig, err.Error()))
		return reconcile.Result{Requeue: true, RequeueAfter: 2 * time.Second}, err
	}

	reqCtx := &util.RequestContext{Ctx: ctx, Cm: cm}
	if err = r.loadClusterAttribute(reqCtx); err != nil {
		klog.Error(Format("load cluster attribute error %s", err.Error()))
		return reconcile.Result{Requeue: true, RequeueAfter: 2 * time.Second}, err
	}

	localModel, err := r.buildLocalModel(reqCtx)
	if err != nil {
		klog.Error(Format("build local model error %s", err.Error()))
		return reconcile.Result{Requeue: true, RequeueAfter: 2 * time.Second}, err
	}

	remoteModel, err := r.buildRemoteModel(reqCtx)
	if err != nil {
		klog.Error(Format("build remote model error %s", err.Error()))
		return reconcile.Result{Requeue: true, RequeueAfter: 2 * time.Second}, err
	}

	if needCleanupResource(reqCtx) {
		err = r.cleanupCloudResource(reqCtx, localModel, remoteModel)
		if err != nil {
			klog.Error(Format("cleanup cloud resource error %s", err.Error()))
			return reconcile.Result{Requeue: true, RequeueAfter: 2 * time.Second}, err
		}
	} else {
		err = r.createCloudResource(reqCtx, localModel, remoteModel)
		if err != nil {
			klog.Error(Format("create cloud resource error %s", err.Error()))
			return reconcile.Result{Requeue: true, RequeueAfter: 2 * time.Second}, err
		}
	}

	if cm != nil {
		err = r.updateRavenConfig(reqCtx, remoteModel)
		if err != nil {
			klog.Error(Format("update configmap %s/%s error %s", cm.GetNamespace(), cm.GetName(), err.Error()))
			return reconcile.Result{Requeue: true, RequeueAfter: 2 * time.Second}, err
		}
	}
	return reconcile.Result{}, nil
}

func (r *ReconcileResource) getRavenConfig(key types.NamespacedName) (*corev1.ConfigMap, error) {
	var cm corev1.ConfigMap
	err := r.Client.Get(context.TODO(), key, &cm)
	if err != nil && !apierrors.IsNotFound(err) {
		return nil, err
	}
	if apierrors.IsNotFound(err) || !cm.DeletionTimestamp.IsZero() {
		return nil, nil
	}
	return cm.DeepCopy(), nil
}

func (r *ReconcileResource) loadClusterAttribute(reqCtx *util.RequestContext) error {
	cluster, err := r.provider.GetClusterID()
	if err != nil {
		return err
	}
	regionId, err := r.provider.GetRegion()
	if err != nil {
		return err
	}
	vpcId, err := r.provider.GetVpcID()
	if err != nil {
		return err
	}
	vswitch, err := r.provider.GetVswitchID()
	if err != nil {
		return err
	}
	reqCtx.ClusterId = cluster
	reqCtx.Region = regionId
	reqCtx.VpcId = vpcId
	reqCtx.VswitchId = vswitch
	return nil
}

func (r *ReconcileResource) buildLocalModel(reqCtx *util.RequestContext) (*util.Model, error) {
	nameKey := ravenmodel.NewNamedKey(reqCtx.ClusterId)
	localModel := util.NewModel(nameKey, reqCtx.Region)
	localModel.SLBModel.VpcId = reqCtx.VpcId
	localModel.SLBModel.VSwitchId = reqCtx.VswitchId
	localModel.SLBModel.Spec = "slb.s2.medium"
	localModel.SLBModel.AddressType = "intranet"
	localModel.EIPModel.Bandwidth = "100"
	localModel.EIPModel.InstanceChargeType = "PostPaid"
	localModel.EIPModel.InternetChargeType = "PayByTraffic"
	localModel.SLBModel.Name = nameKey.String()
	localModel.ACLModel.Name = nameKey.String()
	localModel.EIPModel.Name = nameKey.String()

	localModel.SLBModel.LoadBalancerId = reqCtx.GetLoadBalancerId()
	localModel.SLBModel.Address = reqCtx.GetLoadBalancerAddress()
	localModel.EIPModel.AllocationId = reqCtx.GetEIPId()
	localModel.EIPModel.Address = reqCtx.GetEIPAddress()
	localModel.ACLModel.AccessControlListId = reqCtx.GetACLId()

	if localModel.SLBModel.LoadBalancerId != "" {
		err := r.provider.DescribeLoadBalancer(reqCtx.Ctx, localModel.SLBModel)
		if err != nil {
			if !ravenprvd.IsNotFound(err) {
				return localModel, fmt.Errorf("describe slb %s, error %s", localModel.SLBModel.LoadBalancerId, err.Error())
			} else {
				klog.Warningf(Format("slb id %s is not found, recreate a slb", localModel.SLBModel.LoadBalancerId))
				localModel.SLBModel.LoadBalancerId = ""
			}
		}
	}

	if localModel.EIPModel.AllocationId != "" {
		err := r.provider.DescribeEipAddresses(reqCtx.Ctx, localModel.EIPModel)
		if err != nil {
			if !ravenprvd.IsNotFound(err) {
				return localModel, fmt.Errorf("describe eip %s, error %s", localModel.EIPModel.AllocationId, err.Error())
			} else {
				klog.Warningf(Format("eip id %s is not found recreate a eip", localModel.EIPModel.AllocationId))
				localModel.EIPModel.AllocationId = ""
			}
		}
	}

	if localModel.ACLModel.AccessControlListId != "" {
		err := r.provider.DescribeAccessControlListAttribute(reqCtx.Ctx, localModel.ACLModel)
		if err != nil {
			if !ravenprvd.IsNotFound(err) {
				return localModel, fmt.Errorf("describe acl %s, error %s", localModel.ACLModel.AccessControlListId, err.Error())
			} else {
				klog.Warningf(Format("acl id %s is not found recreate a slb", localModel.ACLModel.AccessControlListId))
				localModel.ACLModel.AccessControlListId = ""
			}
		}
	}

	lMdlJson, err := json.Marshal(localModel)
	if err == nil {
		klog.V(2).Infoln(Format("build local model %s", lMdlJson))
	}

	return localModel, nil
}

func (r *ReconcileResource) buildRemoteModel(reqCtx *util.RequestContext) (*util.Model, error) {
	nameKey := ravenmodel.NewNamedKey(reqCtx.ClusterId)
	remoteModel := util.NewModel(nameKey, reqCtx.Region)
	remoteModel.SLBModel.VpcId = reqCtx.VpcId
	remoteModel.SLBModel.VSwitchId = reqCtx.VswitchId
	remoteModel.SLBModel.Name = nameKey.String()
	remoteModel.ACLModel.Name = nameKey.String()
	remoteModel.EIPModel.Name = nameKey.String()
	if err := r.provider.DescribeLoadBalancers(reqCtx.Ctx, remoteModel.SLBModel); err != nil && !ravenprvd.IsNotFound(err) {
		return remoteModel, err
	}
	if err := r.provider.DescribeAccessControlLists(reqCtx.Ctx, remoteModel.ACLModel); err != nil && !ravenprvd.IsNotFound(err) {
		return remoteModel, err
	}
	if err := r.provider.DescribeEipAddresses(reqCtx.Ctx, remoteModel.EIPModel); err != nil && !ravenprvd.IsNotFound(err) {
		return remoteModel, err
	}

	rMdlJson, err := json.Marshal(remoteModel)
	if err == nil {
		klog.V(2).Infoln(Format("build remote model %s", rMdlJson))
	}
	return remoteModel, nil
}

func needCleanupResource(reqCtx *util.RequestContext) bool {
	if reqCtx.Cm == nil {
		return true
	}
	if reqCtx.Cm.Data == nil {
		return true
	}
	if strings.ToLower(reqCtx.Cm.Data[util.RavenEnableProxy]) == "false" &&
		strings.ToLower(reqCtx.Cm.Data[util.RavenEnableTunnel]) == "false" {
		return true
	}
	return false
}

func (r *ReconcileResource) cleanupCloudResource(reqCtx *util.RequestContext, lModel, rModel *util.Model) error {
	if lModel.SLBModel.LoadBalancerId != "" {
		err := r.CleanupSLB(reqCtx.Ctx, lModel.SLBModel)
		if err != nil {
			return fmt.Errorf("cleanup slb error %s", err.Error())
		}
		rModel.SLBModel.LoadBalancerId = ""
		rModel.SLBModel.Address = ""
	} else {
		if rModel.SLBModel.LoadBalancerId != "" {
			err := r.CleanupSLB(reqCtx.Ctx, rModel.SLBModel)
			if err != nil {
				return fmt.Errorf("cleanup slb error %s", err.Error())
			}
		} else {
			klog.Infoln(Format("can not find slb id, skip cleanup it"))
		}
	}

	if lModel.ACLModel.AccessControlListId != "" {
		err := r.CleanupACL(reqCtx.Ctx, lModel.ACLModel)
		if err != nil {
			return fmt.Errorf("cleanup acl error %s", err.Error())
		}
		rModel.ACLModel.AccessControlListId = ""
	} else {
		if rModel.ACLModel.AccessControlListId != "" {
			err := r.CleanupACL(reqCtx.Ctx, rModel.ACLModel)
			if err != nil {
				return fmt.Errorf("cleanup acl error %s", err.Error())
			}
		} else {
			klog.Infoln(Format("can not find acl id, skip cleanup it"))
		}
	}

	if lModel.EIPModel.AllocationId != "" {
		err := r.CleanupEIP(reqCtx.Ctx, lModel.EIPModel)
		if err != nil {
			return fmt.Errorf("cleanup eip error %s", err.Error())
		}
		rModel.EIPModel.AllocationId = ""
		rModel.EIPModel.Address = ""
	} else {
		if rModel.EIPModel.AllocationId != "" {
			err := r.CleanupEIP(reqCtx.Ctx, rModel.EIPModel)
			if err != nil {
				return fmt.Errorf("cleanup eip error %s", err.Error())
			}
		} else {
			klog.Infoln(Format("can not find eip id, skip cleanup it"))
		}
	}
	return nil
}

func (r *ReconcileResource) createCloudResource(reqCtx *util.RequestContext, lModel, rModel *util.Model) error {
	if rModel.SLBModel.LoadBalancerId == "" {
		if lModel.SLBModel.LoadBalancerId == "" {
			err := r.provider.CreateLoadBalancer(reqCtx.Ctx, lModel.SLBModel)
			if err != nil {
				return fmt.Errorf("create slb error %s", err.Error())
			}
			klog.Infoln(Format("successfully create slb: %s", lModel.SLBModel.LoadBalancerId))
		}
		rModel.SLBModel.LoadBalancerId = lModel.SLBModel.LoadBalancerId
		rModel.SLBModel.Address = lModel.SLBModel.Address
	}

	if rModel.ACLModel.AccessControlListId == "" {
		if lModel.ACLModel.AccessControlListId == "" {
			err := r.provider.CreateAccessControlList(reqCtx.Ctx, lModel.ACLModel)
			if err != nil {
				return fmt.Errorf("create acl error %s", err.Error())
			}
			klog.Infoln(Format("successfully create slb: %s", lModel.ACLModel.AccessControlListId))
		}
		rModel.ACLModel.AccessControlListId = lModel.ACLModel.AccessControlListId
	}

	if rModel.EIPModel.AllocationId == "" {
		if lModel.EIPModel.AllocationId == "" {
			err := r.provider.AllocateEipAddress(reqCtx.Ctx, lModel.EIPModel)
			if err != nil {
				return fmt.Errorf("create eip error %s", err.Error())
			}
			klog.Infoln(Format("successfully create eip: %s", lModel.EIPModel.InstanceId))
		}
		rModel.EIPModel.AllocationId = lModel.EIPModel.AllocationId
		rModel.EIPModel.Address = lModel.EIPModel.Address
		rModel.EIPModel.InstanceId = lModel.EIPModel.InstanceId
	}

	if rModel.EIPModel.InstanceId == "" && rModel.SLBModel.LoadBalancerId != "" {
		err := r.AssociateInstance(reqCtx.Ctx, rModel.EIPModel, rModel.SLBModel.LoadBalancerId)
		if err != nil {
			return fmt.Errorf("associate eip to slb error %s", err.Error())
		}
		klog.Infoln(Format("successfully associate eip: %s into slb %s", rModel.EIPModel.InstanceId, rModel.SLBModel.LoadBalancerId))
		rModel.EIPModel.InstanceId = rModel.SLBModel.LoadBalancerId
	}
	return nil
}

func (r *ReconcileResource) updateRavenConfig(reqCtx *util.RequestContext, model *util.Model) error {
	if reqCtx.Cm.Data == nil {
		reqCtx.Cm.Data = make(map[string]string, 0)
	}
	reqCtx.Cm.Data[util.LoadBalancerId] = model.SLBModel.LoadBalancerId
	reqCtx.Cm.Data[util.LoadBalancerIP] = model.SLBModel.Address
	reqCtx.Cm.Data[util.ElasticIPId] = model.EIPModel.AllocationId
	reqCtx.Cm.Data[util.ElasticIPIP] = model.EIPModel.Address
	reqCtx.Cm.Data[util.ACLId] = model.ACLModel.AccessControlListId

	err := r.Client.Update(context.TODO(), reqCtx.Cm)
	if err != nil {
		return err
	}
	return err
}
