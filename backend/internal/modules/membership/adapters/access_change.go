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
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"slices"
	"time"
)

type MembershipAccessStore struct {
	Pool        *pgxpool.Pool
	Authority   app.MembershipAccessAuthority
	Memberships app.TransactionRepository
	Periods     app.MembershipReactivationPeriods
	Audit       auditing.TransactionRunner
}

var _ app.MembershipAccessRepository = (*MembershipAccessStore)(nil)

func MembershipAccessAuditDefinitions() []audit.Definition {
	return []audit.Definition{{Action: "idn.membership.suspended", TargetType: "membership", Reasons: []string{"suspended"}, RequireReason: true}, {Action: "idn.membership.reactivated", TargetType: "membership", Reasons: []string{"reactivated"}, RequireReason: true}}
}
func (s *MembershipAccessStore) ready() bool {
	return s != nil && s.Pool != nil && s.Authority != nil && s.Memberships != nil && s.Periods != nil && s.Audit != nil
}
func accessChangeError(ctx context.Context, e error) error {
	if e == nil {
		return nil
	}
	for _, known := range []error{d.ErrSelfSuspension, d.ErrConditions, d.ErrEnded, app.ErrAccessOperationReused, kernel.ErrLastOrgOwner, kernel.ErrOrgAccessPolicyUnavailable} {
		if errors.Is(e, known) {
			return known
		}
	}
	if errors.Is(e, app.ErrNotFound) {
		return app.ErrEmployeeNotFound
	}
	if errors.Is(e, audit.ErrReused) {
		return app.ErrAccessOperationReused
	}
	var pe *pgconn.PgError
	if errors.As(e, &pe) {
		switch pe.Code {
		case "P7501":
			return kernel.ErrLastOrgOwner
		case "P7502":
			return kernel.ErrOrgAccessPolicyUnavailable
		default:
			// Target lookup already produced typed NOT_FOUND before mutation.
			// A SQL/deferred commit failure is an outage, never a missing target.
			return app.ErrRegisterUnavailable
		}
	}
	return employeeRegisterError(ctx, e)
}
func (s *MembershipAccessStore) lockAccounts(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, a app.EmployeeActor, target string, write bool) error {
	ids := []string{a.AccountID}
	bound, e := q.New(tx).MembershipAccessAccount(ctx, q.MembershipAccessAccountParams{OrgID: scope.OrgID().String(), ID: target})
	if e != nil && !errors.Is(e, pgx.ErrNoRows) {
		return e
	}
	if bound != "" {
		ids = append(ids, bound)
	}
	slices.Sort(ids)
	ids = slices.Compact(ids)
	for _, id := range ids {
		var e error
		if write {
			e = q.New(tx).LockMembershipAccessAccount(ctx, id)
		} else {
			e = q.New(tx).ShareMembershipAccessAccount(ctx, id)
		}
		if e != nil {
			return e
		}
	}
	return nil
}

type accessStateWire struct {
	ID                 string             `json:"membership_id"`
	EmployeeID         string             `json:"employee_id"`
	Model              d.AccessModel      `json:"access_model"`
	State              d.MembershipState  `json:"state"`
	Revision           int64              `json:"revision"`
	Suspension         d.SuspensionOrigin `json:"suspension_origin"`
	EndReason          d.EndReason        `json:"end_reason"`
	PreviousSuspension d.SuspensionOrigin `json:"previous_suspension_origin"`
}

