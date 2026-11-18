package http

import (
	"context"
	"errors"

	company "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/adapters/http"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	guarded "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/adapters/http"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permissions "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	organization "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters/http"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/tracking/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/tracking/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/go-chi/chi/v5"
)

const (
	RoutePlanReadPath        = "/v1/orgs/{org_id}/routes/plans/read"
	RoutePlanSavePath        = "/v1/orgs/{org_id}/routes/plans/save"
	RouteSuggestionsEvalPath = "/v1/orgs/{org_id}/routes/suggestions/evaluate"
)

type RoutePlanRecords struct{}

func (RoutePlanRecords) Check(_ context.Context, scope kernel.OrgScope, subject access.Subject, policy access.Policy, facts permissions.ResourceFacts, rules []permissions.Rule) error {
	if scope.Validate() != nil || subject.Scope() != scope || facts.Validate(scope) != nil || facts.OrgID != scope.OrgID() || len(rules) == 0 {
		return access.ErrUnavailable
	}
	if policy == app.RoutePlanPolicy(false) || policy == app.RoutePlanPolicy(true) {
		return nil
	}
	return access.ErrUnavailable
}

type routeOrgResource struct{}

func (routeOrgResource) Resolve(_ context.Context, scope kernel.OrgScope, _ access.Subject) (permissions.ResourceFacts, error) {
	if scope.Validate() != nil {
		return permissions.ResourceFacts{}, access.ErrUnavailable
	}
	return permissions.ResourceFacts{OrgID: scope.OrgID(), ID: scope.OrgID().String()}, nil
}

type routePlanReadRequest struct {
	OwnerKind string `json:"owner_kind"`
	OwnerID   string `json:"owner_id"`
}

func (routePlanReadRequest) Credentials() orgapi.Session { return orgapi.Session{} }

func (v *routePlanReadRequest) UnmarshalJSON(raw []byte) error {
	type plain routePlanReadRequest
	var in plain
	if err := strictObject(raw, &in); err != nil {
		return err
	}
	*v = routePlanReadRequest(in)
	return nil
}

type routeOptionsDTO struct {
	AvoidToll     bool `json:"avoid_toll"`
	AvoidMotorway bool `json:"avoid_motorway"`
	AvoidFerry    bool `json:"avoid_ferry"`
	AvoidUnpaved  bool `json:"avoid_unpaved"`
}

type routeComputationInputDTO struct {
	EngineID        string             `json:"engine_id"`
	MapDataVersion  string             `json:"map_data_version"`
	Profile         string             `json:"profile"`
	Kind            string             `json:"kind"`
	DistanceMetres  int64              `json:"distance_m"`
	DurationSeconds int64              `json:"duration_seconds"`
	Line            []geofencePointDTO `json:"line"`
	Attribution     string             `json:"attribution"`
	ComputedAt      string             `json:"computed_at"`
}

type routePlanSaveRequest struct {
	SchemaVersion         int                       `json:"schema_version"`
	OperationID           string                    `json:"operation_id"`
	Revision              *int64                    `json:"revision"`
	OwnerKind             string                    `json:"owner_kind"`
	OwnerID               string                    `json:"owner_id"`
	Waypoints             []domain.RouteWaypoint    `json:"waypoints"`
	Options               routeOptionsDTO           `json:"options"`
	PlannedDistanceMetres *int64                    `json:"planned_distance_m"`
	PlannedDistanceSource *string                   `json:"planned_distance_source"`
	HeavyAssetNotice      bool                      `json:"heavy_asset_notice"`
	Computation           *routeComputationInputDTO `json:"computation"`
}

func (routePlanSaveRequest) Credentials() orgapi.Session { return orgapi.Session{} }
func (v routePlanSaveRequest) CommandFields() orgapi.Command {
	return orgapi.Command{SchemaVersion: v.SchemaVersion, OperationID: v.OperationID, Revision: v.Revision}
}

func (v *routePlanSaveRequest) UnmarshalJSON(raw []byte) error {
	type plain routePlanSaveRequest
	var in plain
	if err := strictObject(raw, &in); err != nil {
		return err
	}
	*v = routePlanSaveRequest(in)
	return nil
}

