package elb

import (
	"context"
	"encoding/json"
	"fmt"
	"golang.org/x/time/rate"
	"strings"
	"sync"
	"time"

	v1 "k8s.io/api/core/v1"
	discovery "k8s.io/api/discovery/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/selection"
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
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	appconfig "github.com/openyurtio/openyurt/cmd/yurt-manager/app/config"
	"github.com/openyurtio/openyurt/cmd/yurt-manager/names"
	nodepoolv1beta1 "github.com/openyurtio/openyurt/pkg/apis/apps/v1beta1"
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
			MaxConcurrentReconciles: 2,
			RateLimiter:             rateLimit,
			RecoverPanic:            true,
		})
	if err != nil {
		return err
	}

	if err = c.Watch(&source.Kind{Type: &v1.Service{}},
		&handler.EnqueueRequestForObject{},
		NewPredictionForServiceEvent(mgr.GetClient(), mgr.GetEventRecorderFor(names.EnsLoadBalancerController))); err != nil {
		return fmt.Errorf("watch resource svc error: %s", err.Error())
	}

	if err = c.Watch(&source.Kind{Type: &discovery.EndpointSlice{}},
		NewEnqueueRequestForEndpointSliceEvent(mgr.GetClient()),
		NewPredictionForEndpointSliceEvent(mgr.GetClient(), mgr.GetEventRecorderFor(names.EnsLoadBalancerController))); err != nil {
		return fmt.Errorf("watch resource endpointslice error: %s", err.Error())
	}

	if err = c.Watch(&source.Kind{Type: &v1.Node{}},
		NewEnqueueRequestForNodeEvent(mgr.GetClient(), mgr.GetEventRecorderFor(names.EnsLoadBalancerController)),
		predicate.NewPredicateFuncs(
			func(object client.Object) bool {
				node, ok := object.(*v1.Node)
				if ok && IsENSNode(node) {
					return true
				}
				return false
			})); err != nil {
		return fmt.Errorf("watch resource ens nodes error: %s", err.Error())
	}

	if err = c.Watch(&source.Kind{Type: &nodepoolv1beta1.NodePool{}},
		NewEnqueueRequestForNodePoolEvent(mgr.GetClient(), mgr.GetEventRecorderFor(names.EnsLoadBalancerController)),
		predicate.NewPredicateFuncs(func(object client.Object) bool {
			nodepool, ok := object.(*nodepoolv1beta1.NodePool)
			if ok && IsENSNodePool(nodepool) {
				return true
			}
			return false
		})); err != nil {
		return fmt.Errorf("watch resource ens nodepools error: %s", err.Error())
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
	provider prvd.Provider
	client   client.Client

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

	elbManager := NewELBManager(recon.provider)
	eipManager := NewEIPManager(recon.provider)
	listenerManager := NewListenerManager(recon.provider)
	serverGroupManager := NewServerGroupManager(recon.client, recon.provider)

	recon.builder = NewModelBuilder(elbManager, eipManager, listenerManager, serverGroupManager)
	recon.applier = NewModelApplier(elbManager, eipManager, listenerManager, serverGroupManager)
	err := recon.finalizer.Register(ELBFinalizer, NewLoadBalancerServiceFinalizer(mgr.GetClient()))
	if err != nil {
		klog.Info("new finalizer error %s, can ignore it", err.Error())
	}
	return recon, nil
}

