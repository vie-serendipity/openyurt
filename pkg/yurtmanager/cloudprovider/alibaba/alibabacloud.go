package alibaba

import (
	"fmt"

	prvd "github.com/openyurtio/openyurt/pkg/yurtmanager/cloudprovider"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/cloudprovider/alibaba/base"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/cloudprovider/alibaba/raven"
)

const Provider = "Provider"

func NewAlibabaCloud(path string) (prvd.Provider, error) {
	mgr, err := base.NewClientMgr(path)
	if err != nil {
		return nil, fmt.Errorf("initialize alibaba cloud client auth: %s", err.Error())
	}
	if mgr == nil {
		return nil, fmt.Errorf("auth should not be nil")
	}

	return AlibabaCloud{
		IMetaData:   mgr.Meta,
		SLBProvider: raven.NewLBProvider(mgr),
		VPCProvider: raven.NewVPCProvider(mgr),
	}, nil
}

var _ prvd.Provider = AlibabaCloud{}

type AlibabaCloud struct {
	prvd.IMetaData
	*raven.SLBProvider
	*raven.VPCProvider
}
