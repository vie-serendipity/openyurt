package sts

import (
	"fmt"

	"github.com/aliyun/alibaba-cloud-sdk-go/sdk"
)

// Client is the sdk client struct, each func corresponds to an OpenAPI
type Client struct {
	sdk.Client
}

func NewClientWithAccessKey(regionId, accessKeyId, accessKeySecret string) (*Client, error) {
	client := &Client{}
	err := client.InitWithAccessKey(regionId, accessKeyId, accessKeySecret)
	if regionId != "" {
		client.Domain = fmt.Sprintf("sts-vpc.%s.aliyuncs.com", regionId)
	}
	return client, err
}