func (r ReconcileELB) Reconcile(_ context.Context, request reconcile.Request) (reconcile.Result, error) {
	klog.Info(Format("reconcile: start reconcile service %v", request.NamespacedName))
	defer klog.Info(Format("successfully reconcile service %v", request.NamespacedName))
	startTime := time.Now()
	svc := &v1.Service{}
	err := r.client.Get(context.Background(), request.NamespacedName, svc)
	if err != nil {
		if apierrors.IsNotFound(err) {
			klog.Info("service not found, skip ", "service: ", request.NamespacedName)
			return reconcile.Result{}, nil
		}
		klog.Error("reconcile: get service failed", "service", request.NamespacedName, "error", err.Error())
		return reconcile.Result{Requeue: true, RequeueAfter: 5 * time.Second}, err
	}

	CID, err := r.provider.GetClusterID()
	if err != nil {
		klog.Error("reconcile: get cluster id failed", "service", request.NamespacedName, "error", err.Error())
		return reconcile.Result{Requeue: true, RequeueAfter: 5 * time.Second}, err
	}

	ctx := context.Background()
	reqCtx := &RequestContext{
		Ctx:       ctx,
		ClusterId: CID,
		Service:   svc,
		AnnoCtx:   NewAnnotationContext(svc),
		Recorder:  r.record,
	}

	klog.Infof("%s: ensure loadbalancer with service details, \n%+v", Key(svc), PrettyJson(svc))

	if needDeleteLoadBalancer(svc) {
		err = r.cleanupLoadBalancerResources(reqCtx)
	} else {
		err = r.reconcileLoadBalancerResources(reqCtx)
	}
	if err != nil {
		klog.Error("reconcile: %s", err.Error())
		return reconcile.Result{Requeue: true, RequeueAfter: 5 * time.Second}, err
	}
	metric.SLBLatency.WithLabelValues("reconcile").Observe(metric.MsSince(startTime))
	return reconcile.Result{}, nil
}

type model struct {
	poolIdentities map[string]*elbmodel.PoolIdentity
}

func needDeleteLoadBalancer(svc *v1.Service) bool {
	return svc.DeletionTimestamp != nil || svc.Spec.Type != v1.ServiceTypeLoadBalancer
}

func (r *ReconcileELB) cleanupLoadBalancerResources(req *RequestContext) error {
	klog.Info(Format("service %s do not need lb any more, try to delete it", Key(req.Service)))
	if HasFinalizer(req.Service, ELBFinalizer) {
		mdl, errList := r.reconcileLoadBalancer(req)
		if len(errList) > 0 {
			r.record.Event(req.Service, v1.EventTypeWarning, FailedSyncLB,
				fmt.Sprintf("Error deleting load balancer: %s", getEventMessage(mdl)))
			return fmt.Errorf("%s failed to clearnup load balancer, error %s", Key(req.Service), getErrorListMessage(errList))
		}

		if err := r.removeServiceLabels(req.Service); err != nil {
			r.record.Event(req.Service, v1.EventTypeWarning, FailedRemoveHash,
				fmt.Sprintf("Error removing service label: %s", err.Error()))
			return fmt.Errorf("%s failed to remove service labels, error: %s", Key(req.Service), err.Error())
		}

		// When service type changes from LoadBalancer to NodePort,
		// we need to clean Ingress attribute in service status
		if err := r.removeServiceStatus(req, req.Service); err != nil {
			r.record.Event(req.Service, v1.EventTypeWarning, FailedUpdateStatus,
				fmt.Sprintf("Error removing load balancer status: %s", err.Error()))
			return fmt.Errorf("%s failed to remove load balancer status, error: %s", Key(req.Service), err.Error())
		}

		if err := Finalize(req.Ctx, req.Service, r.client, r.finalizer); err != nil {
			r.record.Event(req.Service, v1.EventTypeWarning, FailedRemoveFinalizer,
				fmt.Sprintf("Error removing load balancer finalizer: %v", err.Error()))
			return fmt.Errorf("%s failed to remove service finalizer, error: %s", Key(req.Service), err.Error())
		}
	}
	return nil
}

