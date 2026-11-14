package adapters

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/membership"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/domain"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permissions "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	auditing "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/application"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"strings"
	"time"
)

// ErrUnknownInvitationLocator is internal admission routing only. Its public
// response is exactly TOKEN_INVALID, shared with every unusable token state.
var ErrUnknownInvitationLocator = errors.New("membership: unknown invitation locator")

type RedemptionStore struct {
	Pool        *pgxpool.Pool
	Authority   app.RedemptionAuthority
	Managers    app.InvitationAuthority
	Memberships app.TransactionRepository
	Roles       access.TransactionRepository
	Audit       auditing.TransactionRunner
	Secret      []byte
	NewID       func() (string, error)
}

func RedemptionAuditDefinitions() []audit.Definition {
	return []audit.Definition{
		{Action: "idn.invitation.accepted", TargetType: "invitation"}, {Action: "idn.invitation.declined", TargetType: "invitation"},
		{Action: "idn.invitation.confirmed", TargetType: "invitation"}, {Action: "idn.invitation.rejected", TargetType: "invitation"},
	}
}
func (s *RedemptionStore) ready() bool {
	return s != nil && s.Pool != nil && s.Authority != nil && s.Managers != nil && s.Memberships != nil && s.Roles != nil && s.Audit != nil && len(s.Secret) >= 32 && s.NewID != nil
}
func invitationCode(raw string) (id, secret string, ok bool) {
	parts := strings.Split(raw, ".")
	if len(parts) != 3 || parts[0] != "AIN1" {
		return "", "", false
	}
	parsed, e := kernel.ParseOrgID(parts[1])
	if e != nil || parsed.String() != parts[1] {
		return "", "", false
	}
	// A recognized locator with a malformed secret is still a failed secret
	// attempt, not an unknown locator that could bypass the five-attempt fence.
	return parts[1], parts[2], true
}
func (s *RedemptionStore) verifier(scope kernel.OrgScope, id, secret string) string {
	derive := hmac.New(sha256.New, s.Secret)
	derive.Write([]byte("abastevo.enterprise.invitation.key.v1"))
	mac := hmac.New(sha256.New, derive.Sum(nil))
	mac.Write([]byte("invitation.verifier.v1:" + scope.OrgID().String() + ":" + id + ":" + secret))
	return hex.EncodeToString(mac.Sum(nil))
}
func (s *RedemptionStore) consent(scope kernel.OrgScope, v d.InvitationSnapshot, f app.RedemptionFacts, account string) string {
	raw, _ := json.Marshal([]any{"enterprise.invitation.consent.v1", account, scope.OrgID().String(), v.ID, v.Grants, f.Name, f.Verified, f.RootRevision, f.SettingsRevision, f.RequireConfirmation, app.InvitationNoticeVersion, app.InvitationNotice})
	mac := hmac.New(sha256.New, s.Secret)
	mac.Write(raw)
	return hex.EncodeToString(mac.Sum(nil))
}
func canGrantInvitation(scope kernel.OrgScope, issuer string, grants []d.InvitationGrant, assignments []permissions.Assignment, now time.Time) bool {
	roles := []permissions.Role{}
	for _, a := range assignments {
		v, e := a.Snapshot(scope)
		if e != nil {
			return false
		}
		valid, e := a.ValidAt(scope, now)
		if e != nil {
			return false
		}
		if valid && v.MembershipID == issuer && v.Scope == permissions.OrgScope {
			roles = append(roles, v.Role)
		}
	}
	for _, g := range grants {
		if !permissions.CanAssignScopedRole(permissions.SystemCatalog(), roles, permissions.Role(g.Role), permissions.ScopeType(g.Scope)) {
			return false
		}
	}
	return true
}
func redemptionError(ctx context.Context, e error) error {
	if e == nil {
		return nil
	}
	for _, known := range []error{ErrUnknownInvitationLocator, app.ErrTokenInvalid, app.ErrAlreadyMember, app.ErrManagedRestricted, app.ErrConsentChanged, app.ErrEmployeeUnauthenticated, app.ErrInvitationOperationReused, app.ErrInvitationConflict} {
		if errors.Is(e, known) {
			return known
		}
	}
	return invitationError(ctx, e)
}

