/*
Copyright 2023 The OpenYurt Authors.

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

package mutetunnelnodes

import (
	"reflect"
	"testing"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/openyurtio/openyurt/pkg/util"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter"
)

func TestName(t *testing.T) {
	mtnf, _ := NewMutateTunnelNodesFilter()
	if mtnf.Name() != filter.MuteTunnelNodesFilterName {
		t.Errorf("expect %s, but got %s", filter.MuteTunnelNodesFilterName, mtnf.Name())
	}
}

func TestSupportedResourceAndVerbs(t *testing.T) {
	mtnf, _ := NewMutateTunnelNodesFilter()
	rvs := mtnf.SupportedResourceAndVerbs()
	if len(rvs) != 1 {
		t.Errorf("supported more than one resources, %v", rvs)
	}

	for resource, verbs := range rvs {
		if resource != "configmaps" {
			t.Errorf("expect resource is services, but got %s", resource)
		}

		if !verbs.Equal(sets.NewString("get", "list", "watch")) {
			t.Errorf("expect verbs are get/list/watch, but got %v", verbs.UnsortedList())
		}
	}
}

func TestRuntimeObjectFilter(t *testing.T) {
	mtnf, _ := NewMutateTunnelNodesFilter()

	testcases := map[string]struct {
		responseObject runtime.Object
		expectObject   runtime.Object
	}{
		"coredns configmap": {
			responseObject: &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      CorednsConfigMapName,
					Namespace: CorednsConfigMapNamespace,
				},
				Data: map[string]string{
					"Corefile": `.:53 {
        errors
        health :10260 {
           lameduck 15s
        }
        hosts /etc/edge/tunnel-nodes {
            reload 300ms
            fallthrough
        }
        ready
        kubeapi
        k8s_event {
          level info error warning
        }
        kubernetes cluster.local in-addr.arpa ip6.arpa {
          pods verified
          ttl 30
          fallthrough in-addr.arpa ip6.arpa
        }
        prometheus :9153
        forward . /etc/resolv.conf {
          prefer_udp
        }
        cache 30
        log
        loop
        reload
        loadbalance
    }`,
				},
			},
			expectObject: &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      CorednsConfigMapName,
					Namespace: CorednsConfigMapNamespace,
				},
				Data: map[string]string{
					"Corefile": `.:53 {
        errors
        health :10260 {
           lameduck 15s
        }
        ready
        kubeapi
        k8s_event {
          level info error warning
        }
        kubernetes cluster.local in-addr.arpa ip6.arpa {
          pods verified
          ttl 30
          fallthrough in-addr.arpa ip6.arpa
        }
        prometheus :9153
        forward . /etc/resolv.conf {
          prefer_udp
        }
        cache 30
        log
        loop
        reload
        loadbalance
    }`,
				},
			},
		},
		"coredns configmap without tunnelnodes": {
			responseObject: &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      CorednsConfigMapName,
					Namespace: CorednsConfigMapNamespace,
				},
				Data: map[string]string{
					"Corefile": `.:53 {
        errors
        health :10260 {
           lameduck 15s
        }
        ready
        kubeapi
        k8s_event {
          level info error warning
        }
        kubernetes cluster.local in-addr.arpa ip6.arpa {
          pods verified
          ttl 30
          fallthrough in-addr.arpa ip6.arpa
        }
        prometheus :9153
        forward . /etc/resolv.conf {
          prefer_udp
        }
        cache 30
        log
        loop
        reload
        loadbalance
    }`,
				},
			},
			expectObject: &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      CorednsConfigMapName,
					Namespace: CorednsConfigMapNamespace,
				},
				Data: map[string]string{
					"Corefile": `.:53 {
        errors
        health :10260 {
           lameduck 15s
        }
        ready
        kubeapi
        k8s_event {
          level info error warning
        }
        kubernetes cluster.local in-addr.arpa ip6.arpa {
          pods verified
          ttl 30
          fallthrough in-addr.arpa ip6.arpa
        }
        prometheus :9153
        forward . /etc/resolv.conf {
          prefer_udp
        }
        cache 30
        log
        loop
        reload
        loadbalance
    }`,
				},
			},
		},
		"configmapList with coredns configmap": {
			responseObject: &v1.ConfigMapList{
				Items: []v1.ConfigMap{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "foo",
							Namespace: "default",
						},
						Data: map[string]string{
							"foo": "bar",
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      CorednsConfigMapName,
							Namespace: CorednsConfigMapNamespace,
						},
						Data: map[string]string{
							"Corefile": `.:53 {
        errors
        health :10260 {
           lameduck 15s
        }
        hosts /etc/edge/tunnel-nodes {
            reload 300ms
            fallthrough
        }
        ready
        kubeapi
        k8s_event {
          level info error warning
        }
        kubernetes cluster.local in-addr.arpa ip6.arpa {
          pods verified
          ttl 30
          fallthrough in-addr.arpa ip6.arpa
        }
        prometheus :9153
        forward . /etc/resolv.conf {
          prefer_udp
        }
        cache 30
        log
        loop
        reload
        loadbalance
    }`,
						},
					},
				},
			},
			expectObject: &v1.ConfigMapList{
				Items: []v1.ConfigMap{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "foo",
							Namespace: "default",
						},
						Data: map[string]string{
							"foo": "bar",
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      CorednsConfigMapName,
							Namespace: CorednsConfigMapNamespace,
						},
						Data: map[string]string{
							"Corefile": `.:53 {
        errors
        health :10260 {
           lameduck 15s
        }
        ready
        kubeapi
        k8s_event {
          level info error warning
        }
        kubernetes cluster.local in-addr.arpa ip6.arpa {
          pods verified
          ttl 30
          fallthrough in-addr.arpa ip6.arpa
        }
        prometheus :9153
        forward . /etc/resolv.conf {
          prefer_udp
        }
        cache 30
        log
        loop
        reload
        loadbalance
    }`,
						},
					},
				},
			},
		},
		"coredns configmap has empty lines": {
			responseObject: &v1.ConfigMapList{
				Items: []v1.ConfigMap{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "foo",
							Namespace: "default",
						},
						Data: map[string]string{
							"foo": "bar",
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      CorednsConfigMapName,
							Namespace: CorednsConfigMapNamespace,
						},
						Data: map[string]string{
							"Corefile": `.:53 {
    errors
    health {
       lameduck 15s
    }
    ready
    kubeapi
    k8s_event {
      level info error warning
    }

    hosts /etc/edge/tunnel-nodes {
        reload 300ms
        fallthrough
    }
    kubernetes cluster.local in-addr.arpa ip6.arpa {

      pods verified
      ttl 30
      fallthrough in-addr.arpa ip6.arpa
    }
    prometheus :9153
    forward . /etc/resolv.conf {
      prefer_udp
    }
    cache 30
    log
    loop
    reload
    loadbalance
}`,
						},
					},
				},
			},
			expectObject: &v1.ConfigMapList{
				Items: []v1.ConfigMap{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "foo",
							Namespace: "default",
						},
						Data: map[string]string{
							"foo": "bar",
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      CorednsConfigMapName,
							Namespace: CorednsConfigMapNamespace,
						},
						Data: map[string]string{
							"Corefile": `.:53 {
    errors
    health {
       lameduck 15s
    }
    ready
    kubeapi
    k8s_event {
      level info error warning
    }

    kubernetes cluster.local in-addr.arpa ip6.arpa {

      pods verified
      ttl 30
      fallthrough in-addr.arpa ip6.arpa
    }
    prometheus :9153
    forward . /etc/resolv.conf {
      prefer_udp
    }
    cache 30
    log
    loop
    reload
    loadbalance
}`,
						},
					},
				},
			},
		},
		"not configmapList": {
			responseObject: &v1.PodList{
				Items: []v1.Pod{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "pod1",
							Namespace: CorednsConfigMapNamespace,
						},
						Spec: v1.PodSpec{
							Containers: []v1.Container{
								{
									Name:  "nginx",
									Image: "nginx",
								},
							},
						},
					},
				},
			},
			expectObject: &v1.PodList{
				Items: []v1.Pod{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "pod1",
							Namespace: CorednsConfigMapNamespace,
						},
						Spec: v1.PodSpec{
							Containers: []v1.Container{
								{
									Name:  "nginx",
									Image: "nginx",
								},
							},
						},
					},
				},
			},
		},
	}

	stopCh := make(<-chan struct{})
	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			newObj := mtnf.Filter(tc.responseObject, stopCh)
			if tc.expectObject == nil {
				if !util.IsNil(newObj) {
					t.Errorf("RuntimeObjectFilter expect nil obj, but got %v", newObj)
				}
			} else if !reflect.DeepEqual(newObj, tc.expectObject) {
				t.Errorf("RuntimeObjectFilter got error, expected: \n%v\nbut got: \n%v\n", tc.expectObject, newObj)
			}
		})
	}
}