func (r *ReconcileELB) reconcileLoadBalancerResources(req *RequestContext) error {
	// 1.add finalizer of elb
	if err := Finalize(req.Ctx, req.Service, r.client, r.finalizer); err != nil {
		r.record.Event(req.Service, v1.EventTypeWarning, FailedAddFinalizer,
			fmt.Sprintf("Error adding finalizer: %s", err.Error()))
		return fmt.Errorf("%s failed to add service finalizer, error: %s", Key(req.Service), err.Error())
	}

	// 2. build and apply edge lb model
	mdl, errList := r.reconcileLoadBalancer(req)
	if len(errList) > 0 {
		r.record.Event(req.Service, v1.EventTypeWarning, FailedSyncLB,
			fmt.Sprintf("Error build load balancers for nodepool: %s", getEventMessage(mdl)))
	}

	// 3. add labels for service
	if err := r.addServiceLabels(req.Service, mdl); err != nil {
		r.record.Event(req.Service, v1.EventTypeWarning, FailedAddHash,
			fmt.Sprintf("Error adding service label: %s", err.Error()))
		return fmt.Errorf("%s failed to add service labels, error: %s", Key(req.Service), err.Error())
	}

	// 4. update status for service
	if err := r.updateServiceStatus(req, req.Service, mdl); err != nil {
		r.record.Event(req.Service, v1.EventTypeWarning, FailedUpdateStatus,
			fmt.Sprintf("Error updating load balancer status: %s", err.Error()))
		return fmt.Errorf("%s failed to update load balancer status, error: %s", Key(req.Service), err.Error())
	}

	if len(errList) > 0 {
		return fmt.Errorf("%s failed to build load balancer, error %s", Key(req.Service), getErrorListMessage(errList))
	}

	r.record.Event(req.Service, v1.EventTypeNormal, SucceedSyncLB, "Ensured all load balancers")
	return nil
}

func (r *ReconcileELB) reconcileLoadBalancer(reqCtx *RequestContext) (*model, []error) {
	var errList []error
	mdl, err := r.buildModel(reqCtx)

	if err != nil {
		errList = append(errList, err)
		return nil, errList
	}

	keys := make([]string, 0)
	for k := range mdl.poolIdentities {
		keys = append(keys, k)
	}

	var num, iter int
	num = len(mdl.poolIdentities)
	if (num % BatchSize) == 0 {
		iter = num / BatchSize
	} else {
		iter = (num / BatchSize) + 1
	}

	for i := 0; i < iter; i++ {
		if i == iter-1 {
			r.batchProcess(reqCtx, keys[i*BatchSize:], mdl)
		} else {
			r.batchProcess(reqCtx, keys[i*BatchSize:(i+1)*BatchSize], mdl)
		}
	}
	errList = append(errList, r.handleError(reqCtx, mdl)...)
	return mdl, errList
}

func (r *ReconcileELB) buildModel(reqCtx *RequestContext) (*model, error) {
	hasLabel := client.HasLabels{EnsNetworkId, EnsRegionId}
	var nodepoolList nodepoolv1beta1.NodePoolList
	networks := make(map[string]string, 0)
	err := r.client.List(reqCtx.Ctx, &nodepoolList, &hasLabel)
	if err != nil {
		return nil, fmt.Errorf("list ens nodepool error %s", err.Error())
	}
	for idx := range nodepoolList.Items {
		vpcId := nodepoolList.Items[idx].Labels[EnsNetworkId]
		if vpcId != "" {
			networks[vpcId] = nodepoolList.Items[idx].Name
		}
	}

	nw, vs, rg, err := r.provider.FindHadLoadBalancerNetwork(reqCtx.Ctx, GetDefaultLoadBalancerName(reqCtx))
	if err != nil {
		return nil, fmt.Errorf("find had loadbalancer network error %s", err.Error())
	}

	mdl := &model{poolIdentities: make(map[string]*elbmodel.PoolIdentity, 0)}
	for idx := range nw {
		mdl.poolIdentities[networks[nw[idx]]] = elbmodel.NewIdentity(networks[nw[idx]], nw[idx], vs[idx], rg[idx], elbmodel.Delete)
	}

	if needDeleteLoadBalancer(reqCtx.Service) {
		return mdl, nil
	}
	selector := getNodePoolSelector(reqCtx)
	if selector == nil {
		return nil, fmt.Errorf("can not get nodepool selector, error lacks annotation %s", Annotation(NodePoolSelector))
	}
	err = r.client.List(reqCtx.Ctx, &nodepoolList, &client.ListOptions{LabelSelector: selector})
	if err != nil {
		return nil, fmt.Errorf("list selected ens nodepool error %s", err.Error())
	}
	for _, nodepool := range nodepoolList.Items {
		network, vswitch, region := getNetworkAttribute(reqCtx, &nodepool)
		if network == "" || region == "" || vswitch == "" {
			return nil, fmt.Errorf("")
		}
		if _, ok := mdl.poolIdentities[nodepool.Name]; ok {
			mdl.poolIdentities[nodepool.Name].SetAction(elbmodel.Update)
		} else {
			mdl.poolIdentities[nodepool.Name] = elbmodel.NewIdentity(nodepool.Name, network, vswitch, region, elbmodel.Create)
		}
	}
	return mdl, nil
}

