package domain

import (
	"time"

	"github.com/google/uuid"
)

// OperationType identifies the kind of money movement an Operation
// records.
type OperationType string

const (
	// OperationTopUp credits a single wallet from an external source.
	OperationTopUp OperationType = "TOPUP"
	// OperationTransfer moves money from one wallet to another.
	OperationTransfer OperationType = "TRANSFER"
)

// Operation is the durable, idempotent record of a single money
// movement request. Its IdempotencyKey is unique: replaying a request
// with the same key returns the same Operation instead of moving money
// again.
type Operation struct {
	ID             uuid.UUID
	Type           OperationType
	IdempotencyKey string
	// FromWalletID is nil for TOPUP operations and set for TRANSFER.
	FromWalletID *uuid.UUID
	ToWalletID   uuid.UUID
	Amount       Money
	CreatedAt    time.Time
}
