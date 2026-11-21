package http

import (
	"context"
	"encoding/json"
	"errors"
	"mime"
	"net/http"
	"strings"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgdata/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgdata/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/go-chi/chi/v5"
)

const RegionalSeriesPath = "/v1/data/orgs/{org_id}/datasets/{dataset_key}/regional-series"

type regionalReader interface {
	List(context.Context, kernel.OrgScope, domain.Bearer, string, adapters.SeriesInput) (adapters.SeriesPage, error)
}

type seriesRequest struct {
	SchemaVersion int     `json:"schema_version"`
	RequestID     string  `json:"request_id"`
	Revisions     []int64 `json:"revisions"`
	orgapi.Page
}

func RegisterRegionalSeries(router chi.Router, service regionalReader) {
	router.Post(RegionalSeriesPath, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Del("ETag")
		id, err := kernel.ParseOrgID(chi.URLParam(r, "org_id"))
		dataset := chi.URLParam(r, "dataset_key")
		if err != nil || id.String() != chi.URLParam(r, "org_id") || !domain.ValidDatasetKey(dataset) {
			orgapi.WriteError(w, r, orgapi.NotFound)
			return
		}
		scope, err := kernel.NewOrgScope(id)
		if err != nil {
			orgapi.WriteError(w, r, orgapi.NotFound)
			return
		}
		authorization := r.Header.Values("Authorization")
		if len(authorization) != 1 {
			orgapi.WriteError(w, r, orgapi.Unauthenticated)
			return
		}
		rawKey, hasBearer := strings.CutPrefix(authorization[0], "Bearer ")
		key, err := domain.ParseBearer(rawKey)
		if !hasBearer || err != nil {
			orgapi.WriteError(w, r, orgapi.Unauthenticated)
			return
		}
		media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || media != "application/json" || len(r.Header.Values("Content-Type")) != 1 || r.URL.RawQuery != "" {
			orgapi.WriteError(w, r, orgapi.Malformed)
			return
		}
		var input seriesRequest
		raw, err := httpapi.ReadBody(r, orgapi.MaxBodyBytes)
		if err != nil || httpapi.DecodeJSON(raw, &input) != nil || input.SchemaVersion != 1 || !audit.ValidID(input.RequestID) {
			orgapi.WriteError(w, r, orgapi.Malformed)
			return
		}
		page, err := service.List(r.Context(), scope, key, input.RequestID, adapters.SeriesInput{
			Dataset: dataset, Revisions: input.Revisions, Limit: input.Limit, Cursor: input.Cursor})
		if err != nil {
			orgapi.WriteError(w, r, seriesError(err))
			return
		}
		body, err := json.Marshal(page)
		if err != nil {
			orgapi.WriteError(w, r, orgapi.Unavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	})
}

func seriesError(err error) error {
	var limited adapters.RateExceeded
	switch {
	case errors.As(err, &limited):
		return &orgapi.RateLimitError{RetryAfter: limited.RetryAfter}
	case errors.Is(err, adapters.ErrQuotaExceeded):
		return orgapi.LimitReached
	case errors.Is(err, adapters.ErrUsageConflict):
		return orgapi.Conflict
	case errors.Is(err, adapters.ErrSeriesCursor):
		return orgapi.Malformed
	case errors.Is(err, domain.ErrInvalidDataset):
		return orgapi.ValidationFailed
	case errors.Is(err, domain.ErrKeyDenied), errors.Is(err, domain.ErrRightsPending), errors.Is(err, adapters.ErrDatasetNotFound):
		return orgapi.NotFound
	default:
		return orgapi.Unavailable
	}
}
