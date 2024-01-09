package elb

import (
	"context"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"

	v1 "k8s.io/api/core/v1"
	discovery "k8s.io/api/discovery/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	nodepoolv1beta1 "github.com/openyurtio/openyurt/pkg/apis/apps/v1beta1"
)

const (
	IncorrectAnnotation = "IncorrectAnnotations"
)

var _ predicate.Predicate = (*predicationForServiceEvent)(nil)

type predicationForServiceEvent struct {
	client   client.Client
	recorder record.EventRecorder
}

func NewPredictionForServiceEvent(client client.Client, recorder record.EventRecorder) *predicationForServiceEvent {
	return &predicationForServiceEvent{client: client, recorder: recorder}
}

func (p *predicationForServiceEvent) Create(evt event.CreateEvent) bool {
	svc, ok := evt.Object.(*v1.Service)
	if !ok || !needAdd(svc) {
		return false
	}
	if err := validateAnnotation(svc); err != nil {
		klog.Errorf("incorrect annotation, error %s", err.Error())
		p.recorder.Event(svc, v1.EventTypeWarning, IncorrectAnnotation, err.Error())
		return false
	}
	return true
}

func (p *predicationForServiceEvent) Update(evt event.UpdateEvent) bool {
	newSvc, ok1 := evt.ObjectNew.(*v1.Service)
	oldSvc, ok2 := evt.ObjectOld.(*v1.Service)
	if !ok1 || !ok2 || !needUpdate(oldSvc, newSvc) {
		return false
	}
	if err := validateAnnotation(newSvc); err != nil {
		klog.Errorf("incorrect annotation, error %s", err.Error())
		p.recorder.Event(newSvc, v1.EventTypeWarning, IncorrectAnnotation, err.Error())
		return false
	}
	return true

}

func (p *predicationForServiceEvent) Delete(evt event.DeleteEvent) bool {
	return false
}

func (p *predicationForServiceEvent) Generic(evt event.GenericEvent) bool {
	return false
}

var _ predicate.Predicate = (*predicationForEndpointSliceEvent)(nil)

type predicationForEndpointSliceEvent struct {
	client   client.Client
	recorder record.EventRecorder
}

func NewPredictionForEndpointSliceEvent(client client.Client, recorder record.EventRecorder) *predicationForEndpointSliceEvent {
	return &predicationForEndpointSliceEvent{client: client, recorder: recorder}
}

func (p *predicationForEndpointSliceEvent) Create(evt event.CreateEvent) bool {
	es, ok := evt.Object.(*discovery.EndpointSlice)
	if ok && canEnqueueEndpointSlice(es, p.client, p.recorder) {
		return true
	}
	return false
}

func (p *predicationForEndpointSliceEvent) Update(evt event.UpdateEvent) bool {
	es1, ok1 := evt.ObjectOld.(*discovery.EndpointSlice)
	es2, ok2 := evt.ObjectNew.(*discovery.EndpointSlice)
	if ok1 && ok2 && canEnqueueEndpointSlice(es1, p.client, p.recorder) && isEndpointSliceChanged(es1, es2) {
		return true
	}
	return false
}

func (p *predicationForEndpointSliceEvent) Delete(evt event.DeleteEvent) bool {
	es, ok := evt.Object.(*discovery.EndpointSlice)
	if ok && canEnqueueEndpointSlice(es, p.client, p.recorder) {
		return true
	}
	return false
}

func (p *predicationForEndpointSliceEvent) Generic(evt event.GenericEvent) bool {
	return false
}

// NewEnqueueRequestForEndpointSliceEvent, event handler for endpointslice event
func NewEnqueueRequestForEndpointSliceEvent(client client.Client) *enqueueRequestForEndpointSliceEvent {
	return &enqueueRequestForEndpointSliceEvent{
		client: client,
	}
}

type enqueueRequestForEndpointSliceEvent struct {
	client client.Client
}

var _ handler.EventHandler = (*enqueueRequestForEndpointSliceEvent)(nil)

func (h *enqueueRequestForEndpointSliceEvent) Create(e event.CreateEvent, queue workqueue.RateLimitingInterface) {
	es, ok := e.Object.(*discovery.EndpointSlice)
	if ok {
		klog.V(4).Info("controller: endpointslice create event", "endpointslice", Key(es))
		h.enqueueManagedEndpointSlice(queue, es)
	}
}

func (h *enqueueRequestForEndpointSliceEvent) Update(e event.UpdateEvent, queue workqueue.RateLimitingInterface) {
	es1, ok1 := e.ObjectOld.(*discovery.EndpointSlice)
	es2, ok2 := e.ObjectNew.(*discovery.EndpointSlice)

	if ok1 && ok2 {
		klog.V(4).Info(fmt.Sprintf("controller: endpointslice update event, endpoints before [%s], afeter [%s]",
			LogEndpointSlice(es1), LogEndpointSlice(es2)), "endpointslice", Key(es1))
		h.enqueueManagedEndpointSlice(queue, es1)
	}
}

