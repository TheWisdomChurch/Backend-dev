// Package pagination provides cursor-based (keyset) pagination helpers.
//
// Cursor format: base64url( unix_µs_hex : uuid )
// Example: "0000019744a1b2c0:550e8400-e29b-41d4-a716-446655440000"
//
// Keyset query pattern:
//
//	WHERE (created_at, id) < (:cursor_ts, :cursor_id)
//	ORDER BY created_at DESC, id DESC
//	LIMIT :limit
package pagination

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Cursor is a decoded pagination cursor.
type Cursor struct {
	CreatedAt time.Time
	ID        string
}

// Encode encodes a (createdAt, id) pair into an opaque base64url cursor string.
func Encode(createdAt time.Time, id string) string {
	raw := fmt.Sprintf("%x:%s", createdAt.UnixMicro(), id)
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

// Decode decodes a cursor string produced by Encode.
// Returns ErrInvalidCursor when the cursor is malformed.
func Decode(cursor string) (*Cursor, error) {
	if strings.TrimSpace(cursor) == "" {
		return nil, ErrInvalidCursor
	}

	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(cursor))
	if err != nil {
		return nil, ErrInvalidCursor
	}

	parts := strings.SplitN(string(raw), ":", 2)
	if len(parts) != 2 {
		return nil, ErrInvalidCursor
	}

	microsHex := parts[0]
	id := parts[1]
	if id == "" {
		return nil, ErrInvalidCursor
	}

	micros, err := strconv.ParseInt(microsHex, 16, 64)
	if err != nil {
		return nil, ErrInvalidCursor
	}

	return &Cursor{
		CreatedAt: time.UnixMicro(micros).UTC(),
		ID:        id,
	}, nil
}

// ErrInvalidCursor is returned when the cursor string cannot be decoded.
var ErrInvalidCursor = errors.New("invalid pagination cursor")

// PageParams holds decoded request pagination parameters ready for use in a
// repository query.
type PageParams struct {
	Limit  int
	Cursor *Cursor // nil means start from the beginning
}

// ParsePageParams normalises limit (clamped to [1, maxLimit]) and decodes the
// cursor string. Returns an error only when the cursor is present but malformed.
func ParsePageParams(cursorStr string, limit, maxLimit int) (PageParams, error) {
	if limit <= 0 {
		limit = 20
	}
	if maxLimit <= 0 {
		maxLimit = 100
	}
	if limit > maxLimit {
		limit = maxLimit
	}

	params := PageParams{Limit: limit}

	if strings.TrimSpace(cursorStr) != "" {
		c, err := Decode(cursorStr)
		if err != nil {
			return PageParams{}, err
		}
		params.Cursor = c
	}

	return params, nil
}
