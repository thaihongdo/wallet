// Command api is the go-wallet HTTP server entrypoint. It wires config
// -> connection pools -> repositories -> usecases -> router, then runs
// an *http.Server with graceful shutdown on SIGINT/SIGTERM. No global
// state: everything is constructed and passed explicitly from here.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	httpadapter "github.com/thaihongdo/go-wallet/internal/adapter/http"
	"github.com/thaihongdo/go-wallet/internal/adapter/http/middleware"
	"github.com/thaihongdo/go-wallet/internal/adapter/postgres"
	redisadapter "github.com/thaihongdo/go-wallet/internal/adapter/redis"
	"github.com/thaihongdo/go-wallet/internal/config"
	"github.com/thaihongdo/go-wallet/internal/usecase"

	"github.com/google/uuid"
)

const (
	readHeaderTimeout = 5 * time.Second
	readTimeout       = 10 * time.Second
	writeTimeout      = 15 * time.Second
	idleTimeout       = 60 * time.Second
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal startup error", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	logger := newLogger(cfg.LogLevel)
	slog.SetDefault(logger)
	gin.SetMode(cfg.GinMode)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	slog.Info("connecting to postgres")
	pool, err := postgres.NewPool(ctx, cfg.DatabaseURL, cfg.DBMaxConns)
	if err != nil {
		return fmt.Errorf("connect postgres: %w", err)
	}
	defer pool.Close()

	slog.Info("connecting to redis")
	redisClient, err := redisadapter.NewClient(ctx, cfg.RedisURL)
	if err != nil {
		return fmt.Errorf("connect redis: %w", err)
	}
	defer func() {
		if err := redisClient.Close(); err != nil {
			slog.Warn("error closing redis client", "error", err)
		}
	}()

	walletRepo := postgres.NewWalletRepo(pool)
	operationRepo := postgres.NewOperationRepo(pool)
	ledgerRepo := postgres.NewLedgerRepo(pool)
	outboxRepo := postgres.NewOutboxRepo(pool)
	txManager := postgres.NewTxManager(pool)

	idemStore := redisadapter.NewIdempotencyStore(redisClient)
	rateLimiter := redisadapter.NewRateLimiter(redisClient)

	clock := time.Now
	newID := newUUIDv7
	walletUC := usecase.NewWalletUsecase(walletRepo, ledgerRepo, clock, newID)
	topUpUC := usecase.NewTopUpUsecase(txManager, walletRepo, operationRepo, ledgerRepo, outboxRepo, clock, newID)
	transferUC := usecase.NewTransferUsecase(txManager, walletRepo, operationRepo, ledgerRepo, outboxRepo, clock, newID)

	health := httpadapter.NewHealthHandler(pool, redisadapter.NewPinger(redisClient))

	router := httpadapter.NewRouter(httpadapter.Deps{
		Wallets:     walletUC,
		TopUp:       topUpUC,
		Transfers:   transferUC,
		Health:      health,
		APIKeys:     cfg.APIKeys,
		RateLimit:   middleware.RateLimit(rateLimiter, cfg.RateLimitPerMin),
		Idempotency: middleware.Idempotency(idemStore),
	})

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           router,
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
	}

	serverErr := make(chan error, 1)
	go func() {
		slog.Info("listening", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
			return
		}
		serverErr <- nil
	}()

	select {
	case err := <-serverErr:
		if err != nil {
			return fmt.Errorf("server error: %w", err)
		}
	case <-ctx.Done():
		slog.Info("shutdown signal received")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()

	slog.Info("shutting down HTTP server", "timeout", cfg.ShutdownTimeout)
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("error during HTTP server shutdown", "error", err)
	} else {
		slog.Info("HTTP server shut down cleanly")
	}

	return nil
}

// newUUIDv7 generates a time-ordered UUIDv7, used as the production
// usecase.IDGenerator.
func newUUIDv7() uuid.UUID {
	id, err := uuid.NewV7()
	if err != nil {
		// uuid.NewV7 only fails if the system's random source is broken,
		// which is unrecoverable.
		panic(err)
	}
	return id
}

// newLogger builds a slog JSON logger at the level named by levelName
// ("debug", "info", "warn", "error"; defaults to info for an unknown
// value).
func newLogger(levelName string) *slog.Logger {
	var level slog.Level
	if err := level.UnmarshalText([]byte(levelName)); err != nil {
		level = slog.LevelInfo
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
}
