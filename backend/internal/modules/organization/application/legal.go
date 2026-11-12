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

var ErrLegalDuplicate = errors.New("organization: duplicate legal declaration")

// CompanyActor is metadata from normal cookie verification. It conveys no
// personal/managed eligibility or organization authority by itself.
type CompanyActor = CreationActor

type LegalWriteKind string

const (
	DeclareLegal             LegalWriteKind = "DECLARE"
	AddLegalEstablishment    LegalWriteKind = "ADD_ESTABLISHMENT"
	ChangeLegalEstablishment LegalWriteKind = "CHANGE_ESTABLISHMENT"
	WithdrawLegal            LegalWriteKind = "WITHDRAW"
)

type LegalWriteSnapshot struct {
	privateOnboarding
	Kind                                   LegalWriteKind
	OperationID, EntityID, EstablishmentID string
	ExpectedRevision                       int64
	Declaration                            d.LegalDeclaration
	Establishment                          d.EstablishmentInput
	Metadata                               d.EstablishmentMetadata
}
type LegalWrite struct {
	privateOnboarding
	value LegalWriteSnapshot
}

func DeclareLegalCommand(v d.LegalDeclaration) (LegalWrite, error) {
	if err := v.Validate(); err != nil {
		return LegalWrite{}, err
	}
	return LegalWrite{value: LegalWriteSnapshot{Kind: DeclareLegal, OperationID: v.OperationID(), Declaration: v}}, nil
}
func validLegalCommandIDs(entity, operation string, expected int64) bool {
	return canonicalID(entity) && canonicalID(operation) && expected > 0 && expected < math.MaxInt64
}
func AddEstablishmentCommand(entity, operation string, expected int64, input d.EstablishmentInput) (LegalWrite, error) {
	if !validLegalCommandIDs(entity, operation, expected) {
		return LegalWrite{}, d.ErrInvalid
	}
	v, err := d.CanonicalEstablishmentInput(input)
	if err != nil {
		return LegalWrite{}, err
	}
	return LegalWrite{value: LegalWriteSnapshot{Kind: AddLegalEstablishment, EntityID: entity, OperationID: operation, ExpectedRevision: expected, Establishment: v}}, nil
}
func ChangeEstablishmentCommand(entity, establishment, operation string, expected int64, input d.EstablishmentMetadata) (LegalWrite, error) {
	if !validLegalCommandIDs(entity, operation, expected) || !canonicalID(establishment) || establishment == entity {
		return LegalWrite{}, d.ErrInvalid
	}
	v, err := d.CanonicalEstablishmentMetadata(input)
	if err != nil {
		return LegalWrite{}, err
	}
	return LegalWrite{value: LegalWriteSnapshot{Kind: ChangeLegalEstablishment, EntityID: entity, EstablishmentID: establishment, OperationID: operation, ExpectedRevision: expected, Metadata: v}}, nil
}
func WithdrawLegalCommand(entity, operation string, expected int64) (LegalWrite, error) {
	if !validLegalCommandIDs(entity, operation, expected) {
		return LegalWrite{}, d.ErrInvalid
	}
	return LegalWrite{value: LegalWriteSnapshot{Kind: WithdrawLegal, EntityID: entity, OperationID: operation, ExpectedRevision: expected}}, nil
}
func (c LegalWrite) Validate() error {
	v := c.value
	switch v.Kind {
	case DeclareLegal:
		_, err := DeclareLegalCommand(v.Declaration)
		return err
	case AddLegalEstablishment:
		_, err := AddEstablishmentCommand(v.EntityID, v.OperationID, v.ExpectedRevision, v.Establishment)
		return err
	case ChangeLegalEstablishment:
		_, err := ChangeEstablishmentCommand(v.EntityID, v.EstablishmentID, v.OperationID, v.ExpectedRevision, v.Metadata)
		return err
	case WithdrawLegal:
		_, err := WithdrawLegalCommand(v.EntityID, v.OperationID, v.ExpectedRevision)
		return err
	default:
		return d.ErrInvalid
	}
}
func (c LegalWrite) Snapshot() (LegalWriteSnapshot, error) {
	if err := c.Validate(); err != nil {
		return LegalWriteSnapshot{}, err
	}
	return c.value, nil
}
func (c LegalWrite) Digest(scope kernel.OrgScope) string {
	if scope.Validate() != nil || c.Validate() != nil {
		return ""
	}
	v := c.value
	if v.Kind == DeclareLegal {
		return v.Declaration.Digest(scope)
	}
	raw, _ := json.Marshal([]any{1, "org.legal", scope.OrgID().String(), v.Kind, v.OperationID, v.EntityID, v.EstablishmentID, v.ExpectedRevision, v.Establishment.CNPJ, v.Establishment.Role, v.Establishment.TradeName, v.Establishment.MunicipalityCode, v.Metadata.Role, v.Metadata.TradeName, v.Metadata.MunicipalityCode})
	return fmt.Sprintf("%x", sha256.Sum256(raw))
}

