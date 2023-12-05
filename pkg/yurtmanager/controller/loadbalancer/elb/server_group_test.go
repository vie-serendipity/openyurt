package elb

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"

	elbmodel "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/util/cloudprovider/model/elb"
)

func MockSGManager() *ServerGroupManager {
	return &ServerGroupManager{
		kubeClient: MockKubeClient(),
	}
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

	err = mgr.batchAddServerGroup(&RequestContext{Ctx: context.TODO()}, "", sg)
	if err != nil {
		t.Error("failed to batch add server group")
	}

}
