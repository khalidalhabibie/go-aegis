package transfers

import (
	"encoding/json"
	"time"
)

const StatusCreated = "CREATED"
const StatusValidated = "VALIDATED"
const StatusQueued = "QUEUED"
const StatusSigning = "SIGNING"
const StatusSubmitted = "SUBMITTED"
const StatusPendingOnChain = "PENDING_ON_CHAIN"
const StatusConfirmed = "CONFIRMED"
const StatusFailed = "FAILED"

type Transfer struct {
	ID                 string          `json:"id"`
	IdempotencyKey     string          `json:"idempotency_key"`
	Chain              string          `json:"chain"`
	AssetType          string          `json:"asset_type"`
	SourceWalletID     string          `json:"source_wallet_id"`
	DestinationAddress string          `json:"destination_address"`
	Amount             string          `json:"amount"`
	CallbackURL        string          `json:"callback_url"`
	MetadataJSON       json.RawMessage `json:"metadata_json"`
	TxHash             string          `json:"tx_hash"`
	Status             string          `json:"status"`
	CreatedAt          time.Time       `json:"created_at"`
	UpdatedAt          time.Time       `json:"updated_at"`
}

type StatusHistory struct {
	ID         string    `json:"id"`
	TransferID string    `json:"transfer_id"`
	FromStatus *string   `json:"from_status,omitempty"`
	ToStatus   string    `json:"to_status"`
	CreatedAt  time.Time `json:"created_at"`
}
