/*
Copyright 2020 The OpenYurt Authors.
Copyright 2016 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package app implements a server that runs a set of active
// components.  This includes replication controllers, service endpoints and
// nodes.
//
package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
	netutils "k8s.io/utils/net"

	"github.com/openyurtio/openyurt/pkg/controller/certificates"
	daemonpodupdater "github.com/openyurtio/openyurt/pkg/controller/daemonpodupdater"
	lifecyclecontroller "github.com/openyurtio/openyurt/pkg/controller/nodelifecycle"
	"github.com/openyurtio/openyurt/pkg/controller/servicetopology"
	nodeipamcontroller "github.com/openyurtio/openyurt/pkg/controller/nodeipam"
	"github.com/openyurtio/openyurt/pkg/controller/nodeipam/ipam"
	"github.com/openyurtio/openyurt/pkg/controller/util"
	vsagcontroller "github.com/openyurtio/openyurt/pkg/controller/vsag"
)

func startNodeLifecycleController(ctx ControllerContext) (http.Handler, bool, error) {
	lifecycleController, err := lifecyclecontroller.NewNodeLifecycleController(
		ctx.InformerFactory.Coordination().V1().Leases(),
		ctx.InformerFactory.Core().V1().Pods(),
		ctx.InformerFactory.Core().V1().Nodes(),
		ctx.InformerFactory.Apps().V1().DaemonSets(),
		// node lifecycle controller uses existing cluster role from node-controller
		ctx.ClientBuilder.ClientOrDie("node-controller"),
		//ctx.ComponentConfig.KubeCloudShared.NodeMonitorPeriod.Duration,
		5*time.Second,
		ctx.ComponentConfig.NodeLifecycleController.NodeStartupGracePeriod.Duration,
		ctx.ComponentConfig.NodeLifecycleController.NodeMonitorGracePeriod.Duration,
		ctx.ComponentConfig.NodeLifecycleController.PodEvictionTimeout.Duration,
		ctx.ComponentConfig.NodeLifecycleController.NodeEvictionRate,
		ctx.ComponentConfig.NodeLifecycleController.SecondaryNodeEvictionRate,
		ctx.ComponentConfig.NodeLifecycleController.LargeClusterSizeThreshold,
		ctx.ComponentConfig.NodeLifecycleController.UnhealthyZoneThreshold,
		*ctx.ComponentConfig.NodeLifecycleController.EnableTaintManager,
	)
	if err != nil {
		return nil, true, err
	}
	go lifecycleController.Run(ctx.Stop)
	return nil, true, nil
}

func startYurtCSRApproverController(ctx ControllerContext) (http.Handler, bool, error) {
	clientSet := ctx.ClientBuilder.ClientOrDie("yurt-csr-controller")
	csrApprover, err := certificates.NewCSRApprover(clientSet, ctx.InformerFactory)
	if err != nil {
		return nil, false, err
	}
	go csrApprover.Run(2, ctx.Stop)

	return nil, true, nil
}

func startDaemonPodUpdaterController(ctx ControllerContext) (http.Handler, bool, error) {
	daemonPodUpdaterCtrl := daemonpodupdater.NewController(
		ctx.ClientBuilder.ClientOrDie("daemonPodUpdater-controller"),
		ctx.InformerFactory.Apps().V1().DaemonSets(),
		ctx.InformerFactory.Core().V1().Nodes(),
		ctx.InformerFactory.Core().V1().Pods(),
	)

	go daemonPodUpdaterCtrl.Run(2, ctx.Stop)
	return nil, true, nil
}

func startServiceTopologyController(ctx ControllerContext) (http.Handler, bool, error) {
	clientSet := ctx.ClientBuilder.ClientOrDie("yurt-servicetopology-controller")

	svcTopologyController, err := servicetopology.NewServiceTopologyController(
		clientSet,
		ctx.InformerFactory,
		ctx.YurtInformerFactory,
	)
	if err != nil {
		return nil, false, err
	}
	go svcTopologyController.Run(ctx.Stop)
	return nil, true, nil
}

func startNodeIpamController(ctx ControllerContext) (http.Handler, bool, error) {
	var serviceCIDR *net.IPNet
	var secondaryServiceCIDR *net.IPNet

	// failure: bad cidrs in config
	clusterCIDRs, dualStack, err := processCIDRs(ctx.ComponentConfig.NodeIPAMController.ClusterCIDR)
	if err != nil {
		panic(err)
	}

	// failure: more than one cidr but they are not configured as dual stack
	if len(clusterCIDRs) > 1 && !dualStack {
		return nil, false, fmt.Errorf("len of ClusterCIDRs==%v and they are not configured as dual stack (at least one from each IPFamily", len(clusterCIDRs))
	}

	// failure: more than cidrs is not allowed even with dual stack
	if len(clusterCIDRs) > 2 {
		return nil, false, fmt.Errorf("len of clusters is:%v > more than max allowed of 2", len(clusterCIDRs))
	}

	// service cidr processing
	if len(strings.TrimSpace(ctx.ComponentConfig.NodeIPAMController.ServiceCIDR)) != 0 {
		_, serviceCIDR, err = net.ParseCIDR(ctx.ComponentConfig.NodeIPAMController.ServiceCIDR)
		if err != nil {
			klog.Warningf("Unsuccessful parsing of service CIDR %v: %v", ctx.ComponentConfig.NodeIPAMController.ServiceCIDR, err)
		}
	}

	if len(strings.TrimSpace(ctx.ComponentConfig.NodeIPAMController.SecondaryServiceCIDR)) != 0 {
		_, secondaryServiceCIDR, err = net.ParseCIDR(ctx.ComponentConfig.NodeIPAMController.SecondaryServiceCIDR)
		if err != nil {
			klog.Warningf("Unsuccessful parsing of service CIDR %v: %v", ctx.ComponentConfig.NodeIPAMController.SecondaryServiceCIDR, err)
		}
	}

	// the following checks are triggered if both serviceCIDR and secondaryServiceCIDR are provided
	if serviceCIDR != nil && secondaryServiceCIDR != nil {
		// should be dual stack (from different IPFamilies)
		dualstackServiceCIDR, err := netutils.IsDualStackCIDRs([]*net.IPNet{serviceCIDR, secondaryServiceCIDR})
		if err != nil {
			return nil, false, fmt.Errorf("failed to perform dualstack check on serviceCIDR and secondaryServiceCIDR error:%v", err)
		}
		if !dualstackServiceCIDR {
			return nil, false, fmt.Errorf("serviceCIDR and secondaryServiceCIDR are not dualstack (from different IPfamiles)")
		}
	}

	ipamController, err := nodeipamcontroller.NewNodeIpamController(
		ctx.InformerFactory.Core().V1().Nodes(),
		ctx.YurtInformerFactory.Apps().V1alpha1().NodePools(),
		ctx.ClientBuilder.ClientOrDie("nodeipam-controller"),
		ctx.AppsClientBuilder.ClientOrDie("nodeipam-controller"),
		clusterCIDRs,
		serviceCIDR,
		secondaryServiceCIDR,
		int(ctx.ComponentConfig.NodeIPAMController.NodeCIDRMaskSize),
		ipam.RangeAllocatorType,
		ctx.ComponentConfig.NodeIPAMController.LegacyIpam,
	)
	if err != nil {
		return nil, true, err
	}
	go ipamController.Run(ctx.Stop)
	return nil, true, nil
}

func startVsagController(ctx ControllerContext) (http.Handler, bool, error) {
	cloudConfig, guestCloudConfig, err := initCloudConfig(ctx.ComponentConfig.VsagController.HostAkCredSecretName, ctx.ComponentConfig.VsagController.CloudConfigFile)
	if err != nil {
		return nil, false, fmt.Errorf("unable to init cloud config %v", err)
	}

	vsagController, err := vsagcontroller.NewVsagController(
		ctx.InformerFactory.Core().V1().Nodes(),
		ctx.YurtInformerFactory.Apps().V1alpha1().NodePools(),
		ctx.ClientBuilder.ClientOrDie("vsag-controller"),
		ctx.AppsClientBuilder.ClientOrDie("vsag-controller"),
		cloudConfig,
		guestCloudConfig,
		ctx.ComponentConfig.VsagController.VsagCoreImage,
		ctx.ComponentConfig.VsagController.VsagSideCarImage,
		ctx.ComponentConfig.VsagController.VsagHelperImage,
		ctx.ComponentConfig.VsagController.ConcurrentVsagWorkers,
	)
	if err != nil {
		return nil, true, err
	}
	go vsagController.Run(ctx.Stop)
	return nil, true, nil
}

// processCIDRs is a helper function that works on a comma separated cidrs and returns
// a list of typed cidrs
// a flag if cidrs represents a dual stack
// error if failed to parse any of the cidrs
func processCIDRs(cidrsList string) ([]*net.IPNet, bool, error) {
	cidrsSplit := strings.Split(strings.TrimSpace(cidrsList), ",")

	cidrs, err := netutils.ParseCIDRs(cidrsSplit)
	if err != nil {
		return nil, false, err
	}

	// if cidrs has an error then the previous call will fail
	// safe to ignore error checking on next call
	dualstack, _ := netutils.IsDualStackCIDRs(cidrs)

	return cidrs, dualstack, nil
}

func initCloudConfig(hostSecretName, guestCloudConfigFile string) (*util.CloudConfig, *util.CCMCloudConfig, error) {
	var cfg util.CloudConfig
	// read cloud config from meta cluster secret
	clusterConfig, err := rest.InClusterConfig()
	if err != nil {
		return nil, nil, err
	}

	clusterClient, err := kubernetes.NewForConfig(clusterConfig)
	if err != nil {
		return nil, nil, err
	}

	hostSecret, err := clusterClient.CoreV1().Secrets(metav1.NamespaceSystem).Get(context.TODO(), hostSecretName, metav1.GetOptions{})
	if err != nil {
		return nil, nil, err
	}

	if hostSecret.Data == nil {
		return nil, nil, fmt.Errorf("host secret %s has no data", hostSecretName)
	}

	cfg.AccessKeyID = string(hostSecret.Data["AccessKeyID"])
	cfg.AccessKeySecret = string(hostSecret.Data["AccessKeySecret"])
	cfg.ResourceUid = string(hostSecret.Data["ResourceUid"])

	if cfg.AccessKeySecret == "" || cfg.AccessKeyID == "" || cfg.ResourceUid == "" {
		return nil, nil, fmt.Errorf("vsag cloud config missing mandatory info")
	}

	guestconfig, err := os.Open(guestCloudConfigFile)
	if err != nil {
		return nil, nil, err
	}

	defer guestconfig.Close()
	//var guestCfg ccm.CloudConfig
	var guestCfg util.CCMCloudConfig
	if err := json.NewDecoder(guestconfig).Decode(&guestCfg); err != nil {
		return nil, nil, err
	}

	if guestCfg.Global.VpcID == "" || guestCfg.Global.UID == "" || guestCfg.Global.Region == "" ||
		guestCfg.Global.AccessKeyID == "" || guestCfg.Global.AccessKeySecret == "" {
		return nil, nil, fmt.Errorf("guest cloud config missing mandatory info")
	}

	// decode ak/sk
	key, err := base64.StdEncoding.DecodeString(guestCfg.Global.AccessKeyID)
	if err != nil {
		return nil, nil, err
	}
	guestCfg.Global.AccessKeyID = string(key)
	secret, err := base64.StdEncoding.DecodeString(guestCfg.Global.AccessKeySecret)
	if err != nil {
		return nil, nil, err
	}
	guestCfg.Global.AccessKeySecret = string(secret)

	return &cfg, &guestCfg, nil
}
