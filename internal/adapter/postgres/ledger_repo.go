package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/thaihongdo/go-wallet/internal/domain"
)

// LedgerRepo implements usecase.LedgerRepository on Postgres.
type LedgerRepo struct {
	pool *pgxpool.Pool
}

// NewLedgerRepo builds a LedgerRepo backed by pool.
func NewLedgerRepo(pool *pgxpool.Pool) *LedgerRepo {
	return &LedgerRepo{pool: pool}
}

// Insert appends one or more ledger entries in a single statement.
func (r *LedgerRepo) Insert(ctx context.Context, entries ...domain.LedgerEntry) error {
	if len(entries) == 0 {
		return nil
	}
	q := querier(ctx, r.pool)
	batch := make([][]any, len(entries))
	for i, e := range entries {
		batch[i] = []any{e.ID, e.OperationID, e.WalletID, string(e.Direction), int64(e.Amount), int64(e.BalanceAfter), e.CreatedAt}
	}
	_, err := q.Exec(ctx, `
		INSERT INTO ledger_entries (id, operation_id, wallet_id, direction, amount, balance_after, created_at)
		SELECT * FROM UNNEST($1::uuid[], $2::uuid[], $3::uuid[], $4::text[], $5::bigint[], $6::bigint[], $7::timestamptz[])`,
		columnOf(batch, 0), columnOf(batch, 1), columnOf(batch, 2), columnOf(batch, 3), columnOf(batch, 4), columnOf(batch, 5), columnOf(batch, 6),
	)
	if err != nil {
		return fmt.Errorf("insert ledger entries: %w", err)
	}
	return nil
}

// columnOf extracts column i from each row of batch as a slice, for use
// with pgx's array binding (one []any per column, UNNEST-style).
func columnOf(batch [][]any, i int) []any {
	out := make([]any, len(batch))
	for j, row := range batch {
		out[j] = row[i]
	}
	return out
}

// ListByWallet returns entries for walletID with id < *cursor (if
// cursor != nil), ordered by id descending, limited to limit rows.
func (r *LedgerRepo) ListByWallet(ctx context.Context, walletID uuid.UUID, cursor *uuid.UUID, limit int) ([]domain.LedgerEntry, error) {
	q := querier(ctx, r.pool)

	var rows pgx.Rows
	var err error
	if cursor != nil {
		rows, err = q.Query(ctx, `
			SELECT id, operation_id, wallet_id, direction, amount, balance_after, created_at
			FROM ledger_entries WHERE wallet_id = $1 AND id < $2 ORDER BY id DESC LIMIT $3`,
			walletID, *cursor, limit)
	} else {
		rows, err = q.Query(ctx, `
			SELECT id, operation_id, wallet_id, direction, amount, balance_after, created_at
			FROM ledger_entries WHERE wallet_id = $1 ORDER BY id DESC LIMIT $2`,
			walletID, limit)
	}
	if err != nil {
		return nil, fmt.Errorf("list ledger entries: %w", err)
	}
	defer rows.Close()

	var out []domain.LedgerEntry
	for rows.Next() {
		var e domain.LedgerEntry
		var direction string
		var amount, balanceAfter int64
		if err := rows.Scan(&e.ID, &e.OperationID, &e.WalletID, &direction, &amount, &balanceAfter, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan ledger entry: %w", err)
		}
		e.Direction = domain.Direction(direction)
		e.Amount = domain.Money(amount)
		e.BalanceAfter = domain.Money(balanceAfter)
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list ledger entries: %w", err)
	}
	return out, nil
}
