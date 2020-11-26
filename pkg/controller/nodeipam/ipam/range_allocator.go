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
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	informers "k8s.io/client-go/informers/core/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	nodeutil "k8s.io/component-helpers/node/util"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/controller/nodeipam/ipam/cidrset"
	"github.com/openyurtio/openyurt/pkg/controller/util"
	yurtnodeutil "github.com/openyurtio/openyurt/pkg/controller/util/node"
	"github.com/openyurtio/yurt-app-manager-api/pkg/yurtappmanager/apis/apps/v1alpha1"
	"github.com/openyurtio/yurt-app-manager-api/pkg/yurtappmanager/client/clientset/versioned"
	yurtscheme "github.com/openyurtio/yurt-app-manager-api/pkg/yurtappmanager/client/clientset/versioned/scheme"
	informersv1alpha1 "github.com/openyurtio/yurt-app-manager-api/pkg/yurtappmanager/client/informers/externalversions/apps/v1alpha1"
	listersv1alpha1 "github.com/openyurtio/yurt-app-manager-api/pkg/yurtappmanager/client/listers/apps/v1alpha1"
)

// cidrs are reserved, then node resource is patched with them
// this type holds the reservation info for a node
type nodeReservedCIDRs struct {
	allocatedCIDRs []*net.IPNet
	nodeName       string
}

// nodepool cidrs to be persisted
type nodepoolReservedCIDRs struct {
	allocatedCIDRs [][]*net.IPNet
	nodepool       *v1alpha1.NodePool
	isDeleted      bool
}

type rangeAllocator struct {
	client     clientset.Interface
	appsClient versioned.Interface
	// cluster cidrs as passed in during controller creation
	clusterCIDRs []*net.IPNet
	// for each entry in clusterCIDRs we maintain a list of what is used and what is not
	clusterCidrSets []*cidrset.CidrSet
	// (edge k8s): split global cidrsets to subsets per nodepool, and allocate podcidr from each nodepool's cidr subsets
	// key: nodepool name, value: cidr subsets
	nodepoolCidrSets map[string][][]*cidrset.CidrSet
	// subnet mask for node
	nodeSubNetMaskSize int
	// nodeLister is able to list/get nodes and is populated by the shared informer passed to controller
	nodeLister corelisters.NodeLister
	// nodepoolLister is able to list/get nodepools and is populated by the shared informer passed to controller
	nodepoolLister listersv1alpha1.NodePoolLister
	// nodesSynced returns true if the node shared informer has been synced at least once.
	nodesSynced cache.InformerSynced
	// nodepoolSynced returns true if the nodepool shared informer has been synced at least once.
	nodepoolSynced cache.InformerSynced
	// informerCacheSynced is atomic value to record shared informer synced status for once
	informerCacheSynced *atomic.Value

	// Channel that is used to pass updating Nodes and their reserved CIDRs to the background
	// This increases a throughput of CIDR assignment by not blocking on long operations.
	nodeCIDRUpdateChannel chan nodeReservedCIDRs
	// pass updating nodepools and the reserved CIDRs
	nodepoolUpdateChannel chan nodepoolReservedCIDRs

	recorder record.EventRecorder
	// Keep a set of nodes that are currectly being processed to avoid races in CIDR allocation
	lock                  sync.Mutex
	nodesInProcessing     sets.String
	nodepoolsLock         sync.Mutex
	nodepoolsInProcessing sets.String

	legacyIpam bool
}

