package elb

import (
	"fmt"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	discovery "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"

	nodepoolv1beta1 "github.com/openyurtio/openyurt/pkg/apis/apps/v1beta1"
	networkv1alpha1 "github.com/openyurtio/openyurt/pkg/apis/network/v1alpha1"
	"github.com/stretchr/testify/assert"
)

const (
	MockServiceName      = "mock-svc"
	MockServiceNamespace = "mock"
	NodePoolHangzhou     = "np-hangzhou"
	NodePoolShanghai     = "np-shanghai"

	EdgeNodeLabelKey = "alibabacloud.com/is-edge-worker"
	ENSNodeLabelKey  = "alibabacloud.com/ens-instance-id"
)

func MockPoolService(key string) *networkv1alpha1.PoolService {
	lbclass := ELBClass
	ps1 := &networkv1alpha1.PoolService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s", MockServiceName, NodePoolShanghai),
			Namespace: MockServiceNamespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					Kind: "Service",
					Name: MockServiceName,
				},
				{
					Kind: "NodePool",
					Name: NodePoolShanghai,
				},
			},
		},
		Spec: networkv1alpha1.PoolServiceSpec{
			LoadBalancerClass: &lbclass,
		},
	}

	ps2 := &networkv1alpha1.PoolService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s", MockServiceName, NodePoolHangzhou),
			Namespace: MockServiceNamespace,
			UID:       "hello",
			OwnerReferences: []metav1.OwnerReference{
				{
					Kind: "Service",
					Name: MockServiceName,
				},
				{
					Kind: "NodePool",
					Name: NodePoolHangzhou,
				},
			},
		},
		Spec: networkv1alpha1.PoolServiceSpec{
			LoadBalancerClass: &lbclass,
		},
	}
	svc := map[string]*networkv1alpha1.PoolService{ps1.Name: ps1, ps2.Name: ps2}
	return svc[key]
}

func MockENSNodes() []v1.Node {
	nodes := []v1.Node{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "ens-1",
				Labels: map[string]string{
					EdgeNodeLabelKey: "true",
					ENSNodeLabelKey:  "ens-id-1",
					NodePoolLabel:    NodePoolShanghai,
				},
			},
			Status: v1.NodeStatus{
				Addresses: []v1.NodeAddress{
					{
						Type:    v1.NodeInternalIP,
						Address: fmt.Sprintf("10.0.0.1"),
					},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "ens-2",
				Labels: map[string]string{
					EdgeNodeLabelKey: "true",
					ENSNodeLabelKey:  "ens-id-2",
					NodePoolLabel:    NodePoolShanghai,
				},
			},
			Status: v1.NodeStatus{
				Addresses: []v1.NodeAddress{
					{
						Type:    v1.NodeInternalIP,
						Address: fmt.Sprintf("10.0.0.2"),
					},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "ens-3",
				Labels: map[string]string{
					EdgeNodeLabelKey: "true",
					ENSNodeLabelKey:  "ens-id-3",
					NodePoolLabel:    NodePoolHangzhou,
				},
			},
			Status: v1.NodeStatus{
				Addresses: []v1.NodeAddress{
					{
						Type:    v1.NodeInternalIP,
						Address: fmt.Sprintf("10.0.0.3"),
					},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "ens-4",
				Labels: map[string]string{
					EdgeNodeLabelKey: "true",
					ENSNodeLabelKey:  "ens-id-4",
					NodePoolLabel:    NodePoolHangzhou,
				},
			},
			Status: v1.NodeStatus{
				Addresses: []v1.NodeAddress{
					{
						Type:    v1.NodeInternalIP,
						Address: fmt.Sprintf("10.0.0.4"),
					},
				},
			},
		},
	}
	return nodes
}

func MockNodePool() []*nodepoolv1beta1.NodePool {
	np := []*nodepoolv1beta1.NodePool{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: NodePoolHangzhou,
			},
			Spec: nodepoolv1beta1.NodePoolSpec{
				Type: nodepoolv1beta1.Edge,
				Annotations: map[string]string{
					EnsRegionId:  "cn-hangzhou",
					EnsNetworkId: "n-hangzhou",
					EnsVSwitchId: "vsw-hangzhou",
				},
			},
			Status: nodepoolv1beta1.NodePoolStatus{
				Nodes: []string{"ens-3", "ens-4"},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: NodePoolShanghai,
			},
			Spec: nodepoolv1beta1.NodePoolSpec{
				Type: nodepoolv1beta1.Edge,
				Annotations: map[string]string{
					EnsRegionId:  "cn-shanghai",
					EnsNetworkId: "n-shanghai",
					EnsVSwitchId: "vsw-shanghai",
				},
			},
			Status: nodepoolv1beta1.NodePoolStatus{
				Nodes: []string{"ens-1", "ens-2"},
			},
		},
	}
	return np
}
func MockServiceAndEndpointSlice() (svc *v1.Service, es *discovery.EndpointSlice) {
	lbClass := ELBClass
	svc = &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      MockServiceName,
			Namespace: MockServiceNamespace,
		},
		Spec: v1.ServiceSpec{
			Type:              v1.ServiceTypeLoadBalancer,
			LoadBalancerClass: &lbClass,
			Ports: []v1.ServicePort{
				{
					Name:     "cube-80",
					Port:     80,
					Protocol: v1.ProtocolTCP,
					TargetPort: intstr.IntOrString{
						Type:   intstr.Int,
						IntVal: 80,
					},
					NodePort: 30080,
				},
			},
		},
	}
	var PortName = "cube-80"
	var Port int32 = 80
	var ENS1 = "ens-1"
	var ENS2 = "ens-2"
	var ENS3 = "ens-3"
	var ENS4 = "ens-4"
	es = &discovery.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name:      MockServiceName + "-es-1",
			Namespace: MockServiceNamespace,
			Labels:    map[string]string{discovery.LabelServiceName: MockServiceName},
		},
		Ports: []discovery.EndpointPort{
			{
				Name: &PortName,
				Port: &Port,
			},
		},
		Endpoints: []discovery.Endpoint{
			{
				Addresses: []string{"192.168.0.1"},
				NodeName:  &ENS1,
			},
			{
				Addresses: []string{"192.168.0.2"},
				NodeName:  &ENS2,
			},
			{
				Addresses: []string{"192.168.0.3"},
				NodeName:  &ENS3,
			},
			{
				Addresses: []string{"192.168.0.4"},
				NodeName:  &ENS4,
			},
		},
	}
	return
}

