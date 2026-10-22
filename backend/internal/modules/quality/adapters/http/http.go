package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters/read"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
)

// Frozen read streams. Route groups bind one source/stream each; the
// revision lease behind them resolves per request.
const (
	assaySource    = "pmqc"
	assayStream    = "pmqc-assay-v1"
	actionSource   = "enforcement"
	actionStream   = "enforcement-v1"
	sanctionSource = "sanctions"
	sanctionStream = "sanction-v1"
)

// Handler serves anonymous quality reads.
type Handler struct {
	Read           *read.Reader
	Secrets        []byte
	StationsExist  func(ctx context.Context, id string) (bool, error)
	Enabled        func() bool
	CompanyEnabled func() bool
	Now            func() time.Time
}

// RegisterRoutes mounts the quality reads under /v1/quality.
func (h Handler) RegisterRoutes(r chi.Router) {
	r.Get("/v1/quality/revisions", h.revisions)
	r.Get("/v1/quality/revisions/{id}", h.revisionDetail)
	r.Get("/v1/quality/summary", h.summary)
	r.Get("/v1/quality/assays", h.assays)
	r.Get("/v1/quality/samples/{sample_ref}", h.sample)
	r.Get("/v1/quality/actions", h.actions)
	r.Get("/v1/quality/actions/coverage", h.actionCoverage)
	r.Get("/v1/quality/actions/{id}", h.actionOne)
	r.Get("/v1/quality/enforcement/stations", h.enforcementStations)
	r.Get("/v1/quality/enforcement/stations/coverage", h.enforcementStationCoverage)
	r.Get("/v1/quality/sanctions", h.sanctions)
	r.Get("/v1/quality/sanctions/{id}", h.sanctionOne)
}

func (h Handler) now() time.Time {
	if h.Now != nil {
		return h.Now()
	}
	return time.Now()
}

func (h Handler) live() bool {
	return h.Enabled == nil || h.Enabled()
}

func generatedAt(t time.Time) string { return t.UTC().Format(time.RFC3339) }

type wirePeriod struct {
	Label string `json:"label"`
	Start string `json:"start"`
	End   string `json:"end"`
}

type wireLease struct {
	SourceInputContract   string            `json:"source_input_contract,omitempty"`
	SourcePeriod          string            `json:"source_period,omitempty"`
	SourceClockValue      string            `json:"source_clock_value,omitempty"`
	SourceRecordCount     *int              `json:"source_record_count,omitempty"`
	SourceMetadataRecords *int              `json:"source_metadata_records,omitempty"`
	SourceNotes           []read.SourceNote `json:"source_notes,omitempty"`
	SourceClockPrecision  string            `json:"source_clock_precision,omitempty"`
	SourceExtractionMonth string            `json:"source_extraction_month,omitempty"`
	SourceReportYear      *int32            `json:"source_report_year,omitempty"`
	SupersedesRevisionID  *string           `json:"supersedes_revision_id"`
	RevisionStatus        string            `json:"revision_status"`
	ProvenanceState       string            `json:"provenance_state"`
	IsCurrentStream       bool              `json:"is_current_stream"`
	Source                string            `json:"source"`
	Stream                string            `json:"stream"`
	Edition               string            `json:"edition"`
	RevisionID            *string           `json:"revision_id"`
	PublishedAt           *string           `json:"published_at"`
	Available             bool              `json:"available"`
	Stale                 bool              `json:"stale"`
	StaleReason           string            `json:"stale_reason"`
	CoverageNote          string            `json:"coverage_note"`
	Periods               []wirePeriod      `json:"periods"`
}

