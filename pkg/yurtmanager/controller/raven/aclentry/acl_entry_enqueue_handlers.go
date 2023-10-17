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

package aclentry

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/raven/util"
)

type EnqueueRequestForRavenCfgEvent struct {
}

func (h *EnqueueRequestForRavenCfgEvent) Create(e event.CreateEvent, q workqueue.RateLimitingInterface) {
	cm, ok := e.Object.(*corev1.ConfigMap)
	if !ok {
		klog.Error(Format("fail to assert runtime Object to v1.Configmap"))
		return
	}
	if cm.Data[util.ACLEntry] != "" {
		AddRavenConfigToWorkQueue(q)
	}
	return
}

func (h *EnqueueRequestForRavenCfgEvent) Update(e event.UpdateEvent, q workqueue.RateLimitingInterface) {
	newCm, ok := e.ObjectNew.(*corev1.ConfigMap)
	if !ok {
		klog.Error(Format("fail to assert runtime Object to v1.Configmap"))
		return
	}
	oldCm, ok := e.ObjectOld.(*corev1.ConfigMap)
	if !ok {
		klog.Error(Format("fail to assert runtime Object to v1.Configmap"))
		return
	}
	if newCm.Data == nil || oldCm.Data == nil {
		klog.Error(Format("assert Configmap.Data is nil"))
		return
	}
	if newCm.Data[util.ACLId] != oldCm.Data[util.ACLId] {
		AddRavenConfigToWorkQueue(q)
		return
	}
	if newCm.Data[util.ACLEntry] != oldCm.Data[util.ACLEntry] {
		AddRavenConfigToWorkQueue(q)
		return
	}
	return
}

func (h *EnqueueRequestForRavenCfgEvent) Delete(e event.DeleteEvent, q workqueue.RateLimitingInterface) {
	return
}

func (h *EnqueueRequestForRavenCfgEvent) Generic(e event.GenericEvent, q workqueue.RateLimitingInterface) {
	return
}

func AddRavenConfigToWorkQueue(q workqueue.RateLimitingInterface) {
	q.Add(reconcile.Request{
		NamespacedName: types.NamespacedName{Namespace: util.WorkingNamespace, Name: util.RavenGlobalConfig},
	})
}
