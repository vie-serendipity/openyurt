package cloud

import (
	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/requests"
	"github.com/aliyun/alibaba-cloud-sdk-go/services/cbn"
	"k8s.io/klog/v2"

	vsagerror "github.com/openyurtio/openyurt/pkg/controller/util/errors"
)

type ChildInstanceType string

const (
	ChildInstanceVpc ChildInstanceType = "VPC"
)

type CenClient struct {
	client *cbn.Client
	region string
}

func (c *CenClient) PublishRoutes(cenId, vpcId, tableId string, routes []string) error {
	existedRoutes, err := c.listPublishedRoutes(cenId, vpcId, tableId)
	if err != nil {
		return err
	}

	klog.V(4).Infof("publishing routes: %v, cen: %s, vpc: %s, table %s", routes, cenId, vpcId, tableId)
	mergedErr := vsagerror.Errors{}
	for _, route := range routes {
		found := false
		for _, existedRoute := range existedRoutes {
			if existedRoute == route {
				found = true
				break
			}
		}
		if found {
			klog.V(4).Infof("route %s has already been published, cen: %s, vpc: %s, table %s", route, cenId, vpcId, tableId)
			continue
		}
		if err := c.publishRoute(cenId, vpcId, tableId, route); err != nil {
			mergedErr.Add(err)
		} else {
			klog.Infof("published route: %v, cen: %s, vpc: %s, table %s", route, cenId, vpcId, tableId)
		}
	}
	return mergedErr.Err()
}

func (c *CenClient) listPublishedRoutes(cenId, vpcId, tableId string) ([]string, error) {
	ret := []string{}
	for i := 1; ; i++ {
		routes, isFinished, err := c.listPublishedRoutesImpl(cenId, vpcId, tableId, i)
		if err != nil {
			return nil, err
		}
		ret = append(ret, routes...)
		if isFinished {
			break
		}
	}

	return ret, nil
}

func (c *CenClient) listPublishedRoutesImpl(cenId, vpcId, tableId string, index int) ([]string, bool, error) {
	req := cbn.CreateDescribePublishedRouteEntriesRequest()
	req.Scheme = "https"
	req.ConnectTimeout = connectionTimeout
	req.ReadTimeout = readTimeout
	req.CenId = cenId
	req.ChildInstanceId = vpcId
	req.ChildInstanceType = string(ChildInstanceVpc)
	req.ChildInstanceRegionId = c.region
	req.ChildInstanceRouteTableId = tableId
	req.PageNumber = requests.NewInteger(index)

	pageSize := 50
	req.PageSize = requests.NewInteger(pageSize)

	klog.V(4).Infof("alicloud: DescribePublishedRouteEntries req: %v", req)
	response, err := c.client.DescribePublishedRouteEntries(req)
	if err != nil {
		return nil, false, shrinkError(err)
	}
	klog.V(4).Infof("alicloud: DescribePublishedRouteEntries response: %v", response.String())

	isFinished := false
	if response.TotalCount <= index*pageSize {
		isFinished = true
	}

	routes := []string{}
	for _, e := range response.PublishedRouteEntries.PublishedRouteEntry {
		if e.RouteType != string(RouteEntryCustom) ||
			e.PublishStatus != "Published" ||
			e.NextHopType != "Instance" {
			continue
		}
		routes = append(routes, e.DestinationCidrBlock)
	}

	return routes, isFinished, nil
}

func (c *CenClient) publishRoute(cenId, vpcId, tableId, route string) error {
	req := cbn.CreatePublishRouteEntriesRequest()
	req.Scheme = "https"
	req.ConnectTimeout = connectionTimeout
	req.ReadTimeout = readTimeout
	req.CenId = cenId
	req.ChildInstanceId = vpcId
	req.ChildInstanceType = string(ChildInstanceVpc)
	req.ChildInstanceRegionId = c.region
	req.ChildInstanceRouteTableId = tableId
	req.DestinationCidrBlock = route

	klog.V(4).Infof("alicloud: PublishRouteEntries req: %v", req)
	response, err := c.client.PublishRouteEntries(req)
	if err != nil {
		return shrinkError(err)
	}
	klog.V(4).Infof("alicloud: PublishRouteEntries response: %v", response.String())

	return nil
}
