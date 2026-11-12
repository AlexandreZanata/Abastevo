//go:build integration

package adapters_test

import (
	"bytes"
	"errors"
	accountpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/adapters"
	account "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/application"
	accountd "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
	companypg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/adapters"
	company "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/application"
	memberpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/adapters"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/domain"
	orgpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters"
	auditpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/adapters"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"testing"
	"time"
)

func redemptionStore(t *testing.T, fx invitationFixtureState) *memberpg.RedemptionStore {
	t.Helper()
	catalog, e := audit.NewCatalog(memberpg.RedemptionAuditDefinitions())
	if e != nil {
		t.Fatal(e)
	}
	return &memberpg.RedemptionStore{Pool: fx.f.Runtime, Authority: &orgpg.RedemptionAuthority{Roles: fx.store.Roles, Capabilities: fx.flags, Settings: &orgpg.SettingsStore{Pool: fx.f.Runtime}}, Managers: fx.invites.Authority, Memberships: fx.store.Memberships, Roles: fx.store.Roles, Audit: auditpg.NewStore(fx.f.Runtime, catalog), Secret: bytes.Repeat([]byte{47}, 32), NewID: func() (string, error) { return onboardingID(t), nil }}
}
func redemptionActor(t *testing.T, fx invitationFixtureState) app.EmployeeActor {
	t.Helper()

	accounts := &account.Service{Clock: onboardingClock{time.Now().UTC().Truncate(time.Second)}, Store: accountpg.NewPGStore(fx.f.Admin), Hasher: accountd.SHA256Hasher{}, IDGen: jobs.NewUUIDv4, TokenGen: accountd.GenerateToken, KeyGen: accountd.GenerateAccountKey}
	key, e := accounts.CreateKeyAccount(t.Context(), "syn"+onboardingID(t)[:8])
	if e != nil {
		t.Fatal(e)
	}
	auth, e := accounts.LoginWithKey(t.Context(), key.Key)
	if e != nil {
		t.Fatal(e)
	}
	service := &company.Service{Enabled: true, Store: &companypg.Store{Pool: fx.f.Runtime}, Hasher: accountd.SHA256Hasher{}, Prove: companypg.AccountProof(accounts), NewSecret: accountd.GenerateToken, Now: time.Now, NewID: jobs.NewUUIDv4}
	cookie, e := service.Login(t.Context(), auth.Session.FamilyID, auth.Session.AccessToken, "")
	if e != nil {
		t.Fatal(e)
	}
	return app.EmployeeActor{AccountID: cookie.Row.AccountID, SessionID: cookie.Row.ID}
}
func redeemCode(t *testing.T, fx invitationFixtureState, role string) (d.InvitationSnapshot, string) {
	t.Helper()
	c := app.InvitationWrite{Action: app.InvitationIssue, OperationID: onboardingID(t), EmployeeID: invitationEmployee(t, fx), Grants: []d.InvitationGrant{{Role: role, Scope: "SELF"}}}
	value, _, code, e := inviteWrite(t, fx, c)
	if e != nil {
		t.Fatal("issue", e)
	}
	v, e := value.Snapshot(fx.scope)
	if e != nil {
		t.Fatal(e)
	}
	return v, code
}
func TestRedemptionRealPersonalConsentActivationAndImmutableReplay(t *testing.T) {
	fx := invitationFixture(t)
	s := redemptionStore(t, fx)
	actor := redemptionActor(t, fx)
	v, code := redeemCode(t, fx, "DRIVER")
	unsafe := *s
	unsafe.Pool = fx.f.Admin
	if out, e := unsafe.Redeem(t.Context(), actor, app.RedemptionCommand{Action: "PREVIEW", Code: code}); e == nil || out.OrgID != "" {
		t.Fatal("unsafe runtime bootstrap", e)
	}
	preview, e := s.Redeem(t.Context(), actor, app.RedemptionCommand{Action: "PREVIEW", Code: code})
	if e != nil || preview.ConsentDigest == "" || preview.ConfirmationRequired {
		t.Fatal("preview", e)
	}
	c := app.RedemptionCommand{Action: "ACCEPT", Code: code, OperationID: onboardingID(t), Consent: true, ConsentDigest: preview.ConsentDigest}
	accepted, e := s.Redeem(t.Context(), actor, c)
	if e != nil || accepted.State != "ACCEPTED" || accepted.MembershipState != d.MembershipActive {
		t.Fatal("accept", e)
	}
	invitationCount(t, fx, `SELECT count(*) FROM org_memberships WHERE id=$1 AND state='ACTIVE' AND account_id=$2`, 1, v.MembershipID, actor.AccountID)
	invitationCount(t, fx, `SELECT count(*) FROM org_role_assignments WHERE membership_id=$1 AND role='DRIVER' AND scope_target=$1`, 1, v.MembershipID)
	again, e := s.Redeem(t.Context(), actor, c)
	if e != nil || !again.Replayed || again.MembershipState != accepted.MembershipState || !again.AcceptedAt.Equal(accepted.AcceptedAt) {
		t.Fatal("replay", e)
	}
	c.OperationID = onboardingID(t)
	if _, e := s.Redeem(t.Context(), actor, c); !errors.Is(e, app.ErrTokenInvalid) {
		t.Fatal("consumed token", e)
	}
	c.OperationID = onboardingID(t)
	other := redemptionActor(t, fx)
	if _, e := s.Redeem(t.Context(), other, c); !errors.Is(e, app.ErrTokenInvalid) {
		t.Fatal("other account", e)
	}
	if accepted.ConfirmationDeadline.Sub(accepted.AcceptedAt) != 168*time.Hour {
		t.Fatal("deadline")
	}
}