func wireLeaseOf(l read.Lease) wireLease {
	out := wireLease{
		SourceInputContract: l.SourceInputContract, SourcePeriod: l.SourcePeriod, SourceClockValue: l.SourceClockValue, SourceRecordCount: l.SourceRecordCount, SourceMetadataRecords: l.SourceMetadataRecords, SourceNotes: l.SourceNotes,
		SourceClockPrecision: l.SourceClockPrecision, SourceExtractionMonth: l.SourceExtractionMonth, SourceReportYear: l.SourceReportYear,
		SupersedesRevisionID: nullableString(l.SupersedesRevisionID), RevisionStatus: l.Status, ProvenanceState: l.ProvenanceState, IsCurrentStream: l.IsCurrentStream, Source: l.Source, Stream: l.Stream, Edition: l.Edition,
		RevisionID: nullableString(l.RevisionID), Available: l.HasPublished,
		Stale: l.Stale, StaleReason: l.StaleReason,
		CoverageNote: l.CoverageNote, Periods: []wirePeriod{},
	}
	if out.RevisionStatus == "" {
		out.RevisionStatus = "unavailable"
	}
	if out.ProvenanceState == "" {
		out.ProvenanceState = "UNAVAILABLE"
	}
	if !l.PublishedAt.IsZero() {
		out.PublishedAt = nullableString(l.PublishedAt.UTC().Format(time.RFC3339))
	}
	for _, p := range l.Periods {
		out.Periods = append(out.Periods, wirePeriod{Label: p.Label, Start: p.Start, End: p.End})
	}
	return out
}

func (h Handler) disabled(w http.ResponseWriter, r *http.Request) bool {
	if h.live() {
		return false
	}
	httpapi.WriteError(w, r, http.StatusNotFound, "quality.disabled", "quality reads are off", nil)
	return true
}

func (h Handler) exists(w http.ResponseWriter, r *http.Request, id string) bool {
	if h.StationsExist == nil {
		sourceDown(w, r)
		return false
	}
	ok, err := h.StationsExist(r.Context(), id)
	if err != nil {
		httpapi.WriteError(w, r, http.StatusServiceUnavailable, "quality.source-down", "try again later", nil)
		return false
	}
	if !ok {
		httpapi.WriteError(w, r, http.StatusNotFound, "quality.not-found", "station not found", nil)
		return false
	}
	return true
}

func sourceDown(w http.ResponseWriter, r *http.Request) {
	httpapi.WriteError(w, r, http.StatusServiceUnavailable, "quality.source-down", "try again later", nil)
}

func (h Handler) summary(w http.ResponseWriter, r *http.Request) {
	if h.disabled(w, r) {
		return
	}
	if !h.selectRevision(w, r) {
		return
	}
	q := r.URL.Query()
	id, err := application.ValidateStationID(q.Get("station_id"))
	if err != nil {
		httpapi.WriteError(w, r, http.StatusBadRequest, "quality.bad-id", "station id must be a UUID",
			[]httpapi.Detail{{Field: "station_id", Code: "uuid"}})
		return
	}
	fuel, err := application.ValidateFuel(q.Get("fuel"))
	if err != nil {
		httpapi.WriteError(w, r, http.StatusBadRequest, "quality.bad-fuel", "unknown fuel product",
			[]httpapi.Detail{{Field: "fuel", Code: "enum"}})
		return
	}
	if !h.exists(w, r, id) {
		return
	}
	cells, lease, err := h.Read.Summary(r.Context(), h.now(), assaySource, assayStream, id, fuel)
	if err != nil {
		h.readErr(w, r, err)
		return
	}
	wire := make([]map[string]any, 0, len(cells))
	for _, c := range cells {
		wire = append(wire, map[string]any{
			"fuel_product": c.Fuel, "conformity": c.Conformity,
			"count": c.Count, "last_observed": c.LastObserved,
		})
	}
	dimensions, err := h.Read.DimensionsAtLease(r.Context(), lease, id, fuel)
	if err != nil {
		sourceDown(w, r)
		return
	}
	body, _ := json.Marshal(map[string]any{
		"station_id": id, "fuel_cells": wire,
		"sample_dimensions": dimensions,
		"lease":             wireLeaseOf(lease), "generated_at": generatedAt(h.now()),
	})
	httpapi.WriteJSONWithETag(w, r, http.StatusOK, "public, max-age=60", body,
		httpapi.ETag(mustMarshal(map[string]any{"station_id": id, "cells": wire, "sample_dimensions": dimensions, "lease": wireLeaseOf(lease)})))
}

