package http

import (
	"encoding/base64"
	"fmt"

	"github.com/google/uuid"
)

// encodeCursor renders a ledger entry ID as the opaque, URL-safe cursor
// string returned to API clients.
func encodeCursor(id uuid.UUID) string {
	return base64.RawURLEncoding.EncodeToString(id[:])
}

// decodeCursor parses a cursor string produced by encodeCursor.
func decodeCursor(s string) (uuid.UUID, error) {
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return uuid.UUID{}, fmt.Errorf("decode cursor: %w", err)
	}
	id, err := uuid.FromBytes(b)
	if err != nil {
		return uuid.UUID{}, fmt.Errorf("decode cursor: %w", err)
	}
	return id, nil
}
