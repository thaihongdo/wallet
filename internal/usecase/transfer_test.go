package usecase_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/thaihongdo/go-wallet/internal/domain"
	"github.com/thaihongdo/go-wallet/internal/usecase"
)

type transferFixture struct {
	wallets *fakeWalletRepo
	ops     *fakeOperationRepo
	ledger  *fakeLedgerRepo
	outbox  *fakeOutboxRepo
	tx      *fakeTxManager
	uc      *usecase.TransferUsecase
}

func newTransferFixture() *transferFixture {
	wallets := newFakeWalletRepo()
	ops := newFakeOperationRepo()
	ledger := newFakeLedgerRepo()
	outbox := newFakeOutboxRepo()
	tx := newFakeTxManager(wallets, ops, ledger, outbox)
	clock := func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) }
	uc := usecase.NewTransferUsecase(tx, wallets, ops, ledger, outbox, clock, sequentialIDs())
	return &transferFixture{wallets: wallets, ops: ops, ledger: ledger, outbox: outbox, tx: tx, uc: uc}
}

func mustV7(t *testing.T) uuid.UUID {
	t.Helper()
	id, err := uuid.NewV7()
	require.NoError(t, err)
	return id
}

func TestTransfer_Valid(t *testing.T) {
	f := newTransferFixture()
	from := mustV7(t)
	to := mustV7(t)
	f.wallets.put(domain.Wallet{ID: from, Currency: "VND", Balance: 1000})
	f.wallets.put(domain.Wallet{ID: to, Currency: "VND", Balance: 500})

	res, err := f.uc.Transfer(context.Background(), usecase.TransferInput{
		IdempotencyKey: "key-1",
		FromWalletID:   from,
		ToWalletID:     to,
		Amount:         300,
	})
	require.NoError(t, err)
	assert.False(t, res.Replayed)
	assert.Equal(t, domain.Money(700), res.FromBalance)
	assert.Equal(t, domain.Money(800), res.ToBalance)

	fromW, _ := f.wallets.GetByID(context.Background(), from)
	toW, _ := f.wallets.GetByID(context.Background(), to)
	assert.Equal(t, domain.Money(700), fromW.Balance)
	assert.Equal(t, domain.Money(800), toW.Balance)

	assert.Equal(t, 1, f.ops.createCalls)
	require.Len(t, f.ledger.entries, 2)

	var debit, credit *domain.LedgerEntry
	for i := range f.ledger.entries {
		e := &f.ledger.entries[i]
		switch e.Direction {
		case domain.DirectionDebit:
			debit = e
		case domain.DirectionCredit:
			credit = e
		}
	}
	require.NotNil(t, debit)
	require.NotNil(t, credit)
	assert.Equal(t, from, debit.WalletID)
	assert.Equal(t, domain.Money(700), debit.BalanceAfter)
	assert.Equal(t, to, credit.WalletID)
	assert.Equal(t, domain.Money(800), credit.BalanceAfter)

	require.Len(t, f.outbox.events, 1)
	assert.Equal(t, "transfer.completed", f.outbox.events[0].EventType)
}

func TestTransfer_InsufficientBalance(t *testing.T) {
	f := newTransferFixture()
	from := mustV7(t)
	to := mustV7(t)
	f.wallets.put(domain.Wallet{ID: from, Currency: "VND", Balance: 100})
	f.wallets.put(domain.Wallet{ID: to, Currency: "VND", Balance: 0})

	_, err := f.uc.Transfer(context.Background(), usecase.TransferInput{
		IdempotencyKey: "key-1", FromWalletID: from, ToWalletID: to, Amount: 200,
	})
	assert.True(t, errors.Is(err, domain.ErrInsufficientBalance))

	fromW, _ := f.wallets.GetByID(context.Background(), from)
	toW, _ := f.wallets.GetByID(context.Background(), to)
	assert.Equal(t, domain.Money(100), fromW.Balance, "no state change on failure")
	assert.Equal(t, domain.Money(0), toW.Balance)
	assert.Empty(t, f.ledger.entries)
	assert.Empty(t, f.outbox.events)
}

func TestTransfer_SameWallet(t *testing.T) {
	f := newTransferFixture()
	id := mustV7(t)
	f.wallets.put(domain.Wallet{ID: id, Currency: "VND", Balance: 100})

	_, err := f.uc.Transfer(context.Background(), usecase.TransferInput{
		IdempotencyKey: "key-1", FromWalletID: id, ToWalletID: id, Amount: 50,
	})
	assert.True(t, errors.Is(err, domain.ErrSameWallet))
	assert.Zero(t, f.ops.createCalls, "repos must not be called")
}

func TestTransfer_InvalidAmount(t *testing.T) {
	f := newTransferFixture()
	from, to := mustV7(t), mustV7(t)
	f.wallets.put(domain.Wallet{ID: from, Currency: "VND", Balance: 100})
	f.wallets.put(domain.Wallet{ID: to, Currency: "VND", Balance: 0})

	for _, amount := range []domain.Money{0, -1} {
		_, err := f.uc.Transfer(context.Background(), usecase.TransferInput{
			IdempotencyKey: "key-1", FromWalletID: from, ToWalletID: to, Amount: amount,
		})
		assert.True(t, errors.Is(err, domain.ErrInvalidAmount))
	}
}

