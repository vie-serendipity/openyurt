/*
Copyright 2023 The OpenYurt Authors.

Licensed under the Apache License, Version 2.0 (the License);
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an AS IS BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package util

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/rand"
	"net"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/loadbalancer/elb"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/util/cloudprovider/model/raven"
)

// GetNodeInternalIP returns internal ip of the given `node`.
func GetNodeInternalIP(node corev1.Node) string {
	var ip string
	for _, addr := range node.Status.Addresses {
		if addr.Type == corev1.NodeInternalIP && net.ParseIP(addr.Address) != nil {
			ip = addr.Address
			break
		}
	}
	return ip
}

// AddGatewayToWorkQueue adds the Gateway the reconciler's workqueue
func AddGatewayToWorkQueue(gwName string,
	q workqueue.RateLimitingInterface) {
	if gwName != "" {
		q.Add(reconcile.Request{
			NamespacedName: types.NamespacedName{Name: gwName},
		})
	}
}

func CheckServer(ctx context.Context, client client.Client) (enableProxy, enableTunnel bool) {
	var cm corev1.ConfigMap
	enableTunnel = false
	enableProxy = false
	err := client.Get(ctx, types.NamespacedName{Namespace: WorkingNamespace, Name: RavenGlobalConfig}, &cm)
	if err != nil {
		return enableProxy, enableTunnel
	}
	if val, ok := cm.Data[RavenEnableProxy]; ok && strings.ToLower(val) == "true" {
		enableProxy = true
	}
	if val, ok := cm.Data[RavenEnableTunnel]; ok && strings.ToLower(val) == "true" {
		enableTunnel = true
	}
	return enableProxy, enableTunnel
}

func AddDNSConfigmapToWorkQueue(q workqueue.RateLimitingInterface) {
	q.Add(reconcile.Request{
		NamespacedName: types.NamespacedName{Namespace: WorkingNamespace, Name: RavenProxyNodesConfig},
	})
}

func AddGatewayProxyInternalService(q workqueue.RateLimitingInterface) {
	q.Add(reconcile.Request{
		NamespacedName: types.NamespacedName{Namespace: WorkingNamespace, Name: GatewayProxyInternalService},
	})
}

func AddNodePoolToWorkQueue(npName string, q workqueue.RateLimitingInterface) {
	if npName != "" {
		q.Add(reconcile.Request{
			NamespacedName: types.NamespacedName{Name: npName},
		})
	}
}

type Model struct {
	ACLModel *raven.AccessControlListAttribute `json:"acl_model"`
	SLBModel *raven.LoadBalancerAttribute      `json:"slb_model"`
	EIPModel *raven.ElasticIPAttribute         `json:"eip_model"`
}

func NewModel(key raven.NamedKey, region string) *Model {
	return &Model{
		ACLModel: &raven.AccessControlListAttribute{NamedKey: key, Region: region},
		SLBModel: &raven.LoadBalancerAttribute{NamedKey: key, Region: region},
		EIPModel: &raven.ElasticIPAttribute{NamedKey: key, Region: region},
	}
}

type RequestContext struct {
	Ctx context.Context
	Cm  *corev1.ConfigMap
}

func (r *RequestContext) GetEIPId() string {
	if r.Cm == nil || r.Cm.Data == nil {
		return ""
	}
	return r.Cm.Data[ElasticIPId]
}

func (r *RequestContext) GetLoadBalancerId() string {
	if r.Cm == nil || r.Cm.Data == nil {
		return ""
	}
	return r.Cm.Data[LoadBalancerId]
}

func (r *RequestContext) GetEIPAddress() string {
	if r.Cm == nil || r.Cm.Data == nil {
		return ""
	}
	return r.Cm.Data[ElasticIPIP]
}

func (r *RequestContext) GetLoadBalancerAddress() string {
	if r.Cm == nil || r.Cm.Data == nil {
		return ""
	}
	return r.Cm.Data[LoadBalancerIP]
}

func (r *RequestContext) GetACLId() string {
	if r.Cm == nil || r.Cm.Data == nil {
		return ""
	}
	return r.Cm.Data[ACLId]
}

func HashObject(o interface{}) string {
	data, _ := json.Marshal(o)
	var a interface{}
	err := json.Unmarshal(data, &a)
	if err != nil {
		klog.Errorf("unmarshal: %s", err.Error())
	}
	return computeHash(PrettyYaml(a))
}

func PrettyYaml(obj interface{}) string {
	bs, err := yaml.Marshal(obj)
	if err != nil {
		klog.Errorf("could not parse yaml, %v", err.Error())
	}
	return string(bs)
}

func computeHash(target string) string {
	hash := sha256.Sum224([]byte(target))
	return strings.ToLower(hex.EncodeToString(hash[:]))
}

func FormatName(name string) string {
	return strings.Join([]string{name, fmt.Sprintf("%08x", rand.Uint32())}, "-")
}

func GetResourceHash(model *Model) string {
	var op []string
	op = append(op, model.SLBModel.LoadBalancerId, model.SLBModel.Address)
	op = append(op, model.ACLModel.AccessControlListId)
	op = append(op, model.EIPModel.AllocationId, model.EIPModel.Address)
	return elb.HashObject(op)
}

func GetPrevResourceHash(data map[string]string) string {
	var lbId, lbIp, aclId, eipId, eipIp string
	if data != nil {
		lbId = data[LoadBalancerId]
		lbIp = data[LoadBalancerIP]
		aclId = data[ACLId]
		eipId = data[ElasticIPId]
		eipIp = data[ElasticIPIP]
	}
	var op []string
	op = append(op, lbId, lbIp)
	op = append(op, aclId)
	op = append(op, eipId, eipIp)
	return elb.HashObject(op)
}

func Retry(
	backoff *wait.Backoff,
	fun func(cm *corev1.ConfigMap) error,
	cm *corev1.ConfigMap,
) error {
	if backoff == nil {
		backoff = &wait.Backoff{
			Duration: 1 * time.Second,
			Steps:    8,
			Factor:   2,
			Jitter:   4,
		}
	}
	return wait.ExponentialBackoff(
		*backoff,
		func() (bool, error) {
			err := fun(cm)
			if err != nil &&
				strings.Contains(err.Error(), "try again") {
				klog.Errorf("retry with error: %s", err.Error())
				return false, nil
			}
			if err != nil {
				klog.Errorf("retry error: NotRetry, %s", err.Error())
			}
			return true, nil
		},
	)
}
