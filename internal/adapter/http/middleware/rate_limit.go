package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

// RateLimiter increments and returns the request count for key within
// its current fixed window, atomically creating the window's expiry on
// the first increment. Implemented by adapter/redis.
type RateLimiter interface {
	// Incr increments the counter for key (creating it with the given TTL
	// if it doesn't exist) and returns the new count.
	Incr(ctx context.Context, key string, window time.Duration) (int64, error)
}

// RateLimit enforces a fixed-window limit of limitPerMin requests per
// minute per API key. The window key includes the current UTC minute,
// so the limit resets every minute; a request at the boundary between
// two windows can allow a short burst, which is an accepted trade-off
// documented in the README.
//
// If the limiter errors (e.g. Redis is down), this middleware logs a
// warning and fails open: it is a protective measure, not a correctness
// guarantee, so an outage degrades to "unlimited" rather than rejecting
// all traffic.
func RateLimit(limiter RateLimiter, limitPerMin int) gin.HandlerFunc {
	return func(c *gin.Context) {
		window := time.Minute
		minuteBucket := time.Now().UTC().Unix() / int64(window.Seconds())
		key := "rl:" + GetAPIKey(c) + ":" + strconv.FormatInt(minuteBucket, 10)

		count, err := limiter.Incr(c.Request.Context(), key, window)
		if err != nil {
			slog.Warn("rate limiter error, failing open", "error", err, "request_id", GetRequestID(c))
			c.Next()
			return
		}

		remaining := int64(limitPerMin) - count
		if remaining < 0 {
			remaining = 0
		}
		c.Header("X-RateLimit-Limit", strconv.Itoa(limitPerMin))
		c.Header("X-RateLimit-Remaining", strconv.FormatInt(remaining, 10))

		if count > int64(limitPerMin) {
			RespondError(c, http.StatusTooManyRequests, "RATE_LIMITED", "rate limit exceeded")
			return
		}
		c.Next()
	}
}
