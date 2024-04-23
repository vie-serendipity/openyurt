package elb

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	ListenerDefaultScheduler                    = "wrr"
	ListenerDefaultPersistenceTimeout           = 0
	ListenerDefaultEstablishedTimeout           = 900
	ListenerDefaultHealthThreshold              = 3
	ListenerDefaultUnhealthyThreshold           = 3
	ListenerTCPDefaultHealthCheckConnectTimeout = 5
	ListenerUDPDefaultHealthCheckConnectTimeout = 10
	ListenerTCPDefaultHealthCheckInterval       = 2
	ListenerUDPDefaultHealthCheckInterval       = 5
)

const (
	ProtocolTCP   = "tcp"
	ProtocolUDP   = "udp"
	ProtocolHTTP  = "http"
	ProtocolHTTPS = "https"
)

type ListenerNamedKey struct {
	NamedKey
	Port int32
}

func (n *ListenerNamedKey) Key() string {
	if n.Prefix == "" {
		n.Prefix = DEFAULT_PREFIX
	}
	return fmt.Sprintf("%s/%d/%s/%s/%s", n.Prefix, n.Port, n.ServiceName, n.Namespace, n.CID)
}

func (n *ListenerNamedKey) String() string {
	if n == nil {
		return ""
	}
	return n.Key()
}

type EdgeListeners struct {
	BackListener []EdgeListenerAttribute
}

type EdgeListenerAttribute struct {
	NamedKey                  *ListenerNamedKey
	ListenerPort              int
	ListenerProtocol          string
	Description               string
	Scheduler                 string
	Status                    string
	HealthCheckType           string
	PersistenceTimeout        int
	EstablishedTimeout        int
	HealthThreshold           int
	UnhealthyThreshold        int
	HealthCheckConnectTimeout int
	HealthCheckInterval       int
	HealthCheckConnectPort    int
	IsUserManaged             bool
}

func (l *EdgeListenerAttribute) ListenKey() string {
	if l.NamedKey.Prefix == "" {
		l.NamedKey.Prefix = DEFAULT_PREFIX
	}
	return fmt.Sprintf("%s/%d/%s/%s/%s/%s", l.NamedKey.Prefix, l.ListenerPort, l.ListenerProtocol, l.NamedKey.ServiceName, l.NamedKey.Namespace, l.NamedKey.CID)
}

func LoadListenerNamedKey(key string) (*ListenerNamedKey, error) {
	metas := strings.Split(key, "/")
	if len(metas) != 6 || metas[0] != DEFAULT_PREFIX {
		return nil, fmt.Errorf("NamedKey Format Error: k8s.${port}.${protocol}.${service}.${namespace}.${clusterid} format is expected. Got [%s]", key)
	}
	port, err := strconv.Atoi(metas[1])
	if err != nil {
		return nil, err
	}
	return &ListenerNamedKey{
		NamedKey: NamedKey{
			CID:         metas[5],
			Namespace:   metas[4],
			ServiceName: metas[3],
			Prefix:      metas[0],
		},
		Port: int32(port),
	}, nil
}
