//go:build integration

package adapters_test

import (
	"errors"
	"testing"

	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// An account joins a membership only by an explicit bind while INVITED. Ending or
// activating an unbound invitation cannot attach an account in the same step.
func TestAccountBindsOnlyWhileInvited(t *testing.T) {
	f, s, a, _ := fixture(t)
	policy(t, f)
	unbound := func(emp, member int) d.Membership {
		t.Helper()
		m := invited(t, s, a, emp, member)
		if _, err := f.Admin.Exec(t.Context(), `INSERT INTO org_test_role_assignments(org_id,member_id,role) VALUES($1,$2,'MEMBER')`, a.OrgID().String(), key(member)); err != nil {
			t.Fatal(err)
		}
		return m
	}
	stored := func(member int) (string, string) {
		t.Helper()
		var state, account string
		if err := f.Admin.QueryRow(t.Context(), `SELECT state,COALESCE(account_id::text,'') FROM org_memberships WHERE org_id=$1 AND id=$2`, a.OrgID().String(), key(member)).Scan(&state, &account); err != nil {
			t.Fatal(err)
		}
		return state, account
	}
	for _, step := range []struct {
		name, sql string
		member    int
	}{
		{"end", `UPDATE org_memberships SET state='ENDED',end_reason='ADMISSION_ENDED',account_id=$3,revision=revision+1 WHERE org_id=$1 AND id=$2`, 601},
		{"activate", `UPDATE org_memberships SET state='ACTIVE',account_id=$3,revision=revision+1 WHERE org_id=$1 AND id=$2`, 602},
	} {
		unbound(step.member-100, step.member)
		err := database.WithOrgTx(t.Context(), f.Runtime, a, func(tx pgx.Tx) error {
			_, err := tx.Exec(t.Context(), step.sql, a.OrgID().String(), key(step.member), key(401))
			return err
		})
		var refused *pgconn.PgError
		if !errors.As(err, &refused) || refused.Code != "23514" {
			t.Fatal(step.name, "bind at a transition was not refused", err)
		}
		if state, account := stored(step.member); state != "INVITED" || account != "" {
			t.Fatal(step.name, "attached an account at a transition", state, account)
		}
	}
	// The same refusal applies to a forged domain value written through Save.
	m := unbound(503, 603)
	snap, err := m.Snapshot(a)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Account, err = d.PersonalAccount(key(402)); err != nil {
		t.Fatal(err)
	}
	snap.State, snap.EndReason, snap.Revision = d.MembershipEnded, d.AdmissionEnded, 2
	if forged, err := d.RehydrateMembership(a, snap); err == nil {
		if _, err := s.Save(t.Context(), a, 1, forged); !errors.Is(err, d.ErrInvalid) {
			t.Fatal("Save attached an account while ending an invitation", err)
		}
	}
	if state, account := stored(603); state != "INVITED" || account != "" {
		t.Fatal("forged end attached an account", state, account)
	}
	// Binding while INVITED, then activating, remains the supported flow.
	b := bound(t, s, a, 504, 604, 403)
	activate(t, f, s, a, b, "MEMBER")
	if state, account := stored(604); state != "ACTIVE" || account != key(403) {
		t.Fatal("supported bind then activate failed", state, account)
	}
}
