package handlers

import (
	"time"

	"aegis/internal/modules/wallets"
)

type createWalletRequest struct {
	Chain       string `json:"chain"`
	Address     string `json:"address"`
	Label       string `json:"label"`
	SigningType string `json:"signing_type"`
	Status      string `json:"status"`
}

type walletResponse struct {
	ID          string    `json:"id"`
	Chain       string    `json:"chain"`
	Address     string    `json:"address"`
	Label       string    `json:"label"`
	SigningType string    `json:"signing_type"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type walletEnvelope struct {
	Data walletResponse `json:"data"`
}

type walletListEnvelope struct {
	Data []walletResponse `json:"data"`
}

func newWalletResponse(wallet wallets.Wallet) walletResponse {
	return walletResponse{
		ID:          wallet.ID,
		Chain:       wallet.Chain,
		Address:     wallet.Address,
		Label:       wallet.Label,
		SigningType: wallet.SigningType,
		Status:      wallet.Status,
		CreatedAt:   wallet.CreatedAt,
		UpdatedAt:   wallet.UpdatedAt,
	}
}
