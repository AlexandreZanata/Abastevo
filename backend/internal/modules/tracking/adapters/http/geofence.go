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
	GeofenceListPath   = "/v1/orgs/{org_id}/tracking/geofences/list"
	GeofenceCreatePath = "/v1/orgs/{org_id}/tracking/geofences/create"
	GeofenceChangePath = "/v1/orgs/{org_id}/tracking/geofences/{geofence_id}/change"
)

type GeofenceRecords struct{}

func (GeofenceRecords) Check(_ context.Context, scope kernel.OrgScope, subject access.Subject, policy access.Policy, facts permissions.ResourceFacts, rules []permissions.Rule) error {
	if scope.Validate() != nil || subject.Scope() != scope || facts.Validate(scope) != nil || facts.OrgID != scope.OrgID() || len(rules) == 0 {
		return access.ErrUnavailable
	}
	if policy == app.GeofencePolicy(false) || policy == app.GeofencePolicy(true) {
		return nil
	}
	return access.ErrUnavailable
}

type geofenceOrgResource struct{}

func (geofenceOrgResource) Resolve(_ context.Context, scope kernel.OrgScope, _ access.Subject) (permissions.ResourceFacts, error) {
	if scope.Validate() != nil {
		return permissions.ResourceFacts{}, access.ErrUnavailable
	}
	return permissions.ResourceFacts{OrgID: scope.OrgID(), ID: scope.OrgID().String()}, nil
}

type geofenceTargetResource struct{}

func (geofenceTargetResource) Resolve(ctx context.Context, scope kernel.OrgScope, _ access.Subject) (permissions.ResourceFacts, error) {
	if scope.Validate() != nil {
		return permissions.ResourceFacts{}, access.ErrUnavailable
	}
	raw := chi.URLParamFromCtx(ctx, "geofence_id")
	id, err := kernel.ParseOrgID(raw)
	if err != nil || id.String() != raw {
		return permissions.ResourceFacts{}, access.ErrNotFound
	}
	return permissions.ResourceFacts{OrgID: scope.OrgID(), ID: raw}, nil
}

type geofenceListRequest struct {
	UnitID          *string `json:"unit_id"`
	IncludeArchived *bool   `json:"include_archived"`
}

func (geofenceListRequest) Credentials() orgapi.Session { return orgapi.Session{} }

func (v *geofenceListRequest) UnmarshalJSON(raw []byte) error {
	type plain geofenceListRequest
	var in plain
	if err := strictObject(raw, &in); err != nil {
		return err
	}
	*v = geofenceListRequest(in)
	return nil
}

type geofencePointDTO struct {
	LatE7 int32 `json:"lat_e7"`
	LonE7 int32 `json:"lon_e7"`
}

type geofenceCreateRequest struct {
	SchemaVersion int                `json:"schema_version"`
	OperationID   string             `json:"operation_id"`
	Revision      *int64             `json:"revision"`
	Name          string             `json:"name"`
	UnitID        *string            `json:"unit_id"`
	Kind          string             `json:"kind"`
	CenterLatE7   *int32             `json:"center_lat_e7"`
	CenterLonE7   *int32             `json:"center_lon_e7"`
	RadiusMetres  *int32             `json:"radius_m"`
	Vertices      []geofencePointDTO `json:"vertices"`
	OnEnter       bool               `json:"on_enter"`
	OnExit        bool               `json:"on_exit"`
	DwellSeconds  int32              `json:"dwell_seconds"`
	Recipients    []string           `json:"recipients"`
	Enabled       bool               `json:"enabled"`
}

func (geofenceCreateRequest) Credentials() orgapi.Session { return orgapi.Session{} }
func (v geofenceCreateRequest) CommandFields() orgapi.Command {
	return orgapi.Command{SchemaVersion: v.SchemaVersion, OperationID: v.OperationID, Revision: v.Revision}
}

func (v *geofenceCreateRequest) UnmarshalJSON(raw []byte) error {
	type plain geofenceCreateRequest
	var in plain
	if err := strictObject(raw, &in); err != nil {
		return err
	}
	*v = geofenceCreateRequest(in)
	return nil
}

type geofenceChangeRequest struct {
	SchemaVersion int                `json:"schema_version"`
	OperationID   string             `json:"operation_id"`
	Revision      *int64             `json:"revision"`
	Archived      bool               `json:"archived"`
	Name          string             `json:"name"`
	UnitID        *string            `json:"unit_id"`
	Kind          string             `json:"kind"`
	CenterLatE7   *int32             `json:"center_lat_e7"`
	CenterLonE7   *int32             `json:"center_lon_e7"`
	RadiusMetres  *int32             `json:"radius_m"`
	Vertices      []geofencePointDTO `json:"vertices"`
	OnEnter       bool               `json:"on_enter"`
	OnExit        bool               `json:"on_exit"`
	DwellSeconds  int32              `json:"dwell_seconds"`
	Recipients    []string           `json:"recipients"`
	Enabled       bool               `json:"enabled"`
}

