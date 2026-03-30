package transfers

import (
	"context"
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
		&stubSigner{signed: SignedTransfer{RawTransaction: "signed-payload"}},
		&stubBroadcaster{txHash: "0xabc123"},
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
		&stubSigner{},
		&stubBroadcaster{err: TransientError{Operation: "broadcast", Err: errors.New("rpc unavailable")}},
		zerolog.Nop(),
	)

	_, err := processor.ProcessTransfer(context.Background(), "transfer-3")
	if !errors.Is(err, ErrTransient) {
		t.Fatalf("expected transient error, got %v", err)
	}
}

type inMemoryRepository struct {
	transfer    Transfer
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

type stubSigner struct {
	signed SignedTransfer
	err    error
}

func (s *stubSigner) SignTransfer(context.Context, Transfer) (SignedTransfer, error) {
	return s.signed, s.err
}

type stubBroadcaster struct {
	txHash string
	err    error
}

func (b *stubBroadcaster) BroadcastTransfer(context.Context, Transfer, SignedTransfer) (string, error) {
	return b.txHash, b.err
}
