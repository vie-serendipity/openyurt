package cloudprovider

import (
	"context"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/util/cloudprovider/model/elb"
	ravenmodel "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/util/cloudprovider/model/raven"
	"time"
)

type Provider interface {
	IMetaData
	IAccessControlList
	IElasticIP
	ILoadBalancer
	ITagResource
	IEnsLoadBalancer
}

type RoleAuth struct {
	AccessKeyId     string
	AccessKeySecret string
	Expiration      time.Time
	SecurityToken   string
	LastUpdated     time.Time
	Code            string
}

type IMetaData interface {
	GetClusterID() (string, error)
	GetRegion() (string, error)
	GetVpcID() (string, error)
	GetVswitchID() (string, error)
	GetUID() (string, error)
	RoleName() (string, error)
	RamRoleToken(role string) (RoleAuth, error)
}

type ILoadBalancer interface {
	CreateLoadBalancer(ctx context.Context, mdl *ravenmodel.LoadBalancerAttribute) error
	DeleteLoadBalancer(ctx context.Context, mdl *ravenmodel.LoadBalancerAttribute) error
	DescribeLoadBalancer(ctx context.Context, mdl *ravenmodel.LoadBalancerAttribute) error
	DescribeLoadBalancers(ctx context.Context, mdl *ravenmodel.LoadBalancerAttribute) error
}

type IAccessControlList interface {
	CreateAccessControlList(ctx context.Context, mdl *ravenmodel.AccessControlListAttribute) error
	DeleteAccessControlList(ctx context.Context, mdl *ravenmodel.AccessControlListAttribute) error
	AddAccessControlListEntry(ctx context.Context, mdl *ravenmodel.AccessControlListAttribute, entry string) error
	RemoveAccessControlListEntry(ctx context.Context, mdl *ravenmodel.AccessControlListAttribute, entry string) error
	DescribeAccessControlListAttribute(ctx context.Context, mdl *ravenmodel.AccessControlListAttribute) error
	DescribeAccessControlLists(ctx context.Context, mdl *ravenmodel.AccessControlListAttribute) error
}

type IElasticIP interface {
	AllocateEipAddress(ctx context.Context, mdl *ravenmodel.ElasticIPAttribute) error
	AssociateEipAddress(ctx context.Context, mdl *ravenmodel.ElasticIPAttribute, instanceId string) error
	UnassociateEipAddress(ctx context.Context, mdl *ravenmodel.ElasticIPAttribute) error
	ReleaseEipAddress(ctx context.Context, mdl *ravenmodel.ElasticIPAttribute) error
	DescribeEipAddresses(ctx context.Context, mdl *ravenmodel.ElasticIPAttribute) error
}

type ITagResource interface {
	TagResource(ctx context.Context, tags *ravenmodel.TagList, instance *ravenmodel.Instance) error
}

type IEnsLoadBalancer interface {
	// ELB
	FindEdgeLoadBalancer(ctx context.Context, mdl *elb.EdgeLoadBalancer) error
	CreateEdgeLoadBalancer(ctx context.Context, mdl *elb.EdgeLoadBalancer) error
	DeleteEdgeLoadBalancer(ctx context.Context, mdl *elb.EdgeLoadBalancer) error
	DescribeEdgeLoadBalancerById(ctx context.Context, lbId string, mdl *elb.EdgeLoadBalancer) error
	DescribeEdgeLoadBalancerByName(ctx context.Context, lbName string, mdl *elb.EdgeLoadBalancer) error
	SetEdgeLoadBalancerStatus(ctx context.Context, status string, mdl *elb.EdgeLoadBalancer) error
	FindHadLoadBalancerNetwork(ctx context.Context, lbName string) (network, vswtich, region []string, err error)

	//EIP
	CreateEip(ctx context.Context, mdl *elb.EdgeLoadBalancer) error
	ReleaseEip(ctx context.Context, mdl *elb.EdgeLoadBalancer) error
	ModifyEipAttribute(ctx context.Context, eipId string, mdl *elb.EdgeLoadBalancer) error
	AssociateElbEipAddress(ctx context.Context, eipId, lbId string) error
	UnAssociateElbEipAddress(ctx context.Context, eipId string) error
	DescribeEnsEipByInstanceId(ctx context.Context, instanceId string, mdl *elb.EdgeLoadBalancer) error
	DescribeEnsEipByName(ctx context.Context, eipName string, mdl *elb.EdgeLoadBalancer) error
	DescribeEnsEipById(ctx context.Context, eipId string, mdl *elb.EdgeLoadBalancer) error

	// Listener
	FindEdgeLoadBalancerListener(ctx context.Context, lbId string, listeners *elb.EdgeListeners) error
	DescribeEdgeLoadBalancerTCPListener(ctx context.Context, lbId string, port int, listener *elb.EdgeListenerAttribute) error
	DescribeEdgeLoadBalancerUDPListener(ctx context.Context, lbId string, port int, listener *elb.EdgeListenerAttribute) error
	DescribeEdgeLoadBalancerHTTPListener(ctx context.Context, lbId string, port int, listener *elb.EdgeListenerAttribute) error
	DescribeEdgeLoadBalancerHTTPSListener(ctx context.Context, lbId string, port int, listener *elb.EdgeListenerAttribute) error
	StartEdgeLoadBalancerListener(ctx context.Context, lbId string, port int, protocol string) error
	StopEdgeLoadBalancerListener(ctx context.Context, lbId string, port int, protocol string) error
	CreateEdgeLoadBalancerTCPListener(ctx context.Context, lbId string, listener *elb.EdgeListenerAttribute) error
	CreateEdgeLoadBalancerUDPListener(ctx context.Context, lbId string, listener *elb.EdgeListenerAttribute) error
	ModifyEdgeLoadBalancerTCPListener(ctx context.Context, lbId string, listener *elb.EdgeListenerAttribute) error
	ModifyEdgeLoadBalancerUDPListener(ctx context.Context, lbId string, listener *elb.EdgeListenerAttribute) error
	DeleteEdgeLoadBalancerListener(ctx context.Context, lbId string, port int, protocol string) error

	// Server Group
	AddBackendToEdgeServerGroup(ctx context.Context, lbId string, sg *elb.EdgeServerGroup) error
	UpdateEdgeServerGroup(ctx context.Context, lbId string, sg *elb.EdgeServerGroup) error
	RemoveBackendFromEdgeServerGroup(ctx context.Context, lbId string, sg *elb.EdgeServerGroup) error
	FindBackendFromLoadBalancer(ctx context.Context, lbId string, sg *elb.EdgeServerGroup) error

	// ENS instance
	GetEnsRegionIdByNetwork(ctx context.Context, networkId string) (string, error)
	FindNetWorkAndVSwitchByLoadBalancerId(ctx context.Context, lbId string) ([]string, error)
	FindEnsInstancesByNetwork(ctx context.Context, mdl *elb.EdgeLoadBalancer) (map[string]string, error)
	DescribeNetwork(ctx context.Context, mdl *elb.EdgeLoadBalancer) error
}
