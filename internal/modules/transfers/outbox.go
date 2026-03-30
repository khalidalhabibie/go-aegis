package transfers

import (
	"encoding/json"
	"time"
)

const OutboxEventTypeTransferRequested = "transfer.requested"

const OutboxStatusPending = "PENDING"
const OutboxStatusProcessing = "PROCESSING"
const OutboxStatusRetry = "RETRY"
const OutboxStatusDispatched = "DISPATCHED"

type OutboxEvent struct {
	ID           string          `json:"id"`
	TransferID   string          `json:"transfer_id"`
	EventType    string          `json:"event_type"`
	PayloadJSON  json.RawMessage `json:"payload_json"`
	Status       string          `json:"status"`
	AttemptCount int             `json:"attempt_count"`
	AvailableAt  time.Time       `json:"available_at"`
	LockedAt     *time.Time      `json:"locked_at,omitempty"`
	DispatchedAt *time.Time      `json:"dispatched_at,omitempty"`
	LastError    string          `json:"last_error"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
}
