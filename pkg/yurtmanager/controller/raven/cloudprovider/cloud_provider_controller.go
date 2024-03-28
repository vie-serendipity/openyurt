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
	"reflect"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/finalizer"
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

const (
	Finalizer    = "raven.openyurt.io/resource"
	ResourceHash = "raven.openyurt.io/resource"

	FailedAddFinalizer        = "FailedAddFinalizer"
	FailedRemoveFinalizer     = "FailedRemoveFinalizer"
	FailedAddHash             = "FailedAddHash"
	FailedRemoveHash          = "FailedRemoveHash"
	FailedUpdateData          = "FailedUpdateData"
	SucceedCleanCloudResource = "CleanCloudResource"
	FailedClearCloudResource  = "CleanCloudResourceFailed"
	FailedSyncCloudResource   = "EnsureCloudResourceFailed"
	SucceedSyncCloudResource  = "EnsuredCloudResource"
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

	err = c.Watch(&source.Kind{Type: &corev1.ConfigMap{}}, &handler.EnqueueRequestForObject{},
		predicate.Funcs{
			CreateFunc: func(evt event.CreateEvent) bool {
				cm, ok := evt.Object.(*corev1.ConfigMap)
				if !ok {
					return false
				}
				return isRavenConfig(cm)
			},
			UpdateFunc: func(evt event.UpdateEvent) bool {
				newCm, ok1 := evt.ObjectNew.(*corev1.ConfigMap)
				oldCm, ok2 := evt.ObjectOld.(*corev1.ConfigMap)

				if ok1 && ok2 && isRavenConfig(newCm) && isRavenConfig(oldCm) {
					if !reflect.DeepEqual(newCm.DeletionTimestamp.IsZero(), oldCm.DeletionTimestamp.IsZero()) {
						return true
					}
					if (newCm.Data != nil && oldCm.Data == nil) || (newCm.Data == nil && oldCm.Data != nil) {
						return true
					}
					if newCm.Data != nil && oldCm.Data != nil {
						return strings.Join([]string{newCm.Data[util.RavenEnableProxy], newCm.Data[util.RavenEnableTunnel]}, ",") !=
							strings.Join([]string{oldCm.Data[util.RavenEnableProxy], oldCm.Data[util.RavenEnableTunnel]}, ",")
					}
				}
				return false
			},
			DeleteFunc:  func(deleteEvent event.DeleteEvent) bool { return false },
			GenericFunc: func(genericEvent event.GenericEvent) bool { return false },
		})
	if err != nil {
		klog.Error(Format("watch resource corev1.Configmap error: %s", err.Error()))
		return err
	}
	return nil
}

var _ reconcile.Reconciler = &ReconcileResource{}

type ReconcileResource struct {
	client.Client
	scheme            *runtime.Scheme
	recorder          record.EventRecorder
	finalizer         finalizer.Finalizers
	queue             workqueue.RateLimitingInterface
	provider          prvd.Provider
	providerAttribute Attribute
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(ctx context.Context, c *appconfig.CompletedConfig, mgr manager.Manager) (*ReconcileResource, error) {
	if c.Config.ComponentConfig.Generic.CloudProvider == nil {
		klog.Error("can not get alibaba provider")
		return nil, fmt.Errorf("can not get alibaba provider")
	}
	r := &ReconcileResource{
		Client:    mgr.GetClient(),
		scheme:    mgr.GetScheme(),
		finalizer: finalizer.NewFinalizers(),
		provider:  c.Config.ComponentConfig.Generic.CloudProvider,
		queue:     workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "RavenServiceQueue"),
		recorder:  mgr.GetEventRecorderFor(names.RavenCloudProviderController),
	}
	err := r.finalizer.Register(Finalizer, NewRavenFinalizer(r.Client))
	if err != nil {
		klog.Info("new finalizer error %s, can ignore it", err.Error())
	}
	r.providerAttribute, err = newProviderAttribute(r.provider)
	if err != nil {
		return r, fmt.Errorf("new provider attribute error %s", err.Error())
	}
	return r, nil
}

type Attribute struct {
	ClusterId string
	Region    string
	VpcId     string
	VswitchId string
}

