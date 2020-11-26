package cloud

import (
	"fmt"
	"strings"
	"time"

	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/errors"
	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/requests"
	"github.com/aliyun/alibaba-cloud-sdk-go/services/smartag"
	"k8s.io/klog/v2"
)

const (
	// use this description to denote it's created by ack edge controller
	ManagedAckDescription = "managed-by-ack-edge"

	emptyField = "-"
)

type ErrorCode string

const (
	ErrorCodeSagNotFound     ErrorCode = "SAG.InstanceNoFound"
	ErrorCodeSagNotBind      ErrorCode = "SmartAccessGatewayNotBind"
	ErrorCodeSagNotActivated ErrorCode = "SmartAccessGatewayNotActivated"
	ErrorCodeSagCidrConflict ErrorCode = "CidrConflict"
)

const trueVal = true

type SagClient struct {
	client *smartag.Client
	region string
}

type HaState string

const (
	HaStateActive  HaState = "Active"
	HaStateStandby HaState = "Standby"
)

type SagStatus string

const (
	SagOrdered SagStatus = "Ordered"
	SagOffline SagStatus = "Offline"
	SagOnline  SagStatus = "Online"
)

type SagCredential struct {
	Sn      string
	Key     string
	HaState HaState
}

func (s *SagClient) CreateVsag(name, desc string, periodInMonth, bandWidth int) (string, error) {
	req := smartag.CreateCreateSmartAccessGatewayRequest()
	req.Scheme = "https"
	req.ConnectTimeout = connectionTimeout
	req.ReadTimeout = readTimeout
	req.HardWareSpec = "sag-vcpe"
	req.Period = requests.NewInteger(periodInMonth)
	req.ReceiverCountry = emptyField
	req.ReceiverState = emptyField
	req.ReceiverCity = emptyField
	req.ReceiverDistrict = emptyField
	req.ReceiverTown = emptyField
	req.ReceiverZip = emptyField
	req.ReceiverMobile = emptyField
	req.ReceiverName = emptyField
	req.ReceiverEmail = emptyField
	req.BuyerMessage = emptyField
	req.ReceiverAddress = emptyField
	req.HaType = "warm_backup"
	req.ChargeType = "PREPAY"
	req.MaxBandWidth = requests.NewInteger(bandWidth)
	req.RegionId = s.region
	req.AutoPay = requests.NewBoolean(true)
	req.Name = name
	req.Description = desc

	klog.V(4).Infof("alicloud: CreateSmartAccessGateway req: %v", req)
	response, err := s.client.CreateSmartAccessGateway(req)
	if err != nil {
		return "", shrinkError(err)
	}
	klog.V(4).Infof("alicloud: CreateSmartAccessGateway response: %v", response.String())

	return response.SmartAGId, nil
}

func (s *SagClient) DeleteVsag(sagId string) error {
	req := smartag.CreateDeleteSmartAccessGatewayRequest()
	req.Scheme = "https"
	req.ConnectTimeout = connectionTimeout
	req.ReadTimeout = readTimeout
	req.InstanceId = sagId
	req.RegionId = s.region

	klog.V(4).Infof("alicloud: DeleteSmartAccessGateway req: %v", req)
	response, err := s.client.DeleteSmartAccessGateway(req)
	if err != nil {
		return shrinkError(err)
	}
	klog.V(4).Infof("alicloud: DeleteSmartAccessGateway response: %v", response.String())

	return nil
}

