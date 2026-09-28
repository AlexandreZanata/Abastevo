package httpapi

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// CursorTTL bounds how long a page cursor stays usable.
const CursorTTL = 15 * time.Minute

var (
	ErrBadCursor    = errors.New("httpapi: invalid cursor")
	ErrCursorExpiry = errors.New("httpapi: expired cursor")
	ErrCursorFilter = errors.New("httpapi: cursor does not match filters")
)

// cursorPayload is the signed cursor body. FilterHash binds the cursor to
// the exact validated filters; LastKey carries the opaque sort position.
type cursorPayload struct {
	FilterHash string `json:"fh"`
	LastKey    string `json:"lk"`
	ExpiresAt  int64  `json:"exp"`
}

// Seal mints an opaque cursor for filterHash positioned after lastKey.
func Seal(secret []byte, filterHash, lastKey string, now time.Time) string {
	body, _ := json.Marshal(cursorPayload{
		FilterHash: filterHash,
		LastKey:    lastKey,
		ExpiresAt:  now.Add(CursorTTL).Unix(),
	})
	mac := hmac.New(sha256.New, secret)
	mac.Write(body)
	return base64.RawURLEncoding.EncodeToString(append(mac.Sum(nil), body...))
}

// Open verifies a cursor against the current filters, returning the sort
// position. Tampered, expired and filter-changed cursors fail closed.
func Open(secret []byte, token, filterHash string, now time.Time) (string, error) {
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(raw) <= sha256.Size {
		return "", ErrBadCursor
	}
	sum, body := raw[:sha256.Size], raw[sha256.Size:]
	mac := hmac.New(sha256.New, secret)
	mac.Write(body)
	if !hmac.Equal(mac.Sum(nil), sum) {
		return "", ErrBadCursor
	}
	var p cursorPayload
	if err := json.Unmarshal(body, &p); err != nil {
		return "", ErrBadCursor
	}
	if p.FilterHash == "" || p.FilterHash != filterHash {
		return "", ErrCursorFilter
	}
	if now.Unix() > p.ExpiresAt {
		return "", ErrCursorExpiry
	}
	return p.LastKey, nil
}

// FilterHash summarizes validated filters so cursors cannot wander across
// queries. Inputs must already be validated and canonical.
func FilterHash(parts ...string) string {
	h := sha256.New()
	for i, p := range parts {
		if i > 0 {
			h.Write([]byte{0})
		}
		fmt.Fprint(h, p)
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}
