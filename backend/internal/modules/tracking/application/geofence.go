package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/tracking/domain"
)

// Server-side geofences and operational location alerts (EMP-GPS-013,
// EMP-GPS-014, BR-GPS-037..039, VAL-30).
const GeofencePermission = "gps.geofence.manage"

var (
	ErrGeofenceConflict        = errors.New("tracking: geofence conflict")
	ErrGeofenceOperationReused = errors.New("tracking: geofence operation reused")
)

func GeofencePolicy(write bool) access.Policy {
	if write {
		return access.Policy{Permission: GeofencePermission, Capability: "organizations", Effect: access.ConfigurationWrite, Fresh: true}
	}
	return access.Policy{Permission: GeofencePermission, Capability: "organizations", Effect: access.Read}
}

type GeofenceActor struct {
	private
	AccountID, SessionID string
}

func NewGeofenceActor(accountID, sessionID string) (GeofenceActor, error) {
	a := GeofenceActor{AccountID: accountID, SessionID: sessionID}
	if a.Validate() != nil {
		return GeofenceActor{}, ErrTrackUnauthorized
	}
	return a, nil
}

func (a GeofenceActor) Validate() error {
	if !canonicalID(a.AccountID) || !canonicalID(a.SessionID) {
		return ErrTrackUnauthorized
	}
	return nil
}

type GeofenceCreateCommand struct {
	OperationID string
	Spec        domain.GeofenceSpec
}

func (c GeofenceCreateCommand) Validate() error {
	if !canonicalID(c.OperationID) {
		return ErrMalformed
	}
	return c.Spec.Validate()
}