func newProviderAttribute(provider prvd.Provider) (Attribute, error) {
	var attribute Attribute
	cluster, err := provider.GetClusterID()
	if err != nil {
		return attribute, err
	}
	regionId, err := provider.GetRegion()
	if err != nil {
		return attribute, err
	}
	vpcId, err := provider.GetVpcID()
	if err != nil {
		return attribute, err
	}
	vswitch, err := provider.GetVswitchID()
	if err != nil {
		return attribute, err
	}

	attribute.ClusterId = cluster
	attribute.Region = regionId
	attribute.VpcId = vpcId
	attribute.VswitchId = vswitch
	return attribute, nil
}

// Reconcile reads that state of the cluster for a Gateway object and makes changes based on the state read
// and what is in the Gateway.Spec
func (r *ReconcileResource) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	klog.V(2).Info(Format("started reconciling cloud resource for configmap %s ", req.NamespacedName.String()))
	var cm corev1.ConfigMap
	err := r.Client.Get(context.TODO(), req.NamespacedName, &cm)
	if err != nil {
		if apierrors.IsNotFound(err) {
			klog.Infoln(Format("configmap %s not found, skip it", req.NamespacedName.String()))
			return reconcile.Result{}, nil
		}
		klog.Error(Format("[get configmap %s/%s, error]: %s", util.WorkingNamespace, util.RavenGlobalConfig, err.Error()))
		return reconcile.Result{Requeue: true, RequeueAfter: 10 * time.Second}, err
	}
	reqCtx := &util.RequestContext{Ctx: ctx, Cm: &cm}
	if needCleanupResource(reqCtx) {
		if HasFinalizer(reqCtx.Cm, Finalizer) {
			model, err := r.cleanup(reqCtx)
			if err != nil {
				r.recorder.Event(reqCtx.Cm, corev1.EventTypeWarning, FailedClearCloudResource,
					fmt.Sprintf("filed cleanup cloud resource: %s", err.Error()))
				klog.Error(Format("[cleanup cloud resource error]: %s", err.Error()))
				return reconcile.Result{Requeue: true, RequeueAfter: 10 * time.Second}, err
			}
			err = r.removeLabels(reqCtx.Cm)
			if err != nil {
				klog.Error(Format("[remove resource hash from labels]: %s", err.Error()))
				return reconcile.Result{Requeue: true, RequeueAfter: 10 * time.Second}, err
			}
			err = r.updateData(reqCtx.Cm, model)
			if err != nil {
				klog.Error(Format("[update data]: %s", err.Error()))
				return reconcile.Result{Requeue: true, RequeueAfter: 10 * time.Second}, err
			}
			err = Finalize(reqCtx.Ctx, reqCtx.Cm, r.Client, r.finalizer)
			if err != nil {
				klog.Error(Format("[remove finalizer]: %s", err.Error()))
				return reconcile.Result{Requeue: true, RequeueAfter: 10 * time.Second}, err
			}
			r.recorder.Event(reqCtx.Cm, corev1.EventTypeNormal, SucceedCleanCloudResource,
				fmt.Sprintf("success clearnup cloud resource"))
			klog.Infoln(Format("%s, successfully cleanup cloud resource", req.NamespacedName.String()))
		}
	} else {
		err = Finalize(reqCtx.Ctx, reqCtx.Cm, r.Client, r.finalizer)
		if err != nil {
			klog.Error(Format("[add finalizer]: %s", err.Error()))
			return reconcile.Result{Requeue: true, RequeueAfter: 10 * time.Second}, err
		}
		model, err := r.ensure(reqCtx)
		if err != nil {
			r.recorder.Event(reqCtx.Cm, corev1.EventTypeWarning, FailedSyncCloudResource,
				fmt.Sprintf("ensure cloud resource: %s", err.Error()))
			klog.Error(Format("[ensure cloud resource error]: %s", err.Error()))
			return reconcile.Result{Requeue: true, RequeueAfter: 10 * time.Second}, err
		}
		err = r.addLabels(reqCtx.Cm, model)
		if err != nil {
			klog.Error(Format("[add resource hash into labels]: %s", err.Error()))
			return reconcile.Result{Requeue: true, RequeueAfter: 10 * time.Second}, err
		}
		err = r.updateData(reqCtx.Cm, model)
		if err != nil {
			klog.Error(Format("[update data]: %s", err.Error()))
			return reconcile.Result{Requeue: true, RequeueAfter: 10 * time.Second}, err
		}
		r.recorder.Event(reqCtx.Cm, corev1.EventTypeNormal, SucceedSyncCloudResource,
			fmt.Sprintf("success ensure cloud resource, lb id [%s], eip id [%s], acl id [%s]",
				model.SLBModel.LoadBalancerId, model.EIPModel.AllocationId, model.ACLModel.AccessControlListId))
		klog.Infoln(Format("%s, successfully create cloud resource", req.NamespacedName.String()))
	}
	return reconcile.Result{}, nil
}

