package route

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeconsts "k8s.io/kubernetes/cmd/kubeadm/app/constants"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func Test_NodeFromProviderID(t *testing.T) {
	testcases := []struct {
		pid     string
		expect  string
		isError bool
	}{
		{
			pid:     "alicloud://cn-hangzhou.ecs-1",
			expect:  "ecs-1",
			isError: false,
		},
		{
			pid:     "ecs-1",
			expect:  "",
			isError: true,
		},
		{
			pid:     "cn-hangzhou.ecs-1",
			expect:  "ecs-1",
			isError: false,
		},
	}

	for _, tt := range testcases {
		_, get, err := NodeFromProviderID(tt.pid)
		assert.Equal(t, err != nil, tt.isError, "pares pid from node.Spec.ProviderID")
		assert.Equal(t, get, tt.expect, "pares pid from node.Spec.ProviderID")
	}
}

func Test_IsGatewayNode(t *testing.T) {
	testcases := []struct {
		node   v1.Node
		expect bool
	}{
		{
			node: v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node-1",
					Labels: map[string]string{
						GatewayNode: "",
						NodeTypeKey: "false",
					},
				},
			},
			expect: true,
		},
		{
			node: v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node-1",
					Labels: map[string]string{
						NodeTypeKey: "false",
					},
				},
			},
			expect: false,
		},
		{
			node: v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node-1",
					Labels: map[string]string{
						NodeTypeKey: "true",
						GatewayNode: "",
					},
				},
			},
			expect: false,
		},
	}
	for _, tt := range testcases {
		get := isGatewayNode(&tt.node)
		assert.Equal(t, get, tt.expect, "node is gateway")
	}
}

func Test_FindGatewayNodes(t *testing.T) {
	node1 := v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node-1",
			Labels: map[string]string{
				GatewayNode: "",
				NodeTypeKey: "false",
			},
		},
		Spec: v1.NodeSpec{ProviderID: "cn-hangzhou.ecs-1"},
		Status: v1.NodeStatus{
			Conditions: []v1.NodeCondition{
				{
					Type:   v1.NodeReady,
					Status: v1.ConditionTrue,
				},
			},
		},
	}

	node2 := v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node-2",
			Labels: map[string]string{
				kubeconsts.LabelNodeRoleControlPlane: "",
			},
		},
		Status: v1.NodeStatus{
			Conditions: []v1.NodeCondition{
				{
					Type:   v1.NodeNetworkUnavailable,
					Status: v1.ConditionFalse,
				},
			},
		},
	}

	node3 := v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node-3",
			Labels: map[string]string{
				GatewayNode: "",
				NodeTypeKey: "false",
			},
		},
		Status: v1.NodeStatus{
			Conditions: []v1.NodeCondition{
				{
					Type:   v1.NodeReady,
					Status: v1.ConditionTrue,
				},
			},
		},
	}

	node4 := v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node-4",
			Labels: map[string]string{
				GatewayNode: "",
				NodeTypeKey: "false",
			},
			DeletionTimestamp: &metav1.Time{Time: time.Now()},
		},
		Status: v1.NodeStatus{
			Conditions: []v1.NodeCondition{
				{
					Type:   v1.NodeReady,
					Status: v1.ConditionTrue,
				},
			},
		},
	}

	testcase := []struct {
		r       ReconcileRoute
		nodes   []v1.Node
		expect  []v1.Node
		isError bool
	}{
		{
			r:       ReconcileRoute{Client: fake.NewClientBuilder().Build()},
			nodes:   []v1.Node{node1, node2, node3, node4},
			expect:  []v1.Node{node1},
			isError: false,
		},
		{
			r:       ReconcileRoute{Client: fake.NewClientBuilder().Build()},
			nodes:   []v1.Node{node2, node3, node4},
			expect:  []v1.Node{},
			isError: false,
		},
	}

	for _, tt := range testcase {
		for i := range tt.nodes {
			tt.r.Client.Create(context.TODO(), &tt.nodes[i])
		}
		get, err := tt.r.findGatewayNodes(context.TODO())
		assert.Equal(t, tt.isError, err != nil)
		if len(get) == 0 {
			assert.Equal(t, tt.expect, get)
		} else {
			assert.Equal(t, tt.expect[0].Name, get[0].Name)
		}
	}
}
