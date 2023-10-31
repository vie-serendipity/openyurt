package cloudnodepoollifecycle

import (
	"testing"

	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestEnqueueRequestNode_Create(t *testing.T) {
	t.Run("node with nodepool label", func(t *testing.T) {
		e := &EnqueueRequestNode{}
		mockNode := newMockNode("test", "cn-beijing.i-2ze0rh9z4q62exljg99k", "", map[string]string{cloudNodepoolLabelKey: mockNodepoolName})

		q := workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "cloud_nodepool_life_cycle")
		evt := event.CreateEvent{
			mockNode,
		}
		e.Create(evt, q)

		assertQueueItem(t, q, mockNodepoolName)
	})

	t.Run("node without label", func(t *testing.T) {
		e := &EnqueueRequestNode{}
		mockNode := newMockNode("test", "cn-beijing.i-2ze0rh9z4q62exljg99k", "", nil)

		q := workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "cloud_nodepool_life_cycle")
		evt := event.CreateEvent{
			mockNode,
		}
		e.Create(evt, q)

		assertQueueNil(t, q)
	})

	t.Run("node without nodepool label", func(t *testing.T) {
		e := &EnqueueRequestNode{}
		mockNode := newMockNode("test", "cn-beijing.i-2ze0rh9z4q62exljg99k", "", map[string]string{"key1": "value1"})

		q := workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "cloud_nodepool_life_cycle")
		evt := event.CreateEvent{
			mockNode,
		}
		e.Create(evt, q)

		assertQueueNil(t, q)
	})

	t.Run("edge node", func(t *testing.T) {
		e := &EnqueueRequestNode{}
		mockNode := newMockNode("test", "", "", map[string]string{cloudNodepoolLabelKey: mockNodepoolName})

		q := workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "cloud_nodepool_life_cycle")
		evt := event.CreateEvent{
			mockNode,
		}
		e.Create(evt, q)

		assertQueueNil(t, q)
	})

}

func assertQueueItem(t testing.TB, q workqueue.RateLimitingInterface, name string) {
	t.Helper()

	if q.Len() != 1 {
		t.Errorf("expected q.Len is 1, but got %d", q.Len())
		return
	}

	item, _ := q.Get()
	r, ok := item.(reconcile.Request)
	if !ok {
		t.Errorf("expected item type is reconcile.Request, but got %T", item)
	}

	if r.Name != name {
		t.Errorf("expected item name %s, but got %s", name, r.Name)
	}
}

func TestEnqueueRequestNode_Update(t *testing.T) {
	t.Run("node with nodepool label", func(t *testing.T) {
		e := &EnqueueRequestNode{}
		oldNode := newMockNode("test", "cn-beijing.i-2ze0rh9z4q62exljg99k", "", map[string]string{cloudNodepoolLabelKey: mockNodepoolName})
		newNode := newMockNode("test", "cn-beijing.i-2ze0rh9z4q62exljg99k", "", map[string]string{cloudNodepoolLabelKey: mockNodepoolName})

		q := workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "cloud_nodepool_life_cycle")
		evt := event.UpdateEvent{
			oldNode,
			newNode,
		}

		e.Update(evt, q)

		assertQueueItem(t, q, mockNodepoolName)
	})

	t.Run("node without label", func(t *testing.T) {
		e := &EnqueueRequestNode{}
		oldNode := newMockNode("test", "cn-beijing.i-2ze0rh9z4q62exljg99k", "", nil)
		newNode := newMockNode("test", "cn-beijing.i-2ze0rh9z4q62exljg99k", "", nil)

		q := workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "cloud_nodepool_life_cycle")
		evt := event.UpdateEvent{
			oldNode,
			newNode,
		}
		e.Update(evt, q)

		assertQueueNil(t, q)
	})

	t.Run("node without nodepool label", func(t *testing.T) {
		e := &EnqueueRequestNode{}
		oldNode := newMockNode("test", "cn-beijing.i-2ze0rh9z4q62exljg99k", "", map[string]string{"key1": "value1"})
		newNode := newMockNode("test", "cn-beijing.i-2ze0rh9z4q62exljg99k", "", map[string]string{"key1": "value1"})

		q := workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "cloud_nodepool_life_cycle")
		evt := event.UpdateEvent{
			oldNode,
			newNode,
		}
		e.Update(evt, q)

		assertQueueNil(t, q)
	})
}

func TestEnqueueRequestNode_Delete(t *testing.T) {
	t.Run("node with nodepool label", func(t *testing.T) {
		e := &EnqueueRequestNode{}
		mockNode := newMockNode("test", "cn-beijing.i-2ze0rh9z4q62exljg99k", "", map[string]string{cloudNodepoolLabelKey: mockNodepoolName})

		q := workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "cloud_nodepool_life_cycle")
		evt := event.DeleteEvent{
			mockNode,
			false,
		}

		e.Delete(evt, q)

		assertQueueItem(t, q, mockNodepoolName)
	})

	t.Run("node without label", func(t *testing.T) {
		e := &EnqueueRequestNode{}
		mockNode := newMockNode("test", "cn-beijing.i-2ze0rh9z4q62exljg99k", "", nil)

		q := workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "cloud_nodepool_life_cycle")
		evt := event.DeleteEvent{
			mockNode,
			false,
		}
		e.Delete(evt, q)

		assertQueueNil(t, q)
	})

	t.Run("node without nodepool label", func(t *testing.T) {
		e := &EnqueueRequestNode{}
		mockNode := newMockNode("test", "cn-beijing.i-2ze0rh9z4q62exljg99k", "", map[string]string{"key1": "value1"})

		q := workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "cloud_nodepool_life_cycle")
		evt := event.DeleteEvent{
			mockNode,
			false,
		}
		e.Delete(evt, q)

		assertQueueNil(t, q)
	})
}

func TestEnqueueRequestNode_Generic(t *testing.T) {
	t.Run("node with nodepool label", func(t *testing.T) {
		e := &EnqueueRequestNode{}
		mockNode := newMockNode("test", "cn-beijing.i-2ze0rh9z4q62exljg99k", "", map[string]string{cloudNodepoolLabelKey: mockNodepoolName})

		q := workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "cloud_nodepool_life_cycle")
		evt := event.GenericEvent{
			mockNode,
		}

		e.Generic(evt, q)

		assertQueueItem(t, q, mockNodepoolName)
	})

	t.Run("node without label", func(t *testing.T) {
		e := &EnqueueRequestNode{}
		mockNode := newMockNode("test", "cn-beijing.i-2ze0rh9z4q62exljg99k", "", nil)

		q := workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "cloud_nodepool_life_cycle")
		evt := event.GenericEvent{
			mockNode,
		}
		e.Generic(evt, q)

		assertQueueNil(t, q)
	})

	t.Run("node without nodepool label", func(t *testing.T) {
		e := &EnqueueRequestNode{}
		mockNode := newMockNode("test", "cn-beijing.i-2ze0rh9z4q62exljg99k", "", map[string]string{"key1": "value1"})

		q := workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "cloud_nodepool_life_cycle")
		evt := event.GenericEvent{
			mockNode,
		}
		e.Generic(evt, q)

		assertQueueNil(t, q)
	})
}

func assertQueueNil(t testing.TB, q workqueue.RateLimitingInterface) {
	t.Helper()

	if q.Len() != 0 {
		t.Errorf("expected queue q is nil")
	}
}
