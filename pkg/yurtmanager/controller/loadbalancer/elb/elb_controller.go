package elb

import (
	"context"
	"encoding/json"
	"fmt"
	"golang.org/x/time/rate"
	"reflect"
	"strings"
	"time"

	v1 "k8s.io/api/core/v1"
	discovery "k8s.io/api/discovery/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
	v1helper "k8s.io/kubernetes/pkg/apis/core/v1/helper"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/finalizer"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	appconfig "github.com/openyurtio/openyurt/cmd/yurt-manager/app/config"
	"github.com/openyurtio/openyurt/cmd/yurt-manager/names"
	nodepoolv1beta1 "github.com/openyurtio/openyurt/pkg/apis/apps/v1beta1"
	networkapi "github.com/openyurtio/openyurt/pkg/apis/network"
	networkv1alpha1 "github.com/openyurtio/openyurt/pkg/apis/network/v1alpha1"
	prvd "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/util/cloudprovider"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/util/cloudprovider/metrics"
	elbmodel "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/util/cloudprovider/model/elb"
)

func Format(format string, args ...interface{}) string {
	s := fmt.Sprintf(format, args...)
	return fmt.Sprintf("%s: %s", names.EnsLoadBalancerController, s)
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

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r *ReconcileELB) error {
	rateLimit := workqueue.NewMaxOfRateLimiter(
		workqueue.NewItemExponentialFailureRateLimiter(5*time.Second, 300*time.Second),
		// 10 qps, 100 bucket size.  This is only for retry speed and its only the overall factor (not per item)
		&workqueue.BucketRateLimiter{Limiter: rate.NewLimiter(rate.Limit(10), 100)},
	)

	c, err := controller.New(names.EnsLoadBalancerController, mgr,
		controller.Options{Reconciler: r,
			MaxConcurrentReconciles: 5,
			RateLimiter:             rateLimit,
			RecoverPanic:            true,
		})
	if err != nil {
		return err
	}

	if err = c.Watch(&source.Kind{Type: &networkv1alpha1.PoolService{}},
		&handler.EnqueueRequestForObject{},
		NewPredictionForPoolServiceEvent()); err != nil {
		return fmt.Errorf("watch resource ens nodepools error: %s", err.Error())
	}

	if err = c.Watch(&source.Kind{Type: &v1.Service{}},
		NewEnqueueRequestForServiceEvent(mgr.GetClient()),
		NewPredictionForServiceEvent()); err != nil {
		return fmt.Errorf("watch resource svc error: %s", err.Error())
	}

	if err = c.Watch(&source.Kind{Type: &discovery.EndpointSlice{}},
		NewEnqueueRequestForEndpointSliceEvent(mgr.GetClient()),
		NewPredictionForEndpointSliceEvent(mgr.GetClient())); err != nil {
		return fmt.Errorf("watch resource endpointslice error: %s", err.Error())
	}

	if err = c.Watch(&source.Kind{Type: &v1.Node{}},
		NewEnqueueRequestForNodeEvent(mgr.GetClient()),
		NewPredictionForNodeEvent()); err != nil {
		return fmt.Errorf("watch resource ens nodes error: %s", err.Error())
	}

	return nil
}

// ReconcileService implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileELB{}

