# go-wallet — Build Plan for Coding Agent

> **Audience:** an autonomous coding agent building this repository from scratch.
> **Read this whole file before writing code.** Execute phases in order. After each phase, run the phase's verification commands and do not start the next phase until they pass.
> Steps marked **[HUMAN]** require account access (cloud billing, secrets). Do not attempt them; prepare everything around them and list them in your final summary.

---

## 0. Mission

Build **go-wallet**, a small but production-minded **wallet & ledger backend service** in Go. It will be shown in a Senior Golang Backend interview (code review / screen share), so **code clarity, correctness under concurrency, and explicit trade-offs matter more than feature count.**

Time budget: **4 working days**. Respect the scope cut in §2.

### Core capabilities
1. Create a wallet.
2. Get wallet balance.
3. Top up a wallet (idempotent).
4. Transfer money between two wallets (idempotent, concurrency-safe, deadlock-free).
5. List a wallet's ledger entries with keyset (cursor) pagination.

---

## 1. Non-negotiable rules for the agent

1. **Clean Architecture dependency rule:** `adapter → usecase → domain`. Never the reverse.
   - `internal/domain` imports **stdlib + `github.com/google/uuid` only**.
   - `internal/usecase` imports `domain` + stdlib + uuid. **No gin, pgx, redis imports.**
   - `gin` may only be imported in `internal/adapter/http` and `cmd/api`.
   - Enforce with `make arch-check` (§9).
2. **Interfaces are declared by the consumer** (`internal/usecase/ports.go`), implemented in adapters.
3. Usecases receive `context.Context`, **never `*gin.Context`**. Handlers pass `c.Request.Context()`.
4. **Money is `int64`** (VND, no minor unit). Never use float. Single currency `VND` only.
5. IDs are **UUIDv7** (`uuid.NewV7()`), so they are time-ordered and usable as keyset cursors.
6. Errors: domain errors are sentinel values in `domain/errors.go`; wrap with `fmt.Errorf("...: %w", err)`; map to HTTP only in `adapter/http/error_mapper.go`.
7. No global state. Wire everything in `cmd/api/main.go` via constructors.
8. Only use the libraries in §3. If you think another is required, stop and justify it in the final summary instead of adding it.
9. After every phase: `go build ./... && go vet ./... && go test -race ./...` must pass. Commit once per phase with a conventional commit message (`feat: ...`, `test: ...`, `chore: ...`).
10. Do not leave `TODO` in any **Must** item. `TODO` is allowed only for **Could** items, with a one-line reason.
11. Keep functions short, name things clearly, and add a short doc comment on every exported type/func in `domain` and `usecase`.

---

## 2. Scope

| Priority | Items |
|---|---|
| **Must** | Domain + usecases (CreateWallet, GetWallet, TopUp, Transfer, ListEntries); Postgres repos with `TxManager`; row locking with ordered locks; DB constraints as last line of defence; Redis idempotency middleware; Gin HTTP API + error mapping; unit tests; integration concurrency tests; Dockerfile; docker-compose; graceful shutdown; health endpoints; README |
| **Should** | Redis rate limiting; keyset pagination; outbox row written inside the same DB transaction; GitHub Actions CI |
| **Could** | Outbox relay worker; Prometheus `/metrics`; automated CD to Cloud Run |
| **Out of scope** | JWT/OAuth (use static API keys), multi-currency, OpenTelemetry, Kubernetes, ORM, sqlc |

---

## 3. Tech stack (exact modules)

| Purpose | Module |
|---|---|
| Go | latest stable Go toolchain (set in `go.mod`) |
| HTTP | `github.com/gin-gonic/gin` |
| Postgres | `github.com/jackc/pgx/v5` (`pgxpool`, `pgconn` for error codes) |
| Redis | `github.com/redis/go-redis/v9` |
| IDs | `github.com/google/uuid` |
| Migrations | `golang-migrate` CLI (via Docker image `migrate/migrate`) — not imported in app code |
| Tests | `github.com/stretchr/testify`, `github.com/testcontainers/testcontainers-go` (+ `modules/postgres`, `modules/redis`) |
| Logging | stdlib `log/slog` (JSON handler) |
| Config | stdlib `os` — no config library |
| Lint | `golangci-lint` |

