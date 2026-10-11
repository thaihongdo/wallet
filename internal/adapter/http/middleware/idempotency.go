package middleware

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// IdempotencyKeyHeader is the request header carrying the client's
// idempotency key. Handlers also read it directly (via
// RequireIdempotencyKey) so key validation does not depend on this
// middleware being wired in.
const IdempotencyKeyHeader = "Idempotency-Key"

const (
	// MinIdempotencyKeyLen is the minimum length of the Idempotency-Key header per §8.4.
	MinIdempotencyKeyLen = 1
	// MaxIdempotencyKeyLen is the maximum length of the Idempotency-Key header per §8.4.
	MaxIdempotencyKeyLen = 128
	// idempotencyTTL is how long a completed (or in-flight) idempotency
	// record is retained, per §8.4.
	idempotencyTTL = 24 * time.Hour
)

// RequireIdempotencyKey reads and validates the Idempotency-Key header,
// writing a 400 IDEMPOTENCY_KEY_REQUIRED response and returning
// ok=false on failure. Shared by this middleware and the handlers it
// protects, so the rule only lives in one place.
func RequireIdempotencyKey(c *gin.Context) (key string, ok bool) {
	key = c.GetHeader(IdempotencyKeyHeader)
	if len(key) < MinIdempotencyKeyLen || len(key) > MaxIdempotencyKeyLen {
		RespondError(c, http.StatusBadRequest, "IDEMPOTENCY_KEY_REQUIRED", "Idempotency-Key header is required (1..128 chars)")
		return "", false
	}
	return key, true
}

// IdemRecord is the state stored under one idempotency key.
type IdemRecord struct {
	Status     string // "processing" | "completed"
	ReqHash    string
	StatusCode int
	Body       []byte
}

// IdempotencyStore persists idempotency records. Implemented by
// adapter/redis; declared here so the middleware does not import redis
// directly.
type IdempotencyStore interface {
	// Begin tries to atomically acquire key for a new, in-flight request.
	// Returns (nil, true, nil) if acquired. Returns (record, false, nil)
	// if the key already exists (either in-flight or completed).
	Begin(ctx context.Context, key, reqHash string, ttl time.Duration) (*IdemRecord, bool, error)
	// Complete stores the final outcome for a previously-Begin'd key.
	Complete(ctx context.Context, key string, rec IdemRecord, ttl time.Duration) error
	// Release frees a key after a failed (5xx) attempt, so the client can
	// retry under the same key.
	Release(ctx context.Context, key string) error
}

// Idempotency enforces exactly-once handling of money-moving requests
// per §8.4. Required on every route it's attached to: a missing or
// malformed Idempotency-Key header is rejected before the handler runs.
//
// If the store errors (e.g. Redis is down), this middleware logs a
// warning and lets the request through unguarded (fail-open): the
// handler's own idempotency key validation and the database's
// UNIQUE(idempotency_key) constraint remain the source of truth for
// correctness, so failing open here only risks a (harmless) duplicate
// in-flight attempt, never a double-spend.
func Idempotency(store IdempotencyStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		key, ok := RequireIdempotencyKey(c)
		if !ok {
			return
		}

		bodyBytes, err := io.ReadAll(c.Request.Body)
		if err != nil {
			RespondError(c, http.StatusBadRequest, "INVALID_REQUEST", "failed to read request body")
			return
		}
		c.Request.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		reqHash := hashBody(bodyBytes)

		storeKey := "idem:" + GetAPIKey(c) + ":" + c.Request.Method + ":" + c.FullPath() + ":" + key

		existing, acquired, err := store.Begin(c.Request.Context(), storeKey, reqHash, idempotencyTTL)
		if err != nil {
			slog.Warn("idempotency store error, failing open", "error", err, "request_id", GetRequestID(c))
			c.Next()
			return
		}

		if !acquired {
			switch existing.Status {
			case "processing":
				RespondError(c, http.StatusConflict, "IDEMPOTENCY_IN_PROGRESS", "a request with this idempotency key is already in progress")
			case "completed":
				if existing.ReqHash != reqHash {
					RespondError(c, http.StatusUnprocessableEntity, "IDEMPOTENCY_KEY_MISMATCH", "idempotency key reused with a different request body")
					return
				}
				c.Header("Idempotent-Replayed", "true")
				c.Data(existing.StatusCode, "application/json", existing.Body)
				c.Abort()
			default:
				// Unknown status: treat conservatively as in-progress.
				RespondError(c, http.StatusConflict, "IDEMPOTENCY_IN_PROGRESS", "a request with this idempotency key is already in progress")
			}
			return
		}

		rec := &responseRecorder{ResponseWriter: c.Writer, body: &bytes.Buffer{}}
		c.Writer = rec
		c.Next()

		status := rec.Status()
		if status >= http.StatusInternalServerError {
			if err := store.Release(c.Request.Context(), storeKey); err != nil {
				slog.Warn("failed to release idempotency key after error", "error", err, "request_id", GetRequestID(c))
			}
			return
		}
		if err := store.Complete(c.Request.Context(), storeKey, IdemRecord{
			Status:     "completed",
			ReqHash:    reqHash,
			StatusCode: status,
			Body:       rec.body.Bytes(),
		}, idempotencyTTL); err != nil {
			slog.Warn("failed to persist idempotency completion", "error", err, "request_id", GetRequestID(c))
		}
	}
}

// hashBody returns a hex-encoded sha256 digest of body, used to detect
// a reused idempotency key sent with a different request.
func hashBody(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

// responseRecorder captures the response body and status so Idempotency
// can store it for later replay, while still writing through to the
// real client.
type responseRecorder struct {
	gin.ResponseWriter
	body *bytes.Buffer
}

func (r *responseRecorder) Write(b []byte) (int, error) {
	r.body.Write(b)
	return r.ResponseWriter.Write(b)
}

func (r *responseRecorder) WriteString(s string) (int, error) {
	r.body.WriteString(s)
	return r.ResponseWriter.WriteString(s)
}
