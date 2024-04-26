package elb

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"

	prvd "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/util/cloudprovider"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/util/cloudprovider/model/elb"
)

func NewListenerManager(cloud prvd.Provider) *ListenerManager {
	return &ListenerManager{
		cloud: cloud,
	}
}

type ListenerManager struct {
	cloud prvd.Provider
}

func (mgr *ListenerManager) BuildLocalModel(reqCtx *RequestContext, lModel *elb.EdgeLoadBalancer) error {
	for _, port := range reqCtx.Service.Spec.Ports {
		listener, err := mgr.buildListenerFromServicePort(reqCtx, port)
		if err != nil {
			return fmt.Errorf("build local listener from servicePort %d error: %s", port.Port, err.Error())
		}
		lModel.Listeners.BackListener = append(lModel.Listeners.BackListener, listener)
	}
	return mgr.addExternalProtoPorts(reqCtx, lModel)
}

func (mgr *ListenerManager) addExternalProtoPorts(reqCtx *RequestContext, lModel *elb.EdgeLoadBalancer) error {
	exposedPorts := make(map[string]struct{})
	extraProtocolPorts := make([]v1.ServicePort, 0)
	for _, listen := range lModel.Listeners.BackListener {
		exposedPorts[fmt.Sprintf("%s:%d", listen.ListenerProtocol, listen.ListenerPort)] = struct{}{}
	}
	if reqCtx.AnnoCtx.Get(ProtocolPort) != "" {
		protocolPorts, err := GetProtocolPort(reqCtx.AnnoCtx.Get(ProtocolPort))
		if err != nil {
			return fmt.Errorf("add extra proto ports error %s", err.Error())
		}
		for _, v := range protocolPorts {
			if _, ok := exposedPorts[v.String()]; ok {
				continue
			}
			switch v.Protocol {
			case ProtocolTCP:
				extraProtocolPorts = append(extraProtocolPorts, v1.ServicePort{Protocol: v1.ProtocolTCP, Port: int32(v.Port), NodePort: int32(v.Port)})
			case ProtocolUDP:
				extraProtocolPorts = append(extraProtocolPorts, v1.ServicePort{Protocol: v1.ProtocolUDP, Port: int32(v.Port), NodePort: int32(v.Port)})
			default:

			}
		}
	}
	for _, port := range extraProtocolPorts {
		listener, err := mgr.buildListenerFromServicePort(reqCtx, port)
		if err != nil {
			return fmt.Errorf("build local listener from servicePort %d error: %s", port.Port, err.Error())
		}
		lModel.Listeners.BackListener = append(lModel.Listeners.BackListener, listener)
	}
	return nil
}

func (mgr *ListenerManager) BuildRemoteModel(reqCtx *RequestContext, mdl *elb.EdgeLoadBalancer) error {

	err := mgr.cloud.FindEdgeLoadBalancerListener(reqCtx.Ctx, mdl.GetLoadBalancerId(), &mdl.Listeners)
	if err != nil {
		return fmt.Errorf("build remote listener for load balancer %s error: %s", mdl.GetLoadBalancerId(), err.Error())
	}

	err = mgr.buildListenerFromCloud(reqCtx, mdl.GetLoadBalancerId(), &mdl.Listeners)
	if err != nil {
		return fmt.Errorf("build remote listener for load balancer %s error: %s", mdl.GetLoadBalancerId(), err.Error())
	}

	return nil
}

func (mgr *ListenerManager) buildListenerFromServicePort(reqCtx *RequestContext, port v1.ServicePort) (elb.EdgeListenerAttribute, error) {
	listenPort := port.NodePort
	listener := elb.EdgeListenerAttribute{
		NamedKey: &elb.ListenerNamedKey{
			NamedKey: elb.NamedKey{
				Prefix:      elb.DEFAULT_PREFIX,
				CID:         reqCtx.ClusterId,
				Namespace:   reqCtx.Service.Namespace,
				ServiceName: reqCtx.Service.Name,
			},
			Port: listenPort,
		},
		ListenerPort:     int(listenPort),
		ListenerProtocol: strings.ToLower(string(port.Protocol)),
		IsUserManaged:    false,
	}

	err := setListenerFromDefaultConfig(&listener)
	if err != nil {
		return listener, fmt.Errorf("build listener from default config error: %s", err.Error())
	}

	err = setListenerFromAnnotation(reqCtx, &listener)
	if err != nil {
		return listener, fmt.Errorf("build listener from annotation error: %s", err.Error())
	}
	return listener, nil
}

