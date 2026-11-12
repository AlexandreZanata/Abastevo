//go:build integration

package adapters_test

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/adapters"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
)

func TestStoreReplayCandidatesAndCommitFailure(t *testing.T) {
	f, a, b := fixture(t)
	activate(t, f, a, 601, 701, "OWNER")
	activate(t, f, a, 602, 702, "DRIVER")
	s := adapters.NewStore(f.Runtime)
	input := assignment(t, a, key(710), key(602))
	if changed, err := s.Create(t.Context(), a, input); !changed || err != nil {
		t.Fatal(changed, err)
	}
	if changed, err := s.Create(t.Context(), a, input); changed || err != nil {
		t.Fatal(changed, err)
	}
	snapshot, err := input.Snapshot(a)
	if err != nil {
		t.Fatal(err)
	}
	snapshot.Reason = "changed intent"
	different, err := d.RehydrateAssignment(a, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if changed, err := s.Create(t.Context(), a, different); changed || err != d.ErrAssignmentConflict {
		t.Fatal(changed, err)
	}
	if _, err := s.Get(t.Context(), b, key(710)); err != app.ErrNotFound {
		t.Fatal(err)
	}
	if rows, err := s.Candidates(t.Context(), b, key(602)); err != nil || len(rows) != 0 {
		t.Fatal("foreign candidates", err)
	}
	// Ending the separate driver role removes it from candidates while its
	// permanent historical row remains readable.
	if err := end(t, f, a, 702); err != nil {
		t.Fatal(err)
	}
	if rows, err := s.Candidates(t.Context(), a, key(602)); err != nil || len(rows) != 1 {
		t.Fatal(len(rows), err)
	}
	ended, changed, err := s.End(t.Context(), a, key(710), 1)
	if err != nil || !changed {
		t.Fatal(changed, err)
	}
	v, err := ended.Snapshot(a)
	if err != nil || v.EndedAt.IsZero() || v.Revision != 2 {
		t.Fatal("missing server end", err)
	}
	if _, changed, err := s.End(t.Context(), a, key(710), 1); err != nil || changed {
		t.Fatal(changed, err)
	}
	if _, changed, err := s.End(t.Context(), a, key(710), 2); err != d.ErrAssignmentConflict || changed {
		t.Fatal(changed, err)
	}
	if changed, err := s.Create(t.Context(), a, input); changed || err != nil {
		t.Fatal("creation replay after end failed", changed, err)
	}
	// Deferred ownership refusal must not return the value/changed=true that
	// was produced inside the ultimately rolled-back transaction.
	got, changed, err := s.End(t.Context(), a, key(701), 1)
	if err != kernel.ErrLastOrgOwner || changed || !reflect.DeepEqual(got, d.Assignment{}) {
		t.Fatal("failed commit leaked successful end", changed, err)
	}
	owner, err := s.Get(t.Context(), a, key(701))
	if err != nil {
		t.Fatal(err)
	}
	v, err = owner.Snapshot(a)
	if err != nil || v.Revision != 1 || !v.EndedAt.IsZero() {
		t.Fatal("failed commit changed owner", err)
	}
	if rows, err := s.Candidates(t.Context(), a, key(602)); err != nil || len(rows) != 0 {
		t.Fatal(len(rows), err)
	}
}

func TestStoreConcurrentExactCreateAndEnd(t *testing.T) {
	f, a, _ := fixture(t)
	activate(t, f, a, 601, 701, "OWNER")
	seed(t, f, a, 602)
	pool := f.RuntimePool(t, 12)
	s := adapters.NewStore(pool)
	input := assignment(t, a, key(710), key(602))
	var wg sync.WaitGroup
	results := make(chan bool, 12)
	errs := make(chan error, 12)
	for range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			changed, err := s.Create(t.Context(), a, input)
			results <- changed
			errs <- err
		}()
	}
	wg.Wait()
	close(results)
	close(errs)
	changed := 0
	for v := range results {
		if v {
			changed++
		}
	}
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if changed != 1 {
		t.Fatal("create winners", changed)
	}
	if rows, err := s.Candidates(t.Context(), a, key(602)); err != nil || len(rows) != 0 {
		t.Fatal("invited membership got candidates", err)
	}
	results = make(chan bool, 12)
	errs = make(chan error, 12)
	for range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, changed, err := s.End(t.Context(), a, key(710), 1)
			results <- changed
			errs <- err
		}()
	}
	wg.Wait()
	close(results)
	close(errs)
	changed = 0
	for v := range results {
		if v {
			changed++
		}
	}
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if changed != 1 {
		t.Fatal("end winners", changed)
	}
}

