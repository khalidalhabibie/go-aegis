package transfers

import (
	"errors"
	"fmt"
)

var ErrInvalidTransferState = errors.New("invalid transfer state")
var ErrInvalidTransferStatus = errors.New("invalid transfer status")
var ErrInvalidTransactionAttemptStatus = errors.New("invalid transaction attempt status")
var ErrTransactionAttemptConflict = errors.New("transaction attempt conflict")
var ErrSourceWalletNotFound = errors.New("source wallet not found")
var ErrTransient = errors.New("transient error")
var ErrTransactionAttemptNotFound = errors.New("transaction attempt not found")

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

type AttemptConflictError struct {
	AttemptID string
	Expected  string
	Actual    string
}

func (e AttemptConflictError) Error() string {
	return fmt.Sprintf("transaction attempt %s expected status %s but got %s", e.AttemptID, e.Expected, e.Actual)
}

func (e AttemptConflictError) Unwrap() error {
	return ErrTransactionAttemptConflict
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
