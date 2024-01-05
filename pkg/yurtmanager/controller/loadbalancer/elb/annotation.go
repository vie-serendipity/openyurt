package elb

import (
	"fmt"

	v1 "k8s.io/api/core/v1"

	elbmodel "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/util/cloudprovider/model/elb"
)

const (
	AnnotationPrefix             = "service.openyurt.io/"
	AnnotationLoadBalancerPrefix = "elb-"
	AnnotationEIPPrefix          = "eip-"

	NodePoolSelector          = "nodepool-labelselector"
	NodePoolLabelSelectorTerm = "nodepool-labelselector-term"

	ListenerOverride = AnnotationLoadBalancerPrefix + "force-override-listeners"
	BackendOverride  = AnnotationLoadBalancerPrefix + "force-override-servers"
	AddressType      = AnnotationLoadBalancerPrefix + "address-type"
)

const (
	VSwitch        = AnnotationLoadBalancerPrefix + "vswitch-id"
	LoadBalancerId = AnnotationLoadBalancerPrefix + "loadbalancer-id"
	Spec           = AnnotationLoadBalancerPrefix + "spec"
	IPVersion      = AnnotationLoadBalancerPrefix + "ip-version"
	PayType        = AnnotationLoadBalancerPrefix + "pay-type"

	EipBandwidth               = AnnotationLoadBalancerPrefix + AnnotationEIPPrefix + "bandwidth"
	EipInternetChargeType      = AnnotationLoadBalancerPrefix + AnnotationEIPPrefix + "internet-chargetype"
	EipInstanceChargeType      = AnnotationLoadBalancerPrefix + AnnotationEIPPrefix + "instance-chargetype"
	EipInternetProviderService = AnnotationLoadBalancerPrefix + AnnotationEIPPrefix + "isp"

	EdgeServerWeight    = AnnotationLoadBalancerPrefix + "backend-weight"
	BackendLabel        = AnnotationLoadBalancerPrefix + "backend-label"
	ExcludeBackendLabel = AnnotationLoadBalancerPrefix + "exclude-from-edge-load-balancer"
	RemoveUnscheduled   = AnnotationLoadBalancerPrefix + "remove-unscheduled-backend"

	Scheduler                 = AnnotationLoadBalancerPrefix + "scheduler"                    // Scheduler slb scheduler
	HealthCheckConnectPort    = AnnotationLoadBalancerPrefix + "health-check-connect-port"    // HealthCheckConnectPort health check connect port
	HealthyThreshold          = AnnotationLoadBalancerPrefix + "healthy-threshold"            // HealthyThreshold health check healthy thresh hold
	UnhealthyThreshold        = AnnotationLoadBalancerPrefix + "unhealthy-threshold"          // UnhealthyThreshold health check unhealthy thresh hold
	HealthCheckInterval       = AnnotationLoadBalancerPrefix + "health-check-interval"        // HealthCheckInterval health check interval
	HealthCheckConnectTimeout = AnnotationLoadBalancerPrefix + "health-check-connect-timeout" // HealthCheckConnectTimeout health check connect timeout
	PersistenceTimeout        = AnnotationLoadBalancerPrefix + "persistence-timeout"          // PersistenceTimeout persistence timeout
	EstablishedTimeout        = AnnotationLoadBalancerPrefix + "established-timeout"          // EstablishedTimeout connection established time out for TCP
)

var DefaultValue = map[string]string{
	composite(AnnotationPrefix, AddressType): elbmodel.IntranetAddressType,
	composite(AnnotationPrefix, Spec):        elbmodel.ELBDefaultSpec,
	composite(AnnotationPrefix, IPVersion):   elbmodel.IPv4,
}

func composite(p, k string) string {
	return fmt.Sprintf("%s%s", p, k)
}

func Annotation(k string) string {
	return composite(AnnotationPrefix, k)
}

type AnnotationContext struct {
	anno map[string]string
}

func NewAnnotationContext(svc *v1.Service) *AnnotationContext {
	annoCtx := &AnnotationContext{anno: make(map[string]string)}
	if svc == nil {
		return annoCtx
	}
	for k, v := range svc.Annotations {
		annoCtx.anno[k] = v
	}
	return annoCtx
}

func (n *AnnotationContext) Get(k string) string {
	key := composite(AnnotationPrefix, k)
	v, ok := n.anno[key]
	if ok {
		return v
	}
	return ""
}

func (n *AnnotationContext) Has(k string) bool {
	key := composite(AnnotationPrefix, k)
	_, ok := n.anno[key]
	if ok {
		return true
	}
	return false
}

func (n *AnnotationContext) GetDefaultValue(k string) string {
	return DefaultValue[composite(AnnotationPrefix, k)]
}
