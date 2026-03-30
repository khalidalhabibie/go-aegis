package wallets

import (
	"context"
	"errors"
	"fmt"
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
	log        zerolog.Logger
}

type CreateInput struct {
	Chain       string
	Address     string
	Label       string
	SigningType string
	Status      string
}

func NewService(repository Repository, log zerolog.Logger) *Service {
	return &Service{
		repository: repository,
		log:        log,
	}
}

func (s *Service) CreateWallet(ctx context.Context, input CreateInput) (Wallet, error) {
	params, err := normalizeCreateInput(input)
	if err != nil {
		return Wallet{}, err
	}

	wallet, err := s.repository.Create(ctx, params)
	if err != nil {
		return Wallet{}, err
	}

	s.log.Info().
		Str("wallet_id", wallet.ID).
		Str("chain", wallet.Chain).
		Str("address", wallet.Address).
		Str("status", wallet.Status).
		Msg("wallet registered")

	return wallet, nil
}

func (s *Service) GetWallet(ctx context.Context, id string) (Wallet, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Wallet{}, ValidationError{Field: "id", Message: "is required"}
	}

	if _, err := uuid.Parse(id); err != nil {
		return Wallet{}, ValidationError{Field: "id", Message: "must be a valid UUID"}
	}

	return s.repository.GetByID(ctx, id)
}

func (s *Service) ListWallets(ctx context.Context) ([]Wallet, error) {
	return s.repository.List(ctx)
}

func normalizeCreateInput(input CreateInput) (CreateParams, error) {
	params := CreateParams{
		Chain:       strings.ToLower(strings.TrimSpace(input.Chain)),
		Address:     strings.TrimSpace(input.Address),
		Label:       strings.TrimSpace(input.Label),
		SigningType: strings.ToLower(strings.TrimSpace(input.SigningType)),
		Status:      strings.ToUpper(strings.TrimSpace(input.Status)),
	}

	if params.Chain == "" {
		return CreateParams{}, ValidationError{Field: "chain", Message: "is required"}
	}

	if params.Address == "" {
		return CreateParams{}, ValidationError{Field: "address", Message: "is required"}
	}

	if !common.IsHexAddress(params.Address) {
		return CreateParams{}, ValidationError{Field: "address", Message: "must be a valid EVM address"}
	}

	params.Address = common.HexToAddress(params.Address).Hex()

	if params.Label == "" {
		return CreateParams{}, ValidationError{Field: "label", Message: "is required"}
	}

	if params.SigningType == "" {
		return CreateParams{}, ValidationError{Field: "signing_type", Message: "is required"}
	}

	if params.Status == "" {
		params.Status = StatusActive
	}

	switch params.Status {
	case StatusActive, StatusInactive:
	default:
		return CreateParams{}, ValidationError{Field: "status", Message: "must be ACTIVE or INACTIVE"}
	}

	return params, nil
}
