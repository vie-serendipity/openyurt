package elb

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	v1 "k8s.io/api/core/v1"
	discovery "k8s.io/api/discovery/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	networkapi "github.com/openyurtio/openyurt/pkg/apis/network"
	networkv1alpha1 "github.com/openyurtio/openyurt/pkg/apis/network/v1alpha1"
)

var _ handler.EventHandler = (*EnqueueRequestForServiceEvent)(nil)

type EnqueueRequestForServiceEvent struct {
	client client.Client
}

func NewEnqueueRequestForServiceEvent(client client.Client) *EnqueueRequestForServiceEvent {
	return &EnqueueRequestForServiceEvent{
		client: client,
	}
}

func (h *EnqueueRequestForServiceEvent) Create(e event.CreateEvent, queue workqueue.RateLimitingInterface) {
	return
}

func (h *EnqueueRequestForServiceEvent) Update(e event.UpdateEvent, queue workqueue.RateLimitingInterface) {
	_, ok1 := e.ObjectOld.(*v1.Service)
	newSvc, ok2 := e.ObjectNew.(*v1.Service)
	if ok1 && ok2 {
		var psList networkv1alpha1.PoolServiceList
		err := h.client.List(context.TODO(), &psList, &client.ListOptions{
			Namespace:     newSvc.GetNamespace(),
			LabelSelector: labels.SelectorFromSet(map[string]string{networkapi.LabelServiceName: newSvc.GetName()}),
		})
		if err != nil {
			klog.Error(err, "fail to list pool services for object", "object type: ", "service", "object key: ", Key(newSvc))
			return
		}
		for idx := range psList.Items {
			key := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: psList.Items[idx].Namespace,
					Name:      psList.Items[idx].Name,
				},
			}
			klog.Info("enqueue", "pool service", key.String(), " for service: ", Key(newSvc), "queueLen", queue.Len())
			queue.Add(key)
		}
	}
}

func (h *EnqueueRequestForServiceEvent) Delete(e event.DeleteEvent, queue workqueue.RateLimitingInterface) {

}

func (h *EnqueueRequestForServiceEvent) Generic(e event.GenericEvent, queue workqueue.RateLimitingInterface) {

}

func NewEnqueueRequestForEndpointSliceEvent(client client.Client) *EnqueueRequestForEndpointSliceEvent {
	return &EnqueueRequestForEndpointSliceEvent{
		client: client,
	}
}

type EnqueueRequestForEndpointSliceEvent struct {
	client client.Client
}

var _ handler.EventHandler = (*EnqueueRequestForEndpointSliceEvent)(nil)

func (h *EnqueueRequestForEndpointSliceEvent) Create(e event.CreateEvent, queue workqueue.RateLimitingInterface) {
	es, ok := e.Object.(*discovery.EndpointSlice)
	if ok {
		klog.Info("enqueue: endpointslice create event", "endpointslice", Key(es))
		h.enqueueForEndpointSliceCreateOrDeleteEvent(es, queue)
	}
}

func (h *EnqueueRequestForEndpointSliceEvent) Update(e event.UpdateEvent, queue workqueue.RateLimitingInterface) {
	es1, ok1 := e.ObjectOld.(*discovery.EndpointSlice)
	es2, ok2 := e.ObjectNew.(*discovery.EndpointSlice)
	if ok1 && ok2 {
		klog.Info(fmt.Sprintf("controller: endpointslice update event, endpoints before [%s], afeter [%s]",
			LogEndpointSlice(es1), LogEndpointSlice(es2)), "endpointslice", Key(es1))
		h.enqueueForEndpointSliceUpdateEvent(es1, es2, queue)
	}
}

func (h *EnqueueRequestForEndpointSliceEvent) Delete(e event.DeleteEvent, queue workqueue.RateLimitingInterface) {
	es, ok := e.Object.(*discovery.EndpointSlice)
	if ok {
		klog.Info("enqueue: endpointslice delete event", "endpointslice", Key(es))
		h.enqueueForEndpointSliceCreateOrDeleteEvent(es, queue)
	}
}

func (h *EnqueueRequestForEndpointSliceEvent) Generic(e event.GenericEvent, queue workqueue.RateLimitingInterface) {
	// unknown event, ignore
}

