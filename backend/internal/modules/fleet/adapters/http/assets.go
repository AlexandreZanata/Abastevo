package http

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	company "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/adapters/http"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	fuel "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fueling/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	guarded "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/adapters/http"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permissions "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	organization "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters/http"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/go-chi/chi/v5"
)

const AssetPrefix = "/v1/orgs/{org_id}/fleet/assets"
const AssetCreatePath = AssetPrefix + "/create"
const AssetListPath = AssetPrefix + "/list"
const AssetReadPath = AssetPrefix + "/{asset_id}/read"
const AssetChangePath = AssetPrefix + "/{asset_id}/change"
const AssetDiscardPath = AssetPrefix + "/{asset_id}/discard"
const AssetActivatePath = AssetPrefix + "/{asset_id}/activate"

func (v assetFields) profile() (d.Profile, error) {
	p := d.Profile{Kind: v.Kind, Name: v.Name, FleetNumber: v.FleetNumber, Plate: v.Plate, SerialNumber: v.SerialNumber, UsageBasis: v.UsageBasis}
	for _, input := range v.Tanks {
		tank, err := d.NewTank(fuel.Catalog{}, input.Role, input.CapacityMilli, input.Products)
		if err != nil {
			return d.Profile{}, err
		}
		p.Tanks = append(p.Tanks, tank)
	}
	for _, input := range v.ExpectedConsumption {
		expected, err := d.NewExpectedConsumption(fuel.Catalog{}, input.Product, input.Basis, input.Value, input.Source, input.Date)
		if err != nil {
			return d.Profile{}, err
		}
		p.Expected = append(p.Expected, expected)
	}
	return p, nil
}

func assetMapped(err error) error {
	var incomplete *d.ActivationError
	if errors.As(err, &incomplete) {
		fields := make([]string, 0, len(incomplete.Missing()))
		for _, missing := range incomplete.Missing() {
			switch missing {
			case d.MissingUnit:
				fields = append(fields, "unit_id")
			case d.MissingTank:
				fields = append(fields, "tanks")
			case d.MissingMeter:
				fields = append(fields, "meters")
			default:
				return orgapi.Unavailable
			}
		}
		return orgapi.IncompleteAsset(fields)
	}
	var denied *permissions.PermissionDenial
	if errors.As(err, &denied) {
		return orgapi.MissingPermission(string(denied.Permission()))
	}
	if code, ok := orgapi.Registered(err); ok {
		return code
	}
	for _, pair := range []struct {
		source error
		code   orgapi.Code
	}{
		{app.ErrSurfaceDisabled, orgapi.NotFound}, {app.ErrNotFound, orgapi.NotFound}, {app.ErrUnauthenticated, orgapi.Unauthenticated},
		{app.ErrIdentifierTaken, orgapi.AssetIdentifierTaken}, {app.ErrConflict, orgapi.Conflict}, {app.ErrOperationReused, orgapi.OperationReused},
		{d.ErrInvalid, orgapi.ValidationFailed}, {d.ErrKindLocked, orgapi.Conflict},
		{access.ErrNoMembership, orgapi.NotFound}, {access.ErrNotFound, orgapi.NotFound}, {access.ErrUnauthenticated, orgapi.Unauthenticated},
		{access.ErrFreshAuthRequired, orgapi.FreshAuthRequired}, {access.ErrMembershipSuspended, orgapi.MembershipSuspended}, {access.ErrOrgSuspended, orgapi.OrgSuspended}, {access.ErrOrgArchived, orgapi.OrgArchived}, {access.ErrSetupIncomplete, orgapi.OrgSetupIncomplete}, {access.ErrEntitlementRequired, orgapi.EntitlementRequired}, {access.ErrLimitReached, orgapi.LimitReached},
	} {
		if errors.Is(err, pair.source) {
			return pair.code
		}
	}
	return orgapi.Unavailable
}

// FleetRecords accepts only this module's fixed source policies. No creator or
// assignment fact is manufactured: SELF rules remain uncovered until FLEET-006.
type FleetRecords struct{}

