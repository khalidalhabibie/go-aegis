package wallets

import (
	"context"
	"errors"
)

var ErrWalletNotFound = errors.New("wallet not found")
var ErrDuplicateActiveWallet = errors.New("duplicate active wallet")

type Repository interface {
	Create(ctx context.Context, params CreateParams) (Wallet, error)
	GetByID(ctx context.Context, id string) (Wallet, error)
	List(ctx context.Context) ([]Wallet, error)
}

type CreateParams struct {
	Chain       string
	Address     string
	Label       string
	SigningType string
	Status      string
}