func Test_NewPredictionForPoolServiceEvent(t *testing.T) {
	newClass := "slb"
	MockPredicationForPoolServiceEvent := &PredicationForPoolServiceEvent{}
	obj := MockPoolService(fmt.Sprintf("%s-%s", MockServiceName, NodePoolShanghai))
	newObj := obj.DeepCopy()
	newObj.Spec.LoadBalancerClass = &newClass
	cases1 := []struct {
		description string
		evt         event.CreateEvent
		expect      bool
	}{
		{
			description: fmt.Sprintf("test: laodbalancer service with %s is create", ELBClass),
			evt:         event.CreateEvent{Object: obj},
			expect:      true,
		},
		{
			description: fmt.Sprintf("test: laodbalancer service with %s is create", newClass),
			evt:         event.CreateEvent{Object: newObj},
			expect:      false,
		},
	}

	for _, tt := range cases1 {
		res := MockPredicationForPoolServiceEvent.Create(tt.evt)
		if !assert.Equal(t, res, tt.expect) {
			t.Errorf("Test PredicationForPoolServiceEvent.Create, expect %t, but get %t", tt.expect, res)
		}
	}

	cases2 := []struct {
		description string
		evt         event.UpdateEvent
		expect      bool
	}{
		{
			description: "test: laodbalancer service uid is update",
			evt:         event.UpdateEvent{ObjectOld: obj, ObjectNew: updateUID(obj.DeepCopy())},
			expect:      true,
		},
		{
			description: "test: laodbalancer service deletiontimestamp is changed",
			evt:         event.UpdateEvent{ObjectOld: obj, ObjectNew: updateDeleteStatus(obj.DeepCopy())},
			expect:      true,
		},
		{
			description: "test: laodbalancer service concerned annotations is unchanged",
			evt:         event.UpdateEvent{ObjectOld: obj, ObjectNew: updateAnnotation(obj.DeepCopy(), map[string]string{"hello": "hello"})},
			expect:      false,
		},
		{
			description: "test: laodbalancer service concerned annotations is changed",
			evt:         event.UpdateEvent{ObjectOld: obj, ObjectNew: updateAnnotation(obj.DeepCopy(), map[string]string{AnnotationPrefix + "hello": "hello"})},
			expect:      true,
		},
	}

	for _, tt := range cases2 {
		res := MockPredicationForPoolServiceEvent.Update(tt.evt)
		if !assert.Equal(t, res, tt.expect) {
			t.Errorf("Test PredicationForPoolServiceEvent.Update, description %s expect %t, but get %t", tt.description, tt.expect, res)
		}
	}

}

