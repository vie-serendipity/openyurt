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
	"context"
	"fmt"
	prvd "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/util/cloudprovider"
	ravenmodel "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/util/cloudprovider/model/raven"
	"net"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	appconfig "github.com/openyurtio/openyurt/cmd/yurt-manager/app/config"
	"github.com/openyurtio/openyurt/cmd/yurt-manager/names"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/raven/util"
)

func Format(format string, args ...interface{}) string {
	s := fmt.Sprintf(format, args...)
	return fmt.Sprintf("%s: %s", names.RavenACLEntryController, s)
}

// Add creates a new Service Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(ctx context.Context, c *appconfig.CompletedConfig, mgr manager.Manager) error {
	r, err := newReconciler(ctx, c, mgr)
	if err != nil {
		klog.Error(Format("new reconcile error: %s", err.Error()))
		return err
	}
	return add(mgr, r)
}

var _ reconcile.Reconciler = &ReconcileACL{}

type ReconcileACL struct {
	client.Client
	scheme   *runtime.Scheme
	recorder record.EventRecorder
	provider prvd.Provider
	model    *ravenmodel.AccessControlListAttribute
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(ctx context.Context, c *appconfig.CompletedConfig, mgr manager.Manager) (reconcile.Reconciler, error) {
	if c.Config.ComponentConfig.Generic.CloudProvider == nil {
		klog.Error("can not get alibaba provider")
		return nil, fmt.Errorf("can not get alibaba provider")
	}
	return &ReconcileACL{
		Client:   mgr.GetClient(),
		scheme:   mgr.GetScheme(),
		provider: c.Config.ComponentConfig.Generic.CloudProvider,
		recorder: mgr.GetEventRecorderFor(names.RavenACLEntryController),
	}, nil
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New(names.RavenACLEntryController, mgr,
		controller.Options{Reconciler: r, MaxConcurrentReconciles: 1})
	if err != nil {
		return err
	}

	err = c.Watch(&source.Kind{Type: &corev1.ConfigMap{}}, &EnqueueRequestForRavenCfgEvent{}, predicate.NewPredicateFuncs(
		func(object client.Object) bool {
			cm, ok := object.(*corev1.ConfigMap)
			if !ok {
				return false
			}
			if cm.GetNamespace() != util.WorkingNamespace {
				return false
			}
			if cm.GetName() != util.RavenGlobalConfig {
				return false
			}
			return true
		}))
	if err != nil {
		return err
	}
	return nil
}

// Reconcile reads that state of the cluster for a Gateway object and makes changes based on the state read
// and what is in the Gateway.Spec
func (r *ReconcileACL) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	klog.V(2).Info(Format("started reconciling configmap %s", req.String()))
	defer func() {
		klog.V(2).Info(Format("finished reconciling configmap %s", req.String()))
	}()
	var err error
	err = r.buildModel()
	if err != nil {
		klog.Error(Format("init model error: %s", err.Error()))
		return reconcile.Result{Requeue: true, RequeueAfter: time.Second}, err
	}
	cm, err := r.getRavenCfg(ctx, req)
	if err != nil {
		klog.Error(Format("get configmap %s, error %s", req.String(), err.Error()))
		return reconcile.Result{Requeue: true, RequeueAfter: time.Second}, err
	}
	if cm.Data == nil || cm.Data[util.ACLId] == "" {
		klog.Infof(Format("acl id is empty, skip reconcile it"))
		return reconcile.Result{}, nil
	}
	r.model.AccessControlListId = cm.Data[util.ACLId]
	err = r.getLocalACLEntry(cm)
	if err != nil {
		klog.Error(Format("invalid format acl entry error %s", err.Error()))
		return reconcile.Result{Requeue: true, RequeueAfter: 2 * time.Second}, err
	}
	err = r.getRemoteACLEntry(ctx)
	if err != nil {
		return reconcile.Result{Requeue: true, RequeueAfter: 2 * time.Second}, err
	}
	err = r.updateACLEntry(ctx)
	if err != nil {
		klog.Error(Format("update acl entry error %s", err.Error()))
		return reconcile.Result{Requeue: true, RequeueAfter: 2 * time.Second}, fmt.Errorf("update remote acl entry error %s", err.Error())
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileACL) buildModel() error {
	clusterId, err := r.provider.GetClusterID()
	if err != nil {
		return err
	}
	regionId, err := r.provider.GetRegion()
	if err != nil {
		return err
	}
	r.model = &ravenmodel.AccessControlListAttribute{
		NamedKey: ravenmodel.NewNamedKey(clusterId),
		Region:   regionId,
	}
	return nil
}

func (r *ReconcileACL) getRavenCfg(ctx context.Context, req reconcile.Request) (*corev1.ConfigMap, error) {
	var cm corev1.ConfigMap
	err := r.Get(ctx, req.NamespacedName, &cm)
	if err != nil {
		return nil, fmt.Errorf("get configmap %s error %s", req.String(), err.Error())
	}
	return cm.DeepCopy(), nil
}

func (r *ReconcileACL) getLocalACLEntry(cm *corev1.ConfigMap) error {
	if cm == nil || cm.Data == nil {
		return nil
	}
	entry := cm.Data[util.ACLEntry]
	if entry == "" {
		return nil
	}
	inputs := strings.Split(entry, ",")
	entries := make([]string, 0)
	for i := range inputs {
		if inputs[i] == "" {
			continue
		}
		address, err := FormatCIDR(inputs[i])
		if err != nil {
			return err
		}
		entries = append(entries, address)
	}
	r.model.LocalEntries = entries
	return nil
}

func (r *ReconcileACL) getRemoteACLEntry(ctx context.Context) error {
	return r.provider.DescribeAccessControlListAttribute(ctx, r.model)
}

func (r *ReconcileACL) updateACLEntry(ctx context.Context) error {
	var err error

	added, deleted := classifyEntry(r.model.LocalEntries, r.model.RemoteEntries)

	err = r.provider.AddAccessControlListEntry(ctx, r.model, entryConvertString(added))
	if err != nil {
		return err
	}
	err = r.provider.RemoveAccessControlListEntry(ctx, r.model, entryConvertString(deleted))
	if err != nil {
		return err
	}
	return nil
}

func entryConvertString(src []string) string {
	entrys := make([]string, 0)
	for _, entry := range src {
		entrys = append(entrys, fmt.Sprintf("{\"entry\":\"%s\"}", entry))
	}
	return fmt.Sprintf("[%s]", strings.Join(entrys, ","))
}

func classifyEntry(localEntries, remoteEntries []string) (added, deleted []string) {
	added = make([]string, 0)
	deleted = make([]string, 0)
	remoteEntryMap := make(map[string]struct{})
	for i := range remoteEntries {
		remoteEntryMap[remoteEntries[i]] = struct{}{}
	}
	for i := range localEntries {
		if _, ok := remoteEntryMap[localEntries[i]]; ok {
			delete(remoteEntryMap, localEntries[i])
		} else {
			added = append(added, localEntries[i])
		}
	}

	for key := range remoteEntryMap {
		deleted = append(deleted, key)
	}
	return
}

func FormatCIDR(address string) (string, error) {
	_, ipNet, err := net.ParseCIDR(address)
	if err != nil {
		ip := net.ParseIP(address)
		if ip == nil {
			return "", fmt.Errorf("invalid entry %s", address)
		}
		_, ipNet, err = net.ParseCIDR(fmt.Sprintf("%s/32", ip))
		if err != nil {
			return "", fmt.Errorf("invalid entry %s", address)
		}
	}
	if ipNet.String() != address {
		return ipNet.String(), fmt.Errorf("non-standard format, please use %s", ipNet.String())
	}
	return ipNet.String(), nil
}
