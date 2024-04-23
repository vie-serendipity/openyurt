package elb

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/finalizer"

	networkv1alpha1 "github.com/openyurtio/openyurt/pkg/apis/network/v1alpha1"
)

const (
	FailedAddFinalizer       = "FailedAddFinalizer"
	FailedRemoveFinalizer    = "FailedRemoveFinalizer"
	FailedAddHash            = "FailedAddHash"
	FailedRemoveHash         = "FailedRemoveHash"
	FailedUpdateStatus       = "FailedUpdateStatus"
	UnAvailableBackends      = "UnAvailableLoadBalancer"
	SkipSyncBackends         = "SkipSyncBackends"
	FailedSyncLB             = "SyncLoadBalancerFailed"
	FailedValidateAnnotation = "ValidateAnnotationFailed"
	SucceedCleanLB           = "CleanLoadBalancer"
	FailedCleanLB            = "CleanLoadBalancerFailed"
	SucceedSyncLB            = "EnsuredLoadBalancer"
	WaitedSyncLBId           = "WaitBindLoadBalancer"
)

type LoadBalancerServiceFinalizer struct {
	client client.Client
}

var _ finalizer.Finalizer = &LoadBalancerServiceFinalizer{}

func NewLoadBalancerServiceFinalizer(cli client.Client) *LoadBalancerServiceFinalizer {
	return &LoadBalancerServiceFinalizer{client: cli}
}

func (l LoadBalancerServiceFinalizer) Finalize(ctx context.Context, object client.Object) (finalizer.Result, error) {
	var res finalizer.Result
	_, ok := object.(*networkv1alpha1.PoolService)
	if !ok {
		res.Updated = false
		return res, fmt.Errorf("object is not networkv1alpha1.poolservice")
	}
	res.Updated = true
	return res, nil
}

func Finalize(ctx context.Context, obj client.Object, cli client.Client, flz finalizer.Finalizer) error {
	oldObj := obj.DeepCopyObject().(client.Object)
	res, err := flz.Finalize(ctx, obj)
	if err != nil {
		return err
	}
	if res.Updated {
		return cli.Patch(ctx, obj, client.MergeFrom(oldObj))
	}
	return nil
}

// HasFinalizer tests whether k8s object has specified finalizer
func HasFinalizer(obj client.Object, finalizer string) bool {
	f := obj.GetFinalizers()
	for _, e := range f {
		if e == finalizer {
			return true
		}
	}
	return false
}
