// Package domain contains the core wallet/ledger entities and business
// rules. It must not depend on any adapter or framework: only the
// standard library and github.com/google/uuid are allowed imports here.
package domain

import "errors"

// Sentinel domain errors. Adapters map these to transport-specific
// representations (e.g. HTTP status codes); usecases wrap them with
// fmt.Errorf("...: %w", err) to add context without losing identity.
var (
	// ErrInvalidAmount is returned when an amount is not strictly positive.
	ErrInvalidAmount = errors.New("invalid amount")
	// ErrInsufficientBalance is returned when a debit would make a wallet's
	// balance negative.
	ErrInsufficientBalance = errors.New("insufficient balance")
	// ErrBalanceOverflow is returned when a credit would overflow int64.
	ErrBalanceOverflow = errors.New("balance overflow")
	// ErrWalletNotFound is returned when a wallet lookup finds no row.
	ErrWalletNotFound = errors.New("wallet not found")
	// ErrSameWallet is returned when a transfer's source and destination
	// wallets are identical.
	ErrSameWallet = errors.New("source and destination wallet are the same")
	// ErrCurrencyMismatch is returned when a transfer's two wallets use
	// different currencies.
	ErrCurrencyMismatch = errors.New("currency mismatch")
	// ErrDuplicateIdempotencyKey is returned when an idempotency key has
	// already been used for a different (or in-flight) operation.
	ErrDuplicateIdempotencyKey = errors.New("duplicate idempotency key")
	// ErrInvalidInput is returned for malformed or missing input that
	// doesn't fit a more specific sentinel above.
	ErrInvalidInput = errors.New("invalid input")
)
