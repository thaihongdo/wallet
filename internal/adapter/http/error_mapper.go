package http

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/thaihongdo/go-wallet/internal/adapter/http/middleware"
	"github.com/thaihongdo/go-wallet/internal/domain"
)

// errorMapping pairs an HTTP status with a machine-readable code for one
// sentinel domain error.
type errorMapping struct {
	status int
	code   string
}

// domainErrorMappings is checked in order; the first matching sentinel
// (via errors.Is) wins. Order matters because some errors (e.g.
// ErrInvalidAmount) could also satisfy a looser, later check.
var domainErrorMappings = []struct {
	err error
	errorMapping
}{
	{domain.ErrInvalidInput, errorMapping{http.StatusBadRequest, "INVALID_REQUEST"}},
	{domain.ErrInvalidAmount, errorMapping{http.StatusBadRequest, "INVALID_AMOUNT"}},
	{domain.ErrSameWallet, errorMapping{http.StatusBadRequest, "SAME_WALLET"}},
	{domain.ErrWalletNotFound, errorMapping{http.StatusNotFound, "WALLET_NOT_FOUND"}},
	{domain.ErrInsufficientBalance, errorMapping{http.StatusUnprocessableEntity, "INSUFFICIENT_BALANCE"}},
	{domain.ErrCurrencyMismatch, errorMapping{http.StatusUnprocessableEntity, "CURRENCY_MISMATCH"}},
	{domain.ErrBalanceOverflow, errorMapping{http.StatusUnprocessableEntity, "BALANCE_OVERFLOW"}},
}

// writeError maps err to an HTTP status and error code per §8.3, writes
// the standard error body, and logs the full (unmapped) error. Unmapped
// errors become a generic 500 INTERNAL response; their detail is never
// leaked to the client.
func writeError(c *gin.Context, err error) {
	for _, m := range domainErrorMappings {
		if errors.Is(err, m.err) {
			middleware.RespondError(c, m.status, m.code, m.err.Error())
			return
		}
	}

	slog.Error("unhandled error", "error", err, "request_id", middleware.GetRequestID(c))
	middleware.RespondError(c, http.StatusInternalServerError, "INTERNAL", "internal server error")
}
