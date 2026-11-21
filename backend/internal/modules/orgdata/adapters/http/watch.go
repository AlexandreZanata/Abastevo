package http

import (
	"context"
	"errors"
	"strconv"

	company "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/adapters/http"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	organization "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters/http"
	orgapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	data "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgdata/adapters"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgdata/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgdata/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/go-chi/chi/v5"
)

const (
	ListWatchesPath  = "/v1/orgs/{org_id}/data/watchlists/list"
	CreateWatchPath  = "/v1/orgs/{org_id}/data/watchlists/create"
	DisableWatchPath = "/v1/orgs/{org_id}/data/watchlists/{watch_id}/disable"
)

type createWatchRequest struct {
	orgapi.Command
	Dataset  string         `json:"dataset"`
	Fuel     kernel.Product `json:"fuel"`
	Baseline string         `json:"baseline_revision"`
}

func (r *createWatchRequest) UnmarshalJSON(raw []byte) error {
	type plain createWatchRequest
	return keyRequestObject(raw, (*plain)(r))
}

type watchView struct {
	ID           string         `json:"id"`
	Dataset      string         `json:"dataset"`
	Fuel         kernel.Product `json:"fuel"`
	Baseline     string         `json:"baseline_revision"`
	LastObserved string         `json:"last_observed_revision"`
	Active       bool           `json:"active"`
}

func watchDTO(w app.Watch) watchView {
	return watchView{ID: w.ID, Dataset: w.Dataset, Fuel: w.Fuel, Baseline: strconv.FormatInt(w.Baseline, 10),
		LastObserved: strconv.FormatInt(w.LastObserved, 10), Active: w.Active}
}

type watchList struct {
	Items      []watchView `json:"watchlists"`
	NextCursor string      `json:"next_cursor"`
}

type watchResult struct {
	Watch    watchView `json:"watchlist"`
	Replayed bool      `json:"replayed"`
}

func RegisterWatches(router chi.Router, service app.WatchCommands, cookies company.Handler, gateway orgapi.Gateway, authorization access.Authorization) error {
	if service == nil {
		return app.ErrUnavailable
	}
	owned, gateway, err := organization.CompanyEntry(router, cookies, gateway)
	if err != nil {
		return err
	}
	if err := register(owned, ListWatchesPath, gateway, authorization, app.ListKeys, false, func(ctx context.Context, p orgapi.Principal, actor orgapp.CompanyActor, in listRequest) (any, error) {
		page, err := in.Page.Normalize()
		if err != nil {
			return nil, err
		}
		after, err := gateway.OpenCursor(p, ListWatchesPath, "", page.Cursor)
		if err != nil {
			return nil, err
		}
		rows, err := service.ListWatches(ctx, p.Scope, actor, p.MembershipID, after, page.Limit)
		if err != nil {
			return nil, watchError(err)
		}
		result := watchList{Items: make([]watchView, 0, len(rows.Items))}
		for _, row := range rows.Items {
			result.Items = append(result.Items, watchDTO(row))
		}
		if rows.After != "" {
			result.NextCursor, err = gateway.SealCursor(p, ListWatchesPath, "", rows.After)
		}
		return result, err
	}); err != nil {
		return err
	}
	if err := register(owned, CreateWatchPath, gateway, authorization, app.IssueKey, false, func(ctx context.Context, p orgapi.Principal, actor orgapp.CompanyActor, in createWatchRequest) (any, error) {
		baseline, err := strconv.ParseInt(in.Baseline, 10, 64)
		if err != nil || baseline < 1 || strconv.FormatInt(baseline, 10) != in.Baseline || in.Revision != nil {
			return nil, orgapi.Malformed
		}
		watch, replayed, err := service.CreateWatch(ctx, p.Scope, actor, p.MembershipID, app.WatchInput{
			OperationID: in.OperationID, Dataset: in.Dataset, Fuel: in.Fuel, Revision: baseline,
		})
		if err != nil {
			return nil, watchError(err)
		}
		return watchResult{Watch: watchDTO(watch), Replayed: replayed}, nil
	}); err != nil {
		return err
	}
	return register(owned, DisableWatchPath, gateway, authorization, app.RevokeKey, false, func(ctx context.Context, p orgapi.Principal, actor orgapp.CompanyActor, in revokeRequest) (any, error) {
		if in.Revision != nil {
			return nil, orgapi.Malformed
		}
		watch, replayed, err := service.DisableWatch(ctx, p.Scope, actor, p.MembershipID, chi.URLParamFromCtx(ctx, "watch_id"), in.OperationID)
		if err != nil {
			return nil, watchError(err)
		}
		return watchResult{Watch: watchDTO(watch), Replayed: replayed}, nil
	})
}

func watchError(err error) error {
	switch {
	case errors.Is(err, data.ErrDatasetNotFound), errors.Is(err, domain.ErrRightsPending):
		return orgapi.NotFound
	case errors.Is(err, data.ErrWatchConflict):
		return orgapi.Conflict
	case errors.Is(err, access.ErrEntitlementRequired):
		return orgapi.EntitlementRequired
	}
	return scopedDataError("data.key.manage", err)
}