---

## 4. Repository layout

```
go-wallet/
├── cmd/
│   └── api/
│       └── main.go                    # config → pools → repos → usecases → router → http.Server; graceful shutdown
├── internal/
│   ├── config/
│   │   └── config.go                  # Load() from env, with defaults + validation
│   ├── domain/
│   │   ├── money.go                   # type Money int64; validation helpers
│   │   ├── wallet.go                  # Wallet entity, Debit/Credit
│   │   ├── operation.go               # Operation (TOPUP/TRANSFER)
│   │   ├── ledger.go                  # LedgerEntry, Direction
│   │   ├── outbox.go                  # OutboxEvent
│   │   ├── errors.go                  # sentinel errors
│   │   └── *_test.go
│   ├── usecase/
│   │   ├── ports.go                   # all repository/service interfaces
│   │   ├── wallet.go                  # CreateWallet, GetWallet, ListEntries
│   │   ├── topup.go
│   │   ├── transfer.go
│   │   ├── fakes_test.go              # hand-written in-memory fakes (no mock generators)
│   │   ├── wallet_test.go
│   │   ├── topup_test.go
│   │   └── transfer_test.go
│   └── adapter/
│       ├── postgres/
│       │   ├── db.go                  # NewPool(ctx, url, maxConns)
│       │   ├── tx_manager.go          # WithinTx; tx stored in context
│       │   ├── querier.go             # helper: pick tx from ctx or pool
│       │   ├── wallet_repo.go
│       │   ├── operation_repo.go
│       │   ├── ledger_repo.go
│       │   └── outbox_repo.go
│       ├── redis/
│       │   ├── client.go              # NewClient(url) using redis.ParseURL (supports rediss://)
│       │   ├── idempotency_store.go
│       │   └── rate_limiter.go
│       └── http/
│           ├── router.go              # NewRouter(deps) *gin.Engine
│           ├── dto.go                 # request/response structs + mappers
│           ├── error_mapper.go        # domain error → status + code
│           ├── wallet_handler.go
│           ├── transfer_handler.go
│           ├── health_handler.go
│           ├── middleware/
│           │   ├── request_id.go
│           │   ├── logger.go
│           │   ├── recovery.go
│           │   ├── auth.go
│           │   ├── rate_limit.go
│           │   └── idempotency.go
│           └── *_test.go
├── migrations/
│   ├── 000001_init.up.sql
│   └── 000001_init.down.sql
├── test/
│   └── integration/
│       ├── setup_test.go              # testcontainers bootstrap, TestMain
│       ├── transfer_concurrency_test.go
│       ├── idempotency_test.go
│       └── pagination_test.go
├── .github/workflows/ci.yml
├── .golangci.yml
├── .env.example
├── Dockerfile
├── docker-compose.yml
├── Makefile
└── README.md
```

---

## 5. Database schema (`migrations/000001_init.up.sql`)

```sql
CREATE TABLE wallets (
    id          UUID PRIMARY KEY,
    owner_id    TEXT        NOT NULL,
    currency    CHAR(3)     NOT NULL DEFAULT 'VND',
    balance     BIGINT      NOT NULL DEFAULT 0 CHECK (balance >= 0),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_wallets_owner ON wallets(owner_id);

CREATE TABLE operations (
    id               UUID PRIMARY KEY,
    type             TEXT        NOT NULL CHECK (type IN ('TOPUP','TRANSFER')),
    idempotency_key  TEXT        NOT NULL,
    from_wallet_id   UUID        NULL REFERENCES wallets(id),
    to_wallet_id     UUID        NOT NULL REFERENCES wallets(id),
    amount           BIGINT      NOT NULL CHECK (amount > 0),
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_operations_idem UNIQUE (idempotency_key),
    CONSTRAINT chk_transfer_wallets CHECK (
        (type = 'TOPUP'    AND from_wallet_id IS NULL) OR
        (type = 'TRANSFER' AND from_wallet_id IS NOT NULL AND from_wallet_id <> to_wallet_id)
    )
);

CREATE TABLE ledger_entries (
    id             UUID PRIMARY KEY,                 -- UUIDv7, used as keyset cursor
    operation_id   UUID        NOT NULL REFERENCES operations(id),
    wallet_id      UUID        NOT NULL REFERENCES wallets(id),
    direction      TEXT        NOT NULL CHECK (direction IN ('DEBIT','CREDIT')),
    amount         BIGINT      NOT NULL CHECK (amount > 0),
    balance_after  BIGINT      NOT NULL CHECK (balance_after >= 0),
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_ledger_wallet_id_desc ON ledger_entries(wallet_id, id DESC);

CREATE TABLE outbox (
    id              UUID PRIMARY KEY,
    aggregate_type  TEXT        NOT NULL,
    aggregate_id    UUID        NOT NULL,
    event_type      TEXT        NOT NULL,
    payload         JSONB       NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    published_at    TIMESTAMPTZ NULL
);
CREATE INDEX idx_outbox_unpublished ON outbox(created_at) WHERE published_at IS NULL;
```

