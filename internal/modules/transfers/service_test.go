package transfers

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/rs/zerolog"
)

func TestCreateTransferNormalizesInput(t *testing.T) {
	repo := &stubRepository{
		createFn: func(_ context.Context, params CreateParams) (Transfer, bool, error) {
			if params.Status != StatusCreated {
				t.Fatalf("expected status %q, got %q", StatusCreated, params.Status)
			}

			if string(params.MetadataJSON) != `{"purpose":"payout"}` {
				t.Fatalf("unexpected metadata: %s", params.MetadataJSON)
			}

			return Transfer{
				ID:             "00000000-0000-0000-0000-000000000001",
				IdempotencyKey: params.IdempotencyKey,
				Status:         params.Status,
				MetadataJSON:   params.MetadataJSON,
			}, true, nil
		},
	}

	service := NewService(repo, zerolog.Nop())

	transfer, created, err := service.CreateTransfer(context.Background(), CreateInput{
		IdempotencyKey:     " idem-1 ",
		Chain:              "ethereum",
		AssetType:          "native",
		SourceWalletID:     "00000000-0000-0000-0000-000000000123",
		DestinationAddress: "0x000000000000000000000000000000000000dEaD",
		Amount:             "1000000",
		CallbackURL:        "https://example.com/webhook",
		MetadataJSON:       json.RawMessage(`{"purpose":"payout"}`),
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if !created {
		t.Fatal("expected transfer to be created")
	}

	if transfer.IdempotencyKey != "idem-1" {
		t.Fatalf("expected trimmed idempotency key, got %q", transfer.IdempotencyKey)
	}
}

func TestCreateTransferRejectsInvalidAmount(t *testing.T) {
	service := NewService(&stubRepository{}, zerolog.Nop())

	_, _, err := service.CreateTransfer(context.Background(), CreateInput{
		IdempotencyKey:     "idem-1",
		Chain:              "ethereum",
		AssetType:          "native",
		SourceWalletID:     "00000000-0000-0000-0000-000000000123",
		DestinationAddress: "0x000000000000000000000000000000000000dEaD",
		Amount:             "12.5",
	})
	if err == nil {
		t.Fatal("expected validation error")
	}

	if !errors.Is(err, ErrValidation) {
		t.Fatalf("expected validation error, got %v", err)
	}
}

func TestListTransfersNormalizesPagination(t *testing.T) {
	repo := &stubRepository{
		listFn: func(_ context.Context, params ListParams) ([]Transfer, error) {
			if params.Limit != 100 {
				t.Fatalf("expected capped limit 100, got %d", params.Limit)
			}

			if params.Offset != 0 {
				t.Fatalf("expected offset 0, got %d", params.Offset)
			}

			return []Transfer{}, nil
		},
	}

	service := NewService(repo, zerolog.Nop())

	result, err := service.ListTransfers(context.Background(), ListInput{
		Limit:  150,
		Offset: 0,
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if result.Limit != 100 {
		t.Fatalf("expected normalized result limit 100, got %d", result.Limit)
	}
}

func TestCreateTransferRejectsInvalidSourceWalletID(t *testing.T) {
	service := NewService(&stubRepository{}, zerolog.Nop())

	_, _, err := service.CreateTransfer(context.Background(), CreateInput{
		IdempotencyKey:     "idem-1",
		Chain:              "ethereum",
		AssetType:          "native",
		SourceWalletID:     "wallet_123",
		DestinationAddress: "0x000000000000000000000000000000000000dEaD",
		Amount:             "1000",
	})
	if err == nil {
		t.Fatal("expected validation error")
	}

	if !errors.Is(err, ErrValidation) {
		t.Fatalf("expected validation error, got %v", err)
	}
}

type stubRepository struct {
	createFn     func(ctx context.Context, params CreateParams) (Transfer, bool, error)
	getFn        func(ctx context.Context, id string) (Transfer, error)
	listFn       func(ctx context.Context, params ListParams) ([]Transfer, error)
	transitionFn func(ctx context.Context, params TransitionParams) (Transfer, error)
}

func (s *stubRepository) Create(ctx context.Context, params CreateParams) (Transfer, bool, error) {
	if s.createFn == nil {
		return Transfer{}, false, nil
	}

	return s.createFn(ctx, params)
}

func (s *stubRepository) GetByID(ctx context.Context, id string) (Transfer, error) {
	if s.getFn == nil {
		return Transfer{}, ErrTransferNotFound
	}

	return s.getFn(ctx, id)
}

func (s *stubRepository) List(ctx context.Context, params ListParams) ([]Transfer, error) {
	if s.listFn == nil {
		return nil, nil
	}

	return s.listFn(ctx, params)
}

func (s *stubRepository) TransitionStatus(ctx context.Context, params TransitionParams) (Transfer, error) {
	if s.transitionFn == nil {
		return Transfer{}, nil
	}

	return s.transitionFn(ctx, params)
}

func (s *stubRepository) GetLatestAttempt(context.Context, string) (TransactionAttempt, error) {
	return TransactionAttempt{}, ErrTransactionAttemptNotFound
}

func (s *stubRepository) CreateAttempt(context.Context, CreateAttemptParams) (TransactionAttempt, error) {
	return TransactionAttempt{}, nil
}

func (s *stubRepository) UpdateAttempt(context.Context, UpdateAttemptParams) (TransactionAttempt, error) {
	return TransactionAttempt{}, nil
}