func (mgr *ListenerManager) buildListenerFromCloud(reqCtx *RequestContext, lbId string, listeners *elb.EdgeListeners) error {
	if len(listeners.BackListener) < 1 {
		return nil
	}
	backListeners := make([]elb.EdgeListenerAttribute, 0, len(listeners.BackListener))
	for _, listener := range listeners.BackListener {
		ls := new(elb.EdgeListenerAttribute)
		switch listener.ListenerProtocol {
		case elb.ProtocolTCP:
			if err := mgr.cloud.DescribeEdgeLoadBalancerTCPListener(reqCtx.Ctx, lbId, listener.ListenerPort, ls); err != nil {
				return err
			}
		case elb.ProtocolUDP:
			if err := mgr.cloud.DescribeEdgeLoadBalancerUDPListener(reqCtx.Ctx, lbId, listener.ListenerPort, ls); err != nil {
				return err
			}
		case elb.ProtocolHTTP:
			if err := mgr.cloud.DescribeEdgeLoadBalancerHTTPListener(reqCtx.Ctx, lbId, listener.ListenerPort, ls); err != nil {
				return err
			}
		case elb.ProtocolHTTPS:
			if err := mgr.cloud.DescribeEdgeLoadBalancerHTTPSListener(reqCtx.Ctx, lbId, listener.ListenerPort, ls); err != nil {
				return err
			}
		default:
			continue
		}
		nameKey, err := elb.LoadListenerNamedKey(ls.Description)
		if err != nil {
			ls.IsUserManaged = true
			klog.InfoS(fmt.Sprintf("listener description [%s], not expected format. skip user managed port", ls.Description), "service", Key(reqCtx.Service))
		} else {
			ls.IsUserManaged = false
		}
		ls.NamedKey = nameKey
		backListeners = append(backListeners, *ls)
	}
	listeners.BackListener = backListeners
	return nil
}

func (mgr *ListenerManager) batchAddListeners(reqCtx *RequestContext, lbId string, listeners *[]elb.EdgeListenerAttribute) error {
	listenPorts := *listeners
	for _, listener := range listenPorts {
		switch listener.ListenerProtocol {
		case elb.ProtocolTCP:
			if err := mgr.cloud.CreateEdgeLoadBalancerTCPListener(reqCtx.Ctx, lbId, &listener); err != nil {
				return err
			}
		case elb.ProtocolUDP:
			if err := mgr.cloud.CreateEdgeLoadBalancerUDPListener(reqCtx.Ctx, lbId, &listener); err != nil {
				return err
			}
		default:
			continue
		}
		if err := mgr.waitFindListeners(reqCtx, lbId, listener.ListenerPort, listener.ListenerProtocol); err != nil {
			return fmt.Errorf("batch add listener error %s", err.Error())
		}
		klog.InfoS(fmt.Sprintf("finish add listeners %s:%d from load balancer %s", listener.ListenerProtocol, listener.ListenerPort, lbId), "service", Key(reqCtx.Service))
	}
	return nil
}

func (mgr *ListenerManager) batchRemoveListeners(reqCtx *RequestContext, lbId string, listeners *[]elb.EdgeListenerAttribute) error {
	listenPorts := *listeners
	for _, listener := range listenPorts {
		err := mgr.cloud.DeleteEdgeLoadBalancerListener(reqCtx.Ctx, lbId, listener.ListenerPort, listener.ListenerProtocol)
		if err != nil {
			return err
		}
		klog.InfoS(fmt.Sprintf("finish remove listeners %s:%d from load balancer %s", listener.ListenerProtocol, listener.ListenerPort, lbId), "service", Key(reqCtx.Service))
	}

	return nil
}

func (mgr *ListenerManager) batchUpdateListeners(reqCtx *RequestContext, lbId string, listeners *[]elb.EdgeListenerAttribute) error {
	listenPorts := *listeners
	for _, listener := range listenPorts {
		switch listener.ListenerProtocol {
		case elb.ProtocolTCP:
			if err := mgr.cloud.ModifyEdgeLoadBalancerTCPListener(reqCtx.Ctx, lbId, &listener); err != nil {
				return err
			}
		case elb.ProtocolUDP:
			if err := mgr.cloud.ModifyEdgeLoadBalancerUDPListener(reqCtx.Ctx, lbId, &listener); err != nil {
				return err
			}
		default:
			continue
		}
		if err := mgr.waitFindListeners(reqCtx, lbId, listener.ListenerPort, listener.ListenerProtocol); err != nil {
			return err
		}
		klog.InfoS(fmt.Sprintf("finish update listeners %s:%d from load balancer %s", listener.ListenerProtocol, listener.ListenerPort, lbId), "service", Key(reqCtx.Service))
	}
	return nil
}

