package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// APIKeyHeader is the HTTP header callers must set to a key from the
// configured allow-list.
const APIKeyHeader = "X-API-Key"

// apiKeyContextKey is the Gin context key the authenticated API key is
// stored under.
const apiKeyContextKey = "api_key"

// Auth rejects requests whose X-API-Key header is missing or not in
// keys, with 401 and code UNAUTHORIZED. keys is typically the
// comma-separated API_KEYS env var, pre-split by the caller.
func Auth(keys []string) gin.HandlerFunc {
	allowed := make(map[string]struct{}, len(keys))
	for _, k := range keys {
		allowed[k] = struct{}{}
	}
	return func(c *gin.Context) {
		key := c.GetHeader(APIKeyHeader)
		if key == "" {
			RespondError(c, http.StatusUnauthorized, "UNAUTHORIZED", "missing X-API-Key header")
			return
		}
		if _, ok := allowed[key]; !ok {
			RespondError(c, http.StatusUnauthorized, "UNAUTHORIZED", "invalid API key")
			return
		}
		c.Set(apiKeyContextKey, key)
		c.Next()
	}
}

// GetAPIKey returns the authenticated API key stored by Auth, or "" if
// the middleware has not run.
func GetAPIKey(c *gin.Context) string {
	return c.GetString(apiKeyContextKey)
}