func (h *enqueueRequestForEndpointSliceEvent) Delete(e event.DeleteEvent, queue workqueue.RateLimitingInterface) {
	es, ok := e.Object.(*discovery.EndpointSlice)
	if ok {
		klog.V(4).Info("controller: endpointslice delete event", "endpointslice", Key(es))
		h.enqueueManagedEndpointSlice(queue, es)
	}
}

func (h *enqueueRequestForEndpointSliceEvent) Generic(e event.GenericEvent, queue workqueue.RateLimitingInterface) {
	// unknown event, ignore
}

func (h *enqueueRequestForEndpointSliceEvent) enqueueManagedEndpointSlice(queue workqueue.RateLimitingInterface, endpointSlice *discovery.EndpointSlice) {
	serviceName, ok := endpointSlice.Labels[discovery.LabelServiceName]
	if !ok {
		return
	}

	queue.Add(reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: endpointSlice.Namespace,
			Name:      serviceName,
		},
	})

	klog.Info("enqueue", "endpointslice", Key(endpointSlice), "queueLen", queue.Len())
}

func canEnqueueEndpointSlice(es *discovery.EndpointSlice, client client.Client, recorder record.EventRecorder) bool {
	if es == nil {
		return false
	}

	serviceName, ok := es.Labels[discovery.LabelServiceName]
	if !ok {
		return false
	}

	svc := &v1.Service{}
	err := client.Get(context.TODO(),
		types.NamespacedName{
			Namespace: es.Namespace,
			Name:      serviceName,
		}, svc)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			klog.Error(err, "fail to get service, skip reconcile endpointslice",
				"endpointslice", Key(es), "service", serviceName)
		}
		return false
	}

	if !isELBService(svc) {
		// it is safe not to reconcile endpointslice which belongs to the non-loadbalancer svc
		klog.V(4).Info("endpointslice change: loadBalancer is not needed, skip ",
			"endpointslice ", Key(es))
		return false
	}

	err = validateAnnotation(svc)
	if err != nil {
		klog.Errorf("incorrect annotation, error %s", err.Error())
		recorder.Event(svc, v1.EventTypeWarning, IncorrectAnnotation, err.Error())
		return false
	}

	return true
}

func isEndpointSliceChanged(old, new *discovery.EndpointSlice) bool {
	return !reflect.DeepEqual(old.Endpoints, new.Endpoints) || !reflect.DeepEqual(old.Ports, new.Ports)
}

// NewEnqueueRequestForNodeEvent, event handler for node event
func NewEnqueueRequestForNodeEvent(client client.Client, record record.EventRecorder) *enqueueRequestForNodeEvent {
	return &enqueueRequestForNodeEvent{
		client: client,
	}
}

type enqueueRequestForNodeEvent struct {
	client client.Client
}

var _ handler.EventHandler = (*enqueueRequestForNodeEvent)(nil)

func (h *enqueueRequestForNodeEvent) Create(e event.CreateEvent, queue workqueue.RateLimitingInterface) {
	node, ok := e.Object.(*v1.Node)
	if ok {
		klog.V(4).Info("controller: node create event", "node", node.Name)
		enqueueELBServices(queue, h.client, Key(node), "node")
	}
}

func (h *enqueueRequestForNodeEvent) Update(e event.UpdateEvent, queue workqueue.RateLimitingInterface) {
	oldNode, ok1 := e.ObjectOld.(*v1.Node)
	newNode, ok2 := e.ObjectNew.(*v1.Node)

	if ok1 && ok2 {
		//if node label and schedulable condition changed, need to reconcile svc
		if nodeChanged(oldNode, newNode) {
			klog.V(4).Info("controller: node update event", "node", oldNode.Name)
			enqueueELBServices(queue, h.client, Key(newNode), "node")
		}
	}
}

func (h *enqueueRequestForNodeEvent) Delete(e event.DeleteEvent, queue workqueue.RateLimitingInterface) {
	node, ok := e.Object.(*v1.Node)
	if ok {
		klog.V(4).Info("controller: node delete event", "node", node.Name)
		enqueueELBServices(queue, h.client, Key(node), "node")
	}
}

func (h *enqueueRequestForNodeEvent) Generic(e event.GenericEvent, queue workqueue.RateLimitingInterface) {

}

