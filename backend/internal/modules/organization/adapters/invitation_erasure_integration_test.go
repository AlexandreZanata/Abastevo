//go:build integration

package adapters_test

import (
	"context"
	"errors"
	"testing"
	"time"

	accountpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/adapters"
	accountapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/application"
	memberpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/adapters"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/domain"
	orgapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	paypg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/adapters"
	payapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// invitationEraser moves the existing account-erasure definer to an ordinary
// NOSUPERUSER/NOBYPASSRLS owner, so every tombstone and deferred cleanup below
// proves forced-RLS organization scope instead of a superuser bypass.
func invitationEraser(t *testing.T, f *orgtest.Fixture) string {
	t.Helper()
	eraser := f.RuntimeRole + "_invitation_eraser"
	role := pgx.Identifier{eraser}.Sanitize()
	if _, e := f.Admin.Exec(t.Context(), "CREATE ROLE "+role+" NOLOGIN NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE"); e != nil {
		t.Fatal(e)
	}
	invitationEraserOwns(t, f, eraser)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, e := f.Admin.Exec(ctx, "REASSIGN OWNED BY "+role+" TO CURRENT_USER;DROP OWNED BY "+role+";DROP ROLE "+role); e != nil {
			t.Error(e)
		}
	})
	// Personal erasure's pre-existing private tables use fixture DML rights.
	if _, e := f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT,UPDATE,DELETE ON ALL TABLES IN SCHEMA public TO "+pgx.Identifier{f.RuntimeRole}.Sanitize()); e != nil {
		t.Fatal(e)
	}
	return eraser
}

// invitationEraserOwns gives an existing ordinary role the erasure definer's
// ownership and exactly the scoped projections the erasure cascade writes.
func invitationEraserOwns(t *testing.T, f *orgtest.Fixture, name string) {
	t.Helper()
	role := pgx.Identifier{name}.Sanitize()
	if _, e := f.Admin.Exec(t.Context(), "GRANT USAGE ON SCHEMA public TO "+role+
		";GRANT SELECT,UPDATE ON organizations,accounts,org_role_assignments,org_teams,org_team_links,org_device_enrollments,org_ownership_transfers TO "+role+
		";GRANT SELECT ON org_employee_identities,payment_billing_exits TO "+role+
		";GRANT SELECT,INSERT ON org_settings_versions,org_settings_designations,org_settings_notices,org_ownership_notices TO "+role+
		";GRANT SELECT,INSERT,UPDATE ON org_settings_heads TO "+role+
		";GRANT SELECT,UPDATE ON org_invitations,org_invitation_acceptances TO "+role+
		";GRANT SELECT,INSERT ON org_invitation_notices,org_invitation_acceptance_notices TO "+role+
		";ALTER TABLE org_memberships OWNER TO "+role+";ALTER FUNCTION org_membership_account_erasure() OWNER TO "+role+";ALTER FUNCTION org_membership_check_account_exit(uuid) OWNER TO "+role); e != nil {
		t.Fatal(e)
	}
}

// invitationDeleteAccount runs the existing personal deletion service on the
// ordinary runtime pool: billing exit fence, account transaction and erasure.
func invitationDeleteAccount(t *testing.T, f *orgtest.Fixture, accountID string) error {
	t.Helper()
	exit := &payapp.BillingExitService{Store: paypg.NewStore(f.Runtime)}
	service := &accountapp.Service{Store: accountpg.NewPGStore(f.Runtime), Clock: onboardingClock{time.Now().UTC().Truncate(time.Second)}, RequestBillingExit: exit.RequestAccountExit, RequestErasure: func(context.Context, string, []string) error { return nil }}
	_, e := service.DeleteAccount(t.Context(), accountID)
	return e
}

func invitationAccept(t *testing.T, fx invitationFixtureState, s *memberpg.RedemptionStore, actor app.EmployeeActor, role string) d.InvitationSnapshot {
	t.Helper()
	v, code := redeemCode(t, fx, role)
	p, e := s.Redeem(t.Context(), actor, app.RedemptionCommand{Action: "PREVIEW", Code: code})
	if e != nil {
		t.Fatal("preview", e)
	}
	if _, e = s.Redeem(t.Context(), actor, app.RedemptionCommand{Action: "ACCEPT", Code: code, OperationID: onboardingID(t), Consent: true, ConsentDigest: p.ConsentDigest}); e != nil {
		t.Fatal("accept", e)
	}
	return v
}

func invitationSQLCode(t *testing.T, e error, want string) {
	t.Helper()
	var pe *pgconn.PgError
	if !errors.As(e, &pe) || pe.Code != want {
		t.Fatalf("SQL refusal mismatch, want %s: %v", want, e)
	}
}

