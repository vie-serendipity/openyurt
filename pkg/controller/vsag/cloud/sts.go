package cloud

import (
	"fmt"
	"time"

	"github.com/aliyun/alibaba-cloud-sdk-go/sdk"
	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/auth/credentials/providers"
	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/requests"
	"github.com/aliyun/alibaba-cloud-sdk-go/services/sts"
)

const (
	// service ram role name for vsag controller
	defaultRoleFormat = "acs:ram::%s:role/aliyuncsmanagededgerole"

	defaultRoleSessionName = "vsag-controller"
)

type STSClient struct {
	*sts.Client
}

type AssumeRoleRequest struct {
	*requests.RpcRequest
	RoleSessionName string           `position:"Query" name:"RoleSessionName"`
	Policy          string           `position:"Query" name:"Policy"`
	RoleArn         string           `position:"Query" name:"RoleArn"`
	DurationSeconds requests.Integer `position:"Query" name:"DurationSeconds"`
	AssumeRoleFor   string           `position:"Query" name:"AssumeRoleFor"`
}

func NewSTSClientWithAkSk(region, cloudServiceAK, cloudServiceSK string) (*STSClient, error) {
	client, err := sts.NewClientWithAccessKey(region, cloudServiceAK, cloudServiceSK)
	if err != nil {
		return nil, err
	}

	return &STSClient{client}, nil
}

func NewSTSClientWithMetadata(region string) (*STSClient, error) {
	metaProvider := providers.NewInstanceMetadataProvider()
	stsTokenCred, err := metaProvider.Retrieve()
	if err != nil {
		return nil, err
	}

	config := sdk.NewConfig()
	config = config.WithTimeout(30 * time.Second)
	client, err := sts.NewClientWithOptions(region, config, stsTokenCred)
	if err != nil {
		return nil, err
	}

	return &STSClient{client}, nil
}

func (s *STSClient) AssumeUidSTSToken(uid string) (roleAK, roleSK, roleSTSToken string, err error) {
	req := sts.CreateAssumeRoleRequest()
	req.Scheme = "HTTPS"
	//timeout-default-value=1hour
	req.DurationSeconds = requests.NewInteger(defaultTTLSeconds)
	req.RoleSessionName = defaultRoleSessionName
	req.RoleArn = getDefaultRole(uid)

	resp, err := s.AssumeRole(req)
	if err != nil {
		return "", "", "", err
	}

	return resp.Credentials.AccessKeyId,
		resp.Credentials.AccessKeySecret,
		resp.Credentials.SecurityToken,
		nil
}

func (s *STSClient) AssumeUidSTSTokenWithServiceIdentity(uid string) (roleAK, roleSK, roleSTSToken string, err error) {
	req := createAssumeRoleWithServiceIdentityRequest()
	req.Scheme = "HTTPS"
	//timeout-default-value=1hour
	req.DurationSeconds = requests.NewInteger(defaultTTLSeconds)
	req.RoleSessionName = defaultRoleSessionName
	req.AssumeRoleFor = uid

	req.RoleArn = getDefaultRole(uid)
	resp, err := s.assumeRoleWithServiceIdentity(req)
	if err != nil {
		return "", "", "", err
	}

	return resp.Credentials.AccessKeyId,
		resp.Credentials.AccessKeySecret,
		resp.Credentials.SecurityToken,
		nil
}

func (s *STSClient) assumeRoleWithServiceIdentity(request *AssumeRoleRequest) (response *sts.AssumeRoleResponse, err error) {
	response = sts.CreateAssumeRoleResponse()
	err = s.DoAction(request, response)
	return
}

// CreateAssumeRoleRequest creates a request to invoke AssumeRole API
func createAssumeRoleWithServiceIdentityRequest() (request *AssumeRoleRequest) {
	request = &AssumeRoleRequest{
		RpcRequest: &requests.RpcRequest{},
	}
	request.InitWithApiInfo("Sts", "2015-04-01", "assumeRoleWithServiceIdentity", "", "")

	return
}

func getDefaultRole(uid string) string {
	return fmt.Sprintf(defaultRoleFormat, uid)
}
