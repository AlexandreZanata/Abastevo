package http

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	communityread "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/community/adapters/read"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/community/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
)

// FeedHandler exposes only dated public price facts. Signed writes/owner reads
// remain on Handler, with their original proof and privacy requirements.
type FeedHandler struct {
	Summary func(context.Context, application.FeedFilter) ([]communityread.CitySummaryItem, error)
	Read    func(context.Context, application.FeedFilter) ([]communityread.FeedItem, string, error)
	Secrets []byte
}

func (h FeedHandler) RegisterRoutes(r chi.Router) {
	r.Get("/v1/community/feed", h.feed)
	r.Get("/v1/community/summary", h.summary)
}
func (h FeedHandler) feed(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit := 20
	var err error
	if q.Get("limit") != "" {
		limit, err = strconv.Atoi(q.Get("limit"))
	}
	order := q.Get("sort")
	if order == "" {
		order = "recent"
	}
	f, e := application.ValidateFeed(q.Get("state"), q.Get("municipality_code"), q.Get("fuel_product"), order, limit, "")
	if err != nil || e != nil {
		httpapi.WriteError(w, r, 400, "community.bad-feed-filter", "city, fuel, sort or limit invalid", nil)
		return
	}
	fh := httpapi.FilterHash("community-feed-v1", f.State, f.Municipality, f.Product, f.Order, strconv.Itoa(f.Limit))
	key, err := httpapi.ParseCursor(h.Secrets, q.Get("cursor"), fh, time.Now())
	if err == nil {
		f, err = application.ValidateFeed(f.State, f.Municipality, f.Product, f.Order, f.Limit, key)
	}
	if err != nil {
		httpapi.WriteError(w, r, 400, "community.bad-feed-cursor", "cursor invalid, expired or changed", nil)
		return
	}
	if h.Read == nil {
		httpapi.WriteError(w, r, 503, "community.feed-unavailable", "try again later", nil)
		return
	}
	items, key, err := h.Read(r.Context(), f)
	if err != nil {
		httpapi.WriteError(w, r, 503, "community.feed-unavailable", "try again later", nil)
		return
	}
	if items == nil {
		items = []communityread.FeedItem{}
	}
	var next *string
	if key != "" {
		v := httpapi.Seal(h.Secrets, fh, key, time.Now())
		next = &v
	}
	body, _ := json.Marshal(map[string]any{"items": items, "next_cursor": next, "generated_at": generatedNow(),
		"municipality_code": f.Municipality, "state": f.State, "fuel_product": f.Product, "sort": f.Order, "refresh_after_seconds": 15})
	httpapi.WriteJSON(w, r, 200, "no-store", body)
}

// summary is a bounded product population, independent of feed pagination and identity.
func (h FeedHandler) summary(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f, err := application.ValidateFeed(q.Get("state"), q.Get("municipality_code"), "GASOLINE_REGULAR", "recent", 20, "")
	if err != nil {
		httpapi.WriteError(w, r, 400, "community.bad-summary-filter", "city invalid", nil)
		return
	}
	if h.Summary == nil {
		httpapi.WriteError(w, r, 503, "community.summary-unavailable", "try again later", nil)
		return
	}
	items, err := h.Summary(r.Context(), f)
	if err != nil {
		httpapi.WriteError(w, r, 503, "community.summary-unavailable", "try again later", nil)
		return
	}
	if items == nil {
		items = []communityread.CitySummaryItem{}
	}
	body, _ := json.Marshal(map[string]any{"items": items, "generated_at": generatedNow(), "state": f.State, "municipality_code": f.Municipality, "refresh_after_seconds": 15})
	httpapi.WriteJSON(w, r, 200, "no-store", body)
}
