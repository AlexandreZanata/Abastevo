//go:build integration

package adapters_test

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"errors"
	memberpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/adapters"
	members "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/application"
	memberd "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/domain"
	orgpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters"
	auditpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/adapters"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	"github.com/jackc/pgx/v5"
	"strings"
	"testing"
	"time"
)

type invitationFixtureState struct {
	legalFixtureState
	invites *memberpg.InvitationStore
}

func invitationFixture(t *testing.T) invitationFixtureState {
	t.Helper()
	fx := legalFixture(t)
	grantEmployeeTables(t, fx)
	if _, _, e := fx.onboarding.Repository.ActivateOrganization(t.Context(), fx.scope, fx.actor, fx.member, onboardingID(t), 1); e != nil {
		t.Fatal("activate invitation fixture", e)
	}
	catalog, e := audit.NewCatalog(memberpg.InvitationAuditDefinitions())
	if e != nil {
		t.Fatal(e)
	}
	settings := &orgpg.SettingsStore{Pool: fx.f.Runtime}
	store := &memberpg.InvitationStore{Pool: fx.f.Runtime, Authority: &orgpg.InvitationAuthority{Memberships: fx.store.Memberships, Roles: fx.store.Roles, Capabilities: fx.flags, Settings: settings}, Memberships: fx.store.Memberships, Audit: auditpg.NewStore(fx.f.Runtime, catalog)}
	return invitationFixtureState{fx, store}
}
func invitationEmployee(t *testing.T, fx invitationFixtureState) string {
	t.Helper()
	id := onboardingID(t)
	if e := unitSchemaExec(t, fx.legalFixtureState, `INSERT INTO org_employee_identities(org_id,id,state)VALUES($1,$2,'ACTIVE')`, fx.scope.OrgID().String(), id); e != nil {
		t.Fatal(e)
	}
	return id
}
func invitationPlan(t *testing.T, fx invitationFixtureState) (members.InvitationPlan, string) {
	t.Helper()
	factory, e := memberpg.InvitationSecretFactory(bytes.Repeat([]byte{47}, 32))
	if e != nil {
		t.Fatal(e)
	}
	id := onboardingID(t)
	secret, e := factory(fx.scope, id)
	if e != nil {
		t.Fatal(e)
	}
	return members.InvitationPlan{InvitationID: id, MembershipID: onboardingID(t), Verifier: secret.Verifier}, secret.Code
}
func inviteWrite(t *testing.T, fx invitationFixtureState, c members.InvitationWrite) (memberd.Invitation, bool, string, error) {
	t.Helper()
	var plan members.InvitationPlan
	code := ""
	if c.Action != members.InvitationRevoke {
		plan, code = invitationPlan(t, fx)
	}
	v, r, e := fx.invites.WriteInvitation(t.Context(), fx.scope, members.EmployeeActor{AccountID: fx.actor.AccountID, SessionID: fx.actor.SessionID}, fx.member, c, plan)
	if e != nil || r {
		code = ""
	}
	return v, r, code, e
}
func TestInvitationIssueResendRevokeReceiptsAndSecrecy(t *testing.T) {
	fx := invitationFixture(t)
	employee := invitationEmployee(t, fx)
	c := members.InvitationWrite{Action: members.InvitationIssue, OperationID: onboardingID(t), EmployeeID: employee, Grants: []memberd.InvitationGrant{{Role: "DRIVER", Scope: "SELF"}}}
	first, replay, code, e := inviteWrite(t, fx, c)
	if e != nil || replay || code == "" {
		t.Fatal("issue", e)
	}
	v, e := first.Snapshot(fx.scope)
	if e != nil || v.State != memberd.InvitationPending || v.ValidityDays != 7 {
		t.Fatal("pending metadata", e)
	}
	again, r, secret, e := inviteWrite(t, fx, c)
	if e != nil || !r || secret != "" {
		t.Fatal("metadata-only retry", e)
	}
	a, _ := again.Snapshot(fx.scope)
	if a.ID != v.ID || a.State != v.State {
		t.Fatal("receipt changed")
	}
	// No role or account is provisioned by issuance.
	invitationCount(t, fx, `SELECT count(*) FROM org_role_assignments WHERE org_id=$1 AND membership_id=$2`, 0, fx.scope.OrgID().String(), v.MembershipID)
	var account *string
	var state string
	if e := fx.f.Admin.QueryRow(t.Context(), `SELECT account_id::text,state FROM org_memberships WHERE org_id=$1 AND id=$2`, fx.scope.OrgID().String(), v.MembershipID).Scan(&account, &state); e != nil || account != nil || state != "INVITED" {
		t.Fatal("invitation granted access", e)
	}
	resend := members.InvitationWrite{Action: members.InvitationResend, InvitationID: v.ID, ExpectedRevision: 1, OperationID: onboardingID(t)}
	replaced, _, code2, e := inviteWrite(t, fx, resend)
	if e != nil || code2 == code || code2 == "" {
		t.Fatal("resend", e)
	}
	next, _ := replaced.Snapshot(fx.scope)
	if next.MembershipID != v.MembershipID {
		t.Fatal("resend ended reservation")
	}
	old, e := fx.invites.ListInvitations(t.Context(), fx.scope, members.EmployeeActor{AccountID: fx.actor.AccountID, SessionID: fx.actor.SessionID}, fx.member, members.InvitationFilter{ID: v.ID, Limit: 1})
	if e != nil {
		t.Fatal(e)
	}
	oldValue, _ := old.Invitations[0].Snapshot(fx.scope)
	if oldValue.State != memberd.InvitationSuperseded {
		t.Fatal("old code usable")
	}
	revoke := members.InvitationWrite{Action: members.InvitationRevoke, InvitationID: next.ID, ExpectedRevision: 1, OperationID: onboardingID(t)}
	revoked, _, _, e := inviteWrite(t, fx, revoke)
	if e != nil {
		t.Fatal("revoke", e)
	}
	rv, _ := revoked.Snapshot(fx.scope)
	if rv.State != memberd.InvitationRevoked {
		t.Fatal("not revoked")
	}
	if e := fx.f.Admin.QueryRow(t.Context(), `SELECT state FROM org_memberships WHERE org_id=$1 AND id=$2`, fx.scope.OrgID().String(), v.MembershipID).Scan(&state); e != nil || state != "ENDED" {
		t.Fatal("final reservation survived", e)
	}
	_, r, secret, e = inviteWrite(t, fx, revoke)
	if e != nil || !r || secret != "" {
		t.Fatal("terminal replay", e)
	}
	_, _, _, e = inviteWrite(t, fx, members.InvitationWrite{Action: members.InvitationResend, InvitationID: v.ID, ExpectedRevision: 1, OperationID: onboardingID(t)})
	if !errors.Is(e, members.ErrInvitationConflict) {
		t.Fatal("superseded resend", e)
	}
	// Entire business tables/audit must contain neither code nor its secret part.
	var dump string
	if e := fx.f.Admin.QueryRow(t.Context(), `SELECT jsonb_build_array((SELECT jsonb_agg(to_jsonb(x))FROM org_invitations x),(SELECT jsonb_agg(to_jsonb(x))FROM org_invitation_commands x),(SELECT jsonb_agg(to_jsonb(x))FROM org_invitation_notices x),(SELECT jsonb_agg(to_jsonb(x))FROM org_audit_events x))::text`).Scan(&dump); e != nil {
		t.Fatal(e)
	}
	for _, token := range []string{code, code2} {
		parts := strings.Split(token, ".")
		raw, e := base64.RawURLEncoding.DecodeString(parts[2])
		if e != nil {
			t.Fatal(e)
		}
		if strings.Contains(dump, token) || strings.Contains(dump, parts[2]) || strings.Contains(dump, hex.EncodeToString(raw)) {
			t.Fatal("recoverable secret persisted")
		}
	}
}