type redemptionReceipt struct {
	OrgID           string            `json:"organization_id"`
	InvitationID    string            `json:"invitation_id"`
	MembershipID    string            `json:"membership_id"`
	State           string            `json:"state"`
	MembershipState d.MembershipState `json:"membership_state"`
	AcceptedAt      *time.Time        `json:"accepted_at,omitempty"`
	Deadline        *time.Time        `json:"confirmation_deadline,omitempty"`
	Alias           string            `json:"alias,omitempty"`
	Revision        int64             `json:"revision,omitempty"`
}

func (r redemptionReceipt) result() (app.RedemptionResult, error) {
	for _, id := range []string{r.OrgID, r.InvitationID, r.MembershipID} {
		parsed, e := kernel.ParseOrgID(id)
		if e != nil || parsed.String() != id {
			return app.RedemptionResult{}, app.ErrRegisterUnavailable
		}
	}
	if r.State != "ACCEPTED" && r.State != "DECLINED" {
		return app.RedemptionResult{}, app.ErrRegisterUnavailable
	}
	v := app.RedemptionResult{OrgID: r.OrgID, InvitationID: r.InvitationID, MembershipID: r.MembershipID, State: r.State, MembershipState: r.MembershipState}
	if r.State == "DECLINED" {
		if r.MembershipState != d.MembershipEnded || r.AcceptedAt != nil || r.Deadline != nil {
			return app.RedemptionResult{}, app.ErrRegisterUnavailable
		}
	} else {
		if (r.MembershipState != d.MembershipInvited && r.MembershipState != d.MembershipActive) || r.AcceptedAt == nil || r.Deadline == nil || r.AcceptedAt.IsZero() || !r.Deadline.Equal(r.AcceptedAt.Add(168*time.Hour)) {
			return app.RedemptionResult{}, app.ErrRegisterUnavailable
		}
		v.AcceptedAt = *r.AcceptedAt
		v.ConfirmationDeadline = *r.Deadline
		v.ConfirmationRequired = r.MembershipState == d.MembershipInvited
	}
	return v, nil
}
func (s *RedemptionStore) Redeem(ctx context.Context, actor app.EmployeeActor, c app.RedemptionCommand) (app.RedemptionResult, error) {
	if !s.ready() {
		return app.RedemptionResult{}, app.ErrRegisterUnavailable
	}
	if actor.Validate() != nil {
		return app.RedemptionResult{}, app.ErrEmployeeUnauthenticated
	}
	if e := c.Validate(); e != nil {
		return app.RedemptionResult{}, e
	}
	id, secret, validCode := invitationCode(c.Code)
	var out app.RedemptionResult
	var refused error
	e := database.WithOrgBootstrapTx(ctx, s.Pool, func(tx pgx.Tx) error {
		if e := s.Authority.CheckPersonalTx(ctx, tx, actor); e != nil {
			return e
		}
		if !validCode {
			return ErrUnknownInvitationLocator
		}
		queries := q.New(tx)
		if e := queries.InstallInvitationLocator(ctx, id); e != nil {
			return e
		}
		org, e := queries.ResolveInvitationLocator(ctx, id)
		if errors.Is(e, pgx.ErrNoRows) {
			return ErrUnknownInvitationLocator
		}
		if e != nil {
			return e
		}
		oid, e := kernel.ParseOrgID(org)
		if e != nil {
			return app.ErrRegisterUnavailable
		}
		scope, e := kernel.NewOrgScope(oid)
		if e != nil {
			return e
		}
		if _, e := tx.Exec(ctx, `SELECT set_config('anpfuel.org_id',$1,true)`, org); e != nil {
			return e
		}
		if e := database.ValidateOrgTx(ctx, tx, scope); e != nil {
			return e
		}
		if e := s.Authority.LockRedemptionTx(ctx, tx, scope, actor, id, "", true); e != nil {
			return e
		}
		stored, e := queries.RedemptionVerifier(ctx, q.RedemptionVerifierParams{OrgID: org, ID: id})
		if e != nil {
			return e
		}
		candidate := s.verifier(scope, id, secret)
		if !hmac.Equal([]byte(candidate), []byte(stored.Verifier)) {
			if e := queries.FailedInvitationSecret(ctx, q.FailedInvitationSecretParams{OrgID: org, ID: id}); e != nil {
				return e
			}
			refused = app.ErrTokenInvalid
			return nil
		}
		raw, _ := json.Marshal([]any{1, actor.AccountID, c.Action, c.OperationID, id, candidate, c.ConsentDigest, c.Consent})
		digest := hex.EncodeToString(sha256Sum(raw))
		if c.Action != "PREVIEW" {
			prior, e := queries.PersonalInvitationReceipt(ctx, q.PersonalInvitationReceiptParams{OrgID: org, OperationID: c.OperationID})
			if e == nil {
				if prior.ActorID != actor.AccountID || prior.Kind != c.Action || prior.RequestHash != digest {
					return app.ErrTokenInvalid
				}
				var saved redemptionReceipt
				if httpapi.DecodeJSON(prior.Result, &saved) != nil {
					return app.ErrRegisterUnavailable
				}
				out, e = saved.result()
				out.Replayed = true
				return e
			}
			if !errors.Is(e, pgx.ErrNoRows) {
				return e
			}
		}
		if _, e := tx.Exec(ctx, `UPDATE org_invitations SET state='EXPIRED',revision=revision+1,cause='EXPIRED' WHERE org_id=$1 AND id=$2 AND state='PENDING' AND expires_at<=clock_timestamp()`, org, id); e != nil {
			return e
		}
		// An admission past its deadline must not keep this account's INVITED
		// reservation as an existing membership (B-BR-EMP-174).
		if e := queries.ExpireInvitationAcceptances(ctx, org); e != nil {
			return e
		}
		value, e := loadInvitation(ctx, tx, scope, id)
		if e != nil {
			return e
		}
		v, e := value.Snapshot(scope)
		if e != nil {
			return e
		}
		facts, e := s.Authority.CheckRedemptionTx(ctx, tx, scope, actor, id)
		if e != nil {
			return e
		}
		if !value.UsableAt(scope, facts.Now) || !facts.Eligible || !canGrantInvitation(scope, v.IssuerMembershipID, v.Grants, facts.Assignments, facts.Now) {
			refused = app.ErrTokenInvalid
			return nil
		}
		member, e := s.Memberships.GetTx(ctx, tx, scope, v.MembershipID)
		if e != nil {
			return e
		}
		mv, e := member.Snapshot(scope)
		if e != nil {
			return e
		}
		if mv.State != d.MembershipInvited || mv.Account != (d.AccountRef{}) {
			return app.ErrTokenInvalid
		}
		confirm, e := d.InvitationNeedsConfirmation(v.Grants, facts.RequireConfirmation)
		if e != nil {
			return e
		}
		consent := s.consent(scope, v, facts, actor.AccountID)
		if c.Action == "PREVIEW" {
			out = app.RedemptionResult{OrgID: org, InvitationID: id, OrganizationName: facts.Name, Verified: facts.Verified, Grants: v.Grants, ConfirmationRequired: confirm, ConsentDigest: consent}
			return nil
		}
		if c.Action == "ACCEPT" {
			if !hmac.Equal([]byte(consent), []byte(c.ConsentDigest)) {
				return app.ErrConsentChanged
			}
		}
		_, existingErr := queries.ExistingInvitationAccountMember(ctx, q.ExistingInvitationAccountMemberParams{OrgID: org, AccountID: actor.AccountID})
		if existingErr == nil {
			return app.ErrAlreadyMember
		}
		if !errors.Is(existingErr, pgx.ErrNoRows) {
			return existingErr
		}
		action := "idn.invitation.accepted"
		if c.Action == "DECLINE" {
			action = "idn.invitation.declined"
		}
		var receipt redemptionReceipt
		repeated, e := s.Audit.RecordChangeTx(ctx, tx, scope, audit.Event{OperationID: c.OperationID, ActorID: actor.AccountID, Action: action, TargetType: "invitation", TargetID: id, RequestHash: digest}, func(ctx context.Context, tx pgx.Tx) error {
			receipt = redemptionReceipt{OrgID: org, InvitationID: id, MembershipID: v.MembershipID, State: "DECLINED", MembershipState: d.MembershipEnded}
			if c.Action == "ACCEPT" {
				account, e := d.PersonalAccount(actor.AccountID)
				if e != nil {
					return e
				}
				bound, e := member.BindAccount(scope, mv.Revision, account)
				if e != nil {
					return e
				}
				if _, e = s.Memberships.SaveTx(ctx, tx, scope, mv.Revision, bound); e != nil {
					return e
				}
				if e = queries.BeginInvitationAcceptance(ctx, q.BeginInvitationAcceptanceParams{OrgID: org, InvitationID: id, MembershipID: v.MembershipID, AccountID: actor.AccountID, Alias: facts.Alias, ConfirmationRequired: confirm, SettingsRevision: facts.SettingsRevision}); e != nil {
					return e
				}
				if !confirm {
					if e = s.activate(ctx, tx, scope, v, v.IssuerMembershipID, facts.Now); e != nil {
						return e
					}
					if n, e := queries.TransitionInvitationAcceptance(ctx, q.TransitionInvitationAcceptanceParams{OrgID: org, ID: id, State: "ACTIVE", Cause: "AUTO_CONFIRMED", Revision: 1}); e != nil {
						return e
					} else if n != 1 {
						return app.ErrInvitationConflict
					}
				}
				a, e := queries.InvitationAcceptance(ctx, q.InvitationAcceptanceParams{OrgID: org, ID: id})
				if e != nil {
					return e
				}
				at, deadline := a.AcceptedAt.Time.UTC(), a.Deadline.Time.UTC()
				receipt.State = "ACCEPTED"
				receipt.MembershipState = d.MembershipInvited
				if !confirm {
					receipt.MembershipState = d.MembershipActive
				}
				receipt.AcceptedAt = &at
				receipt.Deadline = &deadline
			}
			current, e := s.Authority.CheckRedemptionTx(ctx, tx, scope, actor, id)
			if e != nil {
				return e
			}
			if !current.Eligible || !canGrantInvitation(scope, v.IssuerMembershipID, v.Grants, current.Assignments, current.Now) {
				return app.ErrTokenInvalid
			}
			state, cause := "DECLINED", "PERSON_DECLINED"
			if c.Action == "ACCEPT" {
				state, cause = "ACCEPTED", "PERSON_ACCEPTED"
			}
			n, e := queries.ConsumeInvitation(ctx, q.ConsumeInvitationParams{OrgID: org, ID: id, State: state, Cause: cause})
			if e != nil {
				return e
			}
			if n != 1 {
				return app.ErrTokenInvalid
			}
			encoded, e := json.Marshal(receipt)
			if e != nil {
				return e
			}
			return queries.InsertPersonalInvitationReceipt(ctx, q.InsertPersonalInvitationReceiptParams{OrgID: org, OperationID: c.OperationID, ActorID: actor.AccountID, InvitationID: id, Kind: c.Action, RequestHash: digest, Result: encoded})
		})
		if e != nil {
			return e
		}
		if repeated {
			return app.ErrInvitationOperationReused
		}
		out, e = receipt.result()
		return e
	})
	if e != nil {
		return app.RedemptionResult{}, redemptionError(ctx, e)
	}
	if refused != nil {
		return app.RedemptionResult{}, refused
	}
	return out, nil
}
func sha256Sum(v []byte) []byte { h := sha256.Sum256(v); return h[:] }
func (s *RedemptionStore) activate(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, v d.InvitationSnapshot, assignedBy string, now time.Time) error {
	for _, g := range v.Grants {
		id, e := s.NewID()
		if e != nil {
			return app.ErrRegisterUnavailable
		}
		target := scope.OrgID().String()
		if g.Scope == "SELF" {
			target = v.MembershipID
		}
		a, e := permissions.RehydrateAssignment(scope, permissions.AssignmentSnapshot{OrgID: scope.OrgID(), ID: id, MembershipID: v.MembershipID, Role: permissions.Role(g.Role), Scope: permissions.ScopeType(g.Scope), TargetID: target, ValidFrom: now, AssignedBy: assignedBy, Reason: "Invitation accepted and confirmed", Revision: 1})
		if e != nil {
			return e
		}
		if _, e = s.Roles.CreateTx(ctx, tx, scope, a); e != nil {
			return e
		}
	}
	member, e := s.Memberships.GetTx(ctx, tx, scope, v.MembershipID)
	if e != nil {
		return e
	}
	m, e := member.Snapshot(scope)
	if e != nil {
		return e
	}
	employee, e := s.Memberships.GetEmployeeTx(ctx, tx, scope, v.EmployeeID)
	if e != nil {
		return e
	}
	active, e := member.Transition(scope, m.Revision, d.MembershipActive, d.MembershipConditions{Employee: employee, AdmissionConfirmed: true, ConfirmationSatisfied: true, LiveRoleAssignments: int64(len(v.Grants))})
	if e != nil {
		return e
	}
	_, e = s.Memberships.SaveTx(ctx, tx, scope, m.Revision, active)
	return e
}