// NewCIDRRangeAllocator returns a CIDRAllocator to allocate CIDRs for node (one from each of clusterCIDRs)
// Caller must ensure subNetMaskSize is not less than cluster CIDR mask size.
// Caller must always pass in a list of existing nodes so the new allocator.
// Caller must ensure that ClusterCIDRs are semantically correct e.g (1 for non DualStack, 2 for DualStack etc..)
// can initialize its CIDR map. NodeList is only nil in testing.
func NewCIDRRangeAllocator(client clientset.Interface, appsClient versioned.Interface, nodeInformer informers.NodeInformer, nodepoolInformer informersv1alpha1.NodePoolInformer,
	clusterCIDRs []*net.IPNet, serviceCIDR *net.IPNet, secondaryServiceCIDR *net.IPNet, nodeSubNetMaskSize int, nodeList *v1.NodeList, nodepoolList *v1alpha1.NodePoolList, legacyIpam bool) (CIDRAllocator, error) {
	if client == nil {
		klog.Fatalf("kubeClient is nil when starting NodeController")
	}

	eventBroadcaster := record.NewBroadcaster()
	utilruntime.Must(yurtscheme.AddToScheme(scheme.Scheme))
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, v1.EventSource{Component: "cidrAllocator"})
	eventBroadcaster.StartLogging(klog.Infof)
	klog.V(0).Infof("Sending events to api server.")
	eventBroadcaster.StartRecordingToSink(&v1core.EventSinkImpl{Interface: client.CoreV1().Events("")})

	// create a cidrSet for each cidr we operate on
	// cidrSet are mapped to clusterCIDR by index
	cidrSets := make([]*cidrset.CidrSet, len(clusterCIDRs))
	for idx := range clusterCIDRs {
		cidrSet, err := cidrset.NewCIDRSet(clusterCIDRs[idx], nodeSubNetMaskSize)
		if err != nil {
			return nil, err
		}
		cidrSets[idx] = cidrSet
	}

	ra := &rangeAllocator{
		client:                client,
		appsClient:            appsClient,
		clusterCIDRs:          clusterCIDRs,
		clusterCidrSets:       cidrSets,
		nodepoolCidrSets:      make(map[string][][]*cidrset.CidrSet),
		nodeSubNetMaskSize:    nodeSubNetMaskSize,
		nodeLister:            nodeInformer.Lister(),
		nodepoolLister:        nodepoolInformer.Lister(),
		nodesSynced:           nodeInformer.Informer().HasSynced,
		nodepoolSynced:        nodepoolInformer.Informer().HasSynced,
		informerCacheSynced:   &atomic.Value{},
		nodeCIDRUpdateChannel: make(chan nodeReservedCIDRs, cidrUpdateQueueSize),
		nodepoolUpdateChannel: make(chan nodepoolReservedCIDRs, cidrUpdateQueueSize),
		recorder:              recorder,
		nodesInProcessing:     sets.NewString(),
		nodepoolsInProcessing: sets.NewString(),
		legacyIpam:            legacyIpam,
	}
	ra.informerCacheSynced.Store(false)

	// (edge k8s) in ack case, we don't allow service CIDR conflict
	if serviceCIDR != nil {
		ra.filterOutServiceRange(serviceCIDR)
	} else {
		klog.V(0).Info("No Service CIDR provided. Skipping filtering out service addresses.")
	}

	if secondaryServiceCIDR != nil {
		ra.filterOutServiceRange(secondaryServiceCIDR)
	} else {
		klog.V(0).Info("No Secondary Service CIDR provided. Skipping filtering out secondary service addresses.")
	}

	// (edge k8s) initialize and occupy nodepool cidrsets from cluster cidrsets
	if !legacyIpam {
		if nodepoolList != nil {
			for _, nodepool := range nodepoolList.Items {
				if nodepool.Annotations == nil || nodepool.Annotations[util.NodePoolPodCIDRAnnotation] == "" {
					klog.V(4).Infof("nodepool %s has no cidr assigned, ignoring", nodepool.Name)
					continue
				}
				if err := ra.occupyNodePoolCIDRs(&nodepool); err != nil {
					return nil, err
				}
			}
		}
	}

	// occupy node cidrsets
	if nodeList != nil {
		for _, node := range nodeList.Items {
			nodeCidr := getNodeCIDR(&node)
			if len(nodeCidr) == 0 {
				klog.V(4).Infof("Node %v has no CIDR, ignoring", node.Name)
				continue
			}
			klog.V(4).Infof("Node %v has CIDR %v, occupying it in CIDR map", node.Name, nodeCidr)
			if err := ra.occupyNodeCIDRs(&node); err != nil {
				// This will happen if:
				// 1. We find garbage in the podCIDRs field. Retrying is useless.
				// 2. CIDR out of range: This means a node CIDR has changed.
				// This error will keep crashing controller-manager.
				return nil, err
			}
		}
	}

	nodeInformer.Informer().AddEventHandlerWithResyncPeriod(cache.ResourceEventHandlerFuncs{
		AddFunc: yurtnodeutil.CreateAddNodeHandler(ra.AllocateOrOccupyCIDR),
		UpdateFunc: yurtnodeutil.CreateUpdateNodeHandler(func(_, newNode *v1.Node) error {
			// If the PodCIDRs list is not empty we either:
			// - already processed a Node that already had CIDRs after NC restarted
			//   (cidr is marked as used),
			// - already processed a Node successfully and allocated CIDRs for it
			//   (cidr is marked as used),
			// - already processed a Node but we did saw a "timeout" response and
			//   request eventually got through in this case we haven't released
			//   the allocated CIDRs (cidr is still marked as used).
			// There's a possible error here:
			// - NC sees a new Node and assigns CIDRs X,Y.. to it,
			// - Update Node call fails with a timeout,
			// - Node is updated by some other component, NC sees an update and
			//   assigns CIDRs A,B.. to the Node,
			// - Both CIDR X,Y.. and CIDR A,B.. are marked as used in the local cache,
			//   even though Node sees only CIDR A,B..
			// The problem here is that in in-memory cache we see CIDR X,Y.. as marked,
			// which prevents it from being assigned to any new node. The cluster
			// state is correct.
			// Restart of NC fixes the issue.
			if len(getNodeCIDR(newNode)) == 0 {
				return ra.AllocateOrOccupyCIDR(newNode)
			}
			return nil
		}),
		DeleteFunc: yurtnodeutil.CreateDeleteNodeHandler(ra.ReleaseCIDR),
	}, 1*time.Minute)

	if !legacyIpam {
		nodepoolInformer.Informer().AddEventHandlerWithResyncPeriod(cache.ResourceEventHandlerFuncs{
			AddFunc: func(originalObj interface{}) {
				nodepool := originalObj.(*v1alpha1.NodePool).DeepCopy()
				if err := ra.AllocateOrOccupyNodepoolCIDR(nodepool); err != nil {
					utilruntime.HandleError(fmt.Errorf("Error while processing Nodepool Add: %v", err))
				}
			},
			UpdateFunc: func(origOldObj, origNewObj interface{}) {
				nodepool := origNewObj.(*v1alpha1.NodePool).DeepCopy()
				if err := ra.AllocateOrOccupyNodepoolCIDR(nodepool); err != nil {
					utilruntime.HandleError(fmt.Errorf("Error while processing Nodepool Update: %v", err))
				}
			},
			DeleteFunc: func(originalObj interface{}) {
				originalNodepool, isNode := originalObj.(*v1alpha1.NodePool)
				if !isNode {
					deletedState, ok := originalObj.(cache.DeletedFinalStateUnknown)
					if !ok {
						klog.Errorf("Received unexpected object: %v", originalObj)
						return
					}
					originalNodepool, ok = deletedState.Obj.(*v1alpha1.NodePool)
					if !ok {
						klog.Errorf("DeletedFinalStateUnknown contained non-Nodepool object: %v", deletedState.Obj)
						return
					}
				}
				nodepool := originalNodepool.DeepCopy()
				if err := ra.ReleaseNodepoolCIDR(nodepool); err != nil {
					utilruntime.HandleError(fmt.Errorf("Error while processing Nodepool Delete: %v", err))
				}
			},
		}, 1*time.Minute)
	}

	return ra, nil
}