func Test_NewPredictionForServiceEvent(t *testing.T) {
	MockPredicationForServiceEvent := &PredicationForServiceEvent{}
	obj, _ := MockServiceAndEndpointSlice()
	cases := []struct {
		description string
		evt         event.UpdateEvent
		expect      bool
	}{
		{
			description: "test: laodbalancer service uid is update",
			evt:         event.UpdateEvent{ObjectOld: obj, ObjectNew: updateUID(obj.DeepCopy())},
			expect:      false,
		},
		{
			description: "test: laodbalancer service deletiontimestamp is update",
			evt:         event.UpdateEvent{ObjectOld: obj, ObjectNew: updateDeleteStatus(obj.DeepCopy())},
			expect:      false,
		},
		{
			description: "test: laodbalancer service concerned annotation is update",
			evt:         event.UpdateEvent{ObjectOld: obj, ObjectNew: updateAnnotation(obj.DeepCopy(), map[string]string{AnnotationPrefix + "hello": "hello"})},
			expect:      true,
		},
		{
			description: "test: laodbalancer service  spec.Ports is update",
			evt:         event.UpdateEvent{ObjectOld: obj, ObjectNew: updateSvcPorts(obj.DeepCopy())},
			expect:      true,
		},
	}

	for _, tt := range cases {
		res := MockPredicationForServiceEvent.Update(tt.evt)
		if !assert.Equal(t, res, tt.expect) {
			t.Errorf("Test MockPredicationForServiceEvent.Update, description %s, expect %t, but get %t", tt.description, tt.expect, res)
		}
	}
}

func Test_NewPredictionForEndpointSliceEvent(t *testing.T) {
	svc1, es1 := MockServiceAndEndpointSlice()
	svc2 := svc1.DeepCopy()
	svc2.Name = "svc"
	svc2.Spec.Type = v1.ServiceTypeClusterIP
	es2 := es1.DeepCopy()
	es2.Labels[discovery.LabelServiceName] = "svc"

	kubeClient := fake.NewClientBuilder().WithObjects(svc1, svc2).Build()
	MockPredicationForEndpointSliceEvent := &PredicationForEndpointSliceEvent{client: kubeClient}

	cases1 := []struct {
		description string
		evt         event.CreateEvent
		expect      bool
	}{
		{
			description: "test: elb service endpointslice is created",
			evt:         event.CreateEvent{Object: es1},
			expect:      true,
		},
		{
			description: "test: other service endpointslice is created",
			evt:         event.CreateEvent{Object: es2},
			expect:      false,
		},
	}

	for _, tt := range cases1 {
		res := MockPredicationForEndpointSliceEvent.Create(tt.evt)
		if !assert.Equal(t, res, tt.expect) {
			t.Errorf("Test MockPredicationForEndpointSliceEvent.Create, description %s, expect %t, but get %t", tt.description, tt.expect, res)
		}
	}

	cases2 := []struct {
		description string
		evt         event.DeleteEvent
		expect      bool
	}{
		{
			description: "test: elb service endpointslice is delete",
			evt:         event.DeleteEvent{Object: es1},
			expect:      true,
		},
		{
			description: "test: other service endpointslice is created",
			evt:         event.DeleteEvent{Object: es2},
			expect:      false,
		},
	}

	for _, tt := range cases2 {
		res := MockPredicationForEndpointSliceEvent.Delete(tt.evt)
		if !assert.Equal(t, res, tt.expect) {
			t.Errorf("Test MockPredicationForEndpointSliceEvent.Create, description %s, expect %t, but get %t", tt.description, tt.expect, res)
		}
	}

	newEs1 := es1.DeepCopy()
	newEs1.Endpoints = newEs1.Endpoints[:2]
	cases3 := []struct {
		description string
		evt         event.UpdateEvent
		expect      bool
	}{
		{
			description: "test: the endpoints in endpointslice has changed",
			evt:         event.UpdateEvent{ObjectOld: es1, ObjectNew: newEs1},
			expect:      true,
		},
		{
			description: "test: the meta in endpointslice has changed",
			evt:         event.UpdateEvent{ObjectOld: es1, ObjectNew: updateAnnotation(es1, map[string]string{"hello": "hello"})},
			expect:      false,
		},
	}

	for _, tt := range cases3 {
		res := MockPredicationForEndpointSliceEvent.Update(tt.evt)
		if !assert.Equal(t, res, tt.expect) {
			t.Errorf("Test MockPredicationForEndpointSliceEvent.Update, description %s, expect %t, but get %t", tt.description, tt.expect, res)
		}
	}

}

func updateUID(obj client.Object) client.Object {
	obj.SetUID("foo")
	return obj
}

func updateAnnotation(obj client.Object, anno map[string]string) client.Object {
	obj.SetAnnotations(anno)
	return obj
}

func updateDeleteStatus(obj client.Object) client.Object {
	obj.SetDeletionTimestamp(&metav1.Time{Time: time.Now()})
	return obj
}

func updateSvcPorts(svc *v1.Service) *v1.Service {
	for i := range svc.Spec.Ports {
		svc.Spec.Ports[i].Port = svc.Spec.Ports[i].Port + 10
	}
	return svc
}
