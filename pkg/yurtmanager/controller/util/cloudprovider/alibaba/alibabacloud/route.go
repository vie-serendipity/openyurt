package alibabacloud

import (
	"context"
	"fmt"
	"strings"

	"k8s.io/klog/v2"

	"github.com/aliyun/alibaba-cloud-sdk-go/services/vpc"
	provider "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/util/cloudprovider"
	routemodel "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/util/cloudprovider/model/route"
)

var _ provider.IRouteTables = &VPCProvider{}

func (v *VPCProvider) CreateRoute(ctx context.Context, tableId string, mdl *routemodel.Route) error {
	req := vpc.CreateCreateRouteEntryRequest()
	req.RouteTableId = tableId
	req.DestinationCidrBlock = mdl.DestinationCIDR
	req.NextHopId = mdl.NextHotId
	req.NextHopType = mdl.NextHotType
	req.RouteEntryName = mdl.Name
	req.Description = mdl.Description
	resp, err := v.auth.VPC.CreateRouteEntry(req)
	if err != nil {
		return fmt.Errorf("[table: %s]error create route entry for %s, %s, error: %v", tableId, mdl.NextHotId, mdl.DestinationCIDR, err)
	}
	klog.Infof("RequestId: %s, API: CreateRouteEntry, tableId: %s, destinationCIDR: %s, nextHotId: %s", resp.RequestId, tableId, mdl.DestinationCIDR, mdl.NextHotId)
	if resp != nil {
		mdl.Id = resp.RouteEntryId
	}
	return nil
}

func (v *VPCProvider) DeleteRoute(ctx context.Context, routeEntryId, nextHotId string) error {
	req := vpc.CreateDeleteRouteEntryRequest()
	req.RouteEntryId = routeEntryId
	req.NextHopId = nextHotId
	resp, err := v.auth.VPC.DeleteRouteEntry(req)
	if err != nil {
		if strings.Contains(err.Error(), "InvalidRouteEntryId.NotFound") {
			// route already removed
			return nil
		}
		return err
	}
	klog.Infof("RequestId: %s, API: DeleteRouteEntry, routeEntryId: %s, nextHotId: %s", resp.RequestId, routeEntryId, nextHotId)
	return nil
}

func (v *VPCProvider) FindRoute(ctx context.Context, tableId, destinationCIDR string) ([]*routemodel.Route, error) {
	req := vpc.CreateDescribeRouteEntryListRequest()
	req.RouteTableId = tableId
	req.RouteEntryType = "Custom"
	req.DestinationCidrBlock = destinationCIDR
	resp, err := v.auth.VPC.DescribeRouteEntryList(req)
	if err != nil {
		return nil, fmt.Errorf("error describe route entry list: %v", err)
	}
	klog.Infof("RequestId: %s, API: DescribeRouteEntryList, tableId: %s, destinationCIDR: %s", resp.RequestId, tableId, destinationCIDR)
	routes := make([]*routemodel.Route, 0)
	for _, entry := range resp.RouteEntrys.RouteEntry {
		if len(entry.NextHops.NextHop) > 0 && entry.NextHops.NextHop[0].NextHopType == routemodel.DefaultNextHopTypeInstance {
			route := routemodel.NewRouteModel()
			route.Id = entry.RouteEntryId
			route.Name = entry.RouteEntryName
			route.Description = entry.Description
			route.DestinationCIDR = entry.DestinationCidrBlock
			route.NextHotId = entry.NextHops.NextHop[0].NextHopId
			route.NextHotType = entry.NextHops.NextHop[0].NextHopType
			route.Status = entry.Status
			routes = append(routes, route)
		}
	}
	return routes, nil
}

func (v *VPCProvider) ListRouteTables(ctx context.Context, vpcId string) ([]string, error) {
	req := vpc.CreateDescribeRouteTableListRequest()
	req.VpcId = vpcId
	resp, err := v.auth.VPC.DescribeRouteTableList(req)
	if err != nil {
		return nil, fmt.Errorf("error describe vpc: %v route tables, error: %v", vpcId, err)
	}
	klog.Infof("RequestId: %s, API: DescribeRouteTableList, vpcId: %s", resp.RequestId, vpcId)
	var tableIds []string
	for _, table := range resp.RouterTableList.RouterTableListType {
		tableIds = append(tableIds, table.RouteTableId)
	}
	return tableIds, nil
}