func (s *SagClient) GetCredential(sagId string) ([]*SagCredential, error) {
	var response *smartag.DescribeSmartAccessGatewayAttributeResponse
	var err error
	// sag creation takes time, wait for it
	// retry for 15 times, with 2s interval
	for i := 0; i <= 15; i++ {
		response, err = s.describeSagAttr(sagId)
		if err != nil {
			// for sag not found error, wait and retry
			if i == 15 {
				return nil, shrinkError(err)
			}
			if serverErr, ok := err.(*errors.ServerError); ok && serverErr.ErrorCode() == string(ErrorCodeSagNotFound) {
				klog.V(4).Infof("alicloud: sag %s not found yet, will wait and retry: %v", sagId, serverErr.Error())
				time.Sleep(2 * time.Second)
				continue
			}
			return nil, shrinkError(err)
		}
		// for sag device not found or sag status still in "ordered", wait and retry
		if response == nil || len(response.Devices.Device) == 0 || response.Status == string(SagOrdered) {
			if i == 15 {
				return nil, fmt.Errorf("alicloud: DescribeSmartAccessGatewayAttribute get empty device field, sagId: %s, status: %v", sagId, response.Status)
			}
			klog.V(4).Infof("alicloud: DescribeSmartAccessGatewayAttribute get empty device field, sagId: %s, status: %v, will wait and retry", sagId, response.Status)
			time.Sleep(2 * time.Second)
			continue
		} else {
			break
		}
	}

	ret := []*SagCredential{}
	for _, device := range response.Devices.Device {
		ret = append(ret, &SagCredential{
			Sn:      device.SerialNumber,
			Key:     device.SecretKey,
			HaState: HaState(device.HaState),
		})
	}
	return ret, nil
}

func (s *SagClient) BindCCN(sagId, ccnId, sagUid string) error {
	var err error
	// sag activation takes time, wait for it
	// retry for 10 times, with 2s interval
	for i := 0; i <= 10; i++ {
		err = s.bindCCN(sagId, ccnId, sagUid)
		if err != nil {
			if i == 10 {
				return shrinkError(err)
			}
			if serverErr, ok := err.(*errors.ServerError); ok && serverErr.ErrorCode() == string(ErrorCodeSagNotActivated) {
				klog.V(4).Infof("alicloud: sag %s not activated yet, will wait and retry: %v", sagId, serverErr.Error())
				time.Sleep(2 * time.Second)
				continue
			}
			return shrinkError(err)
		} else {
			break
		}
	}
	return nil
}

// user sag client
func (s *SagClient) bindCCN(sagId, ccnId, sagUid string) error {
	req := smartag.CreateBindSmartAccessGatewayRequest()
	req.Scheme = "https"
	req.ConnectTimeout = connectionTimeout
	req.ReadTimeout = readTimeout
	req.RegionId = s.region
	req.SmartAGId = sagId
	req.CcnId = ccnId

	req.SmartAGUid = requests.Integer(sagUid)

	klog.V(4).Infof("alicloud: BindSmartAccessGateway req: %v", req)
	response, err := s.client.BindSmartAccessGateway(req)
	if err != nil {
		return err
	}
	klog.V(4).Infof("alicloud: BindSmartAccessGateway response: %v", response.String())
	// if sag is stuck in unbinding status, api sdk won't return error, we need to check this case from http response content
	if strings.Contains(response.BaseResponse.GetHttpContentString(), "The specified instance id unbinding") {
		return fmt.Errorf("sag %s is stuck in unbinding status", sagId)
	}

	return nil
}

// service sag client
func (s *SagClient) UnBindCCN(sagId, ccnId string) error {
	req := smartag.CreateUnbindSmartAccessGatewayRequest()
	req.Scheme = "https"
	req.ConnectTimeout = connectionTimeout
	req.ReadTimeout = readTimeout
	req.RegionId = s.region
	req.SmartAGId = sagId
	req.CcnId = ccnId
	klog.V(4).Infof("alicloud: UnbindSmartAccessGateway req: %v", req)
	response, err := s.client.UnbindSmartAccessGateway(req)
	if err != nil {
		if serverErr, ok := err.(*errors.ServerError); ok && serverErr.ErrorCode() == string(ErrorCodeSagNotBind) {
			klog.Infof("alicloud: sag %s already unbound from ccn %s", sagId, ccnId)
			return nil
		}
		return shrinkError(err)
	}
	klog.V(4).Infof("alicloud: UnbindSmartAccessGateway response: %v", response.String())

	return nil
}

