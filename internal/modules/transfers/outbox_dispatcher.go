package transfers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/rs/zerolog"
)

type OutboxDispatcher struct {
	repository        OutboxRepository
	publisher         JobPublisher
	batchSize         int
	pollInterval      time.Duration
	retryDelay        time.Duration
	processingTimeout time.Duration
	log               zerolog.Logger
}

func NewOutboxDispatcher(
	repository OutboxRepository,
	publisher JobPublisher,
	batchSize int,
	pollInterval time.Duration,
	retryDelay time.Duration,
	processingTimeout time.Duration,
	log zerolog.Logger,
) *OutboxDispatcher {
	if batchSize <= 0 {
		batchSize = 25
	}

	if pollInterval <= 0 {
		pollInterval = 2 * time.Second
	}

	if retryDelay <= 0 {
		retryDelay = 2 * time.Second
	}

	if processingTimeout <= 0 {
		processingTimeout = 30 * time.Second
	}

	return &OutboxDispatcher{
		repository:        repository,
		publisher:         publisher,
		batchSize:         batchSize,
		pollInterval:      pollInterval,
		retryDelay:        retryDelay,
		processingTimeout: processingTimeout,
		log:               log,
	}
}

func (d *OutboxDispatcher) Run(ctx context.Context) error {
	ticker := time.NewTicker(d.pollInterval)
	defer ticker.Stop()

	for {
		if err := d.DispatchPending(ctx); err != nil && !errors.Is(err, context.Canceled) {
			d.log.Error().Err(err).Msg("dispatch transfer outbox batch failed")
		}

		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (d *OutboxDispatcher) DispatchPending(ctx context.Context) error {
	staleBefore := time.Now().UTC().Add(-d.processingTimeout)
	events, err := d.repository.ClaimPendingOutbox(ctx, d.batchSize, staleBefore)
	if err != nil {
		return fmt.Errorf("claim pending outbox: %w", err)
	}

	var dispatchErr error
	for _, event := range events {
		if err := d.dispatchEvent(ctx, event); err != nil {
			dispatchErr = errors.Join(dispatchErr, err)
		}
	}

	return dispatchErr
}

func (d *OutboxDispatcher) dispatchEvent(ctx context.Context, event OutboxEvent) error {
	attemptCount := event.AttemptCount + 1

	d.log.Info().
		Str("outbox_id", event.ID).
		Str("transfer_id", event.TransferID).
		Str("event_type", event.EventType).
		Int("attempt_count", attemptCount).
		Msg("dispatching transfer outbox event")

	switch event.EventType {
	case OutboxEventTypeTransferRequested:
		var job TransferJob
		if err := json.Unmarshal(event.PayloadJSON, &job); err != nil {
			return d.scheduleRetry(ctx, event, attemptCount, fmt.Errorf("decode transfer outbox payload: %w", err))
		}

		if err := d.publisher.PublishTransferRequested(ctx, job); err != nil {
			return d.scheduleRetry(ctx, event, attemptCount, err)
		}
	default:
		return d.scheduleRetry(ctx, event, attemptCount, fmt.Errorf("unsupported outbox event type %q", event.EventType))
	}

	if err := d.repository.MarkOutboxDispatched(ctx, event.ID, attemptCount); err != nil {
		return fmt.Errorf("mark outbox dispatched: %w", err)
	}

	d.log.Info().
		Str("outbox_id", event.ID).
		Str("transfer_id", event.TransferID).
		Str("event_type", event.EventType).
		Int("attempt_count", attemptCount).
		Msg("transfer outbox event dispatched")

	return nil
}

func (d *OutboxDispatcher) scheduleRetry(ctx context.Context, event OutboxEvent, attemptCount int, dispatchErr error) error {
	nextAvailableAt := time.Now().UTC().Add(d.retryBackoff(attemptCount))

	if err := d.repository.MarkOutboxRetry(ctx, event.ID, attemptCount, nextAvailableAt, dispatchErr.Error()); err != nil {
		return fmt.Errorf("mark outbox retry after dispatch error: %w", err)
	}

	d.log.Warn().
		Err(dispatchErr).
		Str("outbox_id", event.ID).
		Str("transfer_id", event.TransferID).
		Str("event_type", event.EventType).
		Int("attempt_count", attemptCount).
		Time("next_available_at", nextAvailableAt).
		Msg("transfer outbox event scheduled for retry")

	return nil
}

func (d *OutboxDispatcher) retryBackoff(attemptCount int) time.Duration {
	if attemptCount <= 1 {
		return d.retryDelay
	}

	backoff := d.retryDelay
	for step := 1; step < attemptCount; step++ {
		backoff *= 2
		if backoff >= time.Minute {
			return time.Minute
		}
	}

	return backoff
}
