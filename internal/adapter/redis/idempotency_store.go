package redis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/thaihongdo/go-wallet/internal/adapter/http/middleware"
)

// storedRecord is the JSON shape persisted in Redis for one idempotency
// key.
type storedRecord struct {
	Status     string `json:"status"`
	ReqHash    string `json:"hash"`
	StatusCode int    `json:"status_code,omitempty"`
	Body       []byte `json:"body,omitempty"`
}

// IdempotencyStore implements middleware.IdempotencyStore on Redis.
type IdempotencyStore struct {
	client *redis.Client
}

// NewIdempotencyStore builds an IdempotencyStore backed by client.
func NewIdempotencyStore(client *redis.Client) *IdempotencyStore {
	return &IdempotencyStore{client: client}
}

// Begin tries SET key {...} NX EX ttl. Returns (nil, true, nil) if
// acquired, or (record, false, nil) with the existing record if the key
// is already present (in-flight or completed).
func (s *IdempotencyStore) Begin(ctx context.Context, key, reqHash string, ttl time.Duration) (*middleware.IdemRecord, bool, error) {
	payload, err := json.Marshal(storedRecord{Status: "processing", ReqHash: reqHash})
	if err != nil {
		return nil, false, fmt.Errorf("marshal idempotency record: %w", err)
	}

	ok, err := s.client.SetNX(ctx, key, payload, ttl).Result()
	if err != nil {
		return nil, false, fmt.Errorf("begin idempotency key: %w", err)
	}
	if ok {
		return nil, true, nil
	}

	raw, err := s.client.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		// Key expired/was deleted between SetNX and Get; treat as acquired
		// by retrying once.
		return s.Begin(ctx, key, reqHash, ttl)
	}
	if err != nil {
		return nil, false, fmt.Errorf("get existing idempotency record: %w", err)
	}

	var rec storedRecord
	if err := json.Unmarshal(raw, &rec); err != nil {
		return nil, false, fmt.Errorf("unmarshal idempotency record: %w", err)
	}
	return &middleware.IdemRecord{
		Status:     rec.Status,
		ReqHash:    rec.ReqHash,
		StatusCode: rec.StatusCode,
		Body:       rec.Body,
	}, false, nil
}

// Complete overwrites key with the final, completed record.
func (s *IdempotencyStore) Complete(ctx context.Context, key string, rec middleware.IdemRecord, ttl time.Duration) error {
	payload, err := json.Marshal(storedRecord{
		Status:     rec.Status,
		ReqHash:    rec.ReqHash,
		StatusCode: rec.StatusCode,
		Body:       rec.Body,
	})
	if err != nil {
		return fmt.Errorf("marshal idempotency record: %w", err)
	}
	if err := s.client.Set(ctx, key, payload, ttl).Err(); err != nil {
		return fmt.Errorf("complete idempotency key: %w", err)
	}
	return nil
}

// Release deletes key, allowing the client to retry under the same
// idempotency key after a server error.
func (s *IdempotencyStore) Release(ctx context.Context, key string) error {
	if err := s.client.Del(ctx, key).Err(); err != nil {
		return fmt.Errorf("release idempotency key: %w", err)
	}
	return nil
}
