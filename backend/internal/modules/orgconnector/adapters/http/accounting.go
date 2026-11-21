package http

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	connectorpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgconnector/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgconnector/domain"
	exportapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgexport/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/go-chi/chi/v5"
)

const AccountingPath = "/v1/integrations/orgs/{org_id}/connections/{connection_id}/accounting/{export_id}"

func RegisterAccounting(router chi.Router, downloads connectorpg.AccountingDownloads) {
	router.Get(AccountingPath, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if r.URL.RawQuery != "" || r.ContentLength != 0 || len(r.TransferEncoding) > 0 {
			orgapi.WriteError(w, r, orgapi.Malformed)
			return
		}
		auth := r.Header.Values("Authorization")
		if len(auth) != 1 || !strings.HasPrefix(auth[0], "Bearer ") {
			orgapi.WriteError(w, r, orgapi.Unauthenticated)
			return
		}
		orgID := chi.URLParam(r, "org_id")
		org, err := kernel.ParseOrgID(orgID)
		if err != nil || org.String() != orgID {
			orgapi.WriteError(w, r, orgapi.NotFound)
			return
		}
		scope, err := kernel.NewOrgScope(org)
		if err != nil {
			orgapi.WriteError(w, r, orgapi.NotFound)
			return
		}
		body, etag, err := downloads.Download(r.Context(), scope, chi.URLParam(r, "connection_id"),
			chi.URLParam(r, "export_id"), strings.TrimPrefix(auth[0], "Bearer "), r.Header.Get("If-None-Match"))
		switch {
		case errors.Is(err, exportapp.ErrUnauthenticated):
			orgapi.WriteError(w, r, orgapi.Unauthenticated)
		case errors.Is(err, domain.ErrDenied), errors.Is(err, exportapp.ErrRightsLost), errors.Is(err, exportapp.ErrNotFound):
			orgapi.WriteError(w, r, orgapi.NotFound)
		case err != nil:
			orgapi.WriteError(w, r, orgapi.Unavailable)
		case body == nil:
			w.Header().Set("ETag", etag)
			w.WriteHeader(http.StatusNotModified)
		default:
			w.Header().Set("ETag", etag)
			w.Header().Set("Content-Type", "text/csv; charset=utf-8")
			w.Header().Set("Content-Disposition", `attachment; filename="accounting-v1.csv"`)
			w.Header().Set("Content-Length", strconv.Itoa(len(body)))
			_, _ = w.Write(body)
		}
	})
}