// ReconcileELB reconciles a AutoRepair object
type ReconcileELB struct {
	scheme  *runtime.Scheme
	builder *ModelBuilder
	applier *ModelApplier

	// client
	provider  prvd.Provider
	clusterId string
	client    client.Client

	//record event recorder
	record    record.EventRecorder
	finalizer finalizer.Finalizers
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(ctx context.Context, c *appconfig.CompletedConfig, mgr manager.Manager) (*ReconcileELB, error) {
	if c.Config.ComponentConfig.Generic.CloudProvider == nil {
		klog.Error("can not get alibaba provider")
		return nil, fmt.Errorf("can not get alibaba provider")
	}
	recon := &ReconcileELB{
		provider:  c.Config.ComponentConfig.Generic.CloudProvider,
		client:    mgr.GetClient(),
		scheme:    mgr.GetScheme(),
		record:    mgr.GetEventRecorderFor(names.EnsLoadBalancerController),
		finalizer: finalizer.NewFinalizers(),
	}

	clusterId, err := recon.provider.GetClusterID()
	if err != nil {
		klog.Error("can not find cluster id", "error", err.Error())
		return nil, err
	}
	recon.clusterId = clusterId

	elbManager := NewELBManager(recon.provider)
	eipManager := NewEIPManager(recon.provider)
	listenerManager := NewListenerManager(recon.provider)
	serverGroupManager := NewServerGroupManager(recon.client, recon.provider)

	recon.builder = NewModelBuilder(elbManager, eipManager, listenerManager, serverGroupManager)
	recon.applier = NewModelApplier(elbManager, eipManager, listenerManager, serverGroupManager)
	err = recon.finalizer.Register(ELBFinalizer, NewLoadBalancerServiceFinalizer(mgr.GetClient()))
	if err != nil {
		klog.Info("new finalizer error %s, can ignore it", err.Error())
	}
	return recon, nil
}

func (r ReconcileELB) Reconcile(_ context.Context, request reconcile.Request) (reconcile.Result, error) {
	klog.Info(Format("reconcile: start reconcile pool service %v", request.NamespacedName))
	defer klog.Info(Format("successfully reconcile pool service %v", request.NamespacedName))
	startTime := time.Now()

	var ps networkv1alpha1.PoolService
	err := r.client.Get(context.TODO(), request.NamespacedName, &ps)
	if err != nil {
		if apierrors.IsNotFound(err) {
			klog.Info("pool service not found, skip reconcile it", "pool service: ", request.NamespacedName)
			return reconcile.Result{}, nil
		}
	}

	svcKey, npKey, err := findRelatedServiceAndNodePool(&ps)
	if err != nil {
		klog.Error(err, "owner service and nodepool not found, skip reconcile it", "pool service: ", request.NamespacedName)
		return reconcile.Result{}, nil
	}
	var svc v1.Service
	err = r.client.Get(context.TODO(), svcKey, &svc)
	if err != nil {
		if apierrors.IsNotFound(err) {
			klog.Info("service not found, skip ", "service: ", svcKey.String())
			return reconcile.Result{}, nil
		}
		klog.Error("reconcile: get service failed", "service", svcKey.String(), "error", err.Error())
		return reconcile.Result{Requeue: true, RequeueAfter: 5 * time.Second}, err
	}

	var np nodepoolv1beta1.NodePool
	err = r.client.Get(context.TODO(), npKey, &np)
	if err != nil {
		if apierrors.IsNotFound(err) {
			klog.Info("nodepool not found, skip ", "nodepool: ", npKey.String())
			return reconcile.Result{}, nil
		}
		klog.Error("reconcile: get nodepool failed", "nodepool", npKey.String(), "error", err.Error())
		return reconcile.Result{Requeue: true, RequeueAfter: 5 * time.Second}, err
	}

	poolAttr, err := r.buildPoolAttribute(&np)
	if err != nil {
		klog.Error(err, "reconcile: build pool attribute failed", "poolservice", request.String())
		return reconcile.Result{Requeue: true, RequeueAfter: 5 * time.Second}, err
	}

	ctx := context.Background()
	reqCtx := &RequestContext{
		Ctx:           ctx,
		ClusterId:     r.clusterId,
		Service:       &svc,
		PoolService:   &ps,
		PoolAttribute: &poolAttr,
		AnnoCtx:       NewAnnotationContext(svc.Annotations, ps.Annotations),
		Recorder:      r.record,
	}

	if reqCtx.AnnoCtx.IsManageByUser() && reqCtx.AnnoCtx.Get(LoadBalancerId) == "" {
		r.record.Event(reqCtx.PoolService, v1.EventTypeNormal, WaitedSyncLBId, "wait bind loadbalancer id")
		return reconcile.Result{}, nil
	}

	err = annotationValidator(reqCtx)
	if err != nil {
		r.record.Event(reqCtx.Service, v1.EventTypeWarning, FailedValidateAnnotation,
			fmt.Sprintf("Error vaildate annotation: %s", err.Error()))
		klog.Errorf("error vaildate annotation: %s， skip reconcile pool service %s", err.Error(), request.String())
		return reconcile.Result{}, nil
	}

	klog.Infof("%s: ensure loadbalancer with service details, \n%+v \n%+v", request.String(), PrettyJson(svc), PrettyJson(ps))

	if needDeleteLoadBalancer(reqCtx.PoolService) {
		err = r.cleanupLoadBalancerResources(reqCtx)
	} else {
		err = r.reconcileLoadBalancerResources(reqCtx)
	}
	if err != nil {
		klog.Errorf("reconcile: %s", err.Error())
		return reconcile.Result{Requeue: true, RequeueAfter: 5 * time.Second}, err
	}

	metric.SLBLatency.WithLabelValues("reconcile").Observe(metric.MsSince(startTime))
	return reconcile.Result{}, nil
}

func findRelatedServiceAndNodePool(ps *networkv1alpha1.PoolService) (svcKey, npKey client.ObjectKey, err error) {
	svcKey.Namespace = ps.GetNamespace()
	for idx := range ps.ObjectMeta.OwnerReferences {
		if ps.GetOwnerReferences()[idx].Kind == "Service" {
			svcKey.Name = ps.GetOwnerReferences()[idx].Name
		}
		if ps.GetOwnerReferences()[idx].Kind == "NodePool" {
			npKey.Name = ps.GetOwnerReferences()[idx].Name
		}
	}
	if svcKey.Name == "" {
		if ps.Labels == nil {
			return svcKey, npKey, fmt.Errorf("pool service labels is empty")
		}
		svcName := ps.Labels[networkapi.LabelServiceName]
		if svcName == "" {
			return svcKey, npKey, fmt.Errorf("pool service.labels[%s] is empty", networkapi.LabelServiceName)
		}
		svcKey.Name = svcName
	}

	if npKey.Name == "" {
		if ps.Labels == nil {
			return svcKey, npKey, fmt.Errorf("pool service labels is empty")
		}
		npName := ps.Labels[networkapi.LabelNodePoolName]
		if npName == "" {
			return svcKey, npKey, fmt.Errorf("pool service.labels[%s] is empty", networkapi.LabelNodePoolName)
		}
		npKey.Name = npName
	}
	return svcKey, npKey, nil
}

func (r *ReconcileELB) buildPoolAttribute(np *nodepoolv1beta1.NodePool) (PoolAttribute, error) {
	var pa PoolAttribute
	if np.Annotations == nil {
		np.Labels = make(map[string]string)
	}
	pa.NodePoolID = np.GetName()
	vpc := np.Annotations[EnsNetworkId]
	if vpc == "" {
		return pa, fmt.Errorf("ens nodepool [%s] network id is empty", np.GetName())
	}
	region := np.Annotations[EnsRegionId]
	if region == "" {
		return pa, fmt.Errorf("ens nodepool [%s] region id is empty", np.GetName())
	}
	vswitchs := np.Annotations[EnsVSwitchId]
	if vswitchs == "" {
		return pa, fmt.Errorf("ens nodepool [%s] vswitch id is empty", np.GetName())
	}
	vsws := strings.Split(vswitchs, ",")
	if len(vsws) > 0 {
		pa.VSwitchId = vsws[0]
	}
	pa.VpcId = vpc
	pa.RegionId = region
	return pa, nil
}

func needDeleteLoadBalancer(ps *networkv1alpha1.PoolService) bool {
	return ps.GetDeletionTimestamp() != nil
}

func (r *ReconcileELB) cleanupLoadBalancerResources(req *RequestContext) error {
	klog.Info(Format("pool service %s do not need lb any more, try to delete it", Key(req.PoolService)))
	if HasFinalizer(req.PoolService, ELBFinalizer) {
		_, err := r.buildAndApplyModel(req)
		if err != nil {
			r.record.Event(req.PoolService, v1.EventTypeWarning, FailedCleanLB,
				fmt.Sprintf("Error deleting load balancer for poolservice: [%s],  %s", Key(req.PoolService), err.Error()))
			return err
		}

		if err := r.removePoolServiceLables(req); err != nil {
			r.record.Event(req.PoolService, v1.EventTypeWarning, FailedRemoveHash,
				fmt.Sprintf("Error removing poolservice label: %s", err.Error()))
			return fmt.Errorf("%s failed to remove pool service labels, error: %s", Key(req.PoolService), err.Error())
		}

		if err := r.removePoolServiceStatus(req); err != nil {
			r.record.Event(req.PoolService, v1.EventTypeWarning, FailedUpdateStatus,
				fmt.Sprintf("Error removing load balancer status: %s", err.Error()))
			return fmt.Errorf("%s failed to remove pool service status, error: %s", Key(req.PoolService), err.Error())
		}

		if err := Finalize(req.Ctx, req.PoolService, r.client, r.finalizer); err != nil {
			r.record.Event(req.PoolService, v1.EventTypeWarning, FailedRemoveFinalizer,
				fmt.Sprintf("Error removing load balancer finalizer: %v", err.Error()))
			return fmt.Errorf("%s failed to remove pool service finalizer, error: %s", Key(req.PoolService), err.Error())
		}
	}
	return nil
}

func (r *ReconcileELB) reconcileLoadBalancerResources(req *RequestContext) error {
	// 1.add finalizer of elb
	if err := Finalize(req.Ctx, req.PoolService, r.client, r.finalizer); err != nil {
		r.record.Event(req.PoolService, v1.EventTypeWarning, FailedAddFinalizer,
			fmt.Sprintf("Error adding finalizer: %s", err.Error()))
		return fmt.Errorf("%s failed to add service finalizer, error: %s", Key(req.Service), err.Error())
	}

	// 2. build and apply edge lb model
	lb, err := r.buildAndApplyModel(req)
	if err != nil {
		r.record.Event(req.PoolService, v1.EventTypeWarning, FailedSyncLB,
			fmt.Sprintf("Error syncing load balancer [%s] for poolservice: [%s],  %s", lb.GetLoadBalancerId(), Key(req.PoolService), err.Error()))
		return err
	}

	// 3. add labels for service
	if err := r.addPoolServiceLables(req); err != nil {
		r.record.Event(req.PoolService, v1.EventTypeWarning, FailedAddHash,
			fmt.Sprintf("Error adding poolservice label: %s", err.Error()))
		return fmt.Errorf("%s failed to add pool service labels, error: %s", Key(req.Service), err.Error())
	}

	// 4. update status for service
	if err := r.updatePoolServiceStatus(req, lb); err != nil {
		r.record.Event(req.PoolService, v1.EventTypeWarning, FailedUpdateStatus,
			fmt.Sprintf("Error updating load balancer status: %s", err.Error()))
		return fmt.Errorf("%s failed to update pool service status, error: %s", Key(req.Service), err.Error())
	}

	r.record.Event(req.PoolService, v1.EventTypeNormal, SucceedSyncLB, fmt.Sprintf("Ensured load balancers %s", lb.GetLoadBalancerId()))
	return nil
}

func (r *ReconcileELB) buildAndApplyModel(reqCtx *RequestContext) (*elbmodel.EdgeLoadBalancer, error) {
	// build local model
	lModel, err := r.builder.BuildModel(reqCtx, LocalModel)
	if err != nil {
		return nil, fmt.Errorf("poolservice [%s] build load balancer local model error: %s", Key(reqCtx.PoolService), err.Error())
	}
	mdlJson, err := json.Marshal(lModel)
	if err != nil {
		return nil, fmt.Errorf("poolservice [%s] marshal load balancer model error: %s", Key(reqCtx.PoolService), err.Error())
	}

	klog.V(2).InfoS(fmt.Sprintf("poolservice [%s] local build: %s", Key(reqCtx.PoolService), mdlJson), "service", Key(reqCtx.Service))

	// apply model
	rModel, err := r.applier.Apply(reqCtx, lModel)
	if err != nil {
		return nil, fmt.Errorf("poolservice [%s] apply model error: %s", Key(reqCtx.PoolService), err.Error())
	}

	return rModel, nil
}

func (r *ReconcileELB) addPoolServiceLables(req *RequestContext) error {
	updated := req.PoolService.DeepCopy()
	if updated.Labels == nil {
		updated.Labels = make(map[string]string)
	}
	updated.Labels[LabelServiceHash] = GetServiceHash(req.Service, req.AnnoCtx.anno)
	if err := r.client.Patch(context.TODO(), updated, client.MergeFrom(req.PoolService)); err != nil {
		return fmt.Errorf("%s failed to add pool service hash:, error: %s", Key(req.PoolService), err.Error())
	}
	return nil
}

func (r *ReconcileELB) removePoolServiceLables(req *RequestContext) error {
	updated := req.PoolService.DeepCopy()
	if updated.Labels == nil {
		updated.Labels = make(map[string]string)
	}
	needUpdated := false
	if _, ok := updated.Labels[LabelServiceHash]; ok {
		delete(updated.Labels, LabelServiceHash)
		needUpdated = true
	}
	if needUpdated {
		err := r.client.Patch(context.TODO(), updated, client.MergeFrom(req.PoolService))
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *ReconcileELB) updatePoolServiceStatus(req *RequestContext, mdl *elbmodel.EdgeLoadBalancer) error {
	preLoadBalancerStatus := req.PoolService.Status.LoadBalancer.DeepCopy()
	newLoadBalancerStatus := v1.LoadBalancerStatus{Ingress: make([]v1.LoadBalancerIngress, 0)}
	if mdl == nil {
		return fmt.Errorf("edge model not found, cannot not patch poolservice status")
	}
	var address string
	if mdl.GetEIPAddress() != "" {
		address = mdl.GetEIPAddress()
	} else {
		address = mdl.GetLoadBalancerAddress()
	}
	if address == "" {
		return fmt.Errorf("ingress address is empty, can not patch poolservice status")
	}
	newLoadBalancerStatus.Ingress = append(newLoadBalancerStatus.Ingress, v1.LoadBalancerIngress{IP: address})

	preAggregateToAnnotations := req.PoolService.Status.AggregateToAnnotations
	newAggregateToAnnotations := make(map[string]string)
	newAggregateToAnnotations[LabelLoadBalancerId] = mdl.GetLoadBalancerId()
	if req.AnnoCtx.Get(AddressType) != elbmodel.IntranetAddressType {
		newAggregateToAnnotations[EipId] = mdl.GetEIPId()
	}

	newStatus := networkv1alpha1.PoolServiceStatus{AggregateToAnnotations: newAggregateToAnnotations, LoadBalancer: newLoadBalancerStatus}

	// Write the state if changed
	// TODO: Be careful here ... what if there were other changes to the service?
	if !v1helper.LoadBalancerStatusEqual(preLoadBalancerStatus, &newLoadBalancerStatus) ||
		!reflect.DeepEqual(preAggregateToAnnotations, newAggregateToAnnotations) {
		klog.InfoS(fmt.Sprintf("status: [%v] [%v]", req.PoolService.Status.LoadBalancer, newStatus.LoadBalancer), "poolservice", Key(req.PoolService))
		var retErr error
		_ = Retry(
			&wait.Backoff{
				Duration: 1 * time.Second,
				Steps:    3,
				Factor:   2,
				Jitter:   4,
			},
			func(ps *networkv1alpha1.PoolService) error {
				// get latest svc from the shared informer cache
				psOld := &networkv1alpha1.PoolService{}
				retErr = r.client.Get(req.Ctx, NamespacedName(ps), psOld)
				if retErr != nil {
					return fmt.Errorf("error to get poolservice %s", Key(ps))
				}
				updated := psOld.DeepCopy()
				updated.Status = newStatus
				klog.InfoS(fmt.Sprintf("LoadBalancer: %v", updated.Status.LoadBalancer), "poolservice", Key(ps))
				retErr = r.client.Status().Patch(req.Ctx, updated, client.MergeFrom(psOld))
				if retErr == nil {
					return nil
				}

				// If the object no longer exists, we don't want to recreate it. Just bail
				// out so that we can process the delete, which we should soon be receiving
				// if we haven't already.
				if apierrors.IsNotFound(retErr) {
					klog.ErrorS(retErr, "not persisting update to service that no longer exists", "poolservice", Key(ps))
					retErr = nil
					return nil
				}
				// TODO: Try to resolve the conflict if the change was unrelated to load
				// balancer status. For now, just pass it up the stack.
				if apierrors.IsConflict(retErr) {
					return fmt.Errorf("not persisting update to poolservice %s that "+
						"has been changed since we received it: %v", Key(ps), retErr)
				}
				klog.ErrorS(retErr, "failed to persist updated LoadBalancerStatus"+
					" after creating its load balancer", "poolservice", Key(ps))
				return fmt.Errorf("retry with %s, %s", retErr.Error(), TRY_AGAIN)
			},
			req.PoolService,
		)
		return retErr
	}

	return nil
}

func (r *ReconcileELB) removePoolServiceStatus(req *RequestContext) error {
	preStatus := req.PoolService.Status.DeepCopy()
	newStatus := networkv1alpha1.PoolServiceStatus{}
	if !v1helper.LoadBalancerStatusEqual(&preStatus.LoadBalancer, &newStatus.LoadBalancer) {
		klog.InfoS(fmt.Sprintf("status: [%v] [%v]", preStatus, newStatus), "poolservice", Key(req.PoolService))
	}
	return Retry(
		&wait.Backoff{Duration: 1 * time.Second, Steps: 3, Factor: 2, Jitter: 4},
		func(ps *networkv1alpha1.PoolService) error {
			psOld := &networkv1alpha1.PoolService{}
			err := r.client.Get(req.Ctx, NamespacedName(ps), psOld)
			if err != nil {
				return fmt.Errorf("error to get poolservice %s", Key(ps))
			}
			updated := psOld.DeepCopy()
			updated.Status = newStatus
			klog.InfoS(fmt.Sprintf("LoadBalancer: %v", updated.Status.LoadBalancer), "poolservice", Key(ps))
			err = r.client.Status().Patch(req.Ctx, updated, client.MergeFrom(psOld))
			if err != nil {
				if apierrors.IsNotFound(err) {
					klog.InfoS("not persisting update to service that no longer exists", "poolservice", Key(ps))
					return nil
				}
				if apierrors.IsConflict(err) {
					return fmt.Errorf("not persisting update to poolservice %s that "+
						"has been changed since we received it: %v", Key(ps), err)
				}
				klog.ErrorS(err, "failed to persist updated LoadBalancerStatus after creating its load balancer", "poolservice", Key(ps))
				return fmt.Errorf("retry with %s, %s", err.Error(), TRY_AGAIN)
			}
			return nil
		},
		req.PoolService)
}
