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

package ipam

import (
	"net"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	coreinformers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/openyurtio/openyurt/pkg/controller/kubernetes/controller"
	"github.com/openyurtio/openyurt/pkg/controller/kubernetes/controller/testutil"
	"github.com/openyurtio/openyurt/pkg/controller/nodeipam/ipam/test"
	"github.com/openyurtio/openyurt/pkg/controller/util"
	"github.com/openyurtio/yurt-app-manager-api/pkg/yurtappmanager/apis/apps/v1alpha1"
	appsfake "github.com/openyurtio/yurt-app-manager-api/pkg/yurtappmanager/client/clientset/versioned/fake"
	appsinformers "github.com/openyurtio/yurt-app-manager-api/pkg/yurtappmanager/client/informers/externalversions"
	nodepoolinformers "github.com/openyurtio/yurt-app-manager-api/pkg/yurtappmanager/client/informers/externalversions/apps/v1alpha1"
)

const (
	nodePollInterval = 100 * time.Millisecond
)

var alwaysReady = func() bool { return true }

func waitForUpdatedNodeWithTimeout(nodeHandler *testutil.FakeNodeHandler, number int, timeout time.Duration) error {
	return wait.Poll(nodePollInterval, timeout, func() (bool, error) {
		if len(nodeHandler.GetUpdatedNodesCopy()) >= number {
			return true, nil
		}
		return false, nil
	})
}

func waitForUpdatedNodePoolWithTimeout(nodepoolHandler *test.FakeNodePoolHandler, number int, timeout time.Duration) error {
	return wait.Poll(nodePollInterval, timeout, func() (bool, error) {
		if len(nodepoolHandler.GetUpdatedNodePoolsCopy()) >= number {
			return true, nil
		}
		return false, nil
	})
}

// Creates a fakeNodeInformer using the provided fakeNodeHandler.
func getFakeNodeInformer(fakeNodeHandler *testutil.FakeNodeHandler) coreinformers.NodeInformer {
	fakeClient := &fake.Clientset{}
	fakeInformerFactory := informers.NewSharedInformerFactory(fakeClient, controller.NoResyncPeriodFunc())
	fakeNodeInformer := fakeInformerFactory.Core().V1().Nodes()

	for _, node := range fakeNodeHandler.Existing {
		fakeNodeInformer.Informer().GetStore().Add(node)
	}

	return fakeNodeInformer
}

// Creates a fakeNodePoolInformer using the provided fakeNodePoolHandler.
func getFakeNodePoolInformer(fakeNodePoolHandler *test.FakeNodePoolHandler) nodepoolinformers.NodePoolInformer {
	fakeClient := &appsfake.Clientset{}
	fakeInformerFactory := appsinformers.NewSharedInformerFactory(fakeClient, controller.NoResyncPeriodFunc())
	fakeNodePoolInformer := fakeInformerFactory.Apps().V1alpha1().NodePools()

	for _, node := range fakeNodePoolHandler.Existing {
		fakeNodePoolInformer.Informer().GetStore().Add(node)
	}

	return fakeNodePoolInformer
}

type testCase struct {
	description          string
	fakeNodeHandler      *testutil.FakeNodeHandler
	fakeNodePoolHandler  *test.FakeNodePoolHandler
	clusterCIDRs         []*net.IPNet
	serviceCIDR          *net.IPNet
	secondaryServiceCIDR *net.IPNet
	subNetMaskSize       int
	// key is index of the cidr allocated
	expectedNodeAllocatedCIDR map[int]string
	nodeAllocatedCIDRs        map[int][]string

	expectedNodePoolAllocatedCIDR map[int]string
	nodePoolAllocatedCIDRs        map[int][]string
}

