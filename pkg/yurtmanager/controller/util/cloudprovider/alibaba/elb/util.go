package elb

import (
	"fmt"
	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/errors"
)

func SDKError(api string, err error) error {
	if err == nil {
		return err
	}
	switch err := err.(type) {
	case *errors.ServerError:
		return fmt.Errorf("[SDKError] API: %s, ErrorCode: %s, RequestId: %s, Message: %s",
			api, err.ErrorCode(), err.RequestId(), err.Message())
	default:
		return fmt.Errorf("[SDKError] API: %s, Message: %s", api, err.Error())
	}
}