type routePriceRefDTO struct {
	SourceKind       string `json:"source_kind"`
	Product          string `json:"product"`
	Unit             string `json:"unit"`
	PaymentCondition string `json:"payment_condition"`
	PriceMilliBRL    int64  `json:"price_milli_brl"`
	AgeSeconds       int64  `json:"age_seconds"`
}

type routeCandidateDTO struct {
	StationID          string            `json:"station_id"`
	StationName        string            `json:"station_name"`
	Price              *routePriceRefDTO `json:"price"`
	RouteTotalMetres   int64             `json:"route_total_metres"`
	RouteTotalSeconds  int64             `json:"route_total_seconds"`
	HasPaidPlan        bool              `json:"has_paid_plan"`
	HasActiveAgreement bool              `json:"has_active_agreement"`
}

type routeSuggestionsEvalRequest struct {
	OwnerKind             string              `json:"owner_kind"`
	OwnerID               string              `json:"owner_id"`
	Product               string              `json:"product"`
	PlannedVolumeMilliL   int64               `json:"planned_volume_ml"`
	BaseDistanceMetres    int64               `json:"base_distance_m"`
	BaseDurationSeconds   int64               `json:"base_duration_seconds"`
	ConsumptionMetres     *int64              `json:"consumption_m"`
	ConsumptionMilliL     *int64              `json:"consumption_ml"`
	FreshnessLimitSeconds int64               `json:"freshness_limit_seconds"`
	Reference             *routePriceRefDTO   `json:"reference"`
	Candidates            []routeCandidateDTO `json:"candidates"`
}

func (routeSuggestionsEvalRequest) Credentials() orgapi.Session { return orgapi.Session{} }

func (v *routeSuggestionsEvalRequest) UnmarshalJSON(raw []byte) error {
	type plain routeSuggestionsEvalRequest
	var in plain
	if err := strictObject(raw, &in); err != nil {
		return err
	}
	*v = routeSuggestionsEvalRequest(in)
	return nil
}

func routeMapped(err error) error {
	for _, pair := range []struct {
		source error
		code   orgapi.Code
	}{
		{domain.ErrRoutePointLimit, orgapi.RoutePointLimit},
		{domain.ErrRouteValidation, orgapi.ValidationFailed},
		{domain.ErrRouteProfileUnsupported, orgapi.ValidationFailed},
		{domain.ErrInvalid, orgapi.ValidationFailed},
		{app.ErrRouteConflict, orgapi.Conflict},
		{app.ErrRouteOperationReused, orgapi.OperationReused},
		{audit.ErrReused, orgapi.OperationReused},
	} {
		if errors.Is(err, pair.source) {
			return pair.code
		}
	}
	return trackMapped(err)
}

func projectRoutePlan(p domain.RoutePlan) map[string]any {
	var dist, src any
	if p.PlannedDistanceMetres != nil {
		dist = *p.PlannedDistanceMetres
	}
	if p.PlannedDistanceSource != nil {
		src = string(*p.PlannedDistanceSource)
	}
	return map[string]any{
		"id": p.ID, "owner_kind": string(p.OwnerKind), "owner_id": p.OwnerID, "revision": p.Revision,
		"waypoints": p.Waypoints,
		"options": map[string]any{
			"avoid_toll": p.Options.AvoidToll, "avoid_motorway": p.Options.AvoidMotorway,
			"avoid_ferry": p.Options.AvoidFerry, "avoid_unpaved": p.Options.AvoidUnpaved,
		},
		"planned_distance_m": dist, "planned_distance_source": src,
		"computation_id": nullable(p.ComputationID), "heavy_asset_notice": p.HeavyAssetNotice,
		"created_at": orgtime.FormatInstant(p.CreatedAt),
	}
}

func projectComputation(c domain.RouteComputation) map[string]any {
	line := make([]map[string]any, 0, len(c.Line))
	for _, pt := range c.Line {
		line = append(line, map[string]any{"lat_e7": pt.Lat, "lon_e7": pt.Lon})
	}
	return map[string]any{
		"id": c.ID, "owner_kind": string(c.OwnerKind), "owner_id": c.OwnerID, "plan_revision": c.PlanRevision,
		"engine_id": c.EngineID, "map_data_version": c.MapDataVersion, "profile": string(c.Profile), "kind": string(c.Kind),
		"distance_m": c.DistanceMetres, "duration_seconds": c.DurationSeconds, "line": line,
		"attribution": c.Attribution, "computed_at": orgtime.FormatInstant(c.ComputedAt),
	}
}

