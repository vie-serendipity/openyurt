package ipam

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"gomodules.xyz/jsonpatch/v2"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/controller/nodeipam/ipam/cidrset"
	"github.com/openyurtio/openyurt/pkg/controller/util"
	utilerrors "github.com/openyurtio/openyurt/pkg/controller/util/errors"
	nodeutil "github.com/openyurtio/openyurt/pkg/controller/util/node"
	"github.com/openyurtio/yurt-app-manager-api/pkg/yurtappmanager/apis/apps/v1alpha1"
)

func (r *rangeAllocator) nodepoolUpdateWorker(stopChan <-chan struct{}) {
	for {
		select {
		case workItem, ok := <-r.nodepoolUpdateChannel:
			if !ok {
				klog.Warning("Channel nodepoolUpdateChannel was unexpectedly closed")
				return
			}
			if err := r.updateNodepoolCIDRsAllocation(workItem); err != nil {
				// Requeue the failed nodepool for update again.
				// TODO: retry with ratelimit
				//r.nodepoolUpdateChannel <- workItem
				klog.Errorf("updateNodepoolCIDRsAllocation failed: %v", err)
			}
		case <-stopChan:
			return
		}
	}
}

func (r *rangeAllocator) insertNodepoolToProcessing(nodepoolName string) bool {
	r.nodepoolsLock.Lock()
	defer r.nodepoolsLock.Unlock()
	if r.nodepoolsInProcessing.Has(nodepoolName) {
		return false
	}
	r.nodepoolsInProcessing.Insert(nodepoolName)
	return true
}

func (r *rangeAllocator) removeNodepoolFromProcessing(nodepoolName string) {
	r.nodepoolsLock.Lock()
	defer r.nodepoolsLock.Unlock()
	r.nodepoolsInProcessing.Delete(nodepoolName)
}

// should be called before occupyNodeCIDRs
func (r *rangeAllocator) occupyNodePoolCIDRs(nodepool *v1alpha1.NodePool) error {
	defer r.removeNodepoolFromProcessing(nodepool.Name)
	for idx, cidrStr := range strings.Split(nodepool.Annotations[util.NodePoolPodCIDRAnnotation], ";") {
		cidrs := strings.Split(cidrStr, ",")
		for _, cidr := range cidrs {
			_, nodepoolCIDR, err := net.ParseCIDR(cidr)
			if err != nil {
				return fmt.Errorf("failed to parse nodepool %s, CIDR %s", nodepool.Name, cidr)
			}

			if err := r.clusterCidrSets[idx].Occupy(nodepoolCIDR); err != nil {
				return fmt.Errorf("failed to mark cidr[%v] at idx [%v] as occupied for nodepool: %v : %v", nodepoolCIDR, idx, nodepool.Name, err)
			}

			// initialize nodepool cidrset
			cidrSet, err := cidrset.NewCIDRSet(nodepoolCIDR, r.nodeSubNetMaskSize)
			if err != nil {
				return fmt.Errorf("failed to occupy cidrset for nodepool %s: %v", nodepool.Name, err)
			}
			if r.nodepoolCidrSets[nodepool.Name] == nil {
				r.nodepoolCidrSets[nodepool.Name] = make([][]*cidrset.CidrSet, len(r.clusterCIDRs))
			}
			if r.nodepoolCidrSets[nodepool.Name][idx] == nil {
				r.nodepoolCidrSets[nodepool.Name][idx] = []*cidrset.CidrSet{}
			}

			exists := false
			for _, elem := range r.nodepoolCidrSets[nodepool.Name][idx] {
				if elem.ClusterCIDR.String() == nodepoolCIDR.String() {
					exists = true
					break
				}
			}
			if !exists {
				r.nodepoolCidrSets[nodepool.Name][idx] = append(r.nodepoolCidrSets[nodepool.Name][idx], cidrSet)
			}
			klog.Infof("occupied nodepoolCidrSets: %v, %v", nodepool.Name, cidrSet.ClusterCIDR.String())
		}
	}

	return nil
}

// validate nodepool annotation
func (r *rangeAllocator) isValidEdgeNodepool(nodepool *v1alpha1.NodePool) bool {
	if nodepool == nil || nodepool.Annotations == nil {
		return false
	}

	if nodepool.Annotations[util.NodePoolMaxNodesAnnotation] == "" {
		klog.V(4).Infof("skip nodepool %s with empty annotation", nodepool.Name)
		return false
	}

	return true
}