func enqueueELBServices(queue workqueue.RateLimitingInterface, client client.Client, objectKey string, objectType string) {
	svcs := v1.ServiceList{}
	err := client.List(context.TODO(), &svcs)
	if err != nil {
		klog.Error(err, "fail to list services for object", "object type: ", objectKey, "object key: ", objectType)
		return
	}
	for _, v := range svcs.Items {
		if !isELBService(&v) {
			continue
		}
		queue.Add(reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: v.Namespace,
				Name:      v.Name,
			},
		})
	}
}

type enqueueRequestForNodePoolEvent struct {
	client        client.Client
	eventRecorder record.EventRecorder
}

var _ handler.EventHandler = (*enqueueRequestForNodePoolEvent)(nil)

// NewEnqueueRequestForNodePoolEvent, event handler for nodepool event
func NewEnqueueRequestForNodePoolEvent(client client.Client, record record.EventRecorder) *enqueueRequestForNodePoolEvent {
	return &enqueueRequestForNodePoolEvent{
		client:        client,
		eventRecorder: record,
	}
}

func (h *enqueueRequestForNodePoolEvent) Create(e event.CreateEvent, queue workqueue.RateLimitingInterface) {
	nodePool, ok := e.Object.(*nodepoolv1beta1.NodePool)
	if ok {
		enqueueELBServices(queue, h.client, Key(nodePool), "nodepool")
	}
}

func (h *enqueueRequestForNodePoolEvent) Update(e event.UpdateEvent, queue workqueue.RateLimitingInterface) {
	oldNodePool, ok1 := e.ObjectOld.(*nodepoolv1beta1.NodePool)
	newNodePool, ok2 := e.ObjectNew.(*nodepoolv1beta1.NodePool)

	if ok1 && ok2 {
		if labels.Equals(oldNodePool.Labels, newNodePool.Labels) {
			enqueueELBServices(queue, h.client, Key(newNodePool), "nodepool")
		}
	}
}

func (h *enqueueRequestForNodePoolEvent) Delete(e event.DeleteEvent, queue workqueue.RateLimitingInterface) {
	nodePool, ok := e.Object.(*nodepoolv1beta1.NodePool)
	if ok {
		enqueueELBServices(queue, h.client, Key(nodePool), "nodepool")
	}
}

func (h *enqueueRequestForNodePoolEvent) Generic(e event.GenericEvent, queue workqueue.RateLimitingInterface) {

}

func IsENSNode(node *v1.Node) bool {
	if id, isENS := node.Labels[EnsNodeId]; isENS && id != "" {
		return true
	}
	return false
}

func IsENSNodePool(np *nodepoolv1beta1.NodePool) bool {
	return np.Labels[EnsNetworkId] != "" && np.Labels[EnsRegionId] != "" && np.Annotations[EnsVSwitchId] != ""
}

func nodeChanged(oldNode, newNode *v1.Node) bool {

	if !labels.Equals(oldNode.Labels, newNode.Labels) {
		return true
	}
	if oldNode.Spec.Unschedulable != newNode.Spec.Unschedulable {
		klog.Info(fmt.Sprintf("node changed: %s, spec from=%t, to=%t", oldNode.Name, oldNode.Spec.Unschedulable, newNode.Spec.Unschedulable), "node", oldNode.Name)
		return true
	}
	if nodeConditionChanged(oldNode.Name, oldNode.Status.Conditions, newNode.Status.Conditions) {
		return true
	}
	return false
}

func nodeConditionChanged(name string, oldCondition, newCondition []v1.NodeCondition) bool {
	if len(oldCondition) != len(newCondition) {
		klog.Info(fmt.Sprintf("node changed:  condition length not equal, from=%v, to=%v", oldCondition, newCondition),
			"node", name)
		return true
	}

	sort.SliceStable(oldCondition, func(i, j int) bool {
		return strings.Compare(string(oldCondition[i].Type), string(oldCondition[j].Type)) <= 0
	})

	sort.SliceStable(newCondition, func(i, j int) bool {
		return strings.Compare(string(newCondition[i].Type), string(newCondition[j].Type)) <= 0
	})

	for i := range oldCondition {
		if oldCondition[i].Type != newCondition[i].Type ||
			oldCondition[i].Status != newCondition[i].Status {
			klog.Info(fmt.Sprintf("node changed: condition type(%s,%s) | status(%s,%s)",
				oldCondition[i].Type, newCondition[i].Type, oldCondition[i].Status, newCondition[i].Status), "node", name)
			return true
		}
	}
	return false
}

func needAdd(service *v1.Service) bool {
	if isELBService(service) {
		klog.Infof("service %s is need elb load balancer", Key(service))
		return true
	}
	if HasFinalizer(service, ELBFinalizer) {
		klog.Info("service has service finalizer, which may was LoadBalancer", "service", Key(service))
		return true
	}
	return false
}

