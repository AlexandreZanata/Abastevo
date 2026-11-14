//go:build integration

package orgquota

import (
	"bytes"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
)

func TestIntakeBudgetMetersAllUnitsAndEmptyBodiesAtomically(t *testing.T) {
	f, _, defs := fixture(t, 6, 5, 4)
	d := TransportDefinition{Class: "synthetic.intake", Organization: defs[0].Organization, Account: defs[0].Account, Device: defs[0].Device, Rationale: "Synthetic weighted transport policy."}
	b, e := NewIntakeBudget(f.Runtime, bytes.Repeat([]byte{29}, 32), d)
	if e != nil {
		t.Fatal(e)
	}
	p := principal(t, orgA)
	if e := b.CheckBatch(t.Context(), p, 3); e != nil {
		t.Fatal(e)
	}
	total(t, f.Admin, 9)
	var limited *orgapi.RateLimitError
	if e := b.CheckBatch(t.Context(), p, 2); !errors.As(e, &limited) || limited.RetryAfter <= 0 || limited.RetryAfter > 24*time.Hour {
		t.Fatal("weighted denial", e)
	}
	total(t, f.Admin, 9)
	if e := b.CheckBatch(t.Context(), p, 0); e != nil {
		t.Fatal("empty body not metered", e)
	}
	total(t, f.Admin, 12)
	if e := b.CheckBatch(t.Context(), p, 0); !errors.As(e, &limited) {
		t.Fatal("empty body bypassed quota", e)
	}
	total(t, f.Admin, 12)
}

func TestIntakeBudgetConcurrentWeightedReservationCannotOverspend(t *testing.T) {
	f, _, defs := fixture(t, 6, 6, 6)
	d := TransportDefinition{Class: "synthetic.intake.race", Organization: defs[0].Organization, Account: defs[0].Account, Device: defs[0].Device, Rationale: "Synthetic contention policy."}
	b, e := NewIntakeBudget(f.RuntimePool(t, 16), bytes.Repeat([]byte{31}, 32), d)
	if e != nil {
		t.Fatal(e)
	}
	p := principal(t, orgA)
	var admitted atomic.Int32
	var wg sync.WaitGroup
	start := make(chan struct{})
	failures := make(chan error, 12)
	for range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			e := b.CheckBatch(t.Context(), p, 2)
			if e == nil {
				admitted.Add(1)
			} else {
				var limited *orgapi.RateLimitError
				if !errors.As(e, &limited) {
					failures <- e
				}
			}
		}()
	}
	close(start)
	wg.Wait()
	close(failures)
	for e := range failures {
		t.Fatal(e)
	}
	if admitted.Load() != 3 {
		t.Fatal("weighted overspend", admitted.Load())
	}
	total(t, f.Admin, 18)
}

func TestIntakeBudgetLateCommitFailureRollsBackEveryReservedUnit(t *testing.T) {
	f, _, defs := fixture(t, 9, 9, 9)
	d := TransportDefinition{Class: "synthetic.intake.failure", Organization: defs[0].Organization, Account: defs[0].Account, Device: defs[0].Device, Rationale: "Synthetic deferred commit failure."}
	b, e := NewIntakeBudget(f.Runtime, bytes.Repeat([]byte{37}, 32), d)
	if e != nil {
		t.Fatal(e)
	}
	if _, e := f.Admin.Exec(t.Context(), `CREATE FUNCTION synthetic_intake_budget_failure() RETURNS trigger LANGUAGE plpgsql AS $$BEGIN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='synthetic private admission failure';END$$;
CREATE CONSTRAINT TRIGGER synthetic_intake_budget_failure AFTER INSERT OR UPDATE ON identity_rate_windows DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION synthetic_intake_budget_failure()`); e != nil {
		t.Fatal(e)
	}
	if e := b.CheckBatch(t.Context(), principal(t, orgA), 3); e != orgapi.Unavailable {
		t.Fatal("late admission failure exposed success", e)
	}
	total(t, f.Admin, 0)
	if _, e := f.Admin.Exec(t.Context(), `DROP TRIGGER synthetic_intake_budget_failure ON identity_rate_windows`); e != nil {
		t.Fatal(e)
	}
	if e := b.CheckBatch(t.Context(), principal(t, orgA), 3); e != nil {
		t.Fatal("admission recovery", e)
	}
	total(t, f.Admin, 9)
}