func (h Handler) assays(w http.ResponseWriter, r *http.Request) {
	if h.disabled(w, r) {
		return
	}
	if !h.selectRevision(w, r) {
		return
	}
	q := r.URL.Query()
	id, err := application.ValidateStationID(q.Get("station_id"))
	if err != nil {
		httpapi.WriteError(w, r, http.StatusBadRequest, "quality.bad-id", "station id must be a UUID",
			[]httpapi.Detail{{Field: "station_id", Code: "uuid"}})
		return
	}
	fuel, err := application.ValidateFuel(q.Get("fuel"))
	if err != nil {
		httpapi.WriteError(w, r, http.StatusBadRequest, "quality.bad-fuel", "unknown fuel product",
			[]httpapi.Detail{{Field: "fuel", Code: "enum"}})
		return
	}
	limit, err := httpapi.ParseLimit(q.Get("limit"))
	if err != nil {
		httpapi.WriteError(w, r, http.StatusBadRequest, "quality.bad-limit", "limit must be 1..100",
			[]httpapi.Detail{{Field: "limit", Code: "range"}})
		return
	}
	if !h.exists(w, r, id) {
		return
	}
	// The lease resolves first so the cursor binds to one frozen revision:
	// cursors cannot wander across concurrent publications.
	lease, err := h.Read.LeaseBlock(r.Context(), h.now(), assaySource, assayStream)
	if err != nil {
		h.readErr(w, r, err)
		return
	}
	fh := httpapi.FilterHash("quality-assays", id, fuel, strconv.Itoa(limit), lease.RevisionID)
	lastKey, err := httpapi.ParseCursor(h.Secrets, q.Get("cursor"), fh, h.now())
	if err != nil {
		httpapi.WriteError(w, r, http.StatusBadRequest, "quality.bad-cursor", "cursor invalid, expired or changed",
			[]httpapi.Detail{{Field: "cursor", Code: "invalid"}})
		return
	}
	afterObserved, afterID := splitCursor(lastKey)
	rows, nextKey, _, err := h.Read.AssaysAtLease(r.Context(), lease, id, fuel, afterObserved, afterID, limit)
	if err != nil {
		h.readErr(w, r, err)
		return
	}
	var next *string
	if nextKey != "" {
		sealed := httpapi.Seal(h.Secrets, fh, nextKey, h.now())
		next = &sealed
	}
	wire := make([]map[string]any, 0, len(rows))
	for _, a := range rows {
		wire = append(wire, map[string]any{
			"id": a.ID, "sample_ref": a.SampleRef, "assay_ref": a.AssayRef,
			"observed_on": a.ObservedOn, "fuel_product": a.Fuel,
			"result_text": a.ResultText, "unit": a.Unit,
			"conformity": a.Conformity, "evidence_ref": a.EvidenceRef,
		})
		if a.SourceObservedAt != "" {
			wire[len(wire)-1]["source_observed_at"] = a.SourceObservedAt
		}
	}
	body, _ := json.Marshal(map[string]any{
		"items": wire, "next_cursor": next,
		"station_id": id,
		"lease":      wireLeaseOf(lease), "generated_at": generatedAt(h.now()),
	})
	httpapi.WriteJSONWithETag(w, r, http.StatusOK, "public, max-age=60", body,
		httpapi.ETag(mustMarshal(map[string]any{"items": wire, "next_cursor": nextKey, "lease": wireLeaseOf(lease)})))
}

func splitCursor(lastKey string) (string, string) {
	if lastKey == "" {
		return "", ""
	}
	i := strings.LastIndex(lastKey, "|")
	if i < 0 {
		return "", ""
	}
	return lastKey[:i], lastKey[i+1:]
}

