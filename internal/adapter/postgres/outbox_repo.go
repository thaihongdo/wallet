package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/thaihongdo/go-wallet/internal/domain"
)

// OutboxRepo implements usecase.OutboxRepository on Postgres.
type OutboxRepo struct {
	pool *pgxpool.Pool
}

// NewOutboxRepo builds an OutboxRepo backed by pool.
func NewOutboxRepo(pool *pgxpool.Pool) *OutboxRepo {
	return &OutboxRepo{pool: pool}
}

// Insert appends a new outbox event row.
func (r *OutboxRepo) Insert(ctx context.Context, e domain.OutboxEvent) error {
	q := querier(ctx, r.pool)
	_, err := q.Exec(ctx, `
		INSERT INTO outbox (id, aggregate_type, aggregate_id, event_type, payload, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		e.ID, e.AggregateType, e.AggregateID, e.EventType, e.Payload, e.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert outbox event: %w", err)
	}
	return nil
}