func TestInvitationCapturesProspectiveSettingsValidity(t *testing.T) {
	fx := invitationFixture(t)
	if _, e := fx.f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT ON org_settings_commands TO "+pgx.Identifier{fx.f.RuntimeRole}.Sanitize()); e != nil {
		t.Fatal(e)
	}
	employee := invitationEmployee(t, fx)
	c := members.InvitationWrite{Action: members.InvitationIssue, OperationID: onboardingID(t), EmployeeID: employee, Grants: []memberd.InvitationGrant{{Role: "DRIVER", Scope: "SELF"}}}
	first, _, _, e := inviteWrite(t, fx, c)
	if e != nil {
		t.Fatal(e)
	}
	old, _ := first.Snapshot(fx.scope)
	catalog, e := audit.NewCatalog(orgpg.SettingsAuditDefinitions())
	if e != nil {
		t.Fatal(e)
	}
	settings := &orgpg.SettingsStore{Pool: fx.f.Runtime, Memberships: fx.store.Memberships, Roles: fx.store.Roles, Audit: auditpg.NewStore(fx.f.Runtime, catalog), Capabilities: fx.flags}
	read, e := settings.ReadSettings(t.Context(), fx.scope, fx.actor, fx.member, 0)
	if e != nil {
		t.Fatal(e)
	}
	source, e := read.Settings.Snapshot(fx.scope)
	if e != nil {
		t.Fatal(e)
	}
	values := source.Values
	values.Policies.InvitationValidityDays = 30
	if _, _, e := settings.ChangeSettings(t.Context(), fx.scope, fx.actor, fx.member, settingsCommand(t, source.Revision, values, "")); e != nil {
		t.Fatal(e)
	}
	repeated, r, code, e := inviteWrite(t, fx, c)
	if e != nil || !r || code != "" {
		t.Fatal("settings changed retry", e)
	}
	receipt, _ := repeated.Snapshot(fx.scope)
	if receipt.ValidityDays != 7 || receipt.SettingsRevision != old.SettingsRevision || !receipt.ExpiresAt.Equal(old.ExpiresAt) {
		t.Fatal("new policy rewrote old invitation receipt")
	}
	next, _, _, e := inviteWrite(t, fx, members.InvitationWrite{Action: members.InvitationResend, OperationID: onboardingID(t), InvitationID: old.ID, ExpectedRevision: 1})
	if e != nil {
		t.Fatal(e)
	}
	v, _ := next.Snapshot(fx.scope)
	if v.ValidityDays != 30 || v.SettingsRevision != source.Revision+1 || v.ExpiresAt.Sub(v.CreatedAt) != 30*24*time.Hour {
		t.Fatal("resend did not capture current server validity")
	}
}
