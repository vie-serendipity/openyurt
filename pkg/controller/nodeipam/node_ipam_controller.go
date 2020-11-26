/*
Copyright 2014 The Kubernetes Authors.

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

package nodeipam

import (
	"net"

	v1 "k8s.io/api/core/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	coreinformers "k8s.io/client-go/informers/core/v1"
	clientset "k8s.io/client-go/kubernetes"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/component-base/metrics/prometheus/ratelimiter"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/controller/nodeipam/ipam"
	"github.com/openyurtio/yurt-app-manager-api/pkg/yurtappmanager/client/clientset/versioned"
	"github.com/openyurtio/yurt-app-manager-api/pkg/yurtappmanager/client/informers/externalversions/apps/v1alpha1"
	listersv1alpha1 "github.com/openyurtio/yurt-app-manager-api/pkg/yurtappmanager/client/listers/apps/v1alpha1"
)

// Controller is the controller that manages node ipam state.
type Controller struct {
	allocatorType ipam.CIDRAllocatorType

	clusterCIDRs         []*net.IPNet
	serviceCIDR          *net.IPNet
	secondaryServiceCIDR *net.IPNet
	// Method for easy mocking in appstest.
	lookupIP func(host string) ([]net.IP, error)

	nodeLister         corelisters.NodeLister
	nodeInformerSynced cache.InformerSynced

	nodepoolLister         listersv1alpha1.NodePoolLister
	nodepoolInformerSynced cache.InformerSynced

	cidrAllocator ipam.CIDRAllocator

	forcefullyDeletePod func(*v1.Pod) error
}

// NewNodeIpamController returns a new node IP Address Management controller to
// sync instances from cloudprovider.
// This method returns an error if it is unable to initialize the CIDR bitmap with
// podCIDRs it has already allocated to nodes. Since we don't allow podCIDR changes
// currently, this should be handled as a fatal error.
func NewNodeIpamController(
	nodeInformer coreinformers.NodeInformer,
	nodepoolInformer v1alpha1.NodePoolInformer,
	kubeClient clientset.Interface,
	appsClient versioned.Interface,
	clusterCIDRs []*net.IPNet,
	serviceCIDR *net.IPNet,
	secondaryServiceCIDR *net.IPNet,
	nodeCIDRMaskSize int,
	allocatorType ipam.CIDRAllocatorType,
	legacyIpam bool) (*Controller, error) {

	if kubeClient == nil || appsClient == nil {
		klog.Fatalf("kubeClient is nil when starting Controller")
	}

	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(klog.Infof)

	klog.Infof("Sending events to api server.")
	eventBroadcaster.StartRecordingToSink(
		&v1core.EventSinkImpl{
			Interface: kubeClient.CoreV1().Events(""),
		})

	if kubeClient.CoreV1().RESTClient().GetRateLimiter() != nil {
		ratelimiter.RegisterMetricAndTrackRateLimiterUsage("node_ipam_controller", kubeClient.CoreV1().RESTClient().GetRateLimiter())
	}

	// Cloud CIDR allocator does not rely on clusterCIDR or nodeCIDRMaskSize for allocation.
	if allocatorType != ipam.CloudAllocatorType {
		if len(clusterCIDRs) == 0 {
			klog.Fatal("Controller: Must specify --cluster-cidr if --allocate-node-cidrs is set")
		}

		// TODO: (khenidak) IPv6DualStack beta:
		// - modify mask to allow flexible masks for IPv4 and IPv6
		// - for alpha status they are the same

		// for each cidr, node mask size must be < cidr mask
		for _, cidr := range clusterCIDRs {
			mask := cidr.Mask
			if maskSize, _ := mask.Size(); maskSize > nodeCIDRMaskSize {
				klog.Fatal("Controller: Invalid --cluster-cidr, mask size of cluster CIDR must be less than --node-cidr-mask-size")
			}
		}
	}

	ic := &Controller{
		lookupIP:             net.LookupIP,
		clusterCIDRs:         clusterCIDRs,
		serviceCIDR:          serviceCIDR,
		secondaryServiceCIDR: secondaryServiceCIDR,
		allocatorType:        allocatorType,
	}

	var err error
	ic.cidrAllocator, err = ipam.New(kubeClient, appsClient, nodeInformer, nodepoolInformer, ic.allocatorType, clusterCIDRs,
		ic.serviceCIDR, ic.secondaryServiceCIDR, nodeCIDRMaskSize, legacyIpam)
	if err != nil {
		return nil, err
	}

	ic.nodeLister = nodeInformer.Lister()
	ic.nodeInformerSynced = nodeInformer.Informer().HasSynced
	ic.nodepoolLister = nodepoolInformer.Lister()
	ic.nodepoolInformerSynced = nodepoolInformer.Informer().HasSynced

	return ic, nil
}

// Run starts an asynchronous loop that monitors the status of cluster nodes.
func (nc *Controller) Run(stopCh <-chan struct{}) {
	defer utilruntime.HandleCrash()

	klog.Infof("Starting ipam controller")
	defer klog.Infof("Shutting down ipam controller")

	if !cache.WaitForCacheSync(stopCh, nc.nodeInformerSynced, nc.nodepoolInformerSynced) {
		return
	}

	if nc.allocatorType != ipam.IPAMFromClusterAllocatorType && nc.allocatorType != ipam.IPAMFromCloudAllocatorType {
		go nc.cidrAllocator.Run(stopCh)
	}

	<-stopCh
}
