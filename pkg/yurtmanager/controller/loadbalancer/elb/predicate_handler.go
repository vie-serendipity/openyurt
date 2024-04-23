package elb

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	v1 "k8s.io/api/core/v1"
	discovery "k8s.io/api/discovery/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	kubeconsts "k8s.io/kubernetes/cmd/kubeadm/app/constants"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	networkv1alpha1 "github.com/openyurtio/openyurt/pkg/apis/network/v1alpha1"
)

var _ predicate.Predicate = (*PredicationForPoolServiceEvent)(nil)

type PredicationForPoolServiceEvent struct{}

func NewPredictionForPoolServiceEvent() *PredicationForPoolServiceEvent {
	return &PredicationForPoolServiceEvent{}
}

func (p *PredicationForPoolServiceEvent) Create(evt event.CreateEvent) bool {
	ps, ok := evt.Object.(*networkv1alpha1.PoolService)
	if ok && IsELBPoolService(ps) {
		klog.Info("Create Event: ", "pool service ", Key(ps), fmt.Sprintf("LoadBalancerClass: %v", *ps.Spec.LoadBalancerClass))
		return true
	}
	return false
}

func (p *PredicationForPoolServiceEvent) Update(evt event.UpdateEvent) bool {
	newPs, ok1 := evt.ObjectNew.(*networkv1alpha1.PoolService)
	oldPs, ok2 := evt.ObjectOld.(*networkv1alpha1.PoolService)
	if ok1 && ok2 && IsELBPoolService(newPs) {
		if oldPs.UID != newPs.UID {
			klog.Info("Update Event: ", "pool service ", Key(newPs), fmt.Sprintf("UIDChanged: %v - %v", oldPs.UID, newPs.UID))
			return true
		}
		if !reflect.DeepEqual(newPs.GetDeletionTimestamp().IsZero(), oldPs.GetDeletionTimestamp().IsZero()) {
			klog.Info("Update Event: ", "pool service ", Key(newPs), fmt.Sprintf("DeleteTimestampChanged: %v - %v", oldPs.DeletionTimestamp.IsZero(), newPs.DeletionTimestamp.IsZero()))
			return true
		}
		if !reflect.DeepEqual(ConcernedAnnotation(newPs.Annotations), ConcernedAnnotation(oldPs.Annotations)) {
			klog.Info("Update Event: ", "pool service ", Key(newPs), fmt.Sprintf("AnnotationChanged: %v - %v", oldPs.Annotations, newPs.Annotations))
			return true
		}
	}
	return false
}

func (p *PredicationForPoolServiceEvent) Delete(evt event.DeleteEvent) bool { return false }

func (p *PredicationForPoolServiceEvent) Generic(evt event.GenericEvent) bool {
	return false
}

var _ predicate.Predicate = (*PredicationForServiceEvent)(nil)

type PredicationForServiceEvent struct{}

func NewPredictionForServiceEvent() *PredicationForServiceEvent {
	return &PredicationForServiceEvent{}
}

func (p *PredicationForServiceEvent) Create(evt event.CreateEvent) bool { return false }

func (p *PredicationForServiceEvent) Update(evt event.UpdateEvent) bool {
	newSvc, ok1 := evt.ObjectNew.(*v1.Service)
	oldSvc, ok2 := evt.ObjectOld.(*v1.Service)
	if ok1 && ok2 && IsELBService(oldSvc) && IsELBService(newSvc) {
		if !reflect.DeepEqual(ConcernedAnnotation(oldSvc.Annotations), ConcernedAnnotation(newSvc.Annotations)) {
			klog.Info("Update Event: ", "service ", Key(newSvc), fmt.Sprintf("AnnotationChanged: %v - %v", oldSvc.Annotations, newSvc.Annotations))
			return true
		}
		if !reflect.DeepEqual(oldSvc.Spec.Ports, newSvc.Spec.Ports) {
			klog.Info("Update Event: ", "service ", Key(newSvc), fmt.Sprintf("SpecPortsChanged: %v - %v", oldSvc.Spec, newSvc.Spec))
			return true
		}
	}
	return false
}

func (p *PredicationForServiceEvent) Delete(evt event.DeleteEvent) bool {
	return false
}

func (p *PredicationForServiceEvent) Generic(evt event.GenericEvent) bool {
	return false
}

func ConcernedAnnotation(anno map[string]string) map[string]string {
	concernedAnnotation := make(map[string]string)
	if anno == nil {
		return concernedAnnotation
	}
	for k, v := range anno {
		if strings.HasPrefix(k, AnnotationPrefix) {
			concernedAnnotation[k] = v
		}
	}
	return concernedAnnotation
}

var _ predicate.Predicate = (*PredicationForEndpointSliceEvent)(nil)

type PredicationForEndpointSliceEvent struct {
	client client.Client
}

func NewPredictionForEndpointSliceEvent(client client.Client) *PredicationForEndpointSliceEvent {
	return &PredicationForEndpointSliceEvent{client: client}
}

func (p *PredicationForEndpointSliceEvent) Create(evt event.CreateEvent) bool {
	es, ok := evt.Object.(*discovery.EndpointSlice)
	if ok && IsELBEndpointSlice(es, p.client) {
		klog.Info("Create Event: ", "endpointslice ", Key(es), fmt.Sprintf("Service: %v", es.Labels[discovery.LabelServiceName]))
		return true
	}
	return false
}

