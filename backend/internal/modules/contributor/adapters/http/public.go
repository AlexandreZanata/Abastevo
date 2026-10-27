package http

import (
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
		position, err := httpapi.Open(h.CursorSecret, cursor, httpapi.FilterHash("contributors-v1", ""), time.Now())
		if err != nil {
			writeError(w, r, http.StatusBadRequest, "contributor.invalid")
			return
		}
		stamp, id, ok := strings.Cut(position, ",")
		if !ok || !domain.ValidUUID(id) {
			writeError(w, r, http.StatusBadRequest, "contributor.invalid")
			return
		}
		after, err = time.Parse(time.RFC3339Nano, stamp)
		if err != nil {
			writeError(w, r, http.StatusBadRequest, "contributor.invalid")
			return
		}
		afterID = id
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
		next = httpapi.Seal(h.CursorSecret, httpapi.FilterHash("contributors-v1", ""), last.ContributorSince.UTC().Format(time.RFC3339Nano)+","+last.AccountID, time.Now())
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
