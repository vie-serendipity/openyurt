package elb

const (
	EipDefaultBandwidth          = 5
	EipDefaultInstanceChargeType = "PostPaid"
	EipDefaultInternetChargeType = "95BandwidthByMonth"
)

type EdgeEipAttribute struct {
	Name                    string
	Description             string
	EnsRegionId             string
	AllocationId            string
	IpAddress               string
	InstanceId              string
	InstanceType            string
	Status                  string
	InternetChargeType      string
	InstanceChargeType      string
	InternetProviderService string
	Bandwidth               int
}