func TestStoreCompositionRollbackAndScopeChecks(t *testing.T) {
	f, a, b := fixture(t)
	activate(t, f, a, 601, 701, "OWNER")
	seed(t, f, a, 602)
	s := adapters.NewStore(f.Runtime)
	input := assignment(t, a, key(710), key(602))
	sentinel := errors.New("synthetic abort")
	if err := database.WithOrgTx(t.Context(), f.Runtime, a, func(tx pgx.Tx) error {
		if changed, err := s.CreateTx(t.Context(), tx, a, input); !changed || err != nil {
			t.Fatal(changed, err)
		}
		if _, err := s.GetTx(t.Context(), tx, b, key(710)); err != d.ErrAssignmentScope {
			t.Fatal(err)
		}
		if _, err := s.CandidatesTx(t.Context(), tx, b, key(602)); err != d.ErrAssignmentScope {
			t.Fatal(err)
		}
		if _, err := s.CandidatesTx(t.Context(), tx, a, key(602)); err != nil {
			t.Fatal(err)
		}
		return sentinel
	}); err != sentinel {
		t.Fatal(err)
	}
	if _, err := s.Get(t.Context(), a, key(710)); err != app.ErrNotFound {
		t.Fatal("rollback retained assignment", err)
	}
	if _, err := adapters.NewStore(f.Admin).Get(t.Context(), a, key(701)); err != app.ErrUnavailable {
		t.Fatal("unsafe login accepted", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := s.Get(ctx, a, key(701)); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	// A blocked account lock can be cancelled without an assignment write.
	tx, err := f.Admin.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if _, err := tx.Exec(t.Context(), `SELECT id FROM accounts WHERE id=$1 FOR UPDATE`, key(702)); err != nil {
		t.Fatal(err)
	}
	deadline, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	if changed, err := s.Create(deadline, a, input); changed || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(changed, err)
	}
}

func TestAccountExitAndMembershipEndUseActualRolePolicy(t *testing.T) {
	f, a, _ := fixture(t)
	activate(t, f, a, 601, 701, "OWNER")
	activate(t, f, a, 602, 702, "OWNER")
	s := adapters.NewStore(f.Runtime)
	original, err := s.Get(t.Context(), a, key(701))
	if err != nil {
		t.Fatal(err)
	}
	// The first personal-deletion billing intent reserves one owner before any
	// provider call. The other can no longer end or suspend its own access.
	if _, err := f.Admin.Exec(t.Context(), `INSERT INTO payment_billing_exits(id,account_id,operation_key,reason,state,requested_at,last_event_at) VALUES($1,$2,'synthetic-exit','ACCOUNT_DELETION','PENDING',now(),now())`, key(801), key(701)); err != nil {
		t.Fatal(err)
	}
	if _, changed, err := s.End(t.Context(), a, key(702), 1); changed || err != kernel.ErrLastOrgOwner {
		t.Fatal(changed, err)
	}
	_, err = f.Admin.Exec(t.Context(), `UPDATE accounts SET status='suspended' WHERE id=$1`, key(702))
	code(t, err, "P7501")
	if _, err := f.Admin.Exec(t.Context(), `UPDATE accounts SET status='deleted' WHERE id=$1`, key(701)); err != nil {
		t.Fatal(err)
	}
	ended, err := s.Get(t.Context(), a, key(701))
	if err != nil {
		t.Fatal(err)
	}
	v, err := ended.Snapshot(a)
	if err != nil || v.EndedAt.IsZero() {
		t.Fatal("account erasure left a live assignment", err)
	}
	if rows, err := s.Candidates(t.Context(), a, key(601)); err != nil || len(rows) != 0 {
		t.Fatal("deleted-account candidates", err)
	}
	if changed, err := s.Create(t.Context(), a, original); changed || err != nil {
		t.Fatal("historical create retry failed after account erasure", changed, err)
	}
	// Ending a nonowner atomically ends even a future assignment.
	activate(t, f, a, 603, 703, "DRIVER")
	if err := database.WithOrgTx(t.Context(), f.Runtime, a, func(tx pgx.Tx) error {
		_, err := tx.Exec(t.Context(), `UPDATE org_memberships SET state='ENDED',end_reason='OFFBOARDED',revision=revision+1 WHERE org_id=$1 AND id=$2`, a.OrgID().String(), key(603))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	ended, err = s.Get(t.Context(), a, key(703))
	if err != nil {
		t.Fatal(err)
	}
	v, err = ended.Snapshot(a)
	if err != nil || v.EndedAt.IsZero() {
		t.Fatal("offboarding left role effective", err)
	}
}