`000001_init.down.sql` drops tables in reverse order.

**Why these constraints (put in README):** `CHECK (balance >= 0)` and `UNIQUE (idempotency_key)` guarantee correctness even if application code or Redis fails.

---

## 6. Domain spec (`internal/domain`)

```go
type Money int64

type Wallet struct {
    ID        uuid.UUID
    OwnerID   string
    Currency  string
    Balance   Money
    CreatedAt time.Time
    UpdatedAt time.Time
}

// Debit subtracts amount. Returns ErrInvalidAmount if amount <= 0,
// ErrInsufficientBalance if Balance < amount.
func (w *Wallet) Debit(amount Money) error
// Credit adds amount. Returns ErrInvalidAmount if amount <= 0,
// ErrBalanceOverflow if the result would overflow int64.
func (w *Wallet) Credit(amount Money) error

type OperationType string // "TOPUP" | "TRANSFER"
type Operation struct {
    ID, ToWalletID uuid.UUID
    FromWalletID   *uuid.UUID
    Type           OperationType
    IdempotencyKey string
    Amount         Money
    CreatedAt      time.Time
}

type Direction string // "DEBIT" | "CREDIT"
type LedgerEntry struct {
    ID, OperationID, WalletID uuid.UUID
    Direction    Direction
    Amount       Money
    BalanceAfter Money
    CreatedAt    time.Time
}

type OutboxEvent struct {
    ID, AggregateID uuid.UUID
    AggregateType, EventType string
    Payload   []byte // JSON
    CreatedAt time.Time
}
```

`errors.go` sentinels:
`ErrInvalidAmount, ErrInsufficientBalance, ErrBalanceOverflow, ErrWalletNotFound, ErrSameWallet, ErrCurrencyMismatch, ErrDuplicateIdempotencyKey, ErrInvalidInput`.

---

## 7. Usecase spec (`internal/usecase`)

### 7.1 Ports (`ports.go`)

```go
type TxManager interface {
    // WithinTx runs fn in a DB transaction. The tx travels inside ctx.
    // Commits if fn returns nil, rolls back otherwise (and on panic, then re-panics).
    WithinTx(ctx context.Context, fn func(ctx context.Context) error) error
}

type WalletRepository interface {
    Create(ctx context.Context, w *domain.Wallet) error
    GetByID(ctx context.Context, id uuid.UUID) (*domain.Wallet, error)          // ErrWalletNotFound
    // LockByIDs locks rows with SELECT ... WHERE id = ANY($1) ORDER BY id FOR UPDATE.
    // Ordering by id makes lock acquisition order deterministic → no deadlocks.
    LockByIDs(ctx context.Context, ids []uuid.UUID) ([]*domain.Wallet, error)
    UpdateBalance(ctx context.Context, id uuid.UUID, balance domain.Money) error
}

type OperationRepository interface {
    Create(ctx context.Context, op *domain.Operation) error                    // ErrDuplicateIdempotencyKey on 23505
    GetByIdempotencyKey(ctx context.Context, key string) (*domain.Operation, error) // (nil, nil) if absent
}

type LedgerRepository interface {
    Insert(ctx context.Context, entries ...domain.LedgerEntry) error
    // ListByWallet returns entries with id < cursor (if cursor != nil), ORDER BY id DESC, LIMIT limit.
    ListByWallet(ctx context.Context, walletID uuid.UUID, cursor *uuid.UUID, limit int) ([]domain.LedgerEntry, error)
}

type OutboxRepository interface {
    Insert(ctx context.Context, e domain.OutboxEvent) error
}

type Clock func() time.Time
type IDGenerator func() uuid.UUID
```