// admissionErased reports the B-BR-EMP-170 tombstone: account erasure clears
// the account reference and the copied alias together, never only one.
func admissionErased(row q.InvitationAcceptanceRow) (bool, error) {
	erased := row.AccountID == ""
	if erased != (row.AccountAlias == "") {
		return false, app.ErrRegisterUnavailable
	}
	return erased, nil
}
func admissionView(row q.InvitationAcceptanceRow) (app.AdmissionView, error) {
	if !row.AcceptedAt.Valid || !row.Deadline.Valid || !row.Deadline.Time.Equal(row.AcceptedAt.Time.Add(168*time.Hour)) || row.Revision < 1 {
		return app.AdmissionView{}, app.ErrRegisterUnavailable
	}
	if _, e := d.NewAdmission(d.AdmissionState(row.State), row.AcceptedAt.Time); e != nil {
		return app.AdmissionView{}, app.ErrRegisterUnavailable
	}
	erased, e := admissionErased(row)
	if e != nil {
		return app.AdmissionView{}, e
	}
	return app.AdmissionView{InvitationID: row.InvitationID, MembershipID: row.MembershipID, Alias: row.AccountAlias, AccountErased: erased, State: d.AdmissionState(row.State), Revision: row.Revision, AcceptedAt: row.AcceptedAt.Time.UTC(), Deadline: row.Deadline.Time.UTC()}, nil
}
func (s *RedemptionStore) ManageAdmission(ctx context.Context, scope kernel.OrgScope, actor app.EmployeeActor, member, id, action string, revision int64, operation string) (app.AdmissionView, bool, error) {
	if !s.ready() {
		return app.AdmissionView{}, false, app.ErrRegisterUnavailable
	}
	if employeeCall(scope, actor, member) != nil {
		return app.AdmissionView{}, false, app.ErrEmployeeUnauthenticated
	}
	if _, e := kernel.ParseOrgID(id); e != nil {
		return app.AdmissionView{}, false, d.ErrInvalid
	}
	if action != "READ" && action != "CONFIRM" && action != "REJECT" {
		return app.AdmissionView{}, false, d.ErrInvalid
	}
	if action != "READ" && (revision < 1 || !validUUID(operation)) {
		return app.AdmissionView{}, false, d.ErrInvalid
	}
	policy := app.InvitationIssue
	if action == "READ" {
		policy = app.InvitationRead
	} else if action == "REJECT" {
		policy = app.InvitationRevoke
	}

	var out app.AdmissionView
	var refused error
	replay := false
	e := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		if e := s.Authority.CheckPersonalTx(ctx, tx, actor); e != nil {
			return e
		}
		queries := q.New(tx)
		row, e := queries.InvitationAcceptance(ctx, q.InvitationAcceptanceParams{OrgID: scope.OrgID().String(), ID: id})
		if errors.Is(e, pgx.ErrNoRows) {
			// The lookup supplies account lock ordering only. Even a missing
			// target must pass current manager authority before NOT_FOUND.
			if _, authorityErr := s.Managers.CheckInvitationTx(ctx, tx, scope, actor, member, policy); authorityErr != nil {
				return authorityErr
			}
			return app.ErrEmployeeNotFound
		}
		if e != nil {
			return e
		}
		if e := s.Authority.LockRedemptionTx(ctx, tx, scope, actor, id, row.AccountID, action == "CONFIRM"); e != nil {
			return e
		}
		manager, e := s.Managers.CheckInvitationTx(ctx, tx, scope, actor, member, policy)
		if e != nil {
			return e
		}
		raw, _ := json.Marshal([]any{1, actor.AccountID, id, action, revision, operation})
		digest := hex.EncodeToString(sha256Sum(raw))
		if action != "READ" {
			previous, e := queries.PersonalInvitationReceipt(ctx, q.PersonalInvitationReceiptParams{OrgID: scope.OrgID().String(), OperationID: operation})
			if e == nil {
				if previous.ActorID != actor.AccountID || previous.Kind != action || previous.RequestHash != digest {
					return app.ErrInvitationOperationReused
				}
				var r redemptionReceipt
				if httpapi.DecodeJSON(previous.Result, &r) != nil || r.InvitationID != id || r.Revision < 2 {
					return app.ErrRegisterUnavailable
				}
				out = app.AdmissionView{InvitationID: r.InvitationID, MembershipID: r.MembershipID, Alias: r.Alias, State: d.AdmissionState(r.State), Revision: r.Revision}
				if r.AcceptedAt == nil || r.Deadline == nil {
					return app.ErrRegisterUnavailable
				}
				// The immutable receipt keeps its original bytes; once the
				// person's account is erased the replay projects the tombstone.
				erased, e := admissionErased(row)
				if e != nil {
					return e
				}
				if erased {
					out.Alias, out.AccountErased = "", true
				}
				out.AcceptedAt = *r.AcceptedAt
				out.Deadline = *r.Deadline
				replay = true
				return nil
			}
			if !errors.Is(e, pgx.ErrNoRows) {
				return e
			}
		}
		if _, e := tx.Exec(ctx, `UPDATE org_invitation_acceptances SET state='EXPIRED',revision=revision+1,cause='CONFIRMATION_EXPIRED' WHERE org_id=$1 AND invitation_id=$2 AND state='WAITING' AND deadline<=clock_timestamp()`, scope.OrgID().String(), id); e != nil {
			return e
		}
		row, e = queries.InvitationAcceptance(ctx, q.InvitationAcceptanceParams{OrgID: scope.OrgID().String(), ID: id})
		if e != nil {
			return e
		}
		out, e = admissionView(row)
		if e != nil {
			return e
		}
		if action == "READ" {
			return nil
		}
		if row.State != "WAITING" || row.Revision != revision {
			refused = app.ErrInvitationConflict
			return nil
		}
		inv, e := loadInvitation(ctx, tx, scope, id)
		if e != nil {
			return e
		}
		v, e := inv.Snapshot(scope)
		if e != nil {
			return e
		}
		if action == "CONFIRM" {
			f, e := s.Authority.CheckRedemptionTx(ctx, tx, scope, actor, id)
			if e != nil {
				return e
			}
			if !f.Eligible || !canGrantInvitation(scope, v.IssuerMembershipID, v.Grants, f.Assignments, f.Now) || !canGrantInvitation(scope, member, v.Grants, manager.Assignments, f.Now) {
				return app.ErrTokenInvalid
			}
		}
		event := "idn.invitation.confirmed"
		state, cause := "ACTIVE", "MANAGER_CONFIRMED"
		if action == "REJECT" {
			event, state, cause = "idn.invitation.rejected", "REJECTED", "MANAGER_REJECTED"
		}
		repeated, e := s.Audit.RecordChangeTx(ctx, tx, scope, audit.Event{OperationID: operation, ActorID: actor.AccountID, Action: event, TargetType: "invitation", TargetID: id, RequestHash: digest}, func(ctx context.Context, tx pgx.Tx) error {
			if action == "CONFIRM" {
				if e := s.activate(ctx, tx, scope, v, member, manager.Now); e != nil {
					return e
				}
			}
			// Fresh source proof and issuer role periods are checked again after work;
			// SQL performs the final exact deadline check under the retained root lock.
			if e := s.Authority.CheckPersonalTx(ctx, tx, actor); e != nil {
				return e
			}
			if action == "CONFIRM" {
				f, e := s.Authority.CheckRedemptionTx(ctx, tx, scope, actor, id)
				if e != nil {
					return e
				}
				if !f.Eligible || !canGrantInvitation(scope, v.IssuerMembershipID, v.Grants, f.Assignments, f.Now) {
					return app.ErrTokenInvalid
				}
			}
			if action == "CONFIRM" {
				current, e := s.Managers.CheckInvitationTx(ctx, tx, scope, actor, member, app.InvitationIssue)
				if e != nil {
					return e
				}
				if !canGrantInvitation(scope, member, v.Grants, current.Assignments, current.Now) {
					return app.ErrTokenInvalid
				}
			}
			n, e := queries.TransitionInvitationAcceptance(ctx, q.TransitionInvitationAcceptanceParams{OrgID: scope.OrgID().String(), ID: id, State: state, Cause: cause, Revision: revision, ConfirmedBy: member})
			if e != nil {
				return e
			}
			if n != 1 {
				return app.ErrInvitationConflict
			}
			row, e := queries.InvitationAcceptance(ctx, q.InvitationAcceptanceParams{OrgID: scope.OrgID().String(), ID: id})
			if e != nil {
				return e
			}
			out, e = admissionView(row)
			if e != nil {
				return e
			}
			at, deadline := out.AcceptedAt, out.Deadline
			raw, e := json.Marshal(redemptionReceipt{OrgID: scope.OrgID().String(), InvitationID: id, MembershipID: out.MembershipID, State: string(out.State), Alias: out.Alias, Revision: out.Revision, AcceptedAt: &at, Deadline: &deadline})
			if e != nil {
				return e
			}
			return queries.InsertPersonalInvitationReceipt(ctx, q.InsertPersonalInvitationReceiptParams{OrgID: scope.OrgID().String(), OperationID: operation, ActorID: actor.AccountID, InvitationID: id, Kind: action, RequestHash: digest, Result: raw})
		})
		if e != nil {
			return e
		}
		if repeated {
			return app.ErrInvitationOperationReused
		}
		return nil
	})
	if e != nil {
		return app.AdmissionView{}, false, redemptionError(ctx, e)
	}
	if refused != nil {
		return app.AdmissionView{}, false, refused
	}
	return out, replay, nil
}
func validUUID(id string) bool { p, e := kernel.ParseOrgID(id); return e == nil && p.String() == id }
