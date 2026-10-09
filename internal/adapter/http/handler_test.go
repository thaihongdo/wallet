package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	httpadapter "github.com/thaihongdo/go-wallet/internal/adapter/http"
	"github.com/thaihongdo/go-wallet/internal/domain"
	"github.com/thaihongdo/go-wallet/internal/usecase"
)

const testAPIKey = "test-key"

func init() {
	gin.SetMode(gin.TestMode)
}

type testRouter struct {
	wallets   *fakeWalletUsecase
	topUp     *fakeTopUpUsecase
	transfers *fakeTransferUsecase
	engine    *gin.Engine
}

func newTestRouter() *testRouter {
	tr := &testRouter{
		wallets:   &fakeWalletUsecase{},
		topUp:     &fakeTopUpUsecase{},
		transfers: &fakeTransferUsecase{},
	}
	health := httpadapter.NewHealthHandler(&fakePinger{}, &fakePinger{})
	tr.engine = httpadapter.NewRouter(httpadapter.Deps{
		Wallets:   tr.wallets,
		TopUp:     tr.topUp,
		Transfers: tr.transfers,
		Health:    health,
		APIKeys:   []string{testAPIKey},
	})
	return tr
}

func doRequest(t *testing.T, engine *gin.Engine, method, path string, body any, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		require.NoError(t, err)
		reader = bytes.NewReader(b)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	return rec
}

func authHeaders(extra map[string]string) map[string]string {
	h := map[string]string{"X-API-Key": testAPIKey}
	for k, v := range extra {
		h[k] = v
	}
	return h
}

func decodeBody[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var out T
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	return out
}

// --- Happy paths ---

func TestCreateWallet_HappyPath(t *testing.T) {
	tr := newTestRouter()
	now := time.Now()
	id := uuid.Must(uuid.NewV7())
	tr.wallets.createFn = func(ctx context.Context, ownerID string) (*domain.Wallet, error) {
		assert.Equal(t, "user-1", ownerID)
		return &domain.Wallet{ID: id, OwnerID: ownerID, Currency: "VND", Balance: 0, CreatedAt: now, UpdatedAt: now}, nil
	}

	rec := doRequest(t, tr.engine, http.MethodPost, "/v1/wallets", map[string]string{"owner_id": "user-1"}, authHeaders(nil))
	require.Equal(t, http.StatusCreated, rec.Code)
	resp := decodeBody[httpadapter.WalletResponse](t, rec)
	assert.Equal(t, id, resp.ID)
	assert.Equal(t, "VND", resp.Currency)
}

func TestGetWallet_HappyPath(t *testing.T) {
	tr := newTestRouter()
	id := uuid.Must(uuid.NewV7())
	tr.wallets.getFn = func(ctx context.Context, got uuid.UUID) (*domain.Wallet, error) {
		assert.Equal(t, id, got)
		return &domain.Wallet{ID: id, Currency: "VND", Balance: 500}, nil
	}

	rec := doRequest(t, tr.engine, http.MethodGet, "/v1/wallets/"+id.String(), nil, authHeaders(nil))
	require.Equal(t, http.StatusOK, rec.Code)
	resp := decodeBody[httpadapter.WalletResponse](t, rec)
	assert.Equal(t, domain.Money(500), resp.Balance)
}

