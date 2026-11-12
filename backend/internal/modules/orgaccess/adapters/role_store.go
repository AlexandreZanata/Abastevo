// Package adapters implements current-authority role management in scoped transactions.
package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"time"

	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/orgaccess"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	auditing "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/application"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type RoleStore struct {
	Pool        *pgxpool.Pool
	Assignments *Store
	Audit       auditing.TransactionRunner
	Authority   app.RoleAuthority
}

var _ app.RoleManagementRepository = (*RoleStore)(nil)

func NewRoleStore(pool *pgxpool.Pool, assignments *Store, runner auditing.TransactionRunner, authority app.RoleAuthority) *RoleStore {
	return &RoleStore{Pool: pool, Assignments: assignments, Audit: runner, Authority: authority}
}
func RoleAuditDefinitions() []audit.Definition {
	return []audit.Definition{{Action: "auth.assignment.created", TargetType: "assignment", Reasons: []string{"granted", "changed"}, RequireReason: true}, {Action: "auth.assignment.ended", TargetType: "assignment", Reasons: []string{"ended", "changed"}, RequireReason: true}}
}
func (s *RoleStore) ready() bool {
	return s != nil && s.Pool != nil && s.Assignments != nil && s.Audit != nil && s.Authority != nil
}
func roleStoreError(ctx context.Context, e error) error {
	if e == nil {
		return nil
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	for _, known := range []error{d.ErrAssignmentScope, d.ErrAssignmentInvalid, d.ErrAssignmentConflict, d.ErrAssignmentEnded, d.ErrUnknownRole, d.ErrPermissionDenied, d.ErrCannotAssignOwner, d.ErrSelfModificationForbidden, d.ErrSelfAssignmentForbidden, app.ErrNotFound, app.ErrUnavailable, app.ErrRoleDisabled, app.ErrRoleInvalid, app.ErrRoleTargetNotFound, app.ErrRoleTargetNotActive, app.ErrRoleOperationReused, app.ErrRoleUnauthenticated, app.ErrNoMembership, app.ErrOrgSuspended, app.ErrOrgArchived, app.ErrSetupIncomplete, app.ErrEntitlementRequired, kernel.ErrLastOrgOwner, kernel.ErrOrgAccessPolicyUnavailable, kernel.ErrPendingOrgLimit, kernel.ErrOwnerRequiresPersonal} {
		if errors.Is(e, known) {
			return e
		}
	}
	if errors.Is(e, audit.ErrReused) {
		return app.ErrRoleOperationReused
	}
	var pe *pgconn.PgError
	if errors.As(e, &pe) {
		switch pe.Code {
		case "P7501":
			return kernel.ErrLastOrgOwner
		case "P7502":
			return kernel.ErrOrgAccessPolicyUnavailable
		case "P7503":
			return kernel.ErrPendingOrgLimit
		case "P7505":
			return kernel.ErrOwnerRequiresPersonal
		case "23505":
			return d.ErrAssignmentConflict
		}
	}
	return app.ErrUnavailable
}
func roleCall(scope kernel.OrgScope, a app.CompanyActor, member string) error {
	if scope.Validate() != nil {
		return d.ErrAssignmentScope
	}
	if a.Validate() != nil || validateID(scope, member) != nil {
		return app.ErrRoleUnauthenticated
	}
	return nil
}

// Prelock every affected account in UUID order before the current actor's
// capability/root guard. Role persistence reuses those locks; no root->account
// inversion is introduced when assigning/changing/ending another member.
func (s *RoleStore) lockAccounts(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, a app.CompanyActor, targetMember string, write bool) error {
	ids := []string{a.AccountID}
	if write && targetMember != "" {
		id, e := q.New(tx).AssignmentAccount(ctx, q.AssignmentAccountParams{OrgID: scope.OrgID().String(), MembershipID: targetMember})
		if e != nil && !errors.Is(e, pgx.ErrNoRows) {
			return e
		}
		if id != "" {
			ids = append(ids, id)
		}
	}
	slices.Sort(ids)
	ids = slices.Compact(ids)
	for _, id := range ids {
		var e error
		if write {
			e = q.New(tx).LockOwnerAccount(ctx, id)
		} else {
			e = q.New(tx).LockAssignmentAccount(ctx, id)
		}
		if e != nil {
			return e
		}
	}
	return nil
}
func (s *RoleStore) target(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, member string, role d.Role) error {
	row, e := q.New(tx).CheckTargetMembership(ctx, q.CheckTargetMembershipParams{OrgID: scope.OrgID().String(), MembershipID: member})
	if errors.Is(e, pgx.ErrNoRows) {
		return app.ErrRoleTargetNotFound
	}
	if e != nil {
		return e
	}
	if row.MembershipState != "ACTIVE" || row.EmployeeState != "ACTIVE" || row.AccountStatus != "active" || row.DeletionPending {
		return app.ErrRoleTargetNotActive
	}
	if role == d.Owner && row.AccessModel != "EXISTING_ACCOUNT" {
		return kernel.ErrOwnerRequiresPersonal
	}
	return nil
}
func currentRoleKeys(scope kernel.OrgScope, member string, assignments []d.Assignment, at time.Time) ([]d.Role, error) {
	roles := []d.Role{}
	for _, a := range assignments {
		v, e := a.Snapshot(scope)
		if e != nil {
			return nil, e
		}
		valid, e := a.ValidAt(scope, at)
		if e != nil {
			return nil, e
		}
		if valid && v.MembershipID == member && v.Scope == d.OrgScope {
			roles = append(roles, v.Role)
		}
	}
	return roles, nil
}

// Explicit stored receipt representation; domain JSON always redacts itself.
type roleAssignmentWire struct {
	ID, MembershipID, AssignedBy string
	Role                         d.Role
	Scope                        d.ScopeType
	TargetID                     string
	ValidFrom, ValidTo, EndedAt  time.Time
	Reason                       string
	Revision                     int64
}
type roleReceipt struct {
	CommandReason string
	Assignment    roleAssignmentWire
	Ended         *roleAssignmentWire `json:",omitempty"`
}

func assignmentWire(scope kernel.OrgScope, a d.Assignment) (roleAssignmentWire, error) {
	v, e := a.Snapshot(scope)
	return roleAssignmentWire{v.ID, v.MembershipID, v.AssignedBy, v.Role, v.Scope, v.TargetID, v.ValidFrom, v.ValidTo, v.EndedAt, v.Reason, v.Revision}, e
}
func (w roleAssignmentWire) domain(scope kernel.OrgScope) (d.Assignment, error) {
	return d.RehydrateAssignment(scope, d.AssignmentSnapshot{OrgID: scope.OrgID(), ID: w.ID, MembershipID: w.MembershipID, AssignedBy: w.AssignedBy, Role: w.Role, Scope: w.Scope, TargetID: w.TargetID, ValidFrom: w.ValidFrom, ValidTo: w.ValidTo, EndedAt: w.EndedAt, Reason: w.Reason, Revision: w.Revision})
}
func decodeRoleReceipt(scope kernel.OrgScope, raw []byte, change bool) (roleReceipt, error) {
	var v roleReceipt
	if httpapi.DecodeJSON(raw, &v) != nil || (v.Ended != nil) != change || app.ValidateRoleReason(v.CommandReason) != nil {
		return roleReceipt{}, app.ErrUnavailable
	}
	if _, e := v.Assignment.domain(scope); e != nil {
		return roleReceipt{}, app.ErrUnavailable
	}
	if v.Ended != nil {
		if _, e := v.Ended.domain(scope); e != nil {
			return roleReceipt{}, app.ErrUnavailable
		}
	}
	return v, nil
}

type roleMutation struct {
	kind                                   app.RoleAction
	operation, assignmentID, newID, member string
	expected                               int64
	role                                   d.Role
	scope                                  d.ScopeType
	target                                 string
	from, to                               time.Time
	reason, digest                         string
}

func (s *RoleStore) mutateRole(ctx context.Context, scope kernel.OrgScope, a app.CompanyActor, member string, in roleMutation) (roleReceipt, bool, error) {
	if e := roleCall(scope, a, member); e != nil {
		return roleReceipt{}, false, e
	}
	if !s.ready() {
		return roleReceipt{}, false, app.ErrUnavailable
	}
	var result roleReceipt
	replayed := false
	e := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		queries := q.New(tx)
		var old d.Assignment
		var before d.AssignmentSnapshot
		var lookupErr error
		targetMember := in.member
		requested := in.role
		permissionTarget := scope.OrgID().String()
		if in.kind != app.RoleAssignAction {
			old, lookupErr = s.Assignments.GetTx(ctx, tx, scope, in.assignmentID)
			if lookupErr == nil {
				before, lookupErr = old.Snapshot(scope)
				targetMember = before.MembershipID
			}
			permissionTarget = in.assignmentID
			if in.kind == app.RoleEndAction {
				requested = before.Role
			}
		}
		if requested == "" {
			requested = d.Admin
		}
		if e := s.lockAccounts(ctx, tx, scope, a, targetMember, true); e != nil {
			return e
		}
		candidates, e := s.Authority.CheckRoleTx(ctx, tx, scope, a, member, permissionTarget, in.kind, requested)
		if e != nil {
			return e
		}
		// Authority/root lock is held before reading an idempotent result. It never
		// restores authority removed since an earlier accepted command.
		prior, e := queries.GetRoleCommand(ctx, q.GetRoleCommandParams{OrgID: scope.OrgID().String(), OperationID: in.operation})
		if e == nil {
			if prior.ActorID != a.AccountID || prior.Kind != string(in.kind) || prior.RequestHash != in.digest {
				return app.ErrRoleOperationReused
			}
			result, e = decodeRoleReceipt(scope, prior.Result, in.kind == app.RoleChangeAction)
			if e != nil {
				return e
			}
			replayed = true
			return nil
		}
		if !errors.Is(e, pgx.ErrNoRows) {
			return e
		}
		if lookupErr != nil {
			return lookupErr
		}
		// Re-read target assignment after the root lock to check actual CAS state.
		if in.kind != app.RoleAssignAction {
			old, e = s.Assignments.GetTx(ctx, tx, scope, in.assignmentID)
			if e != nil {
				return e
			}
			before, e = old.Snapshot(scope)
			if e != nil {
				return e
			}
		}
		now, e := queries.RoleManagementClock(ctx)
		if e != nil || !now.Valid || now.InfinityModifier != pgtype.Finite {
			return app.ErrUnavailable
		}
		roles, e := currentRoleKeys(scope, member, candidates, now.Time)
		if e != nil {
			return e
		}
		if in.kind != app.RoleAssignAction && before.Role == d.Owner && !slices.Contains(roles, d.Owner) {
			return d.ErrCannotAssignOwner
		}
		if in.kind == app.RoleAssignAction && targetMember == member {
			return d.ErrSelfAssignmentForbidden
		}
		if in.kind == app.RoleEndAction && !d.CanModifyAssignment(member, targetMember, roles, true) {
			return d.ErrSelfModificationForbidden
		}
		if in.kind != app.RoleEndAction {
			if !d.CanAssignScopedRole(d.SystemCatalog(), roles, in.role, in.scope) {
				return d.ErrEscalation
			}
			if e := s.target(ctx, tx, scope, targetMember, in.role); e != nil {
				return e
			}
		}
		from := in.from
		if from.IsZero() {
			from = now.Time
		}
		if in.kind == app.RoleChangeAction && targetMember == member {
			reducing := d.PermissionsWithin(d.SystemCatalog(), []d.Role{before.Role}, in.role, in.scope == d.SelfScope) && !(before.Scope == d.SelfScope && in.scope != d.SelfScope) && !from.Before(before.ValidFrom) && (before.ValidTo.IsZero() || !in.to.IsZero() && !in.to.After(before.ValidTo))
			if !d.CanModifyAssignment(member, targetMember, roles, reducing) {
				return d.ErrSelfModificationForbidden
			}
		}
		var proposed d.Assignment
		if in.kind != app.RoleEndAction {
			proposed, e = d.RehydrateAssignment(scope, d.AssignmentSnapshot{OrgID: scope.OrgID(), ID: in.newID, MembershipID: targetMember, AssignedBy: member, Role: in.role, Scope: in.scope, TargetID: in.target, ValidFrom: from, ValidTo: in.to, Reason: in.reason, Revision: 1})
			if e != nil {
				return e
			}
		}
		action, eventTarget, reason := "auth.assignment.created", in.newID, "granted"
		if in.kind == app.RoleEndAction {
			action, eventTarget, reason = "auth.assignment.ended", in.assignmentID, "ended"
		}
		if in.kind == app.RoleChangeAction {
			reason = "changed"
		}
		result.CommandReason = in.reason
		repeated, e := s.Audit.RecordChangeTx(ctx, tx, scope, audit.Event{OperationID: in.operation, ActorID: a.AccountID, Action: action, TargetType: "assignment", TargetID: eventTarget, ReasonCode: reason, RequestHash: in.digest}, func(ctx context.Context, tx pgx.Tx) error {
			if in.kind != app.RoleAssignAction {
				ended, changed, e := s.Assignments.EndTx(ctx, tx, scope, in.assignmentID, in.expected)
				if e != nil {
					return e
				}
				if !changed {
					return d.ErrAssignmentConflict
				}
				wire, e := assignmentWire(scope, ended)
				if e != nil {
					return e
				}
				if in.kind == app.RoleEndAction {
					result.Assignment = wire
				} else {
					result.Ended = &wire
				}
			}
			if in.kind != app.RoleEndAction {
				created, e := s.Assignments.CreateTx(ctx, tx, scope, proposed)
				if e != nil {
					return e
				}
				if !created {
					return d.ErrAssignmentConflict
				}
				stored, e := s.Assignments.GetTx(ctx, tx, scope, in.newID)
				if e != nil {
					return e
				}
				wire, e := assignmentWire(scope, stored)
				if e != nil {
					return e
				}
				result.Assignment = wire
			}
			raw, e := json.Marshal(result)
			if e != nil {
				return app.ErrUnavailable
			}
			return queries.InsertRoleCommand(ctx, q.InsertRoleCommandParams{OrgID: scope.OrgID().String(), OperationID: in.operation, ActorID: a.AccountID, Kind: string(in.kind), RequestHash: in.digest, Result: raw})
		})
		if e != nil {
			return e
		}
		if repeated {
			return app.ErrUnavailable
		}
		return nil
	})
	if e != nil {
		return roleReceipt{}, false, roleStoreError(ctx, e)
	}
	return result, replayed, nil
}
func (s *RoleStore) AssignRole(ctx context.Context, scope kernel.OrgScope, a app.CompanyActor, member, newID string, in app.RoleAssignInput) (app.RoleResult, error) {
	if e := in.Validate(scope); e != nil {
		return app.RoleResult{}, e
	}
	if validateID(scope, newID) != nil {
		return app.RoleResult{}, app.ErrRoleInvalid
	}
	v, replay, e := s.mutateRole(ctx, scope, a, member, roleMutation{kind: app.RoleAssignAction, operation: in.OperationID, newID: newID, member: in.MembershipID, role: in.Role, scope: in.Scope, target: in.TargetID, from: in.ValidFrom, to: in.ValidTo, reason: in.Reason, digest: in.Digest(scope)})
	if e != nil {
		return app.RoleResult{}, e
	}
	assignment, e := v.Assignment.domain(scope)
	return app.RoleResult{Assignment: assignment, Replayed: replay}, e
}
func (s *RoleStore) EndRole(ctx context.Context, scope kernel.OrgScope, a app.CompanyActor, member string, in app.RoleEndInput) (app.RoleResult, error) {
	if e := in.Validate(); e != nil {
		return app.RoleResult{}, e
	}
	v, replay, e := s.mutateRole(ctx, scope, a, member, roleMutation{kind: app.RoleEndAction, operation: in.OperationID, assignmentID: in.AssignmentID, expected: in.ExpectedRevision, reason: in.Reason, digest: in.Digest(scope)})
	if e != nil {
		return app.RoleResult{}, e
	}
	assignment, e := v.Assignment.domain(scope)
	return app.RoleResult{Assignment: assignment, Replayed: replay}, e
}
func (s *RoleStore) ChangeRole(ctx context.Context, scope kernel.OrgScope, a app.CompanyActor, member, newID string, in app.RoleChangeInput) (app.RoleChangeResult, error) {
	if e := in.Validate(scope); e != nil {
		return app.RoleChangeResult{}, e
	}
	if validateID(scope, newID) != nil {
		return app.RoleChangeResult{}, app.ErrRoleInvalid
	}
	v, replay, e := s.mutateRole(ctx, scope, a, member, roleMutation{kind: app.RoleChangeAction, operation: in.OperationID, assignmentID: in.AssignmentID, newID: newID, expected: in.ExpectedRevision, role: in.Role, scope: in.Scope, target: in.TargetID, from: in.ValidFrom, to: in.ValidTo, reason: in.Reason, digest: in.Digest(scope)})
	if e != nil {
		return app.RoleChangeResult{}, e
	}
	assignment, e := v.Assignment.domain(scope)
	if e != nil || v.Ended == nil {
		return app.RoleChangeResult{}, app.ErrUnavailable
	}
	ended, e := v.Ended.domain(scope)
	return app.RoleChangeResult{EndedAssignment: ended, NewAssignment: assignment, Replayed: replay}, e
}
func (s *RoleStore) ReadRole(ctx context.Context, scope kernel.OrgScope, a app.CompanyActor, member, id string) (d.Assignment, error) {
	if e := roleCall(scope, a, member); e != nil {
		return d.Assignment{}, e
	}
	if validateID(scope, id) != nil {
		return d.Assignment{}, app.ErrRoleInvalid
	}
	if !s.ready() {
		return d.Assignment{}, app.ErrUnavailable
	}
	var result d.Assignment
	e := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		if e := s.lockAccounts(ctx, tx, scope, a, "", false); e != nil {
			return e
		}
		if _, e := s.Authority.CheckRoleTx(ctx, tx, scope, a, member, id, app.RoleReadAction, d.Admin); e != nil {
			return e
		}
		var e error
		result, e = s.Assignments.GetTx(ctx, tx, scope, id)
		return e
	})
	if e != nil {
		return d.Assignment{}, roleStoreError(ctx, e)
	}
	return result, nil
}
func (s *RoleStore) ListRoles(ctx context.Context, scope kernel.OrgScope, a app.CompanyActor, member string, f app.RoleListFilter) (app.RolePage, error) {
	if e := roleCall(scope, a, member); e != nil {
		return app.RolePage{}, e
	}
	if e := f.Validate(); e != nil {
		return app.RolePage{}, e
	}
	if !s.ready() {
		return app.RolePage{}, app.ErrUnavailable
	}
	result := app.RolePage{Assignments: []d.Assignment{}}
	state := f.State
	if state == "" {
		state = "ACTIVE"
	}
	e := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		if e := s.lockAccounts(ctx, tx, scope, a, "", false); e != nil {
			return e
		}
		if _, e := s.Authority.CheckRoleTx(ctx, tx, scope, a, member, scope.OrgID().String(), app.RoleListAction, d.Admin); e != nil {
			return e
		}
		rows, e := q.New(tx).ListAssignments(ctx, q.ListAssignmentsParams{OrgID: scope.OrgID().String(), MembershipFilter: f.MembershipID, RoleFilter: string(f.Role), StateFilter: state, AfterID: f.AfterID, RowLimit: int32(f.Limit + 1)})
		if e != nil {
			return e
		}
		for i, row := range rows {
			if i == f.Limit {
				last, e := result.Assignments[i-1].Snapshot(scope)
				if e != nil {
					return e
				}
				result.NextID = last.ID
				break
			}
			v, e := value(scope, q.GetAssignmentRow(row))
			if e != nil {
				return e
			}
			result.Assignments = append(result.Assignments, v)
		}
		return nil
	})
	if e != nil {
		return app.RolePage{}, roleStoreError(ctx, e)
	}
	return result, nil
}
