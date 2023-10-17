package provider

import (
	"context"
	"time"

	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/raven/util/model"
)

type Provider interface {
	IAccessControlList
	IElasticIP
	ILoadBalancer
}

type RoleAuth struct {
	AccessKeyId     string
	AccessKeySecret string
	Expiration      time.Time
	SecurityToken   string
	LastUpdated     time.Time
	Code            string
}

type ILoadBalancer interface {
	CreateLoadBalancer(ctx context.Context, mdl *model.LoadBalancerAttribute) error
	DeleteLoadBalancer(ctx context.Context, mdl *model.LoadBalancerAttribute) error
	DescribeLoadBalancer(ctx context.Context, mdl *model.LoadBalancerAttribute) error
	DescribeLoadBalancers(ctx context.Context, mdl *model.LoadBalancerAttribute) error
}

type IAccessControlList interface {
	CreateAccessControlList(ctx context.Context, mdl *model.AccessControlListAttribute) error
	DeleteAccessControlList(ctx context.Context, mdl *model.AccessControlListAttribute) error
	AddAccessControlListEntry(ctx context.Context, mdl *model.AccessControlListAttribute, entry string) error
	RemoveAccessControlListEntry(ctx context.Context, mdl *model.AccessControlListAttribute, entry string) error
	DescribeAccessControlListAttribute(ctx context.Context, mdl *model.AccessControlListAttribute) error
	DescribeAccessControlLists(ctx context.Context, mdl *model.AccessControlListAttribute) error
}

type IElasticIP interface {
	AllocateEipAddress(ctx context.Context, mdl *model.ElasticIPAttribute) error
	AssociateEipAddress(ctx context.Context, mdl *model.ElasticIPAttribute, instanceId string) error
	UnassociateEipAddress(ctx context.Context, mdl *model.ElasticIPAttribute) error
	ReleaseEipAddress(ctx context.Context, mdl *model.ElasticIPAttribute) error
	DescribeEipAddresses(ctx context.Context, mdl *model.ElasticIPAttribute) error
}

type ITagResource interface {
	TagResource(ctx context.Context, tags *model.TagList, instance *model.Instance) error
}
