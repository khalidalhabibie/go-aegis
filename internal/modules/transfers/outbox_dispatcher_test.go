package transfers

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

func TestOutboxDispatcherDispatchPendingMarksDispatched(t *testing.T) {
	repository := &stubOutboxRepository{
		events: []OutboxEvent{
			{
				ID:           "outbox-1",
				TransferID:   "transfer-1",
				EventType:    OutboxEventTypeTransferRequested,
				PayloadJSON:  mustMarshalJob(t, TransferJob{TransferID: "transfer-1", Attempt: 0}),
				Status:       OutboxStatusPending,
				AttemptCount: 0,
			},
		},
	}
	publisher := &stubPublisher{}

	dispatcher := NewOutboxDispatcher(
		repository,
		publisher,
		10,
		time.Second,
		time.Second,
		30*time.Second,
		zerolog.Nop(),
	)

	if err := dispatcher.DispatchPending(context.Background()); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(publisher.jobs) != 1 {
		t.Fatalf("expected one published job, got %d", len(publisher.jobs))
	}

	if len(repository.dispatched) != 1 || repository.dispatched[0].outboxID != "outbox-1" {
		t.Fatalf("expected outbox to be marked dispatched, got %+v", repository.dispatched)
	}

	if len(repository.retries) != 0 {
		t.Fatalf("expected no retries, got %+v", repository.retries)
	}
}

func TestOutboxDispatcherDispatchPendingSchedulesRetryOnPublishFailure(t *testing.T) {
	repository := &stubOutboxRepository{
		events: []OutboxEvent{
			{
				ID:           "outbox-2",
				TransferID:   "transfer-2",
				EventType:    OutboxEventTypeTransferRequested,
				PayloadJSON:  mustMarshalJob(t, TransferJob{TransferID: "transfer-2", Attempt: 0}),
				Status:       OutboxStatusPending,
				AttemptCount: 1,
			},
		},
	}
	publisher := &stubPublisher{err: errors.New("rabbitmq unavailable")}

	dispatcher := NewOutboxDispatcher(
		repository,
		publisher,
		10,
		time.Second,
		2*time.Second,
		30*time.Second,
		zerolog.Nop(),
	)

	if err := dispatcher.DispatchPending(context.Background()); err != nil {
		t.Fatalf("expected retry handling to swallow publish error, got %v", err)
	}

	if len(repository.dispatched) != 0 {
		t.Fatalf("expected no dispatched marks, got %+v", repository.dispatched)
	}

	if len(repository.retries) != 1 {
		t.Fatalf("expected one retry mark, got %+v", repository.retries)
	}

	retry := repository.retries[0]
	if retry.outboxID != "outbox-2" {
		t.Fatalf("expected retry for outbox-2, got %+v", retry)
	}

	if retry.attemptCount != 2 {
		t.Fatalf("expected attempt count 2, got %d", retry.attemptCount)
	}

	if retry.nextAvailableAt.IsZero() {
		t.Fatal("expected retry next available timestamp to be set")
	}
}

type stubOutboxRepository struct {
	events     []OutboxEvent
	dispatched []stubOutboxDispatchMark
	retries    []stubOutboxRetryMark
	claimErr   error
	disposeErr error
	retryErr   error
}

type stubOutboxDispatchMark struct {
	outboxID     string
	attemptCount int
}

type stubOutboxRetryMark struct {
	outboxID        string
	attemptCount    int
	nextAvailableAt time.Time
	lastError       string
}

func (s *stubOutboxRepository) ClaimPendingOutbox(context.Context, int, time.Time) ([]OutboxEvent, error) {
	if s.claimErr != nil {
		return nil, s.claimErr
	}

	return s.events, nil
}

func (s *stubOutboxRepository) MarkOutboxDispatched(_ context.Context, outboxID string, attemptCount int) error {
	if s.disposeErr != nil {
		return s.disposeErr
	}

	s.dispatched = append(s.dispatched, stubOutboxDispatchMark{
		outboxID:     outboxID,
		attemptCount: attemptCount,
	})

	return nil
}

func (s *stubOutboxRepository) MarkOutboxRetry(_ context.Context, outboxID string, attemptCount int, nextAvailableAt time.Time, lastError string) error {
	if s.retryErr != nil {
		return s.retryErr
	}

	s.retries = append(s.retries, stubOutboxRetryMark{
		outboxID:        outboxID,
		attemptCount:    attemptCount,
		nextAvailableAt: nextAvailableAt,
		lastError:       lastError,
	})

	return nil
}

type stubPublisher struct {
	jobs []TransferJob
	err  error
}

func (s *stubPublisher) PublishTransferRequested(_ context.Context, job TransferJob) error {
	if s.err != nil {
		return s.err
	}

	s.jobs = append(s.jobs, job)
	return nil
}

func mustMarshalJob(t *testing.T, job TransferJob) json.RawMessage {
	t.Helper()

	payload, err := json.Marshal(job)
	if err != nil {
		t.Fatalf("marshal transfer job: %v", err)
	}

	return payload
}
