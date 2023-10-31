package cloudnodepoollifecycle

import (
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type EnqueueRequestNode struct {
}

func (e *EnqueueRequestNode) Create(evt event.CreateEvent, q workqueue.RateLimitingInterface) {
	handleNodeObject(evt.Object, q)
}

func (e *EnqueueRequestNode) Update(evt event.UpdateEvent, q workqueue.RateLimitingInterface) {
	handleNodeObject(evt.ObjectNew, q)
}

func (e *EnqueueRequestNode) Delete(evt event.DeleteEvent, q workqueue.RateLimitingInterface) {
	handleNodeObject(evt.Object, q)
}

func (e *EnqueueRequestNode) Generic(evt event.GenericEvent, q workqueue.RateLimitingInterface) {
	handleNodeObject(evt.Object, q)
}

func handleNodeObject(obj client.Object, q workqueue.RateLimitingInterface) {
	no, ok := obj.(*v1.Node)
	if !ok {
		return
	}

	if no.Spec.ProviderID == "" {
		return
	}

	if no.Labels == nil {
		return
	}

	if _, ok := no.Labels[cloudNodepoolLabelKey]; !ok {
		return
	}

	q.Add(reconcile.Request{types.NamespacedName{
		Name: no.Labels[cloudNodepoolLabelKey],
	}})
}