func (r *ReconcileResource) ensure(reqCtx *util.RequestContext) (*util.Model, error) {
	localModel, err := r.buildLocalModel(reqCtx)
	if err != nil {
		return nil, fmt.Errorf("build local model: %s", err.Error())
	}
	remoteModel, err := r.buildRemoteModel(reqCtx, localModel)
	if err != nil {
		return nil, fmt.Errorf("build remote model: %s", err.Error())
	}
	err = r.createCloudResource(reqCtx, localModel, remoteModel)
	if err != nil {
		return nil, fmt.Errorf("create cloud resource: %s", err.Error())
	}
	return remoteModel, nil
}

func (r *ReconcileResource) cleanup(reqCtx *util.RequestContext) (*util.Model, error) {
	localModel, err := r.buildLocalModel(reqCtx)
	if err != nil {
		return nil, fmt.Errorf("build local model: %s", err.Error())
	}

	nameKey := ravenmodel.NewNamedKey(r.providerAttribute.ClusterId)
	remoteModel := util.NewModel(nameKey, r.providerAttribute.Region)
	remoteModel.SLBModel.VpcId = r.providerAttribute.VpcId
	remoteModel.SLBModel.VSwitchId = r.providerAttribute.VswitchId
	remoteModel.SLBModel.Name = nameKey.String()
	remoteModel.ACLModel.Name = nameKey.String()
	remoteModel.EIPModel.Name = nameKey.String()
	if localModel.SLBModel.LoadBalancerId != "" {
		remoteModel.SLBModel.LoadBalancerId = localModel.SLBModel.LoadBalancerId
	}
	if localModel.ACLModel.AccessControlListId != "" {
		remoteModel.ACLModel.AccessControlListId = localModel.ACLModel.AccessControlListId
	}
	if localModel.EIPModel.AllocationId != "" {
		remoteModel.EIPModel.AllocationId = localModel.EIPModel.AllocationId
	}

	err = r.cleanupCloudResource(reqCtx, remoteModel)
	if err != nil {
		return nil, fmt.Errorf("delete cloud resource: %s", err.Error())
	}
	return remoteModel, nil
}

func (r *ReconcileResource) buildLocalModel(reqCtx *util.RequestContext) (*util.Model, error) {
	nameKey := ravenmodel.NewNamedKey(r.providerAttribute.ClusterId)
	localModel := util.NewModel(nameKey, r.providerAttribute.Region)
	localModel.SLBModel.VpcId = r.providerAttribute.VpcId
	localModel.SLBModel.VSwitchId = r.providerAttribute.VswitchId
	localModel.SLBModel.InstanceChargeType = "PayByCLCU"
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

	lMdlJson, err := json.Marshal(localModel)
	if err == nil {
		klog.V(2).Infoln(Format("build local model %s", lMdlJson))
	}

	return localModel, nil
}

