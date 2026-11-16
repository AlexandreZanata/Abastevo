package domain

import (
	"slices"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
)

// EMP-TRIP-009 planned missions (B-BR-EMP-381..383, EDR-246, EDR-247).

// MissionLoad is the closed load vocabulary of a plan.
type MissionLoad string

const (
	LoadNone       MissionLoad = "NONE"
	LoadCargo      MissionLoad = "CARGO"
	LoadPassengers MissionLoad = "PASSENGERS"
)

// MaxMissionPassengers is the technical bound of a passenger count (EDR-247).
const MaxMissionPassengers = 999

var missionLoads = []MissionLoad{LoadNone, LoadCargo, LoadPassengers}

// MissionLoads returns the closed load vocabulary in its fixed order.
func MissionLoads() []MissionLoad { return slices.Clone(missionLoads) }

// ParseMissionLoad accepts only an exact load token.
func ParseMissionLoad(raw string) (MissionLoad, error) {
	if !slices.Contains(missionLoads, MissionLoad(raw)) {
		return "", ErrInvalid
	}
	return MissionLoad(raw), nil
}

// CostCentreRef is the scoped cost centre that is a plan's approved customer
// or project reference.
type CostCentreRef struct{ scopedID }

func NewCostCentreRef(scope kernel.OrgScope, id string) (CostCentreRef, error) {
	r, e := scoped(scope, id)
	return CostCentreRef{r}, e
}

// ValidPlanText is the plan text profile: the METER-001 profile and no
// standalone CPF-shaped token.
func ValidPlanText(raw string) error {
	if err := ValidMissionText(raw); err != nil {
		return err
	}
	if kernel.HasCPFShapedToken(raw) {
		return ErrInvalid
	}
	return nil
}

// PlanInput is a plan as the planner submitted it: exactly one
// destination (text or unit), a reason, an optional cost centre and a load
// token with its only allowed field.
type PlanInput struct {
	redacted
	DestinationText  *string
	DestinationUnit  *UnitRef
	Reason           string
	CostCentre       *CostCentreRef
	Load             string
	CargoDescription *string
	Passengers       *int
}

// PlanSnapshot is a validated plan.
type PlanSnapshot struct {
	redacted
	DestinationText  *string
	DestinationUnit  *UnitRef
	Reason           string
	CostCentre       *CostCentreRef
	Load             MissionLoad
	CargoDescription *string
	Passengers       *int
}

// Plan is one validated planning revision's content.
type Plan struct {
	redacted
	value PlanSnapshot
	valid bool
}

// Snapshot returns an independent copy of the plan.
func (p Plan) Snapshot() PlanSnapshot {
	s := p.value
	s.DestinationText, s.DestinationUnit = copyPointer(s.DestinationText), copyPointer(s.DestinationUnit)
	s.CostCentre, s.CargoDescription, s.Passengers = copyPointer(s.CostCentre), copyPointer(s.CargoDescription), copyPointer(s.Passengers)
	return s
}

// Valid reports a plan built by NewPlan.
func (p Plan) Valid() bool { return p.valid }

// NewPlan validates a plan in the scope (B-BR-EMP-382), first failure first:
// the scope; exactly one destination (the unit reference, else the text
// profile); the reason; the optional cost centre; the load token and its only
// field (NONE none, CARGO a description, PASSENGERS a count 1..999).
func NewPlan(scope kernel.OrgScope, in PlanInput) (Plan, error) {
	if scope.Validate() != nil {
		return Plan{}, ErrScope
	}
	if (in.DestinationText == nil) == (in.DestinationUnit == nil) {
		return Plan{}, ErrInvalid
	}
	if in.DestinationUnit != nil {
		if err := in.DestinationUnit.validate(scope); err != nil {
			return Plan{}, err
		}
	} else if err := ValidPlanText(*in.DestinationText); err != nil {
		return Plan{}, err
	}
	if err := ValidPlanText(in.Reason); err != nil {
		return Plan{}, err
	}
	if in.CostCentre != nil {
		if err := in.CostCentre.validate(scope); err != nil {
			return Plan{}, err
		}
	}
	load, err := ParseMissionLoad(in.Load)
	if err != nil {
		return Plan{}, err
	}
	switch load {
	case LoadNone:
		if in.CargoDescription != nil || in.Passengers != nil {
			return Plan{}, ErrInvalid
		}
	case LoadCargo:
		if in.CargoDescription == nil || in.Passengers != nil {
			return Plan{}, ErrInvalid
		}
		if err := ValidPlanText(*in.CargoDescription); err != nil {
			return Plan{}, err
		}
	case LoadPassengers:
		if in.CargoDescription != nil || in.Passengers == nil || *in.Passengers < 1 || *in.Passengers > MaxMissionPassengers {
			return Plan{}, ErrInvalid
		}
	}
	p := Plan{valid: true, value: PlanSnapshot{DestinationText: in.DestinationText, DestinationUnit: in.DestinationUnit, Reason: in.Reason, CostCentre: in.CostCentre,
		Load: load, CargoDescription: in.CargoDescription, Passengers: in.Passengers}}
	p.value = p.Snapshot()
	return p, nil
}

// Plan records a planning revision (B-BR-EMP-381): scope, the plan content, a
// positive expected revision, CONFLICT for a stale revision, then the table.
// The result is PLANNED at the next revision with the assignee and answer
// withdrawn; unit, asset, title and window are unchanged.
func (m Mission) Plan(scope kernel.OrgScope, expected int64, in PlanInput) (Mission, Plan, error) {
	if err := m.subject(scope); err != nil {
		return Mission{}, Plan{}, err
	}
	plan, err := NewPlan(scope, in)
	if err != nil {
		return Mission{}, Plan{}, err
	}
	if expected < 1 {
		return Mission{}, Plan{}, ErrInvalid
	}
	next, err := m.decide(expected, MissionPlan)
	if err != nil {
		return Mission{}, Plan{}, err
	}
	out := m.value.copy()
	out.State, out.Revision, out.Assignee, out.Answer = next, m.value.Revision+1, nil, nil
	return Mission{valid: true, value: out}, plan, nil
}
