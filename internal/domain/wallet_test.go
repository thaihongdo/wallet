package domain_test

import (
	"errors"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/thaihongdo/go-wallet/internal/domain"
)

func TestWallet_Debit(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		w := domain.Wallet{Balance: 100}
		err := w.Debit(40)
		require.NoError(t, err)
		assert.Equal(t, domain.Money(60), w.Balance)
	})

	t.Run("exact balance leaves zero", func(t *testing.T) {
		w := domain.Wallet{Balance: 100}
		err := w.Debit(100)
		require.NoError(t, err)
		assert.Equal(t, domain.Money(0), w.Balance)
	})

	t.Run("zero amount is invalid", func(t *testing.T) {
		w := domain.Wallet{Balance: 100}
		err := w.Debit(0)
		assert.True(t, errors.Is(err, domain.ErrInvalidAmount))
		assert.Equal(t, domain.Money(100), w.Balance, "balance must be unchanged on error")
	})

	t.Run("negative amount is invalid", func(t *testing.T) {
		w := domain.Wallet{Balance: 100}
		err := w.Debit(-1)
		assert.True(t, errors.Is(err, domain.ErrInvalidAmount))
		assert.Equal(t, domain.Money(100), w.Balance)
	})

	t.Run("amount greater than balance is insufficient", func(t *testing.T) {
		w := domain.Wallet{Balance: 100}
		err := w.Debit(101)
		assert.True(t, errors.Is(err, domain.ErrInsufficientBalance))
		assert.Equal(t, domain.Money(100), w.Balance, "balance must be unchanged on error")
	})
}

func TestWallet_Credit(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		w := domain.Wallet{Balance: 100}
		err := w.Credit(40)
		require.NoError(t, err)
		assert.Equal(t, domain.Money(140), w.Balance)
	})

	t.Run("zero amount is invalid", func(t *testing.T) {
		w := domain.Wallet{Balance: 100}
		err := w.Credit(0)
		assert.True(t, errors.Is(err, domain.ErrInvalidAmount))
		assert.Equal(t, domain.Money(100), w.Balance)
	})

	t.Run("negative amount is invalid", func(t *testing.T) {
		w := domain.Wallet{Balance: 100}
		err := w.Credit(-5)
		assert.True(t, errors.Is(err, domain.ErrInvalidAmount))
		assert.Equal(t, domain.Money(100), w.Balance)
	})

	t.Run("overflow near MaxInt64", func(t *testing.T) {
		w := domain.Wallet{Balance: math.MaxInt64 - 1}
		err := w.Credit(2)
		assert.True(t, errors.Is(err, domain.ErrBalanceOverflow))
		assert.Equal(t, domain.Money(math.MaxInt64-1), w.Balance, "balance must be unchanged on error")
	})

	t.Run("exact MaxInt64 does not overflow", func(t *testing.T) {
		w := domain.Wallet{Balance: math.MaxInt64 - 1}
		err := w.Credit(1)
		require.NoError(t, err)
		assert.Equal(t, domain.Money(math.MaxInt64), w.Balance)
	})
}
