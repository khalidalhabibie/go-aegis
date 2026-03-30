package reconciliation

import (
	"context"
	"fmt"

	"aegis/internal/modules/transfers"

	"github.com/rs/zerolog"
)

type Service struct {
	repository Repository
	checker    ReceiptChecker
	log        zerolog.Logger
}

func NewService(repository Repository, checker ReceiptChecker, log zerolog.Logger) *Service {
	return &Service{
		repository: repository,
		checker:    checker,
		log:        log,
	}
}

func (s *Service) Run(ctx context.Context, triggerSource string) (RunResult, error) {
	if triggerSource == "" {
		triggerSource = "manual"
	}

	candidates, err := s.repository.ListCandidates(ctx)
	if err != nil {
		return RunResult{}, err
	}

	result := RunResult{}
	for _, candidate := range candidates {
		receiptStatus, err := s.checker.CheckReceipt(ctx, candidate.Chain, candidate.TxHash)
		if err != nil {
			return RunResult{}, fmt.Errorf("check receipt for transfer %s: %w", candidate.TransferID, err)
		}

		isMismatch, notes := compareStatuses(candidate.InternalStatus, receiptStatus)
		if _, err := s.repository.CreateResult(ctx, CreateResultParams{
			TransferRequestID: candidate.TransferID,
			TxHash:            candidate.TxHash,
			InternalStatus:    candidate.InternalStatus,
			BlockchainStatus:  receiptStatus,
			IsMismatch:        isMismatch,
			Notes:             notes,
			TriggerSource:     triggerSource,
		}); err != nil {
			return RunResult{}, err
		}

		result.CheckedCount++
		if isMismatch {
			result.MismatchCount++
			s.log.Warn().
				Str("transfer_id", candidate.TransferID).
				Str("internal_status", candidate.InternalStatus).
				Str("blockchain_status", string(receiptStatus)).
				Str("notes", notes).
				Msg("reconciliation mismatch detected")
		} else {
			result.MatchedCount++
		}
	}

	s.log.Info().
		Int("checked_count", result.CheckedCount).
		Int("matched_count", result.MatchedCount).
		Int("mismatch_count", result.MismatchCount).
		Str("trigger_source", triggerSource).
		Msg("reconciliation run completed")

	return result, nil
}

func (s *Service) ListLatestMismatches(ctx context.Context) ([]Result, error) {
	return s.repository.ListLatestMismatches(ctx)
}

func compareStatuses(internalStatus string, blockchainStatus ReceiptStatus) (bool, string) {
	switch internalStatus {
	case transfers.StatusSubmitted, transfers.StatusPendingOnChain:
		switch blockchainStatus {
		case ReceiptStatusPending, ReceiptStatusNotFound:
			return false, ""
		case ReceiptStatusConfirmed:
			return true, fmt.Sprintf("internal status %s but blockchain receipt is CONFIRMED", internalStatus)
		case ReceiptStatusFailed:
			return true, fmt.Sprintf("internal status %s but blockchain receipt is FAILED", internalStatus)
		}
	case transfers.StatusConfirmed:
		if blockchainStatus == ReceiptStatusConfirmed {
			return false, ""
		}
		return true, fmt.Sprintf("internal status CONFIRMED but blockchain receipt is %s", blockchainStatus)
	case transfers.StatusFailed:
		switch blockchainStatus {
		case ReceiptStatusFailed, ReceiptStatusNotFound:
			return false, ""
		case ReceiptStatusPending, ReceiptStatusConfirmed:
			return true, fmt.Sprintf("internal status FAILED but blockchain receipt is %s", blockchainStatus)
		}
	}

	return false, ""
}
