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
		UID       string `json:"uid"`
		ClusterID string `json:"clusterID"`
		Region    string `json:"region"`
		VpcID     string `json:"vpcid"`
		VswitchID string `json:"vswitchid"`
		// route controller
		RouteTableIDS string `json:"routeTableIDs"`
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

func (cc *CloudConfig) UID() string {
	return cc.Global.UID
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
