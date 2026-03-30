package webhooks

import (
	"context"
	"time"
)

type Repository interface {
	ScheduleTransferStatusDeliveries(ctx context.Context, maxAttempts int) (int64, error)
	ListDueDeliveries(ctx context.Context, limit int) ([]Delivery, error)
	MarkDelivered(ctx context.Context, params MarkDeliveredParams) error
	MarkRetry(ctx context.Context, params MarkRetryParams) error
	MarkFailed(ctx context.Context, params MarkFailedParams) error
}

type MarkDeliveredParams struct {
	ID                 string
	AttemptCount       int
	ResponseStatusCode int
	ResponseBody       string
}

type MarkRetryParams struct {
	ID                 string
	AttemptCount       int
	ResponseStatusCode int
	ResponseBody       string
	LastError          string
	NextAttemptAt      time.Time
}

type MarkFailedParams struct {
	ID                 string
	AttemptCount       int
	ResponseStatusCode int
	ResponseBody       string
	LastError          string
}