func TestGetWallet_BadUUID(t *testing.T) {
	tr := newTestRouter()
	rec := doRequest(t, tr.engine, http.MethodGet, "/v1/wallets/not-a-uuid", nil, authHeaders(nil))
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestTopUp_HappyPath(t *testing.T) {
	tr := newTestRouter()
	walletID := uuid.Must(uuid.NewV7())
	opID := uuid.Must(uuid.NewV7())
	tr.topUp.fn = func(ctx context.Context, in usecase.TopUpInput) (usecase.TopUpResult, error) {
		assert.Equal(t, "idem-1", in.IdempotencyKey)
		assert.Equal(t, walletID, in.WalletID)
		assert.Equal(t, domain.Money(100000), in.Amount)
		return usecase.TopUpResult{Operation: domain.Operation{ID: opID}, Balance: 100000}, nil
	}
	tr.wallets.getFn = func(ctx context.Context, id uuid.UUID) (*domain.Wallet, error) {
		return &domain.Wallet{ID: walletID, Currency: "VND", Balance: 100000}, nil
	}

	rec := doRequest(t, tr.engine, http.MethodPost, "/v1/wallets/"+walletID.String()+"/topup",
		map[string]int64{"amount": 100000}, authHeaders(map[string]string{"Idempotency-Key": "idem-1"}))
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Empty(t, rec.Header().Get("Idempotent-Replayed"))
	resp := decodeBody[httpadapter.TopUpResponse](t, rec)
	assert.Equal(t, opID, resp.OperationID)
}

func TestTopUp_Replayed_SetsHeader(t *testing.T) {
	tr := newTestRouter()
	walletID := uuid.Must(uuid.NewV7())
	tr.topUp.fn = func(ctx context.Context, in usecase.TopUpInput) (usecase.TopUpResult, error) {
		return usecase.TopUpResult{Operation: domain.Operation{ID: uuid.Must(uuid.NewV7())}, Replayed: true}, nil
	}
	tr.wallets.getFn = func(ctx context.Context, id uuid.UUID) (*domain.Wallet, error) {
		return &domain.Wallet{ID: walletID}, nil
	}

	rec := doRequest(t, tr.engine, http.MethodPost, "/v1/wallets/"+walletID.String()+"/topup",
		map[string]int64{"amount": 1}, authHeaders(map[string]string{"Idempotency-Key": "idem-1"}))
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "true", rec.Header().Get("Idempotent-Replayed"))
}

func TestTransfer_HappyPath(t *testing.T) {
	tr := newTestRouter()
	from, to := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())
	opID := uuid.Must(uuid.NewV7())
	tr.transfers.fn = func(ctx context.Context, in usecase.TransferInput) (usecase.TransferResult, error) {
		assert.Equal(t, from, in.FromWalletID)
		assert.Equal(t, to, in.ToWalletID)
		return usecase.TransferResult{
			Operation:   domain.Operation{ID: opID, CreatedAt: time.Now()},
			FromBalance: 700, ToBalance: 800,
		}, nil
	}

	body := map[string]any{"from_wallet_id": from, "to_wallet_id": to, "amount": 300}
	rec := doRequest(t, tr.engine, http.MethodPost, "/v1/transfers", body, authHeaders(map[string]string{"Idempotency-Key": "idem-1"}))
	require.Equal(t, http.StatusCreated, rec.Code)
	resp := decodeBody[httpadapter.TransferResponse](t, rec)
	assert.Equal(t, opID, resp.OperationID)
	assert.Equal(t, domain.Money(700), resp.FromBalance)
	assert.Equal(t, domain.Money(800), resp.ToBalance)
}

func TestListEntries_HappyPath(t *testing.T) {
	tr := newTestRouter()
	walletID := uuid.Must(uuid.NewV7())
	entryID := uuid.Must(uuid.NewV7())
	tr.wallets.listFn = func(ctx context.Context, got uuid.UUID, cursor *uuid.UUID, limit int) ([]domain.LedgerEntry, *uuid.UUID, error) {
		assert.Equal(t, walletID, got)
		return []domain.LedgerEntry{{ID: entryID, Direction: domain.DirectionCredit, Amount: 10, BalanceAfter: 10}}, nil, nil
	}

	rec := doRequest(t, tr.engine, http.MethodGet, "/v1/wallets/"+walletID.String()+"/entries?limit=20", nil, authHeaders(nil))
	require.Equal(t, http.StatusOK, rec.Code)
	resp := decodeBody[httpadapter.ListEntriesResponse](t, rec)
	require.Len(t, resp.Items, 1)
	assert.Equal(t, entryID, resp.Items[0].ID)
	assert.Nil(t, resp.NextCursor)
}

