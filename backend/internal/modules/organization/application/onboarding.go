package application

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
)

var (
	ErrSurfaceDisabled = errors.New("organization: surface disabled")
	ErrUnauthenticated = errors.New("organization: current fresh proof required")
	ErrOperationReused = errors.New("organization: operation ID reused")
)

type privateOnboarding struct{}

func (privateOnboarding) Format(f fmt.State, _ rune) {
	_, _ = io.WriteString(f, "OrganizationOnboarding(redacted)")
}
func (privateOnboarding) MarshalJSON() ([]byte, error) {
	return json.Marshal("OrganizationOnboarding(redacted)")
}
func (privateOnboarding) LogValue() slog.Value {
	return slog.StringValue("OrganizationOnboarding(redacted)")
}

// CreationActor references a cookie already verified by the normal company
// entry. The repository rechecks its account/family/freshness under locks.
type CreationActor struct {
	privateOnboarding
	AccountID, SessionID string
}

func (a CreationActor) Validate() error {
	for _, s := range []string{a.AccountID, a.SessionID} {
		if !canonicalID(s) {
			return ErrUnauthenticated
		}
	}
	return nil
}
func canonicalID(s string) bool {
	id, err := kernel.ParseOrgID(s)
	return err == nil && id.String() == s
}

type Genesis struct {
	privateOnboarding
	Scope                                          kernel.OrgScope
	UnitID, EmployeeID, MembershipID, AssignmentID string
}

func (g Genesis) Validate() error {
	if g.Scope.Validate() != nil {
		return domain.ErrScope
	}
	seen := map[string]bool{g.Scope.OrgID().String(): true}
	for _, s := range []string{g.UnitID, g.EmployeeID, g.MembershipID, g.AssignmentID} {
		if !canonicalID(s) || seen[s] {
			return domain.ErrInvalid
		}
		seen[s] = true
	}
	return nil
}

type CreationResult struct {
	privateOnboarding
	OrgID                                          kernel.OrgID
	UnitID, EmployeeID, MembershipID, AssignmentID string
	State                                          domain.State
	Revision                                       int64
	DataMode                                       string
}
type ActivationResult struct {
	privateOnboarding
	OrgID    kernel.OrgID
	State    domain.State
	Revision int64
	DataMode string
}
type OnboardingRepository interface {
	CreateOrganization(context.Context, CreationActor, Genesis, domain.Creation) (CreationResult, bool, error)
	ActivateOrganization(context.Context, kernel.OrgScope, CreationActor, string, string, int64) (ActivationResult, bool, error)
}
type Onboarding struct {
	Enabled    bool
	Repository OnboardingRepository
	NewID      func() (string, error)
}

func (s Onboarding) Create(ctx context.Context, actor CreationActor, command domain.Creation) (CreationResult, bool, error) {
	if !s.Enabled {
		return CreationResult{}, false, ErrSurfaceDisabled
	}
	if err := actor.Validate(); err != nil {
		return CreationResult{}, false, err
	}
	if err := command.Validate(); err != nil {
		return CreationResult{}, false, err
	}
	if s.Repository == nil || s.NewID == nil {
		return CreationResult{}, false, ErrUnavailable
	}
	ids := make([]string, 5)
	for i := range ids {
		id, err := s.NewID()
		if err != nil || !canonicalID(id) {
			return CreationResult{}, false, ErrUnavailable
		}
		ids[i] = id
	}
	id, _ := kernel.ParseOrgID(ids[0])
	scope, err := kernel.NewOrgScope(id)
	if err != nil {
		return CreationResult{}, false, ErrUnavailable
	}
	plan := Genesis{Scope: scope, UnitID: ids[1], EmployeeID: ids[2], MembershipID: ids[3], AssignmentID: ids[4]}
	if plan.Validate() != nil {
		return CreationResult{}, false, ErrUnavailable
	}
	return s.Repository.CreateOrganization(ctx, actor, plan, command)
}
func ActivationDigest(scope kernel.OrgScope, operation string, expected int64) string {
	raw, _ := json.Marshal([]any{1, "org.activate", scope.OrgID().String(), operation, expected})
	return fmt.Sprintf("%x", sha256.Sum256(raw))
}
func (s Onboarding) Activate(ctx context.Context, scope kernel.OrgScope, actor CreationActor, decision access.Decision, operation string, expected int64) (ActivationResult, bool, error) {
	if !s.Enabled {
		return ActivationResult{}, false, ErrSurfaceDisabled
	}
	if scope.Validate() != nil {
		return ActivationResult{}, false, domain.ErrScope
	}
	if actor.Validate() != nil {
		return ActivationResult{}, false, ErrUnauthenticated
	}
	subject := decision.Subject()
	policy := decision.Policy()
	if err := decision.Check(ctx, scope, actor.AccountID, "org.settings.manage", scope.OrgID().String()); err != nil || policy != (access.Policy{Permission: "org.settings.manage", Capability: "organizations", Effect: access.ConfigurationWrite, Fresh: true}) {
		return ActivationResult{}, false, ErrUnauthenticated
	}
	if !canonicalID(operation) || expected < 1 || expected == math.MaxInt64 {
		return ActivationResult{}, false, domain.ErrInvalid
	}
	if s.Repository == nil {
		return ActivationResult{}, false, ErrUnavailable
	}
	return s.Repository.ActivateOrganization(ctx, scope, actor, subject.MembershipID(), operation, expected)
}