// BR-IDN-042/BR-PRIV-014, B-BR-EMP-170: the existing account deletion path
// unlinks every invitation acceptance with its membership, in each organization
// only, and manager reads/replays project a tombstone instead of the alias.
func TestInvitationAcceptanceErasureTombstonesAliasAcrossOrganizations(t *testing.T) {
	fx := invitationFixture(t)
	s := redemptionStore(t, fx)
	manager := invitationActor(fx)
	person := redemptionActor(t, fx)
	survivor := redemptionActor(t, fx)
	confirmed := redemptionActor(t, fx)
	waiting := invitationAccept(t, fx, s, person, "AUDITOR")
	untouched := invitationAccept(t, fx, s, survivor, "AUDITOR")
	manual := invitationAccept(t, fx, s, confirmed, "AUDITOR")
	confirmOperation := onboardingID(t)
	before, _, e := s.ManageAdmission(t.Context(), fx.scope, manager, fx.member, manual.ID, "CONFIRM", 1, confirmOperation)
	if e != nil || before.Alias == "" {
		t.Fatal("manager confirmation", e)
	}
	// A second organization: the same person holds an ACTIVE auto-confirmed
	// admission there. Erasure must reach it under that organization's scope.
	next := legalFixtureFor(t, fx.f, fx.onboarding, fx.actor, fx.flags)
	other := invitationFixtureState{legalFixtureState: next, invites: fx.invites}
	active := invitationAccept(t, other, s, person, "DRIVER")
	otherSurvivor := invitationAccept(t, other, s, survivor, "DRIVER")
	eraser := invitationEraser(t, fx.f)
	if e := invitationDeleteAccount(t, fx.f, person.AccountID); e != nil {
		t.Fatal("existing account deletion", e)
	}
	if e := invitationDeleteAccount(t, fx.f, confirmed.AccountID); e != nil {
		t.Fatal("existing account deletion after confirmation", e)
	}
	for _, tc := range []struct {
		fx    invitationFixtureState
		id    string
		state d.AdmissionState
	}{{fx, waiting.ID, d.AdmissionCancelled}, {other, active.ID, d.AdmissionActive}, {fx, manual.ID, d.AdmissionActive}} {
		read, _, e := s.ManageAdmission(t.Context(), tc.fx.scope, invitationActor(tc.fx), tc.fx.member, tc.id, "READ", 0, "")
		if e != nil || read.State != tc.state || read.Alias != "" || !read.AccountErased {
			t.Fatal("erased acceptance still discloses the account alias", e)
		}
		invitationCount(t, fx, `SELECT count(*) FROM org_invitation_acceptances a JOIN org_memberships m ON(m.org_id,m.id)=(a.org_id,a.membership_id) WHERE a.invitation_id=$1 AND a.account_id IS NULL AND a.account_alias='' AND m.account_id IS NULL AND m.account_tombstone IS NOT NULL AND m.state='ENDED'`, 1, tc.id)
	}
	// The manager's own original confirmation receipt replays without the alias.
	replayed, replay, e := s.ManageAdmission(t.Context(), fx.scope, manager, fx.member, manual.ID, "CONFIRM", 1, confirmOperation)
	if e != nil || !replay || replayed.Alias != "" || !replayed.AccountErased || before.AccountErased || replayed.Revision != before.Revision {
		t.Fatal("manager receipt replay discloses an erased alias", e)
	}
	for _, tc := range []struct {
		fx invitationFixtureState
		id string
	}{{fx, untouched.ID}, {other, otherSurvivor.ID}} {
		read, _, e := s.ManageAdmission(t.Context(), tc.fx.scope, invitationActor(tc.fx), tc.fx.member, tc.id, "READ", 0, "")
		if e != nil || read.Alias == "" || read.AccountErased {
			t.Fatal("unrelated acceptance lost its live account", e)
		}
		invitationCount(t, fx, `SELECT count(*) FROM org_invitation_acceptances WHERE invitation_id=$1 AND account_id=$2`, 1, tc.id, survivor.AccountID)
	}
	// Ordinary runtime cannot use the tombstone, even with the erasure GUC.
	org := fx.scope.OrgID().String()
	tombstone := `UPDATE org_invitation_acceptances SET account_id=NULL,account_alias='' WHERE org_id=$1 AND invitation_id=$2`
	for _, spoof := range []string{"", survivor.AccountID} {
		e := pgx.BeginFunc(t.Context(), fx.f.Runtime, func(tx pgx.Tx) error {
			if _, e := tx.Exec(t.Context(), `SELECT set_config('anpfuel.org_id',$1,true),set_config('anpfuel.erasure_account',$2,true)`, org, spoof); e != nil {
				return e
			}
			_, e := tx.Exec(t.Context(), tombstone, org, untouched.ID)
			return e
		})
		if e == nil {
			t.Fatal("runtime tombstoned a live acceptance", spoof != "")
		}
	}
	// The owner's branch is exact: a live account, any other column, a
	// foreign organization or a different account link is refused.
	refusal := func(prepare []string, statement string, want string) {
		t.Helper()
		e := pgx.BeginFunc(t.Context(), fx.f.Admin, func(tx pgx.Tx) error {
			for _, sql := range prepare {
				if _, e := tx.Exec(t.Context(), sql); e != nil {
					return e
				}
			}
			if _, e := tx.Exec(t.Context(), `SELECT set_config('anpfuel.org_id',$1,true),set_config('anpfuel.erasure_account',$2,true)`, org, survivor.AccountID); e != nil {
				return e
			}
			if _, e := tx.Exec(t.Context(), "SET LOCAL ROLE "+pgx.Identifier{eraser}.Sanitize()); e != nil {
				return e
			}
			_, e := tx.Exec(t.Context(), statement, org, untouched.ID)
			if e == nil {
				return errors.New("owner tombstone branch accepted a forbidden change")
			}
			return e
		})
		invitationSQLCode(t, e, want)
	}
	refusal(nil, tombstone, "23514")
	deleted := []string{`ALTER TABLE accounts DISABLE TRIGGER org_membership_account_erasure`, `UPDATE accounts SET status='deleted' WHERE id='` + survivor.AccountID + `'`}
	for _, statement := range []string{
		`UPDATE org_invitation_acceptances SET account_id=NULL,account_alias='',state='ENDED' WHERE org_id=$1 AND invitation_id=$2`,
		`UPDATE org_invitation_acceptances SET account_id=NULL WHERE org_id=$1 AND invitation_id=$2`,
		`UPDATE org_invitation_acceptances SET account_alias='' WHERE org_id=$1 AND invitation_id=$2`,
		`UPDATE org_invitation_acceptances SET account_id=NULL,account_alias='',revision=revision+1 WHERE org_id=$1 AND invitation_id=$2`,
		`UPDATE org_invitation_acceptances SET account_id='` + manager.AccountID + `',account_alias='synthetic' WHERE org_id=$1 AND invitation_id=$2`,
		`UPDATE org_invitation_acceptances SET account_id=NULL,account_alias='',changed_at=changed_at+interval '1 second' WHERE org_id=$1 AND invitation_id=$2`,
	} {
		refusal(deleted, statement, "23514")
	}
	// The exact tombstone is accepted for the erased link, proving the branch
	// is narrow rather than closed, then the transaction is discarded.
	e = pgx.BeginFunc(t.Context(), fx.f.Admin, func(tx pgx.Tx) error {
		for _, sql := range deleted {
			if _, e := tx.Exec(t.Context(), sql); e != nil {
				return e
			}
		}
		if _, e := tx.Exec(t.Context(), `SELECT set_config('anpfuel.org_id',$1,true),set_config('anpfuel.erasure_account',$2,true)`, org, survivor.AccountID); e != nil {
			return e
		}
		if _, e := tx.Exec(t.Context(), "SET LOCAL ROLE "+pgx.Identifier{eraser}.Sanitize()); e != nil {
			return e
		}
		tag, e := tx.Exec(t.Context(), tombstone, org, untouched.ID)
		if e != nil || tag.RowsAffected() != 1 {
			t.Fatal("exact owner tombstone refused", e)
		}
		return errors.New("rollback")
	})
	if e == nil || e.Error() != "rollback" {
		t.Fatal(e)
	}
	invitationCount(t, fx, `SELECT count(*) FROM org_invitation_acceptances WHERE invitation_id=$1 AND account_id=$2`, 1, untouched.ID, survivor.AccountID)
	if _, e := fx.f.Admin.Exec(t.Context(), `DELETE FROM org_invitation_acceptances WHERE invitation_id=$1`, untouched.ID); e == nil {
		t.Fatal("acceptance history deleted")
	}
}

