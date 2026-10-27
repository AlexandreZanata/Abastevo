package http

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/contributor/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/contributor/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
	"github.com/go-chi/chi/v5"
)

type PublicHandler struct {
	Service        *application.Service
	CursorSecret   []byte
	AllowedOrigins []string
}

func (h PublicHandler) RegisterRoutes(r chi.Router) {
	r.With(h.cors).Get("/v1/contributors", h.list)
	r.With(h.cors).Options("/v1/contributors", h.preflight)
}

func (h PublicHandler) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Vary", "Origin")
		origin := r.Header.Get("Origin")
		if origin != "" && slices.Contains(h.AllowedOrigins, origin) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "GET")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		}
		next.ServeHTTP(w, r)
	})
}

func (h PublicHandler) preflight(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}

func (h PublicHandler) list(w http.ResponseWriter, r *http.Request) {
	limit, ok := parseLimit(r.URL.Query().Get("limit"), 20, 50)
	if !ok {
		writeError(w, r, http.StatusBadRequest, "contributor.invalid")
		return
	}
	var after time.Time
	afterID := ""
	cursor := strings.TrimSpace(r.URL.Query().Get("cursor"))
	if cursor != "" {
		if len(h.CursorSecret) < 32 {
			writeError(w, r, http.StatusServiceUnavailable, "contributor.unavailable")
			return
		}
		var err error
		after, afterID, err = openPublicCursor(h.CursorSecret, cursor, time.Now())
		if err != nil {
			writeError(w, r, http.StatusBadRequest, "contributor.invalid")
			return
		}
	}
	items, err := h.Service.PublicPage(r.Context(), after, afterID, limit+1)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	next := ""
	if len(items) > limit {
		items = items[:limit]
		last := items[len(items)-1]
		sealed, err := sealPublicCursor(h.CursorSecret, last.ContributorSince, last.AccountID, time.Now())
		if err != nil {
			writeError(w, r, http.StatusServiceUnavailable, "contributor.unavailable")
			return
		}
		next = sealed
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		entry := map[string]any{
			"display_name":      item.DisplayName,
			"links":             item.Links,
			"contributor_since": item.ContributorSince.UTC().Format("2006-01"),
			"roles":             item.Roles,
		}
		if item.GitHubLogin != "" {
			entry["github_login"] = item.GitHubLogin
		}
		out = append(out, entry)
	}
	w.Header().Set("Cache-Control", "public, max-age=300")
	writeJSON(w, http.StatusOK, map[string]any{"items": out, "next_cursor": next})
}

// The public list must not reveal account ids (CTB-10), but keyset paging
// resumes from (contributor_since, account_id). The position is therefore
// encrypted with AES-GCM under a key derived from the cursor secret before
// the usual sealed envelope adds expiry and filter binding.
var errPublicCursor = errors.New("contributor: bad cursor")

func publicCursorAEAD(secret []byte) (cipher.AEAD, error) {
	key := sha256.Sum256(append([]byte("abastevo/contributors-cursor/v1\x00"), secret...))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func sealPublicCursor(secret []byte, since time.Time, accountID string, now time.Time) (string, error) {
	aead, err := publicCursorAEAD(secret)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	plain := []byte(since.UTC().Format(time.RFC3339Nano) + "," + accountID)
	position := base64.RawURLEncoding.EncodeToString(aead.Seal(nonce, nonce, plain, nil))
	return httpapi.Seal(secret, httpapi.FilterHash("contributors-v2", ""), position, now), nil
}

func openPublicCursor(secret []byte, token string, now time.Time) (time.Time, string, error) {
	position, err := httpapi.Open(secret, token, httpapi.FilterHash("contributors-v2", ""), now)
	if err != nil {
		return time.Time{}, "", errPublicCursor
	}
	raw, err := base64.RawURLEncoding.DecodeString(position)
	aead, aeadErr := publicCursorAEAD(secret)
	if err != nil || aeadErr != nil || len(raw) < aead.NonceSize() {
		return time.Time{}, "", errPublicCursor
	}
	plain, err := aead.Open(nil, raw[:aead.NonceSize()], raw[aead.NonceSize():], nil)
	if err != nil {
		return time.Time{}, "", errPublicCursor
	}
	stamp, id, ok := strings.Cut(string(plain), ",")
	if !ok || !domain.ValidUUID(id) {
		return time.Time{}, "", errPublicCursor
	}
	since, err := time.Parse(time.RFC3339Nano, stamp)
	if err != nil {
		return time.Time{}, "", errPublicCursor
	}
	return since, id, nil
}
