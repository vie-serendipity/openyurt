package elb

import (
	"fmt"
	"k8s.io/apimachinery/pkg/types"
	"strings"
)

type EdgeLoadBalancerAttribute struct {
	LoadBalancerId     string
	LoadBalancerName   string
	LoadBalancerStatus string
	EnsRegionId        string
	NetworkId          string
	VSwitchId          string
	Address            string
	LoadBalancerSpec   string
	CreateTime         string
	AddressIPVersion   string
	PayType            string

	AddressType   string
	IsUserManaged bool
	IsShared      bool
}

// Default Config
const (
	ELBDefaultSpec    = "elb.s2.medium"
	ELBDefaultPayType = "PostPaid"
)

const (
	InternetAddressType = "internet"
	IntranetAddressType = "intranet"

	IPv4 = "ipv4"
	IPv6 = "ipv6"
)

type FlagType string

const (
	OnFlag  = FlagType("on")
	OffFlag = FlagType("off")
)

var DEFAULT_PREFIX = "k8s"

type NamedKey struct {
	Prefix      string
	CID         string
	Namespace   string
	ServiceName string
	NodePool    string
}

func (n *NamedKey) Key() string {
	if n.Prefix == "" {
		n.Prefix = DEFAULT_PREFIX
	}
	return fmt.Sprintf("%s/%s/%s/%s/%s", n.Prefix, n.ServiceName, n.Namespace, n.NodePool, n.CID)
}

func (n *NamedKey) String() string {
	if n == nil {
		return ""
	}
	return n.Key()
}

func (n *NamedKey) ListenKey() string {
	if n.Prefix == "" {
		n.Prefix = DEFAULT_PREFIX
	}
	return fmt.Sprintf("%s/%s/%s/%s", n.Prefix, n.ServiceName, n.Namespace, n.CID)
}

func LoadNamedKey(key string) (*NamedKey, error) {
	metas := strings.Split(key, "/")
	if len(metas) != 5 || metas[0] != DEFAULT_PREFIX {
		return nil, fmt.Errorf("NamedKey Format Error: k8s.${port}.${protocol}.${service}.${namespace}.${nodepool}.${clusterid} format is expected. Got [%s]", key)
	}
	return &NamedKey{
		CID:         metas[4],
		NodePool:    metas[3],
		Namespace:   metas[2],
		ServiceName: metas[1],
		Prefix:      metas[0],
	}, nil
}

// EdgeLoadBalancer represents a AlibabaCloud ENS LoadBalancer.
type EdgeLoadBalancer struct {
	NamespacedName        types.NamespacedName
	NamedKey              NamedKey
	LoadBalancerAttribute EdgeLoadBalancerAttribute
	EipAttribute          EdgeEipAttribute
	ServerGroup           EdgeServerGroup
	Listeners             EdgeListeners
}

func (l *EdgeLoadBalancer) GetLoadBalancerId() string {
	if l == nil {
		return ""
	}
	return l.LoadBalancerAttribute.LoadBalancerId
}

func (l *EdgeLoadBalancer) GetLoadBalancerName() string {
	if l == nil {
		return ""
	}
	return l.LoadBalancerAttribute.LoadBalancerName
}

func (l *EdgeLoadBalancer) GetLoadBalancerAddress() string {
	if l == nil {
		return ""
	}
	return l.LoadBalancerAttribute.Address
}

func (l *EdgeLoadBalancer) GetNetworkId() string {
	if l == nil {
		return ""
	}
	return l.LoadBalancerAttribute.NetworkId
}

func (l *EdgeLoadBalancer) GetVSwitchId() string {
	if l == nil {
		return ""
	}
	return l.LoadBalancerAttribute.VSwitchId
}

func (l *EdgeLoadBalancer) GetRegionId() string {
	if l == nil {
		return ""
	}
	return l.LoadBalancerAttribute.EnsRegionId
}

func (l *EdgeLoadBalancer) IsShared() bool {
	if l == nil {
		return false
	}
	return l.LoadBalancerAttribute.IsShared
}

func (l *EdgeLoadBalancer) IsUserManaged() bool {
	if l == nil {
		return false
	}
	return l.LoadBalancerAttribute.IsUserManaged
}

func (l *EdgeLoadBalancer) GetEIPId() string {
	if l == nil {
		return ""
	}
	return l.EipAttribute.AllocationId
}

func (l *EdgeLoadBalancer) GetEIPAddress() string {
	if l == nil {
		return ""
	}
	return l.EipAttribute.IpAddress
}

func (l *EdgeLoadBalancer) GetAddressType() string {
	if l == nil {
		return ""
	}
	return l.LoadBalancerAttribute.AddressType
}
