package transfers

import (
	"context"
	"encoding/json"
	"errors"
)

var ErrTransferNotFound = errors.New("transfer not found")

type Repository interface {
	Create(ctx context.Context, params CreateParams) (Transfer, bool, error)
	GetByID(ctx context.Context, id string) (Transfer, error)
	List(ctx context.Context, params ListParams) ([]Transfer, error)
	TransitionStatus(ctx context.Context, params TransitionParams) (Transfer, error)
	GetLatestAttempt(ctx context.Context, transferID string) (TransactionAttempt, error)
	CreateAttempt(ctx context.Context, params CreateAttemptParams) (TransactionAttempt, error)
	UpdateAttempt(ctx context.Context, params UpdateAttemptParams) (TransactionAttempt, error)
}

type CreateParams struct {
	IdempotencyKey     string
	Chain              string
	AssetType          string
	SourceWalletID     string
	DestinationAddress string
	Amount             string
	CallbackURL        string
	MetadataJSON       json.RawMessage
	Status             string
}

type ListParams struct {
	Limit  int
	Offset int
}

type TransitionParams struct {
	ID         string
	FromStatus string
	ToStatus   string
	TxHash     *string
}

type CreateAttemptParams struct {
	TransferID   string
	Nonce        *int64
	RawPayload   json.RawMessage
	TxHash       string
	Status       string
	ErrorMessage string
}

type UpdateAttemptParams struct {
	AttemptID    string
	Status       string
	ErrorMessage string
}
