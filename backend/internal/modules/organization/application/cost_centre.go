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

var ErrCostCentreCodeReserved = errors.New("organization: cost-centre code permanently reserved")

type CostCentreWriteKind string

const (
	CreateCostCentre  CostCentreWriteKind = "CREATE"
	ChangeCostCentre  CostCentreWriteKind = "CHANGE"
	ArchiveCostCentre CostCentreWriteKind = "ARCHIVE"
	RestoreCostCentre CostCentreWriteKind = "RESTORE"
)

type CostCentreWriteInput struct {
	privateOnboarding
	Kind                  CostCentreWriteKind
	OperationID, CentreID string
	ExpectedRevision      int64
	Profile               d.CostCentreProfile
}
type CostCentreWrite struct {
	privateOnboarding
	value CostCentreWriteInput
}

func NewCostCentreWrite(v CostCentreWriteInput) (CostCentreWrite, error) {
	if !canonicalID(v.OperationID) {
		return CostCentreWrite{}, d.ErrInvalid
	}
	switch v.Kind {
	case CreateCostCentre:
		if v.CentreID != "" || v.ExpectedRevision != 0 {
			return CostCentreWrite{}, d.ErrInvalid
		}
	case ChangeCostCentre, ArchiveCostCentre, RestoreCostCentre:
		if !canonicalID(v.CentreID) || v.ExpectedRevision < 1 || v.ExpectedRevision == math.MaxInt64 {
			return CostCentreWrite{}, d.ErrInvalid
		}
	default:
		return CostCentreWrite{}, d.ErrInvalid
	}
	if v.Kind == CreateCostCentre || v.Kind == ChangeCostCentre {
		p, err := d.CanonicalCostCentreProfile(v.Profile)
		if err != nil {
			return CostCentreWrite{}, err
		}
		v.Profile = p
	} else if v.Profile != (d.CostCentreProfile{}) {
		return CostCentreWrite{}, d.ErrInvalid
	}
	return CostCentreWrite{value: v}, nil
}
func (c CostCentreWrite) Snapshot() (CostCentreWriteInput, error) {
	v, err := NewCostCentreWrite(c.value)
	if err != nil {
		return CostCentreWriteInput{}, err
	}
	return v.value, nil
}
func (c CostCentreWrite) Digest(scope kernel.OrgScope) string {
	v, err := c.Snapshot()
	if err != nil || scope.Validate() != nil {
		return ""
	}
	raw, _ := json.Marshal([]any{1, "org.cost_centre.command", scope.OrgID().String(), v.Kind, v.OperationID, v.CentreID, v.ExpectedRevision, v.Profile.Name, v.Profile.Code})
	return fmt.Sprintf("%x", sha256.Sum256(raw))
}

type CostCentreResult struct {
	CentreID string
	State    d.CostCentreState
	Revision int64
}
type CostCentreFilter struct {
	AfterID string
	State   d.CostCentreState
	Limit   int
}

func (f CostCentreFilter) Validate() error {
	if f.Limit < 1 || f.Limit > 100 || f.AfterID != "" && !canonicalID(f.AfterID) || f.State != "" && f.State != d.CostCentreActive && f.State != d.CostCentreArchived {
		return d.ErrInvalid
	}
	return nil
}

type CostCentrePage struct {
	privateOnboarding
	Centres []d.CostCentre
	NextID  string
}
type CostCentreRepository interface {
	WriteCostCentre(context.Context, kernel.OrgScope, CompanyActor, string, string, CostCentreWrite) (CostCentreResult, bool, error)
	ReadCostCentre(context.Context, kernel.OrgScope, CompanyActor, string, string) (d.CostCentre, error)
	ReadCostCentreCode(context.Context, kernel.OrgScope, CompanyActor, string, string) (d.CostCentre, error)
	ListCostCentres(context.Context, kernel.OrgScope, CompanyActor, string, CostCentreFilter) (CostCentrePage, error)
}
type CostCentreLookup interface {
	FindCostCentre(context.Context, kernel.OrgScope, string) (d.CostCentre, error)
}
type CostCentreService struct {
	Enabled    bool
	Repository CostCentreRepository
	NewID      func() (string, error)
}

func (s CostCentreService) Write(ctx context.Context, scope kernel.OrgScope, actor CompanyActor, decision access.Decision, command CostCentreWrite) (CostCentreResult, bool, error) {
	if !s.Enabled {
		return CostCentreResult{}, false, ErrSurfaceDisabled
	}
	v, err := command.Snapshot()
	if err != nil {
		return CostCentreResult{}, false, err
	}
	target := v.CentreID
	if v.Kind == CreateCostCentre {
		target = scope.OrgID().String()
	}
	if err := structureDecision(ctx, scope, actor, decision, target, true); err != nil {
		return CostCentreResult{}, false, err
	}
	if s.Repository == nil {
		return CostCentreResult{}, false, ErrUnavailable
	}
	id := ""
	if v.Kind == CreateCostCentre {
		if s.NewID == nil {
			return CostCentreResult{}, false, ErrUnavailable
		}
		id, err = s.NewID()
		if err != nil || !canonicalID(id) {
			return CostCentreResult{}, false, ErrUnavailable
		}
	}
	return s.Repository.WriteCostCentre(ctx, scope, actor, decision.Subject().MembershipID(), id, command)
}
func (s CostCentreService) Read(ctx context.Context, scope kernel.OrgScope, actor CompanyActor, decision access.Decision, id string) (d.CostCentre, error) {
	if !s.Enabled {
		return d.CostCentre{}, ErrSurfaceDisabled
	}
	if !canonicalID(id) {
		return d.CostCentre{}, d.ErrInvalid
	}
	if err := structureDecision(ctx, scope, actor, decision, id, false); err != nil {
		return d.CostCentre{}, err
	}
	if s.Repository == nil {
		return d.CostCentre{}, ErrUnavailable
	}
	return s.Repository.ReadCostCentre(ctx, scope, actor, decision.Subject().MembershipID(), id)
}
func (s CostCentreService) ReadCode(ctx context.Context, scope kernel.OrgScope, actor CompanyActor, decision access.Decision, code string) (d.CostCentre, error) {
	if !s.Enabled {
		return d.CostCentre{}, ErrSurfaceDisabled
	}
	canonical, err := d.CanonicalCostCentreCode(code)
	if err != nil {
		return d.CostCentre{}, err
	}
	if err := structureDecision(ctx, scope, actor, decision, scope.OrgID().String(), false); err != nil {
		return d.CostCentre{}, err
	}
	if s.Repository == nil {
		return d.CostCentre{}, ErrUnavailable
	}
	return s.Repository.ReadCostCentreCode(ctx, scope, actor, decision.Subject().MembershipID(), canonical)
}
func (s CostCentreService) List(ctx context.Context, scope kernel.OrgScope, actor CompanyActor, decision access.Decision, filter CostCentreFilter) (CostCentrePage, error) {
	if !s.Enabled {
		return CostCentrePage{}, ErrSurfaceDisabled
	}
	if err := filter.Validate(); err != nil {
		return CostCentrePage{}, err
	}
	if err := structureDecision(ctx, scope, actor, decision, scope.OrgID().String(), false); err != nil {
		return CostCentrePage{}, err
	}
	if s.Repository == nil {
		return CostCentrePage{}, ErrUnavailable
	}
	return s.Repository.ListCostCentres(ctx, scope, actor, decision.Subject().MembershipID(), filter)
}
