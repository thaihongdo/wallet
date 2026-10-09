package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/thaihongdo/go-wallet/internal/domain"
)

// pgUniqueViolation is the Postgres SQLSTATE for a unique constraint
// violation (23505).
const pgUniqueViolation = "23505"

// OperationRepo implements usecase.OperationRepository on Postgres.
type OperationRepo struct {
	pool *pgxpool.Pool
}

// NewOperationRepo builds an OperationRepo backed by pool.
func NewOperationRepo(pool *pgxpool.Pool) *OperationRepo {
	return &OperationRepo{pool: pool}
}

// Create inserts a new operation row. Returns
// domain.ErrDuplicateIdempotencyKey if idempotency_key already exists.
func (r *OperationRepo) Create(ctx context.Context, op *domain.Operation) error {
	q := querier(ctx, r.pool)
	_, err := q.Exec(ctx, `
		INSERT INTO operations (id, type, idempotency_key, from_wallet_id, to_wallet_id, amount, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		op.ID, string(op.Type), op.IdempotencyKey, op.FromWalletID, op.ToWalletID, int64(op.Amount), op.CreatedAt,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation && pgErr.ConstraintName == "uq_operations_idem" {
			return fmt.Errorf("insert operation: %w", domain.ErrDuplicateIdempotencyKey)
		}
		return fmt.Errorf("insert operation: %w", err)
	}
	return nil
}

// GetByIdempotencyKey returns the operation with the given idempotency
// key, or (nil, nil) if none exists.
func (r *OperationRepo) GetByIdempotencyKey(ctx context.Context, key string) (*domain.Operation, error) {
	q := querier(ctx, r.pool)
	row := q.QueryRow(ctx, `
		SELECT id, type, idempotency_key, from_wallet_id, to_wallet_id, amount, created_at
		FROM operations WHERE idempotency_key = $1`, key)

	var op domain.Operation
	var opType string
	var amount int64
	err := row.Scan(&op.ID, &opType, &op.IdempotencyKey, &op.FromWalletID, &op.ToWalletID, &amount, &op.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get operation by idempotency key: %w", err)
	}
	op.Type = domain.OperationType(opType)
	op.Amount = domain.Money(amount)
	return &op, nil
}
