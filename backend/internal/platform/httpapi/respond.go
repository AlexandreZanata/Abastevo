package httpapi

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Pagination bounds (API_PLAN): default 20, maximum 100.
const (
	DefaultLimit = 20
	MaxLimit     = 100
)

// Detail is one machine-readable error field.
type Detail struct {
	Field string `json:"field"`
	Code  string `json:"code"`
}

// WriteError renders the ApiError envelope with a safe display message.
// Coordinates, raw input, SQL and keys never belong in code/message/details;
// callers pass machine codes plus a static safe message.
func WriteError(w http.ResponseWriter, r *http.Request, status int, code, message string, details []Detail) {
	if details == nil {
		details = []Detail{}
	}
	traceID := r.Header.Get("X-Request-ID")
	body, _ := json.Marshal(map[string]any{
		"error": map[string]any{
			"code":     code,
			"message":  message,
			"trace_id": traceID,
			"details":  details,
		},
	})
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

// ParseLimit validates the limit query value with the plan default.
func ParseLimit(raw string) (int, error) {
	if strings.TrimSpace(raw) == "" {
		return DefaultLimit, nil
	}
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || n < 1 || n > MaxLimit {
		return 0, fmt.Errorf("limit must be 1..%d", MaxLimit)
	}
	return n, nil
}

// ParseCursor opens an optional cursor token against the current filters.
func ParseCursor(secret []byte, raw, filterHash string, now time.Time) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return "", nil
	}
	return Open(secret, strings.TrimSpace(raw), filterHash, now)
}

// ETag returns the quoted entity tag for a response body.
func ETag(body []byte) string {
	sum := sha256.Sum256(body)
	return fmt.Sprintf(`W/"%x"`, sum)
}

// IfNoneMatch reports whether the client already holds etag.
func IfNoneMatch(r *http.Request, etag string) bool {
	for _, tag := range strings.Split(r.Header.Get("If-None-Match"), ",") {
		if strings.TrimSpace(tag) == etag || strings.TrimSpace(tag) == "*" {
			return true
		}
	}
	return false
}

// WriteJSON renders a body with ETag handling: matching If-None-Match
// short-circuits to 304 without a body. cacheControl is explicit per route.
func WriteJSON(w http.ResponseWriter, r *http.Request, status int, cacheControl string, body []byte) {
	WriteJSONWithETag(w, r, status, cacheControl, body, ETag(body))
}

// WriteJSONWithETag renders body but keys conditional handling off etag,
// which must summarize the stable content. Use it when the envelope carries
// volatile fields (timestamps, minted page cursors) that must not defeat
// shared caching: identical stable content revalidates to 304 even though
// the wire bytes differ.
func WriteJSONWithETag(w http.ResponseWriter, r *http.Request, status int, cacheControl string, body []byte, etag string) {
	w.Header().Set("ETag", etag)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", cacheControl)
	if (r.Method == http.MethodGet || r.Method == http.MethodHead) && status == http.StatusOK && strings.HasPrefix(cacheControl, "public,") && IfNoneMatch(r, etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.WriteHeader(status)
	_, _ = w.Write(body)
}