func (r *ReconcileELB) batchProcess(reqCtx *RequestContext, keys []string, mdl *model) {
	var wg sync.WaitGroup
	for _, val := range keys {
		wg.Add(1)
		go func(name string, wg *sync.WaitGroup) {
			err := r.buildAndApplyModel(reqCtx, mdl.poolIdentities[name])
			mdl.poolIdentities[name].SetError(err)
			wg.Done()
		}(val, &wg)
	}
	wg.Wait()
}

func (r *ReconcileELB) handleError(reqCtx *RequestContext, mdl *model) []error {
	for retry := 0; retry < MaxRetryError; retry++ {
		for key, val := range mdl.poolIdentities {
			if val.GetError() != nil {
				mdl.poolIdentities[key].SetError(r.buildAndApplyModel(reqCtx, mdl.poolIdentities[key]))
			}
		}
	}
	var errList []error
	for _, val := range mdl.poolIdentities {
		if err := val.GetError(); err != nil {
			errList = append(errList, err)
		}
	}
	return errList
}

func (r *ReconcileELB) buildAndApplyModel(reqCtx *RequestContext, pool *elbmodel.PoolIdentity) error {
	// build local model
	lModel, err := r.builder.BuildModel(reqCtx, LocalModel, pool)
	if err != nil {
		return fmt.Errorf("nodepool [%s] build load balancer local model error: %s", pool.GetName(), err.Error())
	}
	mdlJson, err := json.Marshal(lModel)
	if err != nil {
		return fmt.Errorf("nodepool [%s] marshal load balancer model error: %s", pool.GetName(), err.Error())
	}

	klog.V(5).InfoS(fmt.Sprintf("nodepool [%s] local build: %s", pool.GetName(), mdlJson), "service", Key(reqCtx.Service))

	// apply model
	rModel, err := r.applier.Apply(reqCtx, pool, lModel)
	if err != nil {
		return fmt.Errorf("nodepool [%s] apply model error: %s", pool.GetName(), err.Error())
	}
	pool.SetLoadBalancer(rModel.GetLoadBalancerId())
	if rModel.GetEIPId() != "" {
		pool.SetEIP(rModel.GetEIPId())
	}
	if lModel.GetAddressType() == elbmodel.IntranetAddressType {
		pool.SetAddress(rModel.GetLoadBalancerAddress())
	} else {
		pool.SetAddress(rModel.GetEIPAddress())
	}

	return nil
}

func (r *ReconcileELB) addServiceLabels(svc *v1.Service, mdl *model) error {
	updated := svc.DeepCopy()
	if updated.Labels == nil {
		updated.Labels = make(map[string]string)
	}
	serviceHash := GetServiceHash(svc)
	updated.Labels[LabelServiceHash] = serviceHash

	var loadbalancers, eips []string
	for key, val := range mdl.poolIdentities {
		if val.GetAction() != elbmodel.Delete && val.GetError() == nil {
			loadbalancers = append(loadbalancers, fmt.Sprintf("%s=%s", key, val.GetLoadBalancer()))
			if val.GetEIP() != "" {
				eips = append(eips, fmt.Sprintf("%s=%s", key, val.GetEIP()))
			}
		}
	}
	if len(loadbalancers) > 0 {
		updated.Annotations[LabelLoadBalancerId] = strings.Join(loadbalancers, ",")
	}
	if len(eips) > 0 {
		updated.Annotations[EipId] = strings.Join(eips, ",")
	}
	if err := r.client.Status().Patch(context.Background(), updated, client.MergeFrom(svc)); err != nil {
		return fmt.Errorf("%s failed to add service hash:, error: %s", Key(svc), err.Error())
	}
	return nil
}

