package http

import (
	"context"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/thaihongdo/go-wallet/internal/adapter/http/middleware"
	"github.com/thaihongdo/go-wallet/internal/domain"
	"github.com/thaihongdo/go-wallet/internal/usecase"
)

// WalletUsecase is the subset of *usecase.WalletUsecase the handlers
// depend on, declared here so handlers can be tested against a fake.
type WalletUsecase interface {
	CreateWallet(ctx context.Context, ownerID string) (*domain.Wallet, error)
	GetWallet(ctx context.Context, id uuid.UUID) (*domain.Wallet, error)
	ListEntries(ctx context.Context, walletID uuid.UUID, cursor *uuid.UUID, limit int) ([]domain.LedgerEntry, *uuid.UUID, error)
}

// TopUpUsecase is the subset of *usecase.TopUpUsecase the handlers
// depend on.
type TopUpUsecase interface {
	TopUp(ctx context.Context, in usecase.TopUpInput) (usecase.TopUpResult, error)
}

// WalletHandler serves the wallet- and ledger-related endpoints.
type WalletHandler struct {
	wallets WalletUsecase
	topUp   TopUpUsecase
}

// NewWalletHandler builds a WalletHandler from its dependencies.
func NewWalletHandler(wallets WalletUsecase, topUp TopUpUsecase) *WalletHandler {
	return &WalletHandler{wallets: wallets, topUp: topUp}
}

// Create handles POST /v1/wallets.
func (h *WalletHandler) Create(c *gin.Context) {
	var req CreateWalletRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		middleware.RespondError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	w, err := h.wallets.CreateWallet(c.Request.Context(), req.OwnerID)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusCreated, walletToResponse(w))
}

// Get handles GET /v1/wallets/:id.
func (h *WalletHandler) Get(c *gin.Context) {
	id, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}

	w, err := h.wallets.GetWallet(c.Request.Context(), id)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, walletToResponse(w))
}

// TopUp handles POST /v1/wallets/:id/topup.
func (h *WalletHandler) TopUp(c *gin.Context) {
	id, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}

	key, ok := middleware.RequireIdempotencyKey(c)
	if !ok {
		return
	}

	var req TopUpRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		middleware.RespondError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	res, err := h.topUp.TopUp(c.Request.Context(), usecase.TopUpInput{
		IdempotencyKey: key,
		WalletID:       id,
		Amount:         req.Amount,
	})
	if err != nil {
		writeError(c, err)
		return
	}

	if res.Replayed {
		c.Header("Idempotent-Replayed", "true")
	}
	w, err := h.wallets.GetWallet(c.Request.Context(), id)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, TopUpResponse{
		OperationID: res.Operation.ID,
		Wallet:      walletToResponse(w),
	})
}

// ListEntries handles GET /v1/wallets/:id/entries.
func (h *WalletHandler) ListEntries(c *gin.Context) {
	id, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}

	limit := 0
	if raw := c.Query("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil {
			middleware.RespondError(c, http.StatusBadRequest, "INVALID_REQUEST", "limit must be an integer")
			return
		}
		limit = n
	}

	var cursor *uuid.UUID
	if raw := c.Query("cursor"); raw != "" {
		decoded, err := decodeCursor(raw)
		if err != nil {
			middleware.RespondError(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid cursor")
			return
		}
		cursor = &decoded
	}

	entries, next, err := h.wallets.ListEntries(c.Request.Context(), id, cursor, limit)
	if err != nil {
		writeError(c, err)
		return
	}

	items := make([]EntryResponse, len(entries))
	for i, e := range entries {
		items[i] = entryToResponse(e)
	}
	var nextCursor *string
	if next != nil {
		s := encodeCursor(*next)
		nextCursor = &s
	}
	c.JSON(http.StatusOK, ListEntriesResponse{Items: items, NextCursor: nextCursor})
}

// parseUUIDParam parses the named path parameter as a UUID, writing a
// 400 INVALID_REQUEST response and returning ok=false on failure.
func parseUUIDParam(c *gin.Context, name string) (id uuid.UUID, ok bool) {
	id, err := uuid.Parse(c.Param(name))
	if err != nil {
		middleware.RespondError(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid "+name)
		return uuid.UUID{}, false
	}
	return id, true
}
