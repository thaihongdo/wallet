//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/thaihongdo/go-wallet/internal/adapter/http/middleware"
)

// TestRedisIdempotencyStore exercises Begin/Complete/Release semantics
// against a real Redis instance.
func TestRedisIdempotencyStore(t *testing.T) {
	ctx := context.Background()
	key := "integration-test-" + newUUIDv7().String()
	const ttl = time.Minute

	// Begin on a fresh key acquires it.
	rec, acquired, err := idemStore.Begin(ctx, key, "hash-1", ttl)
	require.NoError(t, err)
	assert.True(t, acquired)
	assert.Nil(t, rec)

	// A second Begin while "processing" returns the existing record,
	// unacquired.
	rec2, acquired2, err := idemStore.Begin(ctx, key, "hash-1", ttl)
	require.NoError(t, err)
	assert.False(t, acquired2)
	require.NotNil(t, rec2)
	assert.Equal(t, "processing", rec2.Status)
	assert.Equal(t, "hash-1", rec2.ReqHash)

	// Complete stores the final outcome.
	err = idemStore.Complete(ctx, key, middleware.IdemRecord{
		Status: "completed", ReqHash: "hash-1", StatusCode: 200, Body: []byte(`{"ok":true}`),
	}, ttl)
	require.NoError(t, err)

	rec3, acquired3, err := idemStore.Begin(ctx, key, "hash-1", ttl)
	require.NoError(t, err)
	assert.False(t, acquired3)
	require.NotNil(t, rec3)
	assert.Equal(t, "completed", rec3.Status)
	assert.Equal(t, 200, rec3.StatusCode)
	assert.Equal(t, []byte(`{"ok":true}`), rec3.Body)

	// Release frees the key so a fresh Begin can acquire it again.
	require.NoError(t, idemStore.Release(ctx, key))
	_, acquired4, err := idemStore.Begin(ctx, key, "hash-2", ttl)
	require.NoError(t, err)
	assert.True(t, acquired4, "key must be acquirable again after Release")
}

// TestRedisRateLimiter exercises Incr against a real Redis instance.
func TestRedisRateLimiter(t *testing.T) {
	ctx := context.Background()
	key := "integration-test-rl-" + newUUIDv7().String()

	n1, err := rateLimiter.Incr(ctx, key, time.Minute)
	require.NoError(t, err)
	assert.Equal(t, int64(1), n1)

	n2, err := rateLimiter.Incr(ctx, key, time.Minute)
	require.NoError(t, err)
	assert.Equal(t, int64(2), n2)
}
