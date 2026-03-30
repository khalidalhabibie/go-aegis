package reconciliation

import (
	"context"
	"fmt"

	"aegis/internal/modules/transfers"

	"github.com/jackc/pgx/v5/pgxpool"
)

const reconciliationSelectColumns = `
	id::text,
	transfer_request_id::text,
	tx_hash,
	internal_status,
	blockchain_status,
	is_mismatch,
	notes,
	checked_at,
	trigger_source
`

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) ListCandidates(ctx context.Context) ([]CandidateTransfer, error) {
	rows, err := r.pool.Query(
		ctx,
		`SELECT
			id::text,
			chain,
			tx_hash,
			status
		FROM transfer_requests
		WHERE tx_hash <> ''
			AND status = ANY($1)
		ORDER BY updated_at DESC`,
		[]string{
			transfers.StatusSubmitted,
			transfers.StatusPendingOnChain,
			transfers.StatusConfirmed,
			transfers.StatusFailed,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("list reconciliation candidates: %w", err)
	}
	defer rows.Close()

	candidates := make([]CandidateTransfer, 0)
	for rows.Next() {
		var candidate CandidateTransfer
		if err := rows.Scan(&candidate.TransferID, &candidate.Chain, &candidate.TxHash, &candidate.InternalStatus); err != nil {
			return nil, fmt.Errorf("scan reconciliation candidate: %w", err)
		}

		candidates = append(candidates, candidate)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate reconciliation candidates: %w", err)
	}

	return candidates, nil
}

func (r *PostgresRepository) CreateResult(ctx context.Context, params CreateResultParams) (Result, error) {
	result, err := scanResult(r.pool.QueryRow(
		ctx,
		`INSERT INTO reconciliation_results (
			transfer_request_id,
			tx_hash,
			internal_status,
			blockchain_status,
			is_mismatch,
			notes,
			trigger_source
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING `+reconciliationSelectColumns,
		params.TransferRequestID,
		params.TxHash,
		params.InternalStatus,
		params.BlockchainStatus,
		params.IsMismatch,
		params.Notes,
		params.TriggerSource,
	))
	if err != nil {
		return Result{}, fmt.Errorf("create reconciliation result: %w", err)
	}

	return result, nil
}

func (r *PostgresRepository) ListLatestMismatches(ctx context.Context) ([]Result, error) {
	rows, err := r.pool.Query(
		ctx,
		`WITH latest AS (
			SELECT DISTINCT ON (transfer_request_id)
				`+reconciliationSelectColumns+`
			FROM reconciliation_results
			ORDER BY transfer_request_id, checked_at DESC, created_at DESC
		)
		SELECT
			id,
			transfer_request_id,
			tx_hash,
			internal_status,
			blockchain_status,
			is_mismatch,
			notes,
			checked_at,
			trigger_source
		FROM latest
		WHERE is_mismatch = true
		ORDER BY checked_at DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list reconciliation mismatches: %w", err)
	}
	defer rows.Close()

	results := make([]Result, 0)
	for rows.Next() {
		result, err := scanResult(rows)
		if err != nil {
			return nil, fmt.Errorf("scan reconciliation mismatch: %w", err)
		}

		results = append(results, result)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate reconciliation mismatches: %w", err)
	}

	return results, nil
}

type resultScanner interface {
	Scan(dest ...any) error
}

func scanResult(scanner resultScanner) (Result, error) {
	var result Result
	var blockchainStatus string

	if err := scanner.Scan(
		&result.ID,
		&result.TransferRequestID,
		&result.TxHash,
		&result.InternalStatus,
		&blockchainStatus,
		&result.IsMismatch,
		&result.Notes,
		&result.CheckedAt,
		&result.TriggerSource,
	); err != nil {
		return Result{}, err
	}

	result.BlockchainStatus = ReceiptStatus(blockchainStatus)

	return result, nil
}
