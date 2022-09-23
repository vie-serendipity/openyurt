package vsag

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"
	"time"

	jsonpatch "github.com/evanphx/json-patch"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/controller/util"
	utilerrors "github.com/openyurtio/openyurt/pkg/controller/util/errors"
	nodeutil "github.com/openyurtio/openyurt/pkg/controller/util/node"
	"github.com/openyurtio/openyurt/pkg/controller/vsag/cloud"
	appsv1alpha1 "github.com/openyurtio/yurt-app-manager-api/pkg/yurtappmanager/apis/apps/v1alpha1"
)

var ErrorNpAlreadyDeleted = errors.New("nodepool deleted during configuration")

var SagBandWidthRE = regexp.MustCompile(`\d+`)

func (vc *Controller) nodepoolWorker() {
	for func() bool {
		key, quit := vc.npWorkQueue.Get()
		if quit {
			return false
		}
		defer vc.npWorkQueue.Done(key)

		err := vc.syncNodepool(key.(string))
		if err == nil {
			vc.npWorkQueue.Forget(key)
			return true
		}

		utilruntime.HandleError(fmt.Errorf("error processing nodepool %v (will retry): %v", key, err))
		vc.npWorkQueue.AddRateLimited(key)
		return true
	}() {
	}
}

func (vc *Controller) syncNodepool(key string) error {
	ctx := context.TODO()
	np, err := vc.nodepoolLister.Get(key)
	if apierrors.IsNotFound(err) {
		klog.Infof("nodepool has been deleted %v", key)
		if np, ok := vc.nodepoolCache[key]; ok {
			klog.Infof("got deleted nodepool from cache: %v", key)
			return vc.handleNodepoolDeletion(ctx, np)
		}
		return nil
	}
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("unable to retrieve nodepool %v from store: %v", key, err))
		return err
	}

	if np.DeletionTimestamp != nil {
		err = vc.handleNodepoolDeletion(ctx, np)
		if err == nil {
			vc.nodepoolCacheLock.Lock()
			delete(vc.nodepoolCache, key)
			vc.nodepoolCacheLock.Unlock()
		}
		return err
	}

	err = vc.handleNodepoolUpdate(ctx, np)

	// update cache after reconcile no matter update success or not
	vc.nodepoolCacheLock.Lock()
	vc.nodepoolCache[key] = np
	vc.nodepoolCacheLock.Unlock()

	if err != nil {
		return err
	}
	return nil
}

