package reconciliation

import (
	"context"
	"testing"

	"aegis/internal/modules/transfers"

	"github.com/rs/zerolog"
)

func TestServiceRunRecordsMismatch(t *testing.T) {
	repo := &stubRepository{
		candidates: []CandidateTransfer{{
			TransferID:     "transfer-1",
			Chain:          "ethereum",
			TxHash:         "0xabc",
			InternalStatus: transfers.StatusPendingOnChain,
		}},
	}
	checker := &stubChecker{status: ReceiptStatusConfirmed}

	service := NewService(repo, checker, zerolog.Nop())

	result, err := service.Run(context.Background(), "manual")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if result.MismatchCount != 1 {
		t.Fatalf("expected one mismatch, got %+v", result)
	}

	if repo.created[0].Notes == "" {
		t.Fatal("expected mismatch notes to be stored")
	}
}

func TestServiceRunRecordsMatch(t *testing.T) {
	repo := &stubRepository{
		candidates: []CandidateTransfer{{
			TransferID:     "transfer-2",
			Chain:          "ethereum",
			TxHash:         "0xdef",
			InternalStatus: transfers.StatusConfirmed,
		}},
	}
	checker := &stubChecker{status: ReceiptStatusConfirmed}

	service := NewService(repo, checker, zerolog.Nop())

	result, err := service.Run(context.Background(), "scheduled")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if result.MatchedCount != 1 {
		t.Fatalf("expected one match, got %+v", result)
	}
}

type stubRepository struct {
	candidates []CandidateTransfer
	created    []CreateResultParams
	results    []Result
}

func (s *stubRepository) ListCandidates(context.Context) ([]CandidateTransfer, error) {
	return s.candidates, nil
}

func (s *stubRepository) CreateResult(_ context.Context, params CreateResultParams) (Result, error) {
	s.created = append(s.created, params)
	return Result{
		TransferRequestID: params.TransferRequestID,
		InternalStatus:    params.InternalStatus,
		BlockchainStatus:  params.BlockchainStatus,
		IsMismatch:        params.IsMismatch,
		Notes:             params.Notes,
		TriggerSource:     params.TriggerSource,
	}, nil
}

func (s *stubRepository) ListLatestMismatches(context.Context) ([]Result, error) {
	return s.results, nil
}

type stubChecker struct {
	status ReceiptStatus
}

func (s *stubChecker) CheckReceipt(context.Context, string, string) (ReceiptStatus, error) {
	return s.status, nil
}
