package http

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// Pinger is implemented by anything health.Ready pings to determine
// readiness (pgxpool.Pool and redis.Client both satisfy it).
type Pinger interface {
	Ping(ctx context.Context) error
}

// readinessPingTimeout bounds how long each dependency ping may take
// before /readyz reports not-ready.
const readinessPingTimeout = 2 * time.Second

// HealthHandler serves the liveness and readiness endpoints.
type HealthHandler struct {
	db    Pinger
	redis Pinger
}

// NewHealthHandler builds a HealthHandler from its dependencies.
func NewHealthHandler(db, redis Pinger) *HealthHandler {
	return &HealthHandler{db: db, redis: redis}
}

// Live handles GET /healthz: a liveness probe with no dependency checks.
func (h *HealthHandler) Live(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// Ready handles GET /readyz: pings Postgres and Redis with a bounded
// timeout each, returning 503 if either is unreachable.
func (h *HealthHandler) Ready(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), readinessPingTimeout)
	defer cancel()

	if err := h.db.Ping(ctx); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "down", "dependency": "postgres"})
		return
	}
	if err := h.redis.Ping(ctx); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "down", "dependency": "redis"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
