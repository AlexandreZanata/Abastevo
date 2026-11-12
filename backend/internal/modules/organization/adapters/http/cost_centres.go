package http

import (
	"context"
	"errors"
	"fmt"

	company "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/adapters/http"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	guarded "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/adapters/http"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permissions "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/go-chi/chi/v5"
)

const CostCentrePrefix = "/v1/orgs/{org_id}/cost-centres"
const CostCentreCreatePath = CostCentrePrefix + "/create"
const CostCentreListPath = CostCentrePrefix + "/list"
const CostCentreCodePath = CostCentrePrefix + "/code/read"
const CostCentreReadPath = CostCentrePrefix + "/{cost_centre_id}/read"
const CostCentreChangePath = CostCentrePrefix + "/{cost_centre_id}/change"
const CostCentreArchivePath = CostCentrePrefix + "/{cost_centre_id}/archive"
const CostCentreRestorePath = CostCentrePrefix + "/{cost_centre_id}/restore"

type costCentreFields struct {
	privateRequest
	Name string `json:"name"`
	Code string `json:"code"`
}

func (v costCentreFields) domain() d.CostCentreProfile {
	return d.CostCentreProfile{Name: v.Name, Code: v.Code}
}

type costCentreCreateRequest struct {
	privateRequest
	companyRequest
	SchemaVersion int    `json:"schema_version"`
	OperationID   string `json:"operation_id"`
	costCentreFields
}

func (v costCentreCreateRequest) CommandFields() orgapi.Command {
	return orgapi.Command{SchemaVersion: v.SchemaVersion, OperationID: v.OperationID}
}

type costCentreChangeRequest struct {
	privateRequest
	unitRevisionCommand
	costCentreFields
}
type costCentreTransitionRequest struct{ unitRevisionCommand }
type costCentreReadRequest struct{ companyRequest }
type costCentreCodeRequest struct {
	companyRequest
	Code string `json:"code"`
}
type costCentreListRequest struct {
	companyRequest
	orgapi.Page
	State d.CostCentreState `json:"state,omitempty"`
}

func (v *costCentreCreateRequest) UnmarshalJSON(raw []byte) error {
	type plain costCentreCreateRequest
	var input plain
	if err := decodeLegal(raw, &input); err != nil {
		return err
	}
	*v = costCentreCreateRequest(input)
	return nil
}
func (v *costCentreChangeRequest) UnmarshalJSON(raw []byte) error {
	type plain costCentreChangeRequest
	var input plain
	if err := decodeLegal(raw, &input); err != nil {
		return err
	}
	*v = costCentreChangeRequest(input)
	return nil
}
func (v *costCentreTransitionRequest) UnmarshalJSON(raw []byte) error {
	type plain costCentreTransitionRequest
	var input plain
	if err := decodeLegal(raw, &input); err != nil {
		return err
	}
	*v = costCentreTransitionRequest(input)
	return nil
}
func (v *costCentreReadRequest) UnmarshalJSON(raw []byte) error {
	type plain costCentreReadRequest
	var input plain
	if err := decodeLegal(raw, &input); err != nil {
		return err
	}
	*v = costCentreReadRequest(input)
	return nil
}
func (v *costCentreCodeRequest) UnmarshalJSON(raw []byte) error {
	type plain costCentreCodeRequest
	var input plain
	if err := decodeLegal(raw, &input); err != nil {
		return err
	}
	*v = costCentreCodeRequest(input)
	return nil
}
func (v *costCentreListRequest) UnmarshalJSON(raw []byte) error {
	type plain costCentreListRequest
	var input plain
	if err := decodeLegal(raw, &input); err != nil {
		return err
	}
	*v = costCentreListRequest(input)
	return nil
}

type costCentreResource struct{ lookup app.CostCentreLookup }

