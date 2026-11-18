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

// Route plans, device route computations and neutral station suggestions
// (EMP-ROUTE-002, EMP-ROUTE-003, EMP-ROUTE-008; BR-ROUTE-001..024).
const RoutePlanPermission = "route.plan.manage"

var (
	ErrRouteConflict        = errors.New("tracking: route plan conflict")
	ErrRouteOperationReused = errors.New("tracking: route plan operation reused")
)

func RoutePlanPolicy(write bool) access.Policy {
	if write {
		return access.Policy{Permission: RoutePlanPermission, Capability: "organizations", Effect: access.ConfigurationWrite, Fresh: true}
	}
	return access.Policy{Permission: RoutePlanPermission, Capability: "organizations", Effect: access.Read}
}

type RouteActor struct {
	private
	AccountID, SessionID string
}

func NewRouteActor(accountID, sessionID string) (RouteActor, error) {
	a := RouteActor{AccountID: accountID, SessionID: sessionID}
	if a.Validate() != nil {
		return RouteActor{}, ErrTrackUnauthorized
	}
	return a, nil
}

func (a RouteActor) Validate() error {
	if !canonicalID(a.AccountID) || !canonicalID(a.SessionID) {
		return ErrTrackUnauthorized
	}
	return nil
}

type RoutePlanReadRequest struct {
	OwnerKind domain.RouteOwnerKind
	OwnerID   string
}

func (r RoutePlanReadRequest) Validate() error {
	if _, err := domain.ParseRouteOwnerKind(string(r.OwnerKind)); err != nil || !canonicalID(r.OwnerID) {
		return ErrMalformed
	}
	return nil
}

type RouteComputationInput struct {
	EngineID        string
	MapDataVersion  string
	Profile         domain.RouteProfile
	Kind            domain.RouteComputationKind
	DistanceMetres  int64
	DurationSeconds int64
	Line            []domain.Point
	Attribution     string
	ComputedAt      time.Time
}

type RoutePlanSaveCommand struct {
	OperationID           string
	ExpectedRevision      *int64
	OwnerKind             domain.RouteOwnerKind
	OwnerID               string
	Waypoints             []domain.RouteWaypoint
	Options               domain.RouteOptions
	PlannedDistanceMetres *int64
	PlannedDistanceSource *domain.PlannedDistanceSource
	HeavyAssetNotice      bool
	Computation           *RouteComputationInput
}

func (c RoutePlanSaveCommand) Validate() error {
	if !canonicalID(c.OperationID) {
		return ErrMalformed
	}
	if c.ExpectedRevision != nil && *c.ExpectedRevision < 0 {
		return ErrMalformed
	}
	rev := int64(1)
	if c.ExpectedRevision != nil {
		rev = *c.ExpectedRevision + 1
	}
	compID := ""
	if c.PlannedDistanceSource != nil && *c.PlannedDistanceSource == domain.PlannedDistanceEngine {
		// Temporary UUID placeholder validated here when Computation is provided;
		// adapter assigns or preserves the persisted computation_id in transaction.
		compID = "00000000-0000-4000-8000-000000000001"
		if c.Computation == nil {
			return domain.ErrRouteValidation
		}
	}
	if _, err := domain.ValidateRoutePlanInput(c.OwnerKind, c.OwnerID, rev, c.Waypoints, c.Options, c.PlannedDistanceMetres, c.PlannedDistanceSource, compID); err != nil {
		return err
	}
	if c.Computation != nil {
		comp := domain.RouteComputation{
			OwnerID:         c.OwnerID,
			OwnerKind:       c.OwnerKind,
			PlanRevision:    rev,
			EngineID:        c.Computation.EngineID,
			MapDataVersion:  c.Computation.MapDataVersion,
			Profile:         c.Computation.Profile,
			Kind:            c.Computation.Kind,
			Options:         c.Options,
			DistanceMetres:  c.Computation.DistanceMetres,
			DurationSeconds: c.Computation.DurationSeconds,
			Line:            c.Computation.Line,
			Attribution:     c.Computation.Attribution,
			ComputedAt:      c.Computation.ComputedAt,
		}
		if _, err := domain.ValidateRouteComputation(comp); err != nil {
			return err
		}
	}
	return nil
}

