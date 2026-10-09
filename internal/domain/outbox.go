package domain

import (
	"time"

	"github.com/google/uuid"
)

// OutboxEvent is a durable record of a domain event, written in the same
// database transaction as the state change it describes (the
// transactional outbox pattern). A separate relay process is
// responsible for publishing unpublished events; this repository only
// models the row.
type OutboxEvent struct {
	ID            uuid.UUID
	AggregateType string
	AggregateID   uuid.UUID
	EventType     string
	// Payload is the event body, serialized as JSON.
	Payload   []byte
	CreatedAt time.Time
}
