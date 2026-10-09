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

func newWalletFixture() (*fakeWalletRepo, *fakeLedgerRepo, *usecase.WalletUsecase) {
	wallets := newFakeWalletRepo()
	ledger := newFakeLedgerRepo()
	clock := func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) }
	uc := usecase.NewWalletUsecase(wallets, ledger, clock, sequentialIDs())
	return wallets, ledger, uc
}

func TestCreateWallet(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		_, _, uc := newWalletFixture()
		w, err := uc.CreateWallet(context.Background(), "  user-1  ")
		require.NoError(t, err)
		assert.Equal(t, "user-1", w.OwnerID)
		assert.Equal(t, "VND", w.Currency)
		assert.Equal(t, domain.Money(0), w.Balance)
	})

	t.Run("empty owner id is invalid", func(t *testing.T) {
		_, _, uc := newWalletFixture()
		_, err := uc.CreateWallet(context.Background(), "   ")
		assert.True(t, errors.Is(err, domain.ErrInvalidInput))
	})

	t.Run("owner id over 64 chars is invalid", func(t *testing.T) {
		_, _, uc := newWalletFixture()
		long := make([]byte, 65)
		for i := range long {
			long[i] = 'a'
		}
		_, err := uc.CreateWallet(context.Background(), string(long))
		assert.True(t, errors.Is(err, domain.ErrInvalidInput))
	})
}

func TestGetWallet(t *testing.T) {
	wallets, _, uc := newWalletFixture()
	id := mustV7(t)
	wallets.put(domain.Wallet{ID: id, OwnerID: "user-1", Currency: "VND", Balance: 100})

	w, err := uc.GetWallet(context.Background(), id)
	require.NoError(t, err)
	assert.Equal(t, domain.Money(100), w.Balance)

	_, err = uc.GetWallet(context.Background(), mustV7(t))
	assert.True(t, errors.Is(err, domain.ErrWalletNotFound))
}

func TestListEntries(t *testing.T) {
	t.Run("wallet missing", func(t *testing.T) {
		_, _, uc := newWalletFixture()
		_, _, err := uc.ListEntries(context.Background(), mustV7(t), nil, 20)
		assert.True(t, errors.Is(err, domain.ErrWalletNotFound))
	})

	t.Run("default and clamped limit, next_cursor only when more rows exist", func(t *testing.T) {
		wallets, ledger, uc := newWalletFixture()
		walletID := mustV7(t)
		wallets.put(domain.Wallet{ID: walletID, Currency: "VND", Balance: 1000})

		// Insert 25 entries with strictly increasing (time-ordered) IDs.
		ids := make([]uuid.UUID, 25)
		for i := range ids {
			ids[i] = mustV7(t)
			time.Sleep(time.Microsecond)
		}
		for _, id := range ids {
			require.NoError(t, ledger.Insert(context.Background(), domain.LedgerEntry{
				ID: id, WalletID: walletID, Direction: domain.DirectionCredit, Amount: 1, BalanceAfter: 1,
			}))
		}

		// Default limit (0 -> 20): exactly 20 returned, cursor set because 5 remain.
		entries, cursor, err := uc.ListEntries(context.Background(), walletID, nil, 0)
		require.NoError(t, err)
		assert.Len(t, entries, 20)
		require.NotNil(t, cursor)

		// Next page with the cursor: remaining 5, no further cursor.
		entries2, cursor2, err := uc.ListEntries(context.Background(), walletID, cursor, 20)
		require.NoError(t, err)
		assert.Len(t, entries2, 5)
		assert.Nil(t, cursor2)

		// Limit clamped above 100.
		entriesAll, cursorAll, err := uc.ListEntries(context.Background(), walletID, nil, 1000)
		require.NoError(t, err)
		assert.Len(t, entriesAll, 25)
		assert.Nil(t, cursorAll)

		// Limit clamped below 1.
		entriesMin, cursorMin, err := uc.ListEntries(context.Background(), walletID, nil, -5)
		require.NoError(t, err)
		assert.Len(t, entriesMin, 20, "limit <= 0 falls back to default (20)")
		require.NotNil(t, cursorMin)
	})
}
