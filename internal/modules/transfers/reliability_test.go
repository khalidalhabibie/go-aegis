package transfers

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

func TestCreateTransferPersistsForLaterOutboxDispatch(t *testing.T) {
	repository := newMemoryTransferOutboxRepository()
	service := NewService(repository, CallbackURLPolicy{}, zerolog.Nop())
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

	transfer, created, err := service.CreateTransfer(context.Background(), CreateInput{
		IdempotencyKey:     "idem-outbox-1",
		Chain:              "ethereum",
		AssetType:          "native",
		SourceWalletID:     "00000000-0000-0000-0000-000000000123",
		DestinationAddress: "0x000000000000000000000000000000000000dEaD",
		Amount:             "1000",
		CallbackURL:        "https://hooks.example.com/transfers",
		MetadataJSON:       json.RawMessage(`{"purpose":"reliability"}`),
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if !created {
		t.Fatal("expected transfer to be created")
	}

	if len(repository.outbox) != 1 {
		t.Fatalf("expected one outbox event, got %d", len(repository.outbox))
	}

	if len(publisher.jobs) != 0 {
		t.Fatalf("expected no immediate queue publish during create, got %d jobs", len(publisher.jobs))
	}

	if err := dispatcher.DispatchPending(context.Background()); err != nil {
		t.Fatalf("expected outbox dispatch to succeed, got %v", err)
	}

	if len(publisher.jobs) != 1 {
		t.Fatalf("expected one published job after dispatcher run, got %d", len(publisher.jobs))
	}

	if publisher.jobs[0].TransferID != transfer.ID {
		t.Fatalf("expected published transfer id %q, got %q", transfer.ID, publisher.jobs[0].TransferID)
	}

	if repository.outbox[0].Status != OutboxStatusDispatched {
		t.Fatalf("expected outbox status %q, got %q", OutboxStatusDispatched, repository.outbox[0].Status)
	}
}

type memoryTransferOutboxRepository struct {
	transferCounter int
	transfers       []Transfer
	outbox          []OutboxEvent
}

func newMemoryTransferOutboxRepository() *memoryTransferOutboxRepository {
	return &memoryTransferOutboxRepository{}
}

func (r *memoryTransferOutboxRepository) Create(_ context.Context, params CreateParams) (Transfer, bool, error) {
	for _, existing := range r.transfers {
		if existing.IdempotencyKey == params.IdempotencyKey {
			return existing, false, nil
		}
	}

	r.transferCounter++
	transferID := fmt.Sprintf("00000000-0000-0000-0000-%012d", r.transferCounter)
	now := time.Now().UTC()

	transfer := Transfer{
		ID:                 transferID,
		IdempotencyKey:     params.IdempotencyKey,
		Chain:              params.Chain,
		AssetType:          params.AssetType,
		SourceWalletID:     params.SourceWalletID,
		DestinationAddress: params.DestinationAddress,
		Amount:             params.Amount,
		CallbackURL:        params.CallbackURL,
		MetadataJSON:       params.MetadataJSON,
		Status:             params.Status,
		CreatedAt:          now,
		UpdatedAt:          now,
	}

	r.transfers = append(r.transfers, transfer)

	payload, err := json.Marshal(TransferJob{TransferID: transfer.ID, Attempt: 0})
	if err != nil {
		return Transfer{}, false, err
	}

	r.outbox = append(r.outbox, OutboxEvent{
		ID:           transfer.ID + "-outbox",
		TransferID:   transfer.ID,
		EventType:    OutboxEventTypeTransferRequested,
		PayloadJSON:  payload,
		Status:       OutboxStatusPending,
		AttemptCount: 0,
		AvailableAt:  now,
		CreatedAt:    now,
		UpdatedAt:    now,
	})

	return transfer, true, nil
}

func (r *memoryTransferOutboxRepository) GetByID(_ context.Context, id string) (Transfer, error) {
	for _, transfer := range r.transfers {
		if transfer.ID == id {
			return transfer, nil
		}
	}

	return Transfer{}, ErrTransferNotFound
}

func (r *memoryTransferOutboxRepository) List(context.Context, ListParams) ([]Transfer, error) {
	return r.transfers, nil
}

func (r *memoryTransferOutboxRepository) TransitionStatus(context.Context, TransitionParams) (Transfer, error) {
	return Transfer{}, nil
}

func (r *memoryTransferOutboxRepository) GetLatestAttempt(context.Context, string) (TransactionAttempt, error) {
	return TransactionAttempt{}, ErrTransactionAttemptNotFound
}

func (r *memoryTransferOutboxRepository) CreateAttempt(context.Context, CreateAttemptParams) (TransactionAttempt, error) {
	return TransactionAttempt{}, nil
}

func (r *memoryTransferOutboxRepository) UpdateAttempt(context.Context, UpdateAttemptParams) (TransactionAttempt, error) {
	return TransactionAttempt{}, nil
}

func (r *memoryTransferOutboxRepository) ClaimPendingOutbox(_ context.Context, _ int, _ time.Time) ([]OutboxEvent, error) {
	events := make([]OutboxEvent, 0, len(r.outbox))
	for index := range r.outbox {
		if r.outbox[index].Status == OutboxStatusPending || r.outbox[index].Status == OutboxStatusRetry {
			r.outbox[index].Status = OutboxStatusProcessing
			events = append(events, r.outbox[index])
		}
	}

	return events, nil
}

func (r *memoryTransferOutboxRepository) MarkOutboxDispatched(_ context.Context, outboxID string, attemptCount int) error {
	for index := range r.outbox {
		if r.outbox[index].ID == outboxID {
			r.outbox[index].Status = OutboxStatusDispatched
			r.outbox[index].AttemptCount = attemptCount
			return nil
		}
	}

	return nil
}

func (r *memoryTransferOutboxRepository) MarkOutboxRetry(_ context.Context, outboxID string, attemptCount int, nextAvailableAt time.Time, lastError string) error {
	for index := range r.outbox {
		if r.outbox[index].ID == outboxID {
			r.outbox[index].Status = OutboxStatusRetry
			r.outbox[index].AttemptCount = attemptCount
			r.outbox[index].AvailableAt = nextAvailableAt
			r.outbox[index].LastError = lastError
			return nil
		}
	}

	return nil
}
