package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Querier is the subset of pgx.Tx / pgxpool.Pool that repositories need.
// Depending on each other through this interface lets repositories run
// either inside a transaction or directly against the pool without
// knowing which.
type Querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// querier returns the transaction stored in ctx by TxManager.WithinTx,
// or pool if there is none. Repositories call this at the start of every
// method so they transparently participate in an ambient transaction.
func querier(ctx context.Context, pool *pgxpool.Pool) Querier {
	if tx, ok := txFromContext(ctx); ok {
		return tx
	}
	return pool
}