Usecase structs receive dependencies via constructor, e.g.
`NewTransferUsecase(tx TxManager, wallets WalletRepository, ops OperationRepository, ledger LedgerRepository, outbox OutboxRepository, now Clock, newID IDGenerator) *TransferUsecase`.

### 7.2 Transfer algorithm (`transfer.go`)

Input: `TransferInput{IdempotencyKey string; FromWalletID, ToWalletID uuid.UUID; Amount domain.Money}`
Output: `TransferResult{Operation domain.Operation; FromBalance, ToBalance domain.Money; Replayed bool}`

1. Validate: key non-empty (`ErrInvalidInput`), amount > 0 (`ErrInvalidAmount`), from ≠ to (`ErrSameWallet`).
2. `tx.WithinTx(ctx, func(ctx) error { ... })`:
   1. `existing := ops.GetByIdempotencyKey(key)`; if found → set result from existing, `Replayed=true`, return nil.
   2. `ws := wallets.LockByIDs([from, to])`; if `len(ws) != 2` → `ErrWalletNotFound`.
   3. Resolve `from`, `to` from `ws` by ID (order returned is by id, not by role).
   4. If currencies differ → `ErrCurrencyMismatch`.
   5. `from.Debit(amount)`; `to.Credit(amount)` (propagate domain errors).
   6. Create `Operation{Type: TRANSFER, ...}` via `ops.Create`.
   7. Insert 2 ledger entries (DEBIT on from, CREDIT on to) with `BalanceAfter`.
   8. `UpdateBalance` for both wallets.
   9. Insert outbox event `transfer.completed` with JSON payload `{operation_id, from_wallet_id, to_wallet_id, amount}`.
3. If `WithinTx` returns `ErrDuplicateIdempotencyKey` (concurrent request with same key won the race): outside the tx, `GetByIdempotencyKey(key)`, return it with `Replayed=true`. For the replayed case, read current balances via `GetByID`.
4. Return other errors unchanged (wrapped with context).

### 7.3 TopUp algorithm (`topup.go`)

Same shape as Transfer with a single wallet: validate → WithinTx { idempotency check → `LockByIDs([walletID])` → `Credit` → Operation TOPUP → 1 CREDIT ledger entry → UpdateBalance → outbox `wallet.topped_up` } → handle duplicate key race.

### 7.4 Wallet usecases (`wallet.go`)

- `CreateWallet(ctx, ownerID string) (*domain.Wallet, error)` — ownerID trimmed, 1..64 chars else `ErrInvalidInput`; currency `VND`, balance 0.
- `GetWallet(ctx, id) (*domain.Wallet, error)`.
- `ListEntries(ctx, walletID, cursor *uuid.UUID, limit int) (entries []domain.LedgerEntry, nextCursor *uuid.UUID, err error)` — limit default 20, clamp to [1,100]; verify wallet exists (`ErrWalletNotFound`); fetch `limit+1` rows; if more than `limit`, `nextCursor` = id of the last returned entry.

---

## 8. HTTP API spec (`internal/adapter/http`, Gin)

### 8.1 Common

- Base path `/v1`. JSON only.
- Auth: header `X-API-Key` must be in env `API_KEYS` (comma-separated). Missing/invalid → `401`. Health endpoints are public.
- Every response includes header `X-Request-ID` (take incoming header if present, else generate UUIDv7).
- Error body:
  ```json
  { "error": { "code": "INSUFFICIENT_BALANCE", "message": "insufficient balance" }, "request_id": "..." }
  ```
- Router: `gin.New()` (NOT `gin.Default()`), middleware order: `RequestID → Logger → Recovery → (per group) Auth → RateLimit → Idempotency (only on mutating money routes)`.
- Validation via Gin binding tags; binding failure → `400 INVALID_REQUEST`.

### 8.2 Endpoints