func (r *rangeAllocator) Run(stopCh <-chan struct{}) {
	defer utilruntime.HandleCrash()

	klog.Infof("Starting range CIDR allocator")
	defer klog.Infof("Shutting down range CIDR allocator")

	if !cache.WaitForNamedCacheSync("cidrallocator", stopCh, r.nodesSynced, r.nodepoolSynced) {
		return
	}
	r.informerCacheSynced.Store(true)

	for i := 0; i < cidrUpdateWorkers; i++ {
		go r.worker(stopCh)
		if !r.legacyIpam {
			go r.nodepoolUpdateWorker(stopCh)
		}
	}

	<-stopCh
}

func (r *rangeAllocator) worker(stopChan <-chan struct{}) {
	for {
		select {
		case workItem, ok := <-r.nodeCIDRUpdateChannel:
			if !ok {
				klog.Warning("Channel nodeCIDRUpdateChannel was unexpectedly closed")
				return
			}
			if err := r.updateCIDRsAllocation(workItem); err != nil {
				// TODO: add ratelimit
				// Requeue the failed node for update again.
				//r.nodeCIDRUpdateChannel <- workItem
				klog.Errorf("updateCIDRsAllocation failed: %v", err)
			}
		case <-stopChan:
			return
		}
	}
}

func (r *rangeAllocator) insertNodeToProcessing(nodeName string) bool {
	r.lock.Lock()
	defer r.lock.Unlock()
	if r.nodesInProcessing.Has(nodeName) {
		return false
	}
	r.nodesInProcessing.Insert(nodeName)
	return true
}