func accessWire(v d.AccessState, previous d.SuspensionOrigin) accessStateWire {
	return accessStateWire{v.ID, v.EmployeeID, v.Model, v.State, v.Revision, v.Suspension, v.EndReason, previous}
}
func (w accessStateWire) state(scope kernel.OrgScope) (d.AccessState, error) {
	v := d.AccessState{OrgID: scope.OrgID(), ID: w.ID, EmployeeID: w.EmployeeID, Model: w.Model, State: w.State, Revision: w.Revision, Suspension: w.Suspension, EndReason: w.EndReason}
	return v, v.Validate(scope)
}
func accessStamp(at pgtype.Timestamptz) (time.Time, error) {
	if !at.Valid || at.InfinityModifier != pgtype.Finite || at.Time.IsZero() || at.Time.Year() < 1 || at.Time.Year() > 9999 {
		return time.Time{}, app.ErrRegisterUnavailable
	}
	return at.Time.UTC(), nil
}
func (s *MembershipAccessStore) ChangeMembershipAccess(ctx context.Context, scope kernel.OrgScope, actor app.EmployeeActor, member string, c app.MembershipAccessWrite) (app.MembershipAccessResult, error) {
	if e := employeeCall(scope, actor, member); e != nil {
		return app.MembershipAccessResult{}, e
	}
	if e := c.Validate(); e != nil {
		return app.MembershipAccessResult{}, e
	}
	if !s.ready() {
		return app.MembershipAccessResult{}, app.ErrRegisterUnavailable
	}
	var out app.MembershipAccessResult
	e := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		if e := s.lockAccounts(ctx, tx, scope, actor, c.MembershipID, true); e != nil {
			return e
		}
		now, e := s.Authority.CheckMembershipAccessTx(ctx, tx, scope, actor, member, c.Action)
		if e != nil {
			return e
		}
		if c.Action == app.MembershipSuspend && c.MembershipID == member {
			return d.ErrSelfSuspension
		}
		queries := q.New(tx)
		digest := c.Digest(scope)
		prior, e := queries.MembershipAccessCommand(ctx, q.MembershipAccessCommandParams{OrgID: scope.OrgID().String(), OperationID: c.OperationID})
		if e == nil {
			if prior.ActorID != actor.AccountID || prior.Kind != string(c.Action) || prior.RequestHash != digest || prior.Reason != c.Reason {
				return app.ErrAccessOperationReused
			}
			var wire accessStateWire
			if httpapi.DecodeJSON(prior.Result, &wire) != nil {
				return app.ErrRegisterUnavailable
			}
			state, e := wire.state(scope)
			if e != nil {
				return app.ErrRegisterUnavailable
			}
			at, e := accessStamp(prior.OccurredAt)
			if e != nil {
				return e
			}
			out = app.MembershipAccessResult{State: state, PreviousSuspension: wire.PreviousSuspension, OperationID: c.OperationID, Reason: prior.Reason, OccurredAt: at, Replayed: true}
			return nil
		}
		if !errors.Is(e, pgx.ErrNoRows) {
			return e
		}
		current, e := s.Memberships.GetTx(ctx, tx, scope, c.MembershipID)
		if e != nil {
			return e
		}
		before, e := current.Snapshot(scope)
		if e != nil {
			return e
		}
		if before.Revision != c.ExpectedRevision {
			return d.ErrConflict
		}
		if (c.Action == app.MembershipSuspend && before.State != d.MembershipActive) || (c.Action == app.MembershipReactivate && before.State != d.MembershipSuspended) {
			return d.ErrTransition
		}
		employee, e := s.Memberships.GetEmployeeTx(ctx, tx, scope, before.EmployeeID)
		if e != nil {
			return e
		}
		live := int64(0)
		if c.Action == app.MembershipReactivate {
			periods, e := s.Periods.ReactivationCandidatesTx(ctx, tx, scope, c.MembershipID)
			if e != nil {
				return e
			}
			for _, p := range periods {
				v, e := p.Snapshot(scope)
				if e != nil || v.MembershipID != c.MembershipID {
					return app.ErrRegisterUnavailable
				}
				valid, e := p.ValidAt(scope, now)
				if e != nil {
					return e
				}
				if valid {
					live++
				}
			}
			if live == 0 {
				return d.ErrConditions
			}
		}
		policyAction := "REDUCE"
		if c.Action == app.MembershipReactivate {
			policyAction = "ACTIVATE"
		}
		allowed, e := queries.MembershipAccessPolicy(ctx, q.MembershipAccessPolicyParams{OrgID: scope.OrgID().String(), ID: c.MembershipID, Action: policyAction})
		if e != nil {
			return e
		}
		if allowed == "LAST_OWNER" {
			return kernel.ErrLastOrgOwner
		}
		if allowed != "ALLOW" {
			return kernel.ErrOrgAccessPolicyUnavailable
		}
		next, e := d.PlanManagerAccessChange(scope, current, member, c.ExpectedRevision, d.ManagerAccessAction(c.Action), c.Reason, employee, live, true)
		if e != nil {
			return e
		}
		action, reason := "idn.membership.suspended", "suspended"
		if c.Action == app.MembershipReactivate {
			action, reason = "idn.membership.reactivated", "reactivated"
		}
		repeated, e := s.Audit.RecordChangeTx(ctx, tx, scope, audit.Event{OperationID: c.OperationID, ActorID: actor.AccountID, Action: action, TargetType: "membership", TargetID: c.MembershipID, ReasonCode: reason, RequestHash: digest}, func(ctx context.Context, tx pgx.Tx) error {
			changed, e := s.Memberships.SaveTx(ctx, tx, scope, c.ExpectedRevision, next)
			if e != nil {
				return e
			}
			if !changed {
				return d.ErrConflict
			}
			if _, e := s.Authority.CheckMembershipAccessTx(ctx, tx, scope, actor, member, c.Action); e != nil {
				return e
			}
			state, e := d.AccessStateOf(scope, next)
			if e != nil {
				return e
			}
			raw, e := json.Marshal(accessWire(state, before.Suspension))
			if e != nil {
				return e
			}
			stamp, e := queries.InsertMembershipAccessCommand(ctx, q.InsertMembershipAccessCommandParams{OrgID: scope.OrgID().String(), OperationID: c.OperationID, ActorID: actor.AccountID, MembershipID: c.MembershipID, Kind: string(c.Action), RequestHash: digest, Reason: c.Reason, Result: raw})
			if e != nil {
				return e
			}
			at, e := accessStamp(stamp)
			if e != nil {
				return e
			}
			if e := queries.InsertMembershipAccessNotice(ctx, q.InsertMembershipAccessNoticeParams{OrgID: scope.OrgID().String(), OperationID: c.OperationID}); e != nil {
				return e
			}
			out = app.MembershipAccessResult{State: state, PreviousSuspension: before.Suspension, OperationID: c.OperationID, Reason: c.Reason, OccurredAt: at}
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
		return app.MembershipAccessResult{}, accessChangeError(ctx, e)
	}
	return out, nil
}
func (s *MembershipAccessStore) ReadMembershipAccess(ctx context.Context, scope kernel.OrgScope, actor app.EmployeeActor, member, target string) (app.MembershipAccessResult, error) {
	if e := employeeCall(scope, actor, member); e != nil {
		return app.MembershipAccessResult{}, e
	}
	if !validUUID(target) {
		return app.MembershipAccessResult{}, d.ErrInvalid
	}
	if !s.ready() {
		return app.MembershipAccessResult{}, app.ErrRegisterUnavailable
	}
	var out app.MembershipAccessResult
	e := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		if e := s.lockAccounts(ctx, tx, scope, actor, target, false); e != nil {
			return e
		}
		at, e := s.Authority.CheckMembershipAccessTx(ctx, tx, scope, actor, member, app.MembershipAccessRead)
		if e != nil {
			return e
		}
		current, e := s.Memberships.GetTx(ctx, tx, scope, target)
		if e != nil {
			return e
		}
		state, e := d.AccessStateOf(scope, current)
		if e != nil {
			return e
		}
		out = app.MembershipAccessResult{State: state, OccurredAt: at}
		return nil
	})
	if e != nil {
		return app.MembershipAccessResult{}, accessChangeError(ctx, e)
	}
	return out, nil
}
