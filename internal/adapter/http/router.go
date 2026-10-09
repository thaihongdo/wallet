// Package http adapts the usecase layer onto an HTTP API via Gin. It is
// one of only two packages allowed to import gin (the other is
// cmd/api).
package http

import (
	"github.com/gin-gonic/gin"

	"github.com/thaihongdo/go-wallet/internal/adapter/http/middleware"
)

// Deps wires every dependency NewRouter needs. RateLimit and
// Idempotency are optional (nil-safe): when nil, that middleware is
// skipped, which is useful for tests and for Phase 6 (added in Phase 7).
type Deps struct {
	Wallets   WalletUsecase
	TopUp     TopUpUsecase
	Transfers TransferUsecase
	Health    *HealthHandler

	APIKeys []string

	// RateLimit and Idempotency, when non-nil, are applied to the
	// mutating money routes (topup, transfer) in that order, after Auth.
	RateLimit   gin.HandlerFunc
	Idempotency gin.HandlerFunc
}

// NewRouter builds the complete Gin engine: middleware order is
// RequestID -> Logger -> Recovery globally, then (per protected group)
// Auth -> RateLimit -> Idempotency (idempotency only on the
// money-moving routes). Health endpoints are public and bypass Auth.
func NewRouter(deps Deps) *gin.Engine {
	r := gin.New()
	r.Use(middleware.RequestID(), middleware.Logger(), middleware.Recovery())

	r.GET("/healthz", deps.Health.Live)
	r.GET("/readyz", deps.Health.Ready)

	walletHandler := NewWalletHandler(deps.Wallets, deps.TopUp)
	transferHandler := NewTransferHandler(deps.Transfers)

	v1 := r.Group("/v1")
	v1.Use(middleware.Auth(deps.APIKeys))
	if deps.RateLimit != nil {
		v1.Use(deps.RateLimit)
	}

	v1.POST("/wallets", walletHandler.Create)
	v1.GET("/wallets/:id", walletHandler.Get)
	v1.GET("/wallets/:id/entries", walletHandler.ListEntries)

	money := v1.Group("")
	if deps.Idempotency != nil {
		money.Use(deps.Idempotency)
	}
	money.POST("/wallets/:id/topup", walletHandler.TopUp)
	money.POST("/transfers", transferHandler.Create)

	return r
}
