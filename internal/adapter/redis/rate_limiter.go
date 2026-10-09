package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// incrWithExpireScript atomically increments key and, only on the first
// increment (when the new value is 1), sets its TTL. Using a Lua script
// avoids the race between a plain INCR and a separate EXPIRE call (which
// could otherwise leave a key without a TTL under concurrent access).
var incrWithExpireScript = redis.NewScript(`
local count = redis.call("INCR", KEYS[1])
if count == 1 then
	redis.call("EXPIRE", KEYS[1], ARGV[1])
end
return count
`)

// RateLimiter implements middleware.RateLimiter on Redis using a fixed
// window counter per key.
type RateLimiter struct {
	client *redis.Client
}

// NewRateLimiter builds a RateLimiter backed by client.
func NewRateLimiter(client *redis.Client) *RateLimiter {
	return &RateLimiter{client: client}
}

// Incr increments the counter for key, setting its TTL to window on the
// first increment, and returns the new count.
func (r *RateLimiter) Incr(ctx context.Context, key string, window time.Duration) (int64, error) {
	res, err := incrWithExpireScript.Run(ctx, r.client, []string{key}, int(window.Seconds())).Result()
	if err != nil {
		return 0, fmt.Errorf("incr rate limit counter: %w", err)
	}
	count, ok := res.(int64)
	if !ok {
		return 0, fmt.Errorf("incr rate limit counter: unexpected script result type %T", res)
	}
	return count, nil
}