func (geofenceChangeRequest) Credentials() orgapi.Session { return orgapi.Session{} }
func (v geofenceChangeRequest) CommandFields() orgapi.Command {
	return orgapi.Command{SchemaVersion: v.SchemaVersion, OperationID: v.OperationID, Revision: v.Revision}
}

func (v *geofenceChangeRequest) UnmarshalJSON(raw []byte) error {
	type plain geofenceChangeRequest
	var in plain
	if err := strictObject(raw, &in); err != nil {
		return err
	}
	*v = geofenceChangeRequest(in)
	return nil
}

func geofenceMapped(err error) error {
	for _, pair := range []struct {
		source error
		code   orgapi.Code
	}{
		{domain.ErrGeofenceInvalidGeometry, orgapi.GeofenceInvalidGeometry},
		{domain.ErrInvalid, orgapi.ValidationFailed},
		{app.ErrGeofenceConflict, orgapi.Conflict},
		{app.ErrGeofenceOperationReused, orgapi.OperationReused},
		{audit.ErrReused, orgapi.OperationReused},
	} {
		if errors.Is(err, pair.source) {
			return pair.code
		}
	}
	return trackMapped(err)
}

func specFromCreate(in geofenceCreateRequest) domain.GeofenceSpec {
	unit := ""
	if in.UnitID != nil {
		unit = *in.UnitID
	}
	var cLat, cLon, rad int32
	if in.CenterLatE7 != nil {
		cLat = *in.CenterLatE7
	}
	if in.CenterLonE7 != nil {
		cLon = *in.CenterLonE7
	}
	if in.RadiusMetres != nil {
		rad = *in.RadiusMetres
	}
	verts := make([]domain.Point, 0, len(in.Vertices))
	for _, pt := range in.Vertices {
		verts = append(verts, domain.Point{Lat: pt.LatE7, Lon: pt.LonE7})
	}
	return domain.GeofenceSpec{
		Name: in.Name, UnitID: unit, Kind: domain.GeofenceKind(in.Kind),
		CenterLat: cLat, CenterLon: cLon, RadiusMetres: rad, Vertices: verts,
		OnEnter: in.OnEnter, OnExit: in.OnExit, DwellSeconds: in.DwellSeconds,
		Recipients: in.Recipients, Enabled: in.Enabled,
	}
}

func projectGeofence(v app.GeofenceView) map[string]any {
	verts := make([]map[string]any, 0, len(v.Spec.Vertices))
	for _, pt := range v.Spec.Vertices {
		verts = append(verts, map[string]any{"lat_e7": pt.Lat, "lon_e7": pt.Lon})
	}
	var cLat, cLon, rad any
	if v.Spec.Kind == domain.GeofenceCircle {
		cLat, cLon, rad = v.Spec.CenterLat, v.Spec.CenterLon, v.Spec.RadiusMetres
	}
	recs := v.Spec.Recipients
	if recs == nil {
		recs = []string{}
	}
	return map[string]any{
		"id": v.Spec.ID, "revision": v.Revision, "name": v.Spec.Name, "unit_id": nullable(v.Spec.UnitID),
		"kind": string(v.Spec.Kind), "center_lat_e7": cLat, "center_lon_e7": cLon, "radius_m": rad,
		"vertices": verts, "on_enter": v.Spec.OnEnter, "on_exit": v.Spec.OnExit, "dwell_seconds": v.Spec.DwellSeconds,
		"recipients": recs, "enabled": v.Spec.Enabled, "archived": v.Archived,
		"created_at": orgtime.FormatInstant(v.CreatedAt), "updated_at": orgtime.FormatInstant(v.UpdatedAt),
	}
}