func (r *ReconcileResource) buildRemoteModel(reqCtx *util.RequestContext, localMode *util.Model) (*util.Model, error) {
	nameKey := ravenmodel.NewNamedKey(r.providerAttribute.ClusterId)
	remoteModel := util.NewModel(nameKey, r.providerAttribute.Region)
	remoteModel.SLBModel.VpcId = r.providerAttribute.VpcId
	remoteModel.SLBModel.VSwitchId = r.providerAttribute.VswitchId
	remoteModel.SLBModel.Name = nameKey.String()
	remoteModel.ACLModel.Name = nameKey.String()
	remoteModel.EIPModel.Name = nameKey.String()

	if localMode.SLBModel.LoadBalancerId != "" {
		remoteModel.SLBModel.LoadBalancerId = localMode.SLBModel.LoadBalancerId
	}
	if localMode.ACLModel.AccessControlListId != "" {
		remoteModel.ACLModel.AccessControlListId = localMode.ACLModel.AccessControlListId
	}
	if localMode.EIPModel.AllocationId != "" {
		remoteModel.EIPModel.AllocationId = localMode.EIPModel.AllocationId
	}

	if remoteModel.SLBModel.LoadBalancerId != "" {
		err := r.provider.DescribeLoadBalancer(reqCtx.Ctx, remoteModel.SLBModel)
		if err != nil {
			return remoteModel, fmt.Errorf("describe slb %s, error %s", remoteModel.SLBModel.LoadBalancerId, err.Error())
		}
	} else {
		err := r.provider.DescribeLoadBalancers(reqCtx.Ctx, remoteModel.SLBModel)
		if err != nil {
			if ravenprvd.IsNotFound(err) {
				klog.Infoln(Format("can not find slb named %s, [api]: %s", remoteModel.SLBModel.Name, err.Error()))
			} else {
				return remoteModel, fmt.Errorf("can not find slb named %s, error %s", remoteModel.SLBModel.Name, err.Error())
			}
		}
	}
	if remoteModel.ACLModel.AccessControlListId != "" {
		err := r.provider.DescribeAccessControlListAttribute(reqCtx.Ctx, remoteModel.ACLModel)
		if err != nil {
			return remoteModel, fmt.Errorf("can not find acl id %s, error %s", remoteModel.ACLModel.AccessControlListId, err.Error())
		}
	} else {
		err := r.provider.DescribeAccessControlLists(reqCtx.Ctx, remoteModel.ACLModel)
		if err != nil {
			if ravenprvd.IsNotFound(err) {
				klog.Infoln(Format("can not find acl named %s, [api]: %s", remoteModel.ACLModel.Name, err.Error()))
			} else {
				return remoteModel, fmt.Errorf("can not find acl named %s, error %s", remoteModel.ACLModel.Name, err.Error())
			}
		}
	}

	err := r.provider.DescribeEipAddresses(reqCtx.Ctx, remoteModel.EIPModel)
	if err != nil {
		if ravenprvd.IsNotFound(err) {
			klog.Infoln(Format("can not find eip named %s, [api]: %s", remoteModel.EIPModel.Name, err.Error()))
		} else {
			return remoteModel, fmt.Errorf("can not find eip named %s, error %s", remoteModel.EIPModel.Name, err.Error())
		}
	}

	rMdlJson, err := json.Marshal(remoteModel)
	if err == nil {
		klog.V(2).Infoln(Format("build remote model %s", rMdlJson))
	}
	return remoteModel, nil
}

func (r *ReconcileResource) findSLB(reqCtx *util.RequestContext, model *ravenmodel.LoadBalancerAttribute) error {
	if model.LoadBalancerId != "" {
		err := r.provider.DescribeLoadBalancer(reqCtx.Ctx, model)
		if err != nil {
			return fmt.Errorf("can not find slb id %s, error %s", model.LoadBalancerId, err.Error())
		}
	} else {
		err := r.provider.DescribeLoadBalancers(reqCtx.Ctx, model)
		if err != nil {
			return fmt.Errorf("can not find slb named %s, error %s", model.Name, err.Error())
		}
	}
	return nil
}

func (r *ReconcileResource) findACL(reqCtx *util.RequestContext, model *ravenmodel.AccessControlListAttribute) error {
	if model.AccessControlListId != "" {
		err := r.provider.DescribeAccessControlListAttribute(reqCtx.Ctx, model)
		if err != nil {
			return fmt.Errorf("can not find acl id %s, error %s", model.AccessControlListId, err.Error())
		}
	} else {
		err := r.provider.DescribeAccessControlLists(reqCtx.Ctx, model)
		if err != nil {
			return fmt.Errorf("can not find acl named %s, error %s", model.Name, err.Error())
		}
	}
	return nil
}

