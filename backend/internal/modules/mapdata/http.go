package mapdata

import (
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
)

var productPattern = regexp.MustCompile(`^[A-Z0-9_]{2,40}$`)

type Handler struct {
	Store   *Store
	Secrets []byte
}

func (h Handler) RegisterRoutes(r chi.Router) {
	r.Get("/v1/map/assertions", h.assertions)
}

func (h Handler) assertions(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	minLat, err := parseCoord(q.Get("min_lat"))
	if err != nil {
		badRequest(w, r, "map.bad-bbox", "min_lat", "number")
		return
	}
	minLon, err := parseCoord(q.Get("min_lon"))
	if err != nil {
		badRequest(w, r, "map.bad-bbox", "min_lon", "number")
		return
	}
	maxLat, err := parseCoord(q.Get("max_lat"))
	if err != nil {
		badRequest(w, r, "map.bad-bbox", "max_lat", "number")
		return
	}
	maxLon, err := parseCoord(q.Get("max_lon"))
	if err != nil {
		badRequest(w, r, "map.bad-bbox", "max_lon", "number")
		return
	}
	if minLat < -90 || maxLat > 90 || minLon < -180 || maxLon > 180 ||
		maxLat <= minLat || maxLon <= minLon {
		badRequest(w, r, "map.bad-bbox", "bbox", "bounds")
		return
	}
	if (maxLat-minLat)*(maxLon-minLon) > MaxViewportAreaDeg2 {
		badRequest(w, r, "map.bbox-too-large", "bbox", "area")
		return
	}
	source := strings.ToUpper(strings.TrimSpace(q.Get("source")))
	if source != "ANP" && source != "COMMUNITY" {
		badRequest(w, r, "map.bad-source", "source", "enum")
		return
	}
	product := strings.ToUpper(strings.TrimSpace(q.Get("product")))
	if product != "" && !productPattern.MatchString(product) {
		badRequest(w, r, "map.bad-product", "product", "format")
		return
	}
	limit := MaxLimit
	if raw := strings.TrimSpace(q.Get("limit")); raw != "" {
		limit, err = strconv.Atoi(raw)
		if err != nil || limit < 1 || limit > MaxLimit {
			badRequest(w, r, "map.bad-limit", "limit", "range")
			return
		}
	}
	generation, err := h.Store.ActiveGeneration(r.Context())
	if errors.Is(err, ErrNoActiveGeneration) {
		writeMap(w, r, http.StatusOK, nil, nil, nil)
		return
	}
	if err != nil {
		httpapi.WriteError(w, r, http.StatusInternalServerError, "map.unavailable", "try again later", nil)
		return
	}
	filterHash := httpapi.FilterHash("map", generation.ID,
		strconv.FormatFloat(minLat, 'g', -1, 64), strconv.FormatFloat(minLon, 'g', -1, 64),
		strconv.FormatFloat(maxLat, 'g', -1, 64), strconv.FormatFloat(maxLon, 'g', -1, 64),
		source, product, strconv.Itoa(limit))
	after, err := httpapi.ParseCursor(h.Secrets, q.Get("cursor"), filterHash, time.Now())
	if err != nil {
		httpapi.WriteError(w, r, http.StatusBadRequest, "map.bad-cursor", "cursor invalid, expired or changed",
			[]httpapi.Detail{{Field: "cursor", Code: "invalid"}})
		return
	}
	items, next, err := h.Store.Query(r.Context(), generation.ID, Query{
		MinLat: minLat, MinLon: minLon, MaxLat: maxLat, MaxLon: maxLon,
		Source: source, Product: product, Limit: limit, AfterID: after,
	})
	if errors.Is(err, ErrStaleGeneration) {
		httpapi.WriteError(w, r, http.StatusConflict, "map.stale-generation", "map generation changed; refresh", nil)
		return
	}
	if errors.Is(err, ErrInvalidQuery) {
		badRequest(w, r, "map.bad-query", "query", "invalid")
		return
	}
	if err != nil {
		httpapi.WriteError(w, r, http.StatusInternalServerError, "map.unavailable", "try again later", nil)
		return
	}
	var nextCursor *string
	if next != "" {
		sealed := httpapi.Seal(h.Secrets, filterHash, next, time.Now())
		nextCursor = &sealed
	}
	writeMap(w, r, http.StatusOK, &generation, items, nextCursor)
}

func parseCoord(raw string) (float64, error) {
	value, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, errors.New("invalid coordinate")
	}
	return value, nil
}

func badRequest(w http.ResponseWriter, r *http.Request, code, field, reason string) {
	httpapi.WriteError(w, r, http.StatusBadRequest, code, "invalid map query",
		[]httpapi.Detail{{Field: field, Code: reason}})
}

func writeMap(w http.ResponseWriter, r *http.Request, status int, generation *Generation, items []OverlayAssertion, next *string) {
	if items == nil {
		items = []OverlayAssertion{}
	}
	body, _ := json.Marshal(map[string]any{
		"generation":   generation,
		"items":        items,
		"next_cursor":  next,
		"generated_at": time.Now().UTC().Format(time.RFC3339),
	})
	httpapi.WriteJSON(w, r, status, "no-store", body)
}

func (g *Generation) MarshalJSON() ([]byte, error) {
	if g == nil {
		return []byte("null"), nil
	}
	return json.Marshal(map[string]any{
		"id": g.ID, "key": g.Key, "source_revision": g.SourceRevision,
		"status": g.Status, "activated_at": g.ActivatedAt.UTC().Format(time.RFC3339),
	})
}