// RegisterGeofences mounts EMP-GPS-013/014 geofences & alert endpoints.
func RegisterGeofences(router chi.Router, service app.GeofenceService, cookies company.Handler, gateway orgapi.Gateway, authorization access.Authorization) error {
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
	readPolicy := app.GeofencePolicy(false)
	readRule := orgapi.Rule{Permission: app.GeofencePermission, Write: false, Fresh: readPolicy.Fresh, CompanyOnly: true, Success: 200}
	if err := guarded.Register(owned, GeofenceListPath, gateway, readRule, authorization, readPolicy, geofenceOrgResource{}, func(ctx context.Context, p orgapi.Principal, decision access.Decision, in geofenceListRequest) (any, error) {
		actor, err := organization.CurrentCompanyActor(ctx)
		if err != nil {
			return nil, err
		}
		ga, err := app.NewGeofenceActor(actor.AccountID, actor.SessionID)
		if err != nil {
			return nil, geofenceMapped(err)
		}
		req := app.GeofenceListRequest{}
		if in.UnitID != nil {
			req.UnitID = *in.UnitID
		}
		if in.IncludeArchived != nil {
			req.IncludeArchived = *in.IncludeArchived
		}
		view, err := service.List(ctx, p.Scope, ga, decision, req)
		if err != nil {
			return nil, geofenceMapped(err)
		}
		fences := make([]map[string]any, 0, len(view.Geofences))
		for _, g := range view.Geofences {
			fences = append(fences, projectGeofence(g))
		}
		events := make([]map[string]any, 0, len(view.Events))
		for _, ev := range view.Events {
			events = append(events, map[string]any{
				"id": ev.ID, "geofence_id": ev.GeofenceID, "geofence_name": ev.GeofenceName,
				"trip_id": ev.TripID, "session_id": ev.SessionID, "unit_id": nullable(ev.UnitID),
				"event_kind": ev.EventKind, "fix_at": orgtime.FormatInstant(ev.FixAt),
				"detected_at": orgtime.FormatInstant(ev.DetectedAt), "late_suppressed": ev.LateSuppressed,
			})
		}
		alerts := make([]map[string]any, 0, len(view.Alerts))
		for _, a := range view.Alerts {
			alerts = append(alerts, projectAlert(a))
		}
		return map[string]any{"geofences": fences, "events": events, "alerts": alerts, "data_mode": "SYNTHETIC"}, nil
	}); err != nil {
		return err
	}

	writePolicy := app.GeofencePolicy(true)
	writeRule := orgapi.Rule{Permission: app.GeofencePermission, Write: true, Fresh: writePolicy.Fresh, CompanyOnly: true, Success: 200}
	if err := guarded.Register(owned, GeofenceCreatePath, gateway, writeRule, authorization, writePolicy, geofenceOrgResource{}, func(ctx context.Context, p orgapi.Principal, decision access.Decision, in geofenceCreateRequest) (any, error) {
		if in.Revision != nil {
			return nil, orgapi.Malformed
		}
		actor, err := organization.CurrentCompanyActor(ctx)
		if err != nil {
			return nil, err
		}
		ga, err := app.NewGeofenceActor(actor.AccountID, actor.SessionID)
		if err != nil {
			return nil, geofenceMapped(err)
		}
		ack, err := service.Create(ctx, p.Scope, ga, decision, app.GeofenceCreateCommand{OperationID: in.OperationID, Spec: specFromCreate(in)})
		if err != nil {
			return nil, geofenceMapped(err)
		}
		return map[string]any{"geofence": projectGeofence(ack.Geofence), "replayed": ack.Replayed, "data_mode": "SYNTHETIC"}, nil
	}); err != nil {
		return err
	}

	changeRule := orgapi.Rule{Permission: app.GeofencePermission, Write: true, Fresh: writePolicy.Fresh, RequireRevision: true, CompanyOnly: true, Success: 200}
	return guarded.Register(owned, GeofenceChangePath, gateway, changeRule, authorization, writePolicy, geofenceTargetResource{}, func(ctx context.Context, p orgapi.Principal, decision access.Decision, in geofenceChangeRequest) (any, error) {
		if in.Revision == nil {
			return nil, orgapi.Malformed
		}
		rawID := chi.URLParamFromCtx(ctx, "geofence_id")
		actor, err := organization.CurrentCompanyActor(ctx)
		if err != nil {
			return nil, err
		}
		ga, err := app.NewGeofenceActor(actor.AccountID, actor.SessionID)
		if err != nil {
			return nil, geofenceMapped(err)
		}
		spec := specFromCreate(geofenceCreateRequest{
			Name: in.Name, UnitID: in.UnitID, Kind: in.Kind, CenterLatE7: in.CenterLatE7, CenterLonE7: in.CenterLonE7,
			RadiusMetres: in.RadiusMetres, Vertices: in.Vertices, OnEnter: in.OnEnter, OnExit: in.OnExit,
			DwellSeconds: in.DwellSeconds, Recipients: in.Recipients, Enabled: in.Enabled,
		})
		ack, err := service.Change(ctx, p.Scope, ga, decision, app.GeofenceChangeCommand{
			OperationID: in.OperationID, GeofenceID: rawID, ExpectedRevision: *in.Revision, Archived: in.Archived, Spec: spec,
		})
		if err != nil {
			return nil, geofenceMapped(err)
		}
		return map[string]any{"geofence": projectGeofence(ack.Geofence), "replayed": ack.Replayed, "data_mode": "SYNTHETIC"}, nil
	})
}