func (r *rangeAllocator) removeNodeFromProcessing(nodeName string) {
	r.lock.Lock()
	defer r.lock.Unlock()
	r.nodesInProcessing.Delete(nodeName)
}

// marks node.PodCIDRs[...] as used in allocator's tracked cidrSet
func (r *rangeAllocator) occupyNodeCIDRs(node *v1.Node) error {
	defer r.removeNodeFromProcessing(node.Name)
	nodeCidr := getNodeCIDR(node)
	if len(nodeCidr) == 0 {
		return nil
	}

	for idx, cidr := range nodeCidr {
		_, podCIDR, err := net.ParseCIDR(cidr)
		if err != nil {
			return fmt.Errorf("failed to parse node %s, CIDR %s", node.Name, cidr)
		}
		if err := r.clusterCidrSets[idx].Occupy(podCIDR); err != nil {
			return fmt.Errorf("failed to mark cidr[%v] at idx [%v] as occupied for node: %v: %v", podCIDR, idx, node.Name, err)
		}
		klog.Infof("occupied node cidr %v", podCIDR.String())
	}

	// (edge) for edge node, also mark the nodepool cidrsets as occupied
	if yurtnodeutil.IsCloudNode(node) || r.legacyIpam {
		return nil
	}

	// (edge k8s) don't support edge node not owned by nodepool
	nodepool := r.getNodeCurrentPool(node)
	if nodepool == nil {
		return nil
	}
	// skip nodepool allocation
	if useLegacyIpam(nodepool) {
		return nil
	}

	nodepoolName := nodepool.Name
	if len(r.nodepoolCidrSets[nodepoolName]) == 0 {
		klog.V(4).Infof("empty cidrsets for nodePool %s", nodepool.Name)
		return nil
	}

	if len(r.nodepoolCidrSets[nodepoolName]) != len(r.clusterCIDRs) {
		klog.Errorf("length of nodepool %s cidrs is not equal to cluster cidrs, got %v", nodepool.Name, len(r.nodepoolCidrSets[nodepoolName]))
		return nil
	}

	for idx, cidr := range nodeCidr {
		exists, err := r.occupyNodeCIDRInNodepool(idx, cidr, nodepoolName, node)
		if err != nil {
			return err
		}

		if !exists {
			// node podcidr not located in nodepool's cidr, it might happen when node migrated between nodepool, this should be unexpected
			// try to find the real nodepool and occupy to avoid duplicate IPs
			// don't handle node deletion in this case, restart controller will avoid the node cidr leak
			for npName := range r.nodepoolCidrSets {
				if npName == nodepoolName {
					continue
				}
				innerExists, err := r.occupyNodeCIDRInNodepool(idx, cidr, npName, node)
				if err != nil {
					return err
				}
				if innerExists {
					// exists in other nodepool, occupy it
					klog.Warningf("node %s cidr %s exists in nodepool %s which not belonging to, occupied", node.Name, cidr, npName)
					break
				}
			}
		}
	}

	return nil
}