func TestTransfer_EmptyIdempotencyKey(t *testing.T) {
	f := newTransferFixture()
	from, to := mustV7(t), mustV7(t)
	f.wallets.put(domain.Wallet{ID: from, Currency: "VND", Balance: 100})
	f.wallets.put(domain.Wallet{ID: to, Currency: "VND", Balance: 0})

	_, err := f.uc.Transfer(context.Background(), usecase.TransferInput{
		IdempotencyKey: "", FromWalletID: from, ToWalletID: to, Amount: 10,
	})
	assert.True(t, errors.Is(err, domain.ErrInvalidInput))
}

func TestTransfer_WalletMissing(t *testing.T) {
	f := newTransferFixture()
	from, to := mustV7(t), mustV7(t)
	f.wallets.put(domain.Wallet{ID: from, Currency: "VND", Balance: 100})
	// `to` is never created.

	_, err := f.uc.Transfer(context.Background(), usecase.TransferInput{
		IdempotencyKey: "key-1", FromWalletID: from, ToWalletID: to, Amount: 10,
	})
	assert.True(t, errors.Is(err, domain.ErrWalletNotFound))
}

func TestTransfer_CurrencyMismatch(t *testing.T) {
	f := newTransferFixture()
	from, to := mustV7(t), mustV7(t)
	f.wallets.put(domain.Wallet{ID: from, Currency: "VND", Balance: 100})
	f.wallets.put(domain.Wallet{ID: to, Currency: "USD", Balance: 0})

	_, err := f.uc.Transfer(context.Background(), usecase.TransferInput{
		IdempotencyKey: "key-1", FromWalletID: from, ToWalletID: to, Amount: 10,
	})
	assert.True(t, errors.Is(err, domain.ErrCurrencyMismatch))
}

func TestTransfer_ExistingIdempotencyKey_Replays(t *testing.T) {
	f := newTransferFixture()
	from, to := mustV7(t), mustV7(t)
	f.wallets.put(domain.Wallet{ID: from, Currency: "VND", Balance: 1000})
	f.wallets.put(domain.Wallet{ID: to, Currency: "VND", Balance: 500})

	first, err := f.uc.Transfer(context.Background(), usecase.TransferInput{
		IdempotencyKey: "key-1", FromWalletID: from, ToWalletID: to, Amount: 300,
	})
	require.NoError(t, err)
	require.False(t, first.Replayed)

	second, err := f.uc.Transfer(context.Background(), usecase.TransferInput{
		IdempotencyKey: "key-1", FromWalletID: from, ToWalletID: to, Amount: 300,
	})
	require.NoError(t, err)
	assert.True(t, second.Replayed)
	assert.Equal(t, first.Operation.ID, second.Operation.ID)
	assert.Equal(t, first.FromBalance, second.FromBalance)
	assert.Equal(t, first.ToBalance, second.ToBalance)

	assert.Equal(t, 1, f.ops.createCalls, "no new operation row on replay")
	assert.Len(t, f.ledger.entries, 2, "no new ledger entries on replay")
	assert.Len(t, f.outbox.events, 1, "no new outbox event on replay")
}

func TestTransfer_DuplicateKeyRaceOnCreate_Replays(t *testing.T) {
	f := newTransferFixture()
	from, to := mustV7(t), mustV7(t)
	f.wallets.put(domain.Wallet{ID: from, Currency: "VND", Balance: 1000})
	f.wallets.put(domain.Wallet{ID: to, Currency: "VND", Balance: 500})

	// Simulate a concurrent request that already committed under this key.
	winner := domain.Operation{
		ID:             mustV7(t),
		Type:           domain.OperationTransfer,
		IdempotencyKey: "key-1",
		FromWalletID:   &from,
		ToWalletID:     to,
		Amount:         300,
		CreatedAt:      time.Now(),
	}
	f.ops.seed(winner)
	f.ops.createErr = domain.ErrDuplicateIdempotencyKey

	res, err := f.uc.Transfer(context.Background(), usecase.TransferInput{
		IdempotencyKey: "key-1", FromWalletID: from, ToWalletID: to, Amount: 300,
	})
	require.NoError(t, err)
	assert.True(t, res.Replayed)
	assert.Equal(t, winner.ID, res.Operation.ID)
}

func TestTransfer_LedgerInsertFails_RollsBack(t *testing.T) {
	f := newTransferFixture()
	from, to := mustV7(t), mustV7(t)
	f.wallets.put(domain.Wallet{ID: from, Currency: "VND", Balance: 1000})
	f.wallets.put(domain.Wallet{ID: to, Currency: "VND", Balance: 500})
	f.ledger.insertErr = errors.New("boom")

	_, err := f.uc.Transfer(context.Background(), usecase.TransferInput{
		IdempotencyKey: "key-1", FromWalletID: from, ToWalletID: to, Amount: 300,
	})
	require.Error(t, err)

	fromW, _ := f.wallets.GetByID(context.Background(), from)
	toW, _ := f.wallets.GetByID(context.Background(), to)
	assert.Equal(t, domain.Money(1000), fromW.Balance, "balances rolled back")
	assert.Equal(t, domain.Money(500), toW.Balance)
	assert.Empty(t, f.outbox.events)
}
