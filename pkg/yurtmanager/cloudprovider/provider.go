package cloudprovider

import (
	"context"
	"time"

	ravenmodel "github.com/openyurtio/openyurt/pkg/yurtmanager/cloudprovider/model/raven"
)

type Provider interface {
	IMetaData
	IAccessControlList
	IElasticIP
	ILoadBalancer
	ITagResource
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
	GetAccessID() (string, error)
	GetAccessSecret() (string, error)
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
