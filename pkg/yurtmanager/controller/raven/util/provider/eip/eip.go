package eip

import (
	"context"
	"fmt"
	"strings"

	"github.com/aliyun/alibaba-cloud-sdk-go/services/vpc"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/raven/util/model"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/raven/util/provider"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/raven/util/provider/base"
)

func NewEIPProvider(auth *base.ClientMgr) *VPCProvider {
	return &VPCProvider{auth: auth}
}

var _ provider.IElasticIP = &VPCProvider{}

type VPCProvider struct {
	auth *base.ClientMgr
}

func (v *VPCProvider) DescribeEipAddresses(ctx context.Context, mdl *model.ElasticIPAttribute) error {
	req := vpc.CreateDescribeEipAddressesRequest()
	req.Scheme = "https"
	req.AllocationId = mdl.AllocationId
	req.EipName = mdl.String()
	resp, err := v.auth.EIP.DescribeEipAddresses(req)
	if err != nil {
		return SDKError("DescribeEipAddresses", err)
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

func (v *VPCProvider) AllocateEipAddress(ctx context.Context, mdl *model.ElasticIPAttribute) error {
	req := vpc.CreateAllocateEipAddressRequest()
	req.Scheme = "https"
	req.Name = mdl.String()
	req.RegionId = mdl.Region
	req.Bandwidth = mdl.Bandwidth

	resp, err := v.auth.EIP.AllocateEipAddress(req)
	if err != nil {
		return SDKError("AllocateEipAddress", err)
	}
	if resp == nil || resp.AllocationId == "" || resp.EipAddress == "" {
		return fmt.Errorf("CreateLoadBalancer response is empty")
	}
	mdl.AllocationId = resp.AllocationId
	mdl.Address = resp.EipAddress
	return nil
}

func (v *VPCProvider) AssociateEipAddress(ctx context.Context, mdl *model.ElasticIPAttribute, instanceId string) error {
	req := vpc.CreateAssociateEipAddressRequest()
	req.Scheme = "https"
	req.InstanceType = "SlbInstance"
	req.AllocationId = mdl.AllocationId
	req.InstanceId = instanceId
	_, err := v.auth.EIP.AssociateEipAddress(req)
	if err != nil {
		return SDKError("AssociateEipAddress", err)
	}
	return nil
}

func (v *VPCProvider) UnassociateEipAddress(ctx context.Context, mdl *model.ElasticIPAttribute) error {
	req := vpc.CreateUnassociateEipAddressRequest()
	req.Scheme = "https"
	req.InstanceType = "SlbInstance"
	req.AllocationId = mdl.AllocationId
	req.InstanceId = mdl.InstanceId
	_, err := v.auth.EIP.UnassociateEipAddress(req)
	if err != nil {
		return SDKError("UnassociateEipAddress", err)
	}
	return nil
}

func (v *VPCProvider) ReleaseEipAddress(ctx context.Context, mdl *model.ElasticIPAttribute) error {
	req := vpc.CreateReleaseEipAddressRequest()
	req.Scheme = "https"
	req.AllocationId = mdl.AllocationId
	_, err := v.auth.EIP.ReleaseEipAddress(req)
	if err != nil {
		return SDKError("ReleaseEipAddress", err)
	}
	return nil
}

func SDKError(api string, err error) error {
	return fmt.Errorf("[SDKError] API: vpc:%s, Error: %s", api, err.Error())
}

func IsNotFound(err error) bool {
	return strings.Contains(strings.ToLower(err.Error()), "is not found")
}
