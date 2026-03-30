package webhooks

import (
	"encoding/json"
	"time"
)

const DeliveryStatusPending = "PENDING"
const DeliveryStatusRetrying = "RETRYING"
const DeliveryStatusInProgress = "IN_PROGRESS"
const DeliveryStatusDelivered = "DELIVERED"
const DeliveryStatusFailed = "FAILED"

type Delivery struct {
	ID                      string          `json:"id"`
	TransferRequestID       string          `json:"transfer_request_id"`
	TransferStatusHistoryID string          `json:"transfer_status_history_id"`
	TargetURL               string          `json:"target_url"`
	EventType               string          `json:"event_type"`
	TransferStatus          string          `json:"transfer_status"`
	PayloadJSON             json.RawMessage `json:"payload_json"`
	DeliveryStatus          string          `json:"delivery_status"`
	AttemptCount            int             `json:"attempt_count"`
	MaxAttempts             int             `json:"max_attempts"`
	ResponseStatusCode      int             `json:"response_status_code"`
	ResponseBody            string          `json:"response_body"`
	LastError               string          `json:"last_error"`
	NextAttemptAt           *time.Time      `json:"next_attempt_at"`
	LeaseExpiresAt          *time.Time      `json:"lease_expires_at"`
	DeliveredAt             *time.Time      `json:"delivered_at"`
	CreatedAt               time.Time       `json:"created_at"`
	UpdatedAt               time.Time       `json:"updated_at"`
}

type DispatchResult struct {
	StatusCode int
	Body       string
}
