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
	"net"
	"strconv"
	"time"

	listers "k8s.io/client-go/listers/core/v1"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/cmd/yurthub/app/options"
)

const (
	maxDuration = 60 * time.Second
)

type NetworkManager struct {
	ifController         DummyInterfaceController
	iptablesManager      *IptablesManager
	dummyIfIP            net.IP
	dummyIfName          string
	enableDummyIf        bool
	enableForwardTraffic bool
}

func NewNetworkManager(options *options.YurtHubOptions, serviceLister listers.ServiceLister) (*NetworkManager, error) {
	if !options.EnableForwardTraffic && !options.EnableDummyIf {
		klog.Infof("both traffic forwarding and dummy interface are not enabled, skip network manager")
		return nil, nil
	}
	m := &NetworkManager{
		dummyIfIP:            net.ParseIP(options.HubAgentDummyIfIP),
		dummyIfName:          options.HubAgentDummyIfName,
		enableDummyIf:        options.EnableDummyIf,
		enableForwardTraffic: options.EnableForwardTraffic,
	}

	if m.enableDummyIf {
		m.ifController = NewDummyInterfaceController()
		err := m.ifController.EnsureDummyInterface(m.dummyIfName, m.dummyIfIP)
		if err != nil {
			klog.Errorf("couldn't ensure dummy interface, %v", err)
			return nil, err
		}
		klog.V(2).Infof("create dummy network interface %s(%s)", m.dummyIfName, m.dummyIfIP.String())
	}

	if m.enableForwardTraffic {
		secureServerHost := options.YurtHubProxyHost
		if options.EnableDummyIf {
			secureServerHost = options.HubAgentDummyIfIP
		}
		m.iptablesManager = NewIptablesManager(secureServerHost, strconv.Itoa(options.YurtHubProxySecurePort), serviceLister)
	}

	return m, nil
}

func (m *NetworkManager) Run(stopCh <-chan struct{}) {
	go func() {
		tickerDuration := 1 * time.Second
		ticker := time.NewTicker(tickerDuration)
		defer ticker.Stop()

		for {
			select {
			case <-stopCh:
				if m.enableDummyIf {
					klog.Infof("exit network manager run goroutine normally, dummy interface(%s) are left", m.dummyIfName)
				}

				if m.enableForwardTraffic {
					klog.Infof("exit network manager run goroutine normally, iptables are left")
				}
				return
			case <-ticker.C:
				if m.enableDummyIf {
					err := m.ifController.EnsureDummyInterface(m.dummyIfName, m.dummyIfIP)
					if err != nil {
						klog.Warningf("couldn't ensure dummy interface, %v", err)
					}
				}

				if m.enableForwardTraffic {
					err := m.iptablesManager.EnsureIptablesRules()
					if err != nil {
						klog.Warningf("couldn't ensure iptables for default/kubernetes service, %v", err)
					}
				}

				if tickerDuration < maxDuration {
					tickerDuration = tickerDuration * 2
					if tickerDuration > maxDuration {
						tickerDuration = maxDuration
					}

					ticker.Stop()
					ticker = time.NewTicker(tickerDuration)
				}
			}
		}
	}()
}
