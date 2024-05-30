/*
Copyright 2024 The OpenYurt Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package storage

import (
	"context"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
)

type Controller struct {
	queue Interface
	store Store
}

func NewController(queue Interface, store Store) *Controller {
	return &Controller{queue: queue, store: store}
}

func (c *Controller) Run(ctx context.Context, workers int) {
	for i := 0; i < workers; i++ {
		go wait.UntilWithContext(ctx, c.worker, time.Second)
	}
}

func (c *Controller) worker(ctx context.Context) {
	for c.processNextWorkItem(ctx) {
	}
}

func (c *Controller) processNextWorkItem(ctx context.Context) bool {
	key, items, quit := c.queue.Get()
	if quit {
		return false
	}
	err := c.syncHandler(ctx, key, items)
	c.handleErr(ctx, err, key)

	return true
}

func (c *Controller) syncHandler(ctx context.Context, key Key, items Items) error {
	if key.IsRootKey() {
		objs := make([]runtime.Object, len(items))
		for i := 0; i < len(objs); i++ {
			objs[i] = items[i].Object
		}
		return c.store.Replace(key, objs)
	}

	item := items[len(items)-1]
	var err error
	switch item.Verb {
	case "create":
		err = c.store.Create(key, item.Object)
	case "update":
		_, err = c.store.Update(key, item.Object, item.ResourceVersion)
	case "delete":
		err = c.store.Delete(key)
	}
	return err
}

func (c *Controller) handleErr(ctx context.Context, err error, key Key) {

}

func (c *Controller) compress(items Items) {

}