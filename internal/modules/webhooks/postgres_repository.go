package webhooks

import (
	"context"
	"encoding/json"
	"fmt"

	"aegis/internal/modules/transfers"

	"github.com/jackc/pgx/v5/pgxpool"
)

const deliverySelectColumns = `
	id::text,
	transfer_request_id::text,
	transfer_status_history_id::text,
	target_url,
	event_type,
	transfer_status,
	payload_json::text,
	delivery_status,
	attempt_count,
	max_attempts,
	COALESCE(response_status_code, 0),
	response_body,
	COALESCE(last_error, ''),
	next_attempt_at,
	delivered_at,
	created_at,
	updated_at
`

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) ScheduleTransferStatusDeliveries(ctx context.Context, maxAttempts int) (int64, error) {
	commandTag, err := r.pool.Exec(
		ctx,
		`INSERT INTO webhook_deliveries (
			transfer_request_id,
			transfer_status_history_id,
			target_url,
			event_type,
			transfer_status,
			payload_json,
			delivery_status,
			attempt_count,
			max_attempts,
			next_attempt_at
		)
		SELECT
			tr.id,
			tsh.id,
			tr.callback_url,
			'transfer.status.updated',
			tsh.to_status,
			jsonb_build_object(
				'transfer_id', tr.id,
				'status', tsh.to_status,
				'chain', tr.chain,
				'tx_hash', tr.tx_hash,
				'idempotency_key', tr.idempotency_key,
				'amount', tr.amount::text,
				'source_wallet_id', tr.source_wallet_id,
				'destination_address', tr.destination_address,
				'updated_at', tr.updated_at,
				'status_changed_at', tsh.created_at
			),
			'PENDING',
			0,
			$1,
			NOW()
		FROM transfer_status_history AS tsh
		INNER JOIN transfer_requests AS tr ON tr.id = tsh.transfer_request_id
		WHERE tsh.to_status = ANY($2)
			AND tr.callback_url <> ''
		ON CONFLICT DO NOTHING`,
		maxAttempts,
		[]string{
			transfers.StatusSubmitted,
			transfers.StatusConfirmed,
			transfers.StatusFailed,
		},
	)
	if err != nil {
		return 0, fmt.Errorf("schedule webhook deliveries: %w", err)
	}

	return commandTag.RowsAffected(), nil
}

func (r *PostgresRepository) ListDueDeliveries(ctx context.Context, limit int) ([]Delivery, error) {
	rows, err := r.pool.Query(
		ctx,
		`SELECT `+deliverySelectColumns+`
		FROM webhook_deliveries
		WHERE delivery_status IN ('PENDING', 'RETRYING')
			AND next_attempt_at IS NOT NULL
			AND next_attempt_at <= NOW()
		ORDER BY next_attempt_at ASC, created_at ASC
		LIMIT $1`,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list due webhook deliveries: %w", err)
	}
	defer rows.Close()

	deliveries := make([]Delivery, 0)
	for rows.Next() {
		delivery, err := scanDelivery(rows)
		if err != nil {
			return nil, fmt.Errorf("scan webhook delivery: %w", err)
		}

		deliveries = append(deliveries, delivery)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate webhook deliveries: %w", err)
	}

	return deliveries, nil
}

func (r *PostgresRepository) MarkDelivered(ctx context.Context, params MarkDeliveredParams) error {
	if _, err := r.pool.Exec(
		ctx,
		`UPDATE webhook_deliveries
		SET delivery_status = 'DELIVERED',
			attempt_count = $2,
			response_status_code = $3,
			response_body = $4,
			last_error = '',
			next_attempt_at = NULL,
			delivered_at = NOW(),
			updated_at = NOW()
		WHERE id = $1`,
		params.ID,
		params.AttemptCount,
		params.ResponseStatusCode,
		params.ResponseBody,
	); err != nil {
		return fmt.Errorf("mark webhook delivered: %w", err)
	}

	return nil
}

func (r *PostgresRepository) MarkRetry(ctx context.Context, params MarkRetryParams) error {
	if _, err := r.pool.Exec(
		ctx,
		`UPDATE webhook_deliveries
		SET delivery_status = 'RETRYING',
			attempt_count = $2,
			response_status_code = NULLIF($3, 0),
			response_body = $4,
			last_error = $5,
			next_attempt_at = $6,
			delivered_at = NULL,
			updated_at = NOW()
		WHERE id = $1`,
		params.ID,
		params.AttemptCount,
		params.ResponseStatusCode,
		params.ResponseBody,
		params.LastError,
		params.NextAttemptAt,
	); err != nil {
		return fmt.Errorf("mark webhook retry: %w", err)
	}

	return nil
}

func (r *PostgresRepository) MarkFailed(ctx context.Context, params MarkFailedParams) error {
	if _, err := r.pool.Exec(
		ctx,
		`UPDATE webhook_deliveries
		SET delivery_status = 'FAILED',
			attempt_count = $2,
			response_status_code = NULLIF($3, 0),
			response_body = $4,
			last_error = $5,
			next_attempt_at = NULL,
			delivered_at = NULL,
			updated_at = NOW()
		WHERE id = $1`,
		params.ID,
		params.AttemptCount,
		params.ResponseStatusCode,
		params.ResponseBody,
		params.LastError,
	); err != nil {
		return fmt.Errorf("mark webhook failed: %w", err)
	}

	return nil
}

type deliveryScanner interface {
	Scan(dest ...any) error
}

func scanDelivery(scanner deliveryScanner) (Delivery, error) {
	var delivery Delivery
	var payload string

	if err := scanner.Scan(
		&delivery.ID,
		&delivery.TransferRequestID,
		&delivery.TransferStatusHistoryID,
		&delivery.TargetURL,
		&delivery.EventType,
		&delivery.TransferStatus,
		&payload,
		&delivery.DeliveryStatus,
		&delivery.AttemptCount,
		&delivery.MaxAttempts,
		&delivery.ResponseStatusCode,
		&delivery.ResponseBody,
		&delivery.LastError,
		&delivery.NextAttemptAt,
		&delivery.DeliveredAt,
		&delivery.CreatedAt,
		&delivery.UpdatedAt,
	); err != nil {
		return Delivery{}, err
	}

	if payload == "" {
		delivery.PayloadJSON = json.RawMessage(`{}`)
	} else {
		delivery.PayloadJSON = json.RawMessage(payload)
	}

	return delivery, nil
}
