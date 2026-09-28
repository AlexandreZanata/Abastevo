package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	application "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/official/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
)

// Handler serves anonymous official reads.
type Handler struct {
	Prices        application.PriceReader
	StationsExist func(ctx context.Context, id string) (bool, error)
	Secrets       []byte
}

// RegisterRoutes mounts the official reads under /v1.
func (h Handler) RegisterRoutes(r chi.Router) {
	r.Get("/v1/stations/{station_id}/prices", h.groups)
	r.Get("/v1/stations/{station_id}/official-prices", h.history)
}

func generatedNow() string { return time.Now().UTC().Format(time.RFC3339) }

func (h Handler) exists(w http.ResponseWriter, r *http.Request, id string) bool {
	ok, err := h.StationsExist(r.Context(), id)
	if err != nil {
		httpapi.WriteError(w, r, http.StatusInternalServerError, "official.unavailable", "try again later", nil)
		return false
	}
	if !ok {
		httpapi.WriteError(w, r, http.StatusNotFound, "official.not-found", "station not found", nil)
		return false
	}
	return true
}

type wireOfficial struct {
	Source      string `json:"source"`
	AmountMilli int64  `json:"amount_milli_brl"`
	Currency    string `json:"currency"`
	CollectedOn string `json:"collected_on"`
	SurveyWeek  struct {
		Start string `json:"start"`
		End   string `json:"end"`
	} `json:"survey_week"`
	RevisionID string `json:"revision_id"`
}

type wireGroup struct {
	StationID string         `json:"station_id"`
	Product   string         `json:"fuel_product"`
	Unit      string         `json:"unit"`
	Condition map[string]any `json:"condition"`
	Official  *wireOfficial  `json:"official"`
	Community *string        `json:"community"`
}

func (h Handler) groups(w http.ResponseWriter, r *http.Request) {
	id, err := application.ValidateStationID(chi.URLParam(r, "station_id"))
	if err != nil {
		httpapi.WriteError(w, r, http.StatusBadRequest, "official.bad-id", "station id must be a UUID",
			[]httpapi.Detail{{Field: "station_id", Code: "uuid"}})
		return
	}
	fuel, err := application.ValidateFuel(r.URL.Query().Get("fuel_product"))
	if err != nil {
		httpapi.WriteError(w, r, http.StatusBadRequest, "official.bad-filter", "unknown fuel product",
			[]httpapi.Detail{{Field: "fuel_product", Code: "enum"}})
		return
	}
	if !h.exists(w, r, id) {
		return
	}
	groups, err := h.Prices.Groups(r.Context(), id, fuel)
	if err != nil {
		httpapi.WriteError(w, r, http.StatusInternalServerError, "official.unavailable", "try again later", nil)
		return
	}
	wire := make([]wireGroup, 0, len(groups))
	for _, g := range groups {
		wg := wireGroup{
			StationID: g.StationID, Product: g.Product, Unit: g.Unit,
			Condition: map[string]any{"kind": g.Condition.Kind, "qualifier_id": nil},
			Community: nil,
		}
		if g.Official != nil {
			o := &wireOfficial{
				Source: g.Official.Source, AmountMilli: g.Official.AmountMilli,
				Currency: g.Official.Currency, CollectedOn: g.Official.CollectedOn,
				RevisionID: g.Official.RevisionID,
			}
			o.SurveyWeek.Start = g.Official.SurveyStart
			o.SurveyWeek.End = g.Official.SurveyEnd
			wg.Official = o
		}
		wire = append(wire, wg)
	}
	body, _ := json.Marshal(map[string]any{"items": wire, "generated_at": generatedNow()})
	httpapi.WriteJSON(w, r, http.StatusOK, "public, max-age=60", body)
}

type wireEntry struct {
	RevisionID string `json:"revision_id"`
	SurveyWeek struct {
		Start string `json:"start"`
		End   string `json:"end"`
	} `json:"survey_week"`
	CollectedOn string `json:"collected_on"`
	Product     string `json:"fuel_product"`
	Unit        string `json:"unit"`
	AmountMilli int64  `json:"amount_milli_brl"`
	Currency    string `json:"currency"`
	Source      struct {
		Kind     string `json:"kind"`
		URL      string `json:"url"`
		Checksum string `json:"checksum"`
	} `json:"source"`
}

func (h Handler) history(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	id, err := application.ValidateStationID(chi.URLParam(r, "station_id"))
	if err != nil {
		httpapi.WriteError(w, r, http.StatusBadRequest, "official.bad-id", "station id must be a UUID",
			[]httpapi.Detail{{Field: "station_id", Code: "uuid"}})
		return
	}
	limit, err := httpapi.ParseLimit(q.Get("limit"))
	if err != nil {
		httpapi.WriteError(w, r, http.StatusBadRequest, "official.bad-limit", "limit must be 1..100",
			[]httpapi.Detail{{Field: "limit", Code: "range"}})
		return
	}
	filter, err := application.ValidateHistory(id, q.Get("fuel_product"), q.Get("revision_id"), limit, "")
	if err != nil {
		httpapi.WriteError(w, r, http.StatusBadRequest, "official.bad-filter", "invalid history filter",
			[]httpapi.Detail{{Field: "filter", Code: "invalid"}})
		return
	}
	fh := httpapi.FilterHash("history", id, filter.Fuel, filter.RevisionID, strconv.Itoa(limit))
	lastKey, err := httpapi.ParseCursor(h.Secrets, q.Get("cursor"), fh, time.Now())
	if err != nil {
		httpapi.WriteError(w, r, http.StatusBadRequest, "official.bad-cursor", "cursor invalid, expired or changed",
			[]httpapi.Detail{{Field: "cursor", Code: "invalid"}})
		return
	}
	if lastKey != "" {
		filter, err = application.ValidateHistory(id, q.Get("fuel_product"), q.Get("revision_id"), limit, lastKey)
		if err != nil {
			httpapi.WriteError(w, r, http.StatusBadRequest, "official.bad-cursor", "cursor invalid, expired or changed",
				[]httpapi.Detail{{Field: "cursor", Code: "invalid"}})
			return
		}
	}
	if !h.exists(w, r, id) {
		return
	}
	entries, nextKey, err := h.Prices.History(r.Context(), filter)
	if err != nil {
		if errors.Is(err, application.ErrUnknownStation) {
			httpapi.WriteError(w, r, http.StatusNotFound, "official.not-found", "station not found", nil)
			return
		}
		httpapi.WriteError(w, r, http.StatusInternalServerError, "official.unavailable", "try again later", nil)
		return
	}
	var next *string
	if nextKey != "" {
		sealed := httpapi.Seal(h.Secrets, fh, nextKey, time.Now())
		next = &sealed
	}
	wire := make([]wireEntry, 0, len(entries))
	for _, e := range entries {
		var we wireEntry
		we.RevisionID = e.RevisionID
		we.SurveyWeek.Start, we.SurveyWeek.End = e.WeekStart, e.WeekEnd
		we.CollectedOn, we.Product, we.Unit = e.CollectedOn, e.Product, e.Unit
		we.AmountMilli, we.Currency = e.AmountMilli, e.Currency
		we.Source.Kind, we.Source.URL, we.Source.Checksum = "ANP", e.SourceURL, e.SourceSha
		wire = append(wire, we)
	}
	body, _ := json.Marshal(map[string]any{
		"items": wire, "next_cursor": next, "generated_at": generatedNow(),
	})
	httpapi.WriteJSON(w, r, http.StatusOK, "public, max-age=60", body)
}
