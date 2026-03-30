package transfers

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/rs/zerolog"
)

func TestProcessorProcessTransferHappyPath(t *testing.T) {
	repo := newInMemoryRepository(Transfer{
		ID:     "transfer-1",
		Status: StatusCreated,
	})

	processor := NewProcessor(
		repo,
		&stubSigner{signed: SignedTransfer{RawTransaction: "signed-payload", TxHash: "0xabc123"}},
		&stubBroadcaster{},
		zerolog.Nop(),
	)

	transfer, err := processor.ProcessTransfer(context.Background(), "transfer-1")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if transfer.Status != StatusPendingOnChain {
		t.Fatalf("expected status %q, got %q", StatusPendingOnChain, transfer.Status)
	}

	if transfer.TxHash != "0xabc123" {
		t.Fatalf("expected tx hash to be stored, got %q", transfer.TxHash)
	}

	expectedHistory := []string{
		StatusValidated,
		StatusQueued,
		StatusSigning,
		StatusSubmitted,
		StatusPendingOnChain,
	}
	if len(repo.transitions) != len(expectedHistory) {
		t.Fatalf("expected %d transitions, got %d", len(expectedHistory), len(repo.transitions))
	}

	for index, status := range expectedHistory {
		if repo.transitions[index] != status {
			t.Fatalf("expected transition %d to %q, got %q", index, status, repo.transitions[index])
		}
	}
}

func TestProcessorFailTransferMarksTransferFailed(t *testing.T) {
	repo := newInMemoryRepository(Transfer{
		ID:     "transfer-2",
		Status: StatusSigning,
	})

	processor := NewProcessor(
		repo,
		&stubSigner{err: errors.New("signing failed")},
		&stubBroadcaster{},
		zerolog.Nop(),
	)

	if err := processor.FailTransfer(context.Background(), "transfer-2"); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if repo.transfer.Status != StatusFailed {
		t.Fatalf("expected failed status, got %q", repo.transfer.Status)
	}
}

func TestProcessorReturnsTransientError(t *testing.T) {
	repo := newInMemoryRepository(Transfer{
		ID:     "transfer-3",
		Status: StatusSigning,
	})

	processor := NewProcessor(
		repo,
		&stubSigner{signed: SignedTransfer{RawTransaction: "signed-payload", TxHash: "0xtransient"}},
		&stubBroadcaster{err: TransientError{Operation: "broadcast", Err: errors.New("rpc unavailable")}},
		zerolog.Nop(),
	)

	_, err := processor.ProcessTransfer(context.Background(), "transfer-3")
	if !errors.Is(err, ErrTransient) {
		t.Fatalf("expected transient error, got %v", err)
	}

	attempt, getErr := repo.GetLatestAttempt(context.Background(), "transfer-3")
	if getErr != nil {
		t.Fatalf("expected durable attempt to exist, got %v", getErr)
	}

	if attempt.Status != AttemptStatusSigned {
		t.Fatalf("expected attempt to return to SIGNED, got %q", attempt.Status)
	}
}

func TestProcessorRecoversFromBroadcastedAttemptWithoutRebroadcast(t *testing.T) {
	rawPayload, err := newTransactionAttemptPayload("signed-existing")
	if err != nil {
		t.Fatalf("marshal attempt payload: %v", err)
	}

	repo := newInMemoryRepository(Transfer{
		ID:     "transfer-4",
		Status: StatusSigning,
	})
	repo.attempt = &TransactionAttempt{
		ID:         "attempt-1",
		TransferID: "transfer-4",
		RawPayload: rawPayload,
		TxHash:     "0xrecovered",
		Status:     AttemptStatusBroadcasted,
	}

	signer := &stubSigner{}
	broadcaster := &stubBroadcaster{}
	processor := NewProcessor(repo, signer, broadcaster, zerolog.Nop())

	transfer, err := processor.ProcessTransfer(context.Background(), "transfer-4")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if transfer.Status != StatusPendingOnChain {
		t.Fatalf("expected status %q, got %q", StatusPendingOnChain, transfer.Status)
	}

	if signer.calls != 0 {
		t.Fatalf("expected signer not to be called, got %d calls", signer.calls)
	}

	if broadcaster.calls != 0 {
		t.Fatalf("expected broadcaster not to be called, got %d calls", broadcaster.calls)
	}
}

