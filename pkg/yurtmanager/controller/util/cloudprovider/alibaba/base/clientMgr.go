package base

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
	"k8s.io/klog/v2/klogr"

	"github.com/aliyun/alibaba-cloud-sdk-go/sdk"
	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/auth/credentials"
	"github.com/aliyun/alibaba-cloud-sdk-go/services/ens"
	"github.com/aliyun/alibaba-cloud-sdk-go/services/slb"
	"github.com/aliyun/alibaba-cloud-sdk-go/services/vpc"
	prvd "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/util/cloudprovider"
)

const (
	AgentClusterId  = "ClusterId"
	TokenSyncPeriod = 10 * time.Minute

	KubernetesEdgeControllerManager = "ack.ecm"
)

var log = klogr.New().WithName("clientMgr")

type ClientMgr struct {
	stop   <-chan struct{}
	Region string
	Meta   prvd.IMetaData

	SLB *slb.Client
	EIP *vpc.Client
	ELB *ens.Client
}

func NewClientMgr(cloudConfigPath string) (*ClientMgr, error) {

	if err := CloudCFG.LoadCloudCFG(cloudConfigPath); err != nil {
		return nil, fmt.Errorf("load cloud config %s, error: %s", cloudConfigPath, err.Error())
	}
	meta, err := NewMetaData()
	if err != nil {
		klog.Errorf("new meta data, error %s", err.Error())
		return nil, fmt.Errorf("new meta data, error %s", err.Error())
	}
	clusterId, err := meta.GetClusterID()
	if err != nil {
		return nil, fmt.Errorf("can not determin cluster id, error %s", err.Error())
	}
	region, err := meta.GetRegion()
	if err != nil {
		return nil, fmt.Errorf("can not determin region, error %s", err.Error())
	}

	credential := &credentials.StsTokenCredential{
		AccessKeyId:       "key",
		AccessKeySecret:   "secret",
		AccessKeyStsToken: "",
	}

	vpcli, err := vpc.NewClientWithOptions(region, clientCfg(), credential)
	if err != nil {
		return nil, fmt.Errorf("initialize alibaba vpc client: %s", err.Error())
	}
	vpcli.AppendUserAgent(AgentClusterId, clusterId)

	slbcli, err := slb.NewClientWithOptions(region, clientCfg(), credential)
	if err != nil {
		return nil, fmt.Errorf("initialize alibaba slb client: %s", err.Error())
	}
	slbcli.AppendUserAgent(AgentClusterId, clusterId)

	elbcli, err := ens.NewClientWithOptions("cn-hangzhou", clientCfg(), credential)
	if err != nil {
		return nil, fmt.Errorf("initialize alibaba elb client: %s", err.Error())
	}
	elbcli.AppendUserAgent(AgentClusterId, clusterId)

	auth := &ClientMgr{
		Meta:   meta,
		SLB:    slbcli,
		EIP:    vpcli,
		ELB:    elbcli,
		Region: region,
		stop:   make(<-chan struct{}, 1)}

	err = auth.Start(RefreshToken)
	if err != nil {
		klog.Warningf("refresh token error: %s", err.Error())
	}
	return auth, nil
}

func (mgr *ClientMgr) Start(settoken func(mgr *ClientMgr, token *DefaultToken) error) error {
	initialized := false
	tokenAuth, err := mgr.GetTokenAuth()
	if err != nil {
		return err
	}

	tokenfunc := func() {
		klog.Infoln("refresh token")
		token, err := tokenAuth.NextToken()
		if err != nil {
			log.Error(err, "fail to get next token")
			return
		}
		err = settoken(mgr, token)
		if err != nil {
			log.Error(err, "fail to set token")
			return
		}
		initialized = true
	}

	go wait.Until(
		func() { tokenfunc() },
		TokenSyncPeriod,
		mgr.stop,
	)

	return wait.ExponentialBackoff(
		wait.Backoff{
			Steps:    7,
			Duration: 1 * time.Second,
			Jitter:   1,
			Factor:   2,
		}, func() (done bool, err error) {
			tokenfunc()
			log.Info("wait for Token ready")
			return initialized, nil
		},
	)
}

func (mgr *ClientMgr) GetTokenAuth() (TokenAuth, error) {
	// priority: AddonToken > ServiceToken > AKMode > RamRoleToken
	if _, err := os.Stat(AddonTokenFilePath); err == nil {
		log.Info("use addon token mode to get token")
		return &AddonToken{Region: mgr.Region}, nil
	}

	if CloudCFG.Global.AccessKeyID != "" && CloudCFG.Global.AccessKeySecret != "" {
		if mgr.Meta == nil {
			return nil, fmt.Errorf("can not get token meta data is empty")
		}
		uid, err := mgr.Meta.GetUID()
		if err != nil {
			return nil, fmt.Errorf("can not determin uid")
		}
		ak, err := mgr.Meta.GetAccessID()
		if err != nil {
			return nil, fmt.Errorf("can not determin access key id")
		}
		sk, err := mgr.Meta.GetAccessSecret()
		if err != nil {
			return nil, fmt.Errorf("can not determin access key secret")
		}
		region, err := mgr.Meta.GetRegion()
		if err != nil {
			return nil, fmt.Errorf("can not determin region")
		}
		return &MetaToken{UID: uid, AccessKeyID: ak, AccessKeySecret: sk, Region: region}, nil
	}

	return &RamRoleToken{mgr.Meta}, nil
}

func RefreshToken(mgr *ClientMgr, token *DefaultToken) error {
	log.V(5).Info("refresh token", "region", token.Region)
	credential := &credentials.StsTokenCredential{
		AccessKeyId:       token.AccessKeyId,
		AccessKeySecret:   token.AccessKeySecret,
		AccessKeyStsToken: token.SecurityToken,
	}

	err := mgr.EIP.InitWithOptions(token.Region, clientCfg(), credential)
	if err != nil {
		return fmt.Errorf("init vpc sts token config: %s", err.Error())
	}

	err = mgr.SLB.InitWithOptions(token.Region, clientCfg(), credential)
	if err != nil {
		return fmt.Errorf("init slb sts token config: %s", err.Error())
	}

	err = mgr.ELB.InitWithOptions(token.Region, clientCfg(), credential)
	if err != nil {
		return fmt.Errorf("init elb sts token config: %s", err.Error())
	}

	setCustomizedEndpoint(mgr)
	return nil
}

func clientCfg() *sdk.Config {
	scheme := "HTTPS"
	if os.Getenv("ALICLOUD_CLIENT_SCHEME") == "HTTP" {
		scheme = "HTTP"
	}
	return &sdk.Config{
		Timeout:   20 * time.Second,
		Transport: http.DefaultTransport,
		Scheme:    scheme,
	}
}

func setCustomizedEndpoint(mgr *ClientMgr) {
	if vpcEndpoint, err := parseURL(os.Getenv("VPC_ENDPOINT")); err == nil && vpcEndpoint != "" {
		mgr.EIP.Domain = vpcEndpoint
	}
	if slbEndpoint, err := parseURL(os.Getenv("SLB_ENDPOINT")); err == nil && slbEndpoint != "" {
		mgr.SLB.Domain = slbEndpoint
	}
}

func parseURL(str string) (string, error) {
	if str == "" {
		return "", nil
	}

	if !strings.HasPrefix(str, "http") {
		str = "http://" + str
	}
	u, err := url.Parse(str)
	if err != nil {
		return "", err
	}
	return u.Host, nil
}
