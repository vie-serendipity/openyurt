package base

import (
	"fmt"
	"strings"
	"time"

	sts2 "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/raven/util/provider/sts"
)

const AssumeRoleName = "AliyunCSManagedEdgeRole"

type DefaultToken struct {
	Region          string
	AccessKeyId     string
	AccessKeySecret string
	SecurityToken   string
}

// TokenAuth is an interface of Token auth method
type TokenAuth interface {
	NextToken() (*DefaultToken, error)
}

type MetaToken struct {
	Region          string
	UID             string
	AccessKeyID     string
	AccessKeySecret string
}

func (m *MetaToken) NextToken() (*DefaultToken, error) {
	stsClient, err := sts2.NewClientWithAccessKey(m.Region, m.AccessKeyID, m.AccessKeySecret)
	if err != nil {
		return nil, fmt.Errorf("alibaba cloud: create sts client error: %s", err.Error())
	}
	req := sts2.CreateAssumeRoleWithServiceIdentityRequest()
	req.SetScheme("https")
	req.DurationSeconds = "7200"
	req.AssumeRoleFor = m.UID
	req.RoleArn = strings.ToLower(fmt.Sprintf("acs:ram::%s:role/%s", m.UID, AssumeRoleName))
	req.RoleSessionName = fmt.Sprintf("%s-provision-role-%d", "ecm", time.Now().Unix())
	resp, err := stsClient.AssumeRoleWithServiceIdentity(req)
	if err != nil {
		return nil, fmt.Errorf("alicloud: AssumeRole error: %s", err.Error())
	}
	token := &DefaultToken{
		Region:          m.Region,
		SecurityToken:   resp.Credentials.SecurityToken,
		AccessKeyId:     resp.Credentials.AccessKeyId,
		AccessKeySecret: resp.Credentials.AccessKeySecret,
	}
	return token, nil
}
