package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// txContextKey is an unexported type so only this package can set or
// read the transaction stored in a context.Context, preventing
// collisions with other packages' context values.
type txContextKey struct{}

// txFromContext returns the pgx.Tx stored in ctx by WithinTx, if any.
func txFromContext(ctx context.Context) (pgx.Tx, bool) {
	tx, ok := ctx.Value(txContextKey{}).(pgx.Tx)
	return tx, ok
}

// TxManager runs usecase.TxManager.WithinTx against a pgxpool.Pool,
// storing the active transaction in the context so repositories (via
// querier) transparently execute within it.
type TxManager struct {
	pool *pgxpool.Pool
}

// NewTxManager builds a TxManager backed by pool.
func NewTxManager(pool *pgxpool.Pool) *TxManager {
	return &TxManager{pool: pool}
}

// WithinTx begins a transaction, runs fn with the transaction stored in
// ctx, and commits on success. It rolls back and returns fn's error on
// failure, and rolls back then re-panics if fn panics.
func (m *TxManager) WithinTx(ctx context.Context, fn func(ctx context.Context) error) error {
	tx, err := m.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}

	txCtx := context.WithValue(ctx, txContextKey{}, tx)

	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback(ctx)
			panic(p)
		}
	}()

	if err := fn(txCtx); err != nil {
		if rbErr := tx.Rollback(ctx); rbErr != nil && rbErr != pgx.ErrTxClosed {
			return fmt.Errorf("%w (rollback also failed: %v)", err, rbErr)
		}
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}