func TestHealthz_Public_NoAuthRequired(t *testing.T) {
	tr := newTestRouter()
	rec := doRequest(t, tr.engine, http.MethodGet, "/healthz", nil, nil)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestReadyz_Ok(t *testing.T) {
	tr := newTestRouter()
	rec := doRequest(t, tr.engine, http.MethodGet, "/readyz", nil, nil)
	assert.Equal(t, http.StatusOK, rec.Code)
}

// --- Auth ---

func TestMissingAPIKey_Returns401(t *testing.T) {
	tr := newTestRouter()
	rec := doRequest(t, tr.engine, http.MethodGet, "/v1/wallets/"+uuid.Must(uuid.NewV7()).String(), nil, nil)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestWrongAPIKey_Returns401(t *testing.T) {
	tr := newTestRouter()
	rec := doRequest(t, tr.engine, http.MethodGet, "/v1/wallets/"+uuid.Must(uuid.NewV7()).String(), nil, map[string]string{"X-API-Key": "wrong"})
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

// --- Error mapping (§8.3) ---

func TestErrorMapping(t *testing.T) {
	cases := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{"invalid input", domain.ErrInvalidInput, http.StatusBadRequest, "INVALID_REQUEST"},
		{"invalid amount", domain.ErrInvalidAmount, http.StatusBadRequest, "INVALID_AMOUNT"},
		{"same wallet", domain.ErrSameWallet, http.StatusBadRequest, "SAME_WALLET"},
		{"wallet not found", domain.ErrWalletNotFound, http.StatusNotFound, "WALLET_NOT_FOUND"},
		{"insufficient balance", domain.ErrInsufficientBalance, http.StatusUnprocessableEntity, "INSUFFICIENT_BALANCE"},
		{"currency mismatch", domain.ErrCurrencyMismatch, http.StatusUnprocessableEntity, "CURRENCY_MISMATCH"},
		{"balance overflow", domain.ErrBalanceOverflow, http.StatusUnprocessableEntity, "BALANCE_OVERFLOW"},
		{"unknown error", errors.New("boom"), http.StatusInternalServerError, "INTERNAL"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tr := newTestRouter()
			id := uuid.Must(uuid.NewV7())
			tr.wallets.getFn = func(ctx context.Context, got uuid.UUID) (*domain.Wallet, error) {
				return nil, tc.err
			}
			rec := doRequest(t, tr.engine, http.MethodGet, "/v1/wallets/"+id.String(), nil, authHeaders(nil))
			assert.Equal(t, tc.wantStatus, rec.Code)
			resp := decodeBody[httpadapter.ErrorBody](t, rec)
			assert.Equal(t, tc.wantCode, resp.Error.Code)
			assert.NotEmpty(t, resp.RequestID)
			if tc.wantStatus == http.StatusInternalServerError {
				assert.NotContains(t, rec.Body.String(), "boom", "internal error detail must not leak")
			}
		})
	}
}

func TestTopUp_MissingIdempotencyKey(t *testing.T) {
	tr := newTestRouter()
	walletID := uuid.Must(uuid.NewV7())
	rec := doRequest(t, tr.engine, http.MethodPost, "/v1/wallets/"+walletID.String()+"/topup",
		map[string]int64{"amount": 1}, authHeaders(nil))
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	resp := decodeBody[httpadapter.ErrorBody](t, rec)
	assert.Equal(t, "IDEMPOTENCY_KEY_REQUIRED", resp.Error.Code)
}

func TestTransfer_MissingIdempotencyKey(t *testing.T) {
	tr := newTestRouter()
	body := map[string]any{"from_wallet_id": uuid.Must(uuid.NewV7()), "to_wallet_id": uuid.Must(uuid.NewV7()), "amount": 1}
	rec := doRequest(t, tr.engine, http.MethodPost, "/v1/transfers", body, authHeaders(nil))
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	resp := decodeBody[httpadapter.ErrorBody](t, rec)
	assert.Equal(t, "IDEMPOTENCY_KEY_REQUIRED", resp.Error.Code)
}

func TestXRequestID_EchoedOnResponse(t *testing.T) {
	tr := newTestRouter()
	rec := doRequest(t, tr.engine, http.MethodGet, "/healthz", nil, map[string]string{"X-Request-ID": "my-request-id"})
	assert.Equal(t, "my-request-id", rec.Header().Get("X-Request-ID"))
}

func TestXRequestID_GeneratedWhenAbsent(t *testing.T) {
	tr := newTestRouter()
	rec := doRequest(t, tr.engine, http.MethodGet, "/healthz", nil, nil)
	assert.NotEmpty(t, rec.Header().Get("X-Request-ID"))
}
