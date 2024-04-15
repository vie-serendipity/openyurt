package elb

import (
	"context"
	"fmt"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	elbmodel "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/util/cloudprovider/model/elb"
)

func MockSGManager() *ServerGroupManager {
	return &ServerGroupManager{
		kubeClient: MockKubeClient(),
	}
}

func MockKubeClient() client.Client {
	Items := make([]corev1.Node, 0, 10)
	for i := 0; i < 10; i++ {
		node := corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: fmt.Sprintf("ens-%d", i),
				Labels: map[string]string{
					EdgeNodeLabelKey: "true",
					ENSNodeLabelKey:  fmt.Sprintf("ens-id-%d", i),
				},
			},
			Status: corev1.NodeStatus{
				Addresses: []corev1.NodeAddress{
					{
						Type:    corev1.NodeInternalIP,
						Address: fmt.Sprintf("10.0.0.%d", i+1),
					},
				},
			},
		}
		Items = append(Items, node)
	}
	nodeList := &corev1.NodeList{
		Items: Items,
	}
	objs := []runtime.Object{nodeList}
	return fake.NewClientBuilder().WithRuntimeObjects(objs...).Build()
}

func TestServerGroupManager_BatchAddServerGroup(t *testing.T) {
	mgr := MockSGManager()
	nodeList := &corev1.NodeList{}
	err := mgr.kubeClient.List(context.TODO(), nodeList)
	if err != nil {
		t.Error("failed to list nodes")
	}
	sg := &elbmodel.EdgeServerGroup{}
	for _, node := range nodeList.Items {
		sg.Backends = append(sg.Backends, elbmodel.EdgeBackendAttribute{
			ServerId: node.Name,
			ServerIp: node.Status.Addresses[0].Address,
			Weight:   elbmodel.ServerGroupDefaultServerWeight,
			Port:     elbmodel.ServerGroupDefaultPort,
			Type:     elbmodel.ServerGroupDefaultType,
		})
	}
}
