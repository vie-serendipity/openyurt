package elb

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func Test_ValidatePairs(t *testing.T) {
	cases := []struct {
		input  string
		expect bool
	}{
		{
			"k8s-svc=svc1",
			true,
		},
		{
			"k8s-svc=svc-1",
			true,
		},
		{
			"key1=val1,key2=val2",
			true,
		},
		{
			"k8s.svc=s1,key2=val2",
			true,
		},
		{
			"k8s+svc=s1,key2=val+2,",
			false,
		},
		{
			"k8s+svc=s1,key2=val+2,djasoida",
			false,
		},
	}

	for _, tt := range cases {
		actual := validateLabels(tt.input)
		if !assert.Equal(t, tt.expect, actual) {
			t.Errorf("case input %s, expect %t, actual %t", tt.input, tt.expect, actual)
		}
	}

}
