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