// - delete vsag-helper/vsag appseddeployment
// - delete vsag and remove static route on np deletion
func (vc *Controller) handleNodepoolDeletion(ctx context.Context, np *appsv1alpha1.NodePool) error {
	mergedErr := utilerrors.Errors{}
	ccnRegion, err := vc.getCcnRegion(np)
	if err != nil {
		return err
	}
	cloudClient, err := vc.cloudClientMgr.GetClient(vc.uID, ccnRegion)
	if err != nil {
		return err
	}
	sagId := vc.getSagId(np)
	if sagId == "" {
		klog.V(5).Infof("deleted nodepool %s has no sag id", np.Name)
		return nil
	}
	// remove offline cidrs
	if err := cloudClient.SagResourceClient().RemoveOfflineCIDRs(sagId); err != nil {
		mergedErr.Add(fmt.Errorf("failed to remove offline cidr of sag %s for nodepool %s: %v", sagId, np.Name, err))
	} else {
		klog.Infof("removed offline cidr of sag %s for nodepool %s", sagId, np.Name)
	}

	userCcnId := vc.getCcnId(np)
	if userCcnId == "" {
		mergedErr.Add(fmt.Errorf("empty ccn id from nodepool: %s", np.Name))
	} else {
		// unbind user's ccn to sag
		if err := cloudClient.SagResourceClient().UnBindCCN(sagId, userCcnId); err != nil {
			mergedErr.Add(fmt.Errorf("failed to unbind sag %s from user %s's ccn %s for nodepool %s: %v", sagId, vc.uID, userCcnId, np.Name, err))
		} else {
			klog.Infof("unbound sag %s from user %s's ccn %s for nodepool %s", sagId, vc.uID, userCcnId, np.Name)
		}

		// remove grant sag to user ccn
		if err := cloudClient.SagResourceClient().RevokeCCN(sagId, userCcnId); err != nil {
			mergedErr.Add(fmt.Errorf("failed to revoke sag %s from user %s's ccn %s for nodepool %s: %v", sagId, vc.uID, userCcnId, np.Name, err))
		} else {
			klog.Infof("revoked sag %s from user %s's ccn %s for nodepool %s", sagId, vc.uID, userCcnId, np.Name)
		}

		// Note: sag unbind takes time, we should not delete vsag-core before unbind succeed
		// wait here as a workaround
		klog.Infof("wait for sag unbind")
		time.Sleep(sagUnbindGracePeriod)

		if err := cloudClient.BssClient().CancelAutoRenewal(sagId); err != nil {
			mergedErr.Add(fmt.Errorf("failed to cancel auto-renewal sag %s for nodepool %s,err:%v", sagId, np.Name, err))
		} else {
			klog.Infof("cancel auto-renewal sag %s for nodepool %s success", sagId, np.Name)
		}
	}

	// delete vsag-helper / vsag core
	var apierr error

	vsgHelperLabel := getVsagHelperLabel(np.Name)
	err = wait.PollImmediate(2*time.Second, 4*time.Second, func() (bool, error) {
		apierr = vc.kubeClient.AppsV1().DaemonSets(sagNamespace).DeleteCollection(
			context.TODO(),
			metav1.DeleteOptions{},
			metav1.ListOptions{LabelSelector: labels.SelectorFromSet(vsgHelperLabel).String()})
		if apierr != nil && !apierrors.IsNotFound(apierr) {
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		mergedErr.Add(fmt.Errorf("failed to delete vsag helper for nodepool %s: %v", np.Name, apierr))
	}
	klog.Infof("deleted vsag helper for nodepool %s", np.Name)

	vsgCoreLabel := getVsagCoreLabel(np.Name)
	err = wait.PollImmediate(2*time.Second, 4*time.Second, func() (bool, error) {
		apierr = vc.kubeClient.AppsV1().Deployments(sagNamespace).DeleteCollection(
			context.TODO(),
			metav1.DeleteOptions{},
			metav1.ListOptions{LabelSelector: labels.SelectorFromSet(vsgCoreLabel).String()})
		if apierr != nil && !apierrors.IsNotFound(apierr) {
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		mergedErr.Add(fmt.Errorf("failed to delete vsag core for nodepool %s: %v", np.Name, apierr))
	}
	klog.Infof("deleted vsag core for nodepool %s", np.Name)

	// mark all nodes in nodepool as vsag not managed
	vc.markNodeIsManagedByVsag(ctx, np, "false")

	if err := mergedErr.Err(); err != nil {
		klog.Infof("failed to delete sag config for nodepool %s: %v", np.Name, err)
		vc.recorder.Eventf(np, v1.EventTypeWarning, "DeleteEgwConfigFailed", "failed to delete egw config for nodepool %s: %v", np.Name, err)
		return err
	}

	vc.recorder.Eventf(np, v1.EventTypeNormal, "DeleteEgwConfigSucceed", "deleted egw config for nodepool %s", np.Name)

	// remove finalizer at last step
	if err := vc.removeNodepoolFinalizer(ctx, np); err != nil {
		vc.recorder.Eventf(np, v1.EventTypeWarning, "DeleteEgwConfigFailed", "failed to remove finalizer for nodepool %s: %v", np.Name, err)
		return fmt.Errorf("failed to remove finalizer for nodepool %s: %v", np.Name, err)
	}

	klog.Infof("deleted sag config for nodepool %s", np.Name)
	return nil
}

// - create vcpe if not present, save sn/key in secret
// - grant sag permission to user's ccn
// - bind user's ccn to sag
// - add edge pod cidr to cloud
// - deploy vsag core + sidecar appseddeployment if not present
// - deploy vsag-helper appseddeployment if not present
func (vc *Controller) handleNodepoolUpdate(ctx context.Context, np *appsv1alpha1.NodePool) error {
	ccnRegion, err := vc.getCcnRegion(np)
	if err != nil {
		return err
	}
	cloudClient, err := vc.cloudClientMgr.GetClient(vc.uID, ccnRegion)
	if err != nil {
		return err
	}
	userCcnId := vc.getCcnId(np)
	if userCcnId == "" {
		// skip nodepool don't need sag
		klog.V(4).Infof("empty ccn id from nodepool: %s", np.Name)
		return nil
	}

	edgeNodeHostCIDRBlock, err := vc.getHostCIDRBlock(np)
	if err != nil {
		return fmt.Errorf("failed to get edge node host network segment from nodepool %s: %v", np.Name, err)
	}

	podCidrs, err := vc.getPodCidrs(np)
	if err != nil {
		return fmt.Errorf("failed to get pod cidrs from nodepool %s: %v", np.Name, err)
	}

	sagPeriod, err := vc.getSagPeriod(np)
	if err != nil {
		return fmt.Errorf("failed to get sag period from nodepool %s: %v", np.Name, err)
	}

	sagBandwidth, err := vc.getSagBandwidth(np)
	if err != nil {
		return fmt.Errorf("failed to get sag bandwidth from nodepool %s: %v", np.Name, err)
	}

	cenId := vc.getCenId(np)
	if cenId == "" {
		return fmt.Errorf("empty cen id from nodepool: %s", np.Name)
	}

	vc.nodepoolCacheLock.Lock()
	existNp := vc.nodepoolCache[np.Name]
	vc.nodepoolCacheLock.Unlock()

	existBandwidth, err := vc.getSagBandwidth(existNp)
	if err != nil {
		return fmt.Errorf("failed to get sag bandwidth from exist nodepool %s: %v", existNp.Name, err)
	}
	suspectedBandwidthChange := existBandwidth != sagBandwidth
	klog.Infof("exist nodepool, %v", existNp)
	klog.Infof("exist bandwidth %d, new bandwidth %d for nodepool %s, suspected change: %t", existBandwidth, sagBandwidth, np.Name, suspectedBandwidthChange)

	// vsag is enabled after above check
	once.Do(func() {
		vc.isVsagEnabled = true
		vc.cenID = cenId
	})
	go vc.triggerCloudNodeSync()

	var sagId string
	var updateErr error
	defer func() {
		if updateErr != nil {
			if updateErr == ErrorNpAlreadyDeleted {
				return
			}
			vc.recorder.Eventf(np, v1.EventTypeWarning, "ConfigEgwFailed", "config egw for nodepool %s failed %v", np.Name, updateErr)
			// record error info to nodepool annotation
			status, _ := json.Marshal(util.SagStatus{
				Phase:   "failure",
				EgwID:   sagId,
				CcnID:   userCcnId,
				UID:     vc.uID,
				Message: updateErr.Error(),
			})
			err := vc.patchNodepool(ctx, np, util.NodePoolSagStatusNewAnnotation, string(status), false)
			if err != nil {
				klog.Errorf("failed to set sag status %s to nodepool %s: %v", string(status), np.Name, err)
				vc.recorder.Eventf(np, v1.EventTypeWarning, "SetEgwStatusFailed", "failed to set egw status %s to nodepool %s: %v", string(status), np.Name, err)
			}
		} else {
			vc.recorder.Eventf(np, v1.EventTypeNormal, "ConfigEgwSucceed", "config egw for nodepool %s succeed", np.Name)
			// record error info to nodepool annotation
			status, _ := json.Marshal(util.SagStatus{
				Phase:   "success",
				EgwID:   sagId,
				CcnID:   userCcnId,
				UID:     vc.uID,
				Message: "configure egw succeed",
			})
			err := vc.patchNodepool(ctx, np, util.NodePoolSagStatusNewAnnotation, string(status), true)
			if err != nil {
				klog.Errorf("failed to set sag status %s to nodepool %s: %v", string(status), np.Name, err)
				vc.recorder.Eventf(np, v1.EventTypeWarning, "SetEgwStatusFailed", "failed to set egw status %s to nodepool %s: %v", string(status), np.Name, err)
			}
		}
	}()

	vc.recorder.Event(np, v1.EventTypeNormal, "StartConfigEgw", "start to config egw")
	sagId = vc.getSagId(np)
	sagDesc := vc.clusterRegion + "_" + vc.uID

	if sagId == "" {
		// create vcpe if not present
		// example: edge_c734f871f75d341cbacf6386974c3a075_np700f4e2ffde544a6a0896ad200f85586
		sagName := "edge_" + vc.clusterID + "_" + np.Name
		// sag name length cannot exceed 128
		if len(sagName) > 120 {
			sagName = sagName[0:120]
		}
		sagId, updateErr = cloudClient.BssClient().CreateVsagInstance(sagName, sagDesc, sagPeriod, sagBandwidth)
		if updateErr != nil {
			return fmt.Errorf("failed to create vsag for nodepool %s: %v", np.Name, updateErr)
		}
		klog.Infof("created sag %s for nodepool %s", sagId, np.Name)
		// update sag id in np annotation and finalizer
		if updateErr = vc.patchNodepool(ctx, np, util.NodePoolSagIDNewAnnotation, sagId, true); updateErr != nil {
			// TODO: to avoid sag leak, record sagid info for manual delete sag
			return fmt.Errorf("failed to patch sagid %s to nodepool %s: %v", sagId, np.Name, updateErr)
		}
		// update cache
		np.Annotations[util.NodePoolSagIDNewAnnotation] = sagId
	} else {
		if suspectedBandwidthChange {
			err := vc.handleBandwidthChange(sagId, sagBandwidth, cloudClient)
			if err != nil {
				// bandwidth change failed, update bandwidth annotation of nodepool
				// the nodepool will then be updated into the cache for later retry
				np.Annotations[util.NodePoolSagBandwidthAnnotation] = fmt.Sprintf("%d", existBandwidth)
				return err
			}
		}
	}

	// sleep to avoid SAG.InstanceNoFound error
	time.Sleep(5 * time.Second)
	if updateErr = cloudClient.SagResourceClient().GrantCCN(sagId, userCcnId, vc.uID); updateErr != nil {
		return fmt.Errorf("failed to grant sag %s to ccn %s: %v", sagId, userCcnId, updateErr)
	}
	klog.Infof("granted sag %s to user %s's ccn %s for nodepool %s", sagId, vc.uID, userCcnId, np.Name)

	time.Sleep(5 * time.Second)
	// role play to bind user's ccn to sag
	if updateErr = cloudClient.SagRoleClient().BindCCN(sagId, userCcnId, vc.cloudClientMgr.GetResourceUid()); updateErr != nil {
		return fmt.Errorf("failed to bind sag %s to ccn %s: %v", sagId, userCcnId, updateErr)
	}
	klog.Infof("bound sag %s to user %s's ccn %s for nodepool %s", sagId, vc.uID, userCcnId, np.Name)

	// add edge cidr to sag, execute on sag creation since this annotation can not be modified
	if updateErr = cloudClient.SagResourceClient().AddOfflineCIDRs(sagId, sagDesc, podCidrs, edgeNodeHostCIDRBlock); updateErr != nil {
		return fmt.Errorf("failed to add offline cidrs %v to sag %s: %v", podCidrs, sagId, updateErr)
	}
	klog.Infof("added sag %s offline cidr %v for nodepool %s", sagId, podCidrs, np.Name)

	// get sn/key
	var sagCred []*cloud.SagCredential
	sagCred, updateErr = cloudClient.SagResourceClient().GetCredential(sagId)
	if updateErr != nil {
		return fmt.Errorf("failed to get sag %s credential: %v", sagId, updateErr)
	}
	if len(sagCred) == 0 {
		return fmt.Errorf("no credential for sag %s", sagId)
	}

	err = vc.createOrUpdateVcpeCore(sagCred, np, sagBandwidth, updateErr)
	if err != nil {
		return err
	}
	err = vc.createHelperIfNeeded(np, updateErr)
	if err != nil {
		return err
	}
	// wait for vsag core and helper running before return success
	timeout := 5 * time.Minute
	klog.Infof("waiting upto %v for sag core and helper running in nodepool %s", timeout.String(), np.Name)

	err = vc.waitforPodRunning(np, getVsagCoreLabel(np.Name), timeout)
	if err != nil {
		// might be timeout or nodepool deleted
		klog.Errorf("error waiting for vsag core pods running in nodepool %s: %v", np.Name, err)
		updateErr = err
		return updateErr
	}
	klog.Infof("vsag core running in nodepool %s", np.Name)

	err = vc.waitforPodRunning(np, getVsagHelperLabel(np.Name), timeout)
	if err != nil {
		// might be timeout or nodepool deleted
		klog.Errorf("error waiting for vsag helper pods running in nodepool %s: %v", np.Name, err)
		updateErr = err
		return updateErr
	}
	klog.Infof("vsag helper running in nodepool %s", np.Name)

	// mark all nodes in nodepool as vsag-managed
	// this update will also trigger node sync
	vc.markNodeIsManagedByVsag(ctx, np, "true")
	return nil
}

func (vc *Controller) createOrUpdateVcpeCore(sagCred []*cloud.SagCredential, np *appsv1alpha1.NodePool, sagBandwidth int, updateErr error) error {
	for _, cred := range sagCred {
		// for vcpe, we have two instances for HA, create deployment for each vsag instance
		coreName := strings.ToLower("egw-core-" + np.Name + "-" + string(cred.HaState))
		newDeploy := newVsagDeployment(vc.vsagCoreImage, vc.vsagSidecarImage, cred.Sn, cred.Key, coreName, np, strings.ToLower(string(cred.HaState)), sagBandwidth)
		err := wait.PollImmediate(2*time.Second, 6*time.Second, func() (bool, error) {
			var existDeploy *appsv1.Deployment
			existDeploy, updateErr = vc.kubeClient.AppsV1().Deployments(sagNamespace).Get(context.TODO(), coreName, metav1.GetOptions{})
			// skip create if vsag-core deployment already exists
			if updateErr != nil && apierrors.IsNotFound(updateErr) {
				if _, updateErr = vc.kubeClient.AppsV1().Deployments(sagNamespace).Create(context.TODO(), newDeploy, metav1.CreateOptions{}); updateErr != nil {
					klog.Errorf("failed to create sag deployment: %v", updateErr)
					return false, nil
				}
				return true, nil
			} else {
				if updateErr == nil {
					klog.Warningf("vsag core %s already exists", coreName)
					if existDeploy == nil {
						klog.Errorf("got nil vsag core deployment")
						return false, nil
					}
					if !isTolerationsEqual(existDeploy.Spec.Template.Spec.Tolerations, newDeploy.Spec.Template.Spec.Tolerations) ||
						!resourceRequirementsEquals(existDeploy, newDeploy, ContainerNameCore) {
						klog.Infof("start to recreate vsag core deployment %s as desired spec changed, old spec: %v, new spec: %v",
							newDeploy.Name, existDeploy.Spec.Template.Spec, newDeploy.Spec.Template.Spec)
						// update existing deployment, this might happen when nodepool tolerations modified or core container resource requirement changed
						if updateErr = vc.kubeClient.AppsV1().Deployments(sagNamespace).Delete(context.TODO(), newDeploy.Name, metav1.DeleteOptions{}); updateErr != nil {
							klog.Errorf("failed to delete sag deployment: %v", updateErr)
							return false, nil
						}
						if _, updateErr = vc.kubeClient.AppsV1().Deployments(sagNamespace).Create(context.TODO(), newDeploy, metav1.CreateOptions{}); updateErr != nil {
							klog.Errorf("failed to create sag deployment: %v", updateErr)
							return false, nil
						}
						// TODO: wait for current vsag-core to be running before continue the next
						return true, nil
					}
					return true, nil
				} else {
					klog.Errorf("failed to get vsag core %s: %v", coreName, updateErr)
					return false, nil
				}
			}
		})
		if err != nil {
			return fmt.Errorf("failed to create or update sag deployment in nodepool %s: %v", np.Name, updateErr)
		}
	}
	klog.Infof("deployed sag-core in nodepool %s", np.Name)
	return nil
}

func (vc *Controller) createHelperIfNeeded(np *appsv1alpha1.NodePool, updateErr error) error {
	helperName := "egw-helper-" + np.Name
	helperDs := newVsagHelperDaemonset(vc.vsagHelperImage, helperName, np.Name)
	err := wait.PollImmediate(2*time.Second, 4*time.Second, func() (bool, error) {
		_, updateErr = vc.kubeClient.AppsV1().DaemonSets(sagNamespace).Get(context.TODO(), helperName, metav1.GetOptions{})
		if updateErr != nil && apierrors.IsNotFound(updateErr) {
			_, updateErr = vc.kubeClient.AppsV1().DaemonSets(sagNamespace).Create(context.TODO(), helperDs, metav1.CreateOptions{})
			if updateErr != nil {
				klog.Errorf("failed to create vsag helper daemonset: %v", updateErr)
				return false, nil
			}
			return true, nil
		} else {
			if updateErr == nil {
				klog.Warningf("vsag helper already exists in nodepool %s, skip creating", np.Name)
				return true, nil
			} else {
				klog.Errorf("failed to get vsag helper in nodepool %s: %v", np.Name, updateErr)
				return false, nil
			}
		}
	})
	if err != nil {
		return fmt.Errorf("failed to create vsag helper daemonset in nodepool %s: %v", np.Name, updateErr)
	}
	klog.Infof("deployed sag-helper in nodepool %s", np.Name)
	return nil
}

func (vc *Controller) waitforPodRunning(np *appsv1alpha1.NodePool, podLabels map[string]string, timeout time.Duration) error {
	return wait.PollImmediate(4*time.Second, timeout, func() (bool, error) {
		// if nodepool is deleted, return error directly
		apinp, apierr := vc.nodepoolLister.Get(np.Name)
		if apierr != nil && apierrors.IsNotFound(apierr) {
			klog.Warningf("nodepool %s was already deleted during configuration", np.Name)
			return false, ErrorNpAlreadyDeleted
		}
		if apierr == nil && apinp != nil && apinp.DeletionTimestamp != nil {
			klog.Warningf("nodepool %s was being deleted during configuration", np.Name)
			return false, ErrorNpAlreadyDeleted
		}

		podList, apierr := vc.kubeClient.CoreV1().Pods(sagNamespace).List(context.TODO(), metav1.ListOptions{
			LabelSelector: labels.SelectorFromSet(podLabels).String(),
		})
		if apierr != nil {
			klog.Errorf("failed to list pods: %v", apierr)
			return false, nil
		}
		if len(podList.Items) == 0 {
			klog.Warningf("no pod with label %v found yet", podLabels)
			return false, nil
		}
		for _, pod := range podList.Items {
			if !IsPodReady(&pod) {
				klog.Warningf("pod %s is not ready yet", pod.Namespace+"/"+pod.Name)
				return false, nil
			}
		}
		return true, nil
	})
}

func (vc *Controller) getSagId(np *appsv1alpha1.NodePool) string {
	if np == nil || np.Annotations == nil {
		return ""
	}
	return np.Annotations[util.NodePoolSagIDNewAnnotation]
}

func (vc *Controller) patchNodepool(ctx context.Context, np *appsv1alpha1.NodePool, annKey, annVal string, addFinalizer bool) error {
	var apierr error
	err := wait.PollImmediate(2*time.Second, 4*time.Second, func() (bool, error) {
		apiNp := &appsv1alpha1.NodePool{}
		apiNp, apierr = vc.appsClient.AppsV1alpha1().NodePools().Get(context.TODO(), np.Name, metav1.GetOptions{})
		if apierr != nil {
			klog.Errorf("failed to get nodepool %s: %v", np.Name, apierr)
			return false, nil
		}

		newNp := apiNp.DeepCopy()
		newNp.Annotations[annKey] = annVal
		if addFinalizer {
			if len(newNp.Finalizers) == 0 {
				newNp.Finalizers = []string{util.NodePoolSagConfiguredNewFinalizer}
			} else {
				found := false
				for _, f := range newNp.Finalizers {
					if f == util.NodePoolSagConfiguredNewFinalizer {
						found = true
						break
					}
				}
				if !found {
					newNp.Finalizers = append(newNp.Finalizers, util.NodePoolSagConfiguredNewFinalizer)
				}
			}
		}

		data, err := vc.generatePatch(apiNp, newNp)
		if err != nil {
			return false, err
		}
		klog.Infof("patch bytes for nodepool %s: %v", np.Name, string(data))
		if data == nil {
			return true, nil
		}

		_, apierr = vc.appsClient.AppsV1alpha1().NodePools().Patch(context.TODO(), np.Name, types.MergePatchType, data, metav1.PatchOptions{})
		if apierr != nil {
			klog.Errorf("failed to patch nodepool %s: %v", newNp.Name, apierr)
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		return fmt.Errorf("failed to patch nodepool %s, apierr: %v, err: %v", np.Name, apierr, err)
	}
	return nil
}

func (vc *Controller) removeNodepoolFinalizer(ctx context.Context, np *appsv1alpha1.NodePool) error {
	var apierr error
	err := wait.PollImmediate(2*time.Second, 4*time.Second, func() (bool, error) {
		apiNp := &appsv1alpha1.NodePool{}
		apiNp, apierr = vc.appsClient.AppsV1alpha1().NodePools().Get(context.TODO(), np.Name, metav1.GetOptions{})
		if apierr != nil {
			if apierrors.IsNotFound(apierr) {
				return true, nil
			}
			klog.Errorf("failed to get nodepool %s: %v", np.Name, apierr)
			return false, nil
		}

		newNp := apiNp.DeepCopy()
		if len(newNp.Finalizers) != 0 {
			newFinalizer := []string{}
			for i := range newNp.Finalizers {
				if newNp.Finalizers[i] != util.NodePoolSagConfiguredNewFinalizer {
					newFinalizer = append(newFinalizer, newNp.Finalizers[i])
				}
			}
			newNp.Finalizers = newFinalizer
		}

		data, err := vc.generatePatch(apiNp, newNp)
		if err != nil {
			return false, err
		}
		klog.Infof("patch bytes for nodepool %s: %v", np.Name, string(data))
		if data == nil {
			return true, nil
		}

		_, apierr = vc.appsClient.AppsV1alpha1().NodePools().Patch(context.TODO(), np.Name, types.MergePatchType, data, metav1.PatchOptions{})
		if apierr != nil {
			klog.Errorf("failed to patch nodepool %s: %v", newNp.Name, apierr)
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		return fmt.Errorf("failed to patch nodepool %s, apierr: %v, err: %v", np.Name, apierr, err)
	}
	return nil
}

func (vc *Controller) getCcnId(np *appsv1alpha1.NodePool) string {
	if np == nil || np.Annotations == nil {
		return ""
	}
	return np.Annotations[util.NodePoolCCNAnnotation]
}

func (vc *Controller) getCenId(np *appsv1alpha1.NodePool) string {
	if np == nil || np.Annotations == nil {
		return ""
	}
	return np.Annotations[util.NodePoolCENAnnotation]
}

var ValidRegion = map[string]bool{
	"cn-shanghai":    true, // 中国内地
	"cn-hongkong":    true, // 中国香港
	"ap-southeast-1": true, // 新加坡
	"ap-southeast-2": true, // 澳大利亚（悉尼）
	"ap-southeast-3": true, // 马来西亚（吉隆坡）
	"ap-southeast-5": true, // 印度尼西亚（雅加达）
	"ap-northeast-1": true, // 日本（东京）
	"eu-central-1":   true, // 德国（法兰克福）
}

func (vc *Controller) getCcnRegion(np *appsv1alpha1.NodePool) (string, error) {
	// if region not specified, use default
	if np == nil || np.Annotations == nil || np.Annotations[util.NodePoolCcnRegionAnnotation] == "" {
		return sagDefaultRegion, nil
	}
	// validate if region is available
	region := np.Annotations[util.NodePoolCcnRegionAnnotation]
	// sag supported region
	if _, ok := ValidRegion[region]; !ok {
		return "", fmt.Errorf("unsupported region: %s", region)
	}

	return region, nil
}

func (vc *Controller) getSagPeriod(np *appsv1alpha1.NodePool) (int, error) {
	if np == nil || np.Annotations == nil {
		// set default value to 1 month
		return 1, nil
	}
	periodStr, ok := np.Annotations[util.NodePoolSagPeriodAnnotation]
	if !ok {
		// set default value to 1 month
		return 1, nil
	}
	period, err := strconv.Atoi(periodStr)
	if err != nil {
		return period, err
	}
	// vcpe limit: valid period is 1,3,6,9,12,24,36,48...
	if period <= 12 && period != 1 && period != 3 && period != 6 && period != 9 && period != 12 {
		return 0, fmt.Errorf("period month must be one of 1,3,6,9,12 if less than 12")
	}
	if period > 12 && period%12 != 0 {
		return 0, fmt.Errorf("period month must be integer times of 12 if larger than 12")
	}

	return period, nil
}

func (vc *Controller) getSagBandwidth(np *appsv1alpha1.NodePool) (int, error) {
	if np == nil || np.Annotations == nil {
		// set default value to 10Mbps
		return 10, nil
	}
	bandwidthStr := np.Annotations[util.NodePoolSagBandwidthAnnotation]
	bandwidth, err := strconv.Atoi(bandwidthStr)
	if err != nil {
		return bandwidth, err
	}
	if bandwidth < 10 {
		return bandwidth, fmt.Errorf("bandwidth cannot be less than 10Mbps")
	}
	if bandwidth%5 != 0 {
		return bandwidth, fmt.Errorf("bandwidth should be multiples of 5")
	}
	return bandwidth, nil
}

// getHostCIDRBlock  set the edge node host cidr to vsag cidr
func (vc *Controller) getHostCIDRBlock(np *appsv1alpha1.NodePool) ([]string, error) {
	if np == nil || np.Annotations == nil {
		return nil, fmt.Errorf("empty node host CIDR block from nodepool: %v", np)
	}
	nodeHostCidrStr := np.Annotations[util.EdgeNodeHostCIDRBlock]
	if nodeHostCidrStr == "" {
		return nil, fmt.Errorf("empty node host cidr block from nodepool: %s", np.Name)
	}

	nodeHostCIDR := strings.Split(nodeHostCidrStr, ",")
	// validate pod cidr format
	for _, hostCidr := range nodeHostCIDR {
		ip, mask, err := net.ParseCIDR(hostCidr)
		if err != nil {
			return nil, err
		}
		if ip.String() != mask.IP.String() {
			return nil, fmt.Errorf("cidr %s is not valid", hostCidr)
		}
	}
	return nodeHostCIDR, nil
}

func (vc *Controller) getPodCidrs(np *appsv1alpha1.NodePool) ([]string, error) {
	if np == nil || np.Annotations == nil {
		return nil, fmt.Errorf("empty podcidr from nodepool: %v", np)
	}
	podCidrStr := np.Annotations[util.NodePoolPodCIDRAnnotation]
	if podCidrStr == "" {
		return nil, fmt.Errorf("empty podcidr from nodepool: %s", np.Name)
	}

	podCidrs := strings.Split(podCidrStr, ",")
	// validate pod cidr format
	for _, podCidr := range podCidrs {
		ip, mask, err := net.ParseCIDR(podCidr)
		if err != nil {
			return nil, err
		}
		if ip.String() != mask.IP.String() {
			return nil, fmt.Errorf("cidr %s is not valid", podCidr)
		}
	}
	return podCidrs, nil
}

// mark all nodes in current nodepool with vsag-managed label
// ignore errors
func (vc *Controller) markNodeIsManagedByVsag(ctx context.Context, np *appsv1alpha1.NodePool, isManaged string) {
	wait.PollImmediate(2*time.Second, 4*time.Second, func() (bool, error) {
		nodeList, err := vc.kubeClient.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{
			LabelSelector: labels.SelectorFromSet(map[string]string{appsv1alpha1.LabelCurrentNodePool: np.Name}).String(),
		})
		if err != nil {
			klog.Errorf("failed to list nodes in nodepool %s: %v, ignore", np.Name, err)
			return false, nil
		}

		for _, node := range nodeList.Items {
			newNode := node.DeepCopy()
			newNode.Labels[util.NodeManagedByVsagLabelKey] = isManaged
			data, err := vc.generatePatch(node, newNode)
			if err != nil {
				return false, err
			}

			if _, err := vc.kubeClient.CoreV1().Nodes().Patch(context.TODO(), node.Name, types.MergePatchType, data, metav1.PatchOptions{}); err != nil {
				klog.Errorf("failed to patch node %s: %v, ignore", node.Name, err)
				continue
			}
		}
		return true, nil
	})
}

func (vc *Controller) generatePatch(old, new interface{}) ([]byte, error) {
	originalJSON, err := json.Marshal(old)
	if err != nil {
		return nil, err
	}

	modifiedJSON, err := json.Marshal(new)
	if err != nil {
		return nil, err
	}

	data, err := jsonpatch.CreateMergePatch(originalJSON, modifiedJSON)
	if err != nil {
		return nil, err
	}

	return data, nil
}

// enqueue cloud node for cen route updating
func (vc *Controller) triggerCloudNodeSync() {
	selector := labels.NewSelector()
	requirement, err := labels.NewRequirement(nodeutil.NodeLabelEdgeWorker, selection.NotEquals, []string{"true"})
	if err != nil {
		klog.Error(err)
		return
	}
	selector = selector.Add(*requirement)
	nodes, err := vc.nodeLister.List(selector)
	if err != nil {
		klog.Error(err)
		return
	}
	for _, node := range nodes {
		vc.enqueueNode(node)
	}
}

func (vc *Controller) handleBandwidthChange(sagID string, newBandwidth int, cloudClient *cloud.ClientSet) error {
	sag, err := cloudClient.SagResourceClient().DescribeSmartAccessGateway(sagID)
	if err != nil {
		klog.Error(err)
		return err
	}
	if sag != nil {
		var curBandwidth int
		matches := SagBandWidthRE.FindStringSubmatch(sag.MaxBandwidth)
		if len(matches) > 0 {
			curBandwidth, _ = strconv.Atoi(matches[0])
		}
		if curBandwidth == newBandwidth {
			// bandwidth has not been modified
			klog.Infof("sag %s: bandwidth %d has not been changed", sagID, curBandwidth)
			// return nil directly, no need to retry
			return nil
		} else if newBandwidth > curBandwidth {
			klog.Infof("sag %s: curBandwidth %d, newBandwidth %d, Upgrade", sagID, curBandwidth, newBandwidth)
			err := cloudClient.SagResourceClient().UpgradeSmartAccessGateway(sagID, newBandwidth)
			if err != nil {
				klog.Errorf("failed to handle sag bandwidth upgrade, %v", err)
				return err
			}
			return nil
		} else {
			klog.Infof("sag %s: curBandwidth %d, newBandwidth %d, Downgrade", sagID, curBandwidth, newBandwidth)
			err := cloudClient.SagResourceClient().DowngradeSmartAccessGateway(sagID, newBandwidth)
			if err != nil {
				klog.Errorf("failed to handle sag bandwidth downgrade, %v", err)
				return err
			}
			return nil
		}
	}
	klog.Warningf("sag %s seems not exist", sagID)
	return fmt.Errorf("sag %s seems not exist", sagID)
}

func IsPodReady(pod *v1.Pod) bool {
	if pod == nil {
		return false
	}

	for i := range pod.Status.Conditions {
		if pod.Status.Conditions[i].Type == v1.PodReady {
			if pod.Status.Conditions[i].Status == v1.ConditionTrue {
				return true
			}
		}
	}
	return false
}