| Method & path | Request | Success | Notes |
|---|---|---|---|
| `POST /v1/wallets` | `{"owner_id":"user-1"}` | `201` Wallet | |
| `GET /v1/wallets/:id` | — | `200` Wallet | bad UUID → `400` |
| `POST /v1/wallets/:id/topup` | header `Idempotency-Key`; `{"amount":100000}` | `200` `{"operation_id","wallet":Wallet}` | idempotent |
| `POST /v1/transfers` | header `Idempotency-Key`; `{"from_wallet_id","to_wallet_id","amount"}` | `201` `{"operation_id","from_wallet_id","to_wallet_id","amount","from_balance","to_balance","created_at"}` | idempotent |
| `GET /v1/wallets/:id/entries?limit=20&cursor=` | — | `200` `{"items":[Entry],"next_cursor":"..."|null}` | cursor = base64url(UUID) |
| `GET /healthz` | — | `200 {"status":"ok"}` | liveness, no deps |
| `GET /readyz` | — | `200` / `503` | pings Postgres + Redis with 2s timeout |

Wallet JSON: `{"id","owner_id","currency","balance","created_at","updated_at"}`.
Entry JSON: `{"id","operation_id","direction","amount","balance_after","created_at"}`.
Replayed responses add header `Idempotent-Replayed: true`.

### 8.3 Error mapping (`error_mapper.go`)

| Domain error | HTTP | code |
|---|---|---|
| `ErrInvalidInput`, `ErrInvalidAmount`, `ErrSameWallet` | 400 | `INVALID_REQUEST` / `INVALID_AMOUNT` / `SAME_WALLET` |
| `ErrWalletNotFound` | 404 | `WALLET_NOT_FOUND` |
| `ErrInsufficientBalance` | 422 | `INSUFFICIENT_BALANCE` |
| `ErrCurrencyMismatch` | 422 | `CURRENCY_MISMATCH` |
| `ErrBalanceOverflow` | 422 | `BALANCE_OVERFLOW` |
| (middleware) missing Idempotency-Key | 400 | `IDEMPOTENCY_KEY_REQUIRED` |
| (middleware) key in progress | 409 | `IDEMPOTENCY_IN_PROGRESS` |
| (middleware) key reused with different body | 422 | `IDEMPOTENCY_KEY_MISMATCH` |
| (middleware) rate limited | 429 | `RATE_LIMITED` |
| anything else | 500 | `INTERNAL` (log full error, never leak it) |

### 8.4 Idempotency middleware (`middleware/idempotency.go`)

Port used by middleware (declared in the http package, implemented in `adapter/redis`):
```go
type IdempotencyStore interface {
    // Begin tries SET key {"status":"processing","hash":h} NX EX ttl.
    // Returns (nil, true, nil) if acquired; (record, false, nil) if key exists.
    Begin(ctx context.Context, key, reqHash string, ttl time.Duration) (*IdemRecord, bool, error)
    Complete(ctx context.Context, key string, rec IdemRecord, ttl time.Duration) error
    Release(ctx context.Context, key string) error
}
type IdemRecord struct {
    Status     string // "processing" | "completed"
    ReqHash    string
    StatusCode int
    Body       []byte
}
```
Flow:
1. Header `Idempotency-Key` required (1..128 chars).
2. Redis key = `idem:{apiKey}:{METHOD}:{route}:{idempotencyKey}`; `reqHash` = sha256 of raw body (restore body for handler).
3. `Begin` with TTL 24h:
   - acquired → run handler with a response-capturing writer.
     - status `< 500` → `Complete` with status+body.
     - status `>= 500` → `Release` so client can retry.
   - exists & `processing` → `409 IDEMPOTENCY_IN_PROGRESS`.
   - exists & `completed` & same hash → replay stored status+body, header `Idempotent-Replayed: true`.
   - exists & different hash → `422 IDEMPOTENCY_KEY_MISMATCH`.
4. **If Redis errors:** log warn and **continue to handler** (fail-open). Correctness is still guaranteed by the DB `UNIQUE(idempotency_key)` + usecase replay logic. Pass the idempotency key to the usecase in all cases.

### 8.5 Rate limit middleware (`middleware/rate_limit.go`)

