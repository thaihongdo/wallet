package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/thaihongdo/go-wallet/internal/domain"
)

// WalletRepo implements usecase.WalletRepository on Postgres.
type WalletRepo struct {
	pool *pgxpool.Pool
}

// NewWalletRepo builds a WalletRepo backed by pool.
func NewWalletRepo(pool *pgxpool.Pool) *WalletRepo {
	return &WalletRepo{pool: pool}
}

// Create inserts a new wallet row.
func (r *WalletRepo) Create(ctx context.Context, w *domain.Wallet) error {
	q := querier(ctx, r.pool)
	_, err := q.Exec(ctx, `
		INSERT INTO wallets (id, owner_id, currency, balance, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		w.ID, w.OwnerID, w.Currency, int64(w.Balance), w.CreatedAt, w.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert wallet: %w", err)
	}
	return nil
}

// GetByID returns the wallet with the given ID, or domain.ErrWalletNotFound.
func (r *WalletRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.Wallet, error) {
	q := querier(ctx, r.pool)
	row := q.QueryRow(ctx, `
		SELECT id, owner_id, currency, balance, created_at, updated_at
		FROM wallets WHERE id = $1`, id)
	return scanWallet(row)
}

// LockByIDs locks and returns the wallets for ids, in ascending ID
// order, via SELECT ... FOR UPDATE. Locking in a deterministic order
// regardless of caller-supplied order prevents deadlocks between
// concurrent transactions that lock the same pair of wallets in
// opposite roles (e.g. A->B and B->A transfers running at once).
func (r *WalletRepo) LockByIDs(ctx context.Context, ids []uuid.UUID) ([]*domain.Wallet, error) {
	q := querier(ctx, r.pool)
	rows, err := q.Query(ctx, `
		SELECT id, owner_id, currency, balance, created_at, updated_at
		FROM wallets WHERE id = ANY($1) ORDER BY id FOR UPDATE`, ids)
	if err != nil {
		return nil, fmt.Errorf("lock wallets: %w", err)
	}
	defer rows.Close()

	var out []*domain.Wallet
	for rows.Next() {
		w, err := scanWalletRows(rows)
		if err != nil {
			return nil, fmt.Errorf("lock wallets: %w", err)
		}
		out = append(out, w)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("lock wallets: %w", err)
	}
	return out, nil
}

// UpdateBalance persists a wallet's new balance and bumps updated_at.
func (r *WalletRepo) UpdateBalance(ctx context.Context, id uuid.UUID, balance domain.Money) error {
	q := querier(ctx, r.pool)
	tag, err := q.Exec(ctx, `
		UPDATE wallets SET balance = $2, updated_at = now() WHERE id = $1`,
		id, int64(balance),
	)
	if err != nil {
		return fmt.Errorf("update wallet balance: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("update wallet balance: %w", domain.ErrWalletNotFound)
	}
	return nil
}

// rowScanner is satisfied by both pgx.Row (QueryRow) and pgx.Rows (Query),
// letting a single scan helper serve both call sites.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanWallet(row pgx.Row) (*domain.Wallet, error) {
	w, err := scan(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrWalletNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan wallet: %w", err)
	}
	return w, nil
}

func scanWalletRows(rows pgx.Rows) (*domain.Wallet, error) {
	return scan(rows)
}

func scan(s rowScanner) (*domain.Wallet, error) {
	var w domain.Wallet
	var balance int64
	if err := s.Scan(&w.ID, &w.OwnerID, &w.Currency, &balance, &w.CreatedAt, &w.UpdatedAt); err != nil {
		return nil, err
	}
	w.Balance = domain.Money(balance)
	return &w, nil
}
