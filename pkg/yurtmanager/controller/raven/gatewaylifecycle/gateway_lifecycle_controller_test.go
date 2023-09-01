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
	"fmt"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/openyurtio/openyurt/pkg/apis"
	nodepoolv1beta1 "github.com/openyurtio/openyurt/pkg/apis/apps/v1beta1"
	ravenv1beta1 "github.com/openyurtio/openyurt/pkg/apis/raven/v1beta1"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/raven/util"
)

const (
	Node1Name         = "node-1"
	Node2Name         = "node-2"
	Node3Name         = "node-3"
	Node4Name         = "node-4"
	Node1Address      = "192.168.0.1"
	Node2Address      = "192.168.0.2"
	Node3Address      = "192.168.0.3"
	Node4Address      = "192.168.0.4"
	NodePoolCloud     = "np-cloud"
	NodePoolEdge      = "np-edge"
	NodePoolDedicated = "np-dedicated"
	NodePoolLabel     = "alibabacloud.com/nodepool-id"
	EdgeNode          = "alibabacloud.com/is-edge-worker"
)

func mockReconcile() *ReconcileResource {
	nodeList := &corev1.NodeList{
		Items: []corev1.Node{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: Node1Name,
					Labels: map[string]string{
						NodePoolLabel: NodePoolCloud,
						EdgeNode:      "false",
					},
				},
				Status: corev1.NodeStatus{
					Addresses: []corev1.NodeAddress{
						{
							Type:    corev1.NodeInternalIP,
							Address: Node1Address,
						},
					},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: Node2Name,
					Labels: map[string]string{
						NodePoolLabel: NodePoolCloud,
						EdgeNode:      "false",
					},
				},
				Status: corev1.NodeStatus{
					Addresses: []corev1.NodeAddress{
						{
							Type:    corev1.NodeInternalIP,
							Address: Node2Address,
						},
					},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: Node3Name,
					Labels: map[string]string{
						NodePoolLabel: NodePoolDedicated,
						EdgeNode:      "true",
					},
				},
				Status: corev1.NodeStatus{
					Addresses: []corev1.NodeAddress{
						{
							Type:    corev1.NodeInternalIP,
							Address: Node3Address,
						},
					},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: Node4Name,
					Labels: map[string]string{
						NodePoolLabel: NodePoolEdge,
						EdgeNode:      "true",
					},
				},
				Status: corev1.NodeStatus{
					Addresses: []corev1.NodeAddress{
						{
							Type:    corev1.NodeInternalIP,
							Address: Node4Address,
						},
					},
				},
			},
		},
	}
	configmaps := &corev1.ConfigMapList{
		Items: []corev1.ConfigMap{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      util.RavenGlobalConfig,
					Namespace: util.WorkingNamespace,
				},
				Data: map[string]string{
					util.RavenEnableProxy:  "true",
					util.RavenEnableTunnel: "true",
				},
			},
		},
	}
	nodepooList := &nodepoolv1beta1.NodePoolList{
		Items: []nodepoolv1beta1.NodePool{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: NodePoolCloud,
					Annotations: map[string]string{
						PoolNodesConnectedAnnotationKey: "true",
					},
				},
				Spec: nodepoolv1beta1.NodePoolSpec{
					Type: nodepoolv1beta1.Cloud,
				},
				Status: nodepoolv1beta1.NodePoolStatus{
					Nodes: []string{Node1Name, Node2Name},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: NodePoolDedicated,
					Annotations: map[string]string{
						InterconnectionModeAnnotationKey: "private",
						PoolNodesConnectedAnnotationKey:  "true",
					},
				},
				Spec: nodepoolv1beta1.NodePoolSpec{
					Type: nodepoolv1beta1.Edge,
				},
				Status: nodepoolv1beta1.NodePoolStatus{
					Nodes: []string{Node3Name},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: NodePoolEdge,
					Annotations: map[string]string{
						InterconnectionModeAnnotationKey: "public",
						PoolNodesConnectedAnnotationKey:  "true",
					},
				},
				Spec: nodepoolv1beta1.NodePoolSpec{
					Type: nodepoolv1beta1.Edge,
				},
				Status: nodepoolv1beta1.NodePoolStatus{
					Nodes: []string{Node4Name},
				},
			},
		},
	}
	objs := []runtime.Object{nodeList, nodepooList, configmaps}
	scheme := runtime.NewScheme()
	err := clientgoscheme.AddToScheme(scheme)
	if err != nil {
		return nil
	}
	err = apis.AddToScheme(scheme)
	if err != nil {
		return nil
	}
	return &ReconcileResource{
		Client:   fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objs...).Build(),
		recorder: record.NewFakeRecorder(100),
		option:   util.NewOption(),
	}
}

func TestReconcileResource_Reconcile(t *testing.T) {
	expect := map[string]ravenv1beta1.Gateway{
		"gw-cloud": {ObjectMeta: metav1.ObjectMeta{Name: "gw-cloud"}},
		fmt.Sprintf("%s%s", GatewayPrefix, NodePoolEdge): {ObjectMeta: metav1.ObjectMeta{Name: "gw-np-edge"}},
	}
	r := mockReconcile()
	var npList nodepoolv1beta1.NodePoolList
	_ = r.Client.List(context.TODO(), &npList)
	for _, np := range npList.Items {
		_, err := r.Reconcile(context.TODO(), reconcile.Request{NamespacedName: types.NamespacedName{Name: np.GetName()}})
		if err != nil {
			t.Errorf("failed to reconcile, error %s", err.Error())
		}
	}
	var gwList ravenv1beta1.GatewayList
	_ = r.Client.List(context.TODO(), &gwList)
	for _, gw := range gwList.Items {
		if _, ok := expect[gw.GetName()]; ok {
			continue
		} else {
			t.Errorf("failed to reconcile, error can not generate gatewat %s", gw.GetName())
		}
	}
}
