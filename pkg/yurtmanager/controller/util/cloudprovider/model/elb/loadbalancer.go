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
	AddressType        string
	IsUserManaged      bool
	IsReUse            bool
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
}

func (n *NamedKey) Key() string {
	if n.Prefix == "" {
		n.Prefix = DEFAULT_PREFIX
	}
	return fmt.Sprintf("%s/%s/%s/%s", n.Prefix, n.ServiceName, n.Namespace, n.CID)
}

func (n *NamedKey) String() string {
	if n == nil {
		return ""
	}
	return n.Key()
}

func LoadNamedKey(key string) (*NamedKey, error) {
	metas := strings.Split(key, "/")
	if len(metas) != 4 || metas[0] != DEFAULT_PREFIX {
		return nil, fmt.Errorf("NamedKey Format Error: k8s.${port}.${protocol}.${service}.${namespace}.${clusterid} format is expected. Got [%s]", key)
	}
	return &NamedKey{
		CID:         metas[3],
		Namespace:   metas[2],
		ServiceName: metas[1],
		Prefix:      metas[0],
	}, nil
}

// EdgeLoadBalancer represents a AlibabaCloud ENS LoadBalancer.
type EdgeLoadBalancer struct {
	NamespacedName        types.NamespacedName
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

func (l *EdgeLoadBalancer) IsReUsed() bool {
	if l == nil {
		return false
	}
	return l.LoadBalancerAttribute.IsReUse
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

type ActionType string

const (
	Create = ActionType("Create")
	Update = ActionType("Update")
	Delete = ActionType("Delete")
)

type PoolIdentity struct {
	name         string
	network      string
	vswitch      string
	region       string
	loadbalancer string
	eip          string
	address      string
	action       ActionType
	err          error
}

func NewIdentity(name, network, vswitch, region string, action ActionType) *PoolIdentity {
	return &PoolIdentity{name: name, network: network, vswitch: vswitch, region: region, action: action}
}

func (i *PoolIdentity) GetName() string {
	return i.name
}

func (i *PoolIdentity) SetLoadBalancer(lb string) {
	i.loadbalancer = lb
}

func (i *PoolIdentity) GetLoadBalancer() string {
	return i.loadbalancer
}

func (i *PoolIdentity) SetEIP(eip string) {
	i.eip = eip
}

func (i *PoolIdentity) GetEIP() string {
	return i.eip
}

func (i *PoolIdentity) SetAddress(addr string) {
	i.address = addr
}

func (i *PoolIdentity) GetAddress() string {
	return i.address
}

func (i *PoolIdentity) GetNetwork() string {
	return i.network
}

func (i *PoolIdentity) SetNetwork(nw string) {
	i.network = nw
}

func (i *PoolIdentity) GetVSwitch() string {
	return i.vswitch
}

func (i *PoolIdentity) SetVSwitch(vs string) {
	i.vswitch = vs
}

func (i *PoolIdentity) GetRegion() string {
	return i.region
}

func (i *PoolIdentity) SetRegion(region string) {
	i.region = region
}

func (i *PoolIdentity) GetAction() ActionType {
	return i.action
}

func (i *PoolIdentity) SetAction(action ActionType) {
	i.action = action
}

func (i *PoolIdentity) SetError(err error) {
	i.err = err
}

func (i *PoolIdentity) GetError() error {
	return i.err
}