- Fixed window 60s per API key: Redis key `rl:{apiKey}:{unixMinute}`; `INCR`; if result == 1 then `EXPIRE 60` (use a pipeline or a small Lua script so INCR+EXPIRE are atomic).
- Limit from env `RATE_LIMIT_PER_MIN` (default 60). Headers `X-RateLimit-Limit`, `X-RateLimit-Remaining`.
- Over limit → `429`. Redis error → fail-open with warn log (document trade-off in README).

---

## 9. Infrastructure files

### 9.1 Config (env)

| Var | Default | Required |
|---|---|---|
| `PORT` | `8080` | no (Cloud Run injects it) |
| `DATABASE_URL` | — | yes |
| `REDIS_URL` | — | yes (`redis://` local, `rediss://` cloud) |
| `API_KEYS` | — | yes |
| `RATE_LIMIT_PER_MIN` | `60` | no |
| `DB_MAX_CONNS` | `5` | no |
| `LOG_LEVEL` | `info` | no |
| `GIN_MODE` | `release` | no |
| `SHUTDOWN_TIMEOUT` | `10s` | no |

`config.Load()` returns an error listing all missing required vars. Provide `.env.example`.

### 9.2 `cmd/api/main.go`

- Load config → slog JSON logger → pgxpool (`MaxConns = DB_MAX_CONNS`, ping with timeout) → redis client (`redis.ParseURL`, ping) → repos → usecases → router.
- Wrap Gin in `&http.Server{Addr: ":"+PORT, Handler: router, ReadHeaderTimeout: 5s, ReadTimeout: 10s, WriteTimeout: 15s, IdleTimeout: 60s}`. **Do not use `router.Run()`.**
- `signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)`; on signal: `srv.Shutdown(ctx with SHUTDOWN_TIMEOUT)`, then close redis and pool. Log each step.

### 9.3 `docker-compose.yml`

Services: `postgres:16-alpine` (db `wallet`, user/pass `wallet`, port 5432, healthcheck `pg_isready`), `redis:7-alpine` (port 6379, healthcheck `redis-cli ping`). App is run with `make run` against these.

### 9.4 `Dockerfile` (multi-stage)

```dockerfile
FROM golang:<latest-stable>-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/api /api
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/api"]
```

### 9.5 `Makefile` targets

```
up                 docker compose up -d
down               docker compose down -v
migrate-up         run migrate/migrate docker image against $DATABASE_URL (default local compose URL)
migrate-down       one step down
run                go run ./cmd/api (loads .env)
test               go test -race -count=1 ./internal/...
test-integration   go test -race -count=1 -tags=integration ./test/...
lint               golangci-lint run
arch-check         fail if `go list -deps ./internal/usecase ./internal/domain` contains gin, pgx or go-redis
docker-build       docker build -t go-wallet:local .
```

### 9.6 `.github/workflows/ci.yml`

On push / PR: checkout → setup-go (from `go.mod`) → cache → `make lint` → `make arch-check` → `make test` → `make test-integration` (testcontainers works on `ubuntu-latest`) → `make docker-build`.

---

## 10. Test specification

### 10.1 Domain unit tests
- `Debit`: ok; amount 0 / negative → `ErrInvalidAmount`; amount > balance → `ErrInsufficientBalance`; exact balance → 0.
- `Credit`: ok; invalid amount; overflow near `math.MaxInt64` → `ErrBalanceOverflow`.

### 10.2 Usecase unit tests (table-driven, hand-written fakes in `fakes_test.go`)
Fakes: in-memory maps guarded by `sync.Mutex`; `fakeTxManager` that snapshots state and restores it if `fn` returns error (to verify rollback); injectable error hooks per method.

`Transfer`:
| Case | Expect |
|---|---|
| valid transfer | balances updated, 1 operation, 2 ledger entries (DEBIT/CREDIT, correct `BalanceAfter`), 1 outbox `transfer.completed` |
| insufficient balance | `ErrInsufficientBalance`, no state change |
| same wallet | `ErrSameWallet`, repos not called |
| amount ≤ 0 | `ErrInvalidAmount` |
| empty idempotency key | `ErrInvalidInput` |
| from or to wallet missing | `ErrWalletNotFound` |
| currency mismatch | `ErrCurrencyMismatch` |
| existing idempotency key | returns stored op, `Replayed=true`, no new rows |
| `ops.Create` returns `ErrDuplicateIdempotencyKey` | returns existing op, `Replayed=true` |
| ledger insert fails | error returned, state rolled back |

