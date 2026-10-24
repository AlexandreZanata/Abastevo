package http

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/charging/adapters/read"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
	"github.com/go-chi/chi/v5"
)

// Handler serves bounded public charging/reference reads. Disabled by
// configuration (404 without touching storage); cursors and ETags bind
// full filters and batch; no ingestion, provider fan-out or private
// fields occur here.
type Handler struct {
	Read    read.Reader
	Secrets []byte
	Now     func() time.Time
	Enabled func() bool
}

// RegisterRoutes mounts the charging read surface without disturbing
// fuel, ANP or Directory routes.
func (h Handler) RegisterRoutes(r chi.Router) {
	r.Get("/v1/charging/places", h.places)
	r.Get("/v1/charging/places/{id}", h.placeOne)
	r.Get("/v1/charging/coverage", h.coverage)
}

func (h Handler) disabled(w http.ResponseWriter, r *http.Request) bool {
	if h.Enabled != nil && !h.Enabled() {
		httpapi.WriteError(w, r, http.StatusNotFound, "charging.disabled", "charging reads disabled", nil)
		return true
	}
	return false
}

func (h Handler) places(w http.ResponseWriter, r *http.Request) {
	if h.disabled(w, r) {
		return
	}
	q := r.URL.Query()
	for key := range q {
		switch key {
		case "profile", "revision", "limit", "cursor":
		default:
			httpapi.WriteError(w, r, http.StatusBadRequest, "charging.bad-filter", "unsupported query parameter", nil)
			return
		}
	}
	profile := q.Get("profile")
	if profile != "ocm-br-20261021-v1" && profile != "pbe-2026-pdf-v1" {
		httpapi.WriteError(w, r, http.StatusBadRequest, "charging.bad-filter", "unknown profile",
			[]httpapi.Detail{{Field: "profile", Code: "vocabulary"}})
		return
	}
	limit, err := httpapi.ParseLimit(q.Get("limit"))
	if err != nil {
		httpapi.WriteError(w, r, http.StatusBadRequest, "charging.bad-limit", "limit must be 1..100",
			[]httpapi.Detail{{Field: "limit", Code: "range"}})
		return
	}
	revision := q.Get("revision")
	fh := httpapi.FilterHash("charging-places", profile, revision, strconv.Itoa(limit))
	lastKey, err := httpapi.ParseCursor(h.Secrets, q.Get("cursor"), fh, h.now())
	if err != nil {
		httpapi.WriteError(w, r, http.StatusBadRequest, "charging.bad-cursor", "cursor invalid, expired or changed",
			[]httpapi.Detail{{Field: "cursor", Code: "invalid"}})
		return
	}
	rows, next, err := h.Read.Places(r.Context(), profile, revision, lastKey, limit)
	if err != nil {
		sourceDown(w, r)
		return
	}
	wire := make([]map[string]any, 0, len(rows))
	for _, place := range rows {
		wire = append(wire, map[string]any{"id": place.ID, "entity_key": place.EntityKey, "external_id": place.ExternalID,
			"name": place.Name, "country": place.Country, "latitude": place.Latitude, "longitude": place.Longitude,
			"public_access": place.PublicAccess, "reported_points": place.ReportedPoints})
	}
	var nextCursor *string
	if next != "" {
		sealed := httpapi.Seal(h.Secrets, fh, next, h.now())
		nextCursor = &sealed
	}
	body, _ := json.Marshal(map[string]any{"items": wire, "next_cursor": nextCursor, "generated_at": generatedAt(h.now())})
	httpapi.WriteJSONWithETag(w, r, http.StatusOK, "public, max-age=60", body,
		httpapi.ETag(mustMarshal(map[string]any{"items": wire, "next_cursor": next})))
}

func (h Handler) placeOne(w http.ResponseWriter, r *http.Request) {
	if h.disabled(w, r) {
		return
	}
	id := chi.URLParam(r, "id")
	place, err := h.Read.PlaceDetail(r.Context(), id)
	if err != nil {
		sourceDown(w, r)
		return
	}
	groups := make([]map[string]any, 0, len(place.Groups))
	for _, group := range place.Groups {
		item := map[string]any{"id": group.ID, "entity_key": group.EntityKey, "external_id": group.ExternalID,
			"connector_code": group.ConnectorCode, "raw_connector_code": group.RawConnectorCode, "current": group.Current}
		if group.PowerW != nil {
			item["power_w"] = *group.PowerW
		}
		if group.ReportedQuantity != nil {
			item["reported_quantity"] = *group.ReportedQuantity
		}
		groups = append(groups, item)
	}
	body, _ := json.Marshal(map[string]any{
		"id": place.ID, "entity_key": place.EntityKey, "external_id": place.ExternalID,
		"name": place.Name, "country": place.Country, "latitude": place.Latitude, "longitude": place.Longitude,
		"public_access": place.PublicAccess, "reported_points": place.ReportedPoints,
		"groups": groups, "generated_at": generatedAt(h.now()),
	})
	httpapi.WriteJSONWithETag(w, r, http.StatusOK, "public, max-age=60", body,
		httpapi.ETag(mustMarshal(map[string]any{"id": place.ID, "groups": groups})))
}

func (h Handler) coverage(w http.ResponseWriter, r *http.Request) {
	if h.disabled(w, r) {
		return
	}
	q := r.URL.Query()
	profile := q.Get("profile")
	if profile == "" {
		httpapi.WriteError(w, r, http.StatusBadRequest, "charging.bad-filter", "profile required",
			[]httpapi.Detail{{Field: "profile", Code: "required"}})
		return
	}
	batchID, err := h.Read.LatestBatch(r.Context(), profile)
	if err != nil {
		sourceDown(w, r)
		return
	}
	coverage, err := h.Read.Coverage(r.Context(), batchID)
	if err != nil {
		sourceDown(w, r)
		return
	}
	body, _ := json.Marshal(map[string]any{
		"profile": coverage.Profile, "source_revision": coverage.SourceRevision,
		"places": coverage.Places, "groups": coverage.Groups, "vehicles": coverage.Vehicles,
		"generated_at": generatedAt(h.now()),
	})
	httpapi.WriteJSONWithETag(w, r, http.StatusOK, "public, max-age=60", body,
		httpapi.ETag(mustMarshal(map[string]any{"coverage": coverage})))
}

func (h Handler) now() time.Time {
	if h.Now != nil {
		return h.Now()
	}
	return time.Now()
}

func generatedAt(t time.Time) string { return t.UTC().Format(time.RFC3339) }

func sourceDown(w http.ResponseWriter, r *http.Request) {
	httpapi.WriteError(w, r, http.StatusServiceUnavailable, "charging.unavailable", "charging source unavailable", nil)
}

func mustMarshal(value any) []byte {
	raw, _ := json.Marshal(value)
	return raw
}
