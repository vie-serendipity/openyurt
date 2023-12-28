package raven

import (
	"context"
	"fmt"
	provider "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/util/cloudprovider"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/util/cloudprovider/alibaba/base"
	ravenmodel "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/util/cloudprovider/model/raven"

	"github.com/aliyun/alibaba-cloud-sdk-go/services/slb"
)

func NewLBProvider(auth *base.ClientMgr) *SLBProvider {
	return &SLBProvider{auth: auth}
}

var _ provider.ILoadBalancer = &SLBProvider{}
var _ provider.IAccessControlList = &SLBProvider{}
var _ provider.ITagResource = &SLBProvider{}

const (
	LoadBalancerResourceType      = "instance"
	AccessControlListResourceType = "acl"
)

type SLBProvider struct {
	auth *base.ClientMgr
}

func (s *SLBProvider) DescribeLoadBalancers(ctx context.Context, mdl *ravenmodel.LoadBalancerAttribute) error {
	req := slb.CreateDescribeLoadBalancersRequest()
	req.RegionId = mdl.Region
	req.LoadBalancerName = mdl.Name
	resp, err := s.auth.SLB.DescribeLoadBalancers(req)
	if err != nil {
		return CLBSDKError("DescribeLoadBalancers", err)
	}
	if resp == nil {
		return fmt.Errorf("DescribeLoadBalancers response is empty")
	}
	for _, lb := range resp.LoadBalancers.LoadBalancer {
		if lb.VpcId != mdl.VpcId || lb.VSwitchId != mdl.VSwitchId || lb.LoadBalancerName != mdl.Name {
			continue
		}
		mdl.LoadBalancerId = lb.LoadBalancerId
		mdl.Address = lb.Address
		mdl.Spec = lb.LoadBalancerSpec
		return nil
	}
	return nil
}

func (s *SLBProvider) TagResource(ctx context.Context, tags *ravenmodel.TagList, instance *ravenmodel.Instance) error {
	req := slb.CreateTagResourcesRequest()
	req.RegionId = instance.InstanceRegion
	req.ResourceId = &[]string{instance.InstanceId}
	req.ResourceType = instance.InstanceType
	resourceTags := loadTagResource(tags)
	req.Tag = &resourceTags
	_, err := s.auth.SLB.TagResources(req)
	if err != nil {
		return CLBSDKError("TagResources", err)
	}
	return nil
}

func (s *SLBProvider) CreateLoadBalancer(ctx context.Context, mdl *ravenmodel.LoadBalancerAttribute) error {
	req := slb.CreateCreateLoadBalancerRequest()
	req.LoadBalancerName = mdl.String()
	req.RegionId = mdl.Region
	req.LoadBalancerSpec = mdl.Spec
	req.AddressType = mdl.AddressType
	req.VpcId = mdl.VpcId
	req.VSwitchId = mdl.VSwitchId
	resp, err := s.auth.SLB.CreateLoadBalancer(req)
	if err != nil {
		return CLBSDKError("CreateLoadBalancer", err)
	}
	if resp == nil || resp.LoadBalancerId == "" || resp.Address == "" {
		return fmt.Errorf("CreateLoadBalancer response is empty")
	}
	mdl.LoadBalancerId = resp.LoadBalancerId
	mdl.Address = resp.Address
	return nil
}

func (s *SLBProvider) DeleteLoadBalancer(ctx context.Context, mdl *ravenmodel.LoadBalancerAttribute) error {
	req := slb.CreateDeleteLoadBalancerRequest()
	req.LoadBalancerId = mdl.LoadBalancerId
	_, err := s.auth.SLB.DeleteLoadBalancer(req)
	if err != nil {
		return CLBSDKError("DeleteLoadBalancer", err)
	}
	return nil
}

func (s *SLBProvider) DescribeLoadBalancer(ctx context.Context, mdl *ravenmodel.LoadBalancerAttribute) error {
	req := slb.CreateDescribeLoadBalancerAttributeRequest()
	req.LoadBalancerId = mdl.LoadBalancerId
	resp, err := s.auth.SLB.DescribeLoadBalancerAttribute(req)
	if err != nil {
		return CLBSDKError("DescribeLoadBalancerAttribute", err)
	}
	if resp == nil {
		return fmt.Errorf("DescribeLoadBalancer is nil")
	}
	if resp.LoadBalancerName != mdl.Name {
		mdl.LoadBalancerId = ""
		mdl.Address = ""
		return nil
	}
	mdl.Name = resp.LoadBalancerName
	mdl.Region = resp.RegionId
	mdl.Spec = resp.LoadBalancerSpec
	mdl.Status = resp.LoadBalancerStatus
	mdl.VpcId = resp.VpcId
	mdl.VSwitchId = resp.VSwitchId
	mdl.Address = resp.Address
	mdl.AddressType = resp.AddressType
	return nil
}