func TestAllocateOrOccupyCIDRSuccess(t *testing.T) {
	// all tests operate on a single node
	testCases := []testCase{
		{
			description: "When there's no ServiceCIDR return first CIDR in range",
			fakeNodeHandler: &testutil.FakeNodeHandler{
				Existing: []*v1.Node{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "node0",
						},
					},
				},
				Clientset: fake.NewSimpleClientset(),
			},
			fakeNodePoolHandler: &test.FakeNodePoolHandler{
				Clientset: appsfake.NewSimpleClientset(),
			},
			clusterCIDRs: func() []*net.IPNet {
				_, clusterCIDR, _ := net.ParseCIDR("127.123.234.0/24")
				return []*net.IPNet{clusterCIDR}
			}(),
			serviceCIDR:          nil,
			secondaryServiceCIDR: nil,
			subNetMaskSize:       30,
			expectedNodeAllocatedCIDR: map[int]string{
				0: "127.123.234.0/30",
			},
		},
		{
			description: "Correctly filter out ServiceCIDR",
			fakeNodeHandler: &testutil.FakeNodeHandler{
				Existing: []*v1.Node{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "node0",
						},
					},
				},
				Clientset: fake.NewSimpleClientset(),
			},
			fakeNodePoolHandler: &test.FakeNodePoolHandler{
				Clientset: appsfake.NewSimpleClientset(),
			},
			clusterCIDRs: func() []*net.IPNet {
				_, clusterCIDR, _ := net.ParseCIDR("127.123.234.0/24")
				return []*net.IPNet{clusterCIDR}
			}(),
			serviceCIDR: func() *net.IPNet {
				_, serviceCIDR, _ := net.ParseCIDR("127.123.234.0/26")
				return serviceCIDR
			}(),
			secondaryServiceCIDR: nil,
			subNetMaskSize:       30,
			// it should return first /30 CIDR after service range
			expectedNodeAllocatedCIDR: map[int]string{
				0: "127.123.234.64/30",
			},
		},
		{
			description: "Correctly ignore already allocated CIDRs",
			fakeNodeHandler: &testutil.FakeNodeHandler{
				Existing: []*v1.Node{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "node0",
						},
					},
				},
				Clientset: fake.NewSimpleClientset(),
			},
			fakeNodePoolHandler: &test.FakeNodePoolHandler{
				Clientset: appsfake.NewSimpleClientset(),
			},
			clusterCIDRs: func() []*net.IPNet {
				_, clusterCIDR, _ := net.ParseCIDR("127.123.234.0/24")
				return []*net.IPNet{clusterCIDR}
			}(),
			serviceCIDR: func() *net.IPNet {
				_, serviceCIDR, _ := net.ParseCIDR("127.123.234.0/26")
				return serviceCIDR
			}(),
			secondaryServiceCIDR: nil,
			subNetMaskSize:       30,
			nodeAllocatedCIDRs: map[int][]string{
				0: {"127.123.234.64/30", "127.123.234.68/30", "127.123.234.72/30", "127.123.234.80/30"},
			},
			expectedNodeAllocatedCIDR: map[int]string{
				0: "127.123.234.76/30",
			},
		},
		{
			description: "Dualstack CIDRs v4,v6",
			fakeNodeHandler: &testutil.FakeNodeHandler{
				Existing: []*v1.Node{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "node0",
						},
					},
				},
				Clientset: fake.NewSimpleClientset(),
			},
			fakeNodePoolHandler: &test.FakeNodePoolHandler{
				Clientset: appsfake.NewSimpleClientset(),
			},
			clusterCIDRs: func() []*net.IPNet {
				_, clusterCIDRv4, _ := net.ParseCIDR("127.123.234.0/8")
				_, clusterCIDRv6, _ := net.ParseCIDR("ace:cab:deca::/8")
				return []*net.IPNet{clusterCIDRv4, clusterCIDRv6}
			}(),
			serviceCIDR: func() *net.IPNet {
				_, serviceCIDR, _ := net.ParseCIDR("127.123.234.0/26")
				return serviceCIDR
			}(),
			secondaryServiceCIDR: nil,
		},
		{
			description: "Dualstack CIDRs v6,v4",
			fakeNodeHandler: &testutil.FakeNodeHandler{
				Existing: []*v1.Node{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "node0",
						},
					},
				},
				Clientset: fake.NewSimpleClientset(),
			},
			fakeNodePoolHandler: &test.FakeNodePoolHandler{
				Clientset: appsfake.NewSimpleClientset(),
			},
			clusterCIDRs: func() []*net.IPNet {
				_, clusterCIDRv4, _ := net.ParseCIDR("127.123.234.0/8")
				_, clusterCIDRv6, _ := net.ParseCIDR("ace:cab:deca::/8")
				return []*net.IPNet{clusterCIDRv6, clusterCIDRv4}
			}(),
			serviceCIDR: func() *net.IPNet {
				_, serviceCIDR, _ := net.ParseCIDR("127.123.234.0/26")
				return serviceCIDR
			}(),
			secondaryServiceCIDR: nil,
		},

		{
			description: "Dualstack CIDRs, more than two",
			fakeNodeHandler: &testutil.FakeNodeHandler{
				Existing: []*v1.Node{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "node0",
						},
					},
				},
				Clientset: fake.NewSimpleClientset(),
			},
			fakeNodePoolHandler: &test.FakeNodePoolHandler{
				Clientset: appsfake.NewSimpleClientset(),
			},
			clusterCIDRs: func() []*net.IPNet {
				_, clusterCIDRv4, _ := net.ParseCIDR("127.123.234.0/8")
				_, clusterCIDRv6, _ := net.ParseCIDR("ace:cab:deca::/8")
				_, clusterCIDRv4_2, _ := net.ParseCIDR("10.0.0.0/8")
				return []*net.IPNet{clusterCIDRv4, clusterCIDRv6, clusterCIDRv4_2}
			}(),
			serviceCIDR: func() *net.IPNet {
				_, serviceCIDR, _ := net.ParseCIDR("127.123.234.0/26")
				return serviceCIDR
			}(),
			secondaryServiceCIDR: nil,
		},
		{
			description: "Nodepool: Correctly ignore already allocated CIDRs and allocate 1",
			fakeNodeHandler: &testutil.FakeNodeHandler{
				Existing:  []*v1.Node{},
				Clientset: fake.NewSimpleClientset(),
			},
			fakeNodePoolHandler: &test.FakeNodePoolHandler{
				Existing: []*v1alpha1.NodePool{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:        "nodepool0",
							Annotations: map[string]string{util.NodePoolMaxNodesAnnotation: "1"},
						},
						Spec: v1alpha1.NodePoolSpec{
							Type: v1alpha1.Edge,
						},
					},
				},
				Clientset: appsfake.NewSimpleClientset(),
			},
			clusterCIDRs: func() []*net.IPNet {
				_, clusterCIDR, _ := net.ParseCIDR("127.123.234.0/24")
				return []*net.IPNet{clusterCIDR}
			}(),
			serviceCIDR: func() *net.IPNet {
				_, serviceCIDR, _ := net.ParseCIDR("127.123.234.0/26")
				return serviceCIDR
			}(),
			secondaryServiceCIDR: nil,
			subNetMaskSize:       30,
			nodePoolAllocatedCIDRs: map[int][]string{
				0: {"127.123.234.64/30", "127.123.234.68/30", "127.123.234.72/30", "127.123.234.80/30"},
			},
			expectedNodePoolAllocatedCIDR: map[int]string{
				0: "127.123.234.76/30",
			},
		},
		{
			description: "Nodepool: Correctly ignore already allocated CIDRs and allocate 2",
			fakeNodeHandler: &testutil.FakeNodeHandler{
				Existing:  []*v1.Node{},
				Clientset: fake.NewSimpleClientset(),
			},
			fakeNodePoolHandler: &test.FakeNodePoolHandler{
				Existing: []*v1alpha1.NodePool{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:        "nodepool1",
							Annotations: map[string]string{util.NodePoolMaxNodesAnnotation: "2"},
						},
						Spec: v1alpha1.NodePoolSpec{
							Type: v1alpha1.Edge,
						},
					},
				},
				Clientset: appsfake.NewSimpleClientset(),
			},
			clusterCIDRs: func() []*net.IPNet {
				_, clusterCIDR, _ := net.ParseCIDR("127.123.234.0/24")
				return []*net.IPNet{clusterCIDR}
			}(),
			serviceCIDR: func() *net.IPNet {
				_, serviceCIDR, _ := net.ParseCIDR("127.123.234.0/26")
				return serviceCIDR
			}(),
			secondaryServiceCIDR: nil,
			subNetMaskSize:       30,
			nodePoolAllocatedCIDRs: map[int][]string{
				0: {"127.123.234.64/30", "127.123.234.68/30", "127.123.234.72/30", "127.123.234.80/30"},
			},
			expectedNodePoolAllocatedCIDR: map[int]string{
				0: "127.123.234.76/30,127.123.234.84/30",
			},
		},
	}

	// test function
	testFunc := func(tc testCase) {
		// Initialize the range allocator.
		allocator, err := NewCIDRRangeAllocator(tc.fakeNodeHandler, tc.fakeNodePoolHandler, getFakeNodeInformer(tc.fakeNodeHandler), getFakeNodePoolInformer(tc.fakeNodePoolHandler),
			tc.clusterCIDRs, tc.serviceCIDR, tc.secondaryServiceCIDR, tc.subNetMaskSize, nil, nil, false)
		if err != nil {
			t.Errorf("%v: failed to create CIDRRangeAllocator with error %v", tc.description, err)
			return
		}
		rangeAllocator, ok := allocator.(*rangeAllocator)
		if !ok {
			t.Logf("%v: found non-default implementation of CIDRAllocator, skipping white-box test...", tc.description)
			return
		}
		rangeAllocator.nodesSynced = alwaysReady
		rangeAllocator.nodepoolSynced = alwaysReady
		rangeAllocator.recorder = testutil.NewFakeRecorder()
		go allocator.Run(wait.NeverStop)

		// this is a bit of white box testing
		// pre allocate the cidrs as per the test
		for idx, allocatedList := range tc.nodeAllocatedCIDRs {
			for _, allocated := range allocatedList {
				_, cidr, err := net.ParseCIDR(allocated)
				if err != nil {
					t.Fatalf("%v: unexpected error when parsing CIDR %v: %v", tc.description, allocated, err)
				}
				if err = rangeAllocator.clusterCidrSets[idx].Occupy(cidr); err != nil {
					t.Fatalf("%v: unexpected error when occupying CIDR %v: %v", tc.description, allocated, err)
				}
			}
			if err := allocator.AllocateOrOccupyCIDR(tc.fakeNodeHandler.Existing[0]); err != nil {
				t.Errorf("%v: unexpected error in AllocateOrOccupyCIDR: %v", tc.description, err)
			}
			if err := waitForUpdatedNodeWithTimeout(tc.fakeNodeHandler, 1, wait.ForeverTestTimeout); err != nil {
				t.Fatalf("%v: timeout while waiting for Node update: %v", tc.description, err)
			}
		}

		if len(tc.expectedNodeAllocatedCIDR) != 0 {
			for _, updatedNode := range tc.fakeNodeHandler.GetUpdatedNodesCopy() {
				if len(updatedNode.Spec.PodCIDRs) == 0 {
					continue // not assigned yet
				}
				//match
				for podCIDRIdx, expectedPodCIDR := range tc.expectedNodeAllocatedCIDR {
					if updatedNode.Spec.PodCIDRs[podCIDRIdx] != expectedPodCIDR {
						t.Errorf("%v: Unable to find allocated CIDR %v, found updated Nodes with CIDRs: %v", tc.description, expectedPodCIDR, updatedNode.Spec.PodCIDRs)
						break
					}
				}
			}
		}

		// nodepool test
		for idx, allocatedList := range tc.nodePoolAllocatedCIDRs {
			for _, allocated := range allocatedList {
				_, cidr, err := net.ParseCIDR(allocated)
				if err != nil {
					t.Fatalf("%v: unexpected error when parsing CIDR %v: %v", tc.description, allocated, err)
				}
				if err = rangeAllocator.clusterCidrSets[idx].Occupy(cidr); err != nil {
					t.Fatalf("%v: unexpected error when occupying CIDR %v: %v", tc.description, allocated, err)
				}
			}

			if err := allocator.AllocateOrOccupyNodepoolCIDR(tc.fakeNodePoolHandler.Existing[0]); err != nil {
				t.Errorf("%v: unexpected error in AllocateOrOccupyCIDR: %v", tc.description, err)
			}
			if err := waitForUpdatedNodePoolWithTimeout(tc.fakeNodePoolHandler, 1, wait.ForeverTestTimeout); err != nil {
				t.Fatalf("%v: timeout while waiting for Nodepool update: %v", tc.description, err)
			}
		}

		if len(tc.expectedNodePoolAllocatedCIDR) == 0 {
			// nothing further expected
			return
		}
		for _, updatedNodepool := range tc.fakeNodePoolHandler.GetUpdatedNodePoolsCopy() {
			if updatedNodepool.Annotations == nil || updatedNodepool.Annotations[util.NodePoolPodCIDRAnnotation] == "" {
				continue // not assigned yet
			}
			// for nodepool, only one global cluster cidr is supported
			if tc.expectedNodePoolAllocatedCIDR[0] != updatedNodepool.Annotations[util.NodePoolPodCIDRAnnotation] {
				t.Errorf("%v: Unable to find allocated CIDR %v, found updated Nodepools with CIDRs: %v",
					tc.description, tc.expectedNodePoolAllocatedCIDR[0], updatedNodepool.Annotations[util.NodePoolPodCIDRAnnotation])
			}
		}
	}

	// run the test cases
	for _, tc := range testCases {
		testFunc(tc)
	}
}

