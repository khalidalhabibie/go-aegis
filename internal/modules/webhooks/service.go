package webhooks

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/rs/zerolog"
)

type Service struct {
	repository     Repository
	dispatcher     Dispatcher
	log            zerolog.Logger
	maxAttempts    int
	initialBackoff time.Duration
	batchSize      int
}

func NewService(repository Repository, dispatcher Dispatcher, maxAttempts int, initialBackoff time.Duration, batchSize int, log zerolog.Logger) *Service {
	if maxAttempts <= 0 {
		maxAttempts = 5
	}

	if initialBackoff <= 0 {
		initialBackoff = 2 * time.Second
	}

	if batchSize <= 0 {
		batchSize = 25
	}

	return &Service{
		repository:     repository,
		dispatcher:     dispatcher,
		log:            log,
		maxAttempts:    maxAttempts,
		initialBackoff: initialBackoff,
		batchSize:      batchSize,
	}
}

func (s *Service) RunCycle(ctx context.Context) error {
	scheduled, err := s.repository.ScheduleTransferStatusDeliveries(ctx, s.maxAttempts)
	if err != nil {
		return err
	}

	if scheduled > 0 {
		s.log.Info().Int64("scheduled", scheduled).Msg("scheduled webhook deliveries")
	}

	deliveries, err := s.repository.ListDueDeliveries(ctx, s.batchSize)
	if err != nil {
		return err
	}

	for _, delivery := range deliveries {
		if err := s.processDelivery(ctx, delivery); err != nil {
			return err
		}
	}

	return nil
}

func (s *Service) processDelivery(ctx context.Context, delivery Delivery) error {
	attempt := delivery.AttemptCount + 1

	result, err := s.dispatcher.Dispatch(ctx, delivery)
	if err == nil && result.StatusCode >= http.StatusOK && result.StatusCode < http.StatusMultipleChoices {
		if err := s.repository.MarkDelivered(ctx, MarkDeliveredParams{
			ID:                 delivery.ID,
			AttemptCount:       attempt,
			ResponseStatusCode: result.StatusCode,
			ResponseBody:       result.Body,
		}); err != nil {
			return err
		}

		s.log.Info().
			Str("delivery_id", delivery.ID).
			Str("transfer_id", delivery.TransferRequestID).
			Str("transfer_status", delivery.TransferStatus).
			Int("response_status_code", result.StatusCode).
			Int("attempt", attempt).
			Msg("webhook delivered")
		return nil
	}

	responseBody := result.Body
	lastError := ""
	if err != nil {
		lastError = err.Error()
	} else {
		lastError = fmt.Sprintf("unexpected response status: %d", result.StatusCode)
	}

	if attempt >= delivery.MaxAttempts {
		if err := s.repository.MarkFailed(ctx, MarkFailedParams{
			ID:                 delivery.ID,
			AttemptCount:       attempt,
			ResponseStatusCode: result.StatusCode,
			ResponseBody:       responseBody,
			LastError:          lastError,
		}); err != nil {
			return err
		}

		s.log.Error().
			Str("delivery_id", delivery.ID).
			Str("transfer_id", delivery.TransferRequestID).
			Str("transfer_status", delivery.TransferStatus).
			Int("attempt", attempt).
			Str("last_error", lastError).
			Int("response_status_code", result.StatusCode).
			Msg("webhook delivery failed permanently")
		return nil
	}

	nextAttemptAt := time.Now().UTC().Add(s.backoffForAttempt(attempt))
	if err := s.repository.MarkRetry(ctx, MarkRetryParams{
		ID:                 delivery.ID,
		AttemptCount:       attempt,
		ResponseStatusCode: result.StatusCode,
		ResponseBody:       responseBody,
		LastError:          lastError,
		NextAttemptAt:      nextAttemptAt,
	}); err != nil {
		return err
	}

	s.log.Warn().
		Str("delivery_id", delivery.ID).
		Str("transfer_id", delivery.TransferRequestID).
		Str("transfer_status", delivery.TransferStatus).
		Int("attempt", attempt).
		Time("next_attempt_at", nextAttemptAt).
		Msg("webhook delivery scheduled for retry")

	return nil
}

func (s *Service) backoffForAttempt(attempt int) time.Duration {
	if attempt <= 1 {
		return s.initialBackoff
	}

	return s.initialBackoff * time.Duration(1<<(attempt-1))
}