func (h *EnqueueRequestForEndpointSliceEvent) enqueueForEndpointSliceCreateOrDeleteEvent(es *discovery.EndpointSlice, queue workqueue.RateLimitingInterface) {
	svcKey, err := findRelatedService(es)
	if err != nil {
		klog.Errorf("failed to get service for endpointslice %s, skip reconcile pool service", Key(es))
		return
	}
	nodes := findRelatedNodes(es)
	if len(nodes) < 1 {
		klog.Errorf("no endpoint nodes for endpointslice %s, skip reconcile pool service", Key(es))
		return
	}
	nodePools := findRelatedNodePools(nodes, h.client)
	if len(nodePools) < 1 {
		klog.Errorf("no endpoint nodepools for endpointslice %s, skip reconcile pool service", Key(es))
		return
	}
	h.enqueuePoolServicesForEndpointSlice(svcKey, nodePools, queue)
}

func (h *EnqueueRequestForEndpointSliceEvent) enqueueForEndpointSliceUpdateEvent(esOld, esNew *discovery.EndpointSlice, queue workqueue.RateLimitingInterface) {
	svcNewKey, err := findRelatedService(esNew)
	if err != nil {
		klog.Errorf("failed to get service for endpointslice %s, skip reconcile pool service", Key(esNew))
		return
	}
	svcOldKey, err := findRelatedService(esOld)
	if err != nil {
		klog.Errorf("failed to get service for endpointslice %s, skip reconcile pool service", Key(esNew))
		return
	}
	if svcNewKey != svcOldKey {
		klog.Info("enqueue: endpointslice owner service change event", fmt.Sprintf("before [%s], after [%s]", svcOldKey, svcNewKey))
		h.enqueueForEndpointSliceCreateOrDeleteEvent(esOld, queue)
		h.enqueueForEndpointSliceCreateOrDeleteEvent(esNew, queue)
		return
	}
	nodes := classifyNodes(findRelatedNodes(esNew), findRelatedNodes(esOld))
	if len(nodes) < 1 {
		klog.Errorf("no endpoint nodes changed for endpointslice %s, skip reconcile pool service", Key(esNew))
		return
	}

	nodePools := findRelatedNodePools(nodes, h.client)
	if len(nodePools) < 1 {
		klog.Errorf("no endpoint nodepools changed for endpointslice %s, skip reconcile pool service", Key(esNew))
		return
	}
	klog.Infof("endpoint nodes changed [%v] and nodepools [%s] changed for endpointslice %s", nodes, nodePools, Key(esNew))
	h.enqueuePoolServicesForEndpointSlice(svcNewKey, nodePools, queue)
}

func (h *EnqueueRequestForEndpointSliceEvent) enqueuePoolServicesForEndpointSlice(objectKey *client.ObjectKey, nodePools []string, queue workqueue.RateLimitingInterface) {
	labelSelector, err := formatLabelSelector(objectKey.Namespace, objectKey.Name, nodePools)
	if err != nil {
		klog.Errorf("format label selector err %s for endpointslice %s, skip reconcile pool service", err, objectKey.String())
		return
	}

	var psList networkv1alpha1.PoolServiceList
	err = h.client.List(context.TODO(), &psList, &client.ListOptions{Namespace: objectKey.Namespace, LabelSelector: labelSelector})
	if err != nil {
		klog.Errorf("find related pool service for endpointslice %s, error %s , skip reconcile pool service", objectKey.String(), err)
		return
	}

	for _, ps := range psList.Items {
		if IsELBPoolService(&ps) {
			queue.Add(reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: ps.Namespace,
					Name:      ps.Name,
				},
			})
			klog.Info("enqueue", "poolservice", fmt.Sprintf("%s/%s", ps.Namespace, ps.Name), "for endpointslice", objectKey.String(), "queueLen", queue.Len())
		}
	}
}

func findRelatedService(es *discovery.EndpointSlice) (*client.ObjectKey, error) {
	if es.Labels == nil {
		es.Labels = make(map[string]string)
	}
	svcName, ok := es.Labels[discovery.LabelServiceName]
	if !ok {
		return nil, fmt.Errorf("endpointslice.Label[%s] is empty", discovery.LabelServiceName)
	}
	svcNamespace := es.GetNamespace()
	return &types.NamespacedName{Namespace: svcNamespace, Name: svcName}, nil
}

