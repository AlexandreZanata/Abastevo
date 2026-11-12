package domain

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"math"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
)

type privateCostCentre struct{}

func (privateCostCentre) Format(f fmt.State, _ rune)   { redact(f, "CostCentre(redacted)") }
func (privateCostCentre) MarshalJSON() ([]byte, error) { return json.Marshal("CostCentre(redacted)") }
func (privateCostCentre) LogValue() slog.Value         { return slog.StringValue("CostCentre(redacted)") }

type CostCentreState string

const (
	CostCentreActive   CostCentreState = "ACTIVE"
	CostCentreArchived CostCentreState = "ARCHIVED"
)

type CostCentreProfile struct {
	privateCostCentre
	Name, Code string
}
type CostCentreSnapshot struct {
	privateCostCentre
	OrgID kernel.OrgID
	ID    string
	CostCentreProfile
	State    CostCentreState
	Revision int64
}
type CostCentre struct {
	privateCostCentre
	value CostCentreSnapshot
}

func CanonicalCostCentreCode(raw string) (string, error) {
	value, err := structureText(raw)
	if err != nil {
		return "", ErrInvalid
	}
	return value, nil
}
func CanonicalCostCentreProfile(v CostCentreProfile) (CostCentreProfile, error) {
	name, err := structureText(v.Name)
	if err != nil {
		return CostCentreProfile{}, ErrInvalid
	}
	code, err := structureText(v.Code)
	if err != nil {
		return CostCentreProfile{}, ErrInvalid
	}
	return CostCentreProfile{Name: name, Code: code}, nil
}
func NewCostCentre(scope kernel.OrgScope, id string, p CostCentreProfile) (CostCentre, error) {
	if scope.Validate() != nil {
		return CostCentre{}, ErrScope
	}
	if !canonicalLegalID(id) {
		return CostCentre{}, ErrInvalid
	}
	profile, err := CanonicalCostCentreProfile(p)
	if err != nil {
		return CostCentre{}, err
	}
	return CostCentre{value: CostCentreSnapshot{OrgID: scope.OrgID(), ID: id, CostCentreProfile: profile, State: CostCentreActive, Revision: 1}}, nil
}
func RehydrateCostCentre(scope kernel.OrgScope, v CostCentreSnapshot) (CostCentre, error) {
	if scope.Validate() != nil || v.OrgID != scope.OrgID() {
		return CostCentre{}, ErrScope
	}
	if !canonicalLegalID(v.ID) || v.Revision < 1 || v.State != CostCentreActive && v.State != CostCentreArchived {
		return CostCentre{}, ErrInvalid
	}
	p, err := CanonicalCostCentreProfile(v.CostCentreProfile)
	if err != nil || p != v.CostCentreProfile {
		return CostCentre{}, ErrInvalid
	}
	return CostCentre{value: v}, nil
}
func (c CostCentre) Snapshot(scope kernel.OrgScope) (CostCentreSnapshot, error) {
	if _, err := RehydrateCostCentre(scope, c.value); err != nil {
		return CostCentreSnapshot{}, err
	}
	return c.value, nil
}
func (c CostCentre) next(scope kernel.OrgScope, expected int64) (CostCentreSnapshot, error) {
	v, err := c.Snapshot(scope)
	if err != nil {
		return CostCentreSnapshot{}, err
	}
	if expected < 1 || expected != v.Revision {
		return CostCentreSnapshot{}, ErrConflict
	}
	if v.Revision == math.MaxInt64 {
		return CostCentreSnapshot{}, ErrRevision
	}
	v.Revision++
	return v, nil
}

// Change selects the current label/code. Persistence must reserve every old
// and new normalized code for this stable identity in the same transaction.
func (c CostCentre) Change(scope kernel.OrgScope, expected int64, p CostCentreProfile) (CostCentre, error) {
	v, err := c.next(scope, expected)
	if err != nil {
		return CostCentre{}, err
	}
	profile, err := CanonicalCostCentreProfile(p)
	if err != nil {
		return CostCentre{}, err
	}
	v.CostCentreProfile = profile
	return CostCentre{value: v}, nil
}
func (c CostCentre) Archive(scope kernel.OrgScope, expected int64, usage StructureUsage) (CostCentre, error) {
	v, err := c.next(scope, expected)
	if err != nil {
		return CostCentre{}, err
	}
	if v.State != CostCentreActive {
		return CostCentre{}, ErrTransition
	}
	if usage.Validate() != nil {
		return CostCentre{}, ErrConditions
	}
	if usage.InUse() {
		return CostCentre{}, ErrStructureInUse
	}
	v.State = CostCentreArchived
	return CostCentre{value: v}, nil
}
func (c CostCentre) Restore(scope kernel.OrgScope, expected int64) (CostCentre, error) {
	v, err := c.next(scope, expected)
	if err != nil {
		return CostCentre{}, err
	}
	if v.State != CostCentreArchived {
		return CostCentre{}, ErrTransition
	}
	v.State = CostCentreActive
	return CostCentre{value: v}, nil
}
