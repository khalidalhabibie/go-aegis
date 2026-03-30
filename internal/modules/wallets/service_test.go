package wallets

import (
	"context"
	"errors"
	"testing"

	"github.com/rs/zerolog"
)

func TestCreateWalletNormalizesInput(t *testing.T) {
	repo := &stubRepository{
		createFn: func(_ context.Context, params CreateParams) (Wallet, error) {
			if params.Chain != "ethereum" {
				t.Fatalf("expected normalized chain, got %q", params.Chain)
			}

			if params.Address != "0x000000000000000000000000000000000000dEaD" {
				t.Fatalf("expected normalized address, got %q", params.Address)
			}

			if params.Status != StatusActive {
				t.Fatalf("expected default active status, got %q", params.Status)
			}

			return Wallet{
				ID:      "wallet-1",
				Chain:   params.Chain,
				Address: params.Address,
				Status:  params.Status,
			}, nil
		},
	}

	service := NewService(repo, zerolog.Nop())

	wallet, err := service.CreateWallet(context.Background(), CreateInput{
		Chain:       " Ethereum ",
		Address:     "0x000000000000000000000000000000000000dEaD",
		Label:       "Treasury",
		SigningType: "KMS",
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if wallet.Status != StatusActive {
		t.Fatalf("expected active status, got %q", wallet.Status)
	}
}

func TestCreateWalletRejectsInvalidAddress(t *testing.T) {
	service := NewService(&stubRepository{}, zerolog.Nop())

	_, err := service.CreateWallet(context.Background(), CreateInput{
		Chain:       "ethereum",
		Address:     "invalid",
		Label:       "Treasury",
		SigningType: "kms",
	})
	if err == nil {
		t.Fatal("expected validation error")
	}

	if !errors.Is(err, ErrValidation) {
		t.Fatalf("expected validation error, got %v", err)
	}
}

type stubRepository struct {
	createFn func(ctx context.Context, params CreateParams) (Wallet, error)
	getFn    func(ctx context.Context, id string) (Wallet, error)
	listFn   func(ctx context.Context) ([]Wallet, error)
}

func (s *stubRepository) Create(ctx context.Context, params CreateParams) (Wallet, error) {
	if s.createFn == nil {
		return Wallet{}, nil
	}

	return s.createFn(ctx, params)
}

func (s *stubRepository) GetByID(ctx context.Context, id string) (Wallet, error) {
	if s.getFn == nil {
		return Wallet{}, ErrWalletNotFound
	}

	return s.getFn(ctx, id)
}

func (s *stubRepository) List(ctx context.Context) ([]Wallet, error) {
	if s.listFn == nil {
		return nil, nil
	}

	return s.listFn(ctx)
}
