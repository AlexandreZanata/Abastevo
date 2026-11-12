package application

import (
	"context"
	"encoding/json"
	"math"
	"time"

	admin "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/admin/application"
	adminrules "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/admin/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
	"github.com/jackc/pgx/v5"
)

type SuspensionAction string

const (
	SuspendOrganization    SuspensionAction = "SUSPEND"
	ReactivateOrganization SuspensionAction = "REACTIVATE"
)

func (a SuspensionAction) AdminAction() string {
	switch a {
	case SuspendOrganization:
		return "admin.organization.suspend"
	case ReactivateOrganization:
		return "admin.organization.reactivate"
	}
	return ""
}

type SuspensionCommand struct {
	privateOnboarding
	Action           SuspensionAction
	ExpectedRevision int64
	Category         d.Reason
}

func (c SuspensionCommand) Validate() error {
	if c.Action.AdminAction() == "" || c.ExpectedRevision < 1 || c.ExpectedRevision == math.MaxInt64 {
		return d.ErrInvalid
	}
	switch c.Category {
	case d.Billing, d.Terms, d.Security, d.Legal, d.OwnerRequest:
		return nil
	}
	return d.ErrInvalid
}

type FreshOperatorProof interface {
	ConfirmFreshTx(context.Context, pgx.Tx, string) error
}
type FreshOperatorRunner interface {
	FreshOperatorProof
	MutateFresh(context.Context, admin.Mutation, func(context.Context, pgx.Tx) (admin.MutationResult, []admin.AuditEvent, error)) (admin.MutationResult, error)
}
type SuspensionChange struct {
	privateOnboarding
	Before, After d.Snapshot
	PeriodID      string
	EffectiveAt   time.Time
}
type SuspensionRepository interface {
	ChangeSuspensionTx(context.Context, pgx.Tx, kernel.OrgScope, string, SuspensionCommand) (SuspensionChange, error)
}
type SuspensionService struct {
	Enabled    bool
	Runner     FreshOperatorRunner
	Repository SuspensionRepository
}

func (s SuspensionService) Change(ctx context.Context, scope kernel.OrgScope, m admin.Mutation, c SuspensionCommand) (admin.MutationResult, error) {
	if !s.Enabled {
		return admin.MutationResult{}, ErrSurfaceDisabled
	}
	if scope.Validate() != nil {
		return admin.MutationResult{}, d.ErrScope
	}
	if e := c.Validate(); e != nil {
		return admin.MutationResult{}, e
	}
	if m.Action != c.Action.AdminAction() || m.TargetType != "organization" || m.TargetID != scope.OrgID().String() || !canonicalID(m.Key) || !canonicalID(m.SessionID) || !adminrules.ValidHash(m.RequestHash) || !adminrules.ValidReason(m.Reason) {
		return admin.MutationResult{}, d.ErrInvalid
	}
	if s.Runner == nil || s.Repository == nil {
		return admin.MutationResult{}, ErrUnavailable
	}
	return s.Runner.MutateFresh(ctx, m, func(ctx context.Context, tx pgx.Tx) (admin.MutationResult, []admin.AuditEvent, error) {
		changed, e := s.Repository.ChangeSuspensionTx(ctx, tx, scope, m.SessionID, c)
		if e != nil {
			return admin.MutationResult{}, nil, e
		}
		before, _ := json.Marshal(map[string]any{"state": changed.Before.State, "revision": changed.Before.Revision, "suspension_reason": changed.Before.SuspensionReason})
		after, _ := json.Marshal(map[string]any{"state": changed.After.State, "revision": changed.After.Revision, "suspension_reason": changed.After.SuspensionReason})
		body, e := json.Marshal(map[string]any{"organization_id": scope.OrgID().String(), "state": changed.After.State, "revision": changed.After.Revision, "suspension_reason": changed.After.SuspensionReason, "period_id": changed.PeriodID, "effective_at": changed.EffectiveAt, "notice_state": "PENDING_NOTIFICATION_CORE"})
		if e != nil {
			return admin.MutationResult{}, nil, ErrUnavailable
		}
		return admin.MutationResult{Status: 200, Body: body}, []admin.AuditEvent{{Action: m.Action, TargetType: m.TargetType, TargetID: m.TargetID, Reason: m.Reason, Before: before, After: after}}, nil
	})
}

// SuspensionIntakeRepository joins only an owning verified business command.
// Occurrence time alone grants no authority; an opened instant must come from
// that owner's persisted entity, not from its request body.
type SuspensionIntakeRepository interface {
	AssessSuspensionTx(context.Context, pgx.Tx, kernel.OrgScope, time.Time, time.Time) (SuspensionIntake, error)
}
type SuspensionIntake struct {
	privateOnboarding
	Assessment   d.SuspensionAssessment
	ReceivedAt   time.Time
	RootRevision int64
}