func findRelatedNodes(es *discovery.EndpointSlice) []string {
	nodes := make([]string, 0)
	nodeKeys := make(map[string]struct{})
	for _, ep := range es.Endpoints {
		nodeName := *ep.NodeName
		if _, ok := nodeKeys[nodeName]; ok {
			continue
		}
		nodes = append(nodes, nodeName)
		nodeKeys[*ep.NodeName] = struct{}{}
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i] < nodes[j] })
	return nodes
}

func findRelatedNodePools(nodes []string, cli client.Client) []string {
	nodePools := make([]string, 0)
	nodePoolKeys := make(map[string]struct{})
	for _, nodeName := range nodes {
		var node v1.Node
		err := cli.Get(context.TODO(), client.ObjectKey{Name: nodeName}, &node)
		if err != nil {
			if !apierrors.IsNotFound(err) {
				klog.Error(err, fmt.Sprintf("failed to get node %s, skip detect nodepool", node.GetName()))
				continue
			}
		}
		if node.Labels == nil {
			node.Labels = make(map[string]string)
		}
		if !IsENSNode(&node) {
			continue
		}
		npName, ok := node.Labels[NodePoolLabel]
		if !ok {
			klog.Error("failed to get nodepool  for node %s, skip detect nodepool", node.GetName())
			continue
		}
		if _, ok := nodePoolKeys[npName]; ok {
			continue
		}
		nodePools = append(nodePools, npName)
		nodePoolKeys[npName] = struct{}{}
	}
	sort.Slice(nodePools, func(i, j int) bool { return nodePools[i] < nodePools[j] })
	return nodePools
}

func formatLabelSelector(svcNamespace, svcName string, nodePools []string) (labels.Selector, error) {
	labelSelector := labels.NewSelector()
	svcReq, err := labels.NewRequirement(networkapi.LabelServiceName, selection.Equals, []string{svcName})
	if err != nil {
		return nil, fmt.Errorf("service %s/%s new service label requirement err %s", svcNamespace, svcName, err.Error())
	}
	poolReq, err := labels.NewRequirement(networkapi.LabelNodePoolName, selection.In, nodePools)
	if err != nil {
		return nil, fmt.Errorf("nodepool %v new nodepool label requirement err %s", nodePools, err.Error())
	}
	return labelSelector.Add(*svcReq, *poolReq), nil
}

func classifyNodes(currNodes, prevNodes []string) []string {
	nodes := make([]string, 0)
	prev := make(map[string]struct{})
	for i := range prevNodes {
		prev[prevNodes[i]] = struct{}{}
	}
	for i := range currNodes {
		if _, ok := prev[currNodes[i]]; ok {
			delete(prev, currNodes[i])
			continue
		}
		nodes = append(nodes, currNodes[i])
	}
	for key := range prev {
		nodes = append(nodes, key)
	}
	return nodes
}

func NewEnqueueRequestForNodeEvent(client client.Client) *EnqueueRequestForNodeEvent {
	return &EnqueueRequestForNodeEvent{
		client: client,
	}
}

type EnqueueRequestForNodeEvent struct {
	client client.Client
}

var _ handler.EventHandler = (*EnqueueRequestForNodeEvent)(nil)

func (h *EnqueueRequestForNodeEvent) Create(e event.CreateEvent, queue workqueue.RateLimitingInterface) {
	node, ok := e.Object.(*v1.Node)
	if ok {
		klog.Info("controller: node create event", "node", node.Name)
		enqueueELBPoolServicesForNode(queue, h.client, node)
	}
}

func (h *EnqueueRequestForNodeEvent) Update(e event.UpdateEvent, queue workqueue.RateLimitingInterface) {
	_, ok1 := e.ObjectOld.(*v1.Node)
	newNode, ok2 := e.ObjectNew.(*v1.Node)
	if ok1 && ok2 {
		klog.Info("controller: node update event", "node", newNode.Name)
		enqueueELBPoolServicesForNode(queue, h.client, newNode)
	}
}