// pre-check cidr allocation
func (r *rangeAllocator) AssumeNodepoolCIDR(nodepool *v1alpha1.NodePool) error {
	// TODO: check other nodepool type
	if nodepool.Spec.Type == v1alpha1.Cloud {
		return nil
	}

	if !r.isValidEdgeNodepool(nodepool) {
		// TODO: should block block or set inner default value?
		return fmt.Errorf("nodepool %s is not valid", nodepool.Name)
	}

	if nodepool.DeletionTimestamp != nil {
		return nil
	}

	maxNodesStr := nodepool.Annotations[util.NodePoolMaxNodesAnnotation]
	maxNodes, err := strconv.Atoi(maxNodesStr)
	if err != nil {
		return fmt.Errorf("malformed nodepool %s's annotation: %s, err: %v", nodepool.Name, maxNodesStr, err)
	}

	for idx := range r.clusterCidrSets {
		klog.Infof("allocate nodepool cidr from cluster cidr at idx:%v: %v", idx, r.clusterCidrSets[idx].ClusterCIDR.String())
		capacity := r.clusterCidrSets[idx].GetCapacity()
		if capacity < maxNodes {
			// TODO: also check if allocated cidrs length < 30(sag offline route limit)
			return fmt.Errorf("cluster cidr capacity not enough, left: %v, request: %v", capacity, maxNodes)
		}
	}

	return nil
}