func (c RoutePlanSaveCommand) Hash() string {
	raw, _ := json.Marshal(c)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

type RoutePlanView struct {
	OwnerKind         domain.RouteOwnerKind
	OwnerID           string
	UnitID            string
	Plan              *domain.RoutePlan
	LatestComputation *domain.RouteComputation
	Computations      []domain.RouteComputation
	History           []domain.RoutePlan
}

type RoutePlanAck struct {
	Plan        domain.RoutePlan
	Computation *domain.RouteComputation
	Replayed    bool
}

type RoutePlanRepository interface {
	ReadRoutePlan(ctx context.Context, scope kernel.OrgScope, actor RouteActor, decision access.Decision, request RoutePlanReadRequest) (RoutePlanView, error)
	SaveRoutePlan(ctx context.Context, scope kernel.OrgScope, actor RouteActor, decision access.Decision, cmd RoutePlanSaveCommand) (RoutePlanAck, error)
}

type RouteSuggestionsRequest struct {
	OwnerKind             domain.RouteOwnerKind
	OwnerID               string
	Product               string
	PlannedVolumeMilliL   int64
	BaseDistanceMetres    int64
	BaseDurationSec       int64
	Consumption           *domain.AssetConsumptionRef
	FreshnessLimitSeconds int64
	Reference             *domain.FuelPriceReference
	Candidates            []domain.StationCandidate
}

func (r RouteSuggestionsRequest) Validate() error {
	if (r.OwnerKind != "" || r.OwnerID != "") && (func() bool {
		_, err := domain.ParseRouteOwnerKind(string(r.OwnerKind))
		return err != nil || !canonicalID(r.OwnerID)
	}()) {
		return ErrMalformed
	}
	if r.Product == "" || r.BaseDistanceMetres <= 0 || r.BaseDurationSec < 0 || r.PlannedVolumeMilliL <= 0 || r.FreshnessLimitSeconds <= 0 {
		return domain.ErrRouteValidation
	}
	for _, c := range r.Candidates {
		if c.StationID == "" || c.StationName == "" {
			return domain.ErrRouteValidation
		}
	}
	return nil
}

type RoutePlanService struct {
	Enabled    bool
	Repository RoutePlanRepository
	Now        func() time.Time
}

func (s RoutePlanService) ReadPlan(ctx context.Context, scope kernel.OrgScope, actor RouteActor, decision access.Decision, request RoutePlanReadRequest) (RoutePlanView, error) {
	if !s.Enabled {
		return RoutePlanView{}, ErrTrackDisabled
	}
	if scope.Validate() != nil || request.Validate() != nil {
		return RoutePlanView{}, ErrMalformed
	}
	if actor.Validate() != nil {
		return RoutePlanView{}, ErrTrackUnauthorized
	}
	if absent(s.Repository) {
		return RoutePlanView{}, ErrUnavailable
	}
	return s.Repository.ReadRoutePlan(ctx, scope, actor, decision, request)
}

func (s RoutePlanService) SavePlan(ctx context.Context, scope kernel.OrgScope, actor RouteActor, decision access.Decision, cmd RoutePlanSaveCommand) (RoutePlanAck, error) {
	if !s.Enabled {
		return RoutePlanAck{}, ErrTrackDisabled
	}
	if scope.Validate() != nil {
		return RoutePlanAck{}, ErrMalformed
	}
	if actor.Validate() != nil {
		return RoutePlanAck{}, ErrTrackUnauthorized
	}
	if err := cmd.Validate(); err != nil {
		return RoutePlanAck{}, err
	}
	if absent(s.Repository) {
		return RoutePlanAck{}, ErrUnavailable
	}
	return s.Repository.SaveRoutePlan(ctx, scope, actor, decision, cmd)
}

func (s RoutePlanService) EvaluateSuggestions(ctx context.Context, scope kernel.OrgScope, actor RouteActor, decision access.Decision, req RouteSuggestionsRequest) ([]domain.SuggestionEvaluation, error) {
	if !s.Enabled {
		return nil, ErrTrackDisabled
	}
	if scope.Validate() != nil {
		return nil, ErrMalformed
	}
	if actor.Validate() != nil {
		return nil, ErrTrackUnauthorized
	}
	if err := req.Validate(); err != nil {
		return nil, err
	}
	if err := decision.Check(ctx, scope, actor.AccountID, RoutePlanPermission, scope.OrgID().String()); err != nil {
		return nil, err
	}
	out := make([]domain.SuggestionEvaluation, 0, len(req.Candidates))
	for _, c := range req.Candidates {
		eval, err := domain.EvaluateSuggestion(req.Product, req.PlannedVolumeMilliL, req.BaseDistanceMetres, req.BaseDurationSec, req.Consumption, req.FreshnessLimitSeconds, req.Reference, c)
		if err != nil {
			return nil, domain.ErrRouteValidation
		}
		out = append(out, eval)
	}
	return domain.RankSuggestions(out), nil
}