type LegalPlan struct {
	privateOnboarding
	EntityID, EstablishmentID string
}
type LegalResult struct {
	privateOnboarding
	EntityID, EstablishmentID, State string
	Revision                         int64
}
type LegalFilter struct {
	privateOnboarding
	AfterID string
	Limit   int
	State   d.LegalState
}

func (f LegalFilter) Validate() error {
	if f.Limit < 1 || f.Limit > 100 || (f.AfterID != "" && !canonicalID(f.AfterID)) || (f.State != "" && f.State != d.LegalDeclared && f.State != d.LegalRevoked) {
		return d.ErrInvalid
	}
	return nil
}

type LegalPage struct {
	privateOnboarding
	Entities []d.LegalEntity
	NextID   string
}
type EstablishmentPage struct {
	privateOnboarding
	Establishments []d.Establishment
	NextID         string
}
type LegalReadResult struct {
	privateOnboarding
	Entity           d.LegalEntity
	Revealed         bool
	AuditOperationID string
}
type LegalRepository interface {
	WriteLegal(context.Context, kernel.OrgScope, CompanyActor, string, LegalPlan, LegalWrite) (LegalResult, bool, error)
	ReadLegal(context.Context, kernel.OrgScope, CompanyActor, string, string, bool, string) (LegalReadResult, error)
	ListLegal(context.Context, kernel.OrgScope, CompanyActor, string, LegalFilter) (LegalPage, error)
	ListEstablishments(context.Context, kernel.OrgScope, CompanyActor, string, string, LegalFilter) (EstablishmentPage, error)
}
type LegalLookup interface {
	LegalTargetExists(context.Context, kernel.OrgScope, string, string) (bool, error)
	FindLegalEntity(context.Context, kernel.OrgScope, string) (d.LegalEntity, error)
	FindEstablishment(context.Context, kernel.OrgScope, string) (d.Establishment, error)
}
type LegalService struct {
	Enabled    bool
	Repository LegalRepository
	NewID      func() (string, error)
}