func (mgr *ListenerManager) waitFindListeners(reqCtx *RequestContext, lbId string, port int, protocol string) error {
	waitErr := RetryImmediateOnError(10*time.Second, time.Minute, canSkipError, func() error {
		lis := new(elb.EdgeListenerAttribute)
		switch protocol {
		case elb.ProtocolTCP:
			if err := mgr.cloud.DescribeEdgeLoadBalancerTCPListener(reqCtx.Ctx, lbId, port, lis); err != nil {
				return fmt.Errorf("find no edge loadbalancer listener err %s", err.Error())
			}
		case elb.ProtocolUDP:
			if err := mgr.cloud.DescribeEdgeLoadBalancerUDPListener(reqCtx.Ctx, lbId, port, lis); err != nil {
				return fmt.Errorf("find no edge loadbalancer listener err %s", err.Error())
			}
		}
		if lis == nil || lis.Status == "" {
			return fmt.Errorf("find no edge loadbalancer listener by port %s", fmt.Sprintf("%s:%d", protocol, port))
		}

		if lis.Status == ListenerRunning || lis.Status == ListenerStarting {
			return nil
		}
		if lis.Status == ListenerConfiguring || lis.Status == ListenerStopping {
			return fmt.Errorf("listener %s status is aberrant", lis.NamedKey.String())
		}
		if lis.Status == ListenerStopped {
			if err := mgr.cloud.StartEdgeLoadBalancerListener(reqCtx.Ctx, lbId, port, protocol); err != nil {
				return err
			}
		}
		return nil
	})
	if waitErr != nil {
		return waitErr
	}
	return nil
}

func setListenerFromDefaultConfig(listener *elb.EdgeListenerAttribute) error {
	listener.Scheduler = elb.ListenerDefaultScheduler
	listener.HealthThreshold = elb.ListenerDefaultHealthThreshold
	listener.UnhealthyThreshold = elb.ListenerDefaultUnhealthyThreshold
	listener.Description = listener.ListenKey()
	switch listener.ListenerProtocol {
	case elb.ProtocolTCP:
		listener.PersistenceTimeout = elb.ListenerDefaultPersistenceTimeout
		listener.EstablishedTimeout = elb.ListenerDefaultEstablishedTimeout
		listener.HealthCheckConnectTimeout = elb.ListenerTCPDefaultHealthCheckConnectTimeout
		listener.HealthCheckInterval = elb.ListenerTCPDefaultHealthCheckInterval
		listener.HealthCheckType = elb.ProtocolTCP
		return nil
	case elb.ProtocolUDP:
		listener.HealthCheckConnectTimeout = elb.ListenerUDPDefaultHealthCheckConnectTimeout
		listener.HealthCheckInterval = elb.ListenerUDPDefaultHealthCheckInterval
		return nil
	}
	return fmt.Errorf("unknown listening protocol %s", listener.ListenerProtocol)
}