func (h Handler) sample(w http.ResponseWriter, r *http.Request) {
	if h.disabled(w, r) {
		return
	}
	if !h.selectRevision(w, r) {
		return
	}
	ref := strings.TrimSpace(chi.URLParam(r, "sample_ref"))
	if ref == "" || len(ref) > 128 {
		httpapi.WriteError(w, r, http.StatusBadRequest, "quality.bad-sample", "sample ref required",
			[]httpapi.Detail{{Field: "sample_ref", Code: "required"}})
		return
	}
	detail, err := h.Read.Sample(r.Context(), h.now(), assaySource, assayStream, ref)
	if err != nil {
		h.readErr(w, r, err)
		return
	}
	wire := make([]map[string]any, 0, len(detail.Assays))
	for _, a := range detail.Assays {
		wire = append(wire, map[string]any{
			"id": a.ID, "sample_ref": a.SampleRef, "assay_ref": a.AssayRef, "observed_on": a.ObservedOn,
			"fuel_product": a.Fuel, "result_text": a.ResultText, "unit": a.Unit,
			"conformity": a.Conformity, "evidence_ref": a.EvidenceRef,
		})
		if a.SourceObservedAt != "" {
			wire[len(wire)-1]["source_observed_at"] = a.SourceObservedAt
		}
	}
	links := make([]map[string]any, 0, len(detail.Links))
	for _, l := range detail.Links {
		links = append(links, map[string]any{
			"station_id": nullableString(l.StationID), "automatic": l.Automatic,
			"reason": l.Reason, "active": l.Active,
		})
	}
	body, _ := json.Marshal(map[string]any{
		"sample_ref": ref, "assays": wire, "links": links,
		"lease": wireLeaseOf(detail.Lease), "generated_at": generatedAt(h.now()),
	})
	httpapi.WriteJSONWithETag(w, r, http.StatusOK, "public, max-age=60", body,
		httpapi.ETag(mustMarshal(map[string]any{"sample_ref": ref, "assays": wire, "links": links, "lease": wireLeaseOf(detail.Lease)})))
}

func (h Handler) actions(w http.ResponseWriter, r *http.Request) {
	h.events(w, r, actionSource, actionStream, "actions")
}

// enforcementFacets validates the native-source facet params. Empty means
// no filter; every set facet must be exact and well-formed. Unknown facet
// values return empty pages, malformed values return 400.
func enforcementFacets(w http.ResponseWriter, r *http.Request) (read.EnforcementFilter, bool) {
	q := r.URL.Query()
	var filter read.EnforcementFilter
	bad := func(field, code string) (read.EnforcementFilter, bool) {
		httpapi.WriteError(w, r, http.StatusBadRequest, "quality.bad-filter", field+" filter invalid",
			[]httpapi.Detail{{Field: field, Code: code}})
		return read.EnforcementFilter{}, false
	}
	if uf := q.Get("uf"); uf != "" {
		if len(uf) != 2 || uf[0] < 'A' || uf[0] > 'Z' || uf[1] < 'A' || uf[1] > 'Z' {
			return bad("uf", "shape")
		}
		filter.UF = uf
	}
	if city, err := application.ValidateTextFilter("municipality", q.Get("municipality"), 256); err != nil || (q.Get("municipality") != "" && city == "") {
		return bad("municipality", "length")
	} else {
		filter.Municipality = city
	}
	if segment, err := application.ValidateTextFilter("segment", q.Get("segment"), 256); err != nil {
		return bad("segment", "length")
	} else {
		filter.Segment = segment
	}
	switch kind := q.Get("subject_kind"); kind {
	case "", "company-shaped", "personal-shaped", "other":
		filter.SubjectKind = kind
	default:
		return bad("subject_kind", "vocabulary")
	}
	from, to := q.Get("from_day"), q.Get("to_day")
	if from != "" && !domain.ValidDay(from) {
		return bad("from_day", "calendar")
	}
	if to != "" && !domain.ValidDay(to) {
		return bad("to_day", "calendar")
	}
	if from != "" && to != "" && from > to {
		return bad("from_day", "range")
	}
	filter.FromDay, filter.ToDay = from, to
	return filter, true
}

func (h Handler) sanctions(w http.ResponseWriter, r *http.Request) {
	h.events(w, r, sanctionSource, sanctionStream, "sanctions")
}