// returns if cidr exists in nodepool, and error
func (r *rangeAllocator) occupyNodeCIDRInNodepool(idx int, cidr, nodepoolName string, node *v1.Node) (bool, error) {
	_, podCIDR, err := net.ParseCIDR(cidr)
	if err != nil {
		return false, fmt.Errorf("failed to parse node %s, CIDR %s", node.Name, cidr)
	}
	// (edge k8s) occupy in nodepool subsets, check if the podCIDR is located in nodepool subsets first
	// this might happen when node migrated to a new nodepool, we need to recreate the node in this case
	found := false
	cidrIndex := 0
	for i, nodePoolCidr := range r.nodepoolCidrSets[nodepoolName][idx] {
		cidr := nodePoolCidr.ClusterCIDR
		if cidr.Contains(podCIDR.IP.Mask(cidr.Mask)) {
			found = true
			cidrIndex = i
			break
		}
	}
	if !found {
		klog.Warningf("node %s's podCIDR %v is not located in current nodepool %s's CIDR %v", node.Name, podCIDR, nodepoolName, cidr)
		return false, nil
	}
	if err := r.nodepoolCidrSets[nodepoolName][idx][cidrIndex].Occupy(podCIDR); err != nil {
		return false, fmt.Errorf("failed to mark cidr[%v] at idx [%v] as occupied for node: %v in nodepool: %v %v, err: %v",
			podCIDR, idx, node.Name, nodepoolName, r.nodepoolCidrSets[nodepoolName][idx][cidrIndex].ClusterCIDR.String(), err)
	}
	klog.Infof("occupied node cidr %v", podCIDR.String())

	return true, nil
}

