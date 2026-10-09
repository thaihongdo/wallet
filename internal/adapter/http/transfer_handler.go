package http

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/thaihongdo/go-wallet/internal/adapter/http/middleware"
	"github.com/thaihongdo/go-wallet/internal/usecase"
)

// TransferUsecase is the subset of *usecase.TransferUsecase the handler
// depends on.
type TransferUsecase interface {
	Transfer(ctx context.Context, in usecase.TransferInput) (usecase.TransferResult, error)
}

// TransferHandler serves POST /v1/transfers.
type TransferHandler struct {
	transfers TransferUsecase
}

// NewTransferHandler builds a TransferHandler from its dependency.
func NewTransferHandler(transfers TransferUsecase) *TransferHandler {
	return &TransferHandler{transfers: transfers}
}

// Create handles POST /v1/transfers.
func (h *TransferHandler) Create(c *gin.Context) {
	key, ok := requireIdempotencyKey(c)
	if !ok {
		return
	}

	var req TransferRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		middleware.RespondError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	res, err := h.transfers.Transfer(c.Request.Context(), usecase.TransferInput{
		IdempotencyKey: key,
		FromWalletID:   req.FromWalletID,
		ToWalletID:     req.ToWalletID,
		Amount:         req.Amount,
	})
	if err != nil {
		writeError(c, err)
		return
	}

	if res.Replayed {
		c.Header("Idempotent-Replayed", "true")
	}
	c.JSON(http.StatusCreated, TransferResponse{
		OperationID:  res.Operation.ID,
		FromWalletID: req.FromWalletID,
		ToWalletID:   req.ToWalletID,
		Amount:       req.Amount,
		FromBalance:  res.FromBalance,
		ToBalance:    res.ToBalance,
		CreatedAt:    res.Operation.CreatedAt,
	})
}