func (h Handler) events(w http.ResponseWriter, r *http.Request, source, stream, name string) {
	if h.disabled(w, r) {
		return
	}
	qValues, ok := eventQuery(w, r, source, false)
	if !ok {
		return
	}
	if !h.selectRevision(w, r) {
		return
	}

	if _, scoped := qValues["station_id"]; scoped {
		h.companyEvents(w, r)
		return
	}
	q := r.URL.Query()
	status, err := application.ValidateTextFilter("status", q.Get("status"), 64)
	if err != nil {
		httpapi.WriteError(w, r, http.StatusBadRequest, "quality.bad-filter", "status filter too long",
			[]httpapi.Detail{{Field: "status", Code: "length"}})
		return
	}
	scope, err := application.ValidateTextFilter("scope", q.Get("scope"), 64)
	if err != nil {
		httpapi.WriteError(w, r, http.StatusBadRequest, "quality.bad-filter", "scope filter too long",
			[]httpapi.Detail{{Field: "scope", Code: "length"}})
		return
	}
	limit, err := httpapi.ParseLimit(q.Get("limit"))
	if err != nil {
		httpapi.WriteError(w, r, http.StatusBadRequest, "quality.bad-limit", "limit must be 1..100",
			[]httpapi.Detail{{Field: "limit", Code: "range"}})
		return
	}
	lease, err := h.leaseOf(r, source, stream)
	if err != nil {
		h.readErr(w, r, err)
		return
	}

	if lease.SourceInputContract == application.EnforcementSourceProfile && (status != "" || scope != "") {
		httpapi.WriteError(w, r, http.StatusBadRequest, "quality.bad-filter", "legacy status/scope unavailable for this source", nil)
		return
	}
	var filter read.EnforcementFilter
	if lease.SourceInputContract == application.EnforcementSourceProfile {
		var ok bool
		if filter, ok = enforcementFacets(w, r); !ok {
			return
		}
	}
	fh := httpapi.FilterHash("quality-"+name, status, scope, strconv.Itoa(limit), lease.RevisionID,
		filter.UF, filter.Municipality, filter.Segment, filter.SubjectKind, filter.FromDay, filter.ToDay)
	lastKey, err := httpapi.ParseCursor(h.Secrets, q.Get("cursor"), fh, h.now())
	if err != nil {
		httpapi.WriteError(w, r, http.StatusBadRequest, "quality.bad-cursor", "cursor invalid, expired or changed",
			[]httpapi.Detail{{Field: "cursor", Code: "invalid"}})
		return
	}
	afterNative, afterID := splitCursor(lastKey)
	wire := make([]map[string]any, 0)
	var nextKey string
	if name == "actions" {
		var rows []read.EnforcementEvent
		var next string
		var err error
		if lease.SourceInputContract == application.EnforcementSourceProfile {
			rows, next, _, err = h.Read.EnforcementFilteredAtLease(r.Context(), lease, filter, afterNative, afterID, limit)
		} else {
			rows, next, _, err = h.Read.EnforcementAtLease(r.Context(), lease, status, scope, afterNative, afterID, limit)
		}
		if err != nil {
			sourceDown(w, r)
			return
		}
		nextKey = next
		for _, e := range rows {
			wire = append(wire, wireEnforcement(e))
		}
	} else {
		rows, next, _, err := h.Read.SanctionsAtLease(r.Context(), lease, status, afterNative, afterID, limit)
		if err != nil {
			sourceDown(w, r)
			return
		}
		nextKey = next
		for _, e := range rows {
			wire = append(wire, wireSanction(e))
		}
	}
	var next *string
	if nextKey != "" {
		sealed := httpapi.Seal(h.Secrets, fh, nextKey, h.now())
		next = &sealed
	}
	body, _ := json.Marshal(map[string]any{
		"items": wire, "next_cursor": next,
		"lease": wireLeaseOf(lease), "generated_at": generatedAt(h.now()),
	})
	httpapi.WriteJSONWithETag(w, r, http.StatusOK, "public, max-age=60", body,
		httpapi.ETag(mustMarshal(map[string]any{"items": wire, "next_cursor": nextKey, "lease": wireLeaseOf(lease)})))
}

