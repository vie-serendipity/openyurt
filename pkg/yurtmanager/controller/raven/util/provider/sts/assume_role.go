package sts

import (
	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/requests"
	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/responses"
)

// AssumeRole invokes the sts.AssumeRole API synchronously
// api document: https://help.aliyun.com/api/sts/assumerole.html
func (client *Client) AssumeRoleWithServiceIdentity(request *AssumeRoleWithServiceIdentityRequest) (response *AssumeRoleWithServiceIdentityResponse, err error) {
	response = CreateAssumeRoleWithServiceIdentityResponse()
	err = client.DoAction(request, response)
	return
}

type AssumeRoleWithServiceIdentityRequest struct {
	*requests.RpcRequest
	RoleArn         string           `position:"Query" name:"RoleArn"`
	RoleSessionName string           `position:"Query" name:"RoleSessionName"`
	DurationSeconds requests.Integer `position:"Query" name:"DurationSeconds"`
	Policy          string           `position:"Query" name:"Policy"`
	AssumeRoleFor   string           `position:"Query" name:"AssumeRoleFor"`
}

type AssumeRoleWithServiceIdentityResponse struct {
	*responses.BaseResponse
	AssumedRoleUser AssumedRoleUserWithServiceIdentity            `json:"AssumedRoleUser" xml:"AssumedRoleUser"`
	Credentials     AssumedRoleUserCredentialsWithServiceIdentity `json:"Credentials" xml:"Credentials"`
}

type AssumedRoleUserWithServiceIdentity struct {
	AssumedRoleId string `json:"AssumedRoleId" xml:"AssumedRoleId"`
	Arn           string `json:"Arn" xml:"Arn"`
}

type AssumedRoleUserCredentialsWithServiceIdentity struct {
	AccessKeySecret string `json:"AccessKeySecret" xml:"AccessKeySecret"`
	AccessKeyId     string `json:"AccessKeyId" xml:"AccessKeyId"`
	Expiration      string `json:"Expiration" xml:"Expiration"`
	SecurityToken   string `json:"SecurityToken" xml:"SecurityToken"`
}

// CreateAssumeRoleWithServiceIdentityRequest creates a request to invoke AssumeRole API
func CreateAssumeRoleWithServiceIdentityRequest() (request *AssumeRoleWithServiceIdentityRequest) {
	request = &AssumeRoleWithServiceIdentityRequest{
		RpcRequest: &requests.RpcRequest{},
	}
	request.InitWithApiInfo("Sts", "2015-04-01", "AssumeRoleWithServiceIdentity", "sts", "openAPI")
	return
}

// CreateAssumeRoleWithServiceIdentityResponse creates a response to parse from AssumeRole response
func CreateAssumeRoleWithServiceIdentityResponse() (response *AssumeRoleWithServiceIdentityResponse) {
	response = &AssumeRoleWithServiceIdentityResponse{
		BaseResponse: &responses.BaseResponse{},
	}
	return
}
