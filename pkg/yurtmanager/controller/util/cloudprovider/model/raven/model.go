package raven

import (
	"fmt"
	"os"
	"strings"
)

var DEFAULT_PREFIX = "k8s"

const (
	ComponentName = "raven-agent-ds"
	Namespace     = "kube-system"
)

type NamedKey struct {
	Prefix        string `json:"prefix"`
	CID           string `json:"CID"`
	Namespace     string `json:"namespace"`
	ComponentName string `json:"componentName"`
}

func (n *NamedKey) Key() string {
	if n.Prefix == "" {
		n.Prefix = DEFAULT_PREFIX
	}
	return fmt.Sprintf("%s/%s/%s/%s", n.Prefix, n.ComponentName, n.Namespace, n.CID)
}

func (n *NamedKey) String() string {
	if n == nil {
		return ""
	}
	if os.Getenv("ALIBABACLOUD_TYPE") == "Private" {
		return DedicatedKey(n.Key())
	}
	return n.Key()
}

func DedicatedKey(key string) string {
	return strings.Replace(key, "/", "_", -1)
}

func (n *NamedKey) DeepCopy() NamedKey {
	if n == nil {
		return NamedKey{}
	}
	return NamedKey{Prefix: n.Prefix, CID: n.CID, Namespace: n.Namespace, ComponentName: n.ComponentName}
}

func LoadNamedKey(key string) (*NamedKey, error) {
	metas := strings.Split(key, "/")
	if len(metas) != 4 || metas[0] != DEFAULT_PREFIX {
		return nil, fmt.Errorf("NamedKey Format Error: k8s.${componets}.${namespace}.${clusterid} format is expected. Got [%s]", key)
	}
	return &NamedKey{
		CID:           metas[3],
		Namespace:     metas[2],
		ComponentName: metas[1],
		Prefix:        metas[0],
	}, nil
}

func NewNamedKey(clusterId string) NamedKey {
	return NamedKey{Prefix: DEFAULT_PREFIX, CID: clusterId, ComponentName: ComponentName, Namespace: Namespace}
}

type TagManage interface {
	Convert(src *TagList)
}

type Tag struct {
	Key   string
	Value string
}

type TagList struct {
	Tags []Tag
}

func NewTagList(kv map[string]string) TagList {
	tags := make([]Tag, 0)
	if kv == nil {
		return TagList{tags}
	}
	for k, v := range kv {
		tags = append(tags, Tag{Key: k, Value: v})
	}
	return TagList{Tags: tags}
}

type Instance struct {
	InstanceId     string
	InstanceType   string
	InstanceRegion string
}

type ElasticIPAttribute struct {
	NamedKey           `json:"namedKey"`
	AllocationId       string `json:"allocationId"`
	Name               string `json:"name"`
	Region             string `json:"region"`
	Bandwidth          string `json:"bandwidth"`
	InstanceChargeType string `json:"instanceChargeType"`
	InternetChargeType string `json:"internetChargeType"`
	InstanceId         string `json:"instanceId"`
	Address            string `json:"address"`
	Status             string `json:"status"`
}

type LoadBalancerAttribute struct {
	NamedKey           `json:"namedKey"`
	LoadBalancerId     string `json:"loadBalancerId"`
	Name               string `json:"name"`
	Region             string `json:"region"`
	Spec               string `json:"spec"`
	InstanceChargeType string `json:"instanceChargeType"`
	InternetChargeType string `json:"internetChargeType"`
	Status             string `json:"status"`
	Address            string `json:"address"`
	AddressType        string `json:"addressType"`
	VpcId              string `json:"vpcId"`
	VSwitchId          string `json:"switchId"`
}

type AccessControlListAttribute struct {
	NamedKey            `json:"namedKey"`
	AccessControlListId string   `json:"accessControlListId"`
	Name                string   `json:"name"`
	Region              string   `json:"region"`
	LocalEntries        []string `json:"localEntries"`
	RemoteEntries       []string `json:"remoteEntries"`
}
