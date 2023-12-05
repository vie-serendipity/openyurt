package raven

import (
	"context"
	"fmt"
	provider "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/util/cloudprovider"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/util/cloudprovider/alibaba/base"
	ravenmodel "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/util/cloudprovider/model/raven"
	"strings"

	"github.com/aliyun/alibaba-cloud-sdk-go/services/vpc"
)

func NewVPCProvider(auth *base.ClientMgr) *VPCProvider {
	return &VPCProvider{auth: auth}
}

var _ provider.IElasticIP = &VPCProvider{}

type VPCProvider struct {
	auth *base.ClientMgr
}

func (v *VPCProvider) DescribeEipAddresses(ctx context.Context, mdl *ravenmodel.ElasticIPAttribute) error {
	req := vpc.CreateDescribeEipAddressesRequest()
	req.Scheme = "https"
	req.AllocationId = mdl.AllocationId
	req.EipName = mdl.String()
	resp, err := v.auth.EIP.DescribeEipAddresses(req)
	if err != nil {
		return EIPSDKError("DescribeEipAddresses", err)
	}
	if resp == nil {
		return fmt.Errorf("DescribeEipAddresses response is empty")
	}
	if len(resp.EipAddresses.EipAddress) < 1 {
		return fmt.Errorf("eip %s is not found", mdl.AllocationId)
	}
	for _, eip := range resp.EipAddresses.EipAddress {
		mdl.AllocationId = eip.AllocationId
		mdl.Address = eip.IpAddress
		mdl.Region = eip.RegionId
		mdl.Bandwidth = eip.Bandwidth
		mdl.Status = eip.Status
		mdl.InstanceId = eip.InstanceId
		mdl.Name = eip.Name
		return nil
	}

	return nil
}

func (v *VPCProvider) AllocateEipAddress(ctx context.Context, mdl *ravenmodel.ElasticIPAttribute) error {
	req := vpc.CreateAllocateEipAddressRequest()
	req.Scheme = "https"
	req.Name = mdl.String()
	req.RegionId = mdl.Region
	req.Bandwidth = mdl.Bandwidth

	resp, err := v.auth.EIP.AllocateEipAddress(req)
	if err != nil {
		return EIPSDKError("AllocateEipAddress", err)
	}
	if resp == nil || resp.AllocationId == "" || resp.EipAddress == "" {
		return fmt.Errorf("CreateLoadBalancer response is empty")
	}
	mdl.AllocationId = resp.AllocationId
	mdl.Address = resp.EipAddress
	return nil
}

func (v *VPCProvider) AssociateEipAddress(ctx context.Context, mdl *ravenmodel.ElasticIPAttribute, instanceId string) error {
	req := vpc.CreateAssociateEipAddressRequest()
	req.Scheme = "https"
	req.InstanceType = "SlbInstance"
	req.AllocationId = mdl.AllocationId
	req.InstanceId = instanceId
	_, err := v.auth.EIP.AssociateEipAddress(req)
	if err != nil {
		return EIPSDKError("AssociateEipAddress", err)
	}
	return nil
}

func (v *VPCProvider) UnassociateEipAddress(ctx context.Context, mdl *ravenmodel.ElasticIPAttribute) error {
	req := vpc.CreateUnassociateEipAddressRequest()
	req.Scheme = "https"
	req.InstanceType = "SlbInstance"
	req.AllocationId = mdl.AllocationId
	req.InstanceId = mdl.InstanceId
	_, err := v.auth.EIP.UnassociateEipAddress(req)
	if err != nil {
		return EIPSDKError("UnassociateEipAddress", err)
	}
	return nil
}

func (v *VPCProvider) ReleaseEipAddress(ctx context.Context, mdl *ravenmodel.ElasticIPAttribute) error {
	req := vpc.CreateReleaseEipAddressRequest()
	req.Scheme = "https"
	req.AllocationId = mdl.AllocationId
	_, err := v.auth.EIP.ReleaseEipAddress(req)
	if err != nil {
		return EIPSDKError("ReleaseEipAddress", err)
	}
	return nil
}

func EIPSDKError(api string, err error) error {
	return fmt.Errorf("[SDKError] API: vpc:%s, Error: %s", api, err.Error())
}

func IsNotFound(err error) bool {
	return strings.Contains(strings.ToLower(err.Error()), "is not found") || strings.Contains(strings.ToLower(err.Error()), "not exist")
}