// L1: account erasure revokes the pending invitations the deleted member issued;
// the deferred reservation cleanup must still end the invitee's INVITED period
// after the erasure definer restored the caller's empty organization scope.
func TestInvitationIssuerErasureEndsPendingReservationAfterScopeRestore(t *testing.T) {
	fx := invitationFixture(t)
	invitationEraser(t, fx.f)
	tf := transferFixtureState{legalFixtureState: fx.legalFixtureState, actors: map[string]orgapp.CompanyActor{}}
	issuer, issuerAccount := createTransferMember(t, tf, false)
	issuerActor := app.EmployeeActor{AccountID: tf.actors[issuer].AccountID, SessionID: tf.actors[issuer].SessionID}
	plan, _ := invitationPlan(t, fx)
	value, _, e := fx.invites.WriteInvitation(t.Context(), fx.scope, issuerActor, issuer, app.InvitationWrite{Action: app.InvitationIssue, OperationID: onboardingID(t), EmployeeID: invitationEmployee(t, fx), Grants: []d.InvitationGrant{{Role: "DRIVER", Scope: "SELF"}}}, plan)
	if e != nil {
		t.Fatal("non-owner manager issue", e)
	}
	v, e := value.Snapshot(fx.scope)
	if e != nil {
		t.Fatal(e)
	}
	invitationCount(t, fx, `SELECT count(*) FROM org_memberships WHERE id=$1 AND state='INVITED' AND account_id IS NULL`, 1, v.MembershipID)
	if e := invitationDeleteAccount(t, fx.f, issuerAccount); e != nil {
		t.Fatal("issuer account deletion", e)
	}
	invitationCount(t, fx, `SELECT count(*) FROM org_invitations WHERE id=$1 AND state='REVOKED' AND cause='ISSUER_REDUCED'`, 1, v.ID)
	invitationCount(t, fx, `SELECT count(*) FROM org_memberships WHERE id=$1 AND state='ENDED' AND end_reason='ADMISSION_ENDED'`, 1, v.MembershipID)
	var scope string
	if e := fx.f.Runtime.QueryRow(t.Context(), `SELECT COALESCE(current_setting('anpfuel.org_id',true),'')`).Scan(&scope); e != nil || scope != "" {
		t.Fatal("deferred cleanup leaked organization scope", e)
	}
}

