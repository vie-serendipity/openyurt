/*
Copyright 2021 The OpenYurt Authors.

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

package network

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	listers "k8s.io/client-go/listers/core/v1"
	"k8s.io/klog/v2"
	"k8s.io/utils/exec"
	utilnet "k8s.io/utils/net"

	"github.com/openyurtio/openyurt/pkg/util/iptables"
)

const (
	KubeSvcNamespace = "default"
	KubeSvcName      = "kubernetes"
	KubeSvcPortName  = "https"
)

type iptablesRule struct {
	pos   iptables.RulePosition
	table iptables.Table
	chain iptables.Chain
	args  []string
}

type IptablesManager struct {
	kubeSvcClusterIP    string
	kubeSvcPort         string
	hubSecureServerAddr string
	serviceLister       listers.ServiceLister
	iptables            iptables.Interface
	rules               []iptablesRule
}

func NewIptablesManager(secureServerIP, secureServerPort string, serviceLister listers.ServiceLister) *IptablesManager {
	protocol := iptables.ProtocolIpv4
	if utilnet.IsIPv6String(secureServerIP) {
		protocol = iptables.ProtocolIpv6
	}
	execer := exec.New()
	iptInterface := iptables.New(execer, protocol)

	im := &IptablesManager{
		iptables:            iptInterface,
		rules:               []iptablesRule{},
		serviceLister:       serviceLister,
		hubSecureServerAddr: net.JoinHostPort(secureServerIP, secureServerPort),
	}

	return im
}

func makeupIptablesRules(kubeSvcClusterIP, kubeSvcPort, hubSecureServerAddr string) []iptablesRule {
	return []iptablesRule{
		// dnat kubernetes.default svc traffic to 169.254.2.1:10268 in output chain
		{iptables.Prepend, iptables.TableNAT, iptables.ChainOutput, []string{"-p", "tcp", "-d", kubeSvcClusterIP, "--dport", kubeSvcPort, "-j", "DNAT", "--to-destination", hubSecureServerAddr}},
		// dnat kubernetes.default svc traffic to 169.254.2.1:10268 in prerouting chain
		{iptables.Prepend, iptables.TableNAT, iptables.ChainPrerouting, []string{"-p", "tcp", "-d", kubeSvcClusterIP, "--dport", kubeSvcPort, "-j", "DNAT", "--to-destination", hubSecureServerAddr}},
	}
}

func (im *IptablesManager) EnsureIptablesRules() error {
	// at first, prepare kubeSvcClusterIP and kubeSvcPort
	if len(im.kubeSvcClusterIP) == 0 || len(im.kubeSvcPort) == 0 {
		if err := im.ResolveKubeSvc(); err != nil {
			return err
		}
	}

	// kubeSvcClusterIP and kubeSvcPort is ready, then prepare iptables rules
	if len(im.rules) == 0 {
		im.rules = append(im.rules, makeupIptablesRules(im.kubeSvcClusterIP, im.kubeSvcPort, im.hubSecureServerAddr)...)
	}

	var errs []error
	for _, rule := range im.rules {
		_, err := im.iptables.EnsureRule(rule.pos, rule.table, rule.chain, rule.args...)
		if err != nil {
			errs = append(errs, err)
			klog.Errorf("could not ensure iptables rule(%s -t %s %s %s), %v", rule.pos, rule.table, rule.chain, strings.Join(rule.args, ","), err)
			continue
		}
	}
	return utilerrors.NewAggregate(errs)
}

func (im *IptablesManager) ResolveKubeSvc() error {
	svc, err := im.serviceLister.Services(KubeSvcNamespace).Get(KubeSvcName)
	if err != nil {
		return err
	} else if svc == nil {
		return fmt.Errorf("service %s/%s is not ready, retry later", KubeSvcNamespace, KubeSvcName)
	} else {
		im.kubeSvcClusterIP = svc.Spec.ClusterIP
		for i := range svc.Spec.Ports {
			if svc.Spec.Ports[i].Name == KubeSvcPortName {
				im.kubeSvcPort = strconv.FormatInt(int64(svc.Spec.Ports[i].Port), 10)
				break
			}
		}
		klog.Infof("service %s/%s with clusterIP:Port: %s:%s", KubeSvcNamespace, KubeSvcName, im.kubeSvcClusterIP, im.kubeSvcPort)
	}

	if len(im.kubeSvcClusterIP) == 0 || len(im.kubeSvcPort) == 0 {
		return fmt.Errorf("service %s/%s has no clusterIP or Port, Iptables settings will be skipped", KubeSvcNamespace, KubeSvcName)
	}
	return nil
}

func (im *IptablesManager) CleanUpIptablesRules() error {
	var errs []error
	for _, rule := range im.rules {
		err := im.iptables.DeleteRule(rule.table, rule.chain, rule.args...)
		if err != nil {
			errs = append(errs, err)
			klog.Errorf("could not delete iptables rule(%s -t %s %s %s), %v", rule.pos, rule.table, rule.chain, strings.Join(rule.args, " "), err)
		}
	}
	return utilerrors.NewAggregate(errs)
}
