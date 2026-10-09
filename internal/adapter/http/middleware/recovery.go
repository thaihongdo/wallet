package middleware

import (
	"log/slog"
	"net/http"
	"runtime/debug"

	"github.com/gin-gonic/gin"
)

// Recovery recovers from panics in later handlers, logs the panic value
// with a stack trace, and responds with a generic 500 INTERNAL error
// instead of letting the connection die or leaking panic detail.
func Recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if rec := recover(); rec != nil {
				slog.Error("panic recovered",
					"request_id", GetRequestID(c),
					"panic", rec,
					"stack", string(debug.Stack()),
				)
				RespondError(c, http.StatusInternalServerError, "INTERNAL", "internal server error")
			}
		}()
		c.Next()
	}
}
