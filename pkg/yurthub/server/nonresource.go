/*
Copyright 2020 The OpenYurt Authors.

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

package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"k8s.io/apiserver/pkg/endpoints/handlers/responsewriters"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
	"k8s.io/utils/pointer"

	"github.com/openyurtio/openyurt/cmd/yurthub/app/config"
	"github.com/openyurtio/openyurt/pkg/yurthub/cachemanager"
	"github.com/openyurtio/openyurt/pkg/yurthub/kubernetes/rest"
	"github.com/openyurtio/openyurt/pkg/yurthub/storage"
)

var nonResourceReqPaths = []string{
	"/version",
	"/apis/discovery.k8s.io/v1",
	"/apis/discovery.k8s.io/v1beta1",
}

type NonResourceHandler func(kubeClient *kubernetes.Clientset, sw cachemanager.StorageWrapper, path string) http.Handler

func wrapNonResourceHandler(proxyHandler http.Handler, config *config.YurtHubConfiguration, restMgr *rest.RestConfigManager) http.Handler {
	go prepareNonResourceInfo(restMgr, config.StorageWrapper)
	wrapMux := mux.NewRouter()

	// register handler for non resource requests
	for i := range nonResourceReqPaths {
		wrapMux.Handle(nonResourceReqPaths[i], localCacheHandler(nonResourceHandler, restMgr, config.StorageWrapper, nonResourceReqPaths[i])).Methods("GET")
	}

	// register handler for other requests
	wrapMux.PathPrefix("/").Handler(proxyHandler)
	return wrapMux
}

func localCacheHandler(handler NonResourceHandler, restMgr *rest.RestConfigManager, sw cachemanager.StorageWrapper, path string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := fmt.Sprintf("non-reosurce-info%s", path)
		restCfg := restMgr.GetRestConfig(true)
		if restCfg == nil {
			klog.Infof("get %s non resource data from local cache when cloud-edge line off", path)
			if nonResourceData, err := sw.GetRaw(key); err == nil {
				w.WriteHeader(http.StatusOK)
				writeRawJSON(nonResourceData, w)
			} else if err == storage.ErrStorageNotFound {
				w.WriteHeader(http.StatusNotFound)
				writeErrResponse(path, err, w)
			} else {
				w.WriteHeader(http.StatusInternalServerError)
				writeErrResponse(path, err, w)
			}
			return
		}

		kubeClient, err := kubernetes.NewForConfig(restCfg)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			writeErrResponse(path, err, w)
			return
		}
		handler(kubeClient, sw, path).ServeHTTP(w, r)
	})
}

func nonResourceHandler(kubeClient *kubernetes.Clientset, sw cachemanager.StorageWrapper, path string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := fmt.Sprintf("non-reosurce-info%s", path)
		result := kubeClient.RESTClient().Get().AbsPath(path).Do(context.TODO())
		code := pointer.IntPtr(0)
		result.StatusCode(code)
		if result.Error() != nil {
			err := result.Error()
			w.WriteHeader(*code)
			writeErrResponse(path, err, w)
		} else {
			body, _ := result.Raw()
			w.WriteHeader(*code)
			writeRawJSON(body, w)
			sw.UpdateRaw(key, body)
		}
	})
}

func writeErrResponse(path string, err error, w http.ResponseWriter) {
	klog.Errorf("failed to handle %s non resource request, %v", path, err)
	status := responsewriters.ErrorToAPIStatus(err)
	output, err := json.Marshal(status)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeRawJSON(output, w)
}

func writeRawJSON(output []byte, w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.Write(output)
}

func prepareNonResourceInfo(restMgr *rest.RestConfigManager, sw cachemanager.StorageWrapper) {
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	// if non resource data exist, exit prepare directly
	for _, path := range nonResourceReqPaths {
		key := fmt.Sprintf("non-reosurce-info%s", path)
		if _, err := sw.GetRaw(key); err == nil {
			klog.Infof("non resource cache exist, skip preparation")
			return
		}
	}

	for {
		select {
		case <-tick.C:
			restCfg := restMgr.GetRestConfig(true)
			if restCfg == nil {
				break
			}

			kubeClient, err := kubernetes.NewForConfig(restCfg)
			if err != nil {
				break
			}

			for _, path := range nonResourceReqPaths {
				key := fmt.Sprintf("non-reosurce-info%s", path)
				if nonResourceData, err := kubeClient.RESTClient().Get().AbsPath(path).Do(context.TODO()).Raw(); err != nil {
					continue
				} else {
					klog.Infof("non resource data(%s) is prepared for %s", string(nonResourceData), key)
					sw.UpdateRaw(key, nonResourceData)
				}
			}
			return
		}
	}
}
