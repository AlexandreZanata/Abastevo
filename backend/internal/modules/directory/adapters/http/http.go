package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	application "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
)

// Handler serves anonymous directory reads.
type Handler struct {
	Stations application.StationReader
	Secrets  []byte
}

// RegisterRoutes mounts the directory reads under /v1.
func (h Handler) RegisterRoutes(r chi.Router) {
	r.Get("/v1/stations", h.search)
	r.Get("/v1/stations/nearby", h.nearby)
	r.Get("/v1/stations/{station_id}", h.detail)
}

func generatedNow() string { return time.Now().UTC().Format(time.RFC3339) }

func (h Handler) search(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, err := httpapi.ParseLimit(q.Get("limit"))
	if err != nil {
		httpapi.WriteError(w, r, http.StatusBadRequest, "station.bad-limit", "limit must be 1..100",
			[]httpapi.Detail{{Field: "limit", Code: "range"}})
		return
	}
	filter, err := application.ValidateSearch(q.Get("state"), q.Get("municipality_code"), q.Get("q"), limit, "")
	if err != nil {
		httpapi.WriteError(w, r, http.StatusBadRequest, "station.bad-filter", "invalid search filter",
			[]httpapi.Detail{{Field: "filter", Code: "invalid"}})
		return
	}
	fh := httpapi.FilterHash("search", filter.State, filter.Municipality, filter.Q, strconv.Itoa(limit))
	afterID, err := httpapi.ParseCursor(h.Secrets, q.Get("cursor"), fh, time.Now())
	if err != nil {
		httpapi.WriteError(w, r, http.StatusBadRequest, "station.bad-cursor", "cursor invalid, expired or changed",
			[]httpapi.Detail{{Field: "cursor", Code: "invalid"}})
		return
	}
	filter.AfterID = afterID
	stations, nextKey, err := h.Stations.Search(r.Context(), filter)
	if err != nil {
		httpapi.WriteError(w, r, http.StatusInternalServerError, "station.unavailable", "try again later", nil)
		return
	}
	var next *string
	if nextKey != "" {
		sealed := httpapi.Seal(h.Secrets, fh, nextKey, time.Now())
		next = &sealed
	}
	body, _ := json.Marshal(map[string]any{
		"items": stations, "next_cursor": next, "generated_at": generatedNow(),
	})
	httpapi.WriteJSON(w, r, http.StatusOK, "public, max-age=60", body)
}

func parseCoord(raw string) (float64, error) {
	return strconv.ParseFloat(strings.TrimSpace(raw), 64)
}

func (h Handler) nearby(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	lat, err := parseCoord(q.Get("lat"))
	if err != nil {
		httpapi.WriteError(w, r, http.StatusBadRequest, "station.bad-coord", "lat must be a number",
			[]httpapi.Detail{{Field: "lat", Code: "number"}})
		return
	}
	lon, err := parseCoord(q.Get("lon"))
	if err != nil {
		httpapi.WriteError(w, r, http.StatusBadRequest, "station.bad-coord", "lon must be a number",
			[]httpapi.Detail{{Field: "lon", Code: "number"}})
		return
	}
	radius := 3000
	if raw := strings.TrimSpace(q.Get("radius_m")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil {
			httpapi.WriteError(w, r, http.StatusBadRequest, "station.bad-radius", "radius_m must be an integer",
				[]httpapi.Detail{{Field: "radius_m", Code: "integer"}})
			return
		}
		radius = n
	}
	limit, err := httpapi.ParseLimit(q.Get("limit"))
	if err != nil {
		httpapi.WriteError(w, r, http.StatusBadRequest, "station.bad-limit", "limit must be 1..100",
			[]httpapi.Detail{{Field: "limit", Code: "range"}})
		return
	}
	filter, err := application.ValidateNearby(lat, lon, radius, limit, "")
	if err != nil {
		code := "station.bad-coord"
		if errors.Is(err, application.ErrInvalidFilter) {
			code = "station.bad-limit"
		}
		httpapi.WriteError(w, r, http.StatusBadRequest, code, "position or radius out of bounds",
			[]httpapi.Detail{{Field: "position", Code: "bounds"}})
		return
	}
	fh := httpapi.FilterHash("nearby",
		strconv.FormatFloat(lat, 'g', -1, 64), strconv.FormatFloat(lon, 'g', -1, 64),
		strconv.Itoa(radius), strconv.Itoa(limit))
	lastKey, err := httpapi.ParseCursor(h.Secrets, q.Get("cursor"), fh, time.Now())
	if err != nil {
		httpapi.WriteError(w, r, http.StatusBadRequest, "station.bad-cursor", "cursor invalid, expired or changed",
			[]httpapi.Detail{{Field: "cursor", Code: "invalid"}})
		return
	}
	if lastKey != "" {
		filter, err = application.ValidateNearby(lat, lon, radius, limit, lastKey)
		if err != nil {
			httpapi.WriteError(w, r, http.StatusBadRequest, "station.bad-cursor", "cursor invalid, expired or changed",
				[]httpapi.Detail{{Field: "cursor", Code: "invalid"}})
			return
		}
	}
	stations, nextKey, err := h.Stations.Nearby(r.Context(), filter)
	if err != nil {
		httpapi.WriteError(w, r, http.StatusInternalServerError, "station.unavailable", "try again later", nil)
		return
	}
	var next *string
	if nextKey != "" {
		sealed := httpapi.Seal(h.Secrets, fh, nextKey, time.Now())
		next = &sealed
	}
	body, _ := json.Marshal(map[string]any{
		"items": stations, "next_cursor": next, "generated_at": generatedNow(),
	})
	httpapi.WriteJSON(w, r, http.StatusOK, "no-store", body)
}

func (h Handler) detail(w http.ResponseWriter, r *http.Request) {
	id, err := application.ValidateStationID(chi.URLParam(r, "station_id"))
	if err != nil {
		httpapi.WriteError(w, r, http.StatusBadRequest, "station.bad-id", "station id must be a UUID",
			[]httpapi.Detail{{Field: "station_id", Code: "uuid"}})
		return
	}
	station, err := h.Stations.Detail(r.Context(), id)
	if err != nil {
		if errors.Is(err, application.ErrUnknownStation) {
			httpapi.WriteError(w, r, http.StatusNotFound, "station.not-found", "station not found", nil)
			return
		}
		httpapi.WriteError(w, r, http.StatusInternalServerError, "station.unavailable", "try again later", nil)
		return
	}
	body, _ := json.Marshal(station)
	httpapi.WriteJSON(w, r, http.StatusOK, "public, max-age=30, s-maxage=60", body)
}
