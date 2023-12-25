package elb

import (
	"fmt"

	elbmodel "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/util/cloudprovider/model/elb"
)

type ModelType string

const (
	// LocalModel  is model built based on cluster information
	LocalModel = ModelType("local")

	// RemoteModel is model built based on cloud information
	RemoteModel = ModelType("remote")
)

type IModelBuilder interface {
	Build(reqCtx *RequestContext, identity *elbmodel.PoolIdentity) (*elbmodel.EdgeLoadBalancer, error)
}

type ModelBuilder struct {
	ELBMgr *ELBManager
	LisMgr *ListenerManager
	SGMgr  *ServerGroupManager
}

func NewModelBuilder(elbMgr *ELBManager, lisMgr *ListenerManager, sgMgr *ServerGroupManager) *ModelBuilder {
	return &ModelBuilder{
		ELBMgr: elbMgr,
		LisMgr: lisMgr,
		SGMgr:  sgMgr,
	}
}

func (builder *ModelBuilder) Instance(modelType ModelType) IModelBuilder {
	switch modelType {
	case LocalModel:
		return &localModel{builder}
	case RemoteModel:
		return &remoteModel{builder}
	}
	return &localModel{builder}
}

func (builder *ModelBuilder) BuildModel(reqCtx *RequestContext, modelType ModelType, identity *elbmodel.PoolIdentity) (*elbmodel.EdgeLoadBalancer, error) {
	return builder.Instance(modelType).Build(reqCtx, identity)
}

type localModel struct{ *ModelBuilder }

func (l localModel) Build(reqCtx *RequestContext, pool *elbmodel.PoolIdentity) (*elbmodel.EdgeLoadBalancer, error) {
	mdl, err := l.ELBMgr.BuildLocalModel(reqCtx, pool)
	if err != nil {
		return mdl, fmt.Errorf("build elb attribute error: %s", err.Error())
	}

	if err = l.SGMgr.BuildLocalModel(reqCtx, pool.GetName(), mdl); err != nil {
		return mdl, fmt.Errorf("build server group error: %s", err.Error())
	}

	if err = l.LisMgr.BuildLocalModel(reqCtx, mdl); err != nil {
		return mdl, fmt.Errorf("build elb listener error: %s", err.Error())
	}
	return mdl, nil
}

type remoteModel struct{ *ModelBuilder }

func (r remoteModel) Build(reqCtx *RequestContext, identity *elbmodel.PoolIdentity) (*elbmodel.EdgeLoadBalancer, error) {
	mdl, err := r.ELBMgr.BuildRemoteModel(reqCtx, identity)
	if err != nil {
		return mdl, fmt.Errorf("build elb attribute error: %s", err.Error())
	}

	if err := r.SGMgr.BuildRemoteModel(reqCtx, mdl); err != nil {
		return mdl, fmt.Errorf("build server group error: %s", err.Error())
	}

	if err := r.LisMgr.BuildRemoteModel(reqCtx, mdl); err != nil {
		return mdl, fmt.Errorf("build elb listener error: %s", err.Error())
	}

	return mdl, nil
}
