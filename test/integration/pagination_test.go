//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/thaihongdo/go-wallet/internal/usecase"
)

// TestListEntries_KeysetPagination asserts keyset pagination over a
// wallet's ledger returns every entry exactly once, in stable pages,
// even when new entries are inserted between page fetches.
func TestListEntries_KeysetPagination(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()
	ids := createWallets(t, 1)
	walletID := ids[0]

	// createWallets already performed one top-up (the seed credit); do 44
	// more small top-ups so there are 45 ledger entries in total.
	for i := 0; i < 44; i++ {
		_, err := topUpUC.TopUp(ctx, usecase.TopUpInput{
			IdempotencyKey: "page-seed-" + newUUIDv7().String(),
			WalletID:       walletID,
			Amount:         1,
		})
		require.NoError(t, err)
	}

	seen := map[uuid.UUID]bool{}
	var cursor *uuid.UUID
	pageSizes := []int{}

	for {
		page, next, err := walletUC.ListEntries(ctx, walletID, cursor, 20)
		require.NoError(t, err)
		pageSizes = append(pageSizes, len(page))
		for _, e := range page {
			require.False(t, seen[e.ID], "duplicate entry %s across pages", e.ID)
			seen[e.ID] = true
		}
		if next == nil {
			break
		}
		cursor = next
		if len(pageSizes) > 10 {
			t.Fatal("too many pages, pagination likely looping")
		}
	}

	assert.Equal(t, []int{20, 20, 5}, pageSizes)
	assert.Len(t, seen, 45, "every entry must be returned exactly once, no gaps")
}

// TestListEntries_KeysetPagination_StableUnderInserts verifies that
// inserting new ledger entries between page fetches does not introduce
// duplicates into pages fetched afterwards: newer entries have larger
// (UUIDv7) IDs and sort before the cursor, so they only ever appear
// ahead of the cursor, never behind it.
func TestListEntries_KeysetPagination_StableUnderInserts(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()
	ids := createWallets(t, 1)
	walletID := ids[0]

	for i := 0; i < 24; i++ {
		_, err := topUpUC.TopUp(ctx, usecase.TopUpInput{
			IdempotencyKey: "stable-seed-" + newUUIDv7().String(),
			WalletID:       walletID,
			Amount:         1,
		})
		require.NoError(t, err)
	}

	firstPage, cursor, err := walletUC.ListEntries(ctx, walletID, nil, 20)
	require.NoError(t, err)
	require.Len(t, firstPage, 20)
	require.NotNil(t, cursor)

	// Insert new entries "on top" (newer, larger IDs) between fetches.
	for i := 0; i < 5; i++ {
		_, err := topUpUC.TopUp(ctx, usecase.TopUpInput{
			IdempotencyKey: "inserted-between-" + newUUIDv7().String(),
			WalletID:       walletID,
			Amount:         1,
		})
		require.NoError(t, err)
	}

	secondPage, cursor2, err := walletUC.ListEntries(ctx, walletID, cursor, 20)
	require.NoError(t, err)
	assert.Nil(t, cursor2)

	seen := map[uuid.UUID]bool{}
	for _, e := range firstPage {
		seen[e.ID] = true
	}
	for _, e := range secondPage {
		assert.False(t, seen[e.ID], "entry %s duplicated across pages after concurrent insert", e.ID)
	}
	assert.Len(t, secondPage, 5, "second page must only contain the original remaining entries")
}