func isELBService(service *v1.Service) bool {
	return service.Spec.Type == v1.ServiceTypeLoadBalancer && service.Spec.LoadBalancerClass != nil && *service.Spec.LoadBalancerClass == ELBClass
}

func needUpdate(oldSvc, newSvc *v1.Service) bool {
	if !isELBService(oldSvc) && !isELBService(newSvc) {
		return false
	}

	if isELBService(oldSvc) != isELBService(newSvc) {
		klog.Info(fmt.Sprintf("TypeChanged %v - %v", oldSvc.Spec.Type, newSvc.Spec.Type), "service", Key(oldSvc))
		return true
	}

	if !reflect.DeepEqual(oldSvc.Annotations, newSvc.Annotations) {
		klog.Info(fmt.Sprintf("AnnotationChanged: %v - %v", oldSvc.Annotations, newSvc.Annotations), "service", Key(oldSvc))
		return true
	}

	if !reflect.DeepEqual(oldSvc.Spec, newSvc.Spec) {
		klog.Info(fmt.Sprintf("SpecChanged: %v - %v", oldSvc.Spec, newSvc.Spec), "service", Key(oldSvc))
		return true
	}

	if oldSvc.DeletionTimestamp.IsZero() != newSvc.DeletionTimestamp.IsZero() {
		klog.Info(fmt.Sprintf("DeleteTimestampChanged: %v - %v", oldSvc.DeletionTimestamp.IsZero(), newSvc.DeletionTimestamp.IsZero()), "service", Key(oldSvc))
		return true
	}

	return false
}

func validateAnnotation(svc *v1.Service) error {
	if !isELBService(svc) {
		return nil
	}

	if !svc.DeletionTimestamp.IsZero() {
		return nil
	}

	if svc.Annotations == nil {
		svc.Annotations = make(map[string]string, 0)
	}

	if svc.Annotations[Annotation(NodePoolSelector)] == "" {
		fldPath := field.NewPath("meta").Child("annotations")
		return field.Required(fldPath, Annotation(NodePoolSelector))
	}

	if svc.Annotations[Annotation(NodePoolSelector)] != "" {
		nodeSelectorKey := Annotation(NodePoolSelector)
		nodeSelectorVal := svc.Annotations[nodeSelectorKey]
		if !validateLabels(nodeSelectorVal) {
			fldPath := field.NewPath("meta").Child("annotations").Key(nodeSelectorKey)
			return field.Invalid(fldPath, nodeSelectorVal, fmt.Sprintf("correct format, e.g. key1=val1,key2=val2,..."))
		}
	}

	if svc.Annotations[Annotation(LoadBalancerId)] != "" {
		loadBalancerKey := Annotation(LoadBalancerId)
		loadBalancerVal := svc.Annotations[loadBalancerKey]
		if !validateLabels(svc.Annotations[Annotation(LoadBalancerId)]) {
			fldPath := field.NewPath("meta").Child("annotations").Key(loadBalancerKey)
			return field.Invalid(fldPath, loadBalancerVal, fmt.Sprintf("correct format, e.g. key1=val1,key2=val2,..."))
		}
		if svc.Spec.ExternalTrafficPolicy != v1.ServiceExternalTrafficPolicyTypeLocal {
			if b, err := strconv.ParseBool(svc.Annotations[Annotation(ListenerOverride)]); err == nil && b {
				fldPath := field.NewPath("meta").Child("annotations").Key(Annotation(ListenerOverride))
				return field.Invalid(fldPath, b, fmt.Sprintf("the spec.externalTrafficPolicy must be specified %s if elb is reused", v1.ServiceExternalTrafficPolicyTypeLocal))
			}
			if b, err := strconv.ParseBool(svc.Annotations[Annotation(BackendOverride)]); err == nil && b {
				fldPath := field.NewPath("meta").Child("annotations").Key(Annotation(BackendOverride))
				return field.Invalid(fldPath, b, fmt.Sprintf("the spec.externalTrafficPolicy must be specified %s if elb is reused", v1.ServiceExternalTrafficPolicyTypeLocal))
			}
		}
	}

	if svc.Annotations[Annotation(BackendLabel)] != "" {
		backendKey := Annotation(BackendLabel)
		backendVal := svc.Annotations[backendKey]
		if !validateLabels(backendVal) {
			fldPath := field.NewPath("meta").Child("annotations").Key(backendKey)
			return field.Invalid(fldPath, backendVal, fmt.Sprintf("correct format, e.g. key1=val1,key2=val2,..."))
		}
	}
	return nil
}

func validateLabels(s string) bool {
	return regexp.MustCompile("^([\\w.-]+)=([\\w.-]+)(,([\\w.-]+)=([\\w.-]+))*$").MatchString(s)
}