func (r *ReconcileResource) findEIP(reqCtx *util.RequestContext, model *ravenmodel.ElasticIPAttribute) error {
	err := r.provider.DescribeEipAddresses(reqCtx.Ctx, model)
	if err != nil {
		return fmt.Errorf("can not find eip id %s, named %s, error %s", model.AllocationId, model.Name, err.Error())
	}
	return nil
}

func needCleanupResource(reqCtx *util.RequestContext) bool {
	if reqCtx.Cm.DeletionTimestamp != nil {
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

func (r *ReconcileResource) cleanupCloudResource(reqCtx *util.RequestContext, rModel *util.Model) error {

	err := r.findSLB(reqCtx, rModel.SLBModel)
	if err != nil {
		if !ravenprvd.IsNotFound(err) {
			return fmt.Errorf("find slb error %s", err.Error())
		} else {
			rModel.SLBModel.LoadBalancerId = ""
			klog.Infoln("can not find slb, skip cleanup it")
		}
	}
	if rModel.SLBModel.LoadBalancerId != "" {
		err := r.CleanupSLB(reqCtx.Ctx, rModel.SLBModel)
		if err != nil {
			return fmt.Errorf("cleanup slb error %s", err.Error())
		}
	}

	err = r.findEIP(reqCtx, rModel.EIPModel)
	if err != nil {
		if !ravenprvd.IsNotFound(err) {
			return fmt.Errorf("find eip error %s", err.Error())
		} else {
			rModel.EIPModel.AllocationId = ""
			klog.Infoln("can not find eip, skip cleanup it")
		}
	}
	if rModel.EIPModel.AllocationId != "" {
		err := r.CleanupEIP(reqCtx.Ctx, rModel.EIPModel)
		if err != nil {
			return fmt.Errorf("cleanup eip error %s", err.Error())
		}
	}

	err = r.findACL(reqCtx, rModel.ACLModel)
	if err != nil {
		if !ravenprvd.IsNotFound(err) {
			return fmt.Errorf("find acl error %s", err.Error())
		} else {
			rModel.ACLModel.AccessControlListId = ""
			klog.Infoln("can not find acl, skip cleanup it")
		}
	}
	if rModel.ACLModel.AccessControlListId != "" {
		err := r.CleanupACL(reqCtx.Ctx, rModel.ACLModel)
		if err != nil {
			return fmt.Errorf("cleanup acl error %s", err.Error())
		}
	}
	return nil
}

func (r *ReconcileResource) createCloudResource(reqCtx *util.RequestContext, lModel, rModel *util.Model) error {
	if rModel.SLBModel.LoadBalancerId == "" {
		err := r.provider.CreateLoadBalancer(reqCtx.Ctx, lModel.SLBModel)
		if err != nil {
			return fmt.Errorf("create slb error %s", err.Error())
		}
		klog.Infoln(Format("successfully create slb: %s", lModel.SLBModel.LoadBalancerId))
		rModel.SLBModel.LoadBalancerId = lModel.SLBModel.LoadBalancerId
		rModel.SLBModel.Address = lModel.SLBModel.Address
	}

	if rModel.ACLModel.AccessControlListId == "" {
		err := r.provider.CreateAccessControlList(reqCtx.Ctx, lModel.ACLModel)
		if err != nil {
			return fmt.Errorf("create acl error %s", err.Error())
		}
		klog.Infoln(Format("successfully create acl: %s", lModel.ACLModel.AccessControlListId))
		rModel.ACLModel.AccessControlListId = lModel.ACLModel.AccessControlListId
	}

	if rModel.EIPModel.AllocationId == "" {
		err := r.provider.AllocateEipAddress(reqCtx.Ctx, lModel.EIPModel)
		if err != nil {
			return fmt.Errorf("create eip error %s", err.Error())
		}
		klog.Infoln(Format("successfully create eip: %s", lModel.EIPModel.AllocationId))
		rModel.EIPModel.AllocationId = lModel.EIPModel.AllocationId
		rModel.EIPModel.Address = lModel.EIPModel.Address
		rModel.EIPModel.InstanceId = lModel.EIPModel.InstanceId
	}

	if rModel.EIPModel.InstanceId == "" && rModel.SLBModel.LoadBalancerId != "" {
		err := r.AssociateInstance(reqCtx.Ctx, rModel.EIPModel, rModel.SLBModel.LoadBalancerId)
		if err != nil {
			return fmt.Errorf("associate eip to slb error %s", err.Error())
		}
		klog.Infoln(Format("successfully associate eip: %s into slb %s", rModel.EIPModel.AllocationId, rModel.SLBModel.LoadBalancerId))
		rModel.EIPModel.InstanceId = rModel.SLBModel.LoadBalancerId
	}
	return nil
}

func (r *ReconcileResource) addLabels(cm *corev1.ConfigMap, model *util.Model) error {
	updated := cm.DeepCopy()
	if updated.Labels == nil {
		updated.Labels = make(map[string]string)
	}
	curr := util.GetResourceHash(model)
	prev := updated.Labels[ResourceHash]
	if curr != prev {
		updated.Labels[ResourceHash] = curr
		if err := r.Client.Patch(context.TODO(), updated, client.MergeFrom(cm)); err != nil {
			return fmt.Errorf("%s/%s failed to add resource hash:, error: %s", cm.Namespace, cm.Name, err.Error())
		}
	}
	return nil
}

func (r *ReconcileResource) removeLabels(cm *corev1.ConfigMap) error {
	updated := cm.DeepCopy()
	if updated.Labels == nil {
		return nil
	}
	needUpdate := false
	if _, ok := updated.Labels[ResourceHash]; ok {
		delete(updated.Labels, ResourceHash)
		needUpdate = true
	}
	if needUpdate {
		if err := r.Client.Patch(context.TODO(), updated, client.MergeFrom(cm)); err != nil {
			return fmt.Errorf("%s/%s failed to remove resource hash, error: %s", cm.Namespace, cm.Name, err.Error())
		}
	}
	return nil
}

func (r *ReconcileResource) updateData(cm *corev1.ConfigMap, model *util.Model) error {
	currHash := util.GetResourceHash(model)
	prevHash := util.GetPrevResourceHash(cm.Data)
	if currHash != prevHash {
		var retErr error
		_ = util.Retry(
			&wait.Backoff{
				Duration: 1 * time.Second,
				Steps:    3,
				Factor:   2,
				Jitter:   4,
			},
			func(cm *corev1.ConfigMap) error {
				cmOld := &corev1.ConfigMap{}
				key := client.ObjectKey{Namespace: cm.Namespace, Name: cm.Name}
				retErr = r.Client.Get(context.TODO(), key, cmOld)
				if retErr != nil {
					return fmt.Errorf("error to get cm %s", key.String())
				}
				update := cmOld.DeepCopy()
				if update.Data == nil {
					update.Data = make(map[string]string)
				}
				update.Data[util.LoadBalancerId] = model.SLBModel.LoadBalancerId
				update.Data[util.LoadBalancerIP] = model.SLBModel.Address
				update.Data[util.ElasticIPId] = model.EIPModel.AllocationId
				update.Data[util.ElasticIPIP] = model.EIPModel.Address
				update.Data[util.ACLId] = model.ACLModel.AccessControlListId
				retErr = r.Client.Update(context.TODO(), update)
				if retErr == nil {
					return nil
				}
				if apierrors.IsNotFound(retErr) {
					klog.Errorln(Format("configmap %s, not persisting update to configmap that no longer exists, error %s", retErr))
					retErr = nil
					return nil
				}
				if apierrors.IsConflict(retErr) {
					return fmt.Errorf("not persisting update to configmap %s that "+
						"has been changed since we received it: %v", key.String(), retErr)
				}
				klog.Errorln(Format("configmap %s, failed to persist updated configmap, error %s", retErr))
				return fmt.Errorf("retry with %s, %s", retErr.Error(), "try again")

			},
			cm,
		)
		return retErr
	}
	return nil
}

func isRavenConfig(cm *corev1.ConfigMap) bool {
	return cm.Namespace == util.WorkingNamespace && cm.Name == util.RavenGlobalConfig
}
