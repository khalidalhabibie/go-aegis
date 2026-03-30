package transfers

import (
	"errors"
	"fmt"
)

var ErrInvalidTransferState = errors.New("invalid transfer state")
var ErrTransient = errors.New("transient error")

type InvalidStateError struct {
	TransferID string
	Expected   string
	Actual     string
}

func (e InvalidStateError) Error() string {
	return fmt.Sprintf("transfer %s expected status %s but got %s", e.TransferID, e.Expected, e.Actual)
}

func (e InvalidStateError) Unwrap() error {
	return ErrInvalidTransferState
}

type TransientError struct {
	Operation string
	Err       error
}

func (e TransientError) Error() string {
	if e.Operation == "" {
		return fmt.Sprintf("transient error: %v", e.Err)
	}

	return fmt.Sprintf("%s: %v", e.Operation, e.Err)
}

func (e TransientError) Unwrap() error {
	return ErrTransient
}
