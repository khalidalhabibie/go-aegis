package wallets

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const walletSelectColumns = `
	id::text,
	chain,
	address,
	label,
	signing_type,
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

func (r *PostgresRepository) Create(ctx context.Context, params CreateParams) (Wallet, error) {
	wallet, err := scanWallet(r.pool.QueryRow(
		ctx,
		`INSERT INTO wallets (
			chain,
			address,
			label,
			signing_type,
			status
		) VALUES ($1, $2, $3, $4, $5)
		RETURNING `+walletSelectColumns,
		params.Chain,
		params.Address,
		params.Label,
		params.SigningType,
		params.Status,
	))
	if err != nil {
		var pgError *pgconn.PgError
		if errors.As(err, &pgError) && pgError.Code == "23505" {
			return Wallet{}, ErrDuplicateActiveWallet
		}

		return Wallet{}, fmt.Errorf("create wallet: %w", err)
	}

	return wallet, nil
}

func (r *PostgresRepository) GetByID(ctx context.Context, id string) (Wallet, error) {
	wallet, err := scanWallet(r.pool.QueryRow(
		ctx,
		`SELECT `+walletSelectColumns+` FROM wallets WHERE id = $1`,
		id,
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Wallet{}, ErrWalletNotFound
		}

		return Wallet{}, fmt.Errorf("get wallet by id: %w", err)
	}

	return wallet, nil
}

func (r *PostgresRepository) List(ctx context.Context) ([]Wallet, error) {
	rows, err := r.pool.Query(
		ctx,
		`SELECT `+walletSelectColumns+`
		FROM wallets
		ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list wallets: %w", err)
	}
	defer rows.Close()

	wallets := make([]Wallet, 0)
	for rows.Next() {
		wallet, err := scanWallet(rows)
		if err != nil {
			return nil, fmt.Errorf("scan listed wallet: %w", err)
		}

		wallets = append(wallets, wallet)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate listed wallets: %w", err)
	}

	return wallets, nil
}

type walletRowScanner interface {
	Scan(dest ...any) error
}

func scanWallet(scanner walletRowScanner) (Wallet, error) {
	var wallet Wallet

	if err := scanner.Scan(
		&wallet.ID,
		&wallet.Chain,
		&wallet.Address,
		&wallet.Label,
		&wallet.SigningType,
		&wallet.Status,
		&wallet.CreatedAt,
		&wallet.UpdatedAt,
	); err != nil {
		return Wallet{}, err
	}

	return wallet, nil
}
