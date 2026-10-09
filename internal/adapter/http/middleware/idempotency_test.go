package middleware_test

import (
	"bytes"
	"context"
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

func init() {
	gin.SetMode(gin.TestMode)
}

// fakeIdemStore is an in-memory middleware.IdempotencyStore for tests.
type fakeIdemStore struct {
	mu      sync.Mutex
	records map[string]middleware.IdemRecord
}

func newFakeIdemStore() *fakeIdemStore {
	return &fakeIdemStore{records: map[string]middleware.IdemRecord{}}
}

func (s *fakeIdemStore) Begin(_ context.Context, key, reqHash string, _ time.Duration) (*middleware.IdemRecord, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if rec, ok := s.records[key]; ok {
		cp := rec
		return &cp, false, nil
	}
	s.records[key] = middleware.IdemRecord{Status: "processing", ReqHash: reqHash}
	return nil, true, nil
}

func (s *fakeIdemStore) Complete(_ context.Context, key string, rec middleware.IdemRecord, _ time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records[key] = rec
	return nil
}

func (s *fakeIdemStore) Release(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.records, key)
	return nil
}

func newIdempotencyTestRouter(store middleware.IdempotencyStore, handler gin.HandlerFunc) *gin.Engine {
	r := gin.New()
	r.Use(middleware.RequestID())
	r.POST("/test", middleware.Idempotency(store), handler)
	return r
}

func postWithKey(r *gin.Engine, key, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/test", bytes.NewReader([]byte(body)))
	req.Header.Set("Idempotency-Key", key)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func TestIdempotency_FirstCallExecutes_SecondReplays(t *testing.T) {
	store := newFakeIdemStore()
	calls := 0
	r := newIdempotencyTestRouter(store, func(c *gin.Context) {
		calls++
		c.JSON(http.StatusCreated, gin.H{"n": calls})
	})

	first := postWithKey(r, "key-1", `{"a":1}`)
	require.Equal(t, http.StatusCreated, first.Code)
	assert.Empty(t, first.Header().Get("Idempotent-Replayed"))

	second := postWithKey(r, "key-1", `{"a":1}`)
	require.Equal(t, http.StatusCreated, second.Code)
	assert.Equal(t, "true", second.Header().Get("Idempotent-Replayed"))
	assert.JSONEq(t, first.Body.String(), second.Body.String())
	assert.Equal(t, 1, calls, "handler must run exactly once")
}

func TestIdempotency_SameKeyDifferentBody_422(t *testing.T) {
	store := newFakeIdemStore()
	r := newIdempotencyTestRouter(store, func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	first := postWithKey(r, "key-1", `{"a":1}`)
	require.Equal(t, http.StatusOK, first.Code)

	second := postWithKey(r, "key-1", `{"a":2}`)
	assert.Equal(t, http.StatusUnprocessableEntity, second.Code)
}

func TestIdempotency_KeyInProgress_409(t *testing.T) {
	store := newFakeIdemStore()
	started := make(chan struct{})
	proceed := make(chan struct{})
	r := newIdempotencyTestRouter(store, func(c *gin.Context) {
		close(started)
		<-proceed
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	var wg sync.WaitGroup
	var firstRec *httptest.ResponseRecorder
	wg.Add(1)
	go func() {
		defer wg.Done()
		firstRec = postWithKey(r, "key-1", `{}`)
	}()

	<-started
	second := postWithKey(r, "key-1", `{}`)
	assert.Equal(t, http.StatusConflict, second.Code)

	close(proceed)
	wg.Wait()
	assert.Equal(t, http.StatusOK, firstRec.Code)
}

func TestIdempotency_HandlerError500_KeyReleased(t *testing.T) {
	store := newFakeIdemStore()
	attempt := 0
	r := newIdempotencyTestRouter(store, func(c *gin.Context) {
		attempt++
		if attempt == 1 {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "boom"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	first := postWithKey(r, "key-1", `{}`)
	require.Equal(t, http.StatusInternalServerError, first.Code)

	// Key must have been released: a retry with the same key runs the
	// handler again instead of being rejected or replayed.
	second := postWithKey(r, "key-1", `{}`)
	assert.Equal(t, http.StatusOK, second.Code)
	assert.Equal(t, 2, attempt)
}

func TestIdempotency_MissingKey_400(t *testing.T) {
	store := newFakeIdemStore()
	r := newIdempotencyTestRouter(store, func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest(http.MethodPost, "/test", bytes.NewReader([]byte(`{}`)))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}
