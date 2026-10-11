package http_test

import (
	"context"

	"github.com/google/uuid"

	"github.com/thaihongdo/go-wallet/internal/domain"
	"github.com/thaihongdo/go-wallet/internal/usecase"
)

// fakeWalletUsecase implements httpadapter.WalletUsecase with
// function-field hooks so each test controls exactly what it needs.
type fakeWalletUsecase struct {
	createFn func(ctx context.Context, ownerID string) (*domain.Wallet, error)
	getFn    func(ctx context.Context, id uuid.UUID) (*domain.Wallet, error)
	listFn   func(ctx context.Context, walletID uuid.UUID, cursor *uuid.UUID, limit int) ([]domain.LedgerEntry, *uuid.UUID, error)
}

func (f *fakeWalletUsecase) CreateWallet(ctx context.Context, ownerID string) (*domain.Wallet, error) {
	return f.createFn(ctx, ownerID)
}

func (f *fakeWalletUsecase) GetWallet(ctx context.Context, id uuid.UUID) (*domain.Wallet, error) {
	return f.getFn(ctx, id)
}

func (f *fakeWalletUsecase) ListEntries(ctx context.Context, walletID uuid.UUID, cursor *uuid.UUID, limit int) ([]domain.LedgerEntry, *uuid.UUID, error) {
	return f.listFn(ctx, walletID, cursor, limit)
}

// fakeTopUpUsecase implements httpadapter.TopUpUsecase.
type fakeTopUpUsecase struct {
	fn func(ctx context.Context, in usecase.TopUpInput) (usecase.TopUpResult, error)
}

func (f *fakeTopUpUsecase) TopUp(ctx context.Context, in usecase.TopUpInput) (usecase.TopUpResult, error) {
	return f.fn(ctx, in)
}

// fakeTransferUsecase implements httpadapter.TransferUsecase.
type fakeTransferUsecase struct {
	fn func(ctx context.Context, in usecase.TransferInput) (usecase.TransferResult, error)
}

func (f *fakeTransferUsecase) Transfer(ctx context.Context, in usecase.TransferInput) (usecase.TransferResult, error) {
	return f.fn(ctx, in)
}

// fakePinger implements httpadapter.Pinger.
type fakePinger struct {
	err error
}

func (f *fakePinger) Ping(_ context.Context) error {
	return f.err
}
