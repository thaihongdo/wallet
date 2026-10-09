package domain

import (
	"time"

	"github.com/google/uuid"
)

// Wallet is a single-currency account holding a non-negative balance.
type Wallet struct {
	ID        uuid.UUID
	OwnerID   string
	Currency  string
	Balance   Money
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Debit subtracts amount from the wallet's balance in place.
// Returns ErrInvalidAmount if amount <= 0, or ErrInsufficientBalance if
// the wallet's balance is less than amount. On error, the wallet is left
// unmodified.
func (w *Wallet) Debit(amount Money) error {
	if !amount.IsPositive() {
		return ErrInvalidAmount
	}
	if w.Balance < amount {
		return ErrInsufficientBalance
	}
	w.Balance -= amount
	return nil
}

// Credit adds amount to the wallet's balance in place.
// Returns ErrInvalidAmount if amount <= 0, or ErrBalanceOverflow if the
// result would overflow int64. On error, the wallet is left unmodified.
func (w *Wallet) Credit(amount Money) error {
	if !amount.IsPositive() {
		return ErrInvalidAmount
	}
	if addOverflows(w.Balance, amount) {
		return ErrBalanceOverflow
	}
	w.Balance += amount
	return nil
}
