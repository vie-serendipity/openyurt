package cloud

import (
	"fmt"
	"strings"

	"github.com/aliyun/alibaba-cloud-sdk-go/services/vpc"
	"k8s.io/klog/v2"
)

type RouteEntryType string

const (
	RouteEntrySystem RouteEntryType = "System"
	RouteEntryCustom RouteEntryType = "Custom"
)

// all vpc api calls needs ramrole
type VpcClient struct {
	client *vpc.Client
	region string
}

func (v *VpcClient) GetRouteTables(vpcId string) ([]string, error) {
	req := vpc.CreateDescribeVpcsRequest()
	req.Scheme = "https"
	req.ConnectTimeout = connectionTimeout
	req.ReadTimeout = readTimeout
	req.VpcId = vpcId
	req.RegionId = v.region

	klog.V(4).Infof("alicloud: DescribeVpcs req: %v", req)
	response, err := v.client.DescribeVpcs(req)
	if err != nil {
		return nil, shrinkError(err)
	}
	klog.V(4).Infof("alicloud: DescribeVpcs response: %v", response.String())

	vpcs := response.Vpcs.Vpc
	if len(vpcs) != 1 {
		return nil, fmt.Errorf("alicloud: %d vpc found with id %s, expect 1", len(vpcs), vpcId)
	}
	if len(vpcs[0].RouterTableIds.RouterTableIds) != 1 {
		return nil, fmt.Errorf("alicloud: multiple route tables or no route table found in vpc %s, tables: %v", vpcId, vpcs[0].RouterTableIds.RouterTableIds)
	}

	return vpcs[0].RouterTableIds.RouterTableIds, nil
}

func (v *VpcClient) GetRouteTableRoutes(tableId string) ([]string, error) {
	routes, err := v.getRouteEntryBatch(tableId, "")
	if err != nil {
		return nil, fmt.Errorf("failed to get route entry for table %s, err: %v", tableId, err)
	}
	return routes, nil
}

func (v *VpcClient) getRouteEntryBatch(tableId, nextToken string) ([]string, error) {
	req := vpc.CreateDescribeRouteEntryListRequest()
	req.Scheme = "https"
	req.ConnectTimeout = connectionTimeout
	req.ReadTimeout = readTimeout
	req.RouteTableId = tableId
	req.NextToken = nextToken
	req.RouteEntryType = string(RouteEntryCustom)
	req.RegionId = v.region

	klog.V(4).Infof("alicloud: DescribeRouteEntryList req: %v", req)
	response, err := v.client.DescribeRouteEntryList(req)
	if err != nil {
		return nil, shrinkError(err)
	}
	klog.V(4).Infof("alicloud: DescribeRouteEntryList response: %v", response.String())

	routeEntries := response.RouteEntrys.RouteEntry
	if len(routeEntries) == 0 {
		klog.Warningf("alicloud: table %s has 0 route entry.", tableId)
	}

	routes := []string{}
	for _, e := range routeEntries {
		//skip none custom route
		if e.Type != string(RouteEntryCustom) ||
			// ECMP is not supported yet, skip next hop not equals 1
			len(e.NextHops.NextHop) != 1 ||
			// skip none Instance route
			strings.ToLower(e.NextHops.NextHop[0].NextHopType) != "instance" ||
			// skip DNAT route
			e.DestinationCidrBlock == "0.0.0.0/0" {
			continue
		}

		if e.DestinationCidrBlock == "" {
			klog.Errorf("empty destination cidr of route entry %v", e)
			continue
		}
		routes = append(routes, e.DestinationCidrBlock)
	}
	// get next batch
	if response.NextToken != "" {
		nextRoutes, err := v.getRouteEntryBatch(tableId, response.NextToken)
		if err != nil {
			return nil, err
		}
		routes = append(routes, nextRoutes...)
	}

	return routes, nil
}