// BUC-EMP-89: personal deletion racing a manager confirmation of the same
// WAITING admission serializes on the account-before-root order. Either order
// ends with the acceptance and membership unlinked, no role left and no
// deadlock or partial tombstone.
func TestInvitationAcceptanceErasureRacesManagerConfirmation(t *testing.T) {
	fx := invitationFixture(t)
	s := redemptionStore(t, fx)
	manager := invitationActor(fx)
	type admission struct {
		v      d.InvitationSnapshot
		person app.EmployeeActor
	}
	var admissions []admission
	for range 3 {
		person := redemptionActor(t, fx)
		admissions = append(admissions, admission{invitationAccept(t, fx, s, person, "AUDITOR"), person})
	}
	invitationEraser(t, fx.f)
	for _, a := range admissions {
		start := make(chan struct{})
		errs := make(chan error, 2)
		go func() {
			<-start
			errs <- invitationDeleteAccount(t, fx.f, a.person.AccountID)
		}()
		go func() {
			<-start
			_, _, e := s.ManageAdmission(t.Context(), fx.scope, manager, fx.member, a.v.ID, "CONFIRM", 1, onboardingID(t))
			// A confirmation after the deletion's committed billing-exit fence
			// is refused by the existing role guard (B-BR-EMP-44) and rolls back;
			// the final-state checks below prove it left no effect either way.
			if errors.Is(e, app.ErrInvitationConflict) || errors.Is(e, app.ErrTokenInvalid) || errors.Is(e, app.ErrRegisterUnavailable) {
				e = nil
			}
			errs <- e
		}()
		close(start)
		for range 2 {
			if e := <-errs; e != nil {
				t.Fatal("deletion/confirmation race", e)
			}
		}
		invitationCount(t, fx, `SELECT count(*) FROM org_invitation_acceptances a JOIN org_memberships m ON(m.org_id,m.id)=(a.org_id,a.membership_id) WHERE a.invitation_id=$1 AND a.account_id IS NULL AND a.account_alias='' AND m.account_id IS NULL AND m.account_tombstone IS NOT NULL AND m.state='ENDED'`, 1, a.v.ID)
		invitationCount(t, fx, `SELECT count(*) FROM org_role_assignments WHERE membership_id=$1 AND ended_at IS NULL`, 0, a.v.MembershipID)
		read, _, e := s.ManageAdmission(t.Context(), fx.scope, manager, fx.member, a.v.ID, "READ", 0, "")
		if e != nil || read.Alias != "" || !read.AccountErased {
			t.Fatal("raced erasure left a readable alias", e)
		}
	}
}