// WARNING: If you're adding any return calls or defer any more work from this
// function you have to make sure to update nodesInProcessing properly with the
// disposition of the node when the work is done.
func (r *rangeAllocator) AllocateOrOccupyCIDR(node *v1.Node) error {
	if node == nil {
		return nil
	}
	if !r.insertNodeToProcessing(node.Name) {
		klog.V(2).Infof("Node %v is already in a process of CIDR assignment.", node.Name)
		return nil
	}

	if len(getNodeCIDR(node)) > 0 {
		return r.occupyNodeCIDRs(node)
	}

	// allocate and queue the assignment
	allocated := nodeReservedCIDRs{
		nodeName:       node.Name,
		allocatedCIDRs: make([]*net.IPNet, len(r.clusterCidrSets)),
	}

	nodepool := r.getNodeCurrentPool(node)
	// if node is not labeled with edge worker, while labeled with edge nodepool, ignore
	if yurtnodeutil.IsCloudNode(node) && (nodepool != nil && nodepool.Spec.Type == v1alpha1.Edge) {
		klog.Warningf("skip node %s not labeled with edge worker, while labeled with edge nodepool", node.Name)
		return nil
	}
	// allocate to node directly for:
	//   1. cloud node; 2. edge default nodepool; 3. in cloud nodepool; 4. use global legacy ipam; 5. nodepool use legacy ipam
	if yurtnodeutil.IsCloudNode(node) || (nodepool != nil && yurtnodeutil.IsEdgeDefaultNodepool(nodepool)) ||
		(nodepool != nil && nodepool.Spec.Type == v1alpha1.Cloud) || r.legacyIpam || (nodepool != nil && useLegacyIpam(nodepool)) {
		// allocate directly for cloud node
		for idx := range r.clusterCidrSets {
			klog.Infof("allocate node cidr from cluster cidr at idx:%v: %v", idx, r.clusterCidrSets[idx].ClusterCIDR.String())
			podCIDR, err := r.clusterCidrSets[idx].AllocateNext()
			if err != nil {
				r.removeNodeFromProcessing(node.Name)
				yurtnodeutil.RecordNodeStatusChange(r.recorder, node, "CIDRNotAvailable")
				return fmt.Errorf("failed to allocate cidr from cluster cidr at idx: %v: %v", idx, err)
			}
			allocated.allocatedCIDRs[idx] = podCIDR
		}
	} else {
		// allocate from nodepool for:
		//   1. edge node within edge nodepool, exclude default edge nodepool
		if nodepool == nil {
			r.removeNodeFromProcessing(node.Name)
			yurtnodeutil.RecordNodeStatusChange(r.recorder, node, "NoMatchedNodepool")
			return fmt.Errorf("edge node %s is not labeled with nodepool", node.Name)
		}
		if nodepool.Spec.Type != v1alpha1.Edge {
			r.removeNodeFromProcessing(node.Name)
			yurtnodeutil.RecordNodeStatusChange(r.recorder, node, "UnexpectedNodepoolType")
			return fmt.Errorf("edge node %s nodepool type is unexpected: %v", node.Name, nodepool.Spec.Type)
		}
		nodepoolName := nodepool.Name
		_, exists := r.nodepoolCidrSets[nodepoolName]
		// nodepool cidr is not allocated yet, nodepool worker will retry after it's allocated
		if !exists {
			r.removeNodeFromProcessing(node.Name)
			yurtnodeutil.RecordNodeStatusChange(r.recorder, node, "NodepoolCIDRNotAvailable")
			return fmt.Errorf("nodepool %s cidr is not allocated for node: %s", nodepoolName, node.Name)
		}

		// this should be unreachable, as we initialized nodepool cidrsets beforehand
		if r.nodepoolCidrSets[nodepoolName] == nil {
			r.removeNodeFromProcessing(node.Name)
			yurtnodeutil.RecordNodeStatusChange(r.recorder, node, "NodepoolCIDRNotAvailable")
			return fmt.Errorf("unexpected empty inner CidrSets of nodepool %s", nodepoolName)
		}

		if len(r.nodepoolCidrSets[nodepoolName]) != len(r.clusterCIDRs) {
			r.removeNodeFromProcessing(node.Name)
			yurtnodeutil.RecordNodeStatusChange(r.recorder, node, "NodepoolCIDRNotAvailable")
			return fmt.Errorf("length of nodepool %s cidrs is not equal to cluster cidrs, got: %v", nodepoolName, len(r.nodepoolCidrSets[nodepoolName]))
		}

		for idx := range r.clusterCidrSets {
			allocateSuccess := false
			var err error
			var podCIDR *net.IPNet
			for i := range r.nodepoolCidrSets[nodepoolName][idx] {
				nodepoolCidr := r.nodepoolCidrSets[nodepoolName][idx][i]
				klog.Infof("allocate node cidr from nodepool cidr at idx:%v,%v: %v", idx, i, nodepoolCidr.ClusterCIDR.String())
				podCIDR, err = nodepoolCidr.AllocateNext()
				if err != nil {
					if err == cidrset.ErrCIDRRangeNoCIDRsRemaining {
						klog.Infof("nodepoolcidr %s has no range left, will try next", nodepoolCidr.ClusterCIDR.String())
						continue
					} else {
						r.removeNodeFromProcessing(node.Name)
						yurtnodeutil.RecordNodeStatusChange(r.recorder, node, "CIDRNotAvailable")
						return fmt.Errorf("failed to allocate node cidr from cluster cidr at idx:%v,%v: %v", idx, i, err)
					}
				}
				klog.Infof("allocated node %s cidr: %v", node.Name, podCIDR.String())
				allocateSuccess = true
				allocated.allocatedCIDRs[idx] = podCIDR
				break
			}
			if !allocateSuccess {
				r.removeNodeFromProcessing(node.Name)
				yurtnodeutil.RecordNodeStatusChange(r.recorder, node, "CIDRNotAvailable")
				return fmt.Errorf("failed to allocate node cidr from cluster cidr at idx:%v: %v", idx, err)
			}
		}
	}

	//queue the assignment
	klog.V(4).Infof("Putting node %s with CIDR %v into the work queue", node.Name, allocated.allocatedCIDRs)
	r.nodeCIDRUpdateChannel <- allocated
	return nil
}

