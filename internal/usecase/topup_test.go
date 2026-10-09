package usecase_test

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/thaihongdo/go-wallet/internal/domain"
	"github.com/thaihongdo/go-wallet/internal/usecase"
)

type topUpFixture struct {
	wallets *fakeWalletRepo
	ops     *fakeOperationRepo
	ledger  *fakeLedgerRepo
	outbox  *fakeOutboxRepo
	uc      *usecase.TopUpUsecase
}

func newTopUpFixture() *topUpFixture {
	wallets := newFakeWalletRepo()
	ops := newFakeOperationRepo()
	ledger := newFakeLedgerRepo()
	outbox := newFakeOutboxRepo()
	tx := newFakeTxManager(wallets, ops, ledger, outbox)
	clock := func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) }
	uc := usecase.NewTopUpUsecase(tx, wallets, ops, ledger, outbox, clock, sequentialIDs())
	return &topUpFixture{wallets: wallets, ops: ops, ledger: ledger, outbox: outbox, uc: uc}
}

func TestTopUp_Valid(t *testing.T) {
	f := newTopUpFixture()
	id := mustV7(t)
	f.wallets.put(domain.Wallet{ID: id, Currency: "VND", Balance: 100})

	res, err := f.uc.TopUp(context.Background(), usecase.TopUpInput{
		IdempotencyKey: "key-1", WalletID: id, Amount: 50,
	})
	require.NoError(t, err)
	assert.False(t, res.Replayed)
	assert.Equal(t, domain.Money(150), res.Balance)

	w, _ := f.wallets.GetByID(context.Background(), id)
	assert.Equal(t, domain.Money(150), w.Balance)
	require.Len(t, f.ledger.entries, 1)
	assert.Equal(t, domain.DirectionCredit, f.ledger.entries[0].Direction)
	assert.Equal(t, domain.Money(150), f.ledger.entries[0].BalanceAfter)
	require.Len(t, f.outbox.events, 1)
	assert.Equal(t, "wallet.topped_up", f.outbox.events[0].EventType)
}

func TestTopUp_InvalidAmount(t *testing.T) {
	f := newTopUpFixture()
	id := mustV7(t)
	f.wallets.put(domain.Wallet{ID: id, Currency: "VND", Balance: 100})

	for _, amount := range []domain.Money{0, -10} {
		_, err := f.uc.TopUp(context.Background(), usecase.TopUpInput{
			IdempotencyKey: "key-1", WalletID: id, Amount: amount,
		})
		assert.True(t, errors.Is(err, domain.ErrInvalidAmount))
	}
}

func TestTopUp_WalletMissing(t *testing.T) {
	f := newTopUpFixture()
	_, err := f.uc.TopUp(context.Background(), usecase.TopUpInput{
		IdempotencyKey: "key-1", WalletID: mustV7(t), Amount: 50,
	})
	assert.True(t, errors.Is(err, domain.ErrWalletNotFound))
}

func TestTopUp_Replay(t *testing.T) {
	f := newTopUpFixture()
	id := mustV7(t)
	f.wallets.put(domain.Wallet{ID: id, Currency: "VND", Balance: 100})

	first, err := f.uc.TopUp(context.Background(), usecase.TopUpInput{
		IdempotencyKey: "key-1", WalletID: id, Amount: 50,
	})
	require.NoError(t, err)

	second, err := f.uc.TopUp(context.Background(), usecase.TopUpInput{
		IdempotencyKey: "key-1", WalletID: id, Amount: 50,
	})
	require.NoError(t, err)
	assert.True(t, second.Replayed)
	assert.Equal(t, first.Operation.ID, second.Operation.ID)
	assert.Equal(t, domain.Money(150), second.Balance, "balance credited only once")
	assert.Equal(t, 1, f.ops.createCalls)
	assert.Len(t, f.ledger.entries, 1)
}

func TestTopUp_Overflow(t *testing.T) {
	f := newTopUpFixture()
	id := mustV7(t)
	f.wallets.put(domain.Wallet{ID: id, Currency: "VND", Balance: math.MaxInt64 - 1})

	_, err := f.uc.TopUp(context.Background(), usecase.TopUpInput{
		IdempotencyKey: "key-1", WalletID: id, Amount: 2,
	})
	assert.True(t, errors.Is(err, domain.ErrBalanceOverflow))
}
