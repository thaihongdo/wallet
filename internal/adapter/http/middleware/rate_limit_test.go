package middleware_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/thaihongdo/go-wallet/internal/adapter/http/middleware"
)

// fakeRateLimiter is an in-memory middleware.RateLimiter for tests.
type fakeRateLimiter struct {
	mu     sync.Mutex
	counts map[string]int64

	err error
}

func newFakeRateLimiter() *fakeRateLimiter {
	return &fakeRateLimiter{counts: map[string]int64{}}
}

func (f *fakeRateLimiter) Incr(_ context.Context, key string, _ time.Duration) (int64, error) {
	if f.err != nil {
		return 0, f.err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.counts[key]++
	return f.counts[key], nil
}

func newRateLimitTestRouter(limiter middleware.RateLimiter, limitPerMin int) *gin.Engine {
	r := gin.New()
	r.Use(middleware.RequestID())
	r.GET("/test", middleware.RateLimit(limiter, limitPerMin), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	return r
}

func TestRateLimit_NPlus1thRequest_429(t *testing.T) {
	const limit = 3
	r := newRateLimitTestRouter(newFakeRateLimiter(), limit)

	for i := 1; i <= limit; i++ {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/test", nil))
		require.Equal(t, http.StatusOK, rec.Code, "request %d should be allowed", i)
	}

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/test", nil))
	assert.Equal(t, http.StatusTooManyRequests, rec.Code, "request %d should be rate limited", limit+1)
}

func TestRateLimit_SetsHeaders(t *testing.T) {
	r := newRateLimitTestRouter(newFakeRateLimiter(), 10)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/test", nil))
	assert.Equal(t, "10", rec.Header().Get("X-RateLimit-Limit"))
	assert.Equal(t, "9", rec.Header().Get("X-RateLimit-Remaining"))
}

func TestRateLimit_LimiterError_FailsOpen(t *testing.T) {
	limiter := newFakeRateLimiter()
	limiter.err = errors.New("rate limiter unavailable")
	r := newRateLimitTestRouter(limiter, 1)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/test", nil))
	assert.Equal(t, http.StatusOK, rec.Code, "limiter errors must fail open")
}