func TestRedemptionPrivilegedConfirmationRejectionAndScope(t *testing.T) {
	fx := invitationFixture(t)
	s := redemptionStore(t, fx)
	actor := redemptionActor(t, fx)
	v, code := redeemCode(t, fx, "AUDITOR")
	p, e := s.Redeem(t.Context(), actor, app.RedemptionCommand{Action: "PREVIEW", Code: code})
	if e != nil || !p.ConfirmationRequired {
		t.Fatal("privileged preview", e)
	}
	c := app.RedemptionCommand{Action: "ACCEPT", Code: code, OperationID: onboardingID(t), Consent: true, ConsentDigest: p.ConsentDigest}
	got, e := s.Redeem(t.Context(), actor, c)
	if e != nil || got.MembershipState != d.MembershipInvited {
		t.Fatal("waiting", e)
	}
	invitationCount(t, fx, `SELECT count(*) FROM org_role_assignments WHERE membership_id=$1`, 0, v.MembershipID)
	manager := app.EmployeeActor{AccountID: fx.actor.AccountID, SessionID: fx.actor.SessionID}
	read, _, e := s.ManageAdmission(t.Context(), fx.scope, manager, fx.member, v.ID, "READ", 0, "")
	if e != nil || read.State != d.AdmissionWaiting || read.Alias == "" {
		t.Fatal("manager read", e)
	}
	operation := onboardingID(t)
	confirmed, replay, e := s.ManageAdmission(t.Context(), fx.scope, manager, fx.member, v.ID, "CONFIRM", 1, operation)
	if e != nil || replay || confirmed.State != d.AdmissionActive {
		t.Fatal("confirm", e)
	}
	again, replay, e := s.ManageAdmission(t.Context(), fx.scope, manager, fx.member, v.ID, "CONFIRM", 1, operation)
	if e != nil || !replay || again.Revision != confirmed.Revision {
		t.Fatal("confirm replay", e)
	}
	invitationCount(t, fx, `SELECT count(*) FROM org_role_assignments WHERE membership_id=$1 AND role='AUDITOR'`, 1, v.MembershipID)
	other := redemptionActor(t, fx)
	_, _, knownErr := s.ManageAdmission(t.Context(), fx.scope, other, fx.member, v.ID, "READ", 0, "")
	_, _, missingErr := s.ManageAdmission(t.Context(), fx.scope, other, fx.member, onboardingID(t), "READ", 0, "")
	if knownErr == nil || missingErr == nil || !errors.Is(knownErr, missingErr) {
		t.Fatal("foreign caller manager existence oracle", knownErr, missingErr)
	}
	v2, code2 := redeemCode(t, fx, "AUDITOR")
	actor2 := redemptionActor(t, fx)
	p, e = s.Redeem(t.Context(), actor2, app.RedemptionCommand{Action: "PREVIEW", Code: code2})
	if e != nil {
		t.Fatal(e)
	}
	_, e = s.Redeem(t.Context(), actor2, app.RedemptionCommand{Action: "ACCEPT", Code: code2, OperationID: onboardingID(t), Consent: true, ConsentDigest: p.ConsentDigest})
	if e != nil {
		t.Fatal(e)
	}
	rejected, _, e := s.ManageAdmission(t.Context(), fx.scope, manager, fx.member, v2.ID, "REJECT", 1, onboardingID(t))
	if e != nil || rejected.State != d.AdmissionRejected {
		t.Fatal("reject", e)
	}
	invitationCount(t, fx, `SELECT count(*) FROM org_memberships WHERE id=$1 AND state='ENDED'`, 1, v2.MembershipID)
	invitationCount(t, fx, `SELECT count(*) FROM org_role_assignments WHERE membership_id=$1`, 0, v2.MembershipID)
}

