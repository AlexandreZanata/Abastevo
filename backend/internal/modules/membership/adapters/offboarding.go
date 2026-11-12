package adapters

import (
	"context"
	"encoding/json"
	"errors"
	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/membership"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/domain"
	auditing "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/application"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type OffboardingStore struct {
	Pool        *pgxpool.Pool
	Authority   app.OffboardingAuthority
	Memberships app.TransactionRepository
	Audit       auditing.TransactionRunner
}

var _ app.OffboardingRepository = (*OffboardingStore)(nil)

func OffboardingAuditDefinitions() []audit.Definition {
	return []audit.Definition{{Action: "idn.membership.ended", TargetType: "membership", Reasons: []string{"offboarded", "left"}, RequireReason: true}}
}
func (s *OffboardingStore) ready() bool {
	return s != nil && s.Pool != nil && s.Authority != nil && s.Memberships != nil && s.Audit != nil
}

// Explicit serialization excludes account/tombstone binding and source credentials.
type offboardingWire struct {
	accessStateWire
	EmployeeState    d.EmployeeState  `json:"employee_state"`
	EmployeeRevision int64            `json:"employee_revision"`
	Digest           string           `json:"handover_digest"`
	Items            []d.HandoverItem `json:"items"`
}

func offboardingResultWire(v app.OffboardingView) map[string]any {
	return map[string]any{"membership_id": v.State.ID, "employee_id": v.State.EmployeeID, "access_model": v.State.Model, "state": v.State.State, "revision": v.State.Revision, "suspension_origin": v.State.Suspension, "end_reason": v.State.EndReason, "employee_state": v.EmployeeState, "employee_revision": v.EmployeeRevision, "handover_digest": v.Digest, "items": v.Items}
}
func offboardingError(ctx context.Context, e error) error {
	if errors.Is(e, d.ErrOpenResponsibilities) || errors.Is(e, app.ErrManagedRestricted) {
		return e
	}
	return accessChangeError(ctx, e)
}
func (s *OffboardingStore) authorize(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, a app.EmployeeActor, actorMember, target string, self, write bool) (d.Membership, error) {
	if e := (&MembershipAccessStore{}).lockAccounts(ctx, tx, scope, a, target, write); e != nil {
		return d.Membership{}, e
	}
	authorityMember := actorMember
	if self {
		authorityMember = target
	}
	if e := s.Authority.CheckOffboardingTx(ctx, tx, scope, a, authorityMember, self, write); e != nil {
		return d.Membership{}, e
	}
	m, e := s.Memberships.GetTx(ctx, tx, scope, target)
	if e != nil {
		return d.Membership{}, e
	}
	v, e := m.Snapshot(scope)
	if e != nil {
		return d.Membership{}, e
	}
	if self {
		id, model, e := v.Account.Reference(scope)
		if e != nil || id != a.AccountID || model != d.ExistingAccount {
			return d.Membership{}, app.ErrEmployeeNotFound
		}
	} else if target == actorMember {
		return d.Membership{}, d.ErrSelfSuspension
	}
	if v.Model != d.ExistingAccount {
		return d.Membership{}, app.ErrManagedRestricted
	}
	return m, nil
}
func (s *OffboardingStore) view(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, m d.Membership) (app.OffboardingView, error) {
	v, e := m.Snapshot(scope)
	if e != nil {
		return app.OffboardingView{}, e
	}
	employee, e := s.Memberships.GetEmployeeTx(ctx, tx, scope, v.EmployeeID)
	if e != nil {
		return app.OffboardingView{}, e
	}
	emp, e := employee.Snapshot(scope)
	if e != nil {
		return app.OffboardingView{}, e
	}
	rows, e := q.New(tx).OffboardingItems(ctx, q.OffboardingItemsParams{OrgID: scope.OrgID().String(), ID: v.ID})
	if e != nil {
		return app.OffboardingView{}, e
	}
	items := make([]d.HandoverItem, 0, len(rows))
	for _, r := range rows {
		items = append(items, d.HandoverItem{Key: r.Key, Revision: r.SourceRevision})
	}
	if v.State == d.MembershipEnded {
		raw, e := q.New(tx).ReadOffboardingHandover(ctx, q.ReadOffboardingHandoverParams{OrgID: scope.OrgID().String(), ID: v.ID})
		if e == nil {
			var flags struct {
				Items []d.HandoverItem `json:"items"`
			}
			if httpapi.DecodeJSON(raw, &flags) != nil {
				return app.OffboardingView{}, app.ErrRegisterUnavailable
			}
			items = flags.Items
		} else if !errors.Is(e, pgx.ErrNoRows) {
			return app.OffboardingView{}, e
		}
	}

	digest, e := d.HandoverDigest(scope, m, employee, items)
	if e != nil {
		return app.OffboardingView{}, e
	}
	state, e := d.AccessStateOf(scope, m)
	if e != nil {
		return app.OffboardingView{}, e
	}
	return app.OffboardingView{State: state, EmployeeState: emp.State, EmployeeRevision: emp.Revision, Items: items, Digest: digest}, nil
}
func (s *OffboardingStore) ReadOffboarding(ctx context.Context, scope kernel.OrgScope, a app.EmployeeActor, member, target string, self bool) (app.OffboardingView, error) {
	if scope.Validate() != nil || a.Validate() != nil || !validUUID(target) || !self && !validUUID(member) {
		return app.OffboardingView{}, d.ErrInvalid
	}
	if !s.ready() {
		return app.OffboardingView{}, app.ErrRegisterUnavailable
	}
	var out app.OffboardingView
	e := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		m, e := s.authorize(ctx, tx, scope, a, member, target, self, false)
		if e != nil {
			return e
		}
		out, e = s.view(ctx, tx, scope, m)
		return e
	})
	if e != nil {
		return app.OffboardingView{}, offboardingError(ctx, e)
	}
	if out.Validate(scope) != nil {
		return app.OffboardingView{}, app.ErrRegisterUnavailable
	}
	return out, nil
}
func (s *OffboardingStore) CompleteOffboarding(ctx context.Context, scope kernel.OrgScope, a app.EmployeeActor, member string, c app.OffboardingCommand) (app.OffboardingResult, error) {
	if scope.Validate() != nil || a.Validate() != nil || !c.Self && !validUUID(member) {
		return app.OffboardingResult{}, d.ErrInvalid
	}
	if e := c.Validate(); e != nil {
		return app.OffboardingResult{}, e
	}
	if !s.ready() {
		return app.OffboardingResult{}, app.ErrRegisterUnavailable
	}
	var out app.OffboardingResult
	e := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		m, e := s.authorize(ctx, tx, scope, a, member, c.MembershipID, c.Self, true)
		if e != nil {
			return e
		}
		queries := q.New(tx)
		kind, reason := "OFFBOARD", "offboarded"
		if c.Self {
			kind, reason = "LEAVE", "left"
		}
		digest := c.Digest(scope)
		prior, e := queries.OffboardingCommand(ctx, q.OffboardingCommandParams{OrgID: scope.OrgID().String(), OperationID: c.OperationID})
		if e == nil {
			if prior.ActorID != a.AccountID || prior.Kind != kind || prior.RequestHash != digest || prior.Reason != c.Reason {
				return app.ErrAccessOperationReused
			}
			var wire offboardingWire
			if httpapi.DecodeJSON(prior.Result, &wire) != nil {
				return app.ErrRegisterUnavailable
			}
			state, e := wire.state(scope)
			if e != nil || state.ID != c.MembershipID || state.State != d.MembershipEnded || state.Revision != c.ExpectedRevision+1 || wire.Digest != c.HandoverDigest {
				return app.ErrRegisterUnavailable
			}
			at, e := accessStamp(prior.OccurredAt)
			if e != nil {
				return e
			}
			out = app.OffboardingResult{View: app.OffboardingView{State: state, EmployeeState: wire.EmployeeState, EmployeeRevision: wire.EmployeeRevision, Items: wire.Items, Digest: wire.Digest}, OperationID: c.OperationID, Reason: prior.Reason, OccurredAt: at, Replayed: true}
			return nil
		}
		if !errors.Is(e, pgx.ErrNoRows) {
			return e
		}
		view, e := s.view(ctx, tx, scope, m)
		if e != nil {
			return e
		}
		if view.State.Revision != c.ExpectedRevision || view.EmployeeRevision != c.EmployeeRevision {
			return d.ErrConflict
		}
		if view.State.State != d.MembershipActive && view.State.State != d.MembershipSuspended {
			return d.ErrTransition
		}
		employee, e := s.Memberships.GetEmployeeTx(ctx, tx, scope, view.State.EmployeeID)
		if e != nil {
			return e
		}
		allowed, e := queries.MembershipAccessPolicy(ctx, q.MembershipAccessPolicyParams{OrgID: scope.OrgID().String(), ID: c.MembershipID, Action: "REDUCE"})
		if e != nil {
			return e
		}
		if allowed == "LAST_OWNER" {
			return kernel.ErrLastOrgOwner
		}
		if allowed != "ALLOW" {
			return kernel.ErrOrgAccessPolicyUnavailable
		}
		next, e := d.PlanOffboarding(scope, m, employee, c.ExpectedRevision, c.TerminateEmployee, c.Self, c.HandoverDigest, c.Flags, view.Items, true)
		if e != nil {
			return e
		}
		if c.TerminateEmployee {
			if _, e := employee.Transition(scope, c.EmployeeRevision, d.EmployeeTerminated, d.EmployeeConditions{OwnershipProtected: true, ResponsibilitiesReviewed: true}); e != nil {
				return e
			}
		}
		repeated, e := s.Audit.RecordChangeTx(ctx, tx, scope, audit.Event{OperationID: c.OperationID, ActorID: a.AccountID, Action: "idn.membership.ended", TargetType: "membership", TargetID: c.MembershipID, ReasonCode: reason, RequestHash: digest}, func(ctx context.Context, tx pgx.Tx) error {
			changed, e := s.Memberships.SaveTx(ctx, tx, scope, c.ExpectedRevision, next)
			if e != nil {
				return e
			}
			if !changed {
				return d.ErrConflict
			}
			if c.TerminateEmployee {
				n, e := queries.TerminateOffboardingEmployee(ctx, q.TerminateOffboardingEmployeeParams{OrgID: scope.OrgID().String(), ID: view.State.EmployeeID, ExpectedRevision: c.EmployeeRevision})
				if e != nil {
					return e
				}
				if n != 1 {
					return d.ErrConflict
				}
				view.EmployeeRevision++
				view.EmployeeState = d.EmployeeTerminated
			}
			authorityMember := member
			if c.Self {
				authorityMember = c.MembershipID
			}
			if e := s.Authority.CheckOffboardingTx(ctx, tx, scope, a, authorityMember, c.Self, true); e != nil {
				return e
			}
			state, e := d.AccessStateOf(scope, next)
			if e != nil {
				return e
			}
			view.State = state
			raw, e := json.Marshal(offboardingResultWire(view))
			if e != nil {
				return e
			}
			stamp, e := queries.InsertOffboardingCommand(ctx, q.InsertOffboardingCommandParams{OrgID: scope.OrgID().String(), OperationID: c.OperationID, ActorID: a.AccountID, ID: c.MembershipID, Kind: kind, RequestHash: digest, Reason: c.Reason, Result: raw})
			if e != nil {
				return e
			}
			at, e := accessStamp(stamp)
			if e != nil {
				return e
			}
			if e := queries.InsertOffboardingNotice(ctx, q.InsertOffboardingNoticeParams{OrgID: scope.OrgID().String(), OperationID: c.OperationID}); e != nil {
				return e
			}
			out = app.OffboardingResult{View: view, OperationID: c.OperationID, Reason: c.Reason, OccurredAt: at}
			return nil
		})
		if e != nil {
			return e
		}
		if repeated {
			return app.ErrAccessOperationReused
		}
		return nil
	})
	if e != nil {
		return app.OffboardingResult{}, offboardingError(ctx, e)
	}
	if out.Validate(scope, c) != nil {
		return app.OffboardingResult{}, app.ErrRegisterUnavailable
	}
	return out, nil
}