func (r *rangeAllocator) AllocateOrOccupyNodepoolCIDR(nodepool *v1alpha1.NodePool) error {
	// skip cloud nodepool, only process edge nodepools
	// only process edge nodepool
	if nodepool.Spec.Type != v1alpha1.Edge {
		return nil
	}

	// skip default nodepool
	if nodeutil.IsEdgeDefaultNodepool(nodepool) {
		return nil
	}

	if useLegacyIpam(nodepool) {
		return nil
	}

	if !r.isValidEdgeNodepool(nodepool) {
		return fmt.Errorf("nodepool %s is not valid", nodepool.Name)
	}

	if nodepool.DeletionTimestamp != nil {
		return r.ReleaseNodepoolCIDR(nodepool)
	}

	maxNodesStr := nodepool.Annotations[util.NodePoolMaxNodesAnnotation]
	maxNodes, err := strconv.Atoi(maxNodesStr)
	if err != nil {
		return fmt.Errorf("malformed nodepool %s's annotation: %s, err: %v", nodepool.Name, maxNodesStr, err)
	}
	if !r.insertNodepoolToProcessing(nodepool.Name) {
		klog.V(2).Infof("Nodepool %v is already in a process of CIDR assignment.", nodepool.Name)
		return nil
	}
	// skip for already allocated nodepool
	if nodepool.Annotations != nil && nodepool.Annotations[util.NodePoolPodCIDRAnnotation] != "" {
		// corner case: force delete nodepool, while node still exists, then recreate the nodepool with same cidr,
		// we need to occupy the node's cidrs
		if err := r.occupyNodePoolCIDRs(nodepool); err != nil {
			return err
		}
		nodes, _ := r.getNodesForNodepool(nodepool)
		if len(nodes) > 0 {
			for _, node := range nodes {
				if err := r.occupyNodeCIDRs(node); err != nil {
					return err
				}
			}
		}
		return nil
	}

	allocated := nodepoolReservedCIDRs{
		nodepool:       nodepool,
		allocatedCIDRs: make([][]*net.IPNet, len(r.clusterCidrSets)),
	}

	for idx := range r.clusterCidrSets {
		klog.Infof("allocate nodepool cidr from cluster cidr at idx:%v: %v", idx, r.clusterCidrSets[idx].ClusterCIDR.String())
		nodepoolCIDRs, allocateErr := r.clusterCidrSets[idx].AllocateNextN(maxNodes)
		if allocateErr != nil {
			r.removeNodepoolFromProcessing(nodepool.Name)
			err := fmt.Errorf("failed to allocate nodepool cidr from cluster cidr at idx:%v: %v", idx, allocateErr)
			r.recorder.Eventf(nodepool, v1.EventTypeWarning, "CIDRNotAvailable", err.Error())
			return err
		}
		for _, nodepoolCIDR := range nodepoolCIDRs {
			var nodepoolCidrSet *cidrset.CidrSet
			// Note: cidr mask is not in ipv4 length for AllocateNextN func. use below trick to trim the mask.
			_, nodepoolCIDR, _ = net.ParseCIDR(nodepoolCIDR.String())
			nodepoolCidrSet, allocateErr = cidrset.NewCIDRSet(nodepoolCIDR, r.nodeSubNetMaskSize)
			if allocateErr != nil {
				r.removeNodepoolFromProcessing(nodepool.Name)
				err := fmt.Errorf("failed to allocate nodepool cidr from cluster cidr at idx:%v: %v", idx, allocateErr)
				r.recorder.Eventf(nodepool, v1.EventTypeWarning, "CIDRNotAvailable", err.Error())
				break
			}
			if r.nodepoolCidrSets[nodepool.Name] == nil {
				r.nodepoolCidrSets[nodepool.Name] = make([][]*cidrset.CidrSet, len(r.clusterCIDRs))
			}
			r.nodepoolCidrSets[nodepool.Name][idx] = append(r.nodepoolCidrSets[nodepool.Name][idx], nodepoolCidrSet)
			klog.Infof("allocated nodepool %s cidr: %v", nodepool.Name, nodepoolCIDR.String())
		}
		if allocateErr != nil {
			for idx, nodepoolCIDR := range nodepoolCIDRs {
				klog.Infof("release nodepool cidr from cluster cidr at idx:%v: %v", idx, nodepoolCIDR.String())
				if err := r.clusterCidrSets[idx].Release(nodepoolCIDR); err != nil {
					klog.Errorf("failed to release nodepool %s cidr %s during allocation failure: %v", nodepool.Name, nodepoolCIDR.String(), err)
				}
			}
			delete(r.nodepoolCidrSets, nodepool.Name)
			return allocateErr
		}
		allocated.allocatedCIDRs[idx] = nodepoolCIDRs
	}

	// processing nodes after nodepool allocation
	go func() {
		nodes, err := r.getNodesForNodepool(nodepool)
		if err != nil {
			r.removeNodepoolFromProcessing(nodepool.Name)
			errMsg := fmt.Sprintf("failed to list nodes: %v", err)
			r.recorder.Eventf(nodepool, v1.EventTypeWarning, "ListNodeFailed", errMsg)
			return
		}

		for _, node := range nodes {
			if err := r.AllocateOrOccupyCIDR(node); err != nil {
				r.removeNodepoolFromProcessing(nodepool.Name)
				errMsg := fmt.Sprintf("failed to allocate node %s's cidr: %v", node.Name, err)
				r.recorder.Eventf(nodepool, v1.EventTypeWarning, "AllocateNodeCidrFailed", errMsg)
				return
			}
		}
	}()

	klog.V(4).Infof("Putting nodepool %s with CIDR %v into the work queue", nodepool.Name, allocated.allocatedCIDRs)
	r.nodepoolUpdateChannel <- allocated
	return nil
}

func (r *rangeAllocator) ReleaseNodepoolCIDR(nodepool *v1alpha1.NodePool) error {
	if nodepool == nil || nodepool.Annotations == nil || nodepool.Annotations[util.NodePoolPodCIDRAnnotation] == "" {
		return nil
	}

	if nodepool.DeletionTimestamp == nil {
		return nil
	}

	nodes, err := r.getNodesForNodepool(nodepool)
	if err != nil {
		return err
	}
	// we must ensure node cidr is released first
	if len(nodes) > 0 {
		klog.Warningf("cannot release nodepool %s's CIDR as there are still nodes left, count: %v", nodepool.Name, len(nodes))
		return nil
	}

	mergedErr := utilerrors.Errors{}
	for idx, cidrStr := range strings.Split(nodepool.Annotations[util.NodePoolPodCIDRAnnotation], ";") {
		cidrs := strings.Split(cidrStr, ",")
		for _, cidr := range cidrs {
			_, nodepoolCIDR, err := net.ParseCIDR(cidr)
			if err != nil {
				mergedErr.Add(fmt.Errorf("failed to parse nodepool %s, CIDR %s", nodepool.Name, cidr))
				continue
			}

			klog.V(4).Infof("release CIDR %s for nodepool: %v", cidr, nodepool.Name)
			if err := r.clusterCidrSets[idx].Release(nodepoolCIDR); err != nil {
				mergedErr.Add(fmt.Errorf("error when releasing CIDR %v in nodepool %v: %v", cidr, nodepool.Name, err))
				continue
			}
			klog.Infof("released nodepool %s cidr: %s", nodepool.Name, nodepoolCIDR.String())
		}
	}
	delete(r.nodepoolCidrSets, nodepool.Name)

	if mergedErr.Err() != nil {
		return mergedErr.Err()
	}

	allocated := nodepoolReservedCIDRs{
		nodepool:  nodepool,
		isDeleted: true,
	}
	// remove finalizer in the end
	r.nodepoolUpdateChannel <- allocated
	return nil
}

