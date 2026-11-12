package application

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"math"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
)

var ErrUnitDuplicate = errors.New("organization: duplicate active unit")

type UnitDuplicateError struct {
	privateOnboarding
	field string
}

func (UnitDuplicateError) Error() string   { return ErrUnitDuplicate.Error() }
func (UnitDuplicateError) Unwrap() error   { return ErrUnitDuplicate }
func (e UnitDuplicateError) Field() string { return e.field }
func DuplicateUnit(field string) error {
	if field != "name" && field != "code" {
		return ErrUnavailable
	}
	return UnitDuplicateError{field: field}
}

// Compatibility names retain the completed unit API for shared structure counts.
type UnitUsageError = StructureUsageError

func UnitInUse(usage d.UnitUsage) error { return StructureInUse(usage) }

type UnitWriteKind string

const (
	CreateUnit  UnitWriteKind = "CREATE"
	ChangeUnit  UnitWriteKind = "CHANGE"
	ArchiveUnit UnitWriteKind = "ARCHIVE"
	RestoreUnit UnitWriteKind = "RESTORE"
)

type UnitWriteInput struct {
	privateOnboarding
	Kind                UnitWriteKind
	OperationID, UnitID string
	ExpectedRevision    int64
	Profile             d.UnitProfile
}
type UnitWrite struct {
	privateOnboarding
	value UnitWriteInput
}

func NewUnitWrite(v UnitWriteInput) (UnitWrite, error) {
	if !canonicalID(v.OperationID) {
		return UnitWrite{}, d.ErrInvalid
	}
	switch v.Kind {
	case CreateUnit:
		if v.UnitID != "" || v.ExpectedRevision != 0 {
			return UnitWrite{}, d.ErrInvalid
		}
	case ChangeUnit, ArchiveUnit, RestoreUnit:
		if !canonicalID(v.UnitID) || v.ExpectedRevision < 1 || v.ExpectedRevision == math.MaxInt64 {
			return UnitWrite{}, d.ErrInvalid
		}
	default:
		return UnitWrite{}, d.ErrInvalid
	}
	if v.Kind == CreateUnit || v.Kind == ChangeUnit {
		// Validate normalized text now, but retain omitted-zone intent. The first
		// transaction captures the actual current organization zone; retries hash
		// the original normalized input even after organization settings change.
		zone := v.Profile.TimeZone
		p, err := d.CanonicalUnitProfile(v.Profile, "UTC")
		if err != nil {
			return UnitWrite{}, err
		}
		p.TimeZone = zone
		v.Profile = p
	} else if v.Profile != (d.UnitProfile{}) {
		return UnitWrite{}, d.ErrInvalid
	}
	return UnitWrite{value: v}, nil
}
func (v UnitWrite) Snapshot() (UnitWriteInput, error) {
	canonical, err := NewUnitWrite(v.value)
	if err != nil {
		return UnitWriteInput{}, err
	}
	return canonical.value, nil
}
func (v UnitWrite) Digest(scope kernel.OrgScope) string {
	input, err := v.Snapshot()
	if err != nil || scope.Validate() != nil {
		return ""
	}
	p := input.Profile
	raw, _ := json.Marshal([]any{1, "org.unit.command", scope.OrgID().String(), input.Kind, input.OperationID, input.UnitID, input.ExpectedRevision, p.Name, p.Code, p.TimeZone, p.EstablishmentID, p.MunicipalityCode})
	return fmt.Sprintf("%x", sha256.Sum256(raw))
}

type UnitResult struct {
	UnitID   string
	State    d.UnitState
	Revision int64
}
type UnitFilter struct {
	AfterID string
	State   d.UnitState
	Limit   int
}

func (f UnitFilter) Validate() error {
	if f.Limit < 1 || f.Limit > 100 || f.AfterID != "" && !canonicalID(f.AfterID) || f.State != "" && f.State != d.UnitActive && f.State != d.UnitArchived {
		return d.ErrInvalid
	}
	return nil
}

type UnitPage struct {
	privateOnboarding
	Units  []d.Unit
	NextID string
}
type UnitRepository interface {
	WriteUnit(context.Context, kernel.OrgScope, CompanyActor, string, string, UnitWrite) (UnitResult, bool, error)
	ReadUnit(context.Context, kernel.OrgScope, CompanyActor, string, string) (d.Unit, error)
	ListUnits(context.Context, kernel.OrgScope, CompanyActor, string, UnitFilter) (UnitPage, error)
}
type UnitLookup interface {
	FindUnit(context.Context, kernel.OrgScope, string) (d.Unit, error)
}
type UnitService struct {
	Enabled    bool
	Repository UnitRepository
	NewID      func() (string, error)
}

func UnitPolicy(write bool) access.Policy { return StructurePolicy(write) }
func (s UnitService) Write(ctx context.Context, scope kernel.OrgScope, actor CompanyActor, decision access.Decision, command UnitWrite) (UnitResult, bool, error) {
	if !s.Enabled {
		return UnitResult{}, false, ErrSurfaceDisabled
	}
	v, err := command.Snapshot()
	if err != nil {
		return UnitResult{}, false, err
	}
	target := v.UnitID
	if v.Kind == CreateUnit {
		target = scope.OrgID().String()
	}
	if err := structureDecision(ctx, scope, actor, decision, target, true); err != nil {
		return UnitResult{}, false, err
	}
	if s.Repository == nil {
		return UnitResult{}, false, ErrUnavailable
	}
	id := ""
	if v.Kind == CreateUnit {
		if s.NewID == nil {
			return UnitResult{}, false, ErrUnavailable
		}
		id, err = s.NewID()
		if err != nil || !canonicalID(id) {
			return UnitResult{}, false, ErrUnavailable
		}
	}
	return s.Repository.WriteUnit(ctx, scope, actor, decision.Subject().MembershipID(), id, command)
}
func (s UnitService) Read(ctx context.Context, scope kernel.OrgScope, actor CompanyActor, decision access.Decision, id string) (d.Unit, error) {
	if !s.Enabled {
		return d.Unit{}, ErrSurfaceDisabled
	}
	if !canonicalID(id) {
		return d.Unit{}, d.ErrInvalid
	}
	if err := structureDecision(ctx, scope, actor, decision, id, false); err != nil {
		return d.Unit{}, err
	}
	if s.Repository == nil {
		return d.Unit{}, ErrUnavailable
	}
	return s.Repository.ReadUnit(ctx, scope, actor, decision.Subject().MembershipID(), id)
}
func (s UnitService) List(ctx context.Context, scope kernel.OrgScope, actor CompanyActor, decision access.Decision, filter UnitFilter) (UnitPage, error) {
	if !s.Enabled {
		return UnitPage{}, ErrSurfaceDisabled
	}
	if err := filter.Validate(); err != nil {
		return UnitPage{}, err
	}
	if err := structureDecision(ctx, scope, actor, decision, scope.OrgID().String(), false); err != nil {
		return UnitPage{}, err
	}
	if s.Repository == nil {
		return UnitPage{}, ErrUnavailable
	}
	return s.Repository.ListUnits(ctx, scope, actor, decision.Subject().MembershipID(), filter)
}