func (p *PredicationForEndpointSliceEvent) Update(evt event.UpdateEvent) bool {
	newEs, ok1 := evt.ObjectOld.(*discovery.EndpointSlice)
	oldEs, ok2 := evt.ObjectNew.(*discovery.EndpointSlice)
	if ok1 && ok2 && IsELBEndpointSlice(newEs, p.client) && IsELBEndpointSlice(oldEs, p.client) {
		if !reflect.DeepEqual(oldEs.Endpoints, newEs.Endpoints) {
			klog.Info("Update Event: ", "endpointslice ", Key(newEs), fmt.Sprintf("EndpointsliceChanged: [%v] - [%v]", LogEndpointSlice(oldEs), LogEndpointSlice(newEs)))
			return true
		}
	}
	return false
}

func (p *PredicationForEndpointSliceEvent) Delete(evt event.DeleteEvent) bool {
	es, ok := evt.Object.(*discovery.EndpointSlice)
	if ok && IsELBEndpointSlice(es, p.client) {
		klog.Info("Delete Event: ", "endpointslice ", Key(es), fmt.Sprintf("Service: %v", es.Labels[discovery.LabelServiceName]))
		return true
	}
	return false
}

func (p *PredicationForEndpointSliceEvent) Generic(evt event.GenericEvent) bool {
	return false
}

func IsELBEndpointSlice(es *discovery.EndpointSlice, client client.Client) bool {
	if es == nil {
		return false
	}

	serviceName, ok := es.Labels[discovery.LabelServiceName]
	if !ok {
		return false
	}

	svc := &v1.Service{}
	err := client.Get(context.TODO(), types.NamespacedName{Namespace: es.Namespace, Name: serviceName}, svc)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			klog.Error(err, "fail to get service, skip reconcile endpointslice", "endpointslice", Key(es), "service", serviceName)
		}
		return false
	}

	if !IsELBService(svc) {
		// it is safe not to reconcile endpointslice which belongs to the non-loadbalancer svc
		klog.Info("endpointslice change: loadBalancer is not needed, skip ", "endpointslice ", Key(es))
		return false
	}

	return true
}

var _ predicate.Predicate = (*PredicationForNodeEvent)(nil)

type PredicationForNodeEvent struct{}

func NewPredictionForNodeEvent() *PredicationForNodeEvent {
	return &PredicationForNodeEvent{}
}

func (p *PredicationForNodeEvent) Create(evt event.CreateEvent) bool {
	node, ok := evt.Object.(*v1.Node)
	if ok && !SkipNode(node) {
		klog.Info("Create Event: ", "node ", Key(node), fmt.Sprintf("nodepool: %v", node.Labels[NodePoolLabel]))
		return true
	}
	return false
}

func (p *PredicationForNodeEvent) Update(evt event.UpdateEvent) bool {
	oldNode, ok1 := evt.ObjectOld.(*v1.Node)
	newNode, ok2 := evt.ObjectNew.(*v1.Node)
	if ok1 && ok2 {
		if SkipNode(oldNode) != SkipNode(newNode) {
			klog.Info("Update Event: ", "node ", Key(newNode), fmt.Sprintf("SkipNodeChanged: %v - %v", SkipNode(oldNode), SkipNode(newNode)))
			return true
		}
		if oldNode.Spec.Unschedulable != newNode.Spec.Unschedulable {
			klog.Info("Update Event: ", "node ", Key(newNode), fmt.Sprintf("NodeSchedulableChanged: %v - %v", oldNode.Spec.Unschedulable, newNode.Spec.Unschedulable))
			return true
		}
		oldCondition := NodeCondition(oldNode)
		newCondition := NodeCondition(newNode)
		if oldCondition != newCondition {
			klog.Info("Update Event: ", "node ", Key(newNode), fmt.Sprintf("NodeConditionChanged: %v - %v", oldCondition, newCondition))
			return true
		}
	}
	return false
}

func (p *PredicationForNodeEvent) Delete(evt event.DeleteEvent) bool {
	node, ok := evt.Object.(*v1.Node)
	if ok && !SkipNode(node) {
		klog.Info("Delete Event: ", "node ", Key(node), fmt.Sprintf("nodepool: %v", node.Labels[NodePoolLabel]))
		return true
	}
	return false
}

func (p *PredicationForNodeEvent) Generic(evt event.GenericEvent) bool {
	return false
}

func SkipNode(node *v1.Node) bool {
	if node == nil || node.Labels == nil {
		return true
	}
	if _, ok := node.Labels[EnsNodeId]; !ok {
		return true
	}
	if _, ok := node.Labels[kubeconsts.LabelNodeRoleControlPlane]; ok {
		return true
	}
	if _, ok := node.Labels[kubeconsts.LabelNodeRoleOldControlPlane]; ok {
		return true
	}
	if _, ok := node.Labels[kubeconsts.LabelExcludeFromExternalLB]; ok {
		return true
	}
	if _, ok := node.Labels[v1.LabelNodeExcludeBalancers]; ok {
		return true
	}
	return false
}

func NodeCondition(node *v1.Node) string {
	nodeReadyCondition := v1.ConditionFalse
	if node != nil {
		for i := range node.Status.Conditions {
			if node.Status.Conditions[i].Type == v1.NodeReady {
				nodeReadyCondition = node.Status.Conditions[i].Status
			}
		}
	}
	return string(nodeReadyCondition)
}
