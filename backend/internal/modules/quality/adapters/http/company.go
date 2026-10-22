package http

import (
	"encoding/json"
	"errors"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters/read"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
	"github.com/go-chi/chi/v5"
	"net/http"
	"net/url"
	"strconv"
)

var errEventQuery = errors.New("quality: invalid event query")

// Validate the complete public filter before choosing scoped/global behavior.
func eventQuery(w http.ResponseWriter, r *http.Request, source string, detail bool) (url.Values, bool) {
	q, e := url.ParseQuery(r.URL.RawQuery)
	allowed := map[string]bool{"revision_id": true, "station_id": true}
	if !detail {
		allowed["status"] = true
		allowed["limit"] = true
		allowed["cursor"] = true
		if source == actionSource {
			allowed["scope"] = true
			allowed["uf"] = true
			allowed["municipality"] = true
			allowed["segment"] = true
			allowed["subject_kind"] = true
			allowed["from_day"] = true
			allowed["to_day"] = true
		}
	}
	if e == nil {
		for k, v := range q {
			if !allowed[k] || len(v) != 1 || ((k == "station_id" || k == "revision_id") && v[0] == "") {
				e = errEventQuery
				break
			}
		}
	}
	if _, has := q["station_id"]; has && source != sanctionSource {
		e = errEventQuery
	}
	if e != nil {
		httpapi.WriteError(w, r, 400, "quality.bad-filter", "unsupported, duplicate or invalid query parameter", nil)
		return nil, false
	}
	return q, true
}

func (h Handler) companyContext(w http.ResponseWriter, r *http.Request) (read.Lease, read.CompanyScope, bool) {
	station, e := application.ValidateStationID(r.URL.Query().Get("station_id"))
	if e != nil {
		httpapi.WriteError(w, r, 400, "quality.bad-id", "station id must be a UUID", nil)
		return read.Lease{}, read.CompanyScope{}, false
	}
	if h.CompanyEnabled == nil || !h.CompanyEnabled() {
		httpapi.WriteError(w, r, 404, "quality.company-disabled", "company-scoped reads are off", nil)
		return read.Lease{}, read.CompanyScope{}, false
	}
	if h.Read == nil || len(h.Secrets) < 32 || h.StationsExist == nil {
		sourceDown(w, r)
		return read.Lease{}, read.CompanyScope{}, false
	}
	if !h.exists(w, r, station) {
		return read.Lease{}, read.CompanyScope{}, false
	}
	lease, e := h.leaseOf(r, sanctionSource, sanctionStream)
	if e != nil {
		h.readErr(w, r, e)
		return read.Lease{}, read.CompanyScope{}, false
	}
	scope, e := h.Read.ResolveCompanyScope(r.Context(), h.now(), lease, station)
	if e != nil {
		sourceDown(w, r)
		return read.Lease{}, read.CompanyScope{}, false
	}
	return lease, scope, true
}

func (h Handler) companyEvents(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	status, e := application.ValidateTextFilter("status", q.Get("status"), 64)
	if e != nil {
		httpapi.WriteError(w, r, 400, "quality.bad-filter", "status filter too long", nil)
		return
	}
	limit, e := httpapi.ParseLimit(q.Get("limit"))
	if e != nil {
		httpapi.WriteError(w, r, 400, "quality.bad-limit", "limit must be 1..100", nil)
		return
	}
	lease, scope, ok := h.companyContext(w, r)
	if !ok {
		return
	}
	fh := httpapi.FilterHash("quality-company-sanctions-v1", scope.View().StationID, status, strconv.Itoa(limit), lease.RevisionID, scope.Fingerprint())
	last, e := httpapi.ParseCursor(h.Secrets, q.Get("cursor"), fh, h.now())
	if e != nil {
		httpapi.WriteError(w, r, 400, "quality.bad-cursor", "cursor invalid, expired or changed", nil)
		return
	}
	afterNative, afterID := splitCursor(last)
	rows, nextKey, e := h.Read.CompanySanctionsAtScope(r.Context(), lease, scope, status, afterNative, afterID, limit)
	if e != nil {
		h.readErr(w, r, e)
		return
	}
	items := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		items = append(items, wireSanction(row))
	}
	var next *string
	if nextKey != "" {
		v := httpapi.Seal(h.Secrets, fh, nextKey, h.now())
		next = &v
	}
	stable := map[string]any{"items": items, "next_cursor": nextKey, "lease": wireLeaseOf(lease), "company_scope": scope.View(), "scope_fingerprint": scope.Fingerprint()}
	body, _ := json.Marshal(map[string]any{"items": items, "next_cursor": next, "lease": wireLeaseOf(lease), "company_scope": scope.View(), "generated_at": generatedAt(h.now())})
	httpapi.WriteJSONWithETag(w, r, 200, "public, max-age=0, must-revalidate", body, httpapi.ETag(mustMarshal(stable)))
}

func (h Handler) companyOne(w http.ResponseWriter, r *http.Request) {
	id, e := application.ValidateStationID(chi.URLParam(r, "id"))
	if e != nil {
		httpapi.WriteError(w, r, 400, "quality.bad-id", "id must be a UUID", nil)
		return
	}
	lease, scope, ok := h.companyContext(w, r)
	if !ok {
		return
	}
	row, e := h.Read.CompanySanctionAtScope(r.Context(), lease, scope, id)
	if e != nil {
		h.readErr(w, r, e)
		return
	}
	wire := wireSanction(row)
	wire["lease"] = wireLeaseOf(lease)
	wire["company_scope"] = scope.View()
	stable := map[string]any{"row": wire, "scope_fingerprint": scope.Fingerprint()}
	tag := httpapi.ETag(mustMarshal(stable))
	wire["generated_at"] = generatedAt(h.now())
	body, _ := json.Marshal(wire)
	httpapi.WriteJSONWithETag(w, r, 200, "public, max-age=0, must-revalidate", body, tag)
}