// update allocated cidrs in nodepool annotation and add/remove finalizer
func (r *rangeAllocator) updateNodepoolCIDRsAllocation(data nodepoolReservedCIDRs) error {
	nodepool := data.nodepool
	defer r.removeNodepoolFromProcessing(nodepool.Name)

	if data.isDeleted {
		return r.removeNodepool(nodepool.Name)
	}

	return r.patchNodepoolCidrs(nodepool.Name, data.allocatedCIDRs)
}

func (r *rangeAllocator) patchNodepoolCidrs(nodepoolName string, allocatedCIDRs [][]*net.IPNet) error {
	var allocateErr error
	waitErr := wait.PollImmediate(2*time.Second, 4*time.Second, func() (bool, error) {
		original, err := r.appsClient.AppsV1alpha1().NodePools().Get(context.TODO(), nodepoolName, metav1.GetOptions{})
		if err != nil {
			klog.Errorf("get nodepool %s error: %v", nodepoolName, err)
			if apierrors.IsNotFound(err) {
				return false, fmt.Errorf("nodepool %s not found during update cidrs", nodepoolName)
			}
			return false, nil
		}
		if original != nil && original.Annotations[util.NodePoolPodCIDRAnnotation] != "" {
			// nodepool already allocated
			msg := fmt.Sprintf("Nodepool %v already has a CIDR allocated %v. Releasing the new one.", nodepoolName, original.Annotations[util.NodePoolPodCIDRAnnotation])
			klog.Error(msg)
			allocateErr = errors.New(msg)
			return true, nil
		}

		var originalData, newData []byte
		originalData, err = json.Marshal(original)
		if err != nil {
			klog.Errorf("marshal nodepool %s err: %v", nodepoolName, err)
			return false, nil
		}
		current := original.DeepCopy()
		if current.Finalizers == nil {
			current.Finalizers = []string{}
		}
		if !strings.Contains(strings.Join(current.Finalizers, " "), util.NodePoolCIDRFinalizer) {
			current.Finalizers = append(current.Finalizers, util.NodePoolCIDRFinalizer)
		}

		cidrStr := nodepoolCidrsAsString(allocatedCIDRs)
		if current.Annotations == nil {
			current.Annotations = make(map[string]string)
		}
		current.Annotations[util.NodePoolPodCIDRAnnotation] = cidrStr

		newData, err = json.Marshal(current)
		if err != nil {
			klog.Errorf("marshal nodepool %s err: %v", nodepoolName, err)
			return false, nil
		}
		patch, err := jsonpatch.CreatePatch(originalData, newData)
		if err != nil {
			klog.Errorf("create merge patch for nodepool %s err: %v", nodepoolName, err)
			return false, nil
		}
		patchBytes, err := json.Marshal(patch)
		if err != nil {
			klog.Errorf("create merge patch for nodepool %s err: %v", nodepoolName, err)
			return false, nil
		}
		klog.V(4).Infof("patch bytes of nodepool %s: %v", nodepoolName, string(patchBytes))
		_, allocateErr = r.appsClient.AppsV1alpha1().NodePools().Patch(context.TODO(), nodepoolName, types.JSONPatchType, patchBytes, metav1.PatchOptions{})
		if allocateErr != nil {
			klog.Errorf("update nodepool %s cidrs error: %v", nodepoolName, allocateErr)
			if apierrors.IsNotFound(allocateErr) {
				return false, fmt.Errorf("nodepool %s not found during update cidrs", nodepoolName)
			}
			return false, nil
		}
		return true, nil
	})

	if allocateErr != nil {
		// only handle for non server timeout error during patch
		if !apierrors.IsServerTimeout(allocateErr) {
			// recycle nodepool cidr if update failed
			for idx, nodepoolCIDRs := range allocatedCIDRs {
				for _, nodepoolCIDR := range nodepoolCIDRs {
					if err := r.clusterCidrSets[idx].Release(nodepoolCIDR); err != nil {
						klog.Errorf("failed to release nodepool %s cidr %s during allocation patch failure", nodepoolName, nodepoolCIDR.String())
						continue
					}
					klog.Infof("released nodepool %s cidr: %s during allocation patch failure", nodepoolName, nodepoolCIDR.String())
				}
			}
			delete(r.nodepoolCidrSets, nodepoolName)
		}
		return allocateErr
	}
	return waitErr
}