`TopUp`: valid; invalid amount; wallet missing; replay; overflow.
`ListEntries`: limit default/clamp; `next_cursor` set only when more rows exist; wallet missing.

### 10.3 HTTP tests (`httptest` + real Gin router + fake usecases)
- Each endpoint happy path status + JSON shape.
- Each row of §8.3 mapping.
- Missing `X-API-Key` → 401; health endpoints don't need it.
- Idempotency middleware with an in-memory `IdempotencyStore` fake: first call executes; second call with same key+body replays with header; same key different body → 422; key "processing" → 409; handler 500 → key released.
- Rate limit: N+1th request → 429.

### 10.4 Integration tests (`//go:build integration`, testcontainers Postgres 16 + Redis 7)
`setup_test.go`: `TestMain` starts containers, runs migrations (execute the `.up.sql` file directly), builds real repos/usecases; each test truncates tables.

| Test | Assertion |
|---|---|
| `TestTransfer_ConcurrentRandom` | 5 wallets × 1,000,000 each; 100 goroutines × 20 random transfers; after `wg.Wait()`: sum of balances unchanged, no negative balance, ledger sum per wallet == balance |
| `TestTransfer_NoDeadlockCrossDirection` | 50 goroutines A→B and 50 B→A concurrently; completes within 10s (use `context.WithTimeout`); no error other than `ErrInsufficientBalance` |
| `TestTransfer_IdempotentConcurrentSameKey` | 10 goroutines same key same input → exactly 1 row in `operations`, all results have same operation ID |
| `TestTopUp_Idempotent` | same key twice → balance credited once |
| `TestListEntries_KeysetPagination` | insert 45 entries; page with limit 20 → 20/20/5, no duplicates, no gaps; insert new entries between page fetches → no duplicates in later pages |
| `TestRedisIdempotencyStore` | Begin/Complete/Release semantics against real Redis |

---

## 11. Phased execution plan

### Phase 1 — Scaffold & schema (Day 1, ~2h)
Tasks: `go mod init github.com/<user>/go-wallet`; create layout §4; `docker-compose.yml`; migrations §5; `Makefile`; `.env.example`; `.golangci.yml` (enable `govet, staticcheck, errcheck, ineffassign, unused, revive, gofmt, goimports`).
Verify: `make up && make migrate-up` succeeds; `go build ./...`.

### Phase 2 — Domain (Day 1, ~2h)
Tasks: §6 + tests §10.1.
Verify: `make test`.

### Phase 3 — Usecases (Day 1, ~4h)
Tasks: §7 + fakes + tests §10.2.
Verify: `make test`, `make arch-check`.

### Phase 4 — Postgres adapter (Day 2, ~4h)
Tasks: pool, `TxManager` (tx in context via unexported key; `querier(ctx)` returns tx or pool), repos. Map pg error `23505` on `uq_operations_idem` → `ErrDuplicateIdempotencyKey`; `pgx.ErrNoRows` → `ErrWalletNotFound` / `(nil, nil)` as specified.
Verify: `go build ./...`.

### Phase 5 — Integration tests (Day 2, ~4h)
Tasks: §10.4 (except Redis store test, added in Phase 7).
Verify: `make test-integration` passes with `-race`.
Fallback if testcontainers is blocked locally: allow `TEST_DATABASE_URL` env to point to compose Postgres; keep testcontainers as the default for CI.

### Phase 6 — HTTP layer (Day 3, ~4h)
Tasks: router, middleware (request id, logger, recovery, auth), DTOs, handlers, error mapper, health handlers, tests §10.3 (excluding idempotency/rate limit).
Verify: `make test`; `make run` then smoke test with curl (create wallet → topup → transfer → entries).

### Phase 7 — Redis: idempotency + rate limit (Day 3, ~3h)
Tasks: §8.4, §8.5, Redis adapter, middleware tests, `TestRedisIdempotencyStore`.
Verify: `make test && make test-integration`; manual: repeat a transfer with same key → header `Idempotent-Replayed: true`.

