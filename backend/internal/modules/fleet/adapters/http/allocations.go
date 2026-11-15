package http

import (
	"context"
	"fmt"

	company "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/adapters/http"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	guarded "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/adapters/http"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	organization "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters/http"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/go-chi/chi/v5"
)

const AssetUnitAllocationListPath = AssetPrefix + "/{asset_id}/unit-allocations/list"
const AssetCostCentreAllocationListPath = AssetPrefix + "/{asset_id}/cost-centre-allocations/list"

// allocationRoutes binds each EMP-FLEET-005 history to its path, response
// field and cursor namespace.
var allocationRoutes = []struct {
	path, field, token string
	history            app.AllocationHistory
}{
	{AssetUnitAllocationListPath, "unit_id", "unit", app.UnitAllocationHistory},
	{AssetCostCentreAllocationListPath, "cost_centre_id", "cost_centre", app.CostCentreAllocationHistory},
}

// allocationListRequest admits only the page fields: no instant, structure or
// history selector is caller-chosen.
type allocationListRequest struct {
	companyRequest
	orgapi.Page
}

func (v *allocationListRequest) UnmarshalJSON(raw []byte) error {
	type plain allocationListRequest
	var input plain
	if err := decodeAsset(raw, &input); err != nil {
		return err
	}
	*v = allocationListRequest(input)
	return nil
}

func allocationField(history app.AllocationHistory) (string, bool) {
	for _, route := range allocationRoutes {
		if route.history == history {
			return route.field, true
		}
	}
	return "", false
}

// allocationItems projects stored periods exactly; valid_to is omitted while
// a period is open-ended.
func allocationItems(scope kernel.OrgScope, history app.AllocationHistory, entries []app.AllocationEntry) ([]map[string]any, error) {
	field, ok := allocationField(history)
	if scope.Validate() != nil || !ok {
		return nil, orgapi.Unavailable
	}
	items := make([]map[string]any, 0, len(entries))
	for _, entry := range entries {
		target, err := kernel.ParseOrgID(entry.TargetID)
		if err != nil || target.String() != entry.TargetID || entry.RecordedAt.IsZero() {
			return nil, orgapi.Unavailable
		}
		period, err := d.NewAllocationPeriod(entry.Period.From, entry.Period.To)
		if err != nil {
			return nil, orgapi.Unavailable
		}
		item := map[string]any{field: entry.TargetID, "valid_from": orgtime.FormatInstant(period.From), "recorded_at": orgtime.FormatInstant(entry.RecordedAt)}
		if period.To != nil {
			item["valid_to"] = orgtime.FormatInstant(*period.To)
		}
		items = append(items, item)
	}
	return items, nil
}

// RegisterAssetAllocations exposes the EMP-FLEET-005 history reads through the
// existing company pipeline with current fleet.asset.read for the asset.
func RegisterAssetAllocations(router chi.Router, service app.AllocationHistoryService, cookies company.Handler, gateway orgapi.Gateway, authorization access.Authorization) error {
	if !service.Enabled {
		return nil
	}
	if router == nil || missing(service.Repository) || missing(authorization.Source) || missing(authorization.Capabilities) || missing(authorization.Records) || authorization.Now == nil {
		return app.ErrUnavailable
	}
	owned, gateway, err := organization.CompanyEntry(router, cookies, gateway)
	if err != nil {
		return err
	}
	for _, route := range allocationRoutes {
		if err := guarded.Register(owned, route.path, gateway, assetRule(false, false, 200), authorization, app.Policy(false), assetResource{}, func(ctx context.Context, p orgapi.Principal, decision access.Decision, input allocationListRequest) (any, error) {
			actor, err := currentActor(ctx)
			if err != nil {
				return nil, err
			}
			id, err := assetPathID(ctx)
			if err != nil {
				return nil, orgapi.NotFound
			}
			page, err := input.Page.Normalize()
			if err != nil {
				return nil, orgapi.Malformed
			}
			binding := fmt.Sprintf("fleet.asset.%s_allocations.v1:%s:%d", route.token, id, page.Limit)
			after, err := gateway.OpenCursor(p, route.path, binding, page.Cursor)
			if err != nil {
				return nil, orgapi.Malformed
			}
			filter := app.AllocationFilter{Limit: page.Limit}
			if after != "" {
				at, err := orgtime.ParseInstant(after)
				if err != nil || orgtime.FormatInstant(at) != after {
					return nil, orgapi.Malformed
				}
				filter.After = &at
			}
			v, err := service.History(ctx, p.Scope, actor, decision, id, route.history, filter)
			if err != nil {
				return nil, assetMapped(err)
			}
			items, err := allocationItems(p.Scope, route.history, v.Entries)
			if err != nil {
				return nil, err
			}
			next := ""
			if v.Next != nil {
				next, err = gateway.SealCursor(p, route.path, binding, orgtime.FormatInstant(*v.Next))
				if err != nil {
					return nil, orgapi.Unavailable
				}
			}
			return map[string]any{"items": items, "next_cursor": next}, nil
		}); err != nil {
			return err
		}
	}
	return nil
}