func (h Handler) leaseOf(r *http.Request, source, stream string) (read.Lease, error) {
	return h.Read.LeaseBlock(r.Context(), h.now(), source, stream)
}

func (h Handler) actionOne(w http.ResponseWriter, r *http.Request) {
	h.eventOne(w, r, actionSource, actionStream)
}

// actionCoverage reports the leased enforcement revision's valid counts:
// distinct retained observations versus distinct source-stated cases, with
// the document-day bounds and basis. Counts only, never subject identifiers.
func (h Handler) actionCoverage(w http.ResponseWriter, r *http.Request) {
	if h.disabled(w, r) {
		return
	}
	if !h.selectRevision(w, r) {
		return
	}
	lease, err := h.leaseOf(r, actionSource, actionStream)
	if err != nil {
		h.readErr(w, r, err)
		return
	}
	if lease.SourceInputContract != application.EnforcementSourceProfile {
		httpapi.WriteError(w, r, http.StatusBadRequest, "quality.bad-coverage", "coverage unavailable for this source", nil)
		return
	}
	coverage, _, err := h.Read.EnforcementCoverageAtLease(r.Context(), lease)
	if err != nil {
		sourceDown(w, r)
		return
	}
	body, _ := json.Marshal(map[string]any{
		"observations": coverage.Observations, "cases": coverage.Cases,
		"first_day": coverage.FirstDay, "last_day": coverage.LastDay,
		"date_basis": coverage.DateBasis,
		"lease":      wireLeaseOf(lease), "generated_at": generatedAt(h.now()),
	})
	httpapi.WriteJSONWithETag(w, r, http.StatusOK, "public, max-age=60", body,
		httpapi.ETag(mustMarshal(map[string]any{"coverage": coverage, "lease": wireLeaseOf(lease)})))
}