func (h *EnqueueRequestForNodeEvent) Delete(e event.DeleteEvent, queue workqueue.RateLimitingInterface) {
	node, ok := e.Object.(*v1.Node)
	if ok {
		klog.Info("controller: node delete event", "node", node.Name)
		enqueueELBPoolServicesForNode(queue, h.client, node)
	}
}

func (h *EnqueueRequestForNodeEvent) Generic(e event.GenericEvent, queue workqueue.RateLimitingInterface) {
}

func enqueueELBPoolServicesForNode(queue workqueue.RateLimitingInterface, cli client.Client, node *v1.Node) {
	if node.Labels == nil {
		node.Labels = make(map[string]string)
	}
	np, ok := node.Labels[NodePoolLabel]
	if !ok {
		klog.Errorf("can not find nodepool id for node %s, skip reconcile related poolservice", node.Name)
		return
	}
	var psList networkv1alpha1.PoolServiceList
	err := cli.List(context.TODO(), &psList, &client.ListOptions{LabelSelector: labels.SelectorFromSet(map[string]string{networkapi.LabelNodePoolName: np})})
	if err != nil {
		klog.Errorf("can not find poolservices for nodepool %s, skip reconcile related poolservice", np)
		return
	}

	for _, ps := range psList.Items {
		if IsELBPoolService(&ps) {
			queue.Add(reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: ps.Namespace,
					Name:      ps.Name,
				},
			})
			klog.Info("enqueue", "poolservice", fmt.Sprintf("%s/%s", ps.Namespace, ps.Name), "for node", node.Namespace, "queueLen", queue.Len())
		}
	}
}

func annotationValidator(req *RequestContext) error {
	if !IsELBPoolService(req.PoolService) {
		return nil
	}

	if !req.PoolService.DeletionTimestamp.IsZero() {
		return nil
	}

	if req.AnnoCtx.anno == nil {
		return nil
	}

	if req.AnnoCtx.Get(LoadBalancerId) != "" || req.AnnoCtx.Get(EipId) != "" {
		if !req.AnnoCtx.IsManageByUser() {
			fldPath := field.NewPath("meta").Child("annotations").Key(Annotation(ManagedByUser))
			return field.Invalid(fldPath, req.AnnoCtx.Get(ManagedByUser), fmt.Sprintf("the annotations[%s] must be true if loadbalancer and eip are managed by user", Annotation(ManagedByUser)))
		}
	}

	if req.AnnoCtx.Get(LoadBalancerId) != "" {
		if b, err := strconv.ParseBool(req.AnnoCtx.Get(ListenerOverride)); err == nil && b {
			if req.Service.Spec.ExternalTrafficPolicy != v1.ServiceExternalTrafficPolicyTypeCluster {
				fldPath := field.NewPath("meta").Child("annotations").Key(Annotation(ListenerOverride))
				return field.Invalid(fldPath, b, fmt.Sprintf("the spec.externalTrafficPolicy must be specified %s if elb is reused", v1.ServiceExternalTrafficPolicyTypeCluster))
			}
		}
	}

	if isp := req.AnnoCtx.Get(EipInternetProviderService); isp != "" {
		if !strings.Contains(strings.ToLower(req.PoolAttribute.Region()), isp) {
			fldPath := field.NewPath("meta").Child("annotations").Key(Annotation(EipInternetProviderService))
			return field.Invalid(fldPath, isp, fmt.Sprintf("the nodepool region %s not support ISP %s", req.PoolAttribute.Region(), isp))
		}
	}

	if req.AnnoCtx.Get(BackendLabel) != "" {
		if !validateLabels(req.AnnoCtx.Get(BackendLabel)) {
			fldPath := field.NewPath("meta").Child("annotations").Key(Annotation(BackendLabel))
			return field.Invalid(fldPath, req.AnnoCtx.Get(BackendLabel), fmt.Sprintf("correct format, e.g. key1=val1,key2=val2,..."))
		}
	}
	return nil
}

func validateLabels(s string) bool {
	return regexp.MustCompile("^([\\w.-]+)=([\\w.-]+)(,([\\w.-]+)=([\\w.-]+))*$").MatchString(s)
}
