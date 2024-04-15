package elb

import (
	"context"
	"fmt"
	"strings"

	"k8s.io/klog/v2"

	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/requests"
	"github.com/aliyun/alibaba-cloud-sdk-go/services/ens"
	elbmodel "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/util/cloudprovider/model/elb"
)

const (
	EIPIncorrectStatus = "IncorrectInstanceStatus"
	NotFoundMessage    = "find no"
	SLBInstanceType    = "SlbInstance"
)

func (e *ELBProvider) CreateEip(ctx context.Context, mdl *elbmodel.EdgeLoadBalancer) error {
	req := ens.CreateCreateEipInstanceRequest()
	req.ConnectTimeout = connectionTimeout
	req.ReadTimeout = readTimeout
	req.EnsRegionId = mdl.EipAttribute.EnsRegionId
	req.InstanceChargeType = mdl.EipAttribute.InstanceChargeType
	req.InternetChargeType = mdl.EipAttribute.InternetChargeType
	req.Name = mdl.EipAttribute.Name
	req.Isp = mdl.EipAttribute.InternetProviderService
	req.Bandwidth = requests.NewInteger(mdl.EipAttribute.Bandwidth)
	resp, err := e.auth.ELB.CreateEipInstance(req)
	if err != nil {
		return SDKError("CreateEipInstance", err)
	}
	mdl.EipAttribute.AllocationId = resp.AllocationId
	return nil
}

func (e *ELBProvider) ReleaseEip(ctx context.Context, mdl *elbmodel.EdgeLoadBalancer) error {
	req := ens.CreateReleaseInstanceRequest()
	req.ConnectTimeout = connectionTimeout
	req.ReadTimeout = readTimeout
	req.InstanceId = mdl.GetEIPId()
	_, err := e.auth.ELB.ReleaseInstance(req)
	if err != nil {
		return SDKError("ReleaseInstance", err)
	}
	return nil
}

func (e *ELBProvider) AssociateElbEipAddress(ctx context.Context, eipId, elbId string) error {
	req := ens.CreateAssociateEnsEipAddressRequest()
	req.ConnectTimeout = connectionTimeout
	req.ReadTimeout = readTimeout
	req.AllocationId = eipId
	req.InstanceId = elbId
	req.InstanceType = "SlbInstance"
	_, err := e.auth.ELB.AssociateEnsEipAddress(req)
	if err != nil {
		return SDKError("AssociateEnsEipAddress", err)
	}
	return nil
}

func (e *ELBProvider) UnAssociateElbEipAddress(ctx context.Context, eipId string) error {
	req := ens.CreateUnAssociateEnsEipAddressRequest()
	req.ConnectTimeout = connectionTimeout
	req.ReadTimeout = readTimeout
	req.AllocationId = eipId
	_, err := e.auth.ELB.UnAssociateEnsEipAddress(req)
	if err != nil {
		return SDKError("UnAssociateEnsEipAddress", err)
	}
	return nil
}

func (e *ELBProvider) ModifyEipAttribute(ctx context.Context, eipId string, mdl *elbmodel.EdgeLoadBalancer) error {
	req := ens.CreateModifyEnsEipAddressAttributeRequest()
	req.ConnectTimeout = connectionTimeout
	req.ReadTimeout = readTimeout
	req.AllocationId = eipId
	req.Bandwidth = requests.NewInteger(mdl.EipAttribute.Bandwidth)
	_, err := e.auth.ELB.ModifyEnsEipAddressAttribute(req)
	if err != nil {
		return SDKError("ModifyEnsEipAddressAttribute", err)
	}
	return nil
}