// enforcementStations pages distinctly associated stations for the leased
// enforcement revision. Only exact current bindings qualify; a station
// retired at read time is excluded. Stations without price observations
// are included: this page is about occurrences, never a completeness
// claim about prices.
func (h Handler) enforcementStations(w http.ResponseWriter, r *http.Request) {
	if h.disabled(w, r) {
		return
	}
	if !h.selectRevision(w, r) {
		return
	}
	lease, err := h.leaseOf(r, actionSource, actionStream)
	if err != nil {
		h.readErr(w, r, err)
		return
	}
	if lease.SourceInputContract != application.EnforcementSourceProfile {
		httpapi.WriteError(w, r, http.StatusBadRequest, "quality.bad-source", "station page unavailable for this source", nil)
		return
	}
	q := r.URL.Query()
	for key := range q {
		switch key {
		case "revision_id", "state", "segment", "from_day", "to_day", "limit", "cursor":
		default:
			httpapi.WriteError(w, r, http.StatusBadRequest, "quality.bad-filter", "unsupported query parameter", nil)
			return
		}
	}
	var filter read.EnforcementStationFilter
	if state := q.Get("state"); state != "" {
		if len(state) != 2 || state[0] < 'A' || state[0] > 'Z' || state[1] < 'A' || state[1] > 'Z' {
			httpapi.WriteError(w, r, http.StatusBadRequest, "quality.bad-filter", "state filter invalid",
				[]httpapi.Detail{{Field: "state", Code: "shape"}})
			return
		}
		filter.State = state
	}
	if segment, err := application.ValidateTextFilter("segment", q.Get("segment"), 256); err != nil {
		httpapi.WriteError(w, r, http.StatusBadRequest, "quality.bad-filter", "segment filter invalid",
			[]httpapi.Detail{{Field: "segment", Code: "length"}})
		return
	} else {
		filter.Segment = segment
	}
	from, to := q.Get("from_day"), q.Get("to_day")
	if from != "" && !domain.ValidDay(from) {
		httpapi.WriteError(w, r, http.StatusBadRequest, "quality.bad-filter", "from_day filter invalid",
			[]httpapi.Detail{{Field: "from_day", Code: "calendar"}})
		return
	}
	if to != "" && !domain.ValidDay(to) {
		httpapi.WriteError(w, r, http.StatusBadRequest, "quality.bad-filter", "to_day filter invalid",
			[]httpapi.Detail{{Field: "to_day", Code: "calendar"}})
		return
	}
	if from != "" && to != "" && from > to {
		httpapi.WriteError(w, r, http.StatusBadRequest, "quality.bad-filter", "from_day filter invalid",
			[]httpapi.Detail{{Field: "from_day", Code: "range"}})
		return
	}
	filter.FromDay, filter.ToDay = from, to
	limit, err := httpapi.ParseLimit(q.Get("limit"))
	if err != nil {
		httpapi.WriteError(w, r, http.StatusBadRequest, "quality.bad-limit", "limit must be 1..100",
			[]httpapi.Detail{{Field: "limit", Code: "range"}})
		return
	}
	fh := httpapi.FilterHash("quality-enforcement-stations", filter.State, filter.Segment, filter.FromDay, filter.ToDay, strconv.Itoa(limit), lease.RevisionID)
	lastKey, err := httpapi.ParseCursor(h.Secrets, q.Get("cursor"), fh, h.now())
	if err != nil {
		httpapi.WriteError(w, r, http.StatusBadRequest, "quality.bad-cursor", "cursor invalid, expired or changed",
			[]httpapi.Detail{{Field: "cursor", Code: "invalid"}})
		return
	}
	afterID := lastKey
	rows, next, _, err := h.Read.EnforcementStationsAtLease(r.Context(), lease, filter, afterID, limit)
	if err != nil {
		sourceDown(w, r)
		return
	}
	wire := make([]map[string]any, 0, len(rows))
	for _, station := range rows {
		wire = append(wire, map[string]any{"id": station.ID, "display_name": station.DisplayName, "municipality": station.Municipality, "state": station.State, "status": station.Status, "record_count": station.RecordCount})
	}
	var nextCursor *string
	if next != "" {
		sealed := httpapi.Seal(h.Secrets, fh, next, h.now())
		nextCursor = &sealed
	}
	body, _ := json.Marshal(map[string]any{
		"items": wire, "next_cursor": nextCursor,
		"lease": wireLeaseOf(lease), "generated_at": generatedAt(h.now()),
	})
	httpapi.WriteJSONWithETag(w, r, http.StatusOK, "public, max-age=60", body,
		httpapi.ETag(mustMarshal(map[string]any{"items": wire, "next_cursor": next, "lease": wireLeaseOf(lease)})))
}

// enforcementStationCoverage reports distinct bound stations versus
// distinct bound records for the leased enforcement revision.
func (h Handler) enforcementStationCoverage(w http.ResponseWriter, r *http.Request) {
	if h.disabled(w, r) {
		return
	}
	if !h.selectRevision(w, r) {
		return
	}
	lease, err := h.leaseOf(r, actionSource, actionStream)
	if err != nil {
		h.readErr(w, r, err)
		return
	}
	if lease.SourceInputContract != application.EnforcementSourceProfile {
		httpapi.WriteError(w, r, http.StatusBadRequest, "quality.bad-source", "station coverage unavailable for this source", nil)
		return
	}
	coverage, _, err := h.Read.EnforcementStationCoverageAtLease(r.Context(), lease)
	if err != nil {
		sourceDown(w, r)
		return
	}
	body, _ := json.Marshal(map[string]any{
		"stations": coverage.Stations, "records": coverage.Records,
		"lease": wireLeaseOf(lease), "generated_at": generatedAt(h.now()),
	})
	httpapi.WriteJSONWithETag(w, r, http.StatusOK, "public, max-age=60", body,
		httpapi.ETag(mustMarshal(map[string]any{"coverage": coverage, "lease": wireLeaseOf(lease)})))
}

func (h Handler) sanctionOne(w http.ResponseWriter, r *http.Request) {
	h.eventOne(w, r, sanctionSource, sanctionStream)
}

