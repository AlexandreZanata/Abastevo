package domain

import (
	"strconv"
	"strings"
	"time"
)

// Position is the keyset position of the last item of a page: the feed is
// ordered by (created_at, id) descending, and the next page starts strictly
// after it. The HTTP adapter seals the encoded key into the opaque,
// authenticated, owner-bound `before` cursor.
type Position struct {
	CreatedAt time.Time
	ID        string
}

const positionPrefix = "n1|"

// maxPositionKey bounds a decoded key: prefix, 20 digits, separator, UUID.
const maxPositionKey = 64

// EncodePosition renders the position as its stable sort key.
func EncodePosition(p Position) string {
	return positionPrefix + strconv.FormatInt(p.CreatedAt.UTC().UnixMicro(), 10) + "|" + p.ID
}

// DecodePosition parses a key produced by EncodePosition; anything else is
// invalid.
func DecodePosition(key string) (Position, error) {
	if len(key) > maxPositionKey || !strings.HasPrefix(key, positionPrefix) {
		return Position{}, ErrInvalid
	}
	micros, id, ok := strings.Cut(strings.TrimPrefix(key, positionPrefix), "|")
	if !ok || !IsUUID(id) || micros == "" || (len(micros) > 1 && micros[0] == '0') {
		return Position{}, ErrInvalid
	}
	n, err := strconv.ParseInt(micros, 10, 64)
	if err != nil || n <= 0 {
		return Position{}, ErrInvalid
	}
	return Position{CreatedAt: time.UnixMicro(n).UTC(), ID: id}, nil
}