func managedRedemptionActor(t *testing.T, fx invitationFixtureState) app.EmployeeActor {
	t.Helper()
	id := onboardingID(t)
	tx, e := fx.f.Admin.Begin(t.Context())
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(t.Context())
	if _, e = tx.Exec(t.Context(), `SELECT set_config('anpfuel.org_id',$1,true)`, fx.scope.OrgID().String()); e != nil {
		t.Fatal(e)
	}
	if _, e = tx.Exec(t.Context(), `INSERT INTO accounts(id,alias,status,enterprise_managed_org_id)VALUES($1,'synthetic-managed-recipient','active',$2)`, id, fx.scope.OrgID().String()); e != nil {
		t.Fatal(e)
	}
	if e = tx.Commit(t.Context()); e != nil {
		t.Fatal(e)
	}
	now := time.Now().UTC().Truncate(time.Second)
	hasher := accountd.SHA256Hasher{}
	salt, e := hasher.NewSalt()
	if e != nil {
		t.Fatal(e)
	}
	family := onboardingID(t)
	token := "synthetic-managed-token"
	accounts := &account.Service{Clock: onboardingClock{now}, Store: accountpg.NewPGStore(fx.f.Admin), Hasher: hasher}
	if e := accounts.Store.CreateFamily(t.Context(), accountd.SessionFamily{ID: family, AccountID: id, RefreshSalt: salt, RefreshHash: hasher.Hash(salt, "synthetic-refresh"), AccessSalt: salt, AccessHash: hasher.Hash(salt, token), AccessExpires: now.Add(15 * time.Minute).Unix(), IssuedAt: now.Unix()}); e != nil {
		t.Fatal(e)
	}
	service := &company.Service{Enabled: true, Store: &companypg.Store{Pool: fx.f.Runtime}, Hasher: hasher, Prove: companypg.AccountProof(accounts), Now: func() time.Time { return now }, NewID: jobs.NewUUIDv4, NewSecret: accountd.GenerateToken}
	cookie, e := service.Login(t.Context(), family, token, "")
	if e != nil {
		t.Fatal(e)
	}
	return app.EmployeeActor{AccountID: id, SessionID: cookie.Row.ID}
}
