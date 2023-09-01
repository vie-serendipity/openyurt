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

package gatewaylifecycle

import (
	"context"
	"k8s.io/klog/v2"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"

	nodepoolv1beta1 "github.com/openyurtio/openyurt/pkg/apis/apps/v1beta1"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/raven/util"
)

type EnqueueRequestForNodePoolEvent struct{}

func (h *EnqueueRequestForNodePoolEvent) Create(e event.CreateEvent, q workqueue.RateLimitingInterface) {
	np, ok := e.Object.(*nodepoolv1beta1.NodePool)
	if !ok {
		klog.Error(Format("fail to assert runtime Object to v1beta.NodePool"))
		return
	}

	klog.V(2).Infof(Format("enqueue nodepool %s for create event", np.GetName()))
	util.AddNodePoolToWorkQueue(np.GetName(), q)
	return
}

func (h *EnqueueRequestForNodePoolEvent) Update(e event.UpdateEvent, q workqueue.RateLimitingInterface) {
	newNodePool, ok := e.ObjectNew.(*nodepoolv1beta1.NodePool)
	if !ok {
		klog.Error(Format("fail to assert runtime Object to v1beta.NodePool"))
		return
	}
	oldNodePool, ok := e.ObjectOld.(*nodepoolv1beta1.NodePool)
	if !ok {
		klog.Error(Format("fail to assert runtime Object to v1beta.NodePool"))
		return
	}
	if util.HashObject(newNodePool.Status.Nodes) != util.HashObject(oldNodePool.Status.Nodes) {
		klog.V(2).Infof(Format("enqueue nodepool %s for update event", newNodePool.GetName()))
		util.AddNodePoolToWorkQueue(newNodePool.GetName(), q)
		return
	}

	newConnected := strings.ToLower(newNodePool.Annotations[PoolNodesConnectedAnnotationKey])
	oldConnected := strings.ToLower(oldNodePool.Annotations[PoolNodesConnectedAnnotationKey])

	if oldConnected != newConnected {
		klog.V(2).Infof(Format("enqueue nodepool %s for update event", newNodePool.GetName()))
		util.AddNodePoolToWorkQueue(newNodePool.GetName(), q)
		return
	}

	newInterConnMode := strings.ToLower(newNodePool.Annotations[InterconnectionModeAnnotationKey])
	oldInterConnMode := strings.ToLower(oldNodePool.Annotations[InterconnectionModeAnnotationKey])
	if oldInterConnMode != newInterConnMode {
		klog.V(2).Infof(Format("enqueue nodepool %s for update event", newNodePool.GetName()))
		util.AddNodePoolToWorkQueue(newNodePool.GetName(), q)
		return
	}
	return
}

func (h *EnqueueRequestForNodePoolEvent) Delete(e event.DeleteEvent, q workqueue.RateLimitingInterface) {
	np, ok := e.Object.(*nodepoolv1beta1.NodePool)
	if !ok {
		klog.Error(Format("fail to assert runtime Object to v1beta.NodePool"))
		return
	}

	klog.V(2).Infof(Format("enqueue nodepool %s for delete event", np.GetName()))
	util.AddNodePoolToWorkQueue(np.GetName(), q)
	return
}

func (h *EnqueueRequestForNodePoolEvent) Generic(e event.GenericEvent, q workqueue.RateLimitingInterface) {
	return
}

type EnqueueRequestForRavenConfigEvent struct {
	client client.Client
}

func (h *EnqueueRequestForRavenConfigEvent) Create(e event.CreateEvent, q workqueue.RateLimitingInterface) {
	cm, ok := e.Object.(*corev1.ConfigMap)
	if !ok {
		klog.Error(Format("fail to assert runtime Object to v1alpha1.Gateway"))
		return
	}
	var nodePoolList nodepoolv1beta1.NodePoolList
	err := h.client.List(context.TODO(), &nodePoolList)
	if err != nil {
		klog.Error(Format("fail to list all nodepool for configmap %s/%s create event",
			cm.GetNamespace(), cm.GetName()))
		return
	}

	for _, np := range nodePoolList.Items {
		klog.V(2).Infof(Format("enqueue nodepools %s for onfigmap %s/%s create event",
			np.GetName(), cm.GetNamespace(), cm.GetName()))
		util.AddNodePoolToWorkQueue(np.GetName(), q)
	}
	return
}

func (h *EnqueueRequestForRavenConfigEvent) Update(e event.UpdateEvent, q workqueue.RateLimitingInterface) {
	newCm, ok := e.ObjectNew.(*corev1.ConfigMap)
	if !ok {
		klog.Error(Format("fail to assert runtime Object to v1alpha1.Gateway"))
		return
	}
	oldCm, ok := e.ObjectOld.(*corev1.ConfigMap)
	if !ok {
		klog.Error(Format("fail to assert runtime Object to v1alpha1.Gateway"))
		return
	}
	if newCm.Data == nil || oldCm.Data == nil {
		klog.Error(Format("assert configmap.Data is nil"))
		return
	}
	if util.HashObject(newCm.Data) != util.HashObject(oldCm.Data) {
		var nodePoolList nodepoolv1beta1.NodePoolList
		err := h.client.List(context.TODO(), &nodePoolList)
		if err != nil {
			klog.Error(Format("fail to list all nodepool for configmap %s/%s update event",
				newCm.GetNamespace(), newCm.GetName()))
			return
		}

		for _, np := range nodePoolList.Items {
			klog.V(2).Infof(Format("enqueue nodepools %s for onfigmap %s/%s update event",
				np.GetName(), newCm.GetNamespace(), newCm.GetName()))
			util.AddNodePoolToWorkQueue(np.GetName(), q)
		}
	}
	return
}

func (h *EnqueueRequestForRavenConfigEvent) Delete(e event.DeleteEvent, q workqueue.RateLimitingInterface) {
	cm, ok := e.Object.(*corev1.ConfigMap)
	if !ok {
		klog.Error(Format("fail to assert runtime Object to v1alpha1.Gateway"))
		return
	}
	var nodePoolList nodepoolv1beta1.NodePoolList
	err := h.client.List(context.TODO(), &nodePoolList)
	if err != nil {
		klog.Error(Format("fail to list all nodepool for configmap %s/%s delete event",
			cm.GetNamespace(), cm.GetName()))
		return
	}

	for _, np := range nodePoolList.Items {
		klog.V(2).Infof(Format("enqueue nodepools %s for onfigmap %s/%s delete event",
			np.GetName(), cm.GetNamespace(), cm.GetName()))
		util.AddNodePoolToWorkQueue(np.GetName(), q)
	}
	return
}

func (h *EnqueueRequestForRavenConfigEvent) Generic(e event.GenericEvent, q workqueue.RateLimitingInterface) {
	return
}
