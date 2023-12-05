package elb

// Attributes
const (
	ELBClass     = "alibabacloud.com/elb"
	ELBFinalizer = "service.alibabacloud.com/elb"

	EnsNetworkId = "alibabacloud.com/ens-network-id"
	EnsRegionId  = "alibabacloud.com/ens-region-id"

	EnsVSwitchId = "alibabacloud.com/ens-vswitch-id"

	EnsNodeId           = "alibabacloud.com/ens-instance-id"
	LabelServiceHash    = "service.openyurt.io/hash"
	LabelLoadBalancerId = "service.openyurt.io/loadbalancer-id"
	BaseBackendWeight   = "base"
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
)

const (
	InstanceNotFound     = "find no"
	StatusAberrant       = "aberrant"
	ENSBatchAddMaxNumber = 19
)

const (
	AnnoChanged            = "AnnotationChanged"
	TypeChanged            = "TypeChanged"
	SpecChanged            = "ServiceSpecChanged"
	DeleteTimestampChanged = "DeleteTimestampChanged"
)

const (
	BatchSize     = 5
	MaxRetryError = 5
)