### Phase 8 — Production readiness (Day 4, ~2h)
Tasks: `main.go` §9.2 with graceful shutdown; `/readyz`; Dockerfile §9.4; CI §9.6.
Verify: `make docker-build`; run container against compose services with env vars; send SIGTERM → clean shutdown logs.

### Phase 9 — Deploy to Google Cloud Run (Day 4, ~2–3h)
Agent prepares; **[HUMAN]** executes account steps.

1. **[HUMAN]** Create Neon project (region Singapore / `aws-ap-southeast-1`), copy the **direct (non-pooled)** connection string with `sslmode=require`.
2. **[HUMAN]** Create Upstash Redis DB (region Singapore), copy the `rediss://` TCP URL.
3. **[HUMAN]** Run migrations: `DATABASE_URL=<neon-direct-url> make migrate-up`.
4. **[HUMAN]** GCP: create project, enable billing, enable APIs: Cloud Run, Cloud Build, Artifact Registry, Secret Manager.
5. **[HUMAN]** Create secrets `db-url`, `redis-url`, `api-keys`; grant the Cloud Run runtime service account `roles/secretmanager.secretAccessor`.
6. **[HUMAN]** Deploy:
   ```bash
   gcloud run deploy go-wallet \
     --source . \
     --region asia-southeast1 \
     --allow-unauthenticated \
     --max-instances 2 \
     --set-env-vars GIN_MODE=release,DB_MAX_CONNS=5,RATE_LIMIT_PER_MIN=60 \
     --set-secrets DATABASE_URL=db-url:latest,REDIS_URL=redis-url:latest,API_KEYS=api-keys:latest
   ```
7. Agent writes `scripts/smoke.sh <BASE_URL> <API_KEY>`: create 2 wallets, top up, transfer, replay transfer with same key (expect replay header), list entries, call `/readyz`.
8. **Could:** `.github/workflows/deploy.yml` using `google-github-actions/auth` (Workload Identity Federation) + `google-github-actions/deploy-cloudrun`, triggered on tag `v*`.

Deployment notes for README: app must listen on `$PORT`; Neon free compute scales to zero, so first request after idle is slower (warm with `/readyz` before a demo); keep `DB_MAX_CONNS` small because Cloud Run can run multiple instances.

### Phase 10 — README & polish (Day 4, ~2h)
README sections:
1. What it is (2–3 lines) + live URL placeholder.
2. Architecture diagram (Mermaid): HTTP (Gin) → Usecase → Domain; Usecase → ports ← Postgres/Redis adapters.
3. Run locally (`make up`, `make migrate-up`, `make run`) + curl examples.
4. API table (§8.2).
5. **Design decisions & trade-offs** (each 2–4 lines):
   - Clean Architecture with consumer-side interfaces; Gin confined to adapter.
   - `TxManager` with tx-in-context to keep transaction boundary in usecase.
   - Pessimistic row lock `FOR UPDATE` ordered by id → no deadlocks; why not optimistic locking / Redis distributed lock.
   - Idempotency: Redis fast path + DB unique constraint as source of truth; fail-open on Redis errors.
   - Double-entry ledger + `balance_after` for auditability.
   - UUIDv7 for time-ordered IDs → simple keyset pagination.
   - Outbox written in same tx; relay deferred (Cloud Run CPU throttling when idle; production option: Cloud Run Job + Cloud Scheduler or a min-instance worker).
   - Rate limit fixed window: simple, known burst-at-boundary weakness; sliding window/token bucket as next step.
6. Testing strategy (unit vs integration, `-race`, concurrency tests).
7. What I'd do next (auth/JWT, outbox relay, metrics/tracing, multi-currency).

---

## 12. Definition of Done

- [ ] `make lint`, `make arch-check`, `make test`, `make test-integration` all pass locally and in CI.
- [ ] All §10 tests exist and pass with `-race`.
- [ ] `gin`, `pgx`, `go-redis` do not appear in deps of `internal/domain` or `internal/usecase`.
- [ ] Docker image builds; container shuts down gracefully on SIGTERM.
- [ ] `scripts/smoke.sh` passes against local and (after [HUMAN] steps) the Cloud Run URL.
- [ ] README complete per Phase 10.
- [ ] Final summary from agent lists: what was built, any deviations from this plan with reasons, remaining [HUMAN] steps, and Could items not done.
