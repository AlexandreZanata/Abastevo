package adapters

import (
	"context"
	"encoding/json"
	"errors"
	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/membership"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/domain"
	permissions "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	auditing "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/application"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"regexp"
	"time"
)

type InvitationStore struct {
	Pool        *pgxpool.Pool
	Authority   app.InvitationAuthority
	Memberships app.TransactionRepository
	Audit       auditing.TransactionRunner
}

var _ app.InvitationRepository = (*InvitationStore)(nil)

func InvitationAuditDefinitions() []audit.Definition {
	return []audit.Definition{{Action: "idn.invitation.issued", TargetType: "invitation", Reasons: []string{"issued", "resent"}, RequireReason: true}, {Action: "idn.invitation.revoked", TargetType: "invitation", Reasons: []string{"revoked"}, RequireReason: true}}
}
func (s *InvitationStore) ready() bool {
	return s != nil && s.Pool != nil && s.Authority != nil && s.Memberships != nil && s.Audit != nil
}
func invitationError(ctx context.Context, e error) error {
	if e == nil {
		return nil
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	for _, known := range []error{app.ErrInvitationConflict, app.ErrInvitationOperationReused, app.ErrEmployeeHasAccess, d.ErrConditions} {
		if errors.Is(e, known) {
			return known
		}
	}
	if errors.Is(e, app.ErrNotFound) {
		return app.ErrEmployeeNotFound
	}
	if errors.Is(e, audit.ErrReused) {
		return app.ErrInvitationOperationReused
	}
	var pe *pgconn.PgError
	if errors.As(e, &pe) {
		switch pe.Code {
		case "P8801":
			return d.ErrInvalid
		case "P8803":
			return app.ErrInvitationConflict
		case "23505":
			if pe.ConstraintName == "org_invitations_one_pending" {
				return app.ErrInvitationConflict
			}
		}
		return app.ErrRegisterUnavailable
	}
	return employeeRegisterError(ctx, e)
}

// Deliberate whitelist codec: no verifier or delivery field can enter receipts.
type invitationReceipt struct {
	ID                 string              `json:"id"`
	EmployeeID         string              `json:"employee_id"`
	MembershipID       string              `json:"membership_id"`
	IssuerMembershipID string              `json:"issuer_membership_id"`
	OperationID        string              `json:"operation_id"`
	State              d.InvitationState   `json:"state"`
	Revision           int64               `json:"revision"`
	SettingsRevision   int64               `json:"settings_revision"`
	ValidityDays       int                 `json:"validity_days"`
	FailedAttempts     int                 `json:"failed_attempts,omitempty"`
	CreatedAt          time.Time           `json:"created_at"`
	ExpiresAt          time.Time           `json:"expires_at"`
	ChangedAt          time.Time           `json:"changed_at"`
	Grants             []d.InvitationGrant `json:"grants"`
	Cause              string              `json:"cause"`
	SourceType         string              `json:"source_type"`
	SourceID           string              `json:"source_id"`
	SourceRevision     int64               `json:"source_revision"`
}

func invitationWire(scope kernel.OrgScope, v d.Invitation) (invitationReceipt, error) {
	s, e := v.Snapshot(scope)
	return invitationReceipt{ID: s.ID, EmployeeID: s.EmployeeID, MembershipID: s.MembershipID, IssuerMembershipID: s.IssuerMembershipID, OperationID: s.OperationID, State: s.State, Revision: s.Revision, SettingsRevision: s.SettingsRevision, ValidityDays: s.ValidityDays, FailedAttempts: s.FailedAttempts, CreatedAt: s.CreatedAt, ExpiresAt: s.ExpiresAt, ChangedAt: s.ChangedAt, Grants: s.Grants, Cause: s.Cause, SourceType: s.SourceType, SourceID: s.SourceID, SourceRevision: s.SourceRevision}, e
}
func (v invitationReceipt) domain(scope kernel.OrgScope) (d.Invitation, error) {
	return d.RehydrateInvitation(scope, d.InvitationSnapshot{OrgID: scope.OrgID(), ID: v.ID, EmployeeID: v.EmployeeID, MembershipID: v.MembershipID, IssuerMembershipID: v.IssuerMembershipID, OperationID: v.OperationID, State: v.State, Revision: v.Revision, SettingsRevision: v.SettingsRevision, ValidityDays: v.ValidityDays, FailedAttempts: v.FailedAttempts, CreatedAt: v.CreatedAt, ExpiresAt: v.ExpiresAt, ChangedAt: v.ChangedAt, Grants: v.Grants, Cause: v.Cause, SourceType: v.SourceType, SourceID: v.SourceID, SourceRevision: v.SourceRevision})
}
func invitationRow(scope kernel.OrgScope, v q.ListInvitationsRow) (d.Invitation, error) {
	for _, at := range []pgtype.Timestamptz{v.CreatedAt, v.ExpiresAt, v.ChangedAt} {
		if !at.Valid || at.InfinityModifier != pgtype.Finite {
			return d.Invitation{}, app.ErrRegisterUnavailable
		}
	}
	var grants []d.InvitationGrant
	if json.Unmarshal(v.Grants, &grants) != nil {
		return d.Invitation{}, app.ErrRegisterUnavailable
	}
	result, e := (invitationReceipt{ID: v.ID, EmployeeID: v.EmployeeID, MembershipID: v.MembershipID, IssuerMembershipID: v.IssuerMembershipID, OperationID: v.OperationID, State: d.InvitationState(v.State), Revision: v.Revision, SettingsRevision: v.SettingsRevision, ValidityDays: int(v.ValidityDays), FailedAttempts: int(v.FailedAttempts), CreatedAt: v.CreatedAt.Time.UTC(), ExpiresAt: v.ExpiresAt.Time.UTC(), ChangedAt: v.ChangedAt.Time.UTC(), Grants: grants, Cause: v.Cause, SourceType: v.SourceType, SourceID: v.SourceID, SourceRevision: v.SourceRevision}).domain(scope)
	if e != nil {
		return d.Invitation{}, app.ErrRegisterUnavailable
	}
	return result, nil
}
func loadInvitation(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, id string) (d.Invitation, error) {
	rows, e := q.New(tx).ListInvitations(ctx, q.ListInvitationsParams{OrgID: scope.OrgID().String(), IDFilter: id, RowLimit: 1})
	if e != nil {
		return d.Invitation{}, e
	}
	if len(rows) != 1 {
		return d.Invitation{}, app.ErrEmployeeNotFound
	}
	return invitationRow(scope, rows[0])
}

var invitationHash = regexp.MustCompile(`^[a-f0-9]{64}$`)

func (s *InvitationStore) WriteInvitation(ctx context.Context, scope kernel.OrgScope, actor app.EmployeeActor, member string, c app.InvitationWrite, plan app.InvitationPlan) (d.Invitation, bool, error) {
	if e := employeeCall(scope, actor, member); e != nil {
		return d.Invitation{}, false, e
	}
	c, e := c.Canonical()
	if e != nil {
		return d.Invitation{}, false, e
	}
	if c.Action != app.InvitationRevoke {
		if !invitationHash.MatchString(plan.Verifier) {
			return d.Invitation{}, false, d.ErrInvalid
		}
		for _, id := range []string{plan.InvitationID, plan.MembershipID} {
			if parsed, e := kernel.ParseOrgID(id); e != nil || parsed.String() != id {
				return d.Invitation{}, false, d.ErrInvalid
			}
		}
	} else if plan.InvitationID != "" || plan.MembershipID != "" || plan.Verifier != "" {
		return d.Invitation{}, false, d.ErrInvalid
	}
	if !s.ready() {
		return d.Invitation{}, false, app.ErrRegisterUnavailable
	}
	digest := c.Digest(scope)
	var result d.Invitation
	replay, ready := false, false
	e = database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		facts, e := s.Authority.CheckInvitationTx(ctx, tx, scope, actor, member, c.Action)
		if e != nil {
			return e
		}
		queries := q.New(tx)
		prior, e := queries.InvitationCommand(ctx, q.InvitationCommandParams{OrgID: scope.OrgID().String(), OperationID: c.OperationID})
		if e == nil {
			if prior.ActorID != actor.AccountID || prior.Kind != string(c.Action) || prior.RequestHash != digest {
				return app.ErrInvitationOperationReused
			}
			var wire invitationReceipt
			if httpapi.DecodeJSON(prior.Result, &wire) != nil {
				return app.ErrRegisterUnavailable
			}
			result, e = wire.domain(scope)
			if e != nil {
				return app.ErrRegisterUnavailable
			}
			replay, ready = true, true
			return nil
		}
		if !errors.Is(e, pgx.ErrNoRows) {
			return e
		}
		if e := queries.ExpireInvitations(ctx, scope.OrgID().String()); e != nil {
			return e
		}
		var old d.InvitationSnapshot
		employee, grants := c.EmployeeID, c.Grants
		if c.Action != app.InvitationIssue {
			v, e := loadInvitation(ctx, tx, scope, c.InvitationID)
			if e != nil {
				return e
			}
			old, e = v.Snapshot(scope)
			if e != nil {
				return e
			}
			if old.State != d.InvitationPending || old.Revision != c.ExpectedRevision || !v.UsableAt(scope, facts.Now) {
				return app.ErrInvitationConflict
			}
			employee, grants = old.EmployeeID, old.Grants
		}
		if c.Action != app.InvitationRevoke {
			if facts.EmployeeID == employee {
				return permissions.DenyPermission("idn.invitation.manage")
			}
			roleKeys := []permissions.Role{}
			for _, a := range facts.Assignments {
				v, e := a.Snapshot(scope)
				if e != nil {
					return e
				}
				valid, e := a.ValidAt(scope, facts.Now)
				if e != nil {
					return e
				}
				if valid && v.MembershipID == member && v.Scope == permissions.OrgScope {
					roleKeys = append(roleKeys, v.Role)
				}
			}
			for _, grant := range grants {
				if !permissions.CanAssignScopedRole(permissions.SystemCatalog(), roleKeys, permissions.Role(grant.Role), permissions.ScopeType(grant.Scope)) {
					return permissions.DenyPermission("idn.invitation.manage")
				}
			}
			target, e := s.Memberships.GetEmployeeTx(ctx, tx, scope, employee)
			if e != nil {
				return e
			}
			v, e := target.Snapshot(scope)
			if e != nil {
				return e
			}
			if v.State != d.EmployeeActive {
				return app.ErrInvitationConflict
			}
		}
		invitationID := plan.InvitationID
		membershipID := old.MembershipID
		if c.Action == app.InvitationIssue {
			if e := queries.EndUnusedInvitationReservations(ctx, scope.OrgID().String()); e != nil {
				return e
			}
			live, e := queries.InvitationLiveMembership(ctx, q.InvitationLiveMembershipParams{OrgID: scope.OrgID().String(), EmployeeID: employee})
			if e == nil {
				if live.State == "INVITED" && live.AccessModel == "EXISTING_ACCOUNT" {
					return app.ErrInvitationConflict
				}
				return app.ErrEmployeeHasAccess
			}
			if !errors.Is(e, pgx.ErrNoRows) {
				return e
			}
			membershipID = plan.MembershipID
		} else if c.Action == app.InvitationResend {
			m, e := queries.InvitationLiveMembership(ctx, q.InvitationLiveMembershipParams{OrgID: scope.OrgID().String(), EmployeeID: employee})
			if e != nil {
				return e
			}
			if m.ID != old.MembershipID || m.State != "INVITED" || m.AccountID != "" {
				return app.ErrInvitationConflict
			}
		} else {
			invitationID = c.InvitationID
		}
		action, reason := "idn.invitation.issued", "issued"
		if c.Action == app.InvitationResend {
			reason = "resent"
		}
		if c.Action == app.InvitationRevoke {
			action, reason = "idn.invitation.revoked", "revoked"
		}
		repeated, e := s.Audit.RecordChangeTx(ctx, tx, scope, audit.Event{OperationID: c.OperationID, ActorID: actor.AccountID, Action: action, TargetType: "invitation", TargetID: invitationID, ReasonCode: reason, RequestHash: digest}, func(ctx context.Context, tx pgx.Tx) error {
			if c.Action != app.InvitationIssue {
				state, cause := "REVOKED", "MANAGER_REVOKED"
				if c.Action == app.InvitationResend {
					state, cause = "SUPERSEDED", "REPLACED"
				}
				n, e := queries.TransitionInvitation(ctx, q.TransitionInvitationParams{OrgID: scope.OrgID().String(), ID: c.InvitationID, ExpectedRevision: c.ExpectedRevision, State: state, Cause: cause})
				if e != nil {
					return e
				}
				if n != 1 {
					return app.ErrInvitationConflict
				}
			}
			if c.Action == app.InvitationIssue {
				target, e := s.Memberships.GetEmployeeTx(ctx, tx, scope, employee)
				if e != nil {
					return e
				}
				m, e := d.NewInvited(scope, membershipID, target, d.ExistingAccount)
				if e != nil {
					return e
				}
				created, e := s.Memberships.CreateTx(ctx, tx, scope, m)
				if e != nil {
					return e
				}
				if !created {
					return app.ErrRegisterUnavailable
				}
			}
			if c.Action != app.InvitationRevoke {
				raw, e := json.Marshal(grants)
				if e != nil {
					return app.ErrRegisterUnavailable
				}
				if e := queries.InsertInvitation(ctx, q.InsertInvitationParams{OrgID: scope.OrgID().String(), ID: invitationID, EmployeeID: employee, MembershipID: membershipID, IssuerID: member, OperationID: c.OperationID, Verifier: plan.Verifier, Grants: raw, ValidityDays: int32(facts.ValidityDays), SettingsRevision: facts.SettingsRevision}); e != nil {
					return e
				}
			}
			var e error
			result, e = loadInvitation(ctx, tx, scope, invitationID)
			if e != nil {
				return e
			}
			wire, e := invitationWire(scope, result)
			if e != nil {
				return e
			}
			raw, e := json.Marshal(wire)
			if e != nil {
				return app.ErrRegisterUnavailable
			}
			return queries.InsertInvitationCommand(ctx, q.InsertInvitationCommandParams{OrgID: scope.OrgID().String(), OperationID: c.OperationID, ActorID: actor.AccountID, InvitationID: invitationID, Kind: string(c.Action), RequestHash: digest, Result: raw})
		})
		if e != nil {
			return e
		}
		if repeated {
			return app.ErrRegisterUnavailable
		}
		ready = true
		return nil
	})
	if e != nil {
		if ready && ctx.Err() == nil {
			return d.Invitation{}, false, app.ErrRegisterUnavailable
		}
		return d.Invitation{}, false, invitationError(ctx, e)
	}
	return result, replay, nil
}
func (s *InvitationStore) ListInvitations(ctx context.Context, scope kernel.OrgScope, actor app.EmployeeActor, member string, f app.InvitationFilter) (app.InvitationPage, error) {
	if e := employeeCall(scope, actor, member); e != nil {
		return app.InvitationPage{}, e
	}
	if e := f.Validate(); e != nil {
		return app.InvitationPage{}, e
	}
	if !s.ready() {
		return app.InvitationPage{}, app.ErrRegisterUnavailable
	}
	result := app.InvitationPage{Invitations: []d.Invitation{}}
	ready := false
	e := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		if _, e := s.Authority.CheckInvitationTx(ctx, tx, scope, actor, member, app.InvitationRead); e != nil {
			return e
		}
		queries := q.New(tx)
		if e := queries.ExpireInvitations(ctx, scope.OrgID().String()); e != nil {
			return e
		}
		rows, e := queries.ListInvitations(ctx, q.ListInvitationsParams{OrgID: scope.OrgID().String(), IDFilter: f.ID, EmployeeFilter: f.EmployeeID, StateFilter: string(f.State), AfterID: f.AfterID, RowLimit: int32(f.Limit + 1)})
		if e != nil {
			return e
		}
		for i, row := range rows {
			if i == f.Limit {
				last, e := result.Invitations[i-1].Snapshot(scope)
				if e != nil {
					return e
				}
				result.NextID = last.ID
				break
			}
			v, e := invitationRow(scope, row)
			if e != nil {
				return e
			}
			result.Invitations = append(result.Invitations, v)
		}
		if f.ID != "" && len(result.Invitations) == 0 {
			return app.ErrEmployeeNotFound
		}
		ready = true
		return nil
	})
	if e != nil {
		if ready && ctx.Err() == nil {
			return app.InvitationPage{}, app.ErrRegisterUnavailable
		}
		return app.InvitationPage{}, invitationError(ctx, e)
	}
	return result, nil
}
