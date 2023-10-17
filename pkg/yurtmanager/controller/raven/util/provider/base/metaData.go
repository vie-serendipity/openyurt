package base

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

func NewMetaData() (IMetaData, error) {
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

type MetaData struct {
	UID             string
	AccessKeyID     string
	AccessKeySecret string
	Region          string
	ClusterID       string
	VpcID           string
	VswitchID       string
}

type IMetaData interface {
	GetClusterID() (string, error)
	GetRegion() (string, error)
	GetVpcID() (string, error)
	GetVswitchID() (string, error)
	GetUID() (string, error)
	GetAccessID() (string, error)
	GetAccessSecret() (string, error)
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

func (m *MetaData) LoadCloudCFG() {
	m.UID = CloudCFG.UID()
	m.AccessKeyID = CloudCFG.AccessID()
	m.AccessKeySecret = CloudCFG.AccessSecret()
	m.ClusterID = CloudCFG.ClusterID()
	m.VpcID = CloudCFG.VpcID()
	m.VswitchID = CloudCFG.VswitchID()
	m.Region = CloudCFG.Region()
}
