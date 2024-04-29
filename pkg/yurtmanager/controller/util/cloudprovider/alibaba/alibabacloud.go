package alibaba

import (
	"fmt"

	prvd "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/util/cloudprovider"
	alibabcloud "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/util/cloudprovider/alibaba/alibabacloud"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/util/cloudprovider/alibaba/base"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/util/cloudprovider/alibaba/elb"
	metrics "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/util/cloudprovider/metrics"
)

func NewAlibabaCloud(path string, ensServiceRegion string) (prvd.Provider, error) {
	mgr, err := base.NewClientMgr(path, ensServiceRegion)
	if err != nil {
		return nil, fmt.Errorf("initialize alibaba cloud client auth: %s", err.Error())
	}
	if mgr == nil {
		return nil, fmt.Errorf("auth should not be nil")
	}

	metrics.RegisterPrometheus()

	return AlibabaCloud{
		IMetaData:   mgr.Meta,
		SLBProvider: alibabcloud.NewLBProvider(mgr),
		VPCProvider: alibabcloud.NewVPCProvider(mgr),
		ELBProvider: elb.NewELBProvider(mgr),
	}, nil
}

var _ prvd.Provider = AlibabaCloud{}

type AlibabaCloud struct {
	prvd.IMetaData
	*alibabcloud.SLBProvider
	*alibabcloud.VPCProvider
	*elb.ELBProvider
}