func setListenerFromAnnotation(reqCtx *RequestContext, listener *elb.EdgeListenerAttribute) error {
	var err error
	if strings.ToLower(reqCtx.AnnoCtx.Get(Scheduler)) != "" {
		listener.Scheduler = reqCtx.AnnoCtx.Get(Scheduler)
	}

	if reqCtx.AnnoCtx.Get(HealthCheckConnectPort) != "" {
		healthCheckConnectPort, err := strconv.Atoi(reqCtx.AnnoCtx.Get(HealthCheckConnectPort))
		if err != nil {
			return fmt.Errorf("Annotation healthy threshold must be integer, but got [%s]. message=[%s] ",
				reqCtx.AnnoCtx.Get(HealthCheckConnectPort), err.Error())
		}
		listener.HealthCheckConnectPort = healthCheckConnectPort
	}

	if reqCtx.AnnoCtx.Get(HealthyThreshold) != "" {
		healthyThreshold, err := strconv.Atoi(reqCtx.AnnoCtx.Get(HealthyThreshold))
		if err != nil {
			return fmt.Errorf("Annotation healthy threshold must be integer, but got [%s]. message=[%s] ",
				reqCtx.AnnoCtx.Get(HealthyThreshold), err.Error())
		}
		listener.HealthThreshold = healthyThreshold
	}

	if reqCtx.AnnoCtx.Get(UnhealthyThreshold) != "" {
		unhealthyThreshold, err := strconv.Atoi(reqCtx.AnnoCtx.Get(UnhealthyThreshold))
		if err != nil {
			return fmt.Errorf("Annotation unhealthy threshold must be integer, but got [%s]. message=[%s] ",
				reqCtx.AnnoCtx.Get(UnhealthyThreshold), err.Error())
		}
		listener.UnhealthyThreshold = unhealthyThreshold
	}

	if reqCtx.AnnoCtx.Get(HealthCheckInterval) != "" {
		healthyCheckInterval, err := strconv.Atoi(reqCtx.AnnoCtx.Get(HealthCheckInterval))
		if err != nil {
			return fmt.Errorf("Annotation healthy check interval must be integer, but got [%s]. message=[%s] ",
				reqCtx.AnnoCtx.Get(HealthCheckInterval), err.Error())
		}
		listener.HealthCheckInterval = healthyCheckInterval
	}

	if reqCtx.AnnoCtx.Get(HealthCheckConnectTimeout) != "" {
		healthCheckConnectTimeout, err := strconv.Atoi(reqCtx.AnnoCtx.Get(HealthCheckConnectTimeout))
		if err != nil {
			return fmt.Errorf("Annotation health check connect timeout must be integer, but got [%s]. message=[%s] ",
				reqCtx.AnnoCtx.Get(HealthCheckConnectTimeout), err.Error())
		}
		listener.HealthCheckConnectTimeout = healthCheckConnectTimeout
	}

	if listener.ListenerProtocol == elb.ProtocolUDP {
		return nil
	}

	if reqCtx.AnnoCtx.Get(PersistenceTimeout) != "" {
		timeout, err := strconv.Atoi(reqCtx.AnnoCtx.Get(PersistenceTimeout))
		if err != nil {
			return fmt.Errorf("annotation persistence timeout must be integer, but got [%s]. message=[%s] ",
				reqCtx.AnnoCtx.Get(PersistenceTimeout), err.Error())
		}
		if timeout != 0 && reqCtx.Service.Spec.ExternalTrafficPolicy == v1.ServiceExternalTrafficPolicyTypeCluster {
			return fmt.Errorf("session persistence not support external traffic policy using cluster mode")
		}
		listener.PersistenceTimeout = timeout
	}

	if reqCtx.AnnoCtx.Get(EstablishedTimeout) != "" {
		establishedTimeout, err := strconv.Atoi(reqCtx.AnnoCtx.Get(EstablishedTimeout))
		if err != nil {
			return fmt.Errorf("Annotation established timeout must be integer, but got [%s]. message=[%s] ",
				reqCtx.AnnoCtx.Get(EstablishedTimeout), err.Error())
		}
		listener.EstablishedTimeout = establishedTimeout
	}

	return err
}

