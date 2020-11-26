package cloud

import (
	"fmt"
	"strconv"

	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/requests"
	"github.com/aliyun/alibaba-cloud-sdk-go/services/bssopenapi"
	"k8s.io/klog/v2"
)

// aliyun billing client
type BssClient struct {
	client *bssopenapi.Client
	region string
}

// create vsag instance with auto renewal, return sag id and error
func (b *BssClient) CreateVsagInstance(name, desc string, periodInMonth, bandWidth int) (string, error) {
	req := bssopenapi.CreateCreateInstanceRequest()
	req.Scheme = "https"
	req.ConnectTimeout = connectionTimeout
	req.ReadTimeout = readTimeout

	req.ProductCode = "smartag"
	// postpaid
	req.SubscriptionType = "Subscription"
	req.ProductType = "smartag_v"
	req.Period = requests.NewInteger(periodInMonth)
	// auto renewal
	req.RenewalStatus = "AutoRenewal"
	// set auto renewal period same as period
	req.RenewPeriod = requests.NewInteger(periodInMonth)
	req.Parameter = &[]bssopenapi.CreateInstanceParameter{
		{
			Code:  "type",
			Value: "sag-vcpe",
		},
		{
			Code:  "version",
			Value: "basic",
		},
		{
			Code:  "ha",
			Value: "warm_backup",
		},
		{
			Code:  "bandwidth",
			Value: strconv.Itoa(bandWidth),
		},
		{
			Code:  "ord_num",
			Value: "1",
		},
		{
			Code:  "name",
			Value: name,
		},
		{
			Code:  "description",
			Value: desc,
		},
		{
			Code:  "region",
			Value: b.region,
		},
	}

	klog.V(4).Infof("alicloud: CreateInstance req: %v", req)
	response, err := b.client.CreateInstance(req)
	if err != nil {
		return "", shrinkError(err)
	}
	klog.V(4).Infof("alicloud: CreateInstance response: %v", response.String())
	if !response.Success {
		return "", fmt.Errorf("create vsag instance failed with code %v, message: %v", response.Code, response.Message)
	}

	if response.Data.InstanceId == "" {
		return "", fmt.Errorf("create vsag instance succeed, while the response InstanceId is empty")
	}

	return response.Data.InstanceId, nil
}

func (b *BssClient) CancelAutoRenewal(sagID string) error {
	req := bssopenapi.CreateSetRenewalRequest()
	req.InstanceIDs = sagID
	req.RenewalStatus = "NotRenewal"
	req.ProductCode = "smartag"
	req.ProductType = "smartag_v"
	req.SubscriptionType = "Subscription"
	klog.V(4).Infof("alicloud: CancelAutoRenewal req: %v", req)
	response, err := b.client.SetRenewal(req)
	if err != nil {
		return shrinkError(err)
	}
	klog.V(4).Infof("alicloud: CancelAutoRenewal response: %v", response.String())
	if !response.Success {
		return fmt.Errorf("cancel auto-renewal for vsag instance %s failed with code %v, message: %v",
			sagID, response.Code, response.Message)
	}
	return nil
}