func (s *SagClient) GrantCCN(sagId, ccnId, ccnUid string) error {
	req := smartag.CreateGrantSagInstanceToCcnRequest()
	req.Scheme = "https"
	req.ConnectTimeout = connectionTimeout
	req.ReadTimeout = readTimeout
	req.RegionId = s.region
	req.SmartAGId = sagId
	req.CcnInstanceId = ccnId
	req.CcnUid = requests.Integer(ccnUid)

	klog.V(4).Infof("alicloud: GrantSagInstanceToCcn req: %v", req)
	response, err := s.client.GrantSagInstanceToCcn(req)
	if err != nil {
		return shrinkError(err)
	}
	klog.V(4).Infof("alicloud: GrantSagInstanceToCcn response: %v", response.String())

	return nil
}

func (s *SagClient) RevokeCCN(sagId, ccnId string) error {
	req := smartag.CreateRevokeSagInstanceFromCcnRequest()
	req.Scheme = "https"
	req.ConnectTimeout = connectionTimeout
	req.ReadTimeout = readTimeout
	req.RegionId = s.region
	req.SmartAGId = sagId
	req.CcnInstanceId = ccnId

	klog.V(4).Infof("alicloud: RevokeSagInstanceFromCcn req: %v", req)
	response, err := s.client.RevokeSagInstanceFromCcn(req)
	if err != nil {
		return shrinkError(err)
	}
	klog.V(4).Infof("alicloud: RevokeSagInstanceFromCcn response: %v", response.String())

	return nil
}

func (s *SagClient) AddOfflineCIDRs(sagId, sagDesc string, cidrs []string) error {
	req := smartag.CreateModifySmartAccessGatewayRequest()
	req.Scheme = "https"
	req.ConnectTimeout = connectionTimeout
	req.ReadTimeout = readTimeout
	req.RegionId = s.region
	req.SmartAGId = sagId
	req.Description = sagDesc
	if cidrs == nil || len(cidrs) == 0 {
		return fmt.Errorf("added offline cidrs is empty")
	}
	req.CidrBlock = strings.Join(cidrs, ",")
	req.RoutingStrategy = "static"

	klog.V(4).Infof("alicloud: ModifySmartAccessGateway req: %v", req)
	response, err := s.client.ModifySmartAccessGateway(req)
	if err != nil {
		return shrinkError(err)
	}
	klog.V(4).Infof("alicloud: ModifySmartAccessGateway response: %v", response.String())
	return nil
}

func (s *SagClient) RemoveOfflineCIDRs(sagId string) error {
	req := smartag.CreateModifySmartAccessGatewayRequest()
	req.Scheme = "https"
	req.ConnectTimeout = connectionTimeout
	req.ReadTimeout = readTimeout
	req.RegionId = s.region
	req.SmartAGId = sagId
	// set cidrblock "" for deletion
	req.CidrBlock = ""
	//req.RoutingStrategy = "static"

	klog.V(4).Infof("alicloud: ModifySmartAccessGateway req: %v", req)
	response, err := s.client.ModifySmartAccessGateway(req)
	if err != nil {
		return shrinkError(err)
	}
	klog.V(4).Infof("alicloud: ModifySmartAccessGateway response: %v", response.String())
	return nil
}