func (s *SLBProvider) DescribeAccessControlLists(ctx context.Context, mdl *ravenmodel.AccessControlListAttribute) error {
	req := slb.CreateDescribeAccessControlListsRequest()
	req.RegionId = mdl.Region
	req.AclName = mdl.Name
	resp, err := s.auth.SLB.DescribeAccessControlLists(req)
	if err != nil {
		return CLBSDKError("DescribeAccessControlLists", err)
	}
	if resp == nil {
		return fmt.Errorf("DescribeAccessControlLists response is empty")
	}
	for _, acl := range resp.Acls.Acl {
		if acl.AclName == req.AclName {
			mdl.AccessControlListId = acl.AclId
			return nil
		}
	}
	return nil
}

func (s *SLBProvider) CreateAccessControlList(ctx context.Context, mdl *ravenmodel.AccessControlListAttribute) error {
	req := slb.CreateCreateAccessControlListRequest()
	req.RegionId = mdl.Region
	req.AclName = mdl.NamedKey.String()
	resp, err := s.auth.SLB.CreateAccessControlList(req)
	if err != nil {
		return CLBSDKError("CreateAccessControlList", err)
	}
	if resp == nil || resp.AclId == "" {
		return fmt.Errorf("CreateAccessControlList response is empty")
	}
	mdl.AccessControlListId = resp.AclId
	return nil
}

func (s *SLBProvider) DeleteAccessControlList(ctx context.Context, mdl *ravenmodel.AccessControlListAttribute) error {
	req := slb.CreateDeleteAccessControlListRequest()
	req.RegionId = mdl.Region
	req.AclId = mdl.AccessControlListId
	_, err := s.auth.SLB.DeleteAccessControlList(req)
	if err != nil {
		return CLBSDKError("DeleteAccessControlList", err)
	}
	return nil
}

func (s *SLBProvider) AddAccessControlListEntry(ctx context.Context, mdl *ravenmodel.AccessControlListAttribute, entry string) error {
	if entry == "" || entry == "[]" {
		return nil
	}
	req := slb.CreateAddAccessControlListEntryRequest()
	req.RegionId = mdl.Region
	req.AclId = mdl.AccessControlListId
	req.AclEntrys = entry
	_, err := s.auth.SLB.AddAccessControlListEntry(req)
	if err != nil {
		return CLBSDKError("AddAccessControlListEntry", err)
	}
	return nil
}

func (s *SLBProvider) RemoveAccessControlListEntry(ctx context.Context, mdl *ravenmodel.AccessControlListAttribute, entry string) error {
	if entry == "" || entry == "[]" {
		return nil
	}
	req := slb.CreateRemoveAccessControlListEntryRequest()
	req.RegionId = mdl.Region
	req.AclId = mdl.AccessControlListId
	req.AclEntrys = entry
	_, err := s.auth.SLB.RemoveAccessControlListEntry(req)
	if err != nil {
		return CLBSDKError("RemoveAccessControlListEntry", err)
	}
	return nil
}

func (s *SLBProvider) DescribeAccessControlListAttribute(ctx context.Context, mdl *ravenmodel.AccessControlListAttribute) error {
	req := slb.CreateDescribeAccessControlListAttributeRequest()
	req.RegionId = mdl.Region
	req.AclId = mdl.AccessControlListId
	resp, err := s.auth.SLB.DescribeAccessControlListAttribute(req)
	if err != nil {
		return CLBSDKError("DescribeAccessControlListAttribute", err)
	}
	if resp == nil {
		return fmt.Errorf("DescribeAccessControlListAttribute response is empty")
	}
	mdl.RemoteEntries = aclEntryConvertEntry(resp.AclEntrys)
	mdl.Name = resp.AclName
	return nil
}

func CLBSDKError(api string, err error) error {
	return fmt.Errorf("[SDKError] API: slb:%s, Error: %s", api, err.Error())
}

func loadTagResource(src *ravenmodel.TagList) (dst []slb.TagResourcesTag) {
	dst = make([]slb.TagResourcesTag, 0)
	for _, tag := range src.Tags {
		dst = append(dst, slb.TagResourcesTag{Key: tag.Key, Value: tag.Value})
	}
	return
}

func aclEntryConvertEntry(src slb.AclEntrys) []string {
	ret := make([]string, 0)
	for _, entry := range src.AclEntry {
		if entry.AclEntryIP == "" {
			continue
		}
		ret = append(ret, entry.AclEntryIP)
	}
	return ret
}
