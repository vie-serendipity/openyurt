package base

import (
	"fmt"
	"os"

	"sigs.k8s.io/yaml"
)

var CloudCFG = &CloudConfig{}

// CloudConfig wraps the settings for the Alibaba Cloud provider.
type CloudConfig struct {
	Global struct {
		UID             string `json:"uid"`
		AccessKeyID     string `json:"accessKeyID"`
		AccessKeySecret string `json:"accessKeySecret"`

		// cluster related
		ClusterID            string `json:"clusterID"`
		KubernetesClusterTag string `json:"kubernetesClusterTag"`
		Region               string `json:"region"`
		VpcID                string `json:"vpcid"`
		ZoneID               string `json:"zoneid"`
		VswitchID            string `json:"vswitchid"`

		// service controller
		ServiceBackendType string `json:"serviceBackendType"`
		DisablePublicSLB   bool   `json:"disablePublicSLB"`

		// node controller
		NodeMonitorPeriod  int64 `json:"nodeMonitorPeriod"`
		NodeAddrSyncPeriod int64 `json:"nodeAddrSyncPeriod"`

		// route controller
		RouteTableIDS string `json:"routeTableIDs"`

		// pvtz controller
		PrivateZoneID        string `json:"privateZoneId"`
		PrivateZoneRecordTTL int64  `json:"privateZoneRecordTTL"`

		FeatureGates string `json:"featureGates"`
	}
}

func (cc *CloudConfig) LoadCloudCFG(path string) error {
	if path == "" {
		return fmt.Errorf("cloud config path error: path is empty")
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read cloud config error: %s ", err.Error())
	}
	return yaml.Unmarshal(content, cc)
}

func (cc *CloudConfig) GetKubernetesClusterTag() string {
	if cc.Global.KubernetesClusterTag == "" {
		return "ack.aliyun.com"
	}
	return cc.Global.KubernetesClusterTag
}

func (cc *CloudConfig) UID() string {
	return cc.Global.UID
}

func (cc *CloudConfig) AccessID() string {
	return cc.Global.AccessKeyID
}

func (cc *CloudConfig) AccessSecret() string {
	return cc.Global.AccessKeySecret
}

func (cc *CloudConfig) ClusterID() string {
	return cc.Global.ClusterID
}

func (cc *CloudConfig) Region() string {
	return cc.Global.Region
}

func (cc *CloudConfig) VpcID() string {
	return cc.Global.VpcID
}

func (cc *CloudConfig) VswitchID() string {
	return cc.Global.VswitchID
}