func (s *SagClient) describeSagAttr(sagId string) (*smartag.DescribeSmartAccessGatewayAttributeResponse, error) {
	req := smartag.CreateDescribeSmartAccessGatewayAttributeRequest()
	req.Scheme = "https"
	req.ConnectTimeout = connectionTimeout
	req.ReadTimeout = readTimeout
	req.SmartAGId = sagId
	req.RegionId = s.region

	klog.V(4).Infof("alicloud: DescribeSmartAccessGatewayAttribute req: %v", req)
	response, err := s.client.DescribeSmartAccessGatewayAttribute(req)
	if err != nil {
		// ignore json unmarshal error:
		// [SDK.JsonUnmarshalError] Failed to unmarshal response, but you can get the data via response.GetHttpStatusCode() and response.GetHttpContentString()
		// caused by:
		// smartag.DescribeSmartAccessGatewayAttributeResponse.IpsecStatus: OptimizationType: fuzzyBoolDecoder: unsupported bool value: ds, error found in #10 byte of ...|Type":"ds","IpsecSta|..., bigger context ...|34d064585be024ef843b76457","OptimizationType":"ds","IpsecStatus":"down","RoutingStrategy":"static","|...
		if clientErr, ok := err.(*errors.ClientError); ok && clientErr.ErrorCode() == errors.JsonUnmarshalErrorCode && response != nil {
			klog.Warningf("alicloud: DescribeSmartAccessGatewayAttribute error, proceed anyway: %v", err)
			klog.V(4).Infof("alicloud: DescribeSmartAccessGatewayAttribute response: %v", response.String())
			return response, nil
		}
		return nil, err
	}
	klog.V(4).Infof("alicloud: DescribeSmartAccessGatewayAttribute response: %v", response.String())
	return response, nil
}

func (s *SagClient) UpgradeSmartAccessGateway(sagID string, newBandWidth int) error {
	req := smartag.CreateUpgradeSmartAccessGatewayRequest()
	req.Scheme = "https"
	req.ConnectTimeout = connectionTimeout
	req.ReadTimeout = readTimeout
	req.RegionId = s.region
	req.SmartAGId = sagID
	req.AutoPay = requests.NewBoolean(true)
	req.BandWidthSpec = requests.NewInteger(newBandWidth)
	klog.V(4).Infof("alicloud: UpgradeSmartAccessGateway req: %v", req)

	resp, err := s.client.UpgradeSmartAccessGateway(req)
	if err != nil {
		return shrinkError(err)
	}
	klog.V(4).Infof("alicloud: UpgradeSmartAccessGateway resp: %v", resp.String())
	return nil
}

func (s *SagClient) DowngradeSmartAccessGateway(sagID string, newBandWidth int) error {
	req := smartag.CreateDowngradeSmartAccessGatewayRequest()
	req.Scheme = "https"
	req.ConnectTimeout = connectionTimeout
	req.ReadTimeout = readTimeout
	req.RegionId = s.region
	req.SmartAGId = sagID
	req.AutoPay = requests.NewBoolean(true)
	req.BandWidthSpec = requests.NewInteger(newBandWidth)
	klog.V(4).Infof("alicloud: DowngradeSmartAccessGateway req: %v", req)

	resp, err := s.client.DowngradeSmartAccessGateway(req)
	if err != nil {
		return shrinkError(err)
	}
	klog.V(4).Infof("alicloud: DowngradeSmartAccessGateway resp: %v", resp.String())
	return nil
}

func (s *SagClient) DescribeSmartAccessGateway(sagID string) (*smartag.SmartAccessGateway, error) {
	req := smartag.CreateDescribeSmartAccessGatewaysRequest()
	req.Scheme = "https"
	req.ConnectTimeout = connectionTimeout
	req.ReadTimeout = readTimeout
	req.RegionId = s.region
	req.SmartAGId = sagID

	klog.V(4).Infof("alicloud: DescribeSmartAccessGateway req: %v", req)

	resp, err := s.client.DescribeSmartAccessGateways(req)
	if err != nil {
		return nil, shrinkError(err)
	}

	klog.V(4).Infof("alicloud: DescribeSmartAccessGateway resp: %v", resp)
	if resp != nil {
		if len(resp.SmartAccessGateways.SmartAccessGateway) > 0 {
			return &resp.SmartAccessGateways.SmartAccessGateway[0], nil
		}
	}
	return nil, nil
}

func shrinkError(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf(strings.ReplaceAll(err.Error(), "\n", "; "))
}
