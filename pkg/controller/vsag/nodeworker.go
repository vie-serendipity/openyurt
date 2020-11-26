package vsag

import (
	"fmt"
	"net"
	"time"

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/klog/v2"
)

func (vc *Controller) nodeWorker() {
	for func() bool {
		key, quit := vc.nodeWorkQueue.Get()
		if quit {
			return false
		}
		defer vc.nodeWorkQueue.Done(key)

		err := vc.syncNode(key.(string))
		if err == nil {
			vc.nodeWorkQueue.Forget(key)
			return true
		}

		utilruntime.HandleError(fmt.Errorf("error processing node %v (will retry): %v", key, err))
		// wait for 2m to avoid api rate limit
		vc.nodeWorkQueue.AddAfter(key, 2*time.Minute)
		return true
	}() {
	}
}

func (vc *Controller) syncNode(key string) error {
	if !vc.isVsagEnabled || vc.cenID == "" {
		// don't need to publish cen route if no vsag is used
		// nodepool worker will enqueue node if vsag configured
		klog.V(5).Info("vsag is not enabled, skip syncing node")
		return nil
	}

	if v, ok := vc.cloudNodeCache[key]; ok && v {
		// node already processed
		klog.V(5).Infof("node %s is already processed", key)
		return nil
	}

	node, err := vc.nodeLister.Get(key)
	if apierrors.IsNotFound(err) {
		klog.Infof("node has been deleted %v", key)
		delete(vc.cloudNodeCache, key)
		return nil
	}
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("unable to retrieve node %v from store: %v", key, err))
		return err
	}

	// only vpc/cen client is used here
	cloudClient, err := vc.cloudClientMgr.GetClient(vc.uID, sagDefaultRegion)
	if err != nil {
		return err
	}
	routeTables, err := cloudClient.VpcRoleClient().GetRouteTables(vc.vpcID)
	if err != nil {
		return err
	}

	found := false
	publishedTable := ""
	nodeCidr := vc.getNodeCIDR(node)
	for _, table := range routeTables {
		tableRoutes, err := cloudClient.VpcRoleClient().GetRouteTableRoutes(table)
		if err != nil {
			return err
		}

		for _, tableRoute := range tableRoutes {
			if nodeCidr == tableRoute {
				found = true
				publishedTable = table
				break
			}
		}
		if found {
			break
		}
	}

	if !found {
		return fmt.Errorf("node %s cidr %s is not synced to vpc yet", key, nodeCidr)
	}

	if err := cloudClient.CenRoleClient().PublishRoutes(vc.cenID, vc.vpcID, publishedTable, []string{nodeCidr}); err != nil {
		return err
	}

	vc.setNodeCache(key, true)

	return nil
}

func (vc *Controller) setNodeCache(key string, synced bool) {
	vc.nodeCacheLock.Lock()
	// add to cache
	vc.cloudNodeCache[key] = synced
	vc.nodeCacheLock.Unlock()
}

func (vc *Controller) getNodeCIDR(node *v1.Node) string {
	if node.Spec.PodCIDR != "" {
		return node.Spec.PodCIDR
	}
	for _, cidr := range node.Spec.PodCIDRs {
		ip, _, err := net.ParseCIDR(cidr)
		if err != nil {
			klog.Errorf("node cidr %s is invalid: %v", cidr, err)
			continue
		}
		// only ipv4 is supported now
		if ip.To4() != nil {
			return cidr
		}
	}
	return ""
}