func (r *rangeAllocator) removeNodepool(nodepoolName string) error {
	waitErr := wait.PollImmediate(2*time.Second, 4*time.Second, func() (bool, error) {
		nodepool, err := r.appsClient.AppsV1alpha1().NodePools().Get(context.TODO(), nodepoolName, metav1.GetOptions{})
		if err != nil {
			klog.Errorf("get nodepool %s error: %v", nodepoolName, err)
			if apierrors.IsNotFound(err) {
				return true, nil
			}
			return false, nil
		}
		if nodepool.DeletionTimestamp == nil {
			// nodepool is not deleted, skip
			return true, nil
		}
		patch := []byte(fmt.Sprintf(`[{"op":"remove","path":"/metadata/finalizers","value":["%s"]}]`, util.NodePoolCIDRFinalizer))
		_, err = r.appsClient.AppsV1alpha1().NodePools().Patch(context.TODO(), nodepoolName, types.JSONPatchType, patch, metav1.PatchOptions{})
		if err != nil {
			klog.Infof("patch nodepool %s err: %v", nodepoolName, err)
			if apierrors.IsNotFound(err) {
				return true, nil
			}
			return false, nil
		}
		return true, nil
	})

	return waitErr
}

// get nodepool from node's label, ensure nodepool exists
func (r *rangeAllocator) getNodeCurrentPool(node *v1.Node) *v1alpha1.NodePool {
	if node.Labels == nil || node.Labels[v1alpha1.LabelCurrentNodePool] == "" {
		klog.Infof("node %s is not labeled with nodepool", node.Name)
		return nil
	}
	var nodepool *v1alpha1.NodePool
	var err error
	if r.informerCacheSynced.Load().(bool) {
		nodepool, err = r.nodepoolLister.Get(node.Labels[v1alpha1.LabelCurrentNodePool])
	} else {
		nodepool, err = r.appsClient.AppsV1alpha1().NodePools().Get(context.TODO(), node.Labels[v1alpha1.LabelCurrentNodePool], metav1.GetOptions{})
	}
	if err != nil {
		klog.Errorf("failed to get nodepool for node %s: %v", node.Name, err)
		return nil
	}

	return nodepool
}

func (r *rangeAllocator) getNodesForNodepool(nodepool *v1alpha1.NodePool) ([]*v1.Node, error) {
	if nodepool == nil {
		return nil, nil
	}

	nodes := []*v1.Node{}
	var err error
	if r.informerCacheSynced.Load().(bool) {
		nodes, err = r.nodeLister.List(labels.SelectorFromSet(map[string]string{v1alpha1.LabelCurrentNodePool: nodepool.Name}))
		if err != nil {
			return nil, err
		}
	} else {
		nodeList, err := r.client.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{
			LabelSelector: labels.SelectorFromSet(map[string]string{v1alpha1.LabelCurrentNodePool: nodepool.Name}).String()})
		if err != nil {
			return nil, err
		}
		for _, node := range nodeList.Items {
			nodes = append(nodes, &node)
		}
	}

	return nodes, nil
}

// converts a slice of cidrs into <c-1>,<c-2>;<c-n>
func nodepoolCidrsAsString(inCIDRs [][]*net.IPNet) string {
	outCIDRs := make([]string, len(inCIDRs))
	for idx, inCIDR := range inCIDRs {
		cidrs := []string{}
		for _, cidr := range inCIDR {
			cidrs = append(cidrs, cidr.String())
		}
		outCIDRs[idx] = strings.Join(cidrs, ",")
	}
	return strings.Join(outCIDRs, ";")
}