func getListeners(reqCtx *RequestContext, localModel, remoteModel *elb.EdgeLoadBalancer) (add, remove, update []elb.EdgeListenerAttribute, err error) {
	add = make([]elb.EdgeListenerAttribute, 0)
	remove = make([]elb.EdgeListenerAttribute, 0)
	update = make([]elb.EdgeListenerAttribute, 0)
	if len(localModel.Listeners.BackListener) < 1 && len(remoteModel.Listeners.BackListener) < 1 {
		return
	}
	localMap := make(map[string]int)
	remoteMap := make(map[string]int)
	httpPortMap := make(map[string]string)
	for idx, local := range localModel.Listeners.BackListener {
		localMap[fmt.Sprintf("%s:%d", local.ListenerProtocol, local.ListenerPort)] = idx
	}

	for idx, remote := range remoteModel.Listeners.BackListener {
		switch remote.ListenerProtocol {
		case elb.ProtocolHTTP:
			httpPortMap[strconv.Itoa(remote.ListenerPort)] = elb.ProtocolHTTP
		case elb.ProtocolHTTPS:
			httpPortMap[strconv.Itoa(remote.ListenerPort)] = elb.ProtocolHTTPS
		default:
			remoteMap[fmt.Sprintf("%s:%d", remote.ListenerProtocol, remote.ListenerPort)] = idx
		}
	}
	for _, remote := range remoteModel.Listeners.BackListener {
		_, ok := localMap[fmt.Sprintf("%s:%d", remote.ListenerProtocol, remote.ListenerPort)]
		if !ok {
			if canRemoveListener(reqCtx.ClusterId, remoteModel, remote) {
				remove = append(remove, remote)
			}
		}
	}

	for _, local := range localModel.Listeners.BackListener {
		// if conflict with http/https port, return err
		if proto, ok := httpPortMap[strconv.Itoa(local.ListenerPort)]; ok {
			err = fmt.Errorf("listener %s:%d conflict with %s:%d",
				local.ListenerProtocol, local.ListenerPort, proto, local.ListenerPort)
			return
		}
		// add the listener if protocol:port is not found in remote,
		// update the listener if protocol:port is found in remote, but it is not manager by user
		idx, ok := remoteMap[fmt.Sprintf("%s:%d", local.ListenerProtocol, local.ListenerPort)]
		if !ok {
			add = append(add, local)
			continue
		}
		if localModel.IsShared() && listenerIsChanged(&local, &remoteModel.Listeners.BackListener[idx]) {
			update = append(update, local)
		}
	}
	return
}

func canRemoveListener(clusterId string, mdl *elb.EdgeLoadBalancer, listener elb.EdgeListenerAttribute) bool {
	if listener.IsUserManaged {
		return false
	}
	if listener.NamedKey.CID != clusterId {
		return false
	}
	if listener.NamedKey.Namespace != mdl.NamespacedName.Namespace {
		return false
	}
	if listener.NamedKey.ServiceName != mdl.NamespacedName.Name {
		return false
	}
	return true
}

func listenerIsChanged(local, remote *elb.EdgeListenerAttribute) bool {
	if !isEqual(local.Scheduler, remote.Scheduler) {
		return true
	}
	if !isEqual(local.HealthThreshold, remote.HealthThreshold) {
		return true
	}
	if !isEqual(local.UnhealthyThreshold, remote.UnhealthyThreshold) {
		return true
	}
	if !isEqual(local.HealthCheckConnectTimeout, remote.HealthCheckConnectTimeout) {
		return true
	}
	if !isEqual(local.HealthCheckInterval, remote.HealthCheckInterval) {
		return true
	}
	if !isEqual(local.HealthCheckConnectPort, remote.HealthCheckConnectPort) {
		return true
	}
	if local.ListenerProtocol == elb.ProtocolTCP && remote.ListenerProtocol == elb.ProtocolTCP {
		if !isEqual(local.EstablishedTimeout, remote.EstablishedTimeout) {
			return true
		}
		if !isEqual(local.PersistenceTimeout, remote.PersistenceTimeout) {
			return true
		}
		if !isEqual(local.HealthCheckType, remote.HealthCheckType) {
			return true
		}
	}
	return false
}

type ProtocolPorts struct {
	Protocol string
	Port     int
}

func (p *ProtocolPorts) String() string {
	return fmt.Sprintf("%s:%d", p.Protocol, p.Port)
}

func GetProtocolPort(anno string) (protocolPorts []ProtocolPorts, err error) {
	anno = strings.ToLower(anno)
	if anno == "" {
		err = fmt.Errorf("annotation protocol:port is empty")
		return
	}
	var port int
	for _, v := range strings.Split(anno, ",") {
		pp := strings.Split(v, ":")
		if len(pp) != 2 {
			err = fmt.Errorf("port and protocol format must be like 'https:443' with colon separated. got=[%+v]", pp)
			return
		}
		if pp[0] != ProtocolHttp && pp[0] != ProtocolHttps && pp[0] != ProtocolTCP && pp[0] != ProtocolUDP {
			err = fmt.Errorf("port protocol format must be either [http|https|tcp|udp], protocol not supported wit [%s]", pp[0])
			return
		}
		port, err = strconv.Atoi(pp[1])
		if err != nil {
			err = fmt.Errorf("port format must be 32-bit decimal number， port not supported wit [%s]", pp[1])
			return
		}
		protocolPorts = append(protocolPorts, ProtocolPorts{Protocol: pp[0], Port: port})
	}
	return
}