func (h Handler) eventOne(w http.ResponseWriter, r *http.Request, source, stream string) {
	if h.disabled(w, r) {
		return
	}
	qValues, ok := eventQuery(w, r, source, true)
	if !ok {
		return
	}
	if !h.selectRevision(w, r) {
		return
	}

	if _, scoped := qValues["station_id"]; scoped {
		h.companyOne(w, r)
		return
	}
	id, err := application.ValidateStationID(chi.URLParam(r, "id"))
	if err != nil {
		httpapi.WriteError(w, r, http.StatusBadRequest, "quality.bad-id", "id must be a UUID",
			[]httpapi.Detail{{Field: "id", Code: "uuid"}})
		return
	}
	if source == actionSource {
		event, lease, err := h.Read.EnforcementOne(r.Context(), h.now(), source, stream, id)
		if err != nil {
			h.readErr(w, r, err)
			return
		}
		wire := wireEnforcement(event)
		wire["lease"] = wireLeaseOf(lease)
		wire["generated_at"] = generatedAt(h.now())
		body, _ := json.Marshal(wire)
		httpapi.WriteJSON(w, r, http.StatusOK, "public, max-age=60", body)
		return
	}
	event, lease, err := h.Read.SanctionOne(r.Context(), h.now(), source, stream, id)
	if err != nil {
		h.readErr(w, r, err)
		return
	}
	wire := wireSanction(event)
	wire["lease"] = wireLeaseOf(lease)
	wire["generated_at"] = generatedAt(h.now())
	body, _ := json.Marshal(wire)
	httpapi.WriteJSON(w, r, http.StatusOK, "public, max-age=60", body)
}

func (h Handler) readErr(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, read.ErrNotFound) || errors.Is(err, read.ErrNoLease) {
		httpapi.WriteError(w, r, http.StatusNotFound, "quality.not-found", "row outside the selected lease", nil)
		return
	}
	sourceDown(w, r)
}

func mustMarshal(v any) []byte {
	body, _ := json.Marshal(v)
	return body
}

func nullableString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func wireSanction(e read.SanctionEvent) map[string]any {
	return map[string]any{"id": e.ID, "native_key": e.NativeKey, "identity_kind": e.IdentityKind, "proceeding_ref": e.ProceedingRef, "amount_minor": e.AmountMinor, "status": e.Status, "decided_on": nullableString(e.DecidedOn), "as_of": nullableString(e.AsOf), "as_of_precision": e.ClockPrecision, "extraction_month": nullableString(e.ExtractionMonth), "report_year": e.ReportYear, "duf_ref": nullableString(e.DUFRef), "department": nullableString(e.Department), "due_on": nullableString(e.DueOn), "amount_text": nullableString(e.AmountText), "paid_text": nullableString(e.PaidText), "paid_minor": e.PaidMinor, "evidence_ref": e.EvidenceRef, "input_contract": nullableString(e.InputContract), "source_period": nullableString(e.SourcePeriod), "source_clock_value": nullableString(e.SourceClockValue), "reference_label": nullableString(e.ReferenceLabel), "reference_ref": nullableString(e.ReferenceRef), "source_transit_on": nullableString(e.SourceTransitOn), "unlabelled_field": nullableString(e.UnlabelledField)}
}

// wireEnforcement keeps native source facts separate from legacy event semantics.
func wireEnforcement(e read.EnforcementEvent) map[string]any {
	out := map[string]any{"id": e.ID, "native_key": e.NativeKey, "scope": e.Scope, "status": e.Status, "occurred_on": nullableString(e.OccurredOn), "as_of": nullableString(e.AsOf), "evidence_ref": e.EvidenceRef}
	if e.SourceProfile != "" {
		out["scope"] = nil
		out["status"] = nil
		out["source_profile"] = e.SourceProfile
		out["subject_kind"] = e.SubjectKind
		out["uf"] = e.UF
		out["municipality"] = e.Municipality
		out["segment"] = e.Segment
		out["document_day"] = e.DocumentDay
		out["date_basis"] = domain.DateBasisDocumentDay
		out["document_number"] = e.DocumentNumber
		out["procedure"] = e.Procedure
		out["result"] = e.Result
	}
	return out
}
