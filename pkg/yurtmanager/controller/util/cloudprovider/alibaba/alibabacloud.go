package alibaba

import (
	"fmt"

	prvd "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/util/cloudprovider"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/util/cloudprovider/alibaba/base"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/util/cloudprovider/alibaba/elb"
	raven2 "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/util/cloudprovider/alibaba/raven"
	metrics "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/util/cloudprovider/metrics"
)

func NewAlibabaCloud(path string) (prvd.Provider, error) {
	mgr, err := base.NewClientMgr(path)
	if err != nil {
		return nil, fmt.Errorf("initialize alibaba cloud client auth: %s", err.Error())
	}
	if mgr == nil {
		return nil, fmt.Errorf("auth should not be nil")
	}

	metrics.RegisterPrometheus()

	return AlibabaCloud{
		IMetaData:   mgr.Meta,
		SLBProvider: raven2.NewLBProvider(mgr),
		VPCProvider: raven2.NewVPCProvider(mgr),
		ELBProvider: elb.NewELBProvider(mgr),
	}, nil
}

var _ prvd.Provider = AlibabaCloud{}

type AlibabaCloud struct {
	prvd.IMetaData
	*raven2.SLBProvider
	*raven2.VPCProvider
	*elb.ELBProvider
}
