package transfers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

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

const attemptSelectColumns = `
	id::text,
	transfer_request_id::text,
	nonce,
	raw_payload::text,
	COALESCE(transaction_hash, ''),
	status,
	COALESCE(last_error, ''),
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

	jobPayload, err := json.Marshal(TransferJob{
		TransferID: transfer.ID,
		Attempt:    0,
	})
	if err != nil {
		return Transfer{}, false, fmt.Errorf("marshal transfer outbox payload: %w", err)
	}

	if _, err := tx.Exec(
		ctx,
		`INSERT INTO transfer_outbox (
			transfer_request_id,
			event_type,
			payload_json,
			status,
			available_at
		) VALUES ($1, $2, $3, $4, NOW())`,
		transfer.ID,
		OutboxEventTypeTransferRequested,
		jobPayload,
		OutboxStatusPending,
	); err != nil {
		return Transfer{}, false, fmt.Errorf("insert transfer outbox event: %w", err)
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

func (r *PostgresRepository) GetLatestAttempt(ctx context.Context, transferID string) (TransactionAttempt, error) {
	attempt, err := scanAttempt(r.pool.QueryRow(
		ctx,
		`SELECT `+attemptSelectColumns+`
		FROM transaction_attempts
		WHERE transfer_request_id = $1
		ORDER BY created_at DESC
		LIMIT 1`,
		transferID,
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return TransactionAttempt{}, ErrTransactionAttemptNotFound
		}

		return TransactionAttempt{}, fmt.Errorf("get latest transaction attempt: %w", err)
	}

	return attempt, nil
}

func (r *PostgresRepository) CreateAttempt(ctx context.Context, params CreateAttemptParams) (TransactionAttempt, error) {
	attempt, err := scanAttempt(r.pool.QueryRow(
		ctx,
		`INSERT INTO transaction_attempts (
			transfer_request_id,
			transaction_hash,
			nonce,
			status,
			raw_payload,
			last_error
		) VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING `+attemptSelectColumns,
		params.TransferID,
		params.TxHash,
		params.Nonce,
		params.Status,
		params.RawPayload,
		params.ErrorMessage,
	))
	if err != nil {
		return TransactionAttempt{}, fmt.Errorf("create transaction attempt: %w", err)
	}

	return attempt, nil
}

func (r *PostgresRepository) UpdateAttempt(ctx context.Context, params UpdateAttemptParams) (TransactionAttempt, error) {
	attempt, err := scanAttempt(r.pool.QueryRow(
		ctx,
		`UPDATE transaction_attempts
		SET status = $2,
			last_error = $3,
			updated_at = NOW()
		WHERE id = $1
		RETURNING `+attemptSelectColumns,
		params.AttemptID,
		params.Status,
		params.ErrorMessage,
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return TransactionAttempt{}, ErrTransactionAttemptNotFound
		}

		return TransactionAttempt{}, fmt.Errorf("update transaction attempt: %w", err)
	}

	return attempt, nil
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

func (r *PostgresRepository) ClaimPendingOutbox(ctx context.Context, batchSize int, staleBefore time.Time) ([]OutboxEvent, error) {
	if batchSize <= 0 {
		batchSize = 25
	}

	rows, err := r.pool.Query(
		ctx,
		`WITH candidates AS (
			SELECT id
			FROM transfer_outbox
			WHERE (
				status IN ($1, $2)
				AND available_at <= NOW()
			) OR (
				status = $3
				AND locked_at IS NOT NULL
				AND locked_at <= $4
			)
			ORDER BY available_at ASC, created_at ASC
			LIMIT $5
			FOR UPDATE SKIP LOCKED
		)
		UPDATE transfer_outbox AS outbox
		SET status = $3,
			locked_at = NOW(),
			updated_at = NOW()
		FROM candidates
		WHERE outbox.id = candidates.id
		RETURNING
			outbox.id::text,
			outbox.transfer_request_id::text,
			outbox.event_type,
			outbox.payload_json::text,
			outbox.status,
			outbox.attempt_count,
			outbox.available_at,
			outbox.locked_at,
			outbox.dispatched_at,
			COALESCE(outbox.last_error, ''),
			outbox.created_at,
			outbox.updated_at`,
		OutboxStatusPending,
		OutboxStatusRetry,
		OutboxStatusProcessing,
		staleBefore,
		batchSize,
	)
	if err != nil {
		return nil, fmt.Errorf("claim pending transfer outbox: %w", err)
	}
	defer rows.Close()

	events := make([]OutboxEvent, 0, batchSize)
	for rows.Next() {
		event, scanErr := scanOutboxEvent(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan claimed transfer outbox event: %w", scanErr)
		}

		events = append(events, event)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate claimed transfer outbox events: %w", err)
	}

	return events, nil
}

func (r *PostgresRepository) MarkOutboxDispatched(ctx context.Context, outboxID string, attemptCount int) error {
	if _, err := r.pool.Exec(
		ctx,
		`UPDATE transfer_outbox
		SET status = $2,
			attempt_count = GREATEST(attempt_count, $3),
			dispatched_at = COALESCE(dispatched_at, NOW()),
			locked_at = NULL,
			last_error = NULL,
			updated_at = NOW()
		WHERE id = $1 AND status <> $2`,
		outboxID,
		OutboxStatusDispatched,
		attemptCount,
	); err != nil {
		return fmt.Errorf("mark transfer outbox dispatched: %w", err)
	}

	return nil
}

func (r *PostgresRepository) MarkOutboxRetry(ctx context.Context, outboxID string, attemptCount int, nextAvailableAt time.Time, lastError string) error {
	if _, err := r.pool.Exec(
		ctx,
		`UPDATE transfer_outbox
		SET status = $2,
			attempt_count = GREATEST(attempt_count, $3),
			available_at = $4,
			locked_at = NULL,
			last_error = $5,
			updated_at = NOW()
		WHERE id = $1 AND status <> $6`,
		outboxID,
		OutboxStatusRetry,
		attemptCount,
		nextAvailableAt,
		lastError,
		OutboxStatusDispatched,
	); err != nil {
		return fmt.Errorf("mark transfer outbox retry: %w", err)
	}

	return nil
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

func scanOutboxEvent(scanner rowScanner) (OutboxEvent, error) {
	var event OutboxEvent
	var payload string

	if err := scanner.Scan(
		&event.ID,
		&event.TransferID,
		&event.EventType,
		&payload,
		&event.Status,
		&event.AttemptCount,
		&event.AvailableAt,
		&event.LockedAt,
		&event.DispatchedAt,
		&event.LastError,
		&event.CreatedAt,
		&event.UpdatedAt,
	); err != nil {
		return OutboxEvent{}, err
	}

	event.PayloadJSON = json.RawMessage(payload)

	return event, nil
}

func scanAttempt(scanner rowScanner) (TransactionAttempt, error) {
	var attempt TransactionAttempt
	var rawPayload string

	if err := scanner.Scan(
		&attempt.ID,
		&attempt.TransferID,
		&attempt.Nonce,
		&rawPayload,
		&attempt.TxHash,
		&attempt.Status,
		&attempt.ErrorMessage,
		&attempt.CreatedAt,
		&attempt.UpdatedAt,
	); err != nil {
		return TransactionAttempt{}, err
	}

	attempt.RawPayload = json.RawMessage(rawPayload)

	return attempt, nil
}
