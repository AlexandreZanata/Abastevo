package domain

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
)

var ErrLastUnit = errors.New("organization: last active unit")
var ErrStructureInUse = errors.New("organization: structure currently in use")

type privateUnit struct{}

func (privateUnit) Format(f fmt.State, _ rune)   { redact(f, "OperatingUnit(redacted)") }
func (privateUnit) MarshalJSON() ([]byte, error) { return json.Marshal("OperatingUnit(redacted)") }
func (privateUnit) LogValue() slog.Value         { return slog.StringValue("OperatingUnit(redacted)") }

type UnitState string

const (
	UnitActive   UnitState = "ACTIVE"
	UnitArchived UnitState = "ARCHIVED"
)

type UnitProfile struct {
	privateUnit
	Name, Code, TimeZone, EstablishmentID, MunicipalityCode string
}
type UnitSnapshot struct {
	privateUnit
	OrgID kernel.OrgID
	ID    string
	UnitProfile
	State    UnitState
	Revision int64
}
type Unit struct {
	privateUnit
	value UnitSnapshot
}

// UnitUsage retains the unit API for the shared structure category totals.
type UnitUsage = StructureUsage

type UnitArchiveFacts struct {
	ActiveUnits int64
	Usage       UnitUsage
}

// Defaults are captured as explicit values at the command's server revision;
// no unit silently inherits a later organization time-zone change.
func CanonicalUnitProfile(v UnitProfile, organizationZone string) (UnitProfile, error) {
	if _, ok := kernel.LoadTimeZone(organizationZone); !ok {
		return UnitProfile{}, ErrInvalid
	}
	name, err := structureText(v.Name)
	if err != nil {
		return UnitProfile{}, ErrInvalid
	}
	code, err := structureText(v.Code)
	if err != nil {
		return UnitProfile{}, ErrInvalid
	}
	v.Name, v.Code = name, code
	if v.TimeZone == "" {
		v.TimeZone = organizationZone
	}
	if _, ok := kernel.LoadTimeZone(v.TimeZone); !ok {
		return UnitProfile{}, ErrInvalid
	}
	if v.EstablishmentID != "" && !canonicalLegalID(v.EstablishmentID) {
		return UnitProfile{}, ErrInvalid
	}
	// The municipality profile is shared with declared establishments; it is
	// only a reference shape, never an external address-verification claim.
	metadata, err := CanonicalEstablishmentMetadata(EstablishmentMetadata{MunicipalityCode: v.MunicipalityCode})
	if err != nil {
		return UnitProfile{}, ErrInvalid
	}
	v.MunicipalityCode = metadata.MunicipalityCode
	return v, nil
}
func NewUnit(scope kernel.OrgScope, id string, profile UnitProfile, organizationZone string) (Unit, error) {
	if scope.Validate() != nil {
		return Unit{}, ErrScope
	}
	if !canonicalLegalID(id) {
		return Unit{}, ErrInvalid
	}
	v, err := CanonicalUnitProfile(profile, organizationZone)
	if err != nil {
		return Unit{}, err
	}
	return Unit{value: UnitSnapshot{OrgID: scope.OrgID(), ID: id, UnitProfile: v, State: UnitActive, Revision: 1}}, nil
}
func RehydrateUnit(scope kernel.OrgScope, v UnitSnapshot) (Unit, error) {
	if scope.Validate() != nil || v.OrgID != scope.OrgID() {
		return Unit{}, ErrScope
	}
	if !canonicalLegalID(v.ID) || v.Revision < 1 || (v.State != UnitActive && v.State != UnitArchived) {
		return Unit{}, ErrInvalid
	}
	p, err := CanonicalUnitProfile(v.UnitProfile, v.UnitProfile.TimeZone)
	if err != nil || p != v.UnitProfile {
		return Unit{}, ErrInvalid
	}
	return Unit{value: v}, nil
}
func (u Unit) Snapshot(scope kernel.OrgScope) (UnitSnapshot, error) {
	if _, err := RehydrateUnit(scope, u.value); err != nil {
		return UnitSnapshot{}, err
	}
	return u.value, nil
}
func (u Unit) next(scope kernel.OrgScope, expected int64) (UnitSnapshot, error) {
	v, err := u.Snapshot(scope)
	if err != nil {
		return UnitSnapshot{}, err
	}
	if expected < 1 || expected != v.Revision {
		return UnitSnapshot{}, ErrConflict
	}
	if v.Revision == math.MaxInt64 {
		return UnitSnapshot{}, ErrRevision
	}
	v.Revision++
	return v, nil
}
func (u Unit) Change(scope kernel.OrgScope, expected int64, profile UnitProfile, organizationZone string) (Unit, error) {
	v, err := u.next(scope, expected)
	if err != nil {
		return Unit{}, err
	}
	p, err := CanonicalUnitProfile(profile, organizationZone)
	if err != nil {
		return Unit{}, err
	}
	v.UnitProfile = p
	return Unit{value: v}, nil
}
func (u Unit) Archive(scope kernel.OrgScope, expected int64, facts UnitArchiveFacts) (Unit, error) {
	v, err := u.next(scope, expected)
	if err != nil {
		return Unit{}, err
	}
	if v.State != UnitActive {
		return Unit{}, ErrTransition
	}
	if facts.ActiveUnits < 1 || facts.Usage.Validate() != nil {
		return Unit{}, ErrConditions
	}
	if facts.ActiveUnits < 2 {
		return Unit{}, ErrLastUnit
	}
	if facts.Usage.InUse() {
		return Unit{}, ErrStructureInUse
	}
	v.State = UnitArchived
	return Unit{value: v}, nil
}
func (u Unit) Restore(scope kernel.OrgScope, expected int64) (Unit, error) {
	v, err := u.next(scope, expected)
	if err != nil {
		return Unit{}, err
	}
	if v.State != UnitArchived {
		return Unit{}, ErrTransition
	}
	v.State = UnitActive
	return Unit{value: v}, nil
}

// NewDefaultUnit retains onboarding's existing MAIN identity/profile behavior.
func NewDefaultUnit(scope kernel.OrgScope, id, name, zone string) (Unit, error) {
	return NewUnit(scope, id, UnitProfile{Name: name, Code: "MAIN", TimeZone: zone}, zone)
}
