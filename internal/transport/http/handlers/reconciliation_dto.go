package handlers

import (
	"time"

	"aegis/internal/modules/reconciliation"
)

type reconciliationRunResponse struct {
	Data reconciliationRunData `json:"data"`
}

type reconciliationRunData struct {
	CheckedCount  int    `json:"checked_count"`
	MismatchCount int    `json:"mismatch_count"`
	MatchedCount  int    `json:"matched_count"`
	TriggerSource string `json:"trigger_source"`
}

type reconciliationResultResponse struct {
	ID                string    `json:"id"`
	TransferRequestID string    `json:"transfer_request_id"`
	TxHash            string    `json:"tx_hash"`
	InternalStatus    string    `json:"internal_status"`
	BlockchainStatus  string    `json:"blockchain_status"`
	IsMismatch        bool      `json:"is_mismatch"`
	Notes             string    `json:"notes"`
	CheckedAt         time.Time `json:"checked_at"`
	TriggerSource     string    `json:"trigger_source"`
}

type reconciliationMismatchListEnvelope struct {
	Data []reconciliationResultResponse `json:"data"`
}

func newReconciliationResultResponse(result reconciliation.Result) reconciliationResultResponse {
	return reconciliationResultResponse{
		ID:                result.ID,
		TransferRequestID: result.TransferRequestID,
		TxHash:            result.TxHash,
		InternalStatus:    result.InternalStatus,
		BlockchainStatus:  string(result.BlockchainStatus),
		IsMismatch:        result.IsMismatch,
		Notes:             result.Notes,
		CheckedAt:         result.CheckedAt,
		TriggerSource:     result.TriggerSource,
	}
}
