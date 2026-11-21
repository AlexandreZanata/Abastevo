package http

import (
	"context"
	"strconv"
	"strings"
	"time"

	company "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/adapters/http"
	guarded "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/adapters/http"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	organization "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters/http"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgdata/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgdata/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/go-chi/chi/v5"
)

const UsageReadPath = "/v1/orgs/{org_id}/data/usage/read"

type termView struct {
	Start   time.Time `json:"start"`
	End     time.Time `json:"end"`
	Used    string    `json:"used"`
	Ceiling string    `json:"ceiling"`
}

type rateView struct {
	Calls         string `json:"calls"`
	WindowSeconds int32  `json:"window_seconds"`
}

type datasetView struct {
	Key             string `json:"key"`
	Revision        string `json:"revision"`
	Name            string `json:"name"`
	Region          string `json:"region"`
	Method          string `json:"method"`
	MinimumStations int32  `json:"minimum_stations"`
}

type usageResponse struct {
	ObservedAt time.Time     `json:"observed_at"`
	Term       *termView     `json:"term,omitempty"`
	Rate       *rateView     `json:"rate,omitempty"`
	Enabled    bool          `json:"enabled"`
	Datasets   []datasetView `json:"datasets"`
	NextCursor string        `json:"next_cursor"`
}

func RegisterUsage(router chi.Router, service app.UsageReader, cookies company.Handler, gateway orgapi.Gateway, authorization access.Authorization) error {
	if service == nil {
		return app.ErrUnavailable
	}
	owned, gateway, err := organization.CompanyEntry(router, cookies, gateway)
	if err != nil {
		return err
	}
	policy := app.UsagePolicy()
	records := keyRecords{policy: policy}
	authorization.Records = records
	rule := orgapi.Rule{Permission: string(policy.Permission), CompanyOnly: true, Success: 200}
	return guarded.Register(owned, UsageReadPath, gateway, rule, authorization, policy, records, func(ctx context.Context, p orgapi.Principal, _ access.Decision, in listRequest) (any, error) {
		actor, err := organization.CurrentCompanyActor(ctx)
		if err != nil {
			return nil, orgapi.Unauthenticated
		}
		page, err := in.Page.Normalize()
		if err != nil {
			return nil, err
		}
		cursor, err := gateway.OpenCursor(p, UsageReadPath, "", page.Cursor)
		if err != nil {
			return nil, err
		}
		var after app.DatasetAfter
		if cursor != "" {
			key, revision, ok := strings.Cut(cursor, ":")
			id, err := strconv.ParseInt(revision, 10, 64)
			if !ok || err != nil || id < 1 || !domain.ValidDatasetKey(key) {
				return nil, orgapi.Malformed
			}
			after = app.DatasetAfter{Key: key, Revision: id}
		}
		view, err := service.ReadUsage(ctx, p.Scope, actor, p.MembershipID, after, page.Limit)
		if err != nil {
			return nil, scopedDataError(string(policy.Permission), err)
		}
		out := usageResponse{ObservedAt: view.ObservedAt, Enabled: view.Enabled, Datasets: make([]datasetView, 0, len(view.Datasets))}
		if view.Term != nil {
			out.Term = &termView{Start: view.Term.Start, End: view.Term.End,
				Used: strconv.FormatInt(view.Term.Used, 10), Ceiling: strconv.FormatInt(view.Term.Ceiling, 10)}
		}
		if view.Rate != nil {
			out.Rate = &rateView{Calls: strconv.FormatInt(view.Rate.Calls, 10), WindowSeconds: view.Rate.WindowSeconds}
		}
		for _, row := range view.Datasets {
			out.Datasets = append(out.Datasets, datasetView{Key: row.Key, Revision: strconv.FormatInt(row.Revision, 10),
				Name: row.Name, Region: row.Region, Method: row.Method, MinimumStations: row.MinimumStations})
		}
		if view.After.Key != "" {
			out.NextCursor, err = gateway.SealCursor(p, UsageReadPath, "", view.After.Key+":"+strconv.FormatInt(view.After.Revision, 10))
		}
		return out, err
	})
}
