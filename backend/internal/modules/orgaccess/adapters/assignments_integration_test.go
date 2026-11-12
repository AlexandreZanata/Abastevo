//go:build integration

package adapters_test

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	orgpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters"
	org "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
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
func fixture(t *testing.T) (*orgtest.Fixture, kernel.OrgScope, kernel.OrgScope) {
	t.Helper()
	f := orgtest.New(t)
	f.GrantTeamReductions(t)
	f.GrantSettingsLifecycle(t)
	r := pgx.Identifier{f.RuntimeRole}.Sanitize()
	if _, err := f.Admin.Exec(t.Context(), "GRANT USAGE ON SCHEMA public TO "+r+"; GRANT SELECT,INSERT,UPDATE ON organizations,org_employee_identities,org_memberships,org_role_assignments TO "+r+"; GRANT SELECT,UPDATE ON accounts TO "+r+"; GRANT SELECT,INSERT ON payment_billing_exits TO "+r); err != nil {
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
	return f, a, b
}
func seed(t *testing.T, f *orgtest.Fixture, s kernel.OrgScope, n int) {
	t.Helper()
	if _, err := f.Admin.Exec(t.Context(), `INSERT INTO accounts(id,alias,status) VALUES($1,$2,'active')`, key(n+100), fmt.Sprintf("synthetic-role-%d", n)); err != nil {
		t.Fatal(err)
	}
	if err := database.WithOrgTx(t.Context(), f.Runtime, s, func(tx pgx.Tx) error {
		if _, err := tx.Exec(t.Context(), `INSERT INTO org_employee_identities(org_id,id,state) VALUES($1,$2,'ACTIVE')`, s.OrgID().String(), key(n+200)); err != nil {
			return err
		}
		_, err := tx.Exec(t.Context(), `INSERT INTO org_memberships(org_id,id,employee_id,account_id,access_model,state) VALUES($1,$2,$3,$4,'EXISTING_ACCOUNT','INVITED')`, s.OrgID().String(), key(n), key(n+200), key(n+100))
		return err
	}); err != nil {
		t.Fatal(err)
	}
}
func insert(t *testing.T, tx pgx.Tx, s kernel.OrgScope, id, member int, role, kind, target string, from time.Time, to any) error {
	t.Helper()
	_, err := tx.Exec(t.Context(), `INSERT INTO org_role_assignments(org_id,id,membership_id,role,scope_type,scope_target,valid_from,valid_to,assigned_by,reason) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$3,'synthetic assignment')`, s.OrgID().String(), key(id), key(member), role, kind, target, from, to)
	return err
}
func activate(t *testing.T, f *orgtest.Fixture, s kernel.OrgScope, member, id int, role string) {
	t.Helper()
	seed(t, f, s, member)
	if err := database.WithOrgTx(t.Context(), f.Runtime, s, func(tx pgx.Tx) error {
		kind, target := "SELF", key(member)
		if role == "OWNER" {
			kind, target = "ORG", s.OrgID().String()
		}
		// A current period: 000183 refuses storage starts beyond the bounded skew.
		if err := insert(t, tx, s, id, member, role, kind, target, time.Now().UTC().Add(-time.Minute), nil); err != nil {
			return err
		}
		_, err := tx.Exec(t.Context(), `UPDATE org_memberships SET state='ACTIVE',revision=revision+1 WHERE org_id=$1 AND id=$2`, s.OrgID().String(), key(member))
		return err
	}); err != nil {
		t.Fatal(err)
	}
}
func code(t *testing.T, err error, want string) {
	t.Helper()
	var p *pgconn.PgError
	if !errors.As(err, &p) || p.Code != want {
		t.Fatalf("want SQLSTATE %s, got %v", want, err)
	}
}
func end(t *testing.T, f *orgtest.Fixture, s kernel.OrgScope, id int) error {
	t.Helper()
	return database.WithOrgTx(t.Context(), f.Runtime, s, func(tx pgx.Tx) error {
		_, err := tx.Exec(t.Context(), `UPDATE org_role_assignments SET ended_at=clock_timestamp(),revision=revision+1 WHERE org_id=$1 AND id=$2`, s.OrgID().String(), key(id))
		return err
	})
}

func TestRoleAssignmentScopePeriodsAndHistory(t *testing.T) {
	f, a, b := fixture(t)
	activate(t, f, a, 601, 701, "OWNER")
	activate(t, f, a, 602, 702, "DRIVER")
	activate(t, f, b, 603, 703, "OWNER")
	if err := database.WithOrgTx(t.Context(), f.Runtime, b, func(tx pgx.Tx) error {
		var count int
		err := tx.QueryRow(t.Context(), `SELECT count(*) FROM org_role_assignments WHERE id=$1`, key(701)).Scan(&count)
		if count != 0 {
			t.Fatal("foreign assignment visible")
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, role, kind, target string
		member, id               int
		code                     string
	}{
		{"foreign member", "DRIVER", "SELF", key(603), 603, 704, "23503"},
		{"duplicate", "DRIVER", "SELF", key(602), 602, 705, "23505"},
		{"unsupported", "DRIVER", "UNIT", key(602), 602, 706, "23514"},
		{"widen station", "STATION_ADMIN", "ORG", a.OrgID().String(), 602, 707, "23514"},
		{"foreign target", "DRIVER", "SELF", key(603), 602, 708, "23514"},
		{"self owner", "OWNER", "SELF", key(602), 602, 709, "23514"},
		{"unknown role", "administrator", "ORG", a.OrgID().String(), 602, 710, "23514"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := database.WithOrgTx(t.Context(), f.Runtime, a, func(tx pgx.Tx) error {
				return insert(t, tx, a, tc.id, tc.member, tc.role, tc.kind, tc.target, time.Now().Add(-time.Minute), nil)
			})
			code(t, err, tc.code)
		})
	}
	code(t, end(t, f, a, 701), "P7501")
	if err := end(t, f, a, 702); err != nil {
		t.Fatal(err)
	}
	code(t, end(t, f, a, 702), "55000")
	for _, sql := range []string{`DELETE FROM org_role_assignments`, `TRUNCATE org_role_assignments`, `UPDATE org_role_assignments SET role='ADMIN',revision=revision+1 WHERE id='` + key(701) + `'`} {
		// The owner cannot bypass history triggers either.
		tx, err := f.Admin.Begin(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if _, err = tx.Exec(t.Context(), `SELECT set_config('anpfuel.org_id',$1,true)`, a.OrgID().String()); err != nil {
			t.Fatal(err)
		}
		_, err = tx.Exec(t.Context(), sql)
		if err == nil {
			err = tx.Commit(t.Context())
		}
		_ = tx.Rollback(t.Context())
		if err == nil {
			t.Fatal("history bypass succeeded")
		}
	}
}

func TestConcurrentOwnerReductionsKeepOneOwner(t *testing.T) {
	f, a, _ := fixture(t)
	activate(t, f, a, 601, 701, "OWNER")
	activate(t, f, a, 602, 702, "OWNER")
	pool := f.RuntimePool(t, 4)
	var wg sync.WaitGroup
	results := make(chan error, 2)
	start := make(chan struct{})
	for _, id := range []int{701, 702} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			results <- database.WithOrgTx(t.Context(), pool, a, func(tx pgx.Tx) error {
				_, err := tx.Exec(t.Context(), `UPDATE org_role_assignments SET ended_at=clock_timestamp(),revision=revision+1 WHERE org_id=$1 AND id=$2`, a.OrgID().String(), key(id))
				return err
			})
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	success, refused := 0, 0
	for err := range results {
		if err == nil {
			success++
		} else {
			code(t, err, "P7501")
			refused++
		}
	}
	if success != 1 || refused != 1 {
		t.Fatal(success, refused)
	}
}

func TestOwnerCoverageIncludesScheduledExpiry(t *testing.T) {
	f, a, _ := fixture(t)
	activate(t, f, a, 601, 701, "OWNER")
	activate(t, f, a, 602, 702, "DRIVER")
	at := time.Now().UTC().Add(time.Hour)
	if err := database.WithOrgTx(t.Context(), f.Runtime, a, func(tx pgx.Tx) error {
		return insert(t, tx, a, 703, 602, "OWNER", "ORG", a.OrgID().String(), time.Now().Add(-time.Minute), at)
	}); err != nil {
		t.Fatal(err)
	}
	code(t, end(t, f, a, 701), "P7501")
	if err := database.WithOrgTx(t.Context(), f.Runtime, a, func(tx pgx.Tx) error { return insert(t, tx, a, 704, 602, "OWNER", "ORG", a.OrgID().String(), at, nil) }); err != nil {
		t.Fatal(err)
	}
	if err := end(t, f, a, 701); err != nil {
		t.Fatal("continuous successor intervals were refused", err)
	}
	code(t, end(t, f, a, 704), "P7501")
}
