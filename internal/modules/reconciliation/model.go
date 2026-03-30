package reconciliation

import "time"

type ReceiptStatus string

const (
	ReceiptStatusPending   ReceiptStatus = "PENDING"
	ReceiptStatusConfirmed ReceiptStatus = "CONFIRMED"
	ReceiptStatusFailed    ReceiptStatus = "FAILED"
	ReceiptStatusNotFound  ReceiptStatus = "NOT_FOUND"
)

type CandidateTransfer struct {
	TransferID     string
	Chain          string
	TxHash         string
	InternalStatus string
}

type Result struct {
	ID                string        `json:"id"`
	TransferRequestID string        `json:"transfer_request_id"`
	TxHash            string        `json:"tx_hash"`
	InternalStatus    string        `json:"internal_status"`
	BlockchainStatus  ReceiptStatus `json:"blockchain_status"`
	IsMismatch        bool          `json:"is_mismatch"`
	Notes             string        `json:"notes"`
	CheckedAt         time.Time     `json:"checked_at"`
	TriggerSource     string        `json:"trigger_source"`
}

type RunResult struct {
	CheckedCount  int `json:"checked_count"`
	MismatchCount int `json:"mismatch_count"`
	MatchedCount  int `json:"matched_count"`
}
