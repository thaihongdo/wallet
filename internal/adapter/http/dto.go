package http

import (
	"time"

	"github.com/google/uuid"

	"github.com/thaihongdo/go-wallet/internal/adapter/http/middleware"
	"github.com/thaihongdo/go-wallet/internal/domain"
)

// CreateWalletRequest is the body of POST /v1/wallets.
type CreateWalletRequest struct {
	OwnerID string `json:"owner_id" binding:"required"`
}

// WalletResponse is the wire representation of a domain.Wallet.
type WalletResponse struct {
	ID        uuid.UUID    `json:"id"`
	OwnerID   string       `json:"owner_id"`
	Currency  string       `json:"currency"`
	Balance   domain.Money `json:"balance"`
	CreatedAt time.Time    `json:"created_at"`
	UpdatedAt time.Time    `json:"updated_at"`
}

func walletToResponse(w *domain.Wallet) WalletResponse {
	return WalletResponse{
		ID:        w.ID,
		OwnerID:   w.OwnerID,
		Currency:  w.Currency,
		Balance:   w.Balance,
		CreatedAt: w.CreatedAt,
		UpdatedAt: w.UpdatedAt,
	}
}

// TopUpRequest is the body of POST /v1/wallets/:id/topup.
type TopUpRequest struct {
	Amount domain.Money `json:"amount" binding:"required"`
}

// TopUpResponse is the body of a successful top-up.
type TopUpResponse struct {
	OperationID uuid.UUID      `json:"operation_id"`
	Wallet      WalletResponse `json:"wallet"`
}

// TransferRequest is the body of POST /v1/transfers.
type TransferRequest struct {
	FromWalletID uuid.UUID    `json:"from_wallet_id" binding:"required"`
	ToWalletID   uuid.UUID    `json:"to_wallet_id" binding:"required"`
	Amount       domain.Money `json:"amount" binding:"required"`
}

// TransferResponse is the body of a successful transfer.
type TransferResponse struct {
	OperationID  uuid.UUID    `json:"operation_id"`
	FromWalletID uuid.UUID    `json:"from_wallet_id"`
	ToWalletID   uuid.UUID    `json:"to_wallet_id"`
	Amount       domain.Money `json:"amount"`
	FromBalance  domain.Money `json:"from_balance"`
	ToBalance    domain.Money `json:"to_balance"`
	CreatedAt    time.Time    `json:"created_at"`
}

// EntryResponse is the wire representation of a domain.LedgerEntry.
type EntryResponse struct {
	ID           uuid.UUID        `json:"id"`
	OperationID  uuid.UUID        `json:"operation_id"`
	Direction    domain.Direction `json:"direction"`
	Amount       domain.Money     `json:"amount"`
	BalanceAfter domain.Money     `json:"balance_after"`
	CreatedAt    time.Time        `json:"created_at"`
}

func entryToResponse(e domain.LedgerEntry) EntryResponse {
	return EntryResponse{
		ID:           e.ID,
		OperationID:  e.OperationID,
		Direction:    e.Direction,
		Amount:       e.Amount,
		BalanceAfter: e.BalanceAfter,
		CreatedAt:    e.CreatedAt,
	}
}

// ListEntriesResponse is the body of GET /v1/wallets/:id/entries.
type ListEntriesResponse struct {
	Items      []EntryResponse `json:"items"`
	NextCursor *string         `json:"next_cursor"`
}

// ErrorBody is the JSON shape of every non-2xx response. It is defined
// in the middleware package because middleware (auth, rate limiting,
// idempotency, recovery) must produce it without importing this
// package; handlers and the error mapper reuse the same type via alias.
type ErrorBody = middleware.ErrorBody

// ErrorDetail carries a machine-readable code and a human message.
type ErrorDetail = middleware.ErrorDetail