type inMemoryRepository struct {
	transfer    Transfer
	attempt     *TransactionAttempt
	transitions []string
}

func newInMemoryRepository(transfer Transfer) *inMemoryRepository {
	return &inMemoryRepository{transfer: transfer}
}

func (r *inMemoryRepository) Create(context.Context, CreateParams) (Transfer, bool, error) {
	return r.transfer, true, nil
}

func (r *inMemoryRepository) GetByID(_ context.Context, id string) (Transfer, error) {
	if r.transfer.ID != id {
		return Transfer{}, ErrTransferNotFound
	}

	return r.transfer, nil
}

func (r *inMemoryRepository) List(context.Context, ListParams) ([]Transfer, error) {
	return []Transfer{r.transfer}, nil
}

func (r *inMemoryRepository) TransitionStatus(_ context.Context, params TransitionParams) (Transfer, error) {
	if r.transfer.ID != params.ID {
		return Transfer{}, ErrTransferNotFound
	}

	if r.transfer.Status != params.FromStatus {
		return Transfer{}, InvalidStateError{
			TransferID: r.transfer.ID,
			Expected:   params.FromStatus,
			Actual:     r.transfer.Status,
		}
	}

	r.transfer.Status = params.ToStatus
	if params.TxHash != nil {
		r.transfer.TxHash = *params.TxHash
	}

	r.transitions = append(r.transitions, params.ToStatus)

	return r.transfer, nil
}

func (r *inMemoryRepository) GetLatestAttempt(_ context.Context, transferID string) (TransactionAttempt, error) {
	if r.transfer.ID != transferID || r.attempt == nil {
		return TransactionAttempt{}, ErrTransactionAttemptNotFound
	}

	return *r.attempt, nil
}

func (r *inMemoryRepository) CreateAttempt(_ context.Context, params CreateAttemptParams) (TransactionAttempt, error) {
	r.attempt = &TransactionAttempt{
		ID:           "attempt-" + r.transfer.ID,
		TransferID:   params.TransferID,
		Nonce:        params.Nonce,
		RawPayload:   params.RawPayload,
		TxHash:       params.TxHash,
		Status:       params.Status,
		ErrorMessage: params.ErrorMessage,
	}

	return *r.attempt, nil
}

func (r *inMemoryRepository) UpdateAttempt(_ context.Context, params UpdateAttemptParams) (TransactionAttempt, error) {
	if r.attempt == nil || r.attempt.ID != params.AttemptID {
		return TransactionAttempt{}, ErrTransactionAttemptNotFound
	}

	r.attempt.Status = params.Status
	r.attempt.ErrorMessage = params.ErrorMessage

	return *r.attempt, nil
}

type stubSigner struct {
	signed SignedTransfer
	err    error
	calls  int
}

func (s *stubSigner) SignTransfer(context.Context, Transfer) (SignedTransfer, error) {
	s.calls++
	return s.signed, s.err
}

type stubBroadcaster struct {
	err         error
	calls       int
	lastAttempt TransactionAttempt
}

func (b *stubBroadcaster) BroadcastTransfer(_ context.Context, _ Transfer, attempt TransactionAttempt) error {
	b.calls++
	b.lastAttempt = attempt
	return b.err
}

func TestAttemptPayloadRoundTrip(t *testing.T) {
	rawPayload, err := newTransactionAttemptPayload("signed-round-trip")
	if err != nil {
		t.Fatalf("marshal attempt payload: %v", err)
	}

	attempt := TransactionAttempt{RawPayload: rawPayload}
	payload, err := attempt.Payload()
	if err != nil {
		t.Fatalf("decode attempt payload: %v", err)
	}

	if payload.RawTransaction != "signed-round-trip" {
		t.Fatalf("expected raw transaction to round-trip, got %q", payload.RawTransaction)
	}

	if payload.Encoding != "raw_transaction" {
		t.Fatalf("expected payload encoding raw_transaction, got %q", payload.Encoding)
	}

	if !json.Valid(rawPayload) {
		t.Fatalf("expected valid json payload, got %s", rawPayload)
	}
}