func (e *ELBProvider) DescribeEnsEipByInstanceId(ctx context.Context, elbId string, mdl *elbmodel.EdgeLoadBalancer) error {
	req := ens.CreateDescribeEnsEipAddressesRequest()
	req.ConnectTimeout = connectionTimeout
	req.ReadTimeout = readTimeout
	req.AssociatedInstanceId = elbId
	req.EnsRegionId = mdl.EipAttribute.EnsRegionId
	req.AssociatedInstanceType = SLBInstanceType
	resp, err := e.auth.ELB.DescribeEnsEipAddresses(req)
	if err != nil {
		return SDKError("DescribeEnsEipAddresses", err)
	}
	if resp == nil {
		klog.Errorf("RequestId: %s, eip address %s DescribeLoadBalancerAttribute response is nil", resp.RequestId, elbId)
		return fmt.Errorf("%s by eip allocation Id %s", NotFoundMessage, elbId)
	}
	var found = false
	var instance ens.EipAddress
	for idx := range resp.EipAddresses.EipAddress {
		if resp.EipAddresses.EipAddress[idx].InstanceId == elbId {
			instance = resp.EipAddresses.EipAddress[idx]
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("%s by eip allocation Id %s", NotFoundMessage, elbId)
	}
	loadEipResponse(instance, mdl)
	return nil
}

func (e *ELBProvider) DescribeEnsEipByName(ctx context.Context, eipName string, mdl *elbmodel.EdgeLoadBalancer) error {
	req := ens.CreateDescribeEnsEipAddressesRequest()
	req.ConnectTimeout = connectionTimeout
	req.ReadTimeout = readTimeout
	req.EnsRegionId = mdl.EipAttribute.EnsRegionId
	resp, err := e.auth.ELB.DescribeEnsEipAddresses(req)
	if err != nil {
		return SDKError("DescribeEnsEipAddresses", err)
	}
	if resp == nil {
		klog.Errorf("RequestId: %s, eip address %s DescribeLoadBalancerAttribute response is nil", resp.RequestId, eipName)
		return fmt.Errorf("%s by eip name %s", NotFoundMessage, eipName)
	}
	var found = false
	var instance ens.EipAddress
	for idx := range resp.EipAddresses.EipAddress {
		if resp.EipAddresses.EipAddress[idx].Name == eipName {
			instance = resp.EipAddresses.EipAddress[idx]
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("%s by eip name %s", NotFoundMessage, eipName)
	}
	loadEipResponse(instance, mdl)
	return nil
}

func (e *ELBProvider) DescribeEnsEipById(ctx context.Context, eipId string, mdl *elbmodel.EdgeLoadBalancer) error {
	req := ens.CreateDescribeEnsEipAddressesRequest()
	req.ConnectTimeout = connectionTimeout
	req.ReadTimeout = readTimeout
	req.AllocationId = eipId
	req.EnsRegionId = mdl.EipAttribute.EnsRegionId
	resp, err := e.auth.ELB.DescribeEnsEipAddresses(req)
	if err != nil {
		return SDKError("DescribeEnsEipAddresses", err)
	}
	if resp == nil {
		klog.Errorf("RequestId: %s, eip %s DescribeLoadBalancerAttribute response is nil", resp.RequestId, eipId)
		return fmt.Errorf("%s by eip id %s", NotFoundMessage, eipId)
	}
	var found = false
	var instance ens.EipAddress
	for idx := range resp.EipAddresses.EipAddress {
		if resp.EipAddresses.EipAddress[idx].AllocationId == eipId {
			instance = resp.EipAddresses.EipAddress[idx]
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("%s by eip id %s", NotFoundMessage, eipId)
	}
	loadEipResponse(instance, mdl)
	return nil
}

func loadEipResponse(eip ens.EipAddress, mdl *elbmodel.EdgeLoadBalancer) {
	mdl.EipAttribute.Name = eip.Name
	mdl.EipAttribute.AllocationId = eip.AllocationId
	mdl.EipAttribute.EnsRegionId = eip.EnsRegionId
	mdl.EipAttribute.IpAddress = eip.IpAddress
	mdl.EipAttribute.InternetChargeType = eip.InternetChargeType
	mdl.EipAttribute.InstanceChargeType = eip.ChargeType
	mdl.EipAttribute.Bandwidth = eip.Bandwidth
	mdl.EipAttribute.InstanceId = eip.InstanceId
	mdl.EipAttribute.InstanceType = eip.InstanceType
	mdl.EipAttribute.Status = eip.Status
	mdl.EipAttribute.Description = eip.Description
}

func IsNotFound(err error) bool {
	return strings.Contains(err.Error(), NotFoundMessage)
}
