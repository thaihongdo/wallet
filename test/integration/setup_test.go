//go:build integration

// Package integration exercises the Postgres and Redis adapters against
// real services via testcontainers-go. Set TEST_DATABASE_URL and
// TEST_REDIS_URL to point at already-running services (e.g. the local
// docker-compose stack) to skip starting containers — useful in
// environments where testcontainers itself is unavailable; CI always
// uses the testcontainers path.
package integration

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"

	"github.com/thaihongdo/go-wallet/internal/adapter/postgres"
	redisadapter "github.com/thaihongdo/go-wallet/internal/adapter/redis"
	"github.com/thaihongdo/go-wallet/internal/usecase"
)

var (
	pool        *pgxpool.Pool
	redisClient *redis.Client

	walletRepo    *postgres.WalletRepo
	operationRepo *postgres.OperationRepo
	ledgerRepo    *postgres.LedgerRepo
	outboxRepo    *postgres.OutboxRepo
	txManager     *postgres.TxManager

	idemStore   *redisadapter.IdempotencyStore
	rateLimiter *redisadapter.RateLimiter

	topUpUC    *usecase.TopUpUsecase
	transferUC *usecase.TransferUsecase
	walletUC   *usecase.WalletUsecase
)

func TestMain(m *testing.M) {
	ctx := context.Background()
	code, err := run(ctx, m)
	if err != nil {
		fmt.Fprintln(os.Stderr, "integration setup failed:", err)
		os.Exit(1)
	}
	os.Exit(code)
}

func run(ctx context.Context, m *testing.M) (int, error) {
	dbURL, cleanupDB, err := postgresURL(ctx)
	if err != nil {
		return 0, fmt.Errorf("start postgres: %w", err)
	}
	defer cleanupDB()

	redisURL, cleanupRedis, err := redisURL(ctx)
	if err != nil {
		return 0, fmt.Errorf("start redis: %w", err)
	}
	defer cleanupRedis()

	pool, err = postgres.NewPool(ctx, dbURL, 10)
	if err != nil {
		return 0, fmt.Errorf("connect postgres: %w", err)
	}
	defer pool.Close()

	if err := runMigrations(ctx, pool); err != nil {
		return 0, fmt.Errorf("run migrations: %w", err)
	}

	redisOpts, err := redis.ParseURL(redisURL)
	if err != nil {
		return 0, fmt.Errorf("parse redis url: %w", err)
	}
	redisClient = redis.NewClient(redisOpts)
	defer redisClient.Close()
	if err := redisClient.Ping(ctx).Err(); err != nil {
		return 0, fmt.Errorf("ping redis: %w", err)
	}

	walletRepo = postgres.NewWalletRepo(pool)
	operationRepo = postgres.NewOperationRepo(pool)
	ledgerRepo = postgres.NewLedgerRepo(pool)
	outboxRepo = postgres.NewOutboxRepo(pool)
	txManager = postgres.NewTxManager(pool)

	idemStore = redisadapter.NewIdempotencyStore(redisClient)
	rateLimiter = redisadapter.NewRateLimiter(redisClient)

	clock := time.Now
	newID := newUUIDv7
	topUpUC = usecase.NewTopUpUsecase(txManager, walletRepo, operationRepo, ledgerRepo, outboxRepo, clock, newID)
	transferUC = usecase.NewTransferUsecase(txManager, walletRepo, operationRepo, ledgerRepo, outboxRepo, clock, newID)
	walletUC = usecase.NewWalletUsecase(walletRepo, ledgerRepo, clock, newID)

	return m.Run(), nil
}

// postgresURL returns a Postgres connection string and a cleanup
// function. If TEST_DATABASE_URL is set, it is used directly (no
// container is started, no cleanup needed). Otherwise a Postgres 16
// testcontainer is started.
func postgresURL(ctx context.Context) (string, func(), error) {
	if url := os.Getenv("TEST_DATABASE_URL"); url != "" {
		return url, func() {}, nil
	}

	container, err := tcpostgres.Run(ctx, "postgres:16-alpine",
		tcpostgres.WithDatabase("wallet"),
		tcpostgres.WithUsername("wallet"),
		tcpostgres.WithPassword("wallet"),
		tcpostgres.BasicWaitStrategies(),
	)
	if err != nil {
		return "", nil, err
	}
	url, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { _ = container.Terminate(ctx) }
	return url, cleanup, nil
}

// redisURL returns a Redis connection string and a cleanup function. If
// TEST_REDIS_URL is set, it is used directly. Otherwise a Redis 7
// testcontainer is started.
func redisURL(ctx context.Context) (string, func(), error) {
	if url := os.Getenv("TEST_REDIS_URL"); url != "" {
		return url, func() {}, nil
	}

	container, err := tcredis.Run(ctx, "redis:7-alpine")
	if err != nil {
		return "", nil, err
	}
	url, err := container.ConnectionString(ctx)
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { _ = container.Terminate(ctx) }
	return url, cleanup, nil
}
