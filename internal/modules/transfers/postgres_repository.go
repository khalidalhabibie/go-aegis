package transfers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const transferSelectColumns = `
	id::text,
	idempotency_key,
	chain,
	asset_type,
	source_wallet_id,
	destination_address,
	amount::text,
	callback_url,
	metadata_json::text,
	tx_hash,
	status,
	created_at,
	updated_at
`

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) Create(ctx context.Context, params CreateParams) (Transfer, bool, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Transfer{}, false, fmt.Errorf("begin transfer create transaction: %w", err)
	}

	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(ctx)
		}
	}()

	query := `
		INSERT INTO transfer_requests (
			idempotency_key,
			chain,
			asset_type,
			source_wallet_id,
			destination_address,
			amount,
			callback_url,
			metadata_json,
			status
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (idempotency_key) DO NOTHING
		RETURNING ` + transferSelectColumns

	transfer, err := scanTransfer(tx.QueryRow(
		ctx,
		query,
		params.IdempotencyKey,
		params.Chain,
		params.AssetType,
		params.SourceWalletID,
		params.DestinationAddress,
		params.Amount,
		params.CallbackURL,
		params.MetadataJSON,
		params.Status,
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			existing, getErr := r.getByIdempotencyKey(ctx, params.IdempotencyKey)
			if getErr != nil {
				return Transfer{}, false, getErr
			}

			return existing, false, nil
		}

		return Transfer{}, false, fmt.Errorf("insert transfer request: %w", err)
	}

	if _, err := tx.Exec(
		ctx,
		`INSERT INTO transfer_status_history (transfer_request_id, from_status, to_status) VALUES ($1, $2, $3)`,
		transfer.ID,
		nil,
		transfer.Status,
	); err != nil {
		return Transfer{}, false, fmt.Errorf("insert transfer status history: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return Transfer{}, false, fmt.Errorf("commit transfer request: %w", err)
	}

	committed = true

	return transfer, true, nil
}

func (r *PostgresRepository) GetByID(ctx context.Context, id string) (Transfer, error) {
	transfer, err := scanTransfer(r.pool.QueryRow(
		ctx,
		`SELECT `+transferSelectColumns+` FROM transfer_requests WHERE id = $1`,
		id,
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Transfer{}, ErrTransferNotFound
		}

		return Transfer{}, fmt.Errorf("get transfer by id: %w", err)
	}

	return transfer, nil
}

func (r *PostgresRepository) List(ctx context.Context, params ListParams) ([]Transfer, error) {
	rows, err := r.pool.Query(
		ctx,
		`SELECT `+transferSelectColumns+`
		FROM transfer_requests
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2`,
		params.Limit,
		params.Offset,
	)
	if err != nil {
		return nil, fmt.Errorf("list transfers: %w", err)
	}
	defer rows.Close()

	transfers := make([]Transfer, 0, params.Limit)
	for rows.Next() {
		transfer, err := scanTransfer(rows)
		if err != nil {
			return nil, fmt.Errorf("scan listed transfer: %w", err)
		}

		transfers = append(transfers, transfer)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate listed transfers: %w", err)
	}

	return transfers, nil
}

func (r *PostgresRepository) TransitionStatus(ctx context.Context, params TransitionParams) (Transfer, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Transfer{}, fmt.Errorf("begin transfer transition transaction: %w", err)
	}

	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(ctx)
		}
	}()

	transfer, err := scanTransfer(tx.QueryRow(
		ctx,
		`UPDATE transfer_requests
		SET status = $3,
			tx_hash = CASE WHEN $4::text IS NULL THEN tx_hash ELSE $4 END,
			updated_at = NOW()
		WHERE id = $1 AND status = $2
		RETURNING `+transferSelectColumns,
		params.ID,
		params.FromStatus,
		params.ToStatus,
		params.TxHash,
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			current, getErr := r.GetByID(ctx, params.ID)
			if getErr != nil {
				return Transfer{}, getErr
			}

			return Transfer{}, InvalidStateError{
				TransferID: current.ID,
				Expected:   params.FromStatus,
				Actual:     current.Status,
			}
		}

		return Transfer{}, fmt.Errorf("update transfer status: %w", err)
	}

	if _, err := tx.Exec(
		ctx,
		`INSERT INTO transfer_status_history (transfer_request_id, from_status, to_status) VALUES ($1, $2, $3)`,
		transfer.ID,
		params.FromStatus,
		params.ToStatus,
	); err != nil {
		return Transfer{}, fmt.Errorf("insert transfer status transition: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return Transfer{}, fmt.Errorf("commit transfer status transition: %w", err)
	}

	committed = true

	return transfer, nil
}

func (r *PostgresRepository) getByIdempotencyKey(ctx context.Context, idempotencyKey string) (Transfer, error) {
	transfer, err := scanTransfer(r.pool.QueryRow(
		ctx,
		`SELECT `+transferSelectColumns+` FROM transfer_requests WHERE idempotency_key = $1`,
		idempotencyKey,
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Transfer{}, ErrTransferNotFound
		}

		return Transfer{}, fmt.Errorf("get transfer by idempotency key: %w", err)
	}

	return transfer, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanTransfer(scanner rowScanner) (Transfer, error) {
	var transfer Transfer
	var metadata string

	if err := scanner.Scan(
		&transfer.ID,
		&transfer.IdempotencyKey,
		&transfer.Chain,
		&transfer.AssetType,
		&transfer.SourceWalletID,
		&transfer.DestinationAddress,
		&transfer.Amount,
		&transfer.CallbackURL,
		&metadata,
		&transfer.TxHash,
		&transfer.Status,
		&transfer.CreatedAt,
		&transfer.UpdatedAt,
	); err != nil {
		return Transfer{}, err
	}

	if metadata == "" {
		transfer.MetadataJSON = json.RawMessage(`{}`)
	} else {
		transfer.MetadataJSON = json.RawMessage(metadata)
	}

	return transfer, nil
}
