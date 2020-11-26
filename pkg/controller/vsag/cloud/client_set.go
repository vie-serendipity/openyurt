package cloud

import (
	"fmt"
	"time"

	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/auth/credentials"
	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/auth/credentials/providers"
	"github.com/aliyun/alibaba-cloud-sdk-go/services/bssopenapi"
	"github.com/aliyun/alibaba-cloud-sdk-go/services/cbn"
	"github.com/aliyun/alibaba-cloud-sdk-go/services/smartag"
	"github.com/aliyun/alibaba-cloud-sdk-go/services/vpc"
	"k8s.io/apimachinery/pkg/util/cache"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/controller/util"
)

const (
	defaultTTLSeconds = 3600
	defaultCacheTTL   = (defaultTTLSeconds - 10) * time.Second
	defaultLRUSize    = 500

	connectionTimeout = 30 * time.Second
	readTimeout       = 30 * time.Second

	// for billing client, we use default region in China, and pass actual vcpe region in parameter
	defaultBillingRegion = "cn-shanghai"
)

type ClientSet struct {
	// client with ack resource account
	sagResourceClient *SagClient
	bssClient         *BssClient
	// client with ram role
	sagRoleClient *SagClient
	vpcRoleClient *VpcClient
	cenRoleClient *CenClient
}

func (cs *ClientSet) SagResourceClient() *SagClient {
	return cs.sagResourceClient
}

func (cs *ClientSet) BssClient() *BssClient {
	return cs.bssClient
}

func (cs *ClientSet) SagRoleClient() *SagClient {
	return cs.sagRoleClient
}

func (cs *ClientSet) VpcRoleClient() *VpcClient {
	return cs.vpcRoleClient
}

func (cs *ClientSet) CenRoleClient() *CenClient {
	return cs.cenRoleClient
}

type ClientMgr struct {
	config      *util.CloudConfig
	guestConfig *util.CCMCloudConfig
	cache       *cache.LRUExpireCache
}

func NewClientMgrSet(cfg *util.CloudConfig, guestCfg *util.CCMCloudConfig) *ClientMgr {
	res := &ClientMgr{
		config:      cfg,
		guestConfig: guestCfg,
		cache:       cache.NewLRUExpireCache(defaultLRUSize),
	}

	return res
}

// return initialized and available clientmgr
func (c *ClientMgr) GetClient(uid, sagRegion string) (*ClientSet, error) {
	cacheKey := fmt.Sprintf("%s_%s", uid, sagRegion)
	if res, exist := c.cache.Get(cacheKey); exist {
		if mgr, ok := res.(*ClientSet); ok {
			return mgr, nil
		}
		return nil, fmt.Errorf("error: invalid type for uid %s", uid)
	}
	res, err := c.newClientSet(uid, sagRegion)
	if err != nil {
		return res, err
	}
	c.cache.Add(cacheKey, res, defaultCacheTTL)
	return res, err
}

func (c *ClientMgr) GetResourceUid() string {
	return c.config.ResourceUid
}

// create sag client with metadata server sts
func (c *ClientMgr) newSagClientWithMetadata(assumeRole bool, uid, sagRegion string) (*SagClient, error) {
	if !assumeRole {
		metaProvider := providers.NewInstanceMetadataProvider()
		cred, err := metaProvider.Retrieve()
		if err != nil {
			return nil, err
		}
		stsTokenCred, ok := cred.(*credentials.StsTokenCredential)
		if !ok {
			return nil, fmt.Errorf("instance credential is not ststoken type")
		}

		sagClient, err := smartag.NewClientWithStsToken(sagRegion, stsTokenCred.AccessKeyId,
			stsTokenCred.AccessKeySecret, stsTokenCred.AccessKeyStsToken)
		if err != nil {
			return nil, err
		}

		return &SagClient{sagClient, sagRegion}, nil
	}

	// new sts client in each call
	stsClient, err := NewSTSClientWithMetadata(sagRegion)
	if err != nil {
		klog.Fatalf("alicloud: error init sts client: %v", err)
	}
	ak, sk, stsToken, err := stsClient.AssumeUidSTSTokenWithServiceIdentity(uid)
	if err != nil {
		return nil, err
	}

	sagClient, err := smartag.NewClientWithStsToken(sagRegion, ak, sk, stsToken)
	if err != nil {
		return nil, err
	}

	return &SagClient{sagClient, sagRegion}, nil
}

// create client with ak/sk
func (c *ClientMgr) newClientSet(uid, sagRegion string) (*ClientSet, error) {
	// sag resource client
	sagResourceClient, err := smartag.NewClientWithAccessKey(sagRegion, c.config.AccessKeyID, c.config.AccessKeySecret)
	if err != nil {
		return nil, shrinkError(err)
	}

	// bss client
	bssClient, err := bssopenapi.NewClientWithAccessKey(defaultBillingRegion, c.config.AccessKeyID, c.config.AccessKeySecret)
	if err != nil {
		return nil, shrinkError(err)
	}

	// sag role client
	stsClient, err := NewSTSClientWithAkSk(sagRegion, c.guestConfig.Global.AccessKeyID, c.guestConfig.Global.AccessKeySecret)
	if err != nil {
		return nil, shrinkError(err)
	}
	ak, sk, stsToken, err := stsClient.AssumeUidSTSTokenWithServiceIdentity(uid)
	if err != nil {
		return nil, shrinkError(err)
	}

	sagRoleClient, err := smartag.NewClientWithStsToken(sagRegion, ak, sk, stsToken)
	if err != nil {
		return nil, shrinkError(err)
	}

	// vpc, cen role client
	stsClient, err = NewSTSClientWithAkSk(c.guestConfig.Global.Region, c.guestConfig.Global.AccessKeyID, c.guestConfig.Global.AccessKeySecret)
	if err != nil {
		return nil, shrinkError(err)
	}
	ak, sk, stsToken, err = stsClient.AssumeUidSTSTokenWithServiceIdentity(uid)
	if err != nil {
		return nil, shrinkError(err)
	}

	vpcRoleClient, err := vpc.NewClientWithStsToken(c.guestConfig.Global.Region, ak, sk, stsToken)
	if err != nil {
		return nil, shrinkError(err)
	}

	cenRoleClient, err := cbn.NewClientWithStsToken(c.guestConfig.Global.Region, ak, sk, stsToken)
	if err != nil {
		return nil, shrinkError(err)
	}

	return &ClientSet{
		sagResourceClient: &SagClient{sagResourceClient, sagRegion},
		bssClient:         &BssClient{bssClient, sagRegion},
		sagRoleClient:     &SagClient{sagRoleClient, sagRegion},
		vpcRoleClient:     &VpcClient{vpcRoleClient, c.guestConfig.Global.Region},
		cenRoleClient:     &CenClient{cenRoleClient, c.guestConfig.Global.Region},
	}, nil
}
