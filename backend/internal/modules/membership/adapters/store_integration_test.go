//go:build integration

package adapters_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	accountpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/adapters"
	accountapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/application"
	accountdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/adapters"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/domain"
	orgpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters"
	org "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
	paypg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/adapters"
	payapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/application"
	paydomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func key(n int) string { return fmt.Sprintf("7b0e2a54-0000-4000-8000-%012d", n) }
func scope(t *testing.T, n int) kernel.OrgScope {
	t.Helper()
	id, err := kernel.ParseOrgID(key(n))
	if err != nil {
		t.Fatal(err)
	}
	s, err := kernel.NewOrgScope(id)
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func fixture(t *testing.T) (*orgtest.Fixture, *adapters.Store, kernel.OrgScope, kernel.OrgScope) {
	t.Helper()
	f := orgtest.New(t)
	role := pgx.Identifier{f.RuntimeRole}.Sanitize()
	_, err := f.Admin.Exec(t.Context(), "GRANT USAGE ON SCHEMA public TO "+role+"; GRANT SELECT,INSERT,UPDATE ON organizations,org_employee_identities,org_memberships TO "+role+"; GRANT SELECT,UPDATE ON accounts TO "+role+"; GRANT SELECT ON payment_billing_exits TO "+role)
	if err != nil {
		t.Fatal(err)
	}
	a, b := scope(t, 301), scope(t, 302)
	for _, s := range []kernel.OrgScope{a, b} {
		v, err := org.NewPending(s, org.Profile{})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := orgpg.NewStore(f.Runtime).Create(t.Context(), s, v); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []int{401, 402, 403} {
		if _, err := f.Admin.Exec(t.Context(), `INSERT INTO accounts(id,alias,status) VALUES($1,$2,'active')`, key(id), fmt.Sprintf("synthetic-member-%d", id)); err != nil {
			t.Fatal(err)
		}
	}
	return f, adapters.NewStore(f.Runtime), a, b
}
func employee(t *testing.T, s *adapters.Store, scope kernel.OrgScope, n int) d.Employee {
	t.Helper()
	v, err := d.NewEmployee(scope, key(n))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateEmployee(t.Context(), scope, v); err != nil {
		t.Fatal(err)
	}
	return v
}
func invited(t *testing.T, s *adapters.Store, scope kernel.OrgScope, emp, member int) d.Membership {
	t.Helper()
	e := employee(t, s, scope, emp)
	v, err := d.NewInvited(scope, key(member), e, d.ExistingAccount)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Create(t.Context(), scope, v); err != nil {
		t.Fatal(err)
	}
	return v
}
func bound(t *testing.T, s *adapters.Store, scope kernel.OrgScope, emp, member, account int) d.Membership {
	t.Helper()
	m := invited(t, s, scope, emp, member)
	a, err := d.PersonalAccount(key(account))
	if err != nil {
		t.Fatal(err)
	}
	m, err = m.BindAccount(scope, 1, a)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Save(t.Context(), scope, 1, m); err != nil {
		t.Fatal(err)
	}
	return m
}
func policy(t *testing.T, f *orgtest.Fixture) {
	t.Helper()
	raw, err := os.ReadFile("testdata/owner-policy.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Admin.Exec(t.Context(), string(raw)); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Admin.Exec(t.Context(), "GRANT SELECT ON org_test_role_assignments TO "+pgx.Identifier{f.RuntimeRole}.Sanitize()); err != nil {
		t.Fatal(err)
	}
}
func activate(t *testing.T, f *orgtest.Fixture, s *adapters.Store, scope kernel.OrgScope, m d.Membership, role string) d.Membership {
	t.Helper()
	before, err := m.Snapshot(scope)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Admin.Exec(t.Context(), `INSERT INTO org_test_role_assignments(org_id,member_id,role) VALUES($1,$2,$3)`, scope.OrgID().String(), before.ID, role); err != nil {
		t.Fatal(err)
	}
	e, err := s.GetEmployee(t.Context(), scope, before.EmployeeID)
	if err != nil {
		t.Fatal(err)
	}
	active, err := m.Transition(scope, before.Revision, d.MembershipActive, d.MembershipConditions{Employee: e, AdmissionConfirmed: true, ConfirmationSatisfied: true, LiveRoleAssignments: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Save(t.Context(), scope, before.Revision, active); err != nil {
		t.Fatal(err)
	}
	return active
}
func ended(t *testing.T, scope kernel.OrgScope, m d.Membership) d.Membership {
	t.Helper()
	v, err := m.Snapshot(scope)
	if err != nil {
		t.Fatal(err)
	}
	result, err := m.Transition(scope, v.Revision, d.MembershipEnded, d.MembershipConditions{OwnershipProtected: true, ResponsibilitiesReviewed: true, EndReason: d.Offboarded})
	if err != nil {
		t.Fatal(err)
	}
	return result
}
func noScope(t *testing.T, f *orgtest.Fixture) {
	t.Helper()
	var clean bool
	if err := f.Runtime.QueryRow(t.Context(), `SELECT COALESCE(current_setting('anpfuel.org_id',true),'')='' AND COALESCE(current_setting('anpfuel.erasure_account',true),'')=''`).Scan(&clean); err != nil || !clean {
		t.Fatal("private GUC leaked", err)
	}
}

func TestMembershipRoundTripReplayAndIsolation(t *testing.T) {
	f, s, a, b := fixture(t)
	m := bound(t, s, a, 501, 601, 401)
	if changed, err := s.Save(t.Context(), a, 1, m); changed || err != nil {
		t.Fatal("binding replay", changed, err)
	}
	if _, err := s.Get(t.Context(), b, key(601)); err != app.ErrNotFound {
		t.Fatal(err)
	}
	if _, err := s.GetEmployee(t.Context(), b, key(501)); err != app.ErrNotFound {
		t.Fatal(err)
	}
	if _, err := s.ActiveByAccount(t.Context(), a, key(401)); err != app.ErrNotFound {
		t.Fatal("invited gave access", err)
	}
	if _, err := s.CreateEmployee(t.Context(), a, employee(t, s, a, 501)); err != nil {
		t.Fatal(err)
	}
	// The same employee/member/account IDs in a different org are independent.
	other := bound(t, s, b, 501, 601, 401)
	if snap, err := other.Snapshot(b); err != nil || snap.OrgID != b.OrgID() {
		t.Fatal(err)
	}
	got, err := s.Get(t.Context(), a, key(601))
	if err != nil {
		t.Fatal(err)
	}
	before, _ := m.Snapshot(a)
	after, _ := got.Snapshot(a)
	if before != after {
		t.Fatal("explicit snapshot mapping failed")
	}
	attempt := invited(t, s, a, 502, 602)
	ref, err := d.PersonalAccount(key(401))
	if err != nil {
		t.Fatal(err)
	}
	duplicate, err := attempt.BindAccount(a, 1, ref)
	if err != nil {
		t.Fatal(err)
	}
	if changed, err := s.Save(t.Context(), a, 1, duplicate); changed || err != d.ErrConflict {
		t.Fatal("duplicate live account accepted", changed, err)
	}
	err = database.WithOrgTx(t.Context(), f.Runtime, a, func(tx pgx.Tx) error {
		var count int
		if err := tx.QueryRow(t.Context(), `SELECT count(*) FROM org_memberships WHERE org_id=$1`, b.OrgID().String()).Scan(&count); err != nil {
			return err
		}
		if count != 0 {
			return errors.New("foreign private row exposed")
		}
		tag, err := tx.Exec(t.Context(), `UPDATE org_memberships SET revision=revision+1 WHERE org_id=$1`, b.OrgID().String())
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 0 {
			return errors.New("foreign private update")
		}
		if _, err := s.GetTx(t.Context(), tx, b, key(601)); err != d.ErrScope {
			return errors.New("mismatched transaction scope")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	err = database.WithOrgTx(t.Context(), f.Runtime, b, func(tx pgx.Tx) error {
		_, err := tx.Exec(t.Context(), `INSERT INTO org_memberships(org_id,id,employee_id,access_model,state) VALUES($1,$2,$3,'EXISTING_ACCOUNT','INVITED')`, b.OrgID().String(), key(603), key(502))
		return err
	})
	var pe *pgconn.PgError
	if !errors.As(err, &pe) || pe.Code != "23503" {
		t.Fatal("cross-org employee reference accepted", err)
	}
	if _, err := adapters.NewStore(f.Admin).Get(t.Context(), a, key(601)); err != app.ErrUnavailable {
		t.Fatal("privileged runtime accepted", err)
	}
	noScope(t, f)
}

func TestMembershipPolicyFailClosedAndPeriodsRetainHistory(t *testing.T) {
	f, s, a, _ := fixture(t)
	m := bound(t, s, a, 501, 601, 401)
	e, err := s.GetEmployee(t.Context(), a, key(501))
	if err != nil {
		t.Fatal(err)
	}
	active, err := m.Transition(a, 2, d.MembershipActive, d.MembershipConditions{Employee: e, AdmissionConfirmed: true, ConfirmationSatisfied: true, LiveRoleAssignments: 1})
	if err != nil {
		t.Fatal(err)
	}
	if changed, err := s.Save(t.Context(), a, 2, active); changed || err != kernel.ErrOrgAccessPolicyUnavailable {
		t.Fatal("missing AUTH policy activated", changed, err)
	}
	policy(t, f)
	active = activate(t, f, s, a, m, "MEMBER")
	if _, err := s.ActiveByAccount(t.Context(), a, key(401)); err != nil {
		t.Fatal(err)
	}
	suspended, err := active.Transition(a, 3, d.MembershipSuspended, d.MembershipConditions{OwnershipProtected: true, ReasonRecorded: true, Suspension: d.ByManager})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Save(t.Context(), a, 3, suspended); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ActiveByAccount(t.Context(), a, key(401)); err != app.ErrNotFound {
		t.Fatal("suspended membership gave access", err)
	}
	end := ended(t, a, suspended)
	for i := range 2 {
		changed, err := s.Save(t.Context(), a, 4, end)
		if err != nil || changed != (i == 0) {
			t.Fatal("end/replay", changed, err)
		}
	}
	if _, err := s.ActiveByAccount(t.Context(), a, key(401)); err != app.ErrNotFound {
		t.Fatal(err)
	}
	if changed, err := s.Save(t.Context(), a, 2, active); changed || err != d.ErrEnded {
		t.Fatal("ended period reopened", changed, err)
	}
	var timeline bool
	if err := f.Admin.QueryRow(t.Context(), `SELECT created_at<=started_at AND started_at<=ended_at FROM org_memberships WHERE org_id=$1 AND id=$2`, a.OrgID().String(), key(601)).Scan(&timeline); err != nil || !timeline {
		t.Fatal("period timeline", err)
	}
	next := invited(t, s, a, 501, 602)
	snap, _ := next.Snapshot(a)
	if snap.State != d.MembershipInvited || snap.Account != (d.AccountRef{}) {
		t.Fatal("new period restored access")
	}
	if _, err := s.Get(t.Context(), a, key(601)); err != nil {
		t.Fatal("rehire lost prior period", err)
	}
}

func TestMembershipConcurrentReservationsAndCAS(t *testing.T) {
	f, _, a, _ := fixture(t)
	s := adapters.NewStore(f.RuntimePool(t, 12))
	e := employee(t, s, a, 501)
	run := func(values []d.Membership, save bool) int {
		t.Helper()
		type result struct {
			changed bool
			err     error
		}
		results := make(chan result, len(values))
		start := make(chan struct{})
		var wg sync.WaitGroup
		for _, value := range values {
			wg.Go(func() {
				<-start
				var changed bool
				var err error
				if save {
					changed, err = s.Save(t.Context(), a, 1, value)
				} else {
					changed, err = s.Create(t.Context(), a, value)
				}
				results <- result{changed, err}
			})
		}
		close(start)
		wg.Wait()
		close(results)
		wins := 0
		for r := range results {
			if r.err != nil && r.err != d.ErrConflict {
				t.Fatal(r.err)
			}
			if r.changed {
				wins++
			}
		}
		return wins
	}
	values := make([]d.Membership, 12)
	for i := range values {
		v, err := d.NewInvited(a, key(610+i), e, d.ExistingAccount)
		if err != nil {
			t.Fatal(err)
		}
		values[i] = v
	}
	if wins := run(values, false); wins != 1 {
		t.Fatal("live employee winners", wins)
	}
	var winner string
	if err := f.Admin.QueryRow(t.Context(), `SELECT id::text FROM org_memberships WHERE org_id=$1`, a.OrgID().String()).Scan(&winner); err != nil {
		t.Fatal(err)
	}
	prior, err := s.Get(t.Context(), a, winner)
	if err != nil {
		t.Fatal(err)
	}
	for i := range values {
		ref, err := d.PersonalAccount(key(401 + i%3))
		if err != nil {
			t.Fatal(err)
		}
		values[i], err = prior.BindAccount(a, 1, ref)
		if err != nil {
			t.Fatal(err)
		}
	}
	// Repeated requests for the winning exact account are valid replays;
	// distinct changed account intents conflict, with only one actual update.
	if wins := run(values, true); wins != 1 {
		t.Fatal("CAS winners", wins)
	}
	noScope(t, f)
}

type clock struct{}

func (clock) NowUnix() int64 { return 1794400000 }
func accountService(f *orgtest.Fixture) *accountapp.Service {
	exit := &payapp.BillingExitService{Store: paypg.NewStore(f.Runtime)}
	return &accountapp.Service{Store: accountpg.NewPGStore(f.Runtime), Clock: clock{}, RequestBillingExit: exit.RequestAccountExit, RequestErasure: func(context.Context, string, []string) error { return nil }}
}
func accountGrants(t *testing.T, f *orgtest.Fixture) {
	t.Helper()
	if _, err := f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT,UPDATE,DELETE ON ALL TABLES IN SCHEMA public TO "+pgx.Identifier{f.RuntimeRole}.Sanitize()); err != nil {
		t.Fatal(err)
	}
}

func TestAccountDeletionUsesExistingFlowAndTombstonesEveryOrganization(t *testing.T) {
	f, s, a, b := fixture(t)
	accountGrants(t, f)
	bound(t, s, a, 501, 601, 401)
	previous := bound(t, s, b, 501, 601, 401)
	if _, err := s.Save(t.Context(), b, 2, ended(t, b, previous)); err != nil {
		t.Fatal(err)
	}
	if _, err := accountService(f).DeleteAccount(t.Context(), key(401)); err != nil {
		t.Fatal("existing account deletion", err)
	}
	var tombstones []string
	for _, scope := range []kernel.OrgScope{a, b} {
		value, err := s.Get(t.Context(), scope, key(601))
		if err != nil {
			t.Fatal(err)
		}
		snapshot, _ := value.Snapshot(scope)
		if snapshot.State != d.MembershipEnded || snapshot.Account != (d.AccountRef{}) || snapshot.AccountTombstone == "" {
			t.Fatal("personal link survived deletion")
		}
		if scope == a && snapshot.EndReason != d.AccountDeleted {
			t.Fatal("live period end reason")
		}
		if scope == b && snapshot.EndReason != d.Offboarded {
			t.Fatal("old end reason rewritten")
		}
		tombstones = append(tombstones, snapshot.AccountTombstone)
		if _, err := s.GetEmployee(t.Context(), scope, key(501)); err != nil {
			t.Fatal("employee erased", err)
		}
	}
	if tombstones[0] == tombstones[1] {
		t.Fatal("cross-organization person tombstone")
	}
	if _, err := accountService(f).DeleteAccount(t.Context(), key(401)); err != nil {
		t.Fatal("erasure replay", err)
	}
	noScope(t, f)
	// Knowing the account, setting its erasure GUC or calling internal helpers
	// cannot traverse private links as an ordinary runtime login.
	bound(t, s, a, 502, 602, 402)
	if _, err := f.Runtime.Exec(t.Context(), `SELECT set_config('anpfuel.erasure_account',$1,false)`, key(402)); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := f.Runtime.QueryRow(t.Context(), `SELECT count(*) FROM org_memberships`).Scan(&count); err != nil || count != 0 {
		t.Fatal("spoofed erasure path", err)
	}
	if _, err := f.Runtime.Exec(t.Context(), `SELECT org_membership_check_account_exit($1)`, key(402)); err == nil {
		t.Fatal("runtime called internal cross-org helper")
	}
	if _, err := f.Runtime.Exec(t.Context(), `RESET anpfuel.erasure_account`); err != nil {
		t.Fatal(err)
	}
}

func TestLastOwnerAndUnknownPolicyRefuseBeforeBillingAndRawDeletion(t *testing.T) {
	f, s, a, _ := fixture(t)
	accountGrants(t, f)
	m := bound(t, s, a, 501, 601, 401)
	policy(t, f)
	active := activate(t, f, s, a, m, "OWNER")
	for _, tc := range []struct {
		run  func() error
		want error
	}{
		{func() error { _, err := s.Save(t.Context(), a, 3, ended(t, a, active)); return err }, kernel.ErrLastOrgOwner},
		{func() error {
			_, err := accountpg.NewPGStore(f.Runtime).DeleteAccount(t.Context(), key(401), clock{}.NowUnix())
			return err
		}, accountdomain.ErrEnterpriseLastOwner},
		{func() error { _, err := accountService(f).DeleteAccount(t.Context(), key(401)); return err }, accountdomain.ErrEnterpriseLastOwner},
	} {
		if err := tc.run(); !errors.Is(err, tc.want) {
			t.Fatal("last owner bypass", err)
		}
	}
	if _, err := f.Runtime.Exec(t.Context(), `UPDATE accounts SET status='deleted' WHERE id=$1`, key(401)); err == nil {
		t.Fatal("raw status update bypassed owner")
	}
	if _, err := f.Runtime.Exec(t.Context(), `DELETE FROM accounts WHERE id=$1`, key(401)); err == nil {
		t.Fatal("physical delete bypassed owner")
	}
	var unchanged bool
	if err := f.Admin.QueryRow(t.Context(), `SELECT a.status='active' AND m.state='ACTIVE' AND m.account_tombstone IS NULL AND NOT EXISTS(SELECT 1 FROM payment_billing_exits WHERE account_id=a.id) FROM accounts a JOIN org_memberships m ON m.account_id=a.id WHERE a.id=$1`, key(401)).Scan(&unchanged); err != nil || !unchanged {
		t.Fatal("refusal mutated account/member/billing", err)
	}
	if _, err := f.Admin.Exec(t.Context(), `CREATE OR REPLACE FUNCTION org_membership_access_policy(org UUID,member UUID,action TEXT) RETURNS TEXT LANGUAGE SQL VOLATILE SET search_path=pg_catalog AS $$ SELECT 'UNAVAILABLE'::text $$`); err != nil {
		t.Fatal(err)
	}
	if _, err := accountService(f).DeleteAccount(t.Context(), key(401)); !errors.Is(err, accountdomain.ErrEnterprisePolicyUnavailable) {
		t.Fatal("missing policy did not refuse", err)
	}
	noScope(t, f)
}

func TestAccountDeletionReservationAndChangedReasonCannotRaceOwners(t *testing.T) {
	f, s, a, _ := fixture(t)
	accountGrants(t, f)
	policy(t, f)
	activate(t, f, s, a, bound(t, s, a, 501, 601, 401), "OWNER")
	activate(t, f, s, a, bound(t, s, a, 502, 602, 402), "OWNER")
	pay := paypg.NewStore(f.RuntimePool(t, 4))
	type result struct {
		account int
		err     error
	}
	results := make(chan result, 2)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for _, account := range []int{401, 402} {
		wg.Go(func() {
			<-start
			_, _, _, err := pay.CreateBillingExit(t.Context(), key(account), "account-deletion", payapp.BillingExitAccountDeletion, time.Now().UTC())
			results <- result{account, err}
		})
	}
	close(start)
	wg.Wait()
	close(results)
	winner := 0
	for r := range results {
		if r.err == nil {
			if winner != 0 {
				t.Fatal("both owners reserved deletion")
			}
			winner = r.account
		} else if r.err != kernel.ErrLastOrgOwner {
			t.Fatal(r.err)
		}
	}
	if winner == 0 {
		t.Fatal("no deletion reservation winner")
	}
	if _, err := f.Runtime.Exec(t.Context(), `DELETE FROM payment_billing_exits WHERE account_id=$1`, key(winner)); err == nil {
		t.Fatal("pending deletion fence removed")
	}
	if _, err := f.Runtime.Exec(t.Context(), `UPDATE payment_billing_exits SET reason='OWNER_REQUEST' WHERE account_id=$1`, key(winner)); err == nil {
		t.Fatal("deletion fence reason changed")
	}
	remainingMember := 601
	if winner == 401 {
		remainingMember = 602
	}
	remaining, err := s.Get(t.Context(), a, key(remainingMember))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Save(t.Context(), a, 3, ended(t, a, remaining)); err != kernel.ErrLastOrgOwner {
		t.Fatal("pending deletion counted as surviving owner", err)
	}
	if _, err := s.ActiveByAccount(t.Context(), a, key(winner)); err != app.ErrNotFound {
		t.Fatal("pending deletion retained admission", err)
	}
	if _, err := accountService(f).DeleteAccount(t.Context(), key(winner)); err != nil {
		t.Fatal("reserved deletion could not finish", err)
	}
	// Reason is part of idempotency: an owner-request row cannot stand in for
	// the account-deletion reservation and skip the new trigger.
	if _, _, _, err := pay.CreateBillingExit(t.Context(), key(403), "account-deletion", payapp.BillingExitOwnerRequest, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := pay.CreateBillingExit(t.Context(), key(403), "account-deletion", payapp.BillingExitAccountDeletion, time.Now().UTC()); err != paydomain.ErrConflict {
		t.Fatal("changed reason replay bypassed guard", err)
	}
	noScope(t, f)
}
