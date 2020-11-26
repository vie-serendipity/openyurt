package errors

import (
	"fmt"
)

type Errors struct {
	errorList []error
}

func (e *Errors) Add(err error) {
	if e.errorList == nil {
		e.errorList = []error{}
	}
	e.errorList = append(e.errorList, err)
}

func (e *Errors) Err() error {
	if e.errorList == nil || len(e.errorList) == 0 {
		return nil
	}

	msg := ""
	for _, err := range e.errorList {
		if err != nil {
			msg = fmt.Sprintf("%s[%s]", msg, err.Error())
		}
	}
	if msg == "" {
		return nil
	}

	return fmt.Errorf(msg)
}
