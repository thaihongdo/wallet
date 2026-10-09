package usecase

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/thaihongdo/go-wallet/internal/domain"
)

const (
	defaultCurrency = "VND"

	minOwnerIDLen = 1
	maxOwnerIDLen = 64

	defaultListLimit = 20
	minListLimit     = 1
	maxListLimit     = 100
)

// WalletUsecase implements wallet creation, lookup, and ledger listing.
type WalletUsecase struct {
	wallets WalletRepository
	ledger  LedgerRepository
	now     Clock
	newID   IDGenerator
}

// NewWalletUsecase builds a WalletUsecase from its dependencies.
func NewWalletUsecase(wallets WalletRepository, ledger LedgerRepository, now Clock, newID IDGenerator) *WalletUsecase {
	return &WalletUsecase{wallets: wallets, ledger: ledger, now: now, newID: newID}
}

// CreateWallet creates a new wallet for ownerID with zero balance in
// VND. ownerID is trimmed and must be 1..64 characters, else
// ErrInvalidInput is returned.
func (uc *WalletUsecase) CreateWallet(ctx context.Context, ownerID string) (*domain.Wallet, error) {
	ownerID = strings.TrimSpace(ownerID)
	if len(ownerID) < minOwnerIDLen || len(ownerID) > maxOwnerIDLen {
		return nil, fmt.Errorf("create wallet: %w", domain.ErrInvalidInput)
	}

	now := uc.now()
	w := &domain.Wallet{
		ID:        uc.newID(),
		OwnerID:   ownerID,
		Currency:  defaultCurrency,
		Balance:   0,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := uc.wallets.Create(ctx, w); err != nil {
		return nil, fmt.Errorf("create wallet: %w", err)
	}
	return w, nil
}

// GetWallet returns the wallet with the given ID, or ErrWalletNotFound.
func (uc *WalletUsecase) GetWallet(ctx context.Context, id uuid.UUID) (*domain.Wallet, error) {
	w, err := uc.wallets.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get wallet: %w", err)
	}
	return w, nil
}

// ListEntries returns a page of walletID's ledger entries ordered newest
// first. limit is clamped to [1,100] and defaults to 20 when <= 0. If
// more entries exist beyond the returned page, nextCursor is the ID of
// the last returned entry (pass it back as cursor to fetch the next
// page); otherwise nextCursor is nil.
func (uc *WalletUsecase) ListEntries(ctx context.Context, walletID uuid.UUID, cursor *uuid.UUID, limit int) (entries []domain.LedgerEntry, nextCursor *uuid.UUID, err error) {
	if limit <= 0 {
		limit = defaultListLimit
	}
	if limit < minListLimit {
		limit = minListLimit
	}
	if limit > maxListLimit {
		limit = maxListLimit
	}

	if _, err := uc.wallets.GetByID(ctx, walletID); err != nil {
		return nil, nil, fmt.Errorf("list entries: %w", err)
	}

	rows, err := uc.ledger.ListByWallet(ctx, walletID, cursor, limit+1)
	if err != nil {
		return nil, nil, fmt.Errorf("list entries: %w", err)
	}

	if len(rows) > limit {
		rows = rows[:limit]
		last := rows[len(rows)-1].ID
		nextCursor = &last
	}
	return rows, nextCursor, nil
}
