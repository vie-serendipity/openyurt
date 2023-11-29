package raven

import (
	"fmt"
	"strings"
)

var DEFAULT_PREFIX = "k8s"

const (
	ComponentName = "raven-agent-ds"
	Namespace     = "kube-system"
)

type NamedKey struct {
	Prefix        string
	CID           string
	Namespace     string
	ComponentName string
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
	return n.Key()
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
	NamedKey
	AllocationId       string
	Name               string
	Region             string
	Bandwidth          string
	InstanceChargeType string
	InternetChargeType string
	InstanceId         string
	Address            string
	Status             string
	Tags               TagList
}

type LoadBalancerAttribute struct {
	NamedKey
	LoadBalancerId string
	Name           string
	Region         string
	Spec           string
	Status         string
	Address        string
	AddressType    string
	VpcId          string
	VSwitchId      string
	Tags           TagList
}

type AccessControlListAttribute struct {
	NamedKey
	AccessControlListId string
	Name                string
	Region              string
	Tags                TagList
	LocalEntries        []string
	RemoteEntries       []string
}