// ReleaseCIDR marks node.podCIDRs[...] as unused in our tracked clusterCidrSets
func (r *rangeAllocator) ReleaseCIDR(node *v1.Node) error {
	nodeCidr := getNodeCIDR(node)
	if len(nodeCidr) == 0 {
		return nil
	}

	nodepool := r.getNodeCurrentPool(node)
	// for cloud node or nodes in default edge nodepool, release directly
	if yurtnodeutil.IsCloudNode(node) || (nodepool != nil && yurtnodeutil.IsEdgeDefaultNodepool(nodepool)) ||
		(nodepool != nil && nodepool.Spec.Type == v1alpha1.Cloud) || r.legacyIpam || (nodepool != nil && useLegacyIpam(nodepool)) {
		for idx, cidr := range nodeCidr {
			_, podCIDR, err := net.ParseCIDR(cidr)
			if err != nil {
				return fmt.Errorf("failed to parse CIDR %s on Node %v: %v", cidr, node.Name, err)
			}

			klog.V(4).Infof("release CIDR %s for node:%v", cidr, node.Name)
			if err = r.clusterCidrSets[idx].Release(podCIDR); err != nil {
				return fmt.Errorf("error when releasing CIDR %v: %v", cidr, err)
			}
		}
	} else {
		if nodepool == nil {
			return fmt.Errorf("node %s is not labeled with nodepool", node.Name)
		}
		nodepoolName := nodepool.Name
		// this should be unreachable, as we initialized nodepool cidrsets beforehand
		if len(r.nodepoolCidrSets[nodepoolName]) == 0 {
			return fmt.Errorf("unexpected empty inner CidrSets of nodepool %s", nodepoolName)
		}
		if len(r.nodepoolCidrSets[nodepoolName]) != len(r.clusterCIDRs) {
			return fmt.Errorf("unexpected mismatched length of nodepool %s cidrs and cluster cidrs, got: %v", nodepoolName, len(r.nodepoolCidrSets[nodepoolName]))
		}

		for idx, cidr := range nodeCidr {
			_, podCIDR, err := net.ParseCIDR(cidr)
			if err != nil {
				return fmt.Errorf("failed to parse CIDR %s on Node %v: %v", cidr, node.Name, err)
			}

			klog.V(4).Infof("release CIDR %s for node:%v", cidr, node.Name)
			found := false
			cidrIndex := 0
			for i, nodePoolCidr := range r.nodepoolCidrSets[nodepoolName][idx] {
				cidr := nodePoolCidr.ClusterCIDR
				if cidr.Contains(podCIDR.IP.Mask(cidr.Mask)) {
					found = true
					cidrIndex = i
					break
				}
			}
			if !found {
				klog.Warningf("node %s's podCIDR %v is not located in current nodepool %s's CIDR %v", node.Name, podCIDR, nodepoolName, cidr)
				continue
			}
			if err := r.nodepoolCidrSets[nodepoolName][idx][cidrIndex].Release(podCIDR); err != nil {
				return fmt.Errorf("error when releasing CIDR %v in nodepool %v: %v", cidr, nodepoolName, err)
			}
			klog.Infof("released node %s cidr: %s", node.Name, podCIDR.String())
		}

		// trigger nodepool release, ignore error
		// it's safe as nodepool cidr will only be released when it's nodes are all deleted
		go func() {
			nodepool, err := r.nodepoolLister.Get(nodepoolName)
			if err != nil {
				klog.Errorf("failed to get nodepool %s from lister: %v", nodepoolName, err)
			} else {
				r.ReleaseNodepoolCIDR(nodepool)
			}
		}()
	}

	return nil
}

// Marks all CIDRs with subNetMaskSize that belongs to serviceCIDR as used across all cidrs
// so that they won't be assignable.
func (r *rangeAllocator) filterOutServiceRange(serviceCIDR *net.IPNet) {
	// Checks if service CIDR has a nonempty intersection with cluster
	// CIDR. It is the case if either clusterCIDR contains serviceCIDR with
	// clusterCIDR's Mask applied (this means that clusterCIDR contains
	// serviceCIDR) or vice versa (which means that serviceCIDR contains
	// clusterCIDR).
	for idx, cidr := range r.clusterCIDRs {
		// if they don't overlap then ignore the filtering
		if !cidr.Contains(serviceCIDR.IP.Mask(cidr.Mask)) && !serviceCIDR.Contains(cidr.IP.Mask(serviceCIDR.Mask)) {
			continue
		}

		// at this point, len(cidrSet) == len(clusterCidr)
		if err := r.clusterCidrSets[idx].Occupy(serviceCIDR); err != nil {
			klog.Errorf("Error filtering out service cidr out cluster cidr:%v (index:%v) %v: %v", cidr, idx, serviceCIDR, err)
		}
	}
}

