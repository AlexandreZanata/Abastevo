//go:build integration

package adapters_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/adapters"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
)

func TestUnboundCreateRefusesBeforeTakingRootLock(t *testing.T) {
	f, a, _ := fixture(t)
	if err := database.WithOrgTx(t.Context(), f.Runtime, a, func(tx pgx.Tx) error {
		if _, err := tx.Exec(t.Context(), `INSERT INTO org_employee_identities(org_id,id,state) VALUES($1,$2,'ACTIVE')`, a.OrgID().String(), key(802)); err != nil {
			return err
		}
		_, err := tx.Exec(t.Context(), `INSERT INTO org_memberships(org_id,id,employee_id,access_model,state) VALUES($1,$2,$3,'EXISTING_ACCOUNT','INVITED')`, a.OrgID().String(), key(602), key(802))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	held, err := f.Admin.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = held.Rollback(context.Background()) }()
	if _, err := held.Exec(t.Context(), `SELECT org_id FROM organizations WHERE org_id=$1 FOR UPDATE`, a.OrgID().String()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
	defer cancel()
	if changed, err := adapters.NewStore(f.Runtime).Create(ctx, a, assignment(t, a, key(710), key(602))); changed || err != d.ErrAssignmentInvalid {
		t.Fatal("unbound create waited on root or reported success", changed, err)
	}
}

func TestConcurrentAccountExitReservationsUseActualOwners(t *testing.T) {
	f, a, _ := fixture(t)
	activate(t, f, a, 601, 701, "OWNER")
	activate(t, f, a, 602, 702, "OWNER")
	var wg sync.WaitGroup
	results := make(chan error, 2)
	start := make(chan struct{})
	for _, n := range []int{701, 702} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := f.Admin.Exec(t.Context(), `INSERT INTO payment_billing_exits(id,account_id,operation_key,reason,state,requested_at,last_event_at) VALUES($1,$2,$3,'ACCOUNT_DELETION','PENDING',now(),now())`, key(n+100), key(n), key(n))
			results <- err
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
		t.Fatal("account-exit reservation winners", success, refused)
	}
}

func TestDeferredOwnerGuardCannotBeSkippedByChangingScope(t *testing.T) {
	f, a, b := fixture(t)
	activate(t, f, a, 601, 701, "OWNER")
	err := database.WithOrgTx(t.Context(), f.Runtime, a, func(tx pgx.Tx) error {
		if _, err := tx.Exec(t.Context(), `UPDATE org_role_assignments SET ended_at=clock_timestamp(),revision=revision+1 WHERE org_id=$1 AND id=$2`, a.OrgID().String(), key(701)); err != nil {
			return err
		}
		_, err := tx.Exec(t.Context(), `SELECT set_config('anpfuel.org_id',$1,true)`, b.OrgID().String())
		return err
	})
	code(t, err, "P7501")
}

func TestCandidatesExcludeExpiredScheduledSuspendedAndEndedPeriods(t *testing.T) {
	f, a, _ := fixture(t)
	activate(t, f, a, 601, 701, "OWNER")
	activate(t, f, a, 602, 702, "DRIVER")
	s := adapters.NewStore(f.Runtime)
	now := time.Now().UTC()
	// Storage refuses retroactive or already-over periods (000183), so the
	// expired candidate is a real short period that then elapses.
	expires := now.Add(1500 * time.Millisecond)
	if err := database.WithOrgTx(t.Context(), f.Runtime, a, func(tx pgx.Tx) error {
		if err := insert(t, tx, a, 703, 602, "ADMIN", "ORG", a.OrgID().String(), now.Add(-time.Minute), expires); err != nil {
			return err
		}
		return insert(t, tx, a, 704, 602, "ADMIN", "ORG", a.OrgID().String(), now.Add(time.Hour), nil)
	}); err != nil {
		t.Fatal(err)
	}
	for deadline := time.Now().Add(10 * time.Second); ; time.Sleep(50 * time.Millisecond) {
		var elapsed bool
		if err := f.Admin.QueryRow(t.Context(), `SELECT clock_timestamp()>$1`, expires).Scan(&elapsed); err != nil {
			t.Fatal(err)
		}
		if elapsed {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("synthetic period did not elapse on the database clock")
		}
	}
	if rows, err := s.Candidates(t.Context(), a, key(602)); err != nil || len(rows) != 1 {
		t.Fatal("expired or scheduled assignment became a candidate", len(rows), err)
	}
	if err := database.WithOrgTx(t.Context(), f.Runtime, a, func(tx pgx.Tx) error {
		_, err := tx.Exec(t.Context(), `UPDATE org_memberships SET state='SUSPENDED',suspension_origin='MANAGER',revision=revision+1 WHERE org_id=$1 AND id=$2`, a.OrgID().String(), key(602))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if rows, err := s.Candidates(t.Context(), a, key(602)); err != nil || len(rows) != 0 {
		t.Fatal("suspended membership had candidates", err)
	}
	if err := database.WithOrgTx(t.Context(), f.Runtime, a, func(tx pgx.Tx) error {
		_, err := tx.Exec(t.Context(), `UPDATE org_memberships SET state='ACTIVE',suspension_origin='',revision=revision+1 WHERE org_id=$1 AND id=$2`, a.OrgID().String(), key(602))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if rows, err := s.Candidates(t.Context(), a, key(602)); err != nil || len(rows) != 1 {
		t.Fatal("reactivation did not resolve current roles", err)
	}
	if _, err := f.Admin.Exec(t.Context(), `UPDATE accounts SET status='suspended' WHERE id=$1`, key(702)); err != nil {
		t.Fatal(err)
	}
	if rows, err := s.Candidates(t.Context(), a, key(602)); err != nil || len(rows) != 0 {
		t.Fatal("unavailable account had candidates", err)
	}
}

func TestRoleGuardsRejectUnboundEndedAndManagedOwner(t *testing.T) {
	f, a, _ := fixture(t)
	activate(t, f, a, 601, 701, "OWNER")
	seed(t, f, a, 602)
	if err := database.WithOrgTx(t.Context(), f.Runtime, a, func(tx pgx.Tx) error {
		_, err := tx.Exec(t.Context(), `UPDATE org_memberships SET state='ENDED',end_reason='ADMISSION_ENDED',revision=revision+1 WHERE org_id=$1 AND id=$2`, a.OrgID().String(), key(602))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	err := database.WithOrgTx(t.Context(), f.Runtime, a, func(tx pgx.Tx) error { return insert(t, tx, a, 702, 602, "DRIVER", "SELF", key(602), time.Now(), nil) })
	code(t, err, "23514")
	// New managed membership, not a mutation of a personal access model.
	if _, err := f.Admin.Exec(t.Context(), `INSERT INTO accounts(id,alias,status) VALUES($1,'synthetic-managed','active')`, key(703)); err != nil {
		t.Fatal(err)
	}
	if err := database.WithOrgTx(t.Context(), f.Runtime, a, func(tx pgx.Tx) error {
		if _, err := tx.Exec(t.Context(), `INSERT INTO org_employee_identities(org_id,id,state) VALUES($1,$2,'ACTIVE')`, a.OrgID().String(), key(803)); err != nil {
			return err
		}
		_, err := tx.Exec(t.Context(), `INSERT INTO org_memberships(org_id,id,employee_id,account_id,access_model,state) VALUES($1,$2,$3,$4,'MANAGED','INVITED')`, a.OrgID().String(), key(603), key(803), key(703))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	err = database.WithOrgTx(t.Context(), f.Runtime, a, func(tx pgx.Tx) error {
		return insert(t, tx, a, 703, 603, "OWNER", "ORG", a.OrgID().String(), time.Now(), nil)
	})
	code(t, err, "23514")
	if err := database.WithOrgTx(t.Context(), f.Runtime, a, func(tx pgx.Tx) error {
		if _, err := tx.Exec(t.Context(), `INSERT INTO org_employee_identities(org_id,id,state) VALUES($1,$2,'ACTIVE')`, a.OrgID().String(), key(804)); err != nil {
			return err
		}
		_, err := tx.Exec(t.Context(), `INSERT INTO org_memberships(org_id,id,employee_id,access_model,state) VALUES($1,$2,$3,'EXISTING_ACCOUNT','INVITED')`, a.OrgID().String(), key(604), key(804))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	err = database.WithOrgTx(t.Context(), f.Runtime, a, func(tx pgx.Tx) error { return insert(t, tx, a, 705, 604, "DRIVER", "SELF", key(604), time.Now(), nil) })
	code(t, err, "23514")
	// A valid pending nonowner row alone never activates the membership.
	if err := database.WithOrgTx(t.Context(), f.Runtime, a, func(tx pgx.Tx) error { return insert(t, tx, a, 704, 603, "DRIVER", "SELF", key(603), time.Now(), nil) }); err != nil {
		t.Fatal(err)
	}
	if rows, err := adapters.NewStore(f.Runtime).Candidates(t.Context(), a, key(603)); err != nil || len(rows) != 0 {
		t.Fatal("pending managed assignment became authority", err)
	}
}