func (r costCentreResource) Resolve(ctx context.Context, scope kernel.OrgScope, _ access.Subject) (permissions.ResourceFacts, error) {
	if r.lookup == nil {
		return permissions.ResourceFacts{}, access.ErrUnavailable
	}
	id, err := legalPathID(ctx, "cost_centre_id")
	if err != nil {
		return permissions.ResourceFacts{}, err
	}
	c, err := r.lookup.FindCostCentre(ctx, scope, id)
	if errors.Is(err, app.ErrNotFound) {
		return permissions.ResourceFacts{}, access.ErrNotFound
	}
	if err != nil {
		return permissions.ResourceFacts{}, access.ErrUnavailable
	}
	if _, err := c.Snapshot(scope); err != nil {
		return permissions.ResourceFacts{}, access.ErrUnavailable
	}
	return permissions.ResourceFacts{OrgID: scope.OrgID(), ID: id}, nil
}
func costCentreMapped(err error) error {
	if errors.Is(err, app.ErrCostCentreCodeReserved) {
		return orgapi.UnitDuplicate("code")
	}
	return unitMapped(err)
}
func costCentreAck(v app.CostCentreResult, replay bool) map[string]any {
	return map[string]any{"cost_centre_id": v.CentreID, "state": v.State, "revision": v.Revision, "data_mode": "SYNTHETIC", "replayed": replay}
}
func costCentreView(scope kernel.OrgScope, c d.CostCentre) (map[string]any, error) {
	v, err := c.Snapshot(scope)
	if err != nil {
		return nil, orgapi.Unavailable
	}
	display := v.Name
	if v.State == d.CostCentreArchived {
		display += " (arquivada)"
	}
	return map[string]any{"id": v.ID, "name": v.Name, "display_name": display, "code": v.Code, "state": v.State, "revision": v.Revision, "data_mode": "SYNTHETIC"}, nil
}
func RegisterCostCentres(router chi.Router, service app.CostCentreService, cookies company.Handler, gateway orgapi.Gateway, authorization access.Authorization, roots app.Repository, lookup app.CostCentreLookup) error {
	if !service.Enabled {
		return nil
	}
	if service.Repository == nil || service.NewID == nil || roots == nil || lookup == nil {
		return app.ErrUnavailable
	}
	owned, gateway, err := companyEntry(router, cookies, gateway)
	if err != nil {
		return err
	}
	resource := costCentreResource{lookup}
	writePolicy, readPolicy := app.StructurePolicy(true), app.StructurePolicy(false)
	if err := guarded.Register(owned, CostCentreCreatePath, gateway, unitRule(true, false, 201), authorization, writePolicy, RootFacts{roots}, func(ctx context.Context, p orgapi.Principal, decision access.Decision, input costCentreCreateRequest) (any, error) {
		actor, err := companyActor(ctx)
		if err != nil {
			return nil, err
		}
		command, err := app.NewCostCentreWrite(app.CostCentreWriteInput{Kind: app.CreateCostCentre, OperationID: input.OperationID, Profile: input.costCentreFields.domain()})
		if err != nil {
			return nil, costCentreMapped(err)
		}
		result, replay, err := service.Write(ctx, p.Scope, actor, decision, command)
		if err != nil {
			return nil, costCentreMapped(err)
		}
		return costCentreAck(result, replay), nil
	}); err != nil {
		return err
	}
	if err := guarded.Register(owned, CostCentreChangePath, gateway, unitRule(true, true, 200), authorization, writePolicy, resource, func(ctx context.Context, p orgapi.Principal, decision access.Decision, input costCentreChangeRequest) (any, error) {
		actor, err := companyActor(ctx)
		if err != nil {
			return nil, err
		}
		id, err := legalPathID(ctx, "cost_centre_id")
		if err != nil {
			return nil, orgapi.NotFound
		}
		command, err := app.NewCostCentreWrite(app.CostCentreWriteInput{Kind: app.ChangeCostCentre, OperationID: input.OperationID, CentreID: id, ExpectedRevision: *input.Revision, Profile: input.costCentreFields.domain()})
		if err != nil {
			return nil, costCentreMapped(err)
		}
		result, replay, err := service.Write(ctx, p.Scope, actor, decision, command)
		if err != nil {
			return nil, costCentreMapped(err)
		}
		return costCentreAck(result, replay), nil
	}); err != nil {
		return err
	}
	for _, transition := range []struct {
		path string
		kind app.CostCentreWriteKind
	}{{CostCentreArchivePath, app.ArchiveCostCentre}, {CostCentreRestorePath, app.RestoreCostCentre}} {
		if err := guarded.Register(owned, transition.path, gateway, unitRule(true, true, 200), authorization, writePolicy, resource, func(ctx context.Context, p orgapi.Principal, decision access.Decision, input costCentreTransitionRequest) (any, error) {
			actor, err := companyActor(ctx)
			if err != nil {
				return nil, err
			}
			id, err := legalPathID(ctx, "cost_centre_id")
			if err != nil {
				return nil, orgapi.NotFound
			}
			command, err := app.NewCostCentreWrite(app.CostCentreWriteInput{Kind: transition.kind, OperationID: input.OperationID, CentreID: id, ExpectedRevision: *input.Revision})
			if err != nil {
				return nil, costCentreMapped(err)
			}
			result, replay, err := service.Write(ctx, p.Scope, actor, decision, command)
			if err != nil {
				return nil, costCentreMapped(err)
			}
			return costCentreAck(result, replay), nil
		}); err != nil {
			return err
		}
	}
	if err := guarded.Register(owned, CostCentreReadPath, gateway, unitRule(false, false, 200), authorization, readPolicy, resource, func(ctx context.Context, p orgapi.Principal, decision access.Decision, _ costCentreReadRequest) (any, error) {
		actor, err := companyActor(ctx)
		if err != nil {
			return nil, err
		}
		id, err := legalPathID(ctx, "cost_centre_id")
		if err != nil {
			return nil, orgapi.NotFound
		}
		c, err := service.Read(ctx, p.Scope, actor, decision, id)
		if err != nil {
			return nil, costCentreMapped(err)
		}
		return costCentreView(p.Scope, c)
	}); err != nil {
		return err
	}
	if err := guarded.Register(owned, CostCentreCodePath, gateway, unitRule(false, false, 200), authorization, readPolicy, RootFacts{roots}, func(ctx context.Context, p orgapi.Principal, decision access.Decision, input costCentreCodeRequest) (any, error) {
		actor, err := companyActor(ctx)
		if err != nil {
			return nil, err
		}
		c, err := service.ReadCode(ctx, p.Scope, actor, decision, input.Code)
		if err != nil {
			return nil, costCentreMapped(err)
		}
		return costCentreView(p.Scope, c)
	}); err != nil {
		return err
	}
	return guarded.Register(owned, CostCentreListPath, gateway, unitRule(false, false, 200), authorization, readPolicy, RootFacts{roots}, func(ctx context.Context, p orgapi.Principal, decision access.Decision, input costCentreListRequest) (any, error) {
		actor, err := companyActor(ctx)
		if err != nil {
			return nil, err
		}
		page, err := input.Page.Normalize()
		if err != nil {
			return nil, orgapi.Malformed
		}
		filter := app.CostCentreFilter{Limit: page.Limit, State: input.State}
		if filter.Validate() != nil {
			return nil, orgapi.Malformed
		}
		filters := fmt.Sprintf("cost_centres.v1:%s:%d", input.State, page.Limit)
		filter.AfterID, err = gateway.OpenCursor(p, CostCentreListPath, filters, page.Cursor)
		if err != nil {
			return nil, orgapi.Malformed
		}
		values, err := service.List(ctx, p.Scope, actor, decision, filter)
		if err != nil {
			return nil, costCentreMapped(err)
		}
		items := make([]map[string]any, 0, len(values.Centres))
		for _, c := range values.Centres {
			value, err := costCentreView(p.Scope, c)
			if err != nil {
				return nil, err
			}
			items = append(items, value)
		}
		next := ""
		if values.NextID != "" {
			next, err = gateway.SealCursor(p, CostCentreListPath, filters, values.NextID)
			if err != nil {
				return nil, orgapi.Unavailable
			}
		}
		return map[string]any{"items": items, "next_cursor": next}, nil
	})
}