// RegisterRoutes mounts EMP-ROUTE-002/003/008 route plan & suggestion endpoints.
func RegisterRoutes(router chi.Router, service app.RoutePlanService, cookies company.Handler, gateway orgapi.Gateway, authorization access.Authorization) error {
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
	readPolicy := app.RoutePlanPolicy(false)
	readRule := orgapi.Rule{Permission: app.RoutePlanPermission, Write: false, Fresh: readPolicy.Fresh, CompanyOnly: true, Success: 200}

	if err := guarded.Register(owned, RoutePlanReadPath, gateway, readRule, authorization, readPolicy, routeOrgResource{}, func(ctx context.Context, p orgapi.Principal, decision access.Decision, in routePlanReadRequest) (any, error) {
		actor, err := organization.CurrentCompanyActor(ctx)
		if err != nil {
			return nil, err
		}
		ra, err := app.NewRouteActor(actor.AccountID, actor.SessionID)
		if err != nil {
			return nil, routeMapped(err)
		}
		req := app.RoutePlanReadRequest{OwnerKind: domain.RouteOwnerKind(in.OwnerKind), OwnerID: in.OwnerID}
		view, err := service.ReadPlan(ctx, p.Scope, ra, decision, req)
		if err != nil {
			return nil, routeMapped(err)
		}
		var planObj, latestComp any
		if view.Plan != nil {
			planObj = projectRoutePlan(*view.Plan)
		}
		if view.LatestComputation != nil {
			latestComp = projectComputation(*view.LatestComputation)
		}
		hist := make([]map[string]any, 0, len(view.History))
		for _, h := range view.History {
			hist = append(hist, projectRoutePlan(h))
		}
		comps := make([]map[string]any, 0, len(view.Computations))
		for _, c := range view.Computations {
			comps = append(comps, projectComputation(c))
		}
		return map[string]any{
			"owner_kind": string(view.OwnerKind), "owner_id": view.OwnerID, "unit_id": nullable(view.UnitID),
			"plan": planObj, "latest_computation": latestComp, "history": hist, "computations": comps, "data_mode": "SYNTHETIC",
		}, nil
	}); err != nil {
		return err
	}

	writePolicy := app.RoutePlanPolicy(true)
	writeRule := orgapi.Rule{Permission: app.RoutePlanPermission, Write: true, Fresh: writePolicy.Fresh, CompanyOnly: true, Success: 200}
	if err := guarded.Register(owned, RoutePlanSavePath, gateway, writeRule, authorization, writePolicy, routeOrgResource{}, func(ctx context.Context, p orgapi.Principal, decision access.Decision, in routePlanSaveRequest) (any, error) {
		actor, err := organization.CurrentCompanyActor(ctx)
		if err != nil {
			return nil, err
		}
		ra, err := app.NewRouteActor(actor.AccountID, actor.SessionID)
		if err != nil {
			return nil, routeMapped(err)
		}
		var pSrc *domain.PlannedDistanceSource
		if in.PlannedDistanceSource != nil {
			s := domain.PlannedDistanceSource(*in.PlannedDistanceSource)
			pSrc = &s
		}
		var comp *app.RouteComputationInput
		if in.Computation != nil {
			at, err := orgtime.ParseInstant(in.Computation.ComputedAt)
			if err != nil {
				return nil, orgapi.ValidationFailed
			}
			pts := make([]domain.Point, 0, len(in.Computation.Line))
			for _, l := range in.Computation.Line {
				pts = append(pts, domain.Point{Lat: l.LatE7, Lon: l.LonE7})
			}
			comp = &app.RouteComputationInput{
				EngineID: in.Computation.EngineID, MapDataVersion: in.Computation.MapDataVersion,
				Profile: domain.RouteProfile(in.Computation.Profile), Kind: domain.RouteComputationKind(in.Computation.Kind),
				DistanceMetres: in.Computation.DistanceMetres, DurationSeconds: in.Computation.DurationSeconds,
				Line: pts, Attribution: in.Computation.Attribution, ComputedAt: at,
			}
		}
		cmd := app.RoutePlanSaveCommand{
			OperationID: in.OperationID, ExpectedRevision: in.Revision,
			OwnerKind: domain.RouteOwnerKind(in.OwnerKind), OwnerID: in.OwnerID,
			Waypoints: in.Waypoints,
			Options: domain.RouteOptions{
				AvoidToll: in.Options.AvoidToll, AvoidMotorway: in.Options.AvoidMotorway,
				AvoidFerry: in.Options.AvoidFerry, AvoidUnpaved: in.Options.AvoidUnpaved,
			},
			PlannedDistanceMetres: in.PlannedDistanceMetres, PlannedDistanceSource: pSrc,
			HeavyAssetNotice: in.HeavyAssetNotice, Computation: comp,
		}
		ack, err := service.SavePlan(ctx, p.Scope, ra, decision, cmd)
		if err != nil {
			return nil, routeMapped(err)
		}
		var compOut any
		if ack.Computation != nil {
			compOut = projectComputation(*ack.Computation)
		}
		return map[string]any{"plan": projectRoutePlan(ack.Plan), "computation": compOut, "replayed": ack.Replayed, "data_mode": "SYNTHETIC"}, nil
	}); err != nil {
		return err
	}

	return guarded.Register(owned, RouteSuggestionsEvalPath, gateway, readRule, authorization, readPolicy, routeOrgResource{}, func(ctx context.Context, p orgapi.Principal, decision access.Decision, in routeSuggestionsEvalRequest) (any, error) {
		actor, err := organization.CurrentCompanyActor(ctx)
		if err != nil {
			return nil, err
		}
		ra, err := app.NewRouteActor(actor.AccountID, actor.SessionID)
		if err != nil {
			return nil, routeMapped(err)
		}
		toRef := func(dto *routePriceRefDTO) *domain.FuelPriceReference {
			if dto == nil {
				return nil
			}
			return &domain.FuelPriceReference{
				SourceKind:       domain.PriceSourceKind(dto.SourceKind),
				Product:          dto.Product,
				Unit:             dto.Unit,
				PaymentCondition: dto.PaymentCondition,
				PriceMilliBRL:    dto.PriceMilliBRL,
				AgeSeconds:       dto.AgeSeconds,
			}
		}
		var consumption *domain.AssetConsumptionRef
		if in.ConsumptionMetres != nil && in.ConsumptionMilliL != nil {
			consumption = &domain.AssetConsumptionRef{DistanceMetres: *in.ConsumptionMetres, VolumeMilli: *in.ConsumptionMilliL}
		}
		freshnessLimit := in.FreshnessLimitSeconds
		if freshnessLimit == 0 {
			freshnessLimit = 86400
		}
		cands := make([]domain.StationCandidate, 0, len(in.Candidates))
		for _, c := range in.Candidates {
			cands = append(cands, domain.StationCandidate{
				StationID:          c.StationID,
				StationName:        c.StationName,
				Price:              toRef(c.Price),
				RouteTotalMetres:   c.RouteTotalMetres,
				RouteTotalSeconds:  c.RouteTotalSeconds,
				HasPaidPlan:        c.HasPaidPlan,
				HasActiveAgreement: c.HasActiveAgreement,
			})
		}
		res, err := service.EvaluateSuggestions(ctx, p.Scope, ra, decision, app.RouteSuggestionsRequest{
			OwnerKind:             domain.RouteOwnerKind(in.OwnerKind),
			OwnerID:               in.OwnerID,
			Product:               in.Product,
			PlannedVolumeMilliL:   in.PlannedVolumeMilliL,
			BaseDistanceMetres:    in.BaseDistanceMetres,
			BaseDurationSec:       in.BaseDurationSeconds,
			Consumption:           consumption,
			FreshnessLimitSeconds: freshnessLimit,
			Reference:             toRef(in.Reference),
			Candidates:            cands,
		})
		if err != nil {
			return nil, routeMapped(err)
		}
		return map[string]any{"suggestions": res, "data_mode": "SYNTHETIC"}, nil
	})
}