func LegalPolicy(write, fresh bool) access.Policy {
	effect := access.Read
	if write {
		effect = access.ConfigurationWrite
	}
	return access.Policy{Permission: "org.legal_entity.manage", Capability: "organizations", Effect: effect, Fresh: fresh}
}
func legalDecision(ctx context.Context, scope kernel.OrgScope, actor CompanyActor, decision access.Decision, target string, write, fresh bool) error {
	if scope.Validate() != nil {
		return d.ErrScope
	}
	if actor.Validate() != nil {
		return ErrUnauthenticated
	}
	if decision.Policy() != LegalPolicy(write, fresh) || decision.Check(ctx, scope, actor.AccountID, "org.legal_entity.manage", target) != nil {
		return ErrUnauthenticated
	}
	return nil
}
func (s LegalService) newID() (string, error) {
	if s.NewID == nil {
		return "", ErrUnavailable
	}
	id, err := s.NewID()
	if err != nil || !canonicalID(id) {
		return "", ErrUnavailable
	}
	return id, nil
}
func (s LegalService) Write(ctx context.Context, scope kernel.OrgScope, actor CompanyActor, decision access.Decision, command LegalWrite) (LegalResult, bool, error) {
	if !s.Enabled {
		return LegalResult{}, false, ErrSurfaceDisabled
	}
	v, err := command.Snapshot()
	if err != nil {
		return LegalResult{}, false, err
	}
	target := v.EntityID
	if v.Kind == DeclareLegal {
		target = scope.OrgID().String()
	} else if v.Kind == ChangeLegalEstablishment {
		target = v.EstablishmentID
	}
	if err := legalDecision(ctx, scope, actor, decision, target, true, true); err != nil {
		return LegalResult{}, false, err
	}
	if s.Repository == nil {
		return LegalResult{}, false, ErrUnavailable
	}
	var plan LegalPlan
	if v.Kind == DeclareLegal {
		plan.EntityID, err = s.newID()
		if err == nil && v.Declaration.InputForStorage().Kind == d.Company {
			plan.EstablishmentID, err = s.newID()
		}
	} else if v.Kind == AddLegalEstablishment {
		plan.EstablishmentID, err = s.newID()
	}
	if err != nil || plan.EntityID != "" && plan.EntityID == plan.EstablishmentID {
		return LegalResult{}, false, ErrUnavailable
	}
	return s.Repository.WriteLegal(ctx, scope, actor, decision.Subject().MembershipID(), plan, command)
}
func (s LegalService) Read(ctx context.Context, scope kernel.OrgScope, actor CompanyActor, decision access.Decision, id string, reveal bool) (LegalReadResult, error) {
	if !s.Enabled {
		return LegalReadResult{}, ErrSurfaceDisabled
	}
	if !canonicalID(id) {
		return LegalReadResult{}, d.ErrInvalid
	}
	if err := legalDecision(ctx, scope, actor, decision, id, false, reveal); err != nil {
		return LegalReadResult{}, err
	}
	if s.Repository == nil {
		return LegalReadResult{}, ErrUnavailable
	}
	operation := ""
	var err error
	if reveal {
		operation, err = s.newID()
		if err != nil {
			return LegalReadResult{}, err
		}
	}
	return s.Repository.ReadLegal(ctx, scope, actor, decision.Subject().MembershipID(), id, reveal, operation)
}
func (s LegalService) List(ctx context.Context, scope kernel.OrgScope, actor CompanyActor, decision access.Decision, filter LegalFilter) (LegalPage, error) {
	if !s.Enabled {
		return LegalPage{}, ErrSurfaceDisabled
	}
	if err := filter.Validate(); err != nil {
		return LegalPage{}, err
	}
	if err := legalDecision(ctx, scope, actor, decision, scope.OrgID().String(), false, false); err != nil {
		return LegalPage{}, err
	}
	if s.Repository == nil {
		return LegalPage{}, ErrUnavailable
	}
	return s.Repository.ListLegal(ctx, scope, actor, decision.Subject().MembershipID(), filter)
}
func (s LegalService) Establishments(ctx context.Context, scope kernel.OrgScope, actor CompanyActor, decision access.Decision, entityID string, filter LegalFilter) (EstablishmentPage, error) {
	if !s.Enabled {
		return EstablishmentPage{}, ErrSurfaceDisabled
	}
	if !canonicalID(entityID) || filter.Validate() != nil || filter.State != "" {
		return EstablishmentPage{}, d.ErrInvalid
	}
	if err := legalDecision(ctx, scope, actor, decision, entityID, false, false); err != nil {
		return EstablishmentPage{}, err
	}
	if s.Repository == nil {
		return EstablishmentPage{}, ErrUnavailable
	}
	return s.Repository.ListEstablishments(ctx, scope, actor, decision.Subject().MembershipID(), entityID, filter)
}
