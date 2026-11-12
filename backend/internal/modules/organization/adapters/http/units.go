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

const UnitPrefix = "/v1/orgs/{org_id}/units"
const UnitCreatePath = UnitPrefix + "/create"
const UnitListPath = UnitPrefix + "/list"
const UnitReadPath = UnitPrefix + "/{unit_id}/read"
const UnitChangePath = UnitPrefix + "/{unit_id}/change"
const UnitArchivePath = UnitPrefix + "/{unit_id}/archive"
const UnitRestorePath = UnitPrefix + "/{unit_id}/restore"

type companyRequest struct{ privateRequest }

func (companyRequest) Credentials() orgapi.Session { return orgapi.Session{} }

type unitFields struct {
	privateRequest
	Name             string `json:"name"`
	Code             string `json:"code"`
	TimeZone         string `json:"time_zone,omitempty"`
	EstablishmentID  string `json:"establishment_id,omitempty"`
	MunicipalityCode string `json:"municipality_code,omitempty"`
}

func (v unitFields) domain() d.UnitProfile {
	return d.UnitProfile{Name: v.Name, Code: v.Code, TimeZone: v.TimeZone, EstablishmentID: v.EstablishmentID, MunicipalityCode: v.MunicipalityCode}
}

type unitCreateRequest struct {
	privateRequest
	companyRequest
	SchemaVersion int    `json:"schema_version"`
	OperationID   string `json:"operation_id"`
	unitFields
}

func (v unitCreateRequest) CommandFields() orgapi.Command {
	return orgapi.Command{SchemaVersion: v.SchemaVersion, OperationID: v.OperationID}
}

type unitRevisionCommand struct {
	companyRequest
	SchemaVersion int    `json:"schema_version"`
	OperationID   string `json:"operation_id"`
	Revision      *int64 `json:"revision"`
}

func (v unitRevisionCommand) CommandFields() orgapi.Command {
	return orgapi.Command{SchemaVersion: v.SchemaVersion, OperationID: v.OperationID, Revision: v.Revision}
}

type unitChangeRequest struct {
	privateRequest
	unitRevisionCommand
	unitFields
}
type unitTransitionRequest struct{ unitRevisionCommand }
type unitReadRequest struct{ companyRequest }
type unitListRequest struct {
	companyRequest
	orgapi.Page
	State d.UnitState `json:"state,omitempty"`
}

func (v *unitCreateRequest) UnmarshalJSON(raw []byte) error {
	type plain unitCreateRequest
	var input plain
	if err := decodeLegal(raw, &input); err != nil {
		return err
	}
	*v = unitCreateRequest(input)
	return nil
}
func (v *unitChangeRequest) UnmarshalJSON(raw []byte) error {
	type plain unitChangeRequest
	var input plain
	if err := decodeLegal(raw, &input); err != nil {
		return err
	}
	*v = unitChangeRequest(input)
	return nil
}
func (v *unitTransitionRequest) UnmarshalJSON(raw []byte) error {
	type plain unitTransitionRequest
	var input plain
	if err := decodeLegal(raw, &input); err != nil {
		return err
	}
	*v = unitTransitionRequest(input)
	return nil
}
func (v *unitReadRequest) UnmarshalJSON(raw []byte) error {
	type plain unitReadRequest
	var input plain
	if err := decodeLegal(raw, &input); err != nil {
		return err
	}
	*v = unitReadRequest(input)
	return nil
}
func (v *unitListRequest) UnmarshalJSON(raw []byte) error {
	type plain unitListRequest
	var input plain
	if err := decodeLegal(raw, &input); err != nil {
		return err
	}
	*v = unitListRequest(input)
	return nil
}

type StructureRecords struct{}

// UnitRecords retains the completed unit route contract.
type UnitRecords = StructureRecords

func (StructureRecords) Check(_ context.Context, scope kernel.OrgScope, subject access.Subject, policy access.Policy, facts permissions.ResourceFacts, rules []permissions.Rule) error {
	if scope.Validate() != nil || subject.Scope() != scope || facts.Validate(scope) != nil || facts.OrgID != scope.OrgID() || len(rules) == 0 || policy != app.UnitPolicy(false) && policy != app.UnitPolicy(true) {
		return access.ErrUnavailable
	}
	return nil
}

