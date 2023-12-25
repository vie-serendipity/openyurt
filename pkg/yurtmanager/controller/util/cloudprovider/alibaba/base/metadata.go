package base

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strings"

	prvd "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/util/cloudprovider"
)

const (
	ENDPOINT     = "http://100.100.100.200"
	RAM_SECURITY = "ram/security-credentials"
	VPC_ID       = "vpc-id"
	REGION       = "region-id"
	VSWITCH_ID   = "vswitch-id"
)

func NewCfgMeta() (*MetaData, error) {
	if CloudCFG == nil {
		return &MetaData{}, errors.New("cloud config error")
	}
	key, err := base64.StdEncoding.DecodeString(CloudCFG.AccessID())
	if err != nil {
		return &MetaData{}, errors.New("access id is empty")
	}
	keyId := string(key)

	secret, err := base64.StdEncoding.DecodeString(CloudCFG.AccessSecret())
	if err != nil {
		return &MetaData{}, errors.New("access secret is empty")
	}
	keySecret := string(secret)

	if CloudCFG.VpcID() == "" {
		return &MetaData{}, errors.New("vpc id is empty")
	}

	if CloudCFG.VswitchID() == "" {
		return &MetaData{}, errors.New("vswitch id is empty")
	}

	vswitchs := strings.Split(CloudCFG.VswitchID(), ",")
	if len(vswitchs) == 0 {
		return &MetaData{}, errors.New(fmt.Sprintf("vswitch %s is incorrect", CloudCFG.VswitchID()))
	}

	vswitchId := strings.Split(vswitchs[0], ":")
	var id string
	if len(vswitchId) == 2 {
		id = vswitchId[1]
	}
	if id == "" {
		return &MetaData{}, errors.New(fmt.Sprintf("vswitch %s is incorrect", vswitchId))
	}
	return &MetaData{
		UID:             CloudCFG.UID(),
		AccessKeyID:     keyId,
		AccessKeySecret: keySecret,
		ClusterID:       CloudCFG.ClusterID(),
		VpcID:           CloudCFG.VpcID(),
		VswitchID:       id,
		Region:          CloudCFG.Region(),
	}, nil
}
func NewMetaData() (prvd.IMetaData, error) {
	if CloudCFG.Global.AccessKeyID != "" && CloudCFG.Global.AccessKeySecret != "" {
		return NewCfgMeta()
	}
	return NewBaseMetaData(nil), nil
}

// render meta data from cloud config file
var _ prvd.IMetaData = &MetaData{}

type MetaData struct {
	UID             string
	AccessKeyID     string
	AccessKeySecret string
	Region          string
	ClusterID       string
	VpcID           string
	VswitchID       string
}

func (m *MetaData) GetVpcID() (string, error) {
	if m.VpcID == "" {
		return "", errors.New("vpc id is empty, can not get")
	}
	return m.VpcID, nil
}

func (m *MetaData) GetVswitchID() (string, error) {
	if m.VswitchID == "" {
		return "", errors.New("vswitch id is empty, can not get")
	}
	return m.VswitchID, nil
}

func (m *MetaData) GetRegion() (string, error) {
	if m.Region == "" {
		return "", errors.New("region is empty, can not get")
	}
	return m.Region, nil
}

func (m *MetaData) GetUID() (string, error) {
	if m.UID == "" {
		return "", errors.New("uid is empty, can not get")
	}
	return m.UID, nil
}

func (m *MetaData) GetAccessID() (string, error) {
	if m.AccessKeyID == "" {
		return "", errors.New("access key id is empty, can not get")
	}
	return m.AccessKeyID, nil
}

func (m *MetaData) GetAccessSecret() (string, error) {
	if m.AccessKeySecret == "" {
		return "", errors.New("access key secret is empty, can not get")
	}
	return m.AccessKeySecret, nil
}

func (m *MetaData) GetClusterID() (string, error) {
	if m.ClusterID == "" {
		return "", errors.New("cluster id is empty, can not get")
	}
	return m.ClusterID, nil
}

func (m *MetaData) RoleName() (string, error) {
	return "", nil
}

func (m *MetaData) RamRoleToken(role string) (prvd.RoleAuth, error) {
	return prvd.RoleAuth{}, nil
}

func (m *MetaData) LoadCloudCFG() {
	m.UID = CloudCFG.UID()
	m.AccessKeyID = CloudCFG.AccessID()
	m.AccessKeySecret = CloudCFG.AccessSecret()
	m.ClusterID = CloudCFG.ClusterID()
	m.VpcID = CloudCFG.VpcID()
	m.VswitchID = CloudCFG.VswitchID()
	m.Region = CloudCFG.Region()
}

// render meta data from cloud config file
var _ prvd.IMetaData = &BaseMetaData{}

type BaseMetaData struct {
	client *http.Client
}

func (m *BaseMetaData) GetClusterID() (string, error) {
	if CloudCFG.Global.ClusterID != "" {
		return CloudCFG.Global.ClusterID, nil
	}
	return "", fmt.Errorf("can not get cluster id")
}

func (m *BaseMetaData) GetRegion() (string, error) {
	var region ResultList
	err := m.New().Resource(REGION).Do(&region)
	if err != nil {
		return "", err
	}
	return region.result[0], nil
}

func (m *BaseMetaData) GetVpcID() (string, error) {
	var vpcId ResultList
	err := m.New().Resource(VPC_ID).Do(&vpcId)
	if err != nil {
		return "", err
	}
	return vpcId.result[0], err
}

func (m *BaseMetaData) GetVswitchID() (string, error) {
	var vswithcid ResultList
	err := m.New().Resource(VSWITCH_ID).Do(&vswithcid)
	if err != nil {
		return "", err
	}
	return vswithcid.result[0], err
}

func (m *BaseMetaData) GetUID() (string, error) {
	//TODO implement me
	panic("implement me")
}

func (m *BaseMetaData) GetAccessID() (string, error) {
	return "", nil
}

func (m *BaseMetaData) GetAccessSecret() (string, error) {
	return "", nil
}

func (m *BaseMetaData) Region() (string, error) {
	var region ResultList
	err := m.New().Resource("region-id").Do(&region)
	if err != nil {
		return "", err
	}
	return region.result[0], nil
}

func (m *BaseMetaData) RoleName() (string, error) {
	var roleName ResultList
	err := m.New().Resource("ram/security-credentials/").Do(&roleName)
	if err != nil {
		return "", err
	}
	return roleName.result[0], nil
}

func (m *BaseMetaData) RamRoleToken(role string) (prvd.RoleAuth, error) {
	var roleauth prvd.RoleAuth
	err := m.New().Resource(RAM_SECURITY).SubResource(role).Do(&roleauth)
	if err != nil {
		return prvd.RoleAuth{}, err
	}
	return roleauth, nil
}

func NewBaseMetaData(client *http.Client) *BaseMetaData {
	if client == nil {
		client = &http.Client{}
	}
	return &BaseMetaData{
		client: client,
	}
}

func (m *BaseMetaData) New() *MetaDataRequest {
	return &MetaDataRequest{
		client: m.client,
	}
}
