package transfers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net"
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
	repository        Repository
	callbackURLPolicy CallbackURLPolicy
	log               zerolog.Logger
}

type CallbackURLPolicy struct {
	AllowedHosts        []string
	AllowPrivateTargets bool
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

func NewService(repository Repository, callbackURLPolicy CallbackURLPolicy, log zerolog.Logger) *Service {
	return &Service{
		repository:        repository,
		callbackURLPolicy: callbackURLPolicy,
		log:               log,
	}
}

func (s *Service) CreateTransfer(ctx context.Context, input CreateInput) (Transfer, bool, error) {
	params, err := normalizeCreateInput(input, s.callbackURLPolicy)
	if err != nil {
		return Transfer{}, false, err
	}

	transfer, created, err := s.repository.Create(ctx, params)
	if err != nil {
		return Transfer{}, false, err
	}

	s.log.Info().
		Str("transfer_id", transfer.ID).
		Str("status", transfer.Status).
		Bool("created", created).
		Msg("transfer request persisted")

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

func normalizeCreateInput(input CreateInput, callbackURLPolicy CallbackURLPolicy) (CreateParams, error) {
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

	if _, err := uuid.Parse(params.SourceWalletID); err != nil {
		return CreateParams{}, ValidationError{Field: "source_wallet_id", Message: "must be a valid UUID"}
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

		if parsedURL.User != nil {
			return CreateParams{}, ValidationError{Field: "callback_url", Message: "must not include user credentials"}
		}

		if err := validateCallbackURLTarget(parsedURL, callbackURLPolicy); err != nil {
			return CreateParams{}, err
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

func validateCallbackURLTarget(parsedURL *url.URL, policy CallbackURLPolicy) error {
	hostname := normalizeHostname(parsedURL.Hostname())
	if hostname == "" {
		return ValidationError{Field: "callback_url", Message: "must include a valid hostname"}
	}

	if len(policy.AllowedHosts) > 0 && !isAllowedCallbackHost(hostname, policy.AllowedHosts) {
		return ValidationError{Field: "callback_url", Message: "host is not in the allowed callback host list"}
	}

	if policy.AllowPrivateTargets {
		return nil
	}

	if isLocalOrPrivateCallbackHost(hostname) {
		return ValidationError{Field: "callback_url", Message: "must not target localhost or private network addresses"}
	}

	return nil
}

func isAllowedCallbackHost(hostname string, allowedHosts []string) bool {
	normalizedHost := normalizeHostname(hostname)
	for _, candidate := range allowedHosts {
		allowed := normalizeHostname(candidate)
		if allowed == "" {
			continue
		}

		if normalizedHost == allowed || strings.HasSuffix(normalizedHost, "."+allowed) {
			return true
		}
	}

	return false
}

func isLocalOrPrivateCallbackHost(hostname string) bool {
	normalizedHost := normalizeHostname(hostname)
	switch {
	case normalizedHost == "localhost":
		return true
	case strings.HasSuffix(normalizedHost, ".localhost"),
		strings.HasSuffix(normalizedHost, ".local"),
		strings.HasSuffix(normalizedHost, ".internal"):
		return true
	}

	ip := net.ParseIP(normalizedHost)
	if ip == nil {
		return false
	}

	return ip.IsLoopback() ||
		ip.IsPrivate() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsMulticast() ||
		ip.IsUnspecified()
}

func normalizeHostname(hostname string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(hostname)), ".")
}
