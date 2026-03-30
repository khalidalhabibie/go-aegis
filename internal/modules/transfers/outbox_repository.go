package transfers

import (
	"context"
	"time"
)

type OutboxRepository interface {
	ClaimPendingOutbox(ctx context.Context, batchSize int, staleBefore time.Time) ([]OutboxEvent, error)
	MarkOutboxDispatched(ctx context.Context, outboxID string, attemptCount int) error
	MarkOutboxRetry(ctx context.Context, outboxID string, attemptCount int, nextAvailableAt time.Time, lastError string) error
}
