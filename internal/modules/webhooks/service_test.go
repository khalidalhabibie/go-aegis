package webhooks

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

func TestServiceRunCycleMarksDelivered(t *testing.T) {
	repo := &stubRepository{
		scheduleFn: func(context.Context, int) (int64, error) { return 1, nil },
		listDueFn: func(context.Context, int) ([]Delivery, error) {
			return []Delivery{{
				ID:                "delivery-1",
				TransferRequestID: "transfer-1",
				TransferStatus:    "SUBMITTED",
				MaxAttempts:       5,
			}}, nil
		},
	}
	dispatcher := &stubDispatcher{
		result: DispatchResult{
			StatusCode: 200,
			Body:       "ok",
		},
	}

	service := NewService(repo, dispatcher, 5, time.Second, 10, zerolog.Nop())

	if err := service.RunCycle(context.Background()); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if repo.delivered == nil || repo.delivered.AttemptCount != 1 {
		t.Fatalf("expected delivery to be marked delivered, got %+v", repo.delivered)
	}
}

func TestServiceRunCycleSchedulesRetry(t *testing.T) {
	repo := &stubRepository{
		scheduleFn: func(context.Context, int) (int64, error) { return 0, nil },
		listDueFn: func(context.Context, int) ([]Delivery, error) {
			return []Delivery{{
				ID:                "delivery-2",
				TransferRequestID: "transfer-2",
				TransferStatus:    "FAILED",
				MaxAttempts:       5,
			}}, nil
		},
	}
	dispatcher := &stubDispatcher{
		err: errors.New("network down"),
	}

	service := NewService(repo, dispatcher, 5, time.Second, 10, zerolog.Nop())

	if err := service.RunCycle(context.Background()); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if repo.retry == nil || repo.retry.AttemptCount != 1 {
		t.Fatalf("expected retry to be scheduled, got %+v", repo.retry)
	}
}

type stubRepository struct {
	scheduleFn func(ctx context.Context, maxAttempts int) (int64, error)
	listDueFn  func(ctx context.Context, limit int) ([]Delivery, error)
	delivered  *MarkDeliveredParams
	retry      *MarkRetryParams
	failed     *MarkFailedParams
}

func (s *stubRepository) ScheduleTransferStatusDeliveries(ctx context.Context, maxAttempts int) (int64, error) {
	if s.scheduleFn == nil {
		return 0, nil
	}

	return s.scheduleFn(ctx, maxAttempts)
}

func (s *stubRepository) ListDueDeliveries(ctx context.Context, limit int) ([]Delivery, error) {
	if s.listDueFn == nil {
		return nil, nil
	}

	return s.listDueFn(ctx, limit)
}

func (s *stubRepository) MarkDelivered(_ context.Context, params MarkDeliveredParams) error {
	s.delivered = &params
	return nil
}

func (s *stubRepository) MarkRetry(_ context.Context, params MarkRetryParams) error {
	s.retry = &params
	return nil
}

func (s *stubRepository) MarkFailed(_ context.Context, params MarkFailedParams) error {
	s.failed = &params
	return nil
}

type stubDispatcher struct {
	result DispatchResult
	err    error
}

func (s *stubDispatcher) Dispatch(context.Context, Delivery) (DispatchResult, error) {
	return s.result, s.err
}
