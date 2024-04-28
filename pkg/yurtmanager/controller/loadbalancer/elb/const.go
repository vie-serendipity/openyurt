package elb

// Attributes
const (
	ELBClass     = "alibabacloud.com/elb"
	ELBFinalizer = "service.alibabacloud.com/elb"

	EnsNetworkId = "alibabacloud.com/ens-network-id"
	EnsRegionId  = "alibabacloud.com/ens-region-id"
	EnsVSwitchId = "alibabacloud.com/ens-vswitch-id"

	EnsNodeId           = "alibabacloud.com/ens-instance-id"
	LabelServiceHash    = "service.beta.kubernetes.io/hash"
	LabelLoadBalancerId = "service.alibabacloud.com/loadbalancer-id"
	EipId               = "service.alibabacloud.com/eip-id"

	BaseBackendWeight = "base"
)

// Status
const (
	//ELB
	ELBActive   = "Active"
	ELBInActive = "InActive"

	//EIP
	EipAvailable = "Available"
	EipInUse     = "InUse"

	//ServerGroups
	ENSRunning = "Running"
	ENSStopped = "Stopped"
	ENSExpired = "Expired"

	//Listener
	ListenerRunning     = "Running"
	ListenerStopped     = "Stopped"
	ListenerStarting    = "Starting"
	ListenerConfiguring = "Configuring"
	ListenerStopping    = "Stopping"

	ProtocolTCP   = "tcp"
	ProtocolUDP   = "udp"
	ProtocolHttp  = "http"
	ProtocolHttps = "https"
)

const (
	InstanceNotFound     = "find no"
	StatusAberrant       = "aberrant"
	ENSBatchAddMaxNumber = 19
)
