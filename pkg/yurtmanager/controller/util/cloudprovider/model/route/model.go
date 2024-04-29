package route

const (
	DefaultName                = "edge-container-route"
	DefaultNextHopTypeInstance = "Instance"
	DefaultDescription         = "Create this route to forward traffic destined for container CIDR at the edge"
)

const (
	StatusDeleting  = "Deleting"
	StatusPending   = "Pending"
	StatusAvailable = "Available"
	StatusModifying = "Modifying"
)

type Route struct {
	Name            string `json:"name"`
	Id              string `json:"id"`
	Description     string `json:"description"`
	NextHotId       string `json:"nextHotId"`
	NextNodeName    string `json:"nextNodeName"`
	NextHotType     string `json:"nextHotType"`
	DestinationCIDR string `json:"destinationCIDR"`
	Status          string `json:"status"`
}

func NewRouteModel() *Route {
	return &Route{Name: DefaultName, Description: DefaultDescription, NextHotType: DefaultNextHopTypeInstance}
}
