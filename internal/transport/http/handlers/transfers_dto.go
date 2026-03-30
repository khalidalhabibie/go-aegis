package handlers

import (
	"encoding/json"
	"time"

	"aegis/internal/modules/transfers"
)

type createTransferRequest struct {
	IdempotencyKey     string          `json:"idempotency_key"`
	Chain              string          `json:"chain"`
	AssetType          string          `json:"asset_type"`
	SourceWalletID     string          `json:"source_wallet_id"`
	DestinationAddress string          `json:"destination_address"`
	Amount             string          `json:"amount"`
	CallbackURL        string          `json:"callback_url"`
	MetadataJSON       json.RawMessage `json:"metadata_json"`
}

type listTransfersQuery struct {
	Limit  int `form:"limit"`
	Offset int `form:"offset"`
}

type transferResponse struct {
	ID                 string          `json:"id"`
	IdempotencyKey     string          `json:"idempotency_key"`
	Chain              string          `json:"chain"`
	AssetType          string          `json:"asset_type"`
	SourceWalletID     string          `json:"source_wallet_id"`
	DestinationAddress string          `json:"destination_address"`
	Amount             string          `json:"amount"`
	CallbackURL        string          `json:"callback_url"`
	MetadataJSON       json.RawMessage `json:"metadata_json"`
	TxHash             string          `json:"tx_hash"`
	Status             string          `json:"status"`
	CreatedAt          time.Time       `json:"created_at"`
	UpdatedAt          time.Time       `json:"updated_at"`
}

type transferEnvelope struct {
	Data transferResponse `json:"data"`
}

type transferListEnvelope struct {
	Data []transferResponse `json:"data"`
	Meta listMeta           `json:"meta"`
}

type listMeta struct {
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
	Count  int `json:"count"`
}

func newTransferResponse(transfer transfers.Transfer) transferResponse {
	metadata := transfer.MetadataJSON
	if len(metadata) == 0 {
		metadata = json.RawMessage(`{}`)
	}

	return transferResponse{
		ID:                 transfer.ID,
		IdempotencyKey:     transfer.IdempotencyKey,
		Chain:              transfer.Chain,
		AssetType:          transfer.AssetType,
		SourceWalletID:     transfer.SourceWalletID,
		DestinationAddress: transfer.DestinationAddress,
		Amount:             transfer.Amount,
		CallbackURL:        transfer.CallbackURL,
		MetadataJSON:       metadata,
		TxHash:             transfer.TxHash,
		Status:             transfer.Status,
		CreatedAt:          transfer.CreatedAt,
		UpdatedAt:          transfer.UpdatedAt,
	}
}
