// Package usecase implements application business logic on top of the
// domain layer. It depends only on domain and the standard library:
// no gin, pgx, or redis imports are allowed here (enforced by
// `make arch-check`). All persistence and external-service dependencies
// are declared as interfaces in this file and implemented by adapters.
package usecase

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/thaihongdo/go-wallet/internal/domain"
)

// TxManager runs a function within a single database transaction.
type TxManager interface {
	// WithinTx runs fn in a DB transaction. The transaction travels inside
	// ctx (adapter-specific mechanism) so repositories called from fn
	// participate in it automatically. WithinTx commits if fn returns nil
	// and rolls back otherwise; on panic it rolls back and re-panics.
	WithinTx(ctx context.Context, fn func(ctx context.Context) error) error
}

// WalletRepository persists and retrieves Wallet entities.
type WalletRepository interface {
	// Create inserts a new wallet.
	Create(ctx context.Context, w *domain.Wallet) error
	// GetByID returns the wallet with the given ID, or ErrWalletNotFound.
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Wallet, error)
	// LockByIDs locks the rows for the given wallet IDs (e.g. via
	// `SELECT ... WHERE id = ANY($1) ORDER BY id FOR UPDATE`) and returns
	// them. Ordering the lock acquisition by ID makes lock order
	// deterministic across concurrent calls, which prevents deadlocks when
	// two transactions lock the same pair of wallets in opposite roles.
	LockByIDs(ctx context.Context, ids []uuid.UUID) ([]*domain.Wallet, error)
	// UpdateBalance persists a wallet's new balance.
	UpdateBalance(ctx context.Context, id uuid.UUID, balance domain.Money) error
}

// OperationRepository persists and retrieves Operation entities, which
// are the unit of idempotency for money-moving requests.
type OperationRepository interface {
	// Create inserts a new operation. Returns ErrDuplicateIdempotencyKey if
	// the idempotency key already exists (e.g. unique constraint violation).
	Create(ctx context.Context, op *domain.Operation) error
	// GetByIdempotencyKey returns the operation with the given idempotency
	// key, or (nil, nil) if none exists.
	GetByIdempotencyKey(ctx context.Context, key string) (*domain.Operation, error)
}

// LedgerRepository persists and lists LedgerEntry rows.
type LedgerRepository interface {
	// Insert appends one or more ledger entries.
	Insert(ctx context.Context, entries ...domain.LedgerEntry) error
	// ListByWallet returns entries for walletID with id < *cursor (if
	// cursor is non-nil), ordered by id descending, limited to limit rows.
	ListByWallet(ctx context.Context, walletID uuid.UUID, cursor *uuid.UUID, limit int) ([]domain.LedgerEntry, error)
}

// OutboxRepository persists OutboxEvent rows as part of the
// transactional outbox pattern.
type OutboxRepository interface {
	// Insert appends a new outbox event.
	Insert(ctx context.Context, e domain.OutboxEvent) error
}

// Clock returns the current time. Injected so tests can control time.
type Clock func() time.Time

// IDGenerator produces a new unique identifier. Injected so tests can
// control IDs; production code uses uuid.NewV7 for time-ordered IDs.
type IDGenerator func() uuid.UUID
