/*
Copyright 2016 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	jsonpatch "github.com/evanphx/json-patch"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/klog/v2"

	"github.com/openyurtio/yurt-app-manager-api/pkg/yurtappmanager/apis/apps/v1alpha1"
	"github.com/openyurtio/yurt-app-manager-api/pkg/yurtappmanager/client/clientset/versioned/fake"
	appsv1alpha1 "github.com/openyurtio/yurt-app-manager-api/pkg/yurtappmanager/client/clientset/versioned/typed/apps/v1alpha1"
)

// FakeNodePoolHandler is a fake implementation of NodePoolsInterface and NodePoolInterface. It
// allows test cases to have fine-grained control over mock behaviors. We also need
// PodsInterface and PodInterface to test list & delete pods, which is implemented in
// the embedded client.Fake field.
type FakeNodePoolHandler struct {
	*fake.Clientset

	// Input: Hooks determine if request is valid or not
	CreateHook func(*FakeNodePoolHandler, *v1alpha1.NodePool) bool
	Existing   []*v1alpha1.NodePool

	// Output
	CreatedNodePools        []*v1alpha1.NodePool
	DeletedNodePools        []*v1alpha1.NodePool
	UpdatedNodePools        []*v1alpha1.NodePool
	UpdatedNodePoolStatuses []*v1alpha1.NodePool
	RequestCount            int

	// Synchronization
	lock           sync.Mutex
	DeleteWaitChan chan struct{}
	PatchWaitChan  chan struct{}
}

// FakeLegacyHandler is a fake implementation of CoreV1Interface.
type FakeLegacyHandler struct {
	appsv1alpha1.AppsV1alpha1Interface
	n *FakeNodePoolHandler
}

// GetUpdatedNodePoolsCopy returns a slice of NodePools with updates applied.
func (m *FakeNodePoolHandler) GetUpdatedNodePoolsCopy() []*v1alpha1.NodePool {
	m.lock.Lock()
	defer m.lock.Unlock()
	updatedNodePoolsCopy := make([]*v1alpha1.NodePool, len(m.UpdatedNodePools), len(m.UpdatedNodePools))
	for i, ptr := range m.UpdatedNodePools {
		updatedNodePoolsCopy[i] = ptr
	}
	return updatedNodePoolsCopy
}

// AppsV1alpha1 returns fake AppsV1alpha1Interface.
func (m *FakeNodePoolHandler) AppsV1alpha1() appsv1alpha1.AppsV1alpha1Interface {
	return &FakeLegacyHandler{m.Clientset.AppsV1alpha1(), m}
}

// NodePools return fake NodePoolInterfaces.
func (m *FakeLegacyHandler) NodePools() appsv1alpha1.NodePoolInterface {
	return m.n
}

// Create adds a new NodePool to the fake store.
func (m *FakeNodePoolHandler) Create(ctx context.Context, nodepool *v1alpha1.NodePool, opts metav1.CreateOptions) (*v1alpha1.NodePool, error) {
	m.lock.Lock()
	defer func() {
		m.RequestCount++
		m.lock.Unlock()
	}()
	for _, n := range m.Existing {
		if n.Name == nodepool.Name {
			return nil, apierrors.NewAlreadyExists(v1alpha1.Resource("nodepools"), nodepool.Name)
		}
	}
	if m.CreateHook == nil || m.CreateHook(m, nodepool) {
		nodepoolCopy := *nodepool
		m.CreatedNodePools = append(m.CreatedNodePools, &nodepoolCopy)
		return nodepool, nil
	}
	return nil, errors.New("create error")
}

// Get returns a NodePool from the fake store.
func (m *FakeNodePoolHandler) Get(ctx context.Context, name string, opts metav1.GetOptions) (*v1alpha1.NodePool, error) {
	m.lock.Lock()
	defer func() {
		m.RequestCount++
		m.lock.Unlock()
	}()
	for i := range m.UpdatedNodePools {
		if m.UpdatedNodePools[i].Name == name {
			nodepoolCopy := *m.UpdatedNodePools[i]
			return &nodepoolCopy, nil
		}
	}
	for i := range m.Existing {
		if m.Existing[i].Name == name {
			nodepoolCopy := *m.Existing[i]
			return &nodepoolCopy, nil
		}
	}
	return nil, nil
}

// List returns a list of NodePools from the fake store.
func (m *FakeNodePoolHandler) List(ctx context.Context, opts metav1.ListOptions) (*v1alpha1.NodePoolList, error) {
	m.lock.Lock()
	defer func() {
		m.RequestCount++
		m.lock.Unlock()
	}()
	var nodes []*v1alpha1.NodePool
	for i := 0; i < len(m.UpdatedNodePools); i++ {
		if !contains(m.UpdatedNodePools[i], m.DeletedNodePools) {
			nodes = append(nodes, m.UpdatedNodePools[i])
		}
	}
	for i := 0; i < len(m.Existing); i++ {
		if !contains(m.Existing[i], m.DeletedNodePools) && !contains(m.Existing[i], nodes) {
			nodes = append(nodes, m.Existing[i])
		}
	}
	for i := 0; i < len(m.CreatedNodePools); i++ {
		if !contains(m.CreatedNodePools[i], m.DeletedNodePools) && !contains(m.CreatedNodePools[i], nodes) {
			nodes = append(nodes, m.CreatedNodePools[i])
		}
	}
	nodeList := &v1alpha1.NodePoolList{}
	for _, nodepool := range nodes {
		nodeList.Items = append(nodeList.Items, *nodepool)
	}
	return nodeList, nil
}

// Delete deletes a NodePool from the fake store.
func (m *FakeNodePoolHandler) Delete(ctx context.Context, id string, opt metav1.DeleteOptions) error {
	m.lock.Lock()
	defer func() {
		m.RequestCount++
		if m.DeleteWaitChan != nil {
			m.DeleteWaitChan <- struct{}{}
		}
		m.lock.Unlock()
	}()
	m.DeletedNodePools = append(m.DeletedNodePools, NewNodePool(id))
	return nil
}

// DeleteCollection deletes a collection of NodePools from the fake store.
func (m *FakeNodePoolHandler) DeleteCollection(ctx context.Context, opt metav1.DeleteOptions, listOpts metav1.ListOptions) error {
	return nil
}

// Update updates a NodePool in the fake store.
func (m *FakeNodePoolHandler) Update(ctx context.Context, nodepool *v1alpha1.NodePool, opts metav1.UpdateOptions) (*v1alpha1.NodePool, error) {
	m.lock.Lock()
	defer func() {
		m.RequestCount++
		m.lock.Unlock()
	}()

	nodepoolCopy := *nodepool
	for i, updateNodePool := range m.UpdatedNodePools {
		if updateNodePool.Name == nodepoolCopy.Name {
			m.UpdatedNodePools[i] = &nodepoolCopy
			return nodepool, nil
		}
	}
	m.UpdatedNodePools = append(m.UpdatedNodePools, &nodepoolCopy)
	return nodepool, nil
}

// UpdateStatus updates a status of a NodePool in the fake store.
func (m *FakeNodePoolHandler) UpdateStatus(ctx context.Context, nodepool *v1alpha1.NodePool, opts metav1.UpdateOptions) (*v1alpha1.NodePool, error) {
	m.lock.Lock()
	defer func() {
		m.RequestCount++
		m.lock.Unlock()
	}()

	var origNodePoolCopy v1alpha1.NodePool
	found := false
	for i := range m.Existing {
		if m.Existing[i].Name == nodepool.Name {
			origNodePoolCopy = *m.Existing[i]
			found = true
			break
		}
	}
	updatedNodePoolIndex := -1
	for i := range m.UpdatedNodePools {
		if m.UpdatedNodePools[i].Name == nodepool.Name {
			origNodePoolCopy = *m.UpdatedNodePools[i]
			updatedNodePoolIndex = i
			found = true
			break
		}
	}

	if !found {
		return nil, fmt.Errorf("not found nodepool %v", nodepool)
	}

	origNodePoolCopy.Status = nodepool.Status
	if updatedNodePoolIndex < 0 {
		m.UpdatedNodePools = append(m.UpdatedNodePools, &origNodePoolCopy)
	} else {
		m.UpdatedNodePools[updatedNodePoolIndex] = &origNodePoolCopy
	}

	nodepoolCopy := *nodepool
	m.UpdatedNodePoolStatuses = append(m.UpdatedNodePoolStatuses, &nodepoolCopy)
	return nodepool, nil
}

// PatchStatus patches a status of a NodePool in the fake store.
func (m *FakeNodePoolHandler) PatchStatus(nodepoolName string, data []byte) (*v1alpha1.NodePool, error) {
	m.RequestCount++
	return m.Patch(context.TODO(), nodepoolName, types.StrategicMergePatchType, data, metav1.PatchOptions{}, "status")
}

// Watch watches NodePools in a fake store.
func (m *FakeNodePoolHandler) Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	return watch.NewFake(), nil
}

// Patch patches a NodePool in the fake store.
func (m *FakeNodePoolHandler) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (*v1alpha1.NodePool, error) {
	m.lock.Lock()
	defer func() {
		m.RequestCount++
		if m.PatchWaitChan != nil {
			m.PatchWaitChan <- struct{}{}
		}
		m.lock.Unlock()
	}()
	var nodepoolCopy v1alpha1.NodePool
	for i := range m.Existing {
		if m.Existing[i].Name == name {
			nodepoolCopy = *m.Existing[i]
		}
	}
	updatedNodePoolIndex := -1
	for i := range m.UpdatedNodePools {
		if m.UpdatedNodePools[i].Name == name {
			nodepoolCopy = *m.UpdatedNodePools[i]
			updatedNodePoolIndex = i
		}
	}

	originalObjJS, err := json.Marshal(nodepoolCopy)
	if err != nil {
		klog.Errorf("Failed to marshal %v", nodepoolCopy)
		return nil, nil
	}
	var originalNodePool v1alpha1.NodePool
	if err = json.Unmarshal(originalObjJS, &originalNodePool); err != nil {
		klog.Errorf("Failed to unmarshal original object: %v", err)
		return nil, nil
	}

	var patchedObjJS []byte
	switch pt {
	case types.JSONPatchType:
		patchObj, err := jsonpatch.DecodePatch(data)
		if err != nil {
			klog.Error(err.Error())
			return nil, nil
		}
		if patchedObjJS, err = patchObj.Apply(originalObjJS); err != nil {
			klog.Error(err.Error())
			return nil, nil
		}
	case types.MergePatchType:
		if patchedObjJS, err = jsonpatch.MergePatch(originalObjJS, data); err != nil {
			klog.Error(err.Error())
			return nil, nil
		}
	case types.StrategicMergePatchType:
		if patchedObjJS, err = strategicpatch.StrategicMergePatch(originalObjJS, data, originalNodePool); err != nil {
			klog.Error(err.Error())
			return nil, nil
		}
	default:
		klog.Errorf("unknown Content-Type header for patch: %v", pt)
		return nil, nil
	}

	var updatedNodePool v1alpha1.NodePool
	if err = json.Unmarshal(patchedObjJS, &updatedNodePool); err != nil {
		klog.Errorf("Failed to unmarshal patched object: %v", err)
		return nil, nil
	}

	if updatedNodePoolIndex < 0 {
		m.UpdatedNodePools = append(m.UpdatedNodePools, &updatedNodePool)
	} else {
		m.UpdatedNodePools[updatedNodePoolIndex] = &updatedNodePool
	}

	return &updatedNodePool, nil
}

// NewNodePool is a helper function for creating NodePools for testing.
func NewNodePool(name string) *v1alpha1.NodePool {
	return &v1alpha1.NodePool{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Status:     v1alpha1.NodePoolStatus{},
	}
}

func contains(nodepool *v1alpha1.NodePool, nodes []*v1alpha1.NodePool) bool {
	for i := 0; i < len(nodes); i++ {
		if nodepool.Name == nodes[i].Name {
			return true
		}
	}
	return false
}
