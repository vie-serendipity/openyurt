package cloudnodepoollifecycle

import (
	"context"
	"testing"

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/openyurtio/openyurt/pkg/apis"
	"github.com/openyurtio/openyurt/pkg/apis/apps/v1beta1"
)

var (
	mockNodepoolName  = "np4def010990224b4b868df9c032980dc3"
	mockNodepoolName2 = "np4def010990224b4b868df9c032980dc2"
)

type fakeClient struct {
	createCount int
	deleteCount int
	client.WithWatch
}

func (f *fakeClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	f.WithWatch.Create(ctx, obj, opts...)
	f.createCount++
	return nil
}

func (f *fakeClient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	f.WithWatch.Delete(ctx, obj, opts...)
	f.deleteCount++
	return nil
}

func newFakeClient(object ...client.Object) *fakeClient {
	f := &fakeClient{
		createCount: 0,
		deleteCount: 0,
	}

	if object != nil {
		f.WithWatch = fakeclient.NewClientBuilder().WithScheme(newOpenYurtScheme()).WithObjects(object...).Build()
	} else {
		f.WithWatch = fakeclient.NewClientBuilder().WithScheme(newOpenYurtScheme()).Build()
	}

	return f
}

func TestReconcileCloudNodepoolLifecycle_Reconcile(t *testing.T) {
	t.Run("need to create nodepool", func(t *testing.T) {
		mockNode := newMockNode("test", "cn-beijing.i-2ze0rh9z4q62exljg99k", "", map[string]string{cloudNodepoolLabelKey: mockNodepoolName})
		client := newFakeClient(mockNode)
		r := &ReconcileCloudNodepoolLifecycle{
			Client: client,
		}

		_, err := r.Reconcile(context.Background(), reconcile.Request{types.NamespacedName{
			Name: mockNodepoolName,
		}})

		assertErrorNil(t, err)
		assertNodepoolExists(t, client)
		assertCreateCount(t, 1, client.createCount)
		assertDeleteCount(t, 0, client.deleteCount)
	})

	t.Run("don't need to create nodepool with exist", func(t *testing.T) {
		mockNodePool := newCloudNodepool(mockNodepoolName)
		mockNode := newMockNode("test", "cn-beijing.i-2ze0rh9z4q62exljg99k", "", map[string]string{cloudNodepoolLabelKey: mockNodepoolName})
		client := newFakeClient(mockNodePool, mockNode)
		r := &ReconcileCloudNodepoolLifecycle{
			Client: client,
		}

		_, err := r.Reconcile(context.Background(), reconcile.Request{types.NamespacedName{
			Name: mockNodepoolName,
		}})

		assertErrorNil(t, err)
		assertNodepoolExists(t, client)
		assertCreateCount(t, 0, client.createCount)
		assertDeleteCount(t, 0, client.deleteCount)
	})

	t.Run("don't need to create nodepool with no cloud nodes", func(t *testing.T) {
		mockNode := newMockNode("test", "", "", nil)
		client := newFakeClient(mockNode)
		r := &ReconcileCloudNodepoolLifecycle{
			Client: client,
		}

		_, err := r.Reconcile(context.Background(), reconcile.Request{types.NamespacedName{Name: mockNodepoolName}})

		assertErrorNil(t, err)
		assertCreateCount(t, 0, client.createCount)
		assertDeleteCount(t, 0, client.deleteCount)
	})

	t.Run("label nodes with cloud nodepool", func(t *testing.T) {
		mockNodePool := newCloudNodepool(mockNodepoolName)
		mockNode1 := newMockNode("test", "cn-beijing.i-2ze0rh9z4q62exljg99k", "", map[string]string{cloudNodepoolLabelKey: mockNodepoolName})
		mockNode2 := newMockNode("test2", "cn-beijing.i-2ze0rh9z4q62exljg90k", "", map[string]string{cloudNodepoolLabelKey: mockNodepoolName2})

		client := newFakeClient(mockNode1, mockNode2, mockNodePool)
		r := &ReconcileCloudNodepoolLifecycle{
			Client: client,
		}

		_, err := r.Reconcile(context.Background(), reconcile.Request{types.NamespacedName{
			Name: mockNodepoolName,
		}})

		assertErrorNil(t, err)
		assertCloudNodeDefaultLabelsExist(t, client, mockNode1.Name)
		assertCloudNodeDefaultLabelsNotExist(t, client, mockNode2.Name)
	})

	t.Run("delete nodepool with no nodes", func(t *testing.T) {
		mockNodePool := newCloudNodepool(mockNodepoolName)

		client := newFakeClient(mockNodePool)

		r := &ReconcileCloudNodepoolLifecycle{
			Client: client,
		}

		_, err := r.Reconcile(context.Background(), reconcile.Request{types.NamespacedName{
			Name: mockNodepoolName,
		}})

		assertErrorNil(t, err)
		assertNodepoolIsNotExist(t, client, mockNodepoolName)
		assertCreateCount(t, 0, client.createCount)
		assertDeleteCount(t, 1, client.deleteCount)
	})
}

func newOpenYurtScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	apis.AddToScheme(scheme)

	clientgoscheme.AddToScheme(scheme)
	return scheme
}

func assertNodepoolExists(t *testing.T, c client.Client) {
	t.Helper()

	np := &v1beta1.NodePool{}
	err := c.Get(context.Background(), types.NamespacedName{Name: mockNodepoolName}, np)
	assertErrorNil(t, err)
}

func assertCreateCount(t testing.TB, expectedCount int, gotCount int) {
	t.Helper()

	if expectedCount != gotCount {
		t.Errorf("expected create count is %d, but got %d", expectedCount, gotCount)
	}
}

func assertDeleteCount(t testing.TB, expectedCount int, gotCount int) {
	t.Helper()

	if expectedCount != gotCount {
		t.Errorf("expected delete count is %d, but got %d", expectedCount, gotCount)
	}
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

func assertErrorNil(t testing.TB, err error) {
	t.Helper()

	if err != nil {
		t.Errorf("expected error is nil, but got %v", err)
	}
}

func assertCloudNodeDefaultLabelsExist(t testing.TB, client client.Client, nodename string) {
	t.Helper()

	node := &v1.Node{}
	err := client.Get(context.Background(), types.NamespacedName{
		Name: nodename,
	}, node)
	assertErrorNil(t, err)

	if node.Labels == nil {
		t.Errorf("expected node labels is not nil")
	}

	for key, value := range cloudNodeDefaultLabels {
		if v := node.Labels[key]; v != value {
			t.Errorf("expected label %s key %s, but got: %s", key, value, v)
		}
	}
}

func assertCloudNodeDefaultLabelsNotExist(t testing.TB, client client.Client, nodename string) {
	t.Helper()

	node := &v1.Node{}
	err := client.Get(context.Background(), types.NamespacedName{
		Name: nodename,
	}, node)
	assertErrorNil(t, err)

	if node.Labels == nil {
		return
	}

	for key, value := range cloudNodeDefaultLabels {
		if v := node.Labels[key]; v == value {
			t.Errorf("expected label key %s not found", key)
		}
	}
}

func assertNodepoolIsNotExist(t testing.TB, client client.Client, nodepoolName string) {
	t.Helper()

	np := &v1beta1.NodePool{}
	err := client.Get(context.Background(), types.NamespacedName{
		Name: nodepoolName,
	}, np)

	if !apierrors.IsNotFound(err) {
		t.Errorf("expected err is not found, but got %v", err)
	}
}
