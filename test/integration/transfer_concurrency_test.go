//go:build integration

package integration

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/thaihongdo/go-wallet/internal/domain"
	"github.com/thaihongdo/go-wallet/internal/usecase"
)

const concurrencyTestInitialBalance = domain.Money(1_000_000)

func createWallets(t *testing.T, n int) []uuid.UUID {
	t.Helper()
	ctx := context.Background()
	ids := make([]uuid.UUID, n)
	for i := 0; i < n; i++ {
		w, err := walletUC.CreateWallet(ctx, fmt.Sprintf("owner-%d-%s", i, newUUIDv7()))
		require.NoError(t, err)
		_, err = topUpUC.TopUp(ctx, usecase.TopUpInput{
			IdempotencyKey: "seed-" + w.ID.String(),
			WalletID:       w.ID,
			Amount:         concurrencyTestInitialBalance,
		})
		require.NoError(t, err)
		ids[i] = w.ID
	}
	return ids
}

// TestTransfer_ConcurrentRandom fires many concurrent random transfers
// among a fixed set of wallets and asserts the ledger stays consistent:
// total money is conserved, no wallet goes negative, and each wallet's
// ledger entries sum to its current balance.
func TestTransfer_ConcurrentRandom(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()

	const numWallets = 5
	const numGoroutines = 100
	const transfersPerGoroutine = 20

	ids := createWallets(t, numWallets)
	totalBefore := concurrencyTestInitialBalance * domain.Money(numWallets)

	var wg sync.WaitGroup
	for g := 0; g < numGoroutines; g++ {
		wg.Add(1)
		go func(seed int64) {
			defer wg.Done()
			rnd := rand.New(rand.NewSource(seed))
			for i := 0; i < transfersPerGoroutine; i++ {
				from := ids[rnd.Intn(numWallets)]
				to := ids[rnd.Intn(numWallets)]
				if from == to {
					continue
				}
				amount := domain.Money(1 + rnd.Intn(1000))
				key := fmt.Sprintf("rand-%d-%d-%d", seed, i, rnd.Int())
				_, err := transferUC.Transfer(ctx, usecase.TransferInput{
					IdempotencyKey: key,
					FromWalletID:   from,
					ToWalletID:     to,
					Amount:         amount,
				})
				if err != nil && !errors.Is(err, domain.ErrInsufficientBalance) {
					t.Errorf("unexpected transfer error: %v", err)
				}
			}
		}(int64(g))
	}
	wg.Wait()

	var totalAfter domain.Money
	for _, id := range ids {
		w, err := walletUC.GetWallet(ctx, id)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, int64(w.Balance), int64(0), "wallet %s went negative", id)
		totalAfter += w.Balance

		entries, _, err := walletUC.ListEntries(ctx, id, nil, 100)
		require.NoError(t, err)
		// ListEntries only returns up to 100; fetch all pages to sum the ledger.
		sum := ledgerSignedSum(entries)
		cursor := pageCursor(entries, 100)
		for cursor != nil {
			more, _, err := walletUC.ListEntries(ctx, id, cursor, 100)
			require.NoError(t, err)
			sum += ledgerSignedSum(more)
			cursor = pageCursor(more, 100)
		}
		assert.Equal(t, int64(w.Balance), int64(concurrencyTestInitialBalance)+sum,
			"ledger sum for wallet %s must reconstruct its balance", id)
	}
	assert.Equal(t, int64(totalBefore), int64(totalAfter), "total money must be conserved")
}

func ledgerSignedSum(entries []domain.LedgerEntry) int64 {
	var sum int64
	for _, e := range entries {
		switch e.Direction {
		case domain.DirectionCredit:
			sum += int64(e.Amount)
		case domain.DirectionDebit:
			sum -= int64(e.Amount)
		}
	}
	return sum
}

func pageCursor(entries []domain.LedgerEntry, limit int) *uuid.UUID {
	if len(entries) < limit {
		return nil
	}
	last := entries[len(entries)-1].ID
	return &last
}

// TestTransfer_NoDeadlockCrossDirection runs transfers A->B and B->A
// concurrently. Because wallet row locks are always acquired in
// ascending ID order (WalletRepository.LockByIDs), this must complete
// well within the timeout regardless of each transfer's direction.
func TestTransfer_NoDeadlockCrossDirection(t *testing.T) {
	truncateAll(t)
	ids := createWallets(t, 2)
	a, b := ids[0], ids[1]

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	const perDirection = 50
	var wg sync.WaitGroup
	errs := make(chan error, perDirection*2)

	launch := func(from, to uuid.UUID, tag string) {
		for i := 0; i < perDirection; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				key := fmt.Sprintf("%s-%d", tag, i)
				_, err := transferUC.Transfer(ctx, usecase.TransferInput{
					IdempotencyKey: key, FromWalletID: from, ToWalletID: to, Amount: 10,
				})
				if err != nil && !errors.Is(err, domain.ErrInsufficientBalance) {
					errs <- err
				}
			}(i)
		}
	}
	launch(a, b, "a-to-b")
	launch(b, a, "b-to-a")

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("transfers did not complete within timeout: possible deadlock")
	}
	close(errs)
	for err := range errs {
		t.Errorf("unexpected transfer error: %v", err)
	}
}
