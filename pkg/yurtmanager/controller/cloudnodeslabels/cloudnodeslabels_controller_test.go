package cloudnodeslabels

import (
	"context"
	"reflect"
	"testing"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestReconcileCloudnodeslabels_Reconcile(t *testing.T) {
	t.Run("need to label node", func(t *testing.T) {
		mockNode := newMockNode("test", "cn-beijing.i-b8z0mo9ds1680jue0srw", "", nil)

		client := fakeclient.NewClientBuilder().WithObjects(mockNode).Build()
		r := &ReconcileCloudNodesLabels{
			Client: client,
		}

		_, err := r.Reconcile(context.Background(), reconcile.Request{types.NamespacedName{Name: mockNode.Name}})

		assertErrNil(t, err)
		assertNodeLabels(t, cloudNodeDefaultLabels, getNodeLabels(client, mockNode.Name))
		assertNodeResourceVersion(t, "1000", getNodeResourceVersion(client, mockNode.Name))
	})

	t.Run("don't to label node", func(t *testing.T) {
		mockNode := newMockNode("test", "cn-beijing.i-b8z0mo9ds1680jue0srw", "", cloudNodeDefaultLabels)

		client := fakeclient.NewClientBuilder().WithObjects(mockNode).Build()

		r := &ReconcileCloudNodesLabels{
			Client: client,
		}

		_, err := r.Reconcile(context.Background(), reconcile.Request{types.NamespacedName{
			Name: mockNode.Name,
		}})

		assertErrNil(t, err)
		assertNodeResourceVersion(t, "999", getNodeResourceVersion(client, mockNode.Name))
	})

	t.Run("not cloud node", func(t *testing.T) {
		mockNode := newMockNode("test", "", "", nil)

		client := fakeclient.NewClientBuilder().WithObjects(mockNode).Build()

		r := &ReconcileCloudNodesLabels{
			Client: client,
		}

		_, err := r.Reconcile(context.Background(), reconcile.Request{types.NamespacedName{
			Name: mockNode.Name,
		}})

		assertErrNil(t, err)
		assertNodeLabels(t, nil, getNodeLabels(client, mockNode.Name))
		assertNodeResourceVersion(t, "999", getNodeResourceVersion(client, mockNode.Name))
	})
}

func newMockNode(name string, providerId string, resourceVersion string,
	labels map[string]string) *v1.Node {
	return &v1.Node{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Node",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Labels:          labels,
			ResourceVersion: resourceVersion,
		},
		Spec: v1.NodeSpec{
			ProviderID: providerId,
		},
	}
}

func getNodeLabels(cli client.Client, nodeName string) map[string]string {
	no := &v1.Node{}

	cli.Get(context.Background(), types.NamespacedName{
		Name: nodeName,
	}, no)

	return no.Labels
}

func getNodeResourceVersion(cli client.Client, nodeName string) string {
	no := &v1.Node{}

	cli.Get(context.Background(), types.NamespacedName{
		Name: nodeName,
	}, no)

	return no.ResourceVersion
}

func assertErrNil(t testing.TB, err error) {
	t.Helper()

	if err != nil {
		t.Errorf("expected err is nil")
	}
}

func assertNodeLabels(t testing.TB, expected map[string]string, got map[string]string) {
	t.Helper()

	if !reflect.DeepEqual(expected, got) {
		t.Errorf("expected labels %v, but got %v", expected, got)
	}
}

func assertNodeResourceVersion(t testing.TB, expected string, got string) {
	t.Helper()

	if expected != got {
		t.Errorf("expected resource version to %s, but got %s", expected, got)
	}
}
