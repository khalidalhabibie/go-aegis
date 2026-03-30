package reconciliation

import "context"

type Repository interface {
	ListCandidates(ctx context.Context) ([]CandidateTransfer, error)
	CreateResult(ctx context.Context, params CreateResultParams) (Result, error)
	ListLatestMismatches(ctx context.Context) ([]Result, error)
}

type CreateResultParams struct {
	TransferRequestID string
	TxHash            string
	InternalStatus    string
	BlockchainStatus  ReceiptStatus
	IsMismatch        bool
	Notes             string
	TriggerSource     string
}
