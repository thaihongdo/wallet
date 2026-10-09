// Package redis adapts Redis (via go-redis/v9) for idempotency and rate
// limiting. It is one of only two packages (besides cmd/api) allowed to
// import go-redis.
package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// NewClient creates a go-redis client for url (redis:// or rediss://,
// parsed via redis.ParseURL so TLS and auth embedded in the URL work),
// and verifies connectivity with a 5s-bounded ping.
func NewClient(ctx context.Context, url string) (*redis.Client, error) {
	opts, err := redis.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("parse redis url: %w", err)
	}
	client := redis.NewClient(opts)

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := client.Ping(pingCtx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("ping redis: %w", err)
	}
	return client, nil
}

// Pinger wraps a *redis.Client so it satisfies the http package's
// Pinger interface (Ping(ctx) error), since redis.Client.Ping instead
// returns a *redis.StatusCmd.
type Pinger struct {
	client *redis.Client
}

// NewPinger wraps client for use as a readiness dependency.
func NewPinger(client *redis.Client) Pinger {
	return Pinger{client: client}
}

// Ping pings the wrapped Redis client.
func (p Pinger) Ping(ctx context.Context) error {
	return p.client.Ping(ctx).Err()
}
