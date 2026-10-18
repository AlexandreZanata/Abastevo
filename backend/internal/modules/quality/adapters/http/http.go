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
	Read          *read.Reader
	Secrets       []byte
	StationsExist func(ctx context.Context, id string) (bool, error)
	Enabled       func() bool
	Now           func() time.Time
}

// RegisterRoutes mounts the quality reads under /v1/quality.
func (h Handler) RegisterRoutes(r chi.Router) {
	r.Get("/v1/quality/revisions", h.revisions)
	r.Get("/v1/quality/revisions/{id}", h.revisionDetail)
	r.Get("/v1/quality/summary", h.summary)
	r.Get("/v1/quality/assays", h.assays)
	r.Get("/v1/quality/samples/{sample_ref}", h.sample)
	r.Get("/v1/quality/actions", h.actions)
	r.Get("/v1/quality/actions/{id}", h.actionOne)
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
	SupersedesRevisionID *string      `json:"supersedes_revision_id"`
	RevisionStatus       string       `json:"revision_status"`
	ProvenanceState      string       `json:"provenance_state"`
	IsCurrentStream      bool         `json:"is_current_stream"`
	Source               string       `json:"source"`
	Stream               string       `json:"stream"`
	Edition              string       `json:"edition"`
	RevisionID           *string      `json:"revision_id"`
	PublishedAt          *string      `json:"published_at"`
	Available            bool         `json:"available"`
	Stale                bool         `json:"stale"`
	StaleReason          string       `json:"stale_reason"`
	CoverageNote         string       `json:"coverage_note"`
	Periods              []wirePeriod `json:"periods"`
}

func wireLeaseOf(l read.Lease) wireLease {
	out := wireLease{
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

func (h Handler) sanctions(w http.ResponseWriter, r *http.Request) {
	h.events(w, r, sanctionSource, sanctionStream, "sanctions")
}

func (h Handler) events(w http.ResponseWriter, r *http.Request, source, stream, name string) {
	if h.disabled(w, r) {
		return
	}
	if !h.selectRevision(w, r) {
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
	fh := httpapi.FilterHash("quality-"+name, status, scope, strconv.Itoa(limit), lease.RevisionID)
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
		rows, next, _, err := h.Read.EnforcementAtLease(r.Context(), lease, status, scope, afterNative, afterID, limit)
		if err != nil {
			sourceDown(w, r)
			return
		}
		nextKey = next
		for _, e := range rows {
			wire = append(wire, map[string]any{
				"id": e.ID, "native_key": e.NativeKey, "scope": e.Scope,
				"status": e.Status, "occurred_on": nullableString(e.OccurredOn),
				"as_of": e.AsOf, "evidence_ref": e.EvidenceRef,
			})
		}
	} else {
		rows, next, _, err := h.Read.SanctionsAtLease(r.Context(), lease, status, afterNative, afterID, limit)
		if err != nil {
			sourceDown(w, r)
			return
		}
		nextKey = next
		for _, e := range rows {
			wire = append(wire, map[string]any{
				"id": e.ID, "native_key": e.NativeKey,
				"proceeding_ref": e.ProceedingRef, "amount_minor": e.AmountMinor,
				"status": e.Status, "decided_on": nullableString(e.DecidedOn),
				"as_of": e.AsOf, "evidence_ref": e.EvidenceRef,
			})
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

func (h Handler) sanctionOne(w http.ResponseWriter, r *http.Request) {
	h.eventOne(w, r, sanctionSource, sanctionStream)
}

func (h Handler) eventOne(w http.ResponseWriter, r *http.Request, source, stream string) {
	if h.disabled(w, r) {
		return
	}
	if !h.selectRevision(w, r) {
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
		body, _ := json.Marshal(map[string]any{
			"id": event.ID, "native_key": event.NativeKey, "scope": event.Scope,
			"status": event.Status, "occurred_on": nullableString(event.OccurredOn),
			"as_of": event.AsOf, "evidence_ref": event.EvidenceRef,
			"lease": wireLeaseOf(lease), "generated_at": generatedAt(h.now()),
		})
		httpapi.WriteJSON(w, r, http.StatusOK, "public, max-age=60", body)
		return
	}
	event, lease, err := h.Read.SanctionOne(r.Context(), h.now(), source, stream, id)
	if err != nil {
		h.readErr(w, r, err)
		return
	}
	body, _ := json.Marshal(map[string]any{
		"id": event.ID, "native_key": event.NativeKey,
		"proceeding_ref": event.ProceedingRef, "amount_minor": event.AmountMinor,
		"status": event.Status, "decided_on": nullableString(event.DecidedOn),
		"as_of": event.AsOf, "evidence_ref": event.EvidenceRef,
		"lease": wireLeaseOf(lease), "generated_at": generatedAt(h.now()),
	})
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
