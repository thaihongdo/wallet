package middleware

import "github.com/gin-gonic/gin"

// ErrorBody is the JSON shape of every non-2xx response produced by
// this service, whether from a handler (via the error mapper) or from
// middleware itself (auth, rate limiting, idempotency, recovery).
type ErrorBody struct {
	Error     ErrorDetail `json:"error"`
	RequestID string      `json:"request_id"`
}

// ErrorDetail carries a machine-readable code and a human message.
type ErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// RespondError writes the standard ErrorBody envelope, stamped with the
// current request ID, and aborts the middleware/handler chain.
func RespondError(c *gin.Context, status int, code, message string) {
	c.AbortWithStatusJSON(status, ErrorBody{
		Error:     ErrorDetail{Code: code, Message: message},
		RequestID: GetRequestID(c),
	})
}
