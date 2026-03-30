package webhooks

import (
	"context"
	"errors"
	"time"
)

var ErrDeliveryLeaseLost = errors.New("webhook delivery lease lost")

type Repository interface {
	ScheduleTransferStatusDeliveries(ctx context.Context, maxAttempts int) (int64, error)
	ClaimDueDeliveries(ctx context.Context, limit int, leaseDuration time.Duration) ([]Delivery, error)
	MarkDelivered(ctx context.Context, params MarkDeliveredParams) error
	MarkRetry(ctx context.Context, params MarkRetryParams) error
	MarkFailed(ctx context.Context, params MarkFailedParams) error
}

type MarkDeliveredParams struct {
	ID                 string
	LeaseExpiresAt     time.Time
	AttemptCount       int
	ResponseStatusCode int
	ResponseBody       string
}

type MarkRetryParams struct {
	ID                 string
	LeaseExpiresAt     time.Time
	AttemptCount       int
	ResponseStatusCode int
	ResponseBody       string
	LastError          string
	NextAttemptAt      time.Time
}

type MarkFailedParams struct {
	ID                 string
	LeaseExpiresAt     time.Time
	AttemptCount       int
	ResponseStatusCode int
	ResponseBody       string
	LastError          string
}
