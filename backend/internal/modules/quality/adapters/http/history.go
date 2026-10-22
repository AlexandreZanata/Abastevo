package http

import (
	"encoding/json"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
	"github.com/go-chi/chi/v5"
	"net/http"
	"strconv"
)

func (h *Handler) selectRevision(w http.ResponseWriter, r *http.Request) bool {
	id := r.URL.Query().Get("revision_id")
	if id == "" {
		return true
	}
	id, e := application.ValidateStationID(id)
	if e != nil {
		httpapi.WriteError(w, r, 400, "quality.bad-revision", "revision id must be a UUID", nil)
		return false
	}
	if h.Read == nil {
		sourceDown(w, r)
		return false
	}
	h.Read = h.Read.AtRevision(id)
	return true
}
func revisionStream(w http.ResponseWriter, r *http.Request) (string, string, bool) {
	stream := r.URL.Query().Get("stream")
	switch stream {
	case assayStream:
		return assaySource, stream, true
	case actionStream:
		return actionSource, stream, true
	case sanctionStream:
		return sanctionSource, stream, true
	default:
		httpapi.WriteError(w, r, 400, "quality.bad-stream", "known quality stream required", nil)
		return "", "", false
	}
}
func (h Handler) revisions(w http.ResponseWriter, r *http.Request) {
	if h.disabled(w, r) {
		return
	}
	source, stream, ok := revisionStream(w, r)
	if !ok {
		return
	}
	limit, e := httpapi.ParseLimit(r.URL.Query().Get("limit"))
	if e != nil {
		httpapi.WriteError(w, r, 400, "quality.bad-limit", "limit must be 1..100", nil)
		return
	}
	fh := httpapi.FilterHash("quality-revisions", source, stream, strconv.Itoa(limit))
	after, e := httpapi.ParseCursor(h.Secrets, r.URL.Query().Get("cursor"), fh, h.now())
	if e != nil {
		httpapi.WriteError(w, r, 400, "quality.bad-cursor", "cursor invalid, expired or changed", nil)
		return
	}
	rows, nextKey, e := h.Read.Revisions(r.Context(), source, stream, after, limit)
	if e != nil {
		h.readErr(w, r, e)
		return
	}
	var next *string
	if nextKey != "" {
		v := httpapi.Seal(h.Secrets, fh, nextKey, h.now())
		next = &v
	}
	body, _ := json.Marshal(map[string]any{"source": source, "stream": stream, "items": rows, "next_cursor": next, "catalog_live": true, "generated_at": generatedAt(h.now())})
	httpapi.WriteJSONWithETag(w, r, 200, "public, max-age=60", body, httpapi.ETag(mustMarshal(map[string]any{"source": source, "stream": stream, "items": rows, "next_cursor": nextKey})))
}
func (h Handler) revisionDetail(w http.ResponseWriter, r *http.Request) {
	if h.disabled(w, r) {
		return
	}
	source, stream, ok := revisionStream(w, r)
	if !ok {
		return
	}
	id, e := application.ValidateStationID(chi.URLParam(r, "id"))
	if e != nil {
		httpapi.WriteError(w, r, 400, "quality.bad-revision", "revision id must be a UUID", nil)
		return
	}
	lease, e := h.Read.RevisionLease(r.Context(), h.now(), source, stream, id)
	if e != nil {
		h.readErr(w, r, e)
		return
	}
	resources, e := h.Read.RevisionResources(r.Context(), lease)
	if e != nil {
		h.readErr(w, r, e)
		return
	}
	var counts map[string]int
	var manifest *string
	if lease.HasPublished {
		counts = map[string]int{"input": lease.Counts.Input, "accepted": lease.Counts.Accepted, "duplicates": lease.Counts.Duplicates, "quarantined": lease.Counts.Quarantined}
		manifest = nullableString(lease.ManifestHash)
	}
	stable := map[string]any{"lease": wireLeaseOf(lease), "counts": counts, "manifest_hash": manifest, "resources": resources}
	output := map[string]any{"lease": stable["lease"], "counts": counts, "manifest_hash": manifest, "resources": resources, "generated_at": generatedAt(h.now())}
	body, _ := json.Marshal(output)
	httpapi.WriteJSONWithETag(w, r, 200, "public, max-age=60", body, httpapi.ETag(mustMarshal(stable)))
}
