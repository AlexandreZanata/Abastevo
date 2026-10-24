package http

import (
	"encoding/json"
	"net/http"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/charging/adapters/prepared"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// HistoryHandler serves public community observation history (projected
// with tallies, hidden/deleted/expired excluded). Anonymous reads;
// reporters stay anonymous (no account IDs). Freshness is explicit:
// items carry observed days; stale data is not refreshed here.
type HistoryHandler struct {
	Pool    *pgxpool.Pool
	Enabled func() bool
}

// RegisterHistoryRoutes mounts community history without disturbing
// tariff ranking or other surfaces.
func (h HistoryHandler) RegisterHistoryRoutes(r chi.Router) {
	r.Get("/v1/charging/observations/history", h.history)
}

func (h HistoryHandler) history(w http.ResponseWriter, r *http.Request) {
	if h.Enabled != nil && !h.Enabled() {
		httpapi.WriteError(w, r, http.StatusNotFound, "charging.disabled", "charging reads disabled", nil)
		return
	}
	q := r.URL.Query()
	for key := range q {
		switch key {
		case "place_id", "limit":
		default:
			httpapi.WriteError(w, r, http.StatusBadRequest, "charging.bad-filter", "unsupported query parameter", nil)
			return
		}
	}
	placeID := q.Get("place_id")
	if placeID == "" {
		httpapi.WriteError(w, r, http.StatusBadRequest, "charging.bad-filter", "place_id required",
			[]httpapi.Detail{{Field: "place_id", Code: "required"}})
		return
	}
	limit, err := httpapi.ParseLimit(q.Get("limit"))
	if err != nil {
		httpapi.WriteError(w, r, http.StatusBadRequest, "charging.bad-limit", "limit must be 1..100",
			[]httpapi.Detail{{Field: "limit", Code: "range"}})
		return
	}
	items, err := prepared.Project(r.Context(), h.Pool, placeID, 3650, limit)
	if err != nil {
		httpapi.WriteError(w, r, http.StatusServiceUnavailable, "charging.unavailable", "history unavailable", nil)
		return
	}
	wire := make([]map[string]any, 0, len(items))
	for _, item := range items {
		entry := map[string]any{"id": item.ID, "connector_code": item.ConnectorCode,
			"condition": item.Condition, "observed_day": item.ObservedDay, "note": item.Note,
			"helpful": item.Helpful, "not_helpful": item.NotHelpful}
		if item.PriceMilliBRL != nil {
			entry["price_milli_brl_per_kwh"] = *item.PriceMilliBRL
		}
		wire = append(wire, entry)
	}
	body, _ := json.Marshal(map[string]any{"items": wire})
	httpapi.WriteJSON(w, r, http.StatusOK, "public, max-age=60", body)
}