func (FleetRecords) Check(_ context.Context, scope kernel.OrgScope, subject access.Subject, policy access.Policy, facts permissions.ResourceFacts, rules []permissions.Rule) error {
	if scope.Validate() != nil || subject.Scope() != scope || facts.Validate(scope) != nil || facts.OrgID != scope.OrgID() || len(rules) == 0 || policy != app.Policy(false) && policy != app.Policy(true) {
		return access.ErrUnavailable
	}
	return nil
}

type assetResource struct{ root bool }

func (resource assetResource) Resolve(ctx context.Context, scope kernel.OrgScope, _ access.Subject) (permissions.ResourceFacts, error) {
	if scope.Validate() != nil {
		return permissions.ResourceFacts{}, access.ErrUnavailable
	}
	id := scope.OrgID().String()
	if !resource.root {
		var err error
		id, err = assetPathID(ctx)
		if err != nil {
			return permissions.ResourceFacts{}, access.ErrNotFound
		}
	}
	return permissions.ResourceFacts{OrgID: scope.OrgID(), ID: id}, nil
}
func assetPathID(ctx context.Context) (string, error) {
	raw := chi.URLParamFromCtx(ctx, "asset_id")
	id, err := d.ParseAssetID(raw)
	if err != nil {
		return "", access.ErrNotFound
	}
	return id.Value(), nil
}
func assetRule(write, revision bool, status int) orgapi.Rule {
	return orgapi.Rule{Permission: string(app.Policy(write).Permission), Write: write, Fresh: write, RequireRevision: revision, CompanyOnly: true, Success: status}
}
func missing(port any) bool {
	if port == nil {
		return true
	}
	v := reflect.ValueOf(port)
	switch v.Kind() {
	case reflect.Pointer, reflect.Func, reflect.Interface, reflect.Map, reflect.Slice, reflect.Chan:
		return v.IsNil()
	}
	return false
}
func currentActor(ctx context.Context) (app.FleetActor, error) {
	actor, err := organization.CurrentCompanyActor(ctx)
	if err != nil {
		return app.FleetActor{}, err
	}
	return app.FleetActor{AccountID: actor.AccountID, SessionID: actor.SessionID}, nil
}
func assetAck(result app.AssetResult, replayed bool) (map[string]any, error) {
	if _, err := d.ParseAssetID(result.AssetID); err != nil || result.Revision < 1 {
		return nil, orgapi.Unavailable
	}
	out := map[string]any{"asset_id": result.AssetID, "revision": result.Revision, "discarded": result.Discarded, "replayed": replayed, "data_mode": "SYNTHETIC"}
	if result.Discarded {
		if result.State != "" {
			return nil, orgapi.Unavailable
		}
	} else {
		if _, err := d.ParseState(string(result.State)); err != nil {
			return nil, orgapi.Unavailable
		}
		out["state"] = result.State
	}
	return out, nil
}
func validLabel(raw string) bool {
	if !utf8.ValidString(raw) || strings.TrimSpace(raw) == "" {
		return false
	}
	for _, r := range raw {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}
func assetView(scope kernel.OrgScope, value app.AssetView) (map[string]any, error) {
	if value.Ref.Validate(scope) != nil || value.Revision < 1 || !validLabel(value.CategoryLabel) {
		return nil, orgapi.Unavailable
	}
	if _, err := d.ParseState(string(value.State)); err != nil {
		return nil, orgapi.Unavailable
	}
	category, err := kernel.ParseOrgID(value.Profile.Category)
	if err != nil || category.String() != value.Profile.Category {
		return nil, orgapi.Unavailable
	}
	if value.UnitID != "" {
		unit, err := kernel.ParseOrgID(value.UnitID)
		if err != nil || unit.String() != value.UnitID {
			return nil, orgapi.Unavailable
		}
	}
	profile := value.Profile
	if profile.Kind != d.Vehicle && profile.Kind != d.Machine && profile.Kind != d.Implement {
		return nil, orgapi.Unavailable
	}
	if value.State == d.Active && (value.UnitID == "" || value.Allocation == nil) {
		return nil, orgapi.Unavailable
	}
	tanks := make([]map[string]any, 0, len(profile.Tanks))
	for _, tank := range profile.Tanks {
		v := tank.Snapshot()
		tanks = append(tanks, map[string]any{"role": v.Role, "capacity_milli": strconv.FormatInt(v.Capacity, 10), "unit": v.Unit, "products": v.Products})
	}
	expected := make([]map[string]any, 0, len(profile.Expected))
	for _, reference := range profile.Expected {
		v := reference.Snapshot()
		expected = append(expected, map[string]any{"product": v.Product, "basis": v.Basis, "value": strconv.FormatInt(v.Value, 10), "unit": v.Unit, "source": v.Source, "date": v.Date})
	}
	out := map[string]any{"id": value.Ref.ID().Value(), "kind": profile.Kind, "name": profile.Name, "fleet_number": profile.FleetNumber, "plate": profile.Plate, "serial_number": profile.SerialNumber, "usage_basis": profile.UsageBasis, "category": map[string]any{"id": profile.Category, "label": value.CategoryLabel}, "tanks": tanks, "expected_consumption": expected, "state": value.State, "revision": value.Revision, "unit_id": value.UnitID, "data_mode": "SYNTHETIC"}
	if value.Allocation != nil {
		allocation := value.Allocation
		stamp := orgtime.FormatInstant(allocation.StartedAt)
		if allocation.UnitID != value.UnitID || allocation.StartedAt.IsZero() {
			return nil, orgapi.Unavailable
		}
		if _, err := orgtime.ParseInstant(stamp); err != nil {
			return nil, orgapi.Unavailable
		}
		out["initial_allocation"] = map[string]any{"unit_id": allocation.UnitID, "started_at": stamp}
	}
	return out, nil
}

func RegisterAssets(router chi.Router, service app.AssetService, cookies company.Handler, gateway orgapi.Gateway, authorization access.Authorization) error {
	if !service.Enabled {
		return nil
	}
	if router == nil || missing(service.Repository) || service.NewID == nil || missing(authorization.Source) || missing(authorization.Capabilities) || missing(authorization.Records) || authorization.Now == nil {
		return app.ErrUnavailable
	}
	owned, gateway, err := organization.CompanyEntry(router, cookies, gateway)
	if err != nil {
		return err
	}
	root, resource := assetResource{root: true}, assetResource{}
	writePolicy, readPolicy := app.Policy(true), app.Policy(false)
	if err := guarded.Register(owned, AssetCreatePath, gateway, assetRule(true, false, 201), authorization, writePolicy, root, func(ctx context.Context, p orgapi.Principal, decision access.Decision, input assetCreateRequest) (any, error) {
		actor, err := currentActor(ctx)
		if err != nil {
			return nil, err
		}
		profile, err := input.assetFields.profile()
		if err != nil {
			return nil, assetMapped(err)
		}
		command, err := app.NewAssetWrite(app.AssetWriteInput{Kind: app.CreateAsset, OperationID: input.OperationID, Profile: profile, Category: app.CategoryChoice{ExistingID: input.Category.ExistingID, NewLabel: input.Category.NewLabel}, UnitID: input.UnitID})
		if err != nil {
			return nil, assetMapped(err)
		}
		result, replayed, err := service.Write(ctx, p.Scope, actor, decision, command)
		if err != nil {
			return nil, assetMapped(err)
		}
		return assetAck(result, replayed)
	}); err != nil {
		return err
	}
	if err := guarded.Register(owned, AssetChangePath, gateway, assetRule(true, true, 200), authorization, writePolicy, resource, func(ctx context.Context, p orgapi.Principal, decision access.Decision, input assetChangeRequest) (any, error) {
		actor, err := currentActor(ctx)
		if err != nil {
			return nil, err
		}
		id, err := assetPathID(ctx)
		if err != nil {
			return nil, orgapi.NotFound
		}
		profile, err := input.assetFields.profile()
		if err != nil {
			return nil, assetMapped(err)
		}
		command, err := app.NewAssetWrite(app.AssetWriteInput{Kind: app.ChangeAsset, OperationID: input.OperationID, AssetID: id, ExpectedRevision: *input.Revision, Profile: profile, Category: app.CategoryChoice{ExistingID: input.Category.ExistingID, NewLabel: input.Category.NewLabel}, UnitID: input.UnitID})
		if err != nil {
			return nil, assetMapped(err)
		}
		result, replayed, err := service.Write(ctx, p.Scope, actor, decision, command)
		if err != nil {
			return nil, assetMapped(err)
		}
		return assetAck(result, replayed)
	}); err != nil {
		return err
	}
	for _, transition := range []struct {
		path string
		kind app.AssetWriteKind
	}{{AssetDiscardPath, app.DiscardAsset}, {AssetActivatePath, app.ActivateAsset}} {
		if err := guarded.Register(owned, transition.path, gateway, assetRule(true, true, 200), authorization, writePolicy, resource, func(ctx context.Context, p orgapi.Principal, decision access.Decision, input assetTransitionRequest) (any, error) {
			actor, err := currentActor(ctx)
			if err != nil {
				return nil, err
			}
			id, err := assetPathID(ctx)
			if err != nil {
				return nil, orgapi.NotFound
			}
			command, err := app.NewAssetWrite(app.AssetWriteInput{Kind: transition.kind, OperationID: input.OperationID, AssetID: id, ExpectedRevision: *input.Revision})
			if err != nil {
				return nil, assetMapped(err)
			}
			result, replayed, err := service.Write(ctx, p.Scope, actor, decision, command)
			if err != nil {
				return nil, assetMapped(err)
			}
			return assetAck(result, replayed)
		}); err != nil {
			return err
		}
	}
	if err := guarded.Register(owned, AssetReadPath, gateway, assetRule(false, false, 200), authorization, readPolicy, resource, func(ctx context.Context, p orgapi.Principal, decision access.Decision, _ assetReadRequest) (any, error) {
		actor, err := currentActor(ctx)
		if err != nil {
			return nil, err
		}
		id, err := assetPathID(ctx)
		if err != nil {
			return nil, orgapi.NotFound
		}
		value, err := service.Read(ctx, p.Scope, actor, decision, id)
		if err != nil {
			return nil, assetMapped(err)
		}
		return assetView(p.Scope, value)
	}); err != nil {
		return err
	}
	return guarded.Register(owned, AssetListPath, gateway, assetRule(false, false, 200), authorization, readPolicy, root, func(ctx context.Context, p orgapi.Principal, decision access.Decision, input assetListRequest) (any, error) {
		actor, err := currentActor(ctx)
		if err != nil {
			return nil, err
		}
		page, err := input.Page.Normalize()
		if err != nil {
			return nil, orgapi.Malformed
		}
		filter := app.AssetFilter{Limit: page.Limit, State: input.State, Kind: input.Kind}
		if filter.Validate() != nil {
			return nil, orgapi.Malformed
		}
		filters := fmt.Sprintf("fleet.assets.v1:%s:%s:%d", input.State, input.Kind, page.Limit)
		after, err := gateway.OpenCursor(p, AssetListPath, filters, page.Cursor)
		if err != nil {
			return nil, orgapi.Malformed
		}
		filter.AfterID = after
		values, err := service.List(ctx, p.Scope, actor, decision, filter)
		if err != nil {
			return nil, assetMapped(err)
		}
		items := make([]map[string]any, 0, len(values.Assets))
		for _, value := range values.Assets {
			item, err := assetView(p.Scope, value)
			if err != nil {
				return nil, err
			}
			items = append(items, item)
		}
		next := ""
		if values.NextID != "" {
			next, err = gateway.SealCursor(p, AssetListPath, filters, values.NextID)
			if err != nil {
				return nil, orgapi.Unavailable
			}
		}
		return map[string]any{"items": items, "next_cursor": next}, nil
	})
}
