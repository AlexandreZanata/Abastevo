//go:build integration

package adapters_test

import (
	"context"
	"testing"

	accountpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/adapters"
	accountdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/domain"
	"github.com/jackc/pgx/v5"
)

func TestCreateReplayAndManagedPendingReferences(t *testing.T) {
	f, s, a, _ := fixture(t)
	e := employee(t, s, a, 501)
	initial, err := d.NewInvited(a, key(601), e, d.ExistingAccount)
	if err != nil {
		t.Fatal(err)
	}
	for i := range 2 {
		changed, err := s.Create(t.Context(), a, initial)
		if err != nil || changed != (i == 0) {
			t.Fatal("create/replay", changed, err)
		}
	}
	snapshot, _ := initial.Snapshot(a)
	snapshot.Model = d.ManagedAccess
	changed, err := d.RehydrateMembership(a, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if updated, err := s.Create(t.Context(), a, changed); updated || err != d.ErrConflict {
		t.Fatal("changed creation intent", updated, err)
	}
	other := employee(t, s, a, 502)
	managed, err := d.NewInvited(a, key(602), other, d.ManagedAccess)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Create(t.Context(), a, managed); err != nil {
		t.Fatal(err)
	}
	reference, err := d.ManagedAccount(a, key(402))
	if err != nil {
		t.Fatal(err)
	}
	bound, err := managed.BindAccount(a, 1, reference)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Save(t.Context(), a, 1, bound); err != nil {
		t.Fatal(err)
	}
	got, err := s.Get(t.Context(), a, key(602))
	if err != nil {
		t.Fatal(err)
	}
	result, _ := got.Snapshot(a)
	if result.Model != d.ManagedAccess || result.Account != reference || result.State != d.MembershipInvited {
		t.Fatal("managed pending reference mapping")
	}
	// Legacy root anchors cannot silently acquire a register/active context.
	legacy := f.Organizations[0].Scope(t)
	value, err := d.NewEmployee(legacy, key(501))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateEmployee(t.Context(), legacy, value); err != d.ErrInvalid {
		t.Fatal("legacy root silently adopted", err)
	}
}

func TestErasureFunctionOwnerDriftRefusesRatherThanHidingRows(t *testing.T) {
	f, s, a, _ := fixture(t)
	bound(t, s, a, 501, 601, 401)
	owner := f.RuntimeRole + "_drift"
	role := pgx.Identifier{owner}.Sanitize()
	if _, err := f.Admin.Exec(t.Context(), "CREATE ROLE "+role+" NOLOGIN NOSUPERUSER NOBYPASSRLS; ALTER FUNCTION org_membership_account_erasure() OWNER TO "+role); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := f.Admin.Exec(context.Background(), "REASSIGN OWNED BY "+role+" TO CURRENT_USER; DROP OWNED BY "+role+"; DROP ROLE "+role); err != nil {
			t.Error(err)
		}
	})
	if _, err := accountpg.NewPGStore(f.Admin).DeleteAccount(t.Context(), key(401), clock{}.NowUnix()); err != accountdomain.ErrEnterprisePolicyUnavailable {
		t.Fatal("owner drift hid links", err)
	}
	got, err := s.Get(t.Context(), a, key(601))
	if err != nil {
		t.Fatal(err)
	}
	snapshot, _ := got.Snapshot(a)
	if snapshot.Account == (d.AccountRef{}) || snapshot.AccountTombstone != "" || snapshot.State != d.MembershipInvited {
		t.Fatal("failed owner drift left partial erasure")
	}
}