// updateCIDRsAllocation assigns CIDR to Node and sends an update to the API server.
func (r *rangeAllocator) updateCIDRsAllocation(data nodeReservedCIDRs) error {
	var err error
	var node *v1.Node
	defer r.removeNodeFromProcessing(data.nodeName)
	cidrsString := cidrsAsString(data.allocatedCIDRs)
	node, err = r.nodeLister.Get(data.nodeName)
	if err != nil {
		klog.Errorf("Failed while getting node %v for updating Node.Spec.PodCIDRs: %v", data.nodeName, err)
		return err
	}

	// if cidr list matches the proposed.
	// then we possibly updated this node
	// and just failed to ack the success.
	nodeCidr := getNodeCIDR(node)
	if len(nodeCidr) == len(data.allocatedCIDRs) {
		match := true
		for idx, cidr := range data.allocatedCIDRs {
			if nodeCidr[idx] != cidr.String() {
				match = false
				break
			}
		}
		if match {
			klog.V(4).Infof("Node %v already has allocated CIDR %v. It matches the proposed one.", node.Name, data.allocatedCIDRs)
			return nil
		}
	}

	// node has cidrs, release the reserved
	if len(nodeCidr) != 0 {
		klog.Errorf("Node %v already has a CIDR allocated %v. Releasing the new one.", node.Name, nodeCidr)
		nodeCopy := node.DeepCopy()
		nodeCopy.Spec.PodCIDRs = []string{}
		for _, cidr := range data.allocatedCIDRs {
			nodeCopy.Spec.PodCIDRs = append(nodeCopy.Spec.PodCIDRs, cidr.String())
		}
		// reuse the ReleaseCIDR method
		if releaseErr := r.ReleaseCIDR(nodeCopy); releaseErr != nil {
			klog.Errorf("Error when releasing CIDR for node %s, err:%v", nodeCopy.Name, releaseErr)
		}
		return nil
	}

	// If we reached here, it means that the node has no CIDR currently assigned. So we set it.
	for i := 0; i < cidrUpdateRetries; i++ {
		if err = nodeutil.PatchNodeCIDRs(r.client, types.NodeName(node.Name), cidrsString); err == nil {
			klog.Infof("Set node %v PodCIDR to %v", node.Name, cidrsString)
			return nil
		}
	}
	// failed release back to the pool
	klog.Errorf("Failed to update node %v PodCIDR to %v after multiple attempts: %v", node.Name, cidrsString, err)
	yurtnodeutil.RecordNodeStatusChange(r.recorder, node, "CIDRAssignmentFailed")
	// We accept the fact that we may leak CIDRs here. This is safer than releasing
	// them in case when we don't know if request went through.
	// NodeController restart will return all falsely allocated CIDRs to the pool.
	if !apierrors.IsServerTimeout(err) {
		klog.Errorf("CIDR assignment for node %v failed: %v. Releasing allocated CIDR", node.Name, err)
		nodeCopy := node.DeepCopy()
		nodeCopy.Spec.PodCIDRs = []string{}
		for _, cidr := range data.allocatedCIDRs {
			nodeCopy.Spec.PodCIDRs = append(nodeCopy.Spec.PodCIDRs, cidr.String())
		}
		// reuse the ReleaseCIDR method
		if releaseErr := r.ReleaseCIDR(nodeCopy); releaseErr != nil {
			klog.Errorf("Error when releasing CIDR for node %s, err:%v", nodeCopy.Name, releaseErr)
		}
	}
	return err
}

// converts a slice of cidrs into <c-1>,<c-2>,<c-n>
func cidrsAsString(inCIDRs []*net.IPNet) []string {
	outCIDRs := make([]string, len(inCIDRs))
	for idx, inCIDR := range inCIDRs {
		outCIDRs[idx] = inCIDR.String()
	}
	return outCIDRs
}

func getNodeCIDR(node *v1.Node) []string {
	if node == nil {
		return nil
	}
	if len(node.Spec.PodCIDRs) != 0 {
		return node.Spec.PodCIDRs
	}
	if node.Spec.PodCIDR != "" {
		return []string{node.Spec.PodCIDR}
	}
	return nil
}

// legacy ipam: allocate directly to node, skip nodepool level allocation
// Note: the "legacyIpam" flag is global setting, while the annotation is for specific nodepool
func useLegacyIpam(nodepool *v1alpha1.NodePool) bool {
	if nodepool.Annotations == nil {
		return false
	}
	if nodepool.Annotations[util.NodePoolLegacyIpamAnnotation] == "true" {
		return true
	}
	return false
}
