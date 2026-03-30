package transfers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/url"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

var ErrValidation = errors.New("validation error")

type ValidationError struct {
	Field   string
	Message string
}

func (e ValidationError) Error() string {
	if e.Field == "" {
		return e.Message
	}

	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

func (e ValidationError) Unwrap() error {
	return ErrValidation
}

type Service struct {
	repository Repository
	publisher  JobPublisher
	log        zerolog.Logger
}

type CreateInput struct {
	IdempotencyKey     string
	Chain              string
	AssetType          string
	SourceWalletID     string
	DestinationAddress string
	Amount             string
	CallbackURL        string
	MetadataJSON       json.RawMessage
}

type ListInput struct {
	Limit  int
	Offset int
}

type ListResult struct {
	Items  []Transfer
	Limit  int
	Offset int
}

func NewService(repository Repository, publisher JobPublisher, log zerolog.Logger) *Service {
	return &Service{
		repository: repository,
		publisher:  publisher,
		log:        log,
	}
}

func (s *Service) CreateTransfer(ctx context.Context, input CreateInput) (Transfer, bool, error) {
	params, err := normalizeCreateInput(input)
	if err != nil {
		return Transfer{}, false, err
	}

	transfer, created, err := s.repository.Create(ctx, params)
	if err != nil {
		return Transfer{}, false, err
	}

	if s.publisher != nil && transfer.Status == StatusCreated {
		job := TransferJob{TransferID: transfer.ID, Attempt: 0}
		if err := s.publisher.PublishTransferRequested(ctx, job); err != nil {
			s.log.Error().
				Err(err).
				Str("transfer_id", transfer.ID).
				Str("idempotency_key", transfer.IdempotencyKey).
				Bool("created", created).
				Msg("publish transfer job failed")
			return Transfer{}, false, fmt.Errorf("publish transfer job: %w", err)
		}

		s.log.Info().
			Str("transfer_id", transfer.ID).
			Str("status", transfer.Status).
			Bool("created", created).
			Msg("transfer job published")
	}

	return transfer, created, nil
}

func (s *Service) GetTransfer(ctx context.Context, id string) (Transfer, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Transfer{}, ValidationError{Field: "id", Message: "is required"}
	}

	if _, err := uuid.Parse(id); err != nil {
		return Transfer{}, ValidationError{Field: "id", Message: "must be a valid UUID"}
	}

	return s.repository.GetByID(ctx, id)
}

func (s *Service) ListTransfers(ctx context.Context, input ListInput) (ListResult, error) {
	params := ListParams{
		Limit:  input.Limit,
		Offset: input.Offset,
	}

	if params.Limit <= 0 {
		params.Limit = 20
	}

	if params.Limit > 100 {
		params.Limit = 100
	}

	if params.Offset < 0 {
		return ListResult{}, ValidationError{Field: "offset", Message: "must be greater than or equal to 0"}
	}

	items, err := s.repository.List(ctx, params)
	if err != nil {
		return ListResult{}, err
	}

	return ListResult{
		Items:  items,
		Limit:  params.Limit,
		Offset: params.Offset,
	}, nil
}

func normalizeCreateInput(input CreateInput) (CreateParams, error) {
	params := CreateParams{
		IdempotencyKey:     strings.TrimSpace(input.IdempotencyKey),
		Chain:              strings.TrimSpace(input.Chain),
		AssetType:          strings.TrimSpace(input.AssetType),
		SourceWalletID:     strings.TrimSpace(input.SourceWalletID),
		DestinationAddress: strings.TrimSpace(input.DestinationAddress),
		Amount:             strings.TrimSpace(input.Amount),
		CallbackURL:        strings.TrimSpace(input.CallbackURL),
		MetadataJSON:       input.MetadataJSON,
		Status:             StatusCreated,
	}

	if params.IdempotencyKey == "" {
		return CreateParams{}, ValidationError{Field: "idempotency_key", Message: "is required"}
	}

	if params.Chain == "" {
		return CreateParams{}, ValidationError{Field: "chain", Message: "is required"}
	}

	if params.AssetType == "" {
		return CreateParams{}, ValidationError{Field: "asset_type", Message: "is required"}
	}

	if params.SourceWalletID == "" {
		return CreateParams{}, ValidationError{Field: "source_wallet_id", Message: "is required"}
	}

	if params.DestinationAddress == "" {
		return CreateParams{}, ValidationError{Field: "destination_address", Message: "is required"}
	}

	if !common.IsHexAddress(params.DestinationAddress) {
		return CreateParams{}, ValidationError{Field: "destination_address", Message: "must be a valid EVM address"}
	}

	if params.Amount == "" {
		return CreateParams{}, ValidationError{Field: "amount", Message: "is required"}
	}

	amount, ok := new(big.Int).SetString(params.Amount, 10)
	if !ok || amount.Sign() <= 0 {
		return CreateParams{}, ValidationError{Field: "amount", Message: "must be a positive integer string"}
	}

	if params.CallbackURL != "" {
		parsedURL, err := url.Parse(params.CallbackURL)
		if err != nil || parsedURL.Scheme == "" || parsedURL.Host == "" {
			return CreateParams{}, ValidationError{Field: "callback_url", Message: "must be a valid URL"}
		}

		if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
			return CreateParams{}, ValidationError{Field: "callback_url", Message: "must use http or https"}
		}
	}

	if len(params.MetadataJSON) == 0 {
		params.MetadataJSON = json.RawMessage(`{}`)
	}

	var metadata map[string]any
	if err := json.Unmarshal(params.MetadataJSON, &metadata); err != nil {
		return CreateParams{}, ValidationError{Field: "metadata_json", Message: "must be a valid JSON object"}
	}

	if metadata == nil {
		return CreateParams{}, ValidationError{Field: "metadata_json", Message: "must be a valid JSON object"}
	}

	normalizedMetadata, err := json.Marshal(metadata)
	if err != nil {
		return CreateParams{}, fmt.Errorf("marshal metadata_json: %w", err)
	}

	params.MetadataJSON = normalizedMetadata

	return params, nil
}
