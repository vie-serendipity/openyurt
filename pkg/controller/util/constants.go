package util

const (
	// ============= managed by upper control plane ============
	NodePoolCCNAnnotation          = "nodepool.openyurt.io/ccn-id"
	NodePoolCENAnnotation          = "nodepool.openyurt.io/cen-id"
	NodePoolCcnRegionAnnotation    = "nodepool.openyurt.io/ccn-region"
	NodePoolSagBandwidthAnnotation = "nodepool.openyurt.io/sag-bandwidth"
	NodePoolSagPeriodAnnotation    = "nodepool.openyurt.io/sag-period"

	// to denote whether current edge nodepool is default nodepool
	NodePoolIsEdgeDefaultAnnotation = "nodepool.openyurt.io/is-default"

	// to denote max number of nodes to be
	NodePoolMaxNodesAnnotation = "nodepool.openyurt.io/max-nodes"
	// to denote whether use native node ipam, regardless of nodepool
	NodePoolLegacyIpamAnnotation = "nodepool.openyurt.io/legacy-ipam"

	// ============== managed by vsag controller ============
	// record vsag id
	NodePoolSagIDAnnotation = "nodepool.openyurt.io/sag-id"
	// record vsag status
	NodePoolSagStatusAnnotation = "nodepool.openyurt.io/sag-status"

	// node label, denote the node is in vsag
	NodeManagedByVsagLabelKey = "alibabacloud.com/is-managed-by-vsag"

	// nodepool finalizer to avoid sag configuration leak
	NodePoolSagConfiguredFinalizer = "nodepool.openyurt.io/sag-configured"

	// ============== managed by nodeipam controller ============
	// Note: once specified, the below annotations cannot be mutated
	// pod cidr in format of: "192.64.1.0/24,192.64.128.0/24", splited by ","
	NodePoolPodCIDRAnnotation = "nodepool.openyurt.io/pod-cidrs"

	NodePoolCIDRFinalizer = "nodepool.openyurt.io/cidrs-allocated"
)

// new annotations to hide 'sag' chars
const (
	NodePoolCcnRegionNewAnnotation    = "nodepool.openyurt.io/egw-region"
	NodePoolSagBandwidthNewAnnotation = "nodepool.openyurt.io/egw-bandwidth"
	NodePoolSagPeriodNewAnnotation    = "nodepool.openyurt.io/egw-period"
	NodePoolSagIDNewAnnotation        = "nodepool.openyurt.io/egw-id"
	NodePoolSagStatusNewAnnotation    = "nodepool.openyurt.io/egw-status"

	// node label, denote the node is in vsag
	NodeManagedByVsagNewLabelKey = "alibabacloud.com/is-managed-by-egw"

	// nodepool finalizer to avoid sag configuration leak
	NodePoolSagConfiguredNewFinalizer = "nodepool.openyurt.io/egw-configured"
)

const (
	// labels for vsag-core and vsag-helper deployment
	K8sAppLabelKey   = "k8s-app"
	NodepoolLabelKey = "nodepool"
	NodeLabelArchKey = "kubernetes.io/arch"

	// TODO: hide 'vsag' chars
	VsagCoreAppLabelVal   = "egw-core"
	VsagHelperAppLabelVal = "egw-helper"

	NodeArchValueAmd64 = "amd64"
)

// configMapName + "-" + nodepoolName + "-" + haState
const ConfigMapNameFormat = "%s-%s-%s"

// the structure to describe sag status in nodepool annotation
type SagStatus struct {
	// success or failure
	Phase   string `json:"phase"`
	EgwID   string `json:"egwID"`
	CcnID   string `json:"ccnID"`
	UID     string `json:"uid"`
	Message string `json:"message"`
}

// configuration for cloud client
type CloudConfig struct {
	AccessKeyID     string `json:"AccessKeyID"`
	AccessKeySecret string `json:"AccessKeySecret"`
	ResourceUid     string `json:"ResourceUid"`
}

// TODO: vendor from ccm
// CloudConfig wraps the settings for the Alicloud provider.
type CCMCloudConfig struct {
	Global struct {
		KubernetesClusterTag string `json:"kubernetesClusterTag"`
		NodeMonitorPeriod    int64  `json:"nodeMonitorPeriod"`
		NodeAddrSyncPeriod   int64  `json:"nodeAddrSyncPeriod"`
		UID                  string `json:"uid"`
		VpcID                string `json:"vpcid"`
		Region               string `json:"region"`
		ZoneID               string `json:"zoneid"`
		VswitchID            string `json:"vswitchid"`
		ClusterID            string `json:"clusterID"`
		RouteTableIDS        string `json:"routeTableIDs"`

		DisablePublicSLB bool `json:"disablePublicSLB"`

		AccessKeyID     string `json:"accessKeyID"`
		AccessKeySecret string `json:"accessKeySecret"`
	}
}
