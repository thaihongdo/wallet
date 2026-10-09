package domain

import (
	"time"

	"github.com/google/uuid"
)

// Direction indicates whether a LedgerEntry decreased (DEBIT) or
// increased (CREDIT) a wallet's balance.
type Direction string

const (
	// DirectionDebit marks an entry that decreased the wallet's balance.
	DirectionDebit Direction = "DEBIT"
	// DirectionCredit marks an entry that increased the wallet's balance.
	DirectionCredit Direction = "CREDIT"
)

// LedgerEntry is one immutable line in a wallet's double-entry ledger.
// BalanceAfter records the wallet's balance immediately after this entry
// was applied, which makes the ledger self-auditing: replaying entries
// in ID order must reproduce the wallet's current balance.
type LedgerEntry struct {
	ID           uuid.UUID
	OperationID  uuid.UUID
	WalletID     uuid.UUID
	Direction    Direction
	Amount       Money
	BalanceAfter Money
	CreatedAt    time.Time
}