func (r *ReconcileELB) removeServiceLabels(svc *v1.Service) error {
	updated := svc.DeepCopy()
	needUpdated := false
	if _, ok := updated.Labels[LabelServiceHash]; ok {
		delete(updated.Labels, LabelServiceHash)
		needUpdated = true
	}

	delete(updated.Annotations, LabelLoadBalancerId)
	delete(updated.Annotations, EipId)

	if needUpdated {
		err := r.client.Status().Patch(context.TODO(), updated, client.MergeFrom(svc))
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *ReconcileELB) updateServiceStatus(reqCtx *RequestContext, svc *v1.Service, mdl *model) error {
	preStatus := svc.Status.LoadBalancer.DeepCopy()
	newStatus := &v1.LoadBalancerStatus{Ingress: make([]v1.LoadBalancerIngress, 0)}
	if mdl == nil {
		return fmt.Errorf("edge model not found, cannot not patch service status")
	}
	for _, v := range mdl.poolIdentities {
		if v.GetAction() == elbmodel.Delete || v.GetError() != nil {
			continue
		}
		if v.GetAddress() == "" {
			return fmt.Errorf("eip address is empty, can not patch service status")
		} else {
			newStatus.Ingress = append(newStatus.Ingress, v1.LoadBalancerIngress{IP: v.GetAddress()})
		}
	}

	// Write the state if changed
	// TODO: Be careful here ... what if there were other changes to the service?
	if !v1helper.LoadBalancerStatusEqual(preStatus, newStatus) {
		klog.InfoS(fmt.Sprintf("status: [%v] [%v]", preStatus, newStatus), "service", Key(svc))
		var retErr error
		_ = Retry(
			&wait.Backoff{
				Duration: 1 * time.Second,
				Steps:    3,
				Factor:   2,
				Jitter:   4,
			},
			func(svc *v1.Service) error {
				// get latest svc from the shared informer cache
				svcOld := &v1.Service{}
				retErr = r.client.Get(reqCtx.Ctx, NamespacedName(svc), svcOld)
				if retErr != nil {
					return fmt.Errorf("error to get svc %s", Key(svc))
				}
				updated := svcOld.DeepCopy()
				updated.Status.LoadBalancer = *newStatus
				klog.InfoS(fmt.Sprintf("LoadBalancer: %v", updated.Status.LoadBalancer), "service", Key(svc))
				retErr = r.client.Status().Patch(reqCtx.Ctx, updated, client.MergeFrom(svcOld))
				if retErr == nil {
					return nil
				}

				// If the object no longer exists, we don't want to recreate it. Just bail
				// out so that we can process the delete, which we should soon be receiving
				// if we haven't already.
				if apierrors.IsNotFound(retErr) {
					klog.ErrorS(retErr, "not persisting update to service that no longer exists", "service", Key(svc))
					retErr = nil
					return nil
				}
				// TODO: Try to resolve the conflict if the change was unrelated to load
				// balancer status. For now, just pass it up the stack.
				if apierrors.IsConflict(retErr) {
					return fmt.Errorf("not persisting update to service %s that "+
						"has been changed since we received it: %v", Key(svc), retErr)
				}
				klog.ErrorS(retErr, "failed to persist updated LoadBalancerStatus"+
					" after creating its load balancer", "service", Key(svc))
				return fmt.Errorf("retry with %s, %s", retErr.Error(), TRY_AGAIN)
			},
			svc,
		)
		return retErr
	}

	return nil
}

func (r *ReconcileELB) removeServiceStatus(reqCtx *RequestContext, svc *v1.Service) error {
	preStatus := svc.Status.LoadBalancer.DeepCopy()
	newStatus := &v1.LoadBalancerStatus{}
	if !v1helper.LoadBalancerStatusEqual(preStatus, newStatus) {
		klog.InfoS(fmt.Sprintf("status: [%v] [%v]", preStatus, newStatus), "service", Key(svc))
	}
	return Retry(
		&wait.Backoff{Duration: 1 * time.Second, Steps: 3, Factor: 2, Jitter: 4},
		func(svc *v1.Service) error {
			oldSvc := &v1.Service{}
			err := r.client.Get(reqCtx.Ctx, NamespacedName(svc), oldSvc)
			if err != nil {
				return fmt.Errorf("get svc %s, err", Key(svc))
			}
			updated := oldSvc.DeepCopy()
			updated.Status.LoadBalancer = *newStatus
			klog.InfoS(fmt.Sprintf("LoadBalancer: %v", updated.Status.LoadBalancer), "service", Key(svc))
			err = r.client.Status().Patch(reqCtx.Ctx, updated, client.MergeFrom(oldSvc))
			if err != nil {
				if apierrors.IsNotFound(err) {
					klog.InfoS("not persisting update to service that no longer exists", "service", Key(svc))
					return nil
				}
				if apierrors.IsConflict(err) {
					return fmt.Errorf("not persisting update to service %s that "+
						"has been changed since we received it: %v", Key(svc), err)
				}
				klog.ErrorS(err, "failed to persist updated LoadBalancerStatus after creating its load balancer", "service", Key(svc))
				return fmt.Errorf("retry with %s, %s", err.Error(), TRY_AGAIN)
			}
			return nil
		},
		svc)
}

func getNodePoolSelector(reqCtx *RequestContext) labels.Selector {
	var selector labels.Selector
	set := make(map[string]string, 0)
	nodePoolSelector := reqCtx.AnnoCtx.Get(NodePoolSelector)
	if nodePoolSelector == "" {
		return nil
	}
	labelSelector := strings.Split(nodePoolSelector, ",")
	for idx := range labelSelector {
		keyAndValue := strings.Split(labelSelector[idx], "=")
		if len(keyAndValue) == 2 && keyAndValue[0] != "" && keyAndValue[1] != "" {
			set[keyAndValue[0]] = keyAndValue[1]
		}
	}
	selector = labels.SelectorFromSet(set).Add(getDefaultRequired()...)
	return selector
}

func getDefaultRequired() labels.Requirements {
	req := make(labels.Requirements, 0)
	networkReq, err := labels.NewRequirement(EnsNetworkId, selection.Exists, []string{})
	if err != nil {
		klog.ErrorS(err, fmt.Sprintf("new default requirement error label=[%s]", EnsNetworkId))
	} else {
		req = append(req, *networkReq)
	}

	regionReq, err := labels.NewRequirement(EnsRegionId, selection.Exists, []string{})
	if err != nil {
		klog.ErrorS(err, fmt.Sprintf("new default requirement error label=[%s] ", EnsRegionId))
	} else {
		req = append(req, *regionReq)
	}
	return req
}

func getEventMessage(mdl *model) string {
	np := make([]string, 0)
	for key, val := range mdl.poolIdentities {
		if val.GetError() != nil {
			np = append(np, key)
		}
	}
	return fmt.Sprintf("[%s]", strings.Join(np, ","))
}

func getErrorListMessage(errList []error) string {
	errMessages := make([]string, len(errList))
	for idx := range errList {
		errMessages[idx] = fmt.Sprintf("errors [%d] : %s", idx, errList[idx].Error())
	}
	return strings.Join(errMessages, "\n")
}

func getNetworkAttribute(reqCtx *RequestContext, np *nodepoolv1beta1.NodePool) (nw, vsw, region string) {
	nw = np.Labels[EnsNetworkId]
	region = np.Labels[EnsRegionId]
	if nw == "" {
		return
	}
	vsws := strings.Split(reqCtx.AnnoCtx.Get(VSwitch), ",")
	keyAndVal := make(map[string]string)
	if vsws != nil {
		for idx := range vsws {
			kv := strings.Split(vsws[idx], "=")
			if len(kv) == 2 {
				keyAndVal[kv[0]] = kv[1]
			}
		}
	}
	vsw, ok := keyAndVal[nw]
	if ok && vsw != "" {
		return
	}
	vsws = strings.Split(np.Annotations[EnsVSwitchId], ",")
	if len(vsws) > 0 {
		vsw = vsws[0]
		return
	}
	return
}
