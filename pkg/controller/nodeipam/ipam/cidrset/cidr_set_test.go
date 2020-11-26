/*
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

package cidrset

import (
	"math/big"
	"net"
	"reflect"
	"sort"
	"testing"

	"github.com/cilium/cilium/pkg/ip"
	"k8s.io/klog/v2"
)

func TestCIDRSetFullyAllocated(t *testing.T) {
	cases := []struct {
		clusterCIDRStr string
		subNetMaskSize int
		expectedCIDR   string
		description    string
	}{
		{
			clusterCIDRStr: "127.123.234.0/30",
			subNetMaskSize: 30,
			expectedCIDR:   "127.123.234.0/30",
			description:    "Fully allocated CIDR with IPv4",
		},
		{
			clusterCIDRStr: "beef:1234::/30",
			subNetMaskSize: 30,
			expectedCIDR:   "beef:1234::/30",
			description:    "Fully allocated CIDR with IPv6",
		},
	}
	for _, tc := range cases {
		_, clusterCIDR, _ := net.ParseCIDR(tc.clusterCIDRStr)
		a, err := NewCIDRSet(clusterCIDR, tc.subNetMaskSize)
		if err != nil {
			t.Fatalf("unexpected error: %v for %v", err, tc.description)
		}
		p, err := a.AllocateNext()
		if err != nil {
			t.Fatalf("unexpected error: %v for %v", err, tc.description)
		}
		if p.String() != tc.expectedCIDR {
			t.Fatalf("unexpected allocated cidr: %v, expecting %v for %v",
				p.String(), tc.expectedCIDR, tc.description)
		}

		_, err = a.AllocateNext()
		if err == nil {
			t.Fatalf("expected error because of fully-allocated range for %v", tc.description)
		}

		a.Release(p)

		p, err = a.AllocateNext()
		if err != nil {
			t.Fatalf("unexpected error: %v for %v", err, tc.description)
		}
		if p.String() != tc.expectedCIDR {
			t.Fatalf("unexpected allocated cidr: %v, expecting %v for %v",
				p.String(), tc.expectedCIDR, tc.description)
		}
		_, err = a.AllocateNext()
		if err == nil {
			t.Fatalf("expected error because of fully-allocated range for %v", tc.description)
		}
	}
}

func TestIndexToCIDRBlock(t *testing.T) {
	cases := []struct {
		clusterCIDRStr string
		subnetMaskSize int
		index          int
		CIDRBlock      string
		description    string
	}{
		{
			clusterCIDRStr: "127.123.3.0/16",
			subnetMaskSize: 24,
			index:          0,
			CIDRBlock:      "127.123.0.0/24",
			description:    "1st IP address indexed with IPv4",
		},
		{
			clusterCIDRStr: "127.123.0.0/16",
			subnetMaskSize: 24,
			index:          15,
			CIDRBlock:      "127.123.15.0/24",
			description:    "16th IP address indexed with IPv4",
		},
		{
			clusterCIDRStr: "192.168.5.219/28",
			subnetMaskSize: 32,
			index:          5,
			CIDRBlock:      "192.168.5.213/32",
			description:    "5th IP address indexed with IPv4",
		},
		{
			clusterCIDRStr: "2001:0db8:1234:3::/48",
			subnetMaskSize: 64,
			index:          0,
			CIDRBlock:      "2001:db8:1234::/64",
			description:    "1st IP address indexed with IPv6 /64",
		},
		{
			clusterCIDRStr: "2001:0db8:1234::/48",
			subnetMaskSize: 64,
			index:          15,
			CIDRBlock:      "2001:db8:1234:f::/64",
			description:    "16th IP address indexed with IPv6 /64",
		},
		{
			clusterCIDRStr: "2001:0db8:85a3::8a2e:0370:7334/50",
			subnetMaskSize: 63,
			index:          6425,
			CIDRBlock:      "2001:db8:85a3:3232::/63",
			description:    "6426th IP address indexed with IPv6 /63",
		},
		{
			clusterCIDRStr: "2001:0db8::/32",
			subnetMaskSize: 48,
			index:          0,
			CIDRBlock:      "2001:db8::/48",
			description:    "1st IP address indexed with IPv6 /48",
		},
		{
			clusterCIDRStr: "2001:0db8::/32",
			subnetMaskSize: 48,
			index:          15,
			CIDRBlock:      "2001:db8:f::/48",
			description:    "16th IP address indexed with IPv6 /48",
		},
		{
			clusterCIDRStr: "2001:0db8:85a3::8a2e:0370:7334/32",
			subnetMaskSize: 48,
			index:          6425,
			CIDRBlock:      "2001:db8:1919::/48",
			description:    "6426th IP address indexed with IPv6 /48",
		},
		{
			clusterCIDRStr: "2001:0db8:1234:ff00::/56",
			subnetMaskSize: 72,
			index:          0,
			CIDRBlock:      "2001:db8:1234:ff00::/72",
			description:    "1st IP address indexed with IPv6 /72",
		},
		{
			clusterCIDRStr: "2001:0db8:1234:ff00::/56",
			subnetMaskSize: 72,
			index:          15,
			CIDRBlock:      "2001:db8:1234:ff00:f00::/72",
			description:    "16th IP address indexed with IPv6 /72",
		},
		{
			clusterCIDRStr: "2001:0db8:1234:ff00::0370:7334/56",
			subnetMaskSize: 72,
			index:          6425,
			CIDRBlock:      "2001:db8:1234:ff19:1900::/72",
			description:    "6426th IP address indexed with IPv6 /72",
		},
		{
			clusterCIDRStr: "2001:0db8:1234:0:1234::/80",
			subnetMaskSize: 96,
			index:          0,
			CIDRBlock:      "2001:db8:1234:0:1234::/96",
			description:    "1st IP address indexed with IPv6 /96",
		},
		{
			clusterCIDRStr: "2001:0db8:1234:0:1234::/80",
			subnetMaskSize: 96,
			index:          15,
			CIDRBlock:      "2001:db8:1234:0:1234:f::/96",
			description:    "16th IP address indexed with IPv6 /96",
		},
		{
			clusterCIDRStr: "2001:0db8:1234:ff00::0370:7334/80",
			subnetMaskSize: 96,
			index:          6425,
			CIDRBlock:      "2001:db8:1234:ff00:0:1919::/96",
			description:    "6426th IP address indexed with IPv6 /96",
		},
	}
	for _, tc := range cases {
		_, clusterCIDR, _ := net.ParseCIDR(tc.clusterCIDRStr)
		a, err := NewCIDRSet(clusterCIDR, tc.subnetMaskSize)
		if err != nil {
			t.Fatalf("error for %v ", tc.description)
		}
		cidr := a.indexToCIDRBlock(tc.index)
		if cidr.String() != tc.CIDRBlock {
			t.Fatalf("error for %v index %d %s", tc.description, tc.index, cidr.String())
		}
	}
}

func TestCIDRSet_RandomishAllocation(t *testing.T) {
	cases := []struct {
		clusterCIDRStr string
		description    string
	}{
		{
			clusterCIDRStr: "127.123.234.0/16",
			description:    "RandomishAllocation with IPv4",
		},
		{
			clusterCIDRStr: "beef:1234::/16",
			description:    "RandomishAllocation with IPv6",
		},
	}
	for _, tc := range cases {
		_, clusterCIDR, _ := net.ParseCIDR(tc.clusterCIDRStr)
		a, err := NewCIDRSet(clusterCIDR, 24)
		if err != nil {
			t.Fatalf("Error allocating CIDRSet for %v", tc.description)
		}
		// allocate all the CIDRs
		var cidrs []*net.IPNet

		for i := 0; i < 256; i++ {
			if c, err := a.AllocateNext(); err == nil {
				cidrs = append(cidrs, c)
			} else {
				t.Fatalf("unexpected error: %v for %v", err, tc.description)
			}
		}

		//var err error
		_, err = a.AllocateNext()
		if err == nil {
			t.Fatalf("expected error because of fully-allocated range for %v", tc.description)
		}
		// release them all
		for i := 0; i < len(cidrs); i++ {
			a.Release(cidrs[i])
		}

		// allocate the CIDRs again
		var rcidrs []*net.IPNet
		for i := 0; i < 256; i++ {
			if c, err := a.AllocateNext(); err == nil {
				rcidrs = append(rcidrs, c)
			} else {
				t.Fatalf("unexpected error: %d, %v for %v", i, err, tc.description)
			}
		}
		_, err = a.AllocateNext()
		if err == nil {
			t.Fatalf("expected error because of fully-allocated range for %v", tc.description)
		}

		if !reflect.DeepEqual(cidrs, rcidrs) {
			t.Fatalf("expected re-allocated cidrs are the same collection for %v", tc.description)
		}
	}
}

func TestCIDRSet_AllocationOccupied(t *testing.T) {
	cases := []struct {
		clusterCIDRStr string
		description    string
	}{
		{
			clusterCIDRStr: "127.123.234.0/16",
			description:    "AllocationOccupied with IPv4",
		},
		{
			clusterCIDRStr: "beef:1234::/16",
			description:    "AllocationOccupied with IPv6",
		},
	}
	for _, tc := range cases {
		_, clusterCIDR, _ := net.ParseCIDR(tc.clusterCIDRStr)
		a, err := NewCIDRSet(clusterCIDR, 24)
		if err != nil {
			t.Fatalf("Error allocating CIDRSet for %v", tc.description)
		}
		// allocate all the CIDRs
		var cidrs []*net.IPNet
		var numCIDRs = 256

		for i := 0; i < numCIDRs; i++ {
			if c, err := a.AllocateNext(); err == nil {
				cidrs = append(cidrs, c)
			} else {
				t.Fatalf("unexpected error: %v for %v", err, tc.description)
			}
		}

		//var err error
		_, err = a.AllocateNext()
		if err == nil {
			t.Fatalf("expected error because of fully-allocated range for %v", tc.description)
		}
		// release them all
		for i := 0; i < len(cidrs); i++ {
			a.Release(cidrs[i])
		}
		// occupy the last 128 CIDRs
		for i := numCIDRs / 2; i < numCIDRs; i++ {
			a.Occupy(cidrs[i])
		}

		// allocate the first 128 CIDRs again
		var rcidrs []*net.IPNet
		for i := 0; i < numCIDRs/2; i++ {
			if c, err := a.AllocateNext(); err == nil {
				rcidrs = append(rcidrs, c)
			} else {
				t.Fatalf("unexpected error: %d, %v for %v", i, err, tc.description)
			}
		}
		_, err = a.AllocateNext()
		if err == nil {
			t.Fatalf("expected error because of fully-allocated range for %v", tc.description)
		}

		// check Occupy() work properly
		for i := numCIDRs / 2; i < numCIDRs; i++ {
			rcidrs = append(rcidrs, cidrs[i])
		}
		if !reflect.DeepEqual(cidrs, rcidrs) {
			t.Fatalf("expected re-allocated cidrs are the same collection for %v", tc.description)
		}
	}
}

func TestGetBitforCIDR(t *testing.T) {
	cases := []struct {
		clusterCIDRStr string
		subNetMaskSize int
		subNetCIDRStr  string
		expectedBit    int
		expectErr      bool
		description    string
	}{
		{
			clusterCIDRStr: "127.0.0.0/8",
			subNetMaskSize: 16,
			subNetCIDRStr:  "127.0.0.0/16",
			expectedBit:    0,
			expectErr:      false,
			description:    "Get 0 Bit with IPv4",
		},
		{
			clusterCIDRStr: "be00::/8",
			subNetMaskSize: 16,
			subNetCIDRStr:  "be00::/16",
			expectedBit:    0,
			expectErr:      false,
			description:    "Get 0 Bit with IPv6",
		},
		{
			clusterCIDRStr: "127.0.0.0/8",
			subNetMaskSize: 16,
			subNetCIDRStr:  "127.123.0.0/16",
			expectedBit:    123,
			expectErr:      false,
			description:    "Get 123rd Bit with IPv4",
		},
		{
			clusterCIDRStr: "be00::/8",
			subNetMaskSize: 16,
			subNetCIDRStr:  "beef::/16",
			expectedBit:    0xef,
			expectErr:      false,
			description:    "Get xef Bit with IPv6",
		},
		{
			clusterCIDRStr: "127.0.0.0/8",
			subNetMaskSize: 16,
			subNetCIDRStr:  "127.168.0.0/16",
			expectedBit:    168,
			expectErr:      false,
			description:    "Get 168th Bit with IPv4",
		},
		{
			clusterCIDRStr: "be00::/8",
			subNetMaskSize: 16,
			subNetCIDRStr:  "be68::/16",
			expectedBit:    0x68,
			expectErr:      false,
			description:    "Get x68th Bit with IPv6",
		},
		{
			clusterCIDRStr: "127.0.0.0/8",
			subNetMaskSize: 16,
			subNetCIDRStr:  "127.224.0.0/16",
			expectedBit:    224,
			expectErr:      false,
			description:    "Get 224th Bit with IPv4",
		},
		{
			clusterCIDRStr: "be00::/8",
			subNetMaskSize: 16,
			subNetCIDRStr:  "be24::/16",
			expectedBit:    0x24,
			expectErr:      false,
			description:    "Get x24th Bit with IPv6",
		},
		{
			clusterCIDRStr: "192.168.0.0/16",
			subNetMaskSize: 24,
			subNetCIDRStr:  "192.168.12.0/24",
			expectedBit:    12,
			expectErr:      false,
			description:    "Get 12th Bit with IPv4",
		},
		{
			clusterCIDRStr: "beef::/16",
			subNetMaskSize: 24,
			subNetCIDRStr:  "beef:1200::/24",
			expectedBit:    0x12,
			expectErr:      false,
			description:    "Get x12th Bit with IPv6",
		},
		{
			clusterCIDRStr: "192.168.0.0/16",
			subNetMaskSize: 24,
			subNetCIDRStr:  "192.168.151.0/24",
			expectedBit:    151,
			expectErr:      false,
			description:    "Get 151st Bit with IPv4",
		},
		{
			clusterCIDRStr: "beef::/16",
			subNetMaskSize: 24,
			subNetCIDRStr:  "beef:9700::/24",
			expectedBit:    0x97,
			expectErr:      false,
			description:    "Get x97st Bit with IPv6",
		},
		{
			clusterCIDRStr: "192.168.0.0/16",
			subNetMaskSize: 24,
			subNetCIDRStr:  "127.168.224.0/24",
			expectErr:      true,
			description:    "Get error with IPv4",
		},
		{
			clusterCIDRStr: "beef::/16",
			subNetMaskSize: 24,
			subNetCIDRStr:  "2001:db00::/24",
			expectErr:      true,
			description:    "Get error with IPv6",
		},
	}

	for _, tc := range cases {
		_, clusterCIDR, err := net.ParseCIDR(tc.clusterCIDRStr)
		if err != nil {
			t.Fatalf("unexpected error: %v for %v", err, tc.description)
		}

		cs, err := NewCIDRSet(clusterCIDR, tc.subNetMaskSize)
		if err != nil {
			t.Fatalf("Error allocating CIDRSet for %v", tc.description)
		}
		_, subnetCIDR, err := net.ParseCIDR(tc.subNetCIDRStr)
		if err != nil {
			t.Fatalf("unexpected error: %v for %v", err, tc.description)
		}

		got, err := cs.getIndexForCIDR(subnetCIDR)
		if err == nil && tc.expectErr {
			klog.Errorf("expected error but got null for %v", tc.description)
			continue
		}

		if err != nil && !tc.expectErr {
			klog.Errorf("unexpected error: %v for %v", err, tc.description)
			continue
		}

		if got != tc.expectedBit {
			klog.Errorf("expected %v, but got %v for %v", tc.expectedBit, got, tc.description)
		}
	}
}

func TestOccupy(t *testing.T) {
	cases := []struct {
		clusterCIDRStr    string
		subNetMaskSize    int
		subNetCIDRStr     string
		expectedUsedBegin int
		expectedUsedEnd   int
		expectErr         bool
		description       string
	}{
		{
			clusterCIDRStr:    "172.20.16.0/20",
			subNetMaskSize:    24,
			subNetCIDRStr:     "172.20.16.0/24",
			expectedUsedBegin: 0,
			expectedUsedEnd:   0,
			expectErr:         false,
			description:       "Occupy all Bits with IPv4",
		},
		{
			clusterCIDRStr:    "2001:beef:1200::/40",
			subNetMaskSize:    48,
			subNetCIDRStr:     "2001:beef:1200::/40",
			expectedUsedBegin: 0,
			expectedUsedEnd:   255,
			expectErr:         false,
			description:       "Occupy all Bits with IPv6",
		},
		{
			clusterCIDRStr:    "127.0.0.0/8",
			subNetMaskSize:    16,
			subNetCIDRStr:     "127.0.0.0/2",
			expectedUsedBegin: 0,
			expectedUsedEnd:   255,
			expectErr:         false,
			description:       "Occupy every Bit with IPv4",
		},
		{
			clusterCIDRStr:    "2001:beef:1200::/40",
			subNetMaskSize:    48,
			subNetCIDRStr:     "2001:beef:1234::/34",
			expectedUsedBegin: 0,
			expectedUsedEnd:   255,
			expectErr:         false,
			description:       "Occupy every Bit with IPv6",
		},
		{
			clusterCIDRStr:    "127.0.0.0/8",
			subNetMaskSize:    16,
			subNetCIDRStr:     "127.0.0.0/16",
			expectedUsedBegin: 0,
			expectedUsedEnd:   0,
			expectErr:         false,
			description:       "Occupy 1st Bit with IPv4",
		},
		{
			clusterCIDRStr:    "2001:beef:1200::/40",
			subNetMaskSize:    48,
			subNetCIDRStr:     "2001:beef:1200::/48",
			expectedUsedBegin: 0,
			expectedUsedEnd:   0,
			expectErr:         false,
			description:       "Occupy 1st Bit with IPv6",
		},
		{
			clusterCIDRStr:    "127.0.0.0/8",
			subNetMaskSize:    32,
			subNetCIDRStr:     "127.0.0.0/16",
			expectedUsedBegin: 0,
			expectedUsedEnd:   65535,
			expectErr:         false,
			description:       "Occupy 65535 Bits with IPv4",
		},
		{
			clusterCIDRStr:    "2001:beef:1200::/48",
			subNetMaskSize:    64,
			subNetCIDRStr:     "2001:beef:1200::/48",
			expectedUsedBegin: 0,
			expectedUsedEnd:   65535,
			expectErr:         false,
			description:       "Occupy 65535 Bits with IPv6",
		},
		{
			clusterCIDRStr:    "127.0.0.0/7",
			subNetMaskSize:    16,
			subNetCIDRStr:     "127.0.0.0/15",
			expectedUsedBegin: 256,
			expectedUsedEnd:   257,
			expectErr:         false,
			description:       "Occupy 257th Bit with IPv4",
		},
		{
			clusterCIDRStr:    "2001:beef:7f00::/39",
			subNetMaskSize:    48,
			subNetCIDRStr:     "2001:beef:7f00::/47",
			expectedUsedBegin: 256,
			expectedUsedEnd:   257,
			expectErr:         false,
			description:       "Occupy 257th Bit with IPv6",
		},
		{
			clusterCIDRStr:    "127.0.0.0/7",
			subNetMaskSize:    15,
			subNetCIDRStr:     "127.0.0.0/15",
			expectedUsedBegin: 128,
			expectedUsedEnd:   128,
			expectErr:         false,
			description:       "Occupy 128th Bit with IPv4",
		},
		{
			clusterCIDRStr:    "2001:beef:7f00::/39",
			subNetMaskSize:    47,
			subNetCIDRStr:     "2001:beef:7f00::/47",
			expectedUsedBegin: 128,
			expectedUsedEnd:   128,
			expectErr:         false,
			description:       "Occupy 128th Bit with IPv6",
		},
		{
			clusterCIDRStr:    "127.0.0.0/7",
			subNetMaskSize:    18,
			subNetCIDRStr:     "127.0.0.0/15",
			expectedUsedBegin: 1024,
			expectedUsedEnd:   1031,
			expectErr:         false,
			description:       "Occupy 1031st Bit with IPv4",
		},
		{
			clusterCIDRStr:    "2001:beef:7f00::/39",
			subNetMaskSize:    50,
			subNetCIDRStr:     "2001:beef:7f00::/47",
			expectedUsedBegin: 1024,
			expectedUsedEnd:   1031,
			expectErr:         false,
			description:       "Occupy 1031st Bit with IPv6",
		},
	}

	for _, tc := range cases {
		_, clusterCIDR, err := net.ParseCIDR(tc.clusterCIDRStr)
		if err != nil {
			t.Fatalf("unexpected error: %v for %v", err, tc.description)
		}

		cs, err := NewCIDRSet(clusterCIDR, tc.subNetMaskSize)
		if err != nil {
			t.Fatalf("Error allocating CIDRSet for %v", tc.description)
		}

		_, subnetCIDR, err := net.ParseCIDR(tc.subNetCIDRStr)
		if err != nil {
			t.Fatalf("unexpected error: %v for %v", err, tc.description)
		}

		err = cs.Occupy(subnetCIDR)
		if err == nil && tc.expectErr {
			t.Errorf("expected error but got none for %v", tc.description)
			continue
		}
		if err != nil && !tc.expectErr {
			t.Errorf("unexpected error: %v for %v", err, tc.description)
			continue
		}

		expectedUsed := big.Int{}
		for i := tc.expectedUsedBegin; i <= tc.expectedUsedEnd; i++ {
			expectedUsed.SetBit(&expectedUsed, i, 1)
		}
		if expectedUsed.Cmp(&cs.used) != 0 {
			t.Errorf("error for %v", tc.description)
		}
	}
}

func TestCIDRSetv6(t *testing.T) {
	cases := []struct {
		clusterCIDRStr string
		subNetMaskSize int
		expectedCIDR   string
		expectedCIDR2  string
		expectErr      bool
		description    string
	}{
		{
			clusterCIDRStr: "127.0.0.0/8",
			subNetMaskSize: 32,
			expectErr:      false,
			expectedCIDR:   "127.0.0.0/32",
			expectedCIDR2:  "127.0.0.1/32",
			description:    "Max cluster subnet size with IPv4",
		},
		{
			clusterCIDRStr: "beef:1234::/32",
			subNetMaskSize: 49,
			expectErr:      true,
			description:    "Max cluster subnet size with IPv6",
		},
		{
			clusterCIDRStr: "2001:beef:1234:369b::/60",
			subNetMaskSize: 64,
			expectedCIDR:   "2001:beef:1234:3690::/64",
			expectedCIDR2:  "2001:beef:1234:3691::/64",
			expectErr:      false,
			description:    "Allocate a few IPv6",
		},
	}
	for _, tc := range cases {
		_, clusterCIDR, _ := net.ParseCIDR(tc.clusterCIDRStr)
		a, err := NewCIDRSet(clusterCIDR, tc.subNetMaskSize)
		if err != nil {
			if tc.expectErr {
				continue
			}
			t.Fatalf("Error allocating CIDRSet for %v", tc.description)
		}

		p, err := a.AllocateNext()
		if err == nil && tc.expectErr {
			t.Errorf("expected error but got none for %v", tc.description)
			continue
		}
		if err != nil && !tc.expectErr {
			t.Errorf("unexpected error: %v for %v", err, tc.description)
			continue
		}
		if !tc.expectErr {
			if p.String() != tc.expectedCIDR {
				t.Fatalf("unexpected allocated cidr: %s for %v", p.String(), tc.description)
			}
		}
		p2, _ := a.AllocateNext()
		if !tc.expectErr {
			if p2.String() != tc.expectedCIDR2 {
				t.Fatalf("unexpected allocated cidr: %s for %v", p2.String(), tc.description)
			}
		}
	}
}

func TestCoalesceCIDRs(t *testing.T) {
	cases := []struct {
		cidrStrs         []string
		expectedCidrStrs []string
		description      string
	}{
		{
			[]string{"192.0.128.0/24", "192.0.128.1/24"},
			[]string{"192.0.128.0/24"},
			"cidrs contained",
		},
		{
			[]string{"192.0.6.0/23", "192.0.8.0/23", "192.0.4.0/23"},
			[]string{"192.0.4.0/22", "192.0.8.0/23"},
			"cidrs partially merged 1",
		},
		{
			[]string{"192.0.4.0/23", "192.0.6.0/23", "192.0.8.0/23", "192.0.10.0/23", "192.0.12.0/23"},
			[]string{"192.0.4.0/22", "192.0.8.0/22", "192.0.12.0/23"},
			"cidrs partially merged 2",
		},
		{
			[]string{"192.0.4.0/23", "192.0.6.0/23", "192.0.8.0/23", "192.0.10.0/23", "192.0.12.0/23", "192.0.14.0/23"},
			[]string{"192.0.4.0/22", "192.0.8.0/21"},
			"cidrs partially merged 3",
		},
		{
			[]string{"192.0.4.0/23", "192.0.6.0/23", "192.0.8.0/23", "192.0.10.0/23", "192.0.12.0/23", "192.0.14.0/23", "192.0.16.0/23"},
			[]string{"192.0.4.0/22", "192.0.8.0/21", "192.0.16.0/23"},
			"cidrs partially merged 4",
		},
		{
			[]string{"192.0.2.0/23", "192.0.4.0/23", "192.0.6.0/23", "192.0.8.0/23", "192.0.10.0/23", "192.0.12.0/23", "192.0.14.0/23"},
			[]string{"192.0.2.0/23", "192.0.4.0/22", "192.0.8.0/21"},
			"cidrs partially merged 5",
		},
		{
			[]string{"192.0.0.0/23", "192.0.2.0/23", "192.0.4.0/23", "192.0.6.0/23", "192.0.8.0/23", "192.0.10.0/23", "192.0.12.0/23", "192.0.14.0/23"},
			[]string{"192.0.0.0/20"},
			"cidrs all merged",
		},
	}
	for _, tc := range cases {
		cidrs := []*net.IPNet{}
		for _, cidrStr := range tc.cidrStrs {
			cidrs = append(cidrs, createIPNet(t, cidrStr))
		}
		mergedV4CIDRs, _ := ip.CoalesceCIDRs(cidrs)
		m := tostring(mergedV4CIDRs)
		sort.Strings(m)
		sort.Strings(tc.expectedCidrStrs)
		if !reflect.DeepEqual(tc.expectedCidrStrs, m) {
			t.Fatalf("desc: %v, expected: %v, actual: %v", tc.description, tc.expectedCidrStrs, m)
		}
	}
}

func TestAllocateNextN(t *testing.T) {
	cases := []struct {
		clusterCIDRStr  string
		subNetMaskSize  int
		n               int
		allocatedBefore int
		expectedCIDR    []string
		expectError     bool
		description     string
	}{
		{
			clusterCIDRStr:  "127.123.0.0/16",
			subNetMaskSize:  24,
			n:               1,
			allocatedBefore: 0,
			expectedCIDR:    []string{"127.123.0.0/24"},
			expectError:     false,
			description:     "case1",
		},
		{
			clusterCIDRStr:  "127.123.0.0/16",
			subNetMaskSize:  23,
			n:               5,
			allocatedBefore: 0,
			expectedCIDR:    []string{"127.123.0.0/21", "127.123.8.0/23"},
			expectError:     false,
			description:     "case2",
		},
		{
			clusterCIDRStr:  "127.123.0.0/16",
			subNetMaskSize:  23,
			n:               5,
			allocatedBefore: 3,
			expectedCIDR:    []string{"127.123.6.0/23", "127.123.8.0/21"},
			expectError:     false,
			description:     "case3",
		},
		{
			clusterCIDRStr:  "127.123.0.0/24",
			subNetMaskSize:  25,
			n:               2,
			allocatedBefore: 0,
			expectedCIDR:    []string{"127.123.0.0/24"},
			expectError:     false,
			description:     "case4",
		},
		{
			clusterCIDRStr:  "127.123.0.0/24",
			subNetMaskSize:  25,
			n:               3,
			allocatedBefore: 0,
			expectedCIDR:    []string{"127.123.0.0/24"},
			expectError:     true,
			description:     "case5",
		},
	}
	for _, tc := range cases {
		_, clusterCIDR, _ := net.ParseCIDR(tc.clusterCIDRStr)
		a, err := NewCIDRSet(clusterCIDR, tc.subNetMaskSize)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		for i := 0; i < tc.allocatedBefore; i++ {
			a.AllocateNext()
		}
		allocatedCdirs, err := a.AllocateNextN(tc.n)
		if err != nil {
			if tc.expectError {
				continue
			}
			t.Fatalf("unexpected error: %v", err)
		}
		cidrsStr := tostring(allocatedCdirs)
		sort.Strings(tc.expectedCIDR)
		sort.Strings(cidrsStr)
		if !reflect.DeepEqual(tc.expectedCIDR, cidrsStr) {
			t.Fatalf("%v: expected: %v, got: %v", tc.description, tc.expectedCIDR, cidrsStr)
		}
	}
}

func createIPNet(t *testing.T, address string) *net.IPNet {
	_, n, err := net.ParseCIDR(address)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func tostring(cidrs []*net.IPNet) []string {
	ret := []string{}
	for _, cidr := range cidrs {
		ret = append(ret, cidr.String())
	}
	return ret
}
