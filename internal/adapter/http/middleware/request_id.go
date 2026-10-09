// Package middleware holds Gin middleware for the HTTP adapter:
// request ID propagation, structured logging, panic recovery, API-key
// auth, rate limiting, and idempotency.
package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// requestIDContextKey is the Gin context key the request ID is stored
// under, and the HTTP header it is read from / echoed on.
const (
	requestIDContextKey = "request_id"
	// RequestIDHeader is the HTTP header carrying the request ID.
	RequestIDHeader = "X-Request-ID"
)

// RequestID assigns every request a request ID: the incoming
// X-Request-ID header if present, otherwise a freshly generated UUIDv7.
// The ID is stored in the Gin context (read it with GetRequestID) and
// echoed back on the X-Request-ID response header.
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader(RequestIDHeader)
		if id == "" {
			if generated, err := uuid.NewV7(); err == nil {
				id = generated.String()
			}
		}
		c.Set(requestIDContextKey, id)
		c.Header(RequestIDHeader, id)
		c.Next()
	}
}

// GetRequestID returns the request ID stored by RequestID, or "" if the
// middleware has not run (e.g. in a unit test that calls a handler
// directly).
func GetRequestID(c *gin.Context) string {
	return c.GetString(requestIDContextKey)
}