type unitResource struct{ lookup app.UnitLookup }

func (r unitResource) Resolve(ctx context.Context, scope kernel.OrgScope, _ access.Subject) (permissions.ResourceFacts, error) {
	if r.lookup == nil {
		return permissions.ResourceFacts{}, access.ErrUnavailable
	}
	id, err := legalPathID(ctx, "unit_id")
	if err != nil {
		return permissions.ResourceFacts{}, err
	}
	unit, err := r.lookup.FindUnit(ctx, scope, id)
	if errors.Is(err, app.ErrNotFound) {
		return permissions.ResourceFacts{}, access.ErrNotFound
	}
	if err != nil {
		return permissions.ResourceFacts{}, access.ErrUnavailable
	}
	if _, err := unit.Snapshot(scope); err != nil {
		return permissions.ResourceFacts{}, access.ErrUnavailable
	}
	return permissions.ResourceFacts{OrgID: scope.OrgID(), ID: id}, nil
}
func unitRule(write, revision bool, status int) orgapi.Rule {
	return orgapi.Rule{Permission: "org.structure.manage", Write: write, Fresh: write, RequireRevision: revision, CompanyOnly: true, Success: status}
}
func unitMapped(err error) error {
	var duplicate app.UnitDuplicateError
	if errors.As(err, &duplicate) {
		return orgapi.UnitDuplicate(duplicate.Field())
	}
	var inUse app.UnitUsageError
	if errors.As(err, &inUse) {
		v := inUse.Usage()
		return orgapi.StructureInUseCounts(orgapi.StructureCounts{Assets: v.Assets, Employees: v.Employees, Budgets: v.Budgets, Operations: v.Operations})
	}
	if errors.Is(err, d.ErrLastUnit) {
		return orgapi.LastUnit
	}
	if errors.Is(err, d.ErrStructureInUse) {
		// A bare source refusal cannot supply the required current category counts.
		return orgapi.Unavailable
	}
	return legalMapped(err, "identifier")
}
func unitAck(value app.UnitResult, replay bool) map[string]any {
	return map[string]any{"unit_id": value.UnitID, "state": value.State, "revision": value.Revision, "replayed": replay, "data_mode": "SYNTHETIC"}
}
func unitView(scope kernel.OrgScope, unit d.Unit) (map[string]any, error) {
	value, err := unit.Snapshot(scope)
	if err != nil {
		return nil, orgapi.Unavailable
	}
	display := value.Name
	if value.State == d.UnitArchived {
		display += " (arquivada)"
	}
	result := map[string]any{"id": value.ID, "name": value.Name, "display_name": display, "code": value.Code, "time_zone": value.TimeZone, "municipality_code": value.MunicipalityCode, "state": value.State, "revision": value.Revision, "data_mode": "SYNTHETIC"}
	if value.EstablishmentID != "" {
		result["establishment_id"] = value.EstablishmentID
	}
	return result, nil
}
func RegisterUnits(router chi.Router, service app.UnitService, cookies company.Handler, gateway orgapi.Gateway, authorization access.Authorization, roots app.Repository, lookup app.UnitLookup) error {
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
	resource := unitResource{lookup}
	writePolicy, readPolicy := app.UnitPolicy(true), app.UnitPolicy(false)
	if err := guarded.Register(owned, UnitCreatePath, gateway, unitRule(true, false, 201), authorization, writePolicy, RootFacts{roots}, func(ctx context.Context, p orgapi.Principal, decision access.Decision, input unitCreateRequest) (any, error) {
		actor, err := companyActor(ctx)
		if err != nil {
			return nil, err
		}
		command, err := app.NewUnitWrite(app.UnitWriteInput{Kind: app.CreateUnit, OperationID: input.OperationID, Profile: input.unitFields.domain()})
		if err != nil {
			return nil, unitMapped(err)
		}
		result, replay, err := service.Write(ctx, p.Scope, actor, decision, command)
		if err != nil {
			return nil, unitMapped(err)
		}
		return unitAck(result, replay), nil
	}); err != nil {
		return err
	}
	if err := guarded.Register(owned, UnitChangePath, gateway, unitRule(true, true, 200), authorization, writePolicy, resource, func(ctx context.Context, p orgapi.Principal, decision access.Decision, input unitChangeRequest) (any, error) {
		actor, err := companyActor(ctx)
		if err != nil {
			return nil, err
		}
		id, err := legalPathID(ctx, "unit_id")
		if err != nil {
			return nil, orgapi.NotFound
		}
		command, err := app.NewUnitWrite(app.UnitWriteInput{Kind: app.ChangeUnit, OperationID: input.OperationID, UnitID: id, ExpectedRevision: *input.Revision, Profile: input.unitFields.domain()})
		if err != nil {
			return nil, unitMapped(err)
		}
		result, replay, err := service.Write(ctx, p.Scope, actor, decision, command)
		if err != nil {
			return nil, unitMapped(err)
		}
		return unitAck(result, replay), nil
	}); err != nil {
		return err
	}
	for _, transition := range []struct {
		path string
		kind app.UnitWriteKind
	}{{UnitArchivePath, app.ArchiveUnit}, {UnitRestorePath, app.RestoreUnit}} {
		if err := guarded.Register(owned, transition.path, gateway, unitRule(true, true, 200), authorization, writePolicy, resource, func(ctx context.Context, p orgapi.Principal, decision access.Decision, input unitTransitionRequest) (any, error) {
			actor, err := companyActor(ctx)
			if err != nil {
				return nil, err
			}
			id, err := legalPathID(ctx, "unit_id")
			if err != nil {
				return nil, orgapi.NotFound
			}
			command, err := app.NewUnitWrite(app.UnitWriteInput{Kind: transition.kind, OperationID: input.OperationID, UnitID: id, ExpectedRevision: *input.Revision})
			if err != nil {
				return nil, unitMapped(err)
			}
			result, replay, err := service.Write(ctx, p.Scope, actor, decision, command)
			if err != nil {
				return nil, unitMapped(err)
			}
			return unitAck(result, replay), nil
		}); err != nil {
			return err
		}
	}
	if err := guarded.Register(owned, UnitReadPath, gateway, unitRule(false, false, 200), authorization, readPolicy, resource, func(ctx context.Context, p orgapi.Principal, decision access.Decision, _ unitReadRequest) (any, error) {
		actor, err := companyActor(ctx)
		if err != nil {
			return nil, err
		}
		id, err := legalPathID(ctx, "unit_id")
		if err != nil {
			return nil, orgapi.NotFound
		}
		value, err := service.Read(ctx, p.Scope, actor, decision, id)
		if err != nil {
			return nil, unitMapped(err)
		}
		return unitView(p.Scope, value)
	}); err != nil {
		return err
	}
	return guarded.Register(owned, UnitListPath, gateway, unitRule(false, false, 200), authorization, readPolicy, RootFacts{roots}, func(ctx context.Context, p orgapi.Principal, decision access.Decision, input unitListRequest) (any, error) {
		actor, err := companyActor(ctx)
		if err != nil {
			return nil, err
		}
		page, err := input.Page.Normalize()
		if err != nil {
			return nil, orgapi.Malformed
		}
		filter := app.UnitFilter{Limit: page.Limit, State: input.State}
		if filter.Validate() != nil {
			return nil, orgapi.Malformed
		}
		filters := fmt.Sprintf("units.v1:%s:%d", input.State, page.Limit)
		after, err := gateway.OpenCursor(p, UnitListPath, filters, page.Cursor)
		if err != nil {
			return nil, orgapi.Malformed
		}
		filter.AfterID = after
		values, err := service.List(ctx, p.Scope, actor, decision, filter)
		if err != nil {
			return nil, unitMapped(err)
		}
		items := make([]map[string]any, 0, len(values.Units))
		for _, unit := range values.Units {
			view, err := unitView(p.Scope, unit)
			if err != nil {
				return nil, err
			}
			items = append(items, view)
		}
		next := ""
		if values.NextID != "" {
			next, err = gateway.SealCursor(p, UnitListPath, filters, values.NextID)
			if err != nil {
				return nil, orgapi.Unavailable
			}
		}
		return map[string]any{"items": items, "next_cursor": next}, nil
	})
}