func (c GeofenceCreateCommand) Hash() string {
	raw, _ := json.Marshal(struct {
		Kind   string         `json:"kind"`
		Name   string         `json:"name"`
		Unit   string         `json:"unit_id"`
		Shape  string         `json:"shape"`
		CLat   int32          `json:"center_lat_e7"`
		CLon   int32          `json:"center_lon_e7"`
		Radius int32          `json:"radius_m"`
		Verts  []domain.Point `json:"vertices"`
		Enter  bool           `json:"on_enter"`
		Exit   bool           `json:"on_exit"`
		Dwell  int32          `json:"dwell_seconds"`
		Recs   []string       `json:"recipients"`
		En     bool           `json:"enabled"`
	}{
		Kind: "CREATE", Name: c.Spec.Name, Unit: c.Spec.UnitID, Shape: string(c.Spec.Kind),
		CLat: c.Spec.CenterLat, CLon: c.Spec.CenterLon, Radius: c.Spec.RadiusMetres, Verts: c.Spec.Vertices,
		Enter: c.Spec.OnEnter, Exit: c.Spec.OnExit, Dwell: c.Spec.DwellSeconds, Recs: c.Spec.Recipients, En: c.Spec.Enabled,
	})
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

type GeofenceChangeCommand struct {
	OperationID      string
	GeofenceID       string
	ExpectedRevision int64
	Archived         bool
	Spec             domain.GeofenceSpec
}

func (c GeofenceChangeCommand) Validate() error {
	if !canonicalID(c.OperationID) || !canonicalID(c.GeofenceID) || c.ExpectedRevision < 1 {
		return ErrMalformed
	}
	return c.Spec.Validate()
}

func (c GeofenceChangeCommand) Hash() string {
	raw, _ := json.Marshal(struct {
		Kind   string         `json:"kind"`
		ID     string         `json:"id"`
		Rev    int64          `json:"rev"`
		Arch   bool           `json:"archived"`
		Name   string         `json:"name"`
		Unit   string         `json:"unit_id"`
		Shape  string         `json:"shape"`
		CLat   int32          `json:"center_lat_e7"`
		CLon   int32          `json:"center_lon_e7"`
		Radius int32          `json:"radius_m"`
		Verts  []domain.Point `json:"vertices"`
		Enter  bool           `json:"on_enter"`
		Exit   bool           `json:"on_exit"`
		Dwell  int32          `json:"dwell_seconds"`
		Recs   []string       `json:"recipients"`
		En     bool           `json:"enabled"`
	}{
		Kind: "CHANGE", ID: c.GeofenceID, Rev: c.ExpectedRevision, Arch: c.Archived, Name: c.Spec.Name, Unit: c.Spec.UnitID,
		Shape: string(c.Spec.Kind), CLat: c.Spec.CenterLat, CLon: c.Spec.CenterLon, Radius: c.Spec.RadiusMetres, Verts: c.Spec.Vertices,
		Enter: c.Spec.OnEnter, Exit: c.Spec.OnExit, Dwell: c.Spec.DwellSeconds, Recs: c.Spec.Recipients, En: c.Spec.Enabled,
	})
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

type GeofenceView struct {
	Spec      domain.GeofenceSpec
	Revision  int64
	Archived  bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

type GeofenceEventView struct {
	ID             string
	GeofenceID     string
	GeofenceName   string
	TripID         string
	SessionID      string
	UnitID         string
	EventKind      string
	FixAt          time.Time
	DetectedAt     time.Time
	LateSuppressed bool
}

type AlertView struct {
	ID               string
	Kind             string
	TripID           string
	SessionID        string
	UnitID           string
	GeofenceID       string
	EventKind        string
	AnchorAt         time.Time
	ObservedAt       time.Time
	DurationSeconds  int64
	ThresholdSeconds int64
	RadiusMetres     int64
	LateSuppressed   bool
}

type GeofenceListRequest struct {
	UnitID          string
	IncludeArchived bool
}

func (r GeofenceListRequest) Validate() error {
	if r.UnitID != "" && !canonicalID(r.UnitID) {
		return ErrMalformed
	}
	return nil
}

type GeofenceListView struct {
	Geofences []GeofenceView
	Events    []GeofenceEventView
	Alerts    []AlertView
}

type GeofenceAck struct {
	Geofence GeofenceView
	Replayed bool
}

type GeofenceRepository interface {
	ListGeofences(ctx context.Context, scope kernel.OrgScope, actor GeofenceActor, decision access.Decision, request GeofenceListRequest) (GeofenceListView, error)
	CreateGeofence(ctx context.Context, scope kernel.OrgScope, actor GeofenceActor, decision access.Decision, cmd GeofenceCreateCommand) (GeofenceAck, error)
	ChangeGeofence(ctx context.Context, scope kernel.OrgScope, actor GeofenceActor, decision access.Decision, cmd GeofenceChangeCommand) (GeofenceAck, error)
}

type GeofenceService struct {
	Enabled    bool
	Repository GeofenceRepository
}

func (s GeofenceService) List(ctx context.Context, scope kernel.OrgScope, actor GeofenceActor, decision access.Decision, request GeofenceListRequest) (GeofenceListView, error) {
	if !s.Enabled {
		return GeofenceListView{}, ErrTrackDisabled
	}
	if scope.Validate() != nil || request.Validate() != nil {
		return GeofenceListView{}, ErrMalformed
	}
	if actor.Validate() != nil {
		return GeofenceListView{}, ErrTrackUnauthorized
	}
	if absent(s.Repository) {
		return GeofenceListView{}, ErrUnavailable
	}
	return s.Repository.ListGeofences(ctx, scope, actor, decision, request)
}

func (s GeofenceService) Create(ctx context.Context, scope kernel.OrgScope, actor GeofenceActor, decision access.Decision, cmd GeofenceCreateCommand) (GeofenceAck, error) {
	if !s.Enabled {
		return GeofenceAck{}, ErrTrackDisabled
	}
	if scope.Validate() != nil {
		return GeofenceAck{}, ErrMalformed
	}
	if actor.Validate() != nil {
		return GeofenceAck{}, ErrTrackUnauthorized
	}
	if err := cmd.Validate(); err != nil {
		return GeofenceAck{}, err
	}
	if absent(s.Repository) {
		return GeofenceAck{}, ErrUnavailable
	}
	return s.Repository.CreateGeofence(ctx, scope, actor, decision, cmd)
}

func (s GeofenceService) Change(ctx context.Context, scope kernel.OrgScope, actor GeofenceActor, decision access.Decision, cmd GeofenceChangeCommand) (GeofenceAck, error) {
	if !s.Enabled {
		return GeofenceAck{}, ErrTrackDisabled
	}
	if scope.Validate() != nil {
		return GeofenceAck{}, ErrMalformed
	}
	if actor.Validate() != nil {
		return GeofenceAck{}, ErrTrackUnauthorized
	}
	if err := cmd.Validate(); err != nil {
		return GeofenceAck{}, err
	}
	if absent(s.Repository) {
		return GeofenceAck{}, ErrUnavailable
	}
	return s.Repository.ChangeGeofence(ctx, scope, actor, decision, cmd)
}