func TestAllocateOrOccupyCIDRFailure(t *testing.T) {
	testCases := []testCase{
		{
			description: "When there's no ServiceCIDR return first CIDR in range",
			fakeNodeHandler: &testutil.FakeNodeHandler{
				Existing: []*v1.Node{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "node0",
						},
					},
				},
				Clientset: fake.NewSimpleClientset(),
			},
			fakeNodePoolHandler: &test.FakeNodePoolHandler{
				Clientset: appsfake.NewSimpleClientset(),
			},
			clusterCIDRs: func() []*net.IPNet {
				_, clusterCIDR, _ := net.ParseCIDR("127.123.234.0/28")
				return []*net.IPNet{clusterCIDR}
			}(),
			serviceCIDR:          nil,
			secondaryServiceCIDR: nil,
			subNetMaskSize:       30,
			nodeAllocatedCIDRs: map[int][]string{
				0: {"127.123.234.0/30", "127.123.234.4/30", "127.123.234.8/30", "127.123.234.12/30"},
			},
		},
		{
			description: "Nodepool: When there's no ServiceCIDR return first CIDR in range",
			fakeNodeHandler: &testutil.FakeNodeHandler{
				Existing:  []*v1.Node{},
				Clientset: fake.NewSimpleClientset(),
			},
			fakeNodePoolHandler: &test.FakeNodePoolHandler{
				Existing: []*v1alpha1.NodePool{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:        "nodepool1",
							Annotations: map[string]string{util.NodePoolMaxNodesAnnotation: "2"},
						},
						Spec: v1alpha1.NodePoolSpec{
							Type: v1alpha1.Edge,
						},
					},
				},
				Clientset: appsfake.NewSimpleClientset(),
			},
			clusterCIDRs: func() []*net.IPNet {
				_, clusterCIDR, _ := net.ParseCIDR("127.123.234.0/28")
				return []*net.IPNet{clusterCIDR}
			}(),
			serviceCIDR:          nil,
			secondaryServiceCIDR: nil,
			subNetMaskSize:       30,
			nodePoolAllocatedCIDRs: map[int][]string{
				0: {"127.123.234.0/30", "127.123.234.4/30", "127.123.234.8/30", "127.123.234.12/30"},
			},
			expectedNodePoolAllocatedCIDR: map[int]string{},
		},
	}

	testFunc := func(tc testCase) {
		// Initialize the range allocator.
		allocator, err := NewCIDRRangeAllocator(tc.fakeNodeHandler, tc.fakeNodePoolHandler, getFakeNodeInformer(tc.fakeNodeHandler), getFakeNodePoolInformer(tc.fakeNodePoolHandler),
			tc.clusterCIDRs, tc.serviceCIDR, tc.secondaryServiceCIDR, tc.subNetMaskSize, nil, nil, false)
		if err != nil {
			t.Logf("%v: failed to create CIDRRangeAllocator with error %v", tc.description, err)
		}
		rangeAllocator, ok := allocator.(*rangeAllocator)
		if !ok {
			t.Logf("%v: found non-default implementation of CIDRAllocator, skipping white-box test...", tc.description)
			return
		}
		rangeAllocator.nodesSynced = alwaysReady
		rangeAllocator.nodepoolSynced = alwaysReady
		rangeAllocator.recorder = testutil.NewFakeRecorder()
		go allocator.Run(wait.NeverStop)

		// this is a bit of white box testing
		for setIdx, allocatedList := range tc.nodeAllocatedCIDRs {
			for _, allocated := range allocatedList {
				_, cidr, err := net.ParseCIDR(allocated)
				if err != nil {
					t.Fatalf("%v: unexpected error when parsing CIDR %v: %v", tc.description, cidr, err)
				}
				err = rangeAllocator.clusterCidrSets[setIdx].Occupy(cidr)
				if err != nil {
					t.Fatalf("%v: unexpected error when occupying CIDR %v: %v", tc.description, cidr, err)
				}
			}
		}
		if len(tc.fakeNodeHandler.Existing) != 0 {
			if err := allocator.AllocateOrOccupyCIDR(tc.fakeNodeHandler.Existing[0]); err == nil {
				t.Errorf("%v: unexpected success in AllocateOrOccupyCIDR: %v", tc.description, err)
			}
			// We don't expect any updates, so just sleep for some time
			time.Sleep(time.Second)
			if len(tc.fakeNodeHandler.GetUpdatedNodesCopy()) != 0 {
				t.Fatalf("%v: unexpected update of nodes: %v", tc.description, tc.fakeNodeHandler.GetUpdatedNodesCopy())
			}
		}
		if len(tc.expectedNodeAllocatedCIDR) != 0 {
			for _, updatedNode := range tc.fakeNodeHandler.GetUpdatedNodesCopy() {
				if len(updatedNode.Spec.PodCIDRs) == 0 {
					continue // not assigned yet
				}
				//match
				for podCIDRIdx, expectedPodCIDR := range tc.expectedNodeAllocatedCIDR {
					if updatedNode.Spec.PodCIDRs[podCIDRIdx] == expectedPodCIDR {
						t.Errorf("%v: found cidr %v that should not be allocated on node with CIDRs:%v", tc.description, expectedPodCIDR, updatedNode.Spec.PodCIDRs)
						break
					}
				}
			}
		}

		// nodepool
		for idx, allocatedList := range tc.nodePoolAllocatedCIDRs {
			for _, allocated := range allocatedList {
				_, cidr, err := net.ParseCIDR(allocated)
				if err != nil {
					t.Fatalf("%v: unexpected error when parsing CIDR %v: %v", tc.description, allocated, err)
				}
				if err = rangeAllocator.clusterCidrSets[idx].Occupy(cidr); err != nil {
					t.Fatalf("%v: unexpected error when occupying CIDR %v: %v", tc.description, allocated, err)
				}
			}
		}
		if len(tc.fakeNodePoolHandler.Existing) != 0 {
			if err := allocator.AllocateOrOccupyNodepoolCIDR(tc.fakeNodePoolHandler.Existing[0]); err == nil {
				t.Errorf("%v: unexpected error in AllocateOrOccupyCIDR: %v", tc.description, err)
			}
			time.Sleep(time.Second)
			if len(tc.fakeNodePoolHandler.GetUpdatedNodePoolsCopy()) != 0 {
				t.Fatalf("%v: unexpected update of nodepools: %v", tc.description, tc.fakeNodePoolHandler.GetUpdatedNodePoolsCopy())
			}
		}
		if len(tc.expectedNodePoolAllocatedCIDR) == 0 {
			// nothing further expected
			return
		}
		for _, updatedNodepool := range tc.fakeNodePoolHandler.GetUpdatedNodePoolsCopy() {
			if updatedNodepool.Annotations == nil || updatedNodepool.Annotations[util.NodePoolPodCIDRAnnotation] == "" {
				continue // not assigned yet
			}
			// for nodepool, only one global cluster cidr is supported
			if tc.expectedNodePoolAllocatedCIDR[0] != updatedNodepool.Annotations[util.NodePoolPodCIDRAnnotation] {
				t.Errorf("%v: Unable to find allocated CIDR %v, found updated Nodepools with CIDRs: %v",
					tc.description, tc.expectedNodePoolAllocatedCIDR[0], updatedNodepool.Annotations[util.NodePoolPodCIDRAnnotation])
			}
		}
	}
	for _, tc := range testCases {
		testFunc(tc)
	}
}

