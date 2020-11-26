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
	"context"
	"fmt"
	"net"
	"time"

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/wait"
	informers "k8s.io/client-go/informers/core/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	"github.com/openyurtio/yurt-app-manager-api/pkg/yurtappmanager/apis/apps/v1alpha1"
	"github.com/openyurtio/yurt-app-manager-api/pkg/yurtappmanager/client/clientset/versioned"
	informersv1alpha1 "github.com/openyurtio/yurt-app-manager-api/pkg/yurtappmanager/client/informers/externalversions/apps/v1alpha1"
)

// CIDRAllocatorType is the type of the allocator to use.
type CIDRAllocatorType string

const (
	// RangeAllocatorType is the allocator that uses an internal CIDR
	// range allocator to do node CIDR range allocations.
	RangeAllocatorType CIDRAllocatorType = "RangeAllocator"
	// CloudAllocatorType is the allocator that uses cloud platform
	// support to do node CIDR range allocations.
	CloudAllocatorType CIDRAllocatorType = "CloudAllocator"
	// IPAMFromClusterAllocatorType uses the ipam controller sync'ing the node
	// CIDR range allocations from the cluster to the cloud.
	IPAMFromClusterAllocatorType = "IPAMFromCluster"
	// IPAMFromCloudAllocatorType uses the ipam controller sync'ing the node
	// CIDR range allocations from the cloud to the cluster.
	IPAMFromCloudAllocatorType = "IPAMFromCloud"

	// The amount of time the nodecontroller polls on the list nodes endpoint.
	apiserverStartupGracePeriod = 10 * time.Minute

	// The no. of NodeSpec updates NC can process concurrently.
	cidrUpdateWorkers = 30

	// The max no. of NodeSpec updates that can be enqueued.
	cidrUpdateQueueSize = 5000

	// cidrUpdateRetries is the no. of times a NodeSpec update will be retried before dropping it.
	cidrUpdateRetries = 3
)

// CIDRAllocator is an interface implemented by things that know how
// to allocate/occupy/recycle CIDR for nodes.
type CIDRAllocator interface {
	// AllocateOrOccupyCIDR looks at the given node, assigns it a valid
	// CIDR if it doesn't currently have one or mark the CIDR as used if
	// the node already have one.
	AllocateOrOccupyCIDR(node *v1.Node) error
	// ReleaseCIDR releases the CIDR of the removed node
	ReleaseCIDR(node *v1.Node) error
	// AllocateOrOccupyNodepoolCIDR allocate cluster cidrs to edge nodepool
	AllocateOrOccupyNodepoolCIDR(nodepool *v1alpha1.NodePool) error
	// ReleaseNodepoolCIDR release cidrs on nodepool deletion
	ReleaseNodepoolCIDR(nodepool *v1alpha1.NodePool) error
	// AssumeNodepoolCIDR check if cluster cidrs capacity matches nodepool cidr request
	AssumeNodepoolCIDR(nodepool *v1alpha1.NodePool) error
	// Run starts all the working logic of the allocator.
	Run(stopCh <-chan struct{})
}

// New creates a new CIDR range allocator.
func New(kubeClient clientset.Interface, appsClient versioned.Interface, nodeInformer informers.NodeInformer, nodepoolInformer informersv1alpha1.NodePoolInformer,
	allocatorType CIDRAllocatorType, clusterCIDRs []*net.IPNet, serviceCIDR *net.IPNet, secondaryServiceCIDR *net.IPNet, nodeCIDRMaskSize int, legacyIpam bool) (CIDRAllocator, error) {
	nodeList, err := listNodes(kubeClient)
	if err != nil {
		return nil, err
	}
	nodepoolList, err := listNodepools(appsClient)
	if err != nil {
		return nil, err
	}
	switch allocatorType {
	case RangeAllocatorType:
		return NewCIDRRangeAllocator(kubeClient, appsClient, nodeInformer, nodepoolInformer, clusterCIDRs, serviceCIDR, secondaryServiceCIDR, nodeCIDRMaskSize, nodeList, nodepoolList, legacyIpam)
	default:
		return nil, fmt.Errorf("invalid CIDR allocator type: %v", allocatorType)
	}
}

func listNodes(kubeClient clientset.Interface) (*v1.NodeList, error) {
	var nodeList *v1.NodeList
	// We must poll because apiserver might not be up. This error causes
	// controller manager to restart.
	if pollErr := wait.Poll(10*time.Second, apiserverStartupGracePeriod, func() (bool, error) {
		var err error
		nodeList, err = kubeClient.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{
			FieldSelector: fields.Everything().String(),
			LabelSelector: labels.Everything().String(),
		})
		if err != nil {
			klog.Errorf("Failed to list all nodes: %v", err)
			return false, nil
		}
		return true, nil
	}); pollErr != nil {
		return nil, fmt.Errorf("failed to list all nodes in %v, cannot proceed without updating CIDR map",
			apiserverStartupGracePeriod)
	}
	return nodeList, nil
}

func listNodepools(appsClient versioned.Interface) (*v1alpha1.NodePoolList, error) {
	var nodepoolList *v1alpha1.NodePoolList
	// We must poll because apiserver might not be up. This error causes
	// controller manager to restart.
	if pollErr := wait.Poll(10*time.Second, apiserverStartupGracePeriod, func() (bool, error) {
		var err error
		nodepoolList, err = appsClient.AppsV1alpha1().NodePools().List(context.TODO(), metav1.ListOptions{
			FieldSelector: fields.Everything().String(),
			LabelSelector: labels.Everything().String(),
		})
		if err != nil {
			klog.Errorf("Failed to list all nodepools: %v", err)
			if apierrors.IsNotFound(err) {
				return true, nil
			}
			return false, nil
		}
		return true, nil
	}); pollErr != nil {
		return nil, fmt.Errorf("failed to list all nodepools in %v, cannot proceed without updating CIDR map",
			apiserverStartupGracePeriod)
	}
	return nodepoolList, nil
}
