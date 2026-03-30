package webhooks

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

func TestServiceRunCycleMarksDelivered(t *testing.T) {
	leaseExpiresAt := time.Now().UTC().Add(30 * time.Second)
	repo := &stubRepository{
		scheduleFn: func(context.Context, int) (int64, error) { return 1, nil },
		claimDueFn: func(context.Context, int, time.Duration) ([]Delivery, error) {
			return []Delivery{{
				ID:                "delivery-1",
				TransferRequestID: "transfer-1",
				TransferStatus:    "SUBMITTED",
				MaxAttempts:       5,
				LeaseExpiresAt:    &leaseExpiresAt,
			}}, nil
		},
	}
	dispatcher := &stubDispatcher{
		result: DispatchResult{
			StatusCode: 200,
			Body:       "ok",
		},
	}

	service := NewService(repo, dispatcher, 5, time.Second, 10, 30*time.Second, zerolog.Nop())

	if err := service.RunCycle(context.Background()); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if repo.delivered == nil || repo.delivered.AttemptCount != 1 {
		t.Fatalf("expected delivery to be marked delivered, got %+v", repo.delivered)
	}
}

func TestServiceRunCycleSchedulesRetry(t *testing.T) {
	leaseExpiresAt := time.Now().UTC().Add(30 * time.Second)
	repo := &stubRepository{
		scheduleFn: func(context.Context, int) (int64, error) { return 0, nil },
		claimDueFn: func(context.Context, int, time.Duration) ([]Delivery, error) {
			return []Delivery{{
				ID:                "delivery-2",
				TransferRequestID: "transfer-2",
				TransferStatus:    "FAILED",
				MaxAttempts:       5,
				LeaseExpiresAt:    &leaseExpiresAt,
			}}, nil
		},
	}
	dispatcher := &stubDispatcher{
		err: errors.New("network down"),
	}

	service := NewService(repo, dispatcher, 5, time.Second, 10, 30*time.Second, zerolog.Nop())

	if err := service.RunCycle(context.Background()); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if repo.retry == nil || repo.retry.AttemptCount != 1 {
		t.Fatalf("expected retry to be scheduled, got %+v", repo.retry)
	}
}

func TestServiceRunCycleClaimsDeliveryOnceAcrossConcurrentWorkers(t *testing.T) {
	var mu sync.Mutex
	claimed := false
	leaseExpiresAt := time.Now().UTC().Add(30 * time.Second)

	repo := &stubRepository{
		scheduleFn: func(context.Context, int) (int64, error) { return 0, nil },
		claimDueFn: func(context.Context, int, time.Duration) ([]Delivery, error) {
			mu.Lock()
			defer mu.Unlock()

			if claimed {
				return nil, nil
			}

			claimed = true

			return []Delivery{{
				ID:                "delivery-3",
				TransferRequestID: "transfer-3",
				TransferStatus:    "SUBMITTED",
				MaxAttempts:       5,
				LeaseExpiresAt:    &leaseExpiresAt,
			}}, nil
		},
	}

	dispatcher := &stubDispatcher{
		result: DispatchResult{StatusCode: 200, Body: "ok"},
	}

	service := NewService(repo, dispatcher, 5, time.Second, 10, 30*time.Second, zerolog.Nop())

	start := make(chan struct{})
	var group sync.WaitGroup
	group.Add(2)

	for worker := 0; worker < 2; worker++ {
		go func() {
			defer group.Done()
			<-start
			if err := service.RunCycle(context.Background()); err != nil {
				t.Errorf("expected no error, got %v", err)
			}
		}()
	}

	close(start)
	group.Wait()

	if dispatcher.calls.Load() != 1 {
		t.Fatalf("expected exactly one dispatch call, got %d", dispatcher.calls.Load())
	}
}

func TestServiceRunCycleIgnoresLeaseLossDuringPersist(t *testing.T) {
	leaseExpiresAt := time.Now().UTC().Add(30 * time.Second)
	repo := &stubRepository{
		scheduleFn: func(context.Context, int) (int64, error) { return 0, nil },
		claimDueFn: func(context.Context, int, time.Duration) ([]Delivery, error) {
			return []Delivery{{
				ID:                "delivery-4",
				TransferRequestID: "transfer-4",
				TransferStatus:    "SUBMITTED",
				MaxAttempts:       5,
				LeaseExpiresAt:    &leaseExpiresAt,
			}}, nil
		},
		markDeliveredErr: ErrDeliveryLeaseLost,
	}

	dispatcher := &stubDispatcher{
		result: DispatchResult{
			StatusCode: 200,
			Body:       "ok",
		},
	}

	service := NewService(repo, dispatcher, 5, time.Second, 10, 30*time.Second, zerolog.Nop())

	if err := service.RunCycle(context.Background()); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

type stubRepository struct {
	scheduleFn       func(ctx context.Context, maxAttempts int) (int64, error)
	claimDueFn       func(ctx context.Context, limit int, leaseDuration time.Duration) ([]Delivery, error)
	delivered        *MarkDeliveredParams
	retry            *MarkRetryParams
	failed           *MarkFailedParams
	markDeliveredErr error
	markRetryErr     error
	markFailedErr    error
}

func (s *stubRepository) ScheduleTransferStatusDeliveries(ctx context.Context, maxAttempts int) (int64, error) {
	if s.scheduleFn == nil {
		return 0, nil
	}

	return s.scheduleFn(ctx, maxAttempts)
}

func (s *stubRepository) ClaimDueDeliveries(ctx context.Context, limit int, leaseDuration time.Duration) ([]Delivery, error) {
	if s.claimDueFn == nil {
		return nil, nil
	}

	return s.claimDueFn(ctx, limit, leaseDuration)
}

func (s *stubRepository) MarkDelivered(_ context.Context, params MarkDeliveredParams) error {
	if s.markDeliveredErr != nil {
		return s.markDeliveredErr
	}

	s.delivered = &params
	return nil
}

func (s *stubRepository) MarkRetry(_ context.Context, params MarkRetryParams) error {
	if s.markRetryErr != nil {
		return s.markRetryErr
	}

	s.retry = &params
	return nil
}

func (s *stubRepository) MarkFailed(_ context.Context, params MarkFailedParams) error {
	if s.markFailedErr != nil {
		return s.markFailedErr
	}

	s.failed = &params
	return nil
}

type stubDispatcher struct {
	result DispatchResult
	err    error
	calls  atomic.Int32
}

func (s *stubDispatcher) Dispatch(context.Context, Delivery) (DispatchResult, error) {
	s.calls.Add(1)
	return s.result, s.err
}