type releaseTestCase struct {
	description                      string
	fakeNodeHandler                  *testutil.FakeNodeHandler
	fakeNodePoolHandler              *test.FakeNodePoolHandler
	clusterCIDRs                     []*net.IPNet
	serviceCIDR                      *net.IPNet
	secondaryServiceCIDR             *net.IPNet
	subNetMaskSize                   int
	expectedAllocatedCIDRFirstRound  map[int]string
	expectedAllocatedCIDRSecondRound map[int]string
	allocatedCIDRs                   map[int][]string
	cidrsToRelease                   [][]string
}

func TestReleaseCIDRSuccess(t *testing.T) {
	testCases := []releaseTestCase{
		{
			description: "Correctly release preallocated CIDR",
			fakeNodeHandler: &testutil.FakeNodeHandler{
				Existing: []*v1.Node{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "node0",
						},
					},
				},
				Clientset: fake.NewSimpleClientset(),
			},
			fakeNodePoolHandler: &test.FakeNodePoolHandler{
				Clientset: appsfake.NewSimpleClientset(),
			},
			clusterCIDRs: func() []*net.IPNet {
				_, clusterCIDR, _ := net.ParseCIDR("127.123.234.0/28")
				return []*net.IPNet{clusterCIDR}
			}(),
			serviceCIDR:          nil,
			secondaryServiceCIDR: nil,
			subNetMaskSize:       30,
			allocatedCIDRs: map[int][]string{
				0: {"127.123.234.0/30", "127.123.234.4/30", "127.123.234.8/30", "127.123.234.12/30"},
			},
			expectedAllocatedCIDRFirstRound: nil,
			cidrsToRelease: [][]string{
				{"127.123.234.4/30"},
			},
			expectedAllocatedCIDRSecondRound: map[int]string{
				0: "127.123.234.4/30",
			},
		},
		{
			description: "Correctly recycle CIDR",
			fakeNodeHandler: &testutil.FakeNodeHandler{
				Existing: []*v1.Node{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "node0",
						},
					},
				},
				Clientset: fake.NewSimpleClientset(),
			},
			fakeNodePoolHandler: &test.FakeNodePoolHandler{
				Clientset: appsfake.NewSimpleClientset(),
			},
			clusterCIDRs: func() []*net.IPNet {
				_, clusterCIDR, _ := net.ParseCIDR("127.123.234.0/28")
				return []*net.IPNet{clusterCIDR}
			}(),
			serviceCIDR:          nil,
			secondaryServiceCIDR: nil,
			subNetMaskSize:       30,
			allocatedCIDRs: map[int][]string{
				0: {"127.123.234.4/30", "127.123.234.8/30", "127.123.234.12/30"},
			},
			expectedAllocatedCIDRFirstRound: map[int]string{
				0: "127.123.234.0/30",
			},
			cidrsToRelease: [][]string{
				{"127.123.234.0/30"},
			},
			expectedAllocatedCIDRSecondRound: map[int]string{
				0: "127.123.234.0/30",
			},
		},
	}

	testFunc := func(tc releaseTestCase) {
		// Initialize the range allocator.
		allocator, _ := NewCIDRRangeAllocator(tc.fakeNodeHandler, tc.fakeNodePoolHandler, getFakeNodeInformer(tc.fakeNodeHandler), getFakeNodePoolInformer(tc.fakeNodePoolHandler),
			tc.clusterCIDRs, tc.serviceCIDR, tc.secondaryServiceCIDR, tc.subNetMaskSize, nil, nil, false)
		rangeAllocator, ok := allocator.(*rangeAllocator)
		if !ok {
			t.Logf("%v: found non-default implementation of CIDRAllocator, skipping white-box test...", tc.description)
			return
		}
		rangeAllocator.nodesSynced = alwaysReady
		rangeAllocator.nodepoolSynced = alwaysReady
		rangeAllocator.recorder = testutil.NewFakeRecorder()
		go allocator.Run(wait.NeverStop)

		// this is a bit of white box testing
		for setIdx, allocatedList := range tc.allocatedCIDRs {
			for _, allocated := range allocatedList {
				_, cidr, err := net.ParseCIDR(allocated)
				if err != nil {
					t.Fatalf("%v: unexpected error when parsing CIDR %v: %v", tc.description, allocated, err)
				}
				err = rangeAllocator.clusterCidrSets[setIdx].Occupy(cidr)
				if err != nil {
					t.Fatalf("%v: unexpected error when occupying CIDR %v: %v", tc.description, allocated, err)
				}
			}
		}

		err := allocator.AllocateOrOccupyCIDR(tc.fakeNodeHandler.Existing[0])
		if len(tc.expectedAllocatedCIDRFirstRound) != 0 {
			if err != nil {
				t.Fatalf("%v: unexpected error in AllocateOrOccupyCIDR: %v", tc.description, err)
			}
			if err := waitForUpdatedNodeWithTimeout(tc.fakeNodeHandler, 1, wait.ForeverTestTimeout); err != nil {
				t.Fatalf("%v: timeout while waiting for Node update: %v", tc.description, err)
			}
		} else {
			if err == nil {
				t.Fatalf("%v: unexpected success in AllocateOrOccupyCIDR: %v", tc.description, err)
			}
			// We don't expect any updates here
			time.Sleep(time.Second)
			if len(tc.fakeNodeHandler.GetUpdatedNodesCopy()) != 0 {
				t.Fatalf("%v: unexpected update of nodes: %v", tc.description, tc.fakeNodeHandler.GetUpdatedNodesCopy())
			}
		}
		for _, cidrToRelease := range tc.cidrsToRelease {
			nodeToRelease := v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node0",
				},
			}
			nodeToRelease.Spec.PodCIDRs = cidrToRelease
			err = allocator.ReleaseCIDR(&nodeToRelease)
			if err != nil {
				t.Fatalf("%v: unexpected error in ReleaseCIDR: %v", tc.description, err)
			}
		}
		if err = allocator.AllocateOrOccupyCIDR(tc.fakeNodeHandler.Existing[0]); err != nil {
			t.Fatalf("%v: unexpected error in AllocateOrOccupyCIDR: %v", tc.description, err)
		}
		if err := waitForUpdatedNodeWithTimeout(tc.fakeNodeHandler, 1, wait.ForeverTestTimeout); err != nil {
			t.Fatalf("%v: timeout while waiting for Node update: %v", tc.description, err)
		}

		if len(tc.expectedAllocatedCIDRSecondRound) == 0 {
			// nothing further expected
			return
		}
		for _, updatedNode := range tc.fakeNodeHandler.GetUpdatedNodesCopy() {
			if len(updatedNode.Spec.PodCIDRs) == 0 {
				continue // not assigned yet
			}
			//match
			for podCIDRIdx, expectedPodCIDR := range tc.expectedAllocatedCIDRSecondRound {
				if updatedNode.Spec.PodCIDRs[podCIDRIdx] != expectedPodCIDR {
					t.Errorf("%v: found cidr %v that should not be allocated on node with CIDRs:%v", tc.description, expectedPodCIDR, updatedNode.Spec.PodCIDRs)
					break
				}
			}
		}
	}

	for _, tc := range testCases {
		testFunc(tc)
	}
}
