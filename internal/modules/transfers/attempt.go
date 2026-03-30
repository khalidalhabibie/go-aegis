package transfers

import (
	"encoding/json"
	"time"
)

const AttemptStatusSigned = "SIGNED"
const AttemptStatusBroadcasting = "BROADCASTING"
const AttemptStatusBroadcasted = "BROADCASTED"
const AttemptStatusFailed = "FAILED"

type TransactionAttempt struct {
	ID           string          `json:"id"`
	TransferID   string          `json:"transfer_id"`
	Nonce        *int64          `json:"nonce,omitempty"`
	RawPayload   json.RawMessage `json:"raw_payload"`
	TxHash       string          `json:"tx_hash"`
	Status       string          `json:"status"`
	ErrorMessage string          `json:"error_message"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

type TransactionAttemptPayload struct {
	Encoding       string `json:"encoding"`
	RawTransaction string `json:"raw_transaction"`
}

func newTransactionAttemptPayload(rawTransaction string) (json.RawMessage, error) {
	payload, err := json.Marshal(TransactionAttemptPayload{
		Encoding:       "raw_transaction",
		RawTransaction: rawTransaction,
	})
	if err != nil {
		return nil, err
	}

	return payload, nil
}

func (a TransactionAttempt) Payload() (TransactionAttemptPayload, error) {
	var payload TransactionAttemptPayload
	if err := json.Unmarshal(a.RawPayload, &payload); err != nil {
		return TransactionAttemptPayload{}, err
	}

	return payload, nil
}
