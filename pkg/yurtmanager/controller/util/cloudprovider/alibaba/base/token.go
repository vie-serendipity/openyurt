package base

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"reflect"
	"strings"
	"time"

	prvd "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/util/cloudprovider"
)

const AssumeRoleName = "AliyunCSManagedEdgeRole"

type DefaultToken struct {
	Region          string
	AccessKeyId     string
	AccessKeySecret string
	SecurityToken   string
}

// TokenAuth is an interface of Token auth method
type TokenAuth interface {
	NextToken() (*DefaultToken, error)
}

type MetaToken struct {
	Region          string
	UID             string
	AccessKeyID     string
	AccessKeySecret string
}

func (m *MetaToken) NextToken() (*DefaultToken, error) {
	stsClient, err := NewClientWithAccessKey(m.Region, m.AccessKeyID, m.AccessKeySecret)
	if err != nil {
		return nil, fmt.Errorf("alibaba cloud: create sts client error: %s", err.Error())
	}
	req := CreateAssumeRoleWithServiceIdentityRequest()
	req.SetScheme("https")
	req.DurationSeconds = "7200"
	req.AssumeRoleFor = m.UID
	req.RoleArn = strings.ToLower(fmt.Sprintf("acs:ram::%s:role/%s", m.UID, AssumeRoleName))
	req.RoleSessionName = fmt.Sprintf("%s-provision-role-%d", "ecm", time.Now().Unix())
	resp, err := stsClient.AssumeRoleWithServiceIdentity(req)
	if err != nil {
		return nil, fmt.Errorf("alicloud: AssumeRole error: %s", err.Error())
	}
	token := &DefaultToken{
		Region:          m.Region,
		SecurityToken:   resp.Credentials.SecurityToken,
		AccessKeyId:     resp.Credentials.AccessKeyId,
		AccessKeySecret: resp.Credentials.AccessKeySecret,
	}
	return token, nil
}

type RamRoleToken struct {
	meta prvd.IMetaData
}

func (f *RamRoleToken) NextToken() (*DefaultToken, error) {
	roleName, err := f.meta.RoleName()
	if err != nil {
		return nil, fmt.Errorf("role name: %s", err.Error())
	}
	// use instance ram file way.
	role, err := f.meta.RamRoleToken(roleName)
	if err != nil {
		return nil, fmt.Errorf("ramrole Token retrieve: %s", err.Error())
	}
	region, err := f.meta.GetRegion()
	if err != nil {
		return nil, fmt.Errorf("read region error: %s", err.Error())
	}

	return &DefaultToken{
		Region:          region,
		AccessKeyId:     role.AccessKeyId,
		AccessKeySecret: role.AccessKeySecret,
		SecurityToken:   role.SecurityToken,
	}, nil
}

type ResultList struct {
	result []string
}

type IMetaDataRequest interface {
	Version(version string) IMetaDataRequest
	ResourceType(rtype string) IMetaDataRequest
	Resource(resource string) IMetaDataRequest
	SubResource(sub string) IMetaDataRequest
	Url() (string, error)
	Do(api interface{}) error
}

type MetaDataRequest struct {
	version      string
	resourceType string
	resource     string
	subResource  string
	client       *http.Client
}

func (vpc *MetaDataRequest) Version(version string) IMetaDataRequest {
	vpc.version = version
	return vpc
}

func (vpc *MetaDataRequest) ResourceType(rtype string) IMetaDataRequest {
	vpc.resourceType = rtype
	return vpc
}

func (vpc *MetaDataRequest) Resource(resource string) IMetaDataRequest {
	vpc.resource = resource
	return vpc
}

func (vpc *MetaDataRequest) SubResource(sub string) IMetaDataRequest {
	vpc.subResource = sub
	return vpc
}

func (vpc *MetaDataRequest) Url() (string, error) {
	if vpc.version == "" {
		vpc.version = "latest"
	}
	if vpc.resourceType == "" {
		vpc.resourceType = "meta-data"
	}
	if vpc.resource == "" {
		return "", errors.New("the resource you want to visit must not be nil!")
	}
	endpoint := os.Getenv("METADATA_ENDPOINT")
	if endpoint == "" {
		endpoint = ENDPOINT
	}
	r := fmt.Sprintf("%s/%s/%s/%s", endpoint, vpc.version, vpc.resourceType, vpc.resource)
	if vpc.subResource == "" {
		return r, nil
	}
	return fmt.Sprintf("%s/%s", r, vpc.subResource), nil
}

func (vpc *MetaDataRequest) Do(api interface{}) (err error) {
	var res = ""
	var retry = AttemptStrategy{
		Min:   5,
		Total: 5 * time.Second,
		Delay: 200 * time.Millisecond,
	}
	for r := retry.Start(); r.Next(); {
		res, err = vpc.send()
		if !shouldRetry(err) {
			break
		}
	}
	if err != nil {
		return err
	}
	return vpc.Decode(res, api)
}

func (vpc *MetaDataRequest) Decode(data string, api interface{}) error {
	if data == "" {
		url, _ := vpc.Url()
		return fmt.Errorf("metadata: alivpc decode data must not be nil. url=[%s]\n", url)
	}
	switch api := api.(type) {
	case *ResultList:
		api.result = strings.Split(data, "\n")
		return nil
	case *prvd.RoleAuth:
		return json.Unmarshal([]byte(data), api)
	default:
		return fmt.Errorf("metadata: unknow type to decode, type=%s\n", reflect.TypeOf(api))
	}
}

func (vpc *MetaDataRequest) send() (string, error) {
	url, err := vpc.Url()
	if err != nil {
		return "", err
	}
	requ, err := http.NewRequest(http.MethodGet, url, nil)

	if err != nil {
		return "", err
	}
	resp, err := vpc.client.Do(requ)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("Aliyun Metadata API Error: Status Code: %d", resp.StatusCode)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
