//go:build integration

package integration

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/thaihongdo/go-wallet/internal/domain"
	"github.com/thaihongdo/go-wallet/internal/usecase"
)

// TestTransfer_IdempotentConcurrentSameKey fires many concurrent
// transfer requests that share one idempotency key and input. Exactly
// one underlying operation must be created, and every caller must
// observe that same operation.
func TestTransfer_IdempotentConcurrentSameKey(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()
	ids := createWallets(t, 2)
	from, to := ids[0], ids[1]

	const n = 10
	const key = "same-transfer-key"

	var wg sync.WaitGroup
	results := make([]usecase.TransferResult, n)
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i], errs[i] = transferUC.Transfer(ctx, usecase.TransferInput{
				IdempotencyKey: key, FromWalletID: from, ToWalletID: to, Amount: 1000,
			})
		}(i)
	}
	wg.Wait()

	firstOpID := results[0].Operation.ID
	for i := 0; i < n; i++ {
		require.NoError(t, errs[i])
		assert.Equal(t, firstOpID, results[i].Operation.ID, "goroutine %d saw a different operation", i)
	}

	var count int
	err := pool.QueryRow(ctx, `SELECT count(*) FROM operations WHERE idempotency_key = $1`, key).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count, "exactly one operation row must exist for the shared key")
}

// TestTopUp_Idempotent asserts that replaying the same top-up request
// credits the wallet exactly once.
func TestTopUp_Idempotent(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()
	ids := createWallets(t, 1)
	walletID := ids[0]

	balanceBefore, err := walletUC.GetWallet(ctx, walletID)
	require.NoError(t, err)

	const key = "same-topup-key"
	first, err := topUpUC.TopUp(ctx, usecase.TopUpInput{IdempotencyKey: key, WalletID: walletID, Amount: 5000})
	require.NoError(t, err)
	require.False(t, first.Replayed)

	second, err := topUpUC.TopUp(ctx, usecase.TopUpInput{IdempotencyKey: key, WalletID: walletID, Amount: 5000})
	require.NoError(t, err)
	assert.True(t, second.Replayed)

	after, err := walletUC.GetWallet(ctx, walletID)
	require.NoError(t, err)
	assert.Equal(t, balanceBefore.Balance+domain.Money(5000), after.Balance, "balance credited exactly once")

	var count int
	err = pool.QueryRow(ctx, `SELECT count(*) FROM operations WHERE idempotency_key = $1`, key).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}
