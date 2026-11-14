//go:build integration

package adapters_test

import (
	"context"
	"errors"
	"testing"
	"time"

	amend "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgamend/domain"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	reviewpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/review/adapters"
	reviewapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/review/application"
	period "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/review/domain/period"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/jackc/pgx/v5"
)

// periodRecords installs a test-only owning record table in the disposable
// database. No production consumer exists yet (fuelings/expenses are future
// owners); this stands in for one to prove the gate inside its transaction.
func periodRecords(t *testing.T, fx legalFixtureState) {
	t.Helper()
	role := pgx.Identifier{fx.f.RuntimeRole}.Sanitize()
	if _, err := fx.f.Admin.Exec(t.Context(), `CREATE TABLE synthetic_period_records(org_id UUID NOT NULL,id UUID PRIMARY KEY,reporting_day DATE NOT NULL,period_state TEXT NOT NULL,period_revision BIGINT NOT NULL,written_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp());GRANT SELECT,INSERT ON synthetic_period_records TO `+role); err != nil {
		t.Fatal(err)
	}
}

type periodConsumerResult struct {
	fact    reviewapp.PeriodFact
	written bool
	err     error
}

// periodConsumerWrite is the test-only owning record command. It consults the
// gate inside its own scoped transaction and writes only into a writable
// period. hold, when set, runs after the gate and before the write.
func periodConsumerWrite(ctx context.Context, fx legalFixtureState, occurred time.Time, unit string, hold func(pid int)) periodConsumerResult {
	var out periodConsumerResult
	out.err = database.WithOrgTx(ctx, fx.f.Runtime, fx.scope, func(tx pgx.Tx) error {
		var pid int
		if err := tx.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&pid); err != nil {
			return err
		}
		fact, err := reviewpg.PeriodGate{}.StateForOccurrenceTx(ctx, tx, fx.scope, occurred, unit)
		if err != nil {
			return err
		}
		out.fact = fact
		if hold != nil {
			hold(pid)
		}
		if !fact.Writable() {
			return nil
		}
		id, err := newPeriodID()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO synthetic_period_records(org_id,id,reporting_day,period_state,period_revision) VALUES($1,$2,$3,$4,$5)`, fx.scope.OrgID().String(), id, fact.Day, string(fact.State), fact.Revision); err != nil {
			return err
		}
		out.written = true
		return nil
	})
	if out.err != nil {
		out.written = false
	}
	return out
}

func newPeriodID() (string, error) { return jobs.NewUUIDv4() }

// periodAwaitWaiter observes an actual lock wait of a runtime backend blocked
// by holder. It polls catalog state only; the barrier is the lock itself.
func periodAwaitWaiter(t *testing.T, ctx context.Context, fx legalFixtureState, holder int, done <-chan struct{}) int {
	t.Helper()
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		var waiter int
		err := fx.f.Admin.QueryRow(ctx, `SELECT pid FROM pg_stat_activity WHERE datname=current_database() AND usename=$1 AND wait_event_type='Lock' AND $2=ANY(pg_blocking_pids(pid)) LIMIT 1`, fx.f.RuntimeRole, holder).Scan(&waiter)
		if err == nil {
			return waiter
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			t.Fatal("lock wait observation failed", err)
		}
		select {
		case <-done:
			t.Fatal("the expected lock wait did not happen")
		case <-ticker.C:
		case <-ctx.Done():
			t.Fatal("no actual lock wait observed")
		}
	}
}

func periodGateFixture(t *testing.T) periodFixtureState {
	t.Helper()
	fx := periodFixture(t)
	periodRecords(t, fx.legalFixtureState)
	return fx
}

func periodGate(t *testing.T, fx legalFixtureState, day string) reviewapp.PeriodFact {
	t.Helper()
	var fact reviewapp.PeriodFact
	err := database.WithOrgTx(t.Context(), fx.f.Runtime, fx.scope, func(tx pgx.Tx) error {
		var err error
		fact, err = reviewpg.PeriodGate{}.StateForDayTx(t.Context(), tx, fx.scope, day)
		return err
	})
	if err != nil {
		t.Fatal("period gate", err)
	}
	return fact
}

func periodAttribute(t *testing.T, fx legalFixtureState, at time.Time, unit string) (reviewapp.PeriodFact, error) {
	t.Helper()
	var fact reviewapp.PeriodFact
	err := database.WithOrgTx(t.Context(), fx.f.Runtime, fx.scope, func(tx pgx.Tx) error {
		var err error
		fact, err = reviewpg.PeriodGate{}.StateForOccurrenceTx(t.Context(), tx, fx.scope, at, unit)
		return err
	})
	return fact, err
}

func periodUnit(t *testing.T, fx legalFixtureState, zone string) string {
	t.Helper()
	id := onboardingID(t)
	if err := unitSchemaExec(t, fx, `INSERT INTO org_units(org_id,id,name,code,time_zone,state) VALUES($1,$2,$3,$4,$5,'ACTIVE')`, fx.scope.OrgID().String(), id, "Synthetic unit "+id[:8], "U"+id[:6], zone); err != nil {
		t.Fatal(err)
	}
	return id
}

// Negative "wrong time zone" (B-BR-EMP-211/214, BR-TIME-004): the exact
// boundary instant belongs to the local day of the organization zone
// (America/Cuiaba, UTC-4) or of the unit zone, never to the UTC day.
func TestPeriodGateAttributesBoundaryInOrganizationAndUnitZone(t *testing.T) {
	fx := periodGateFixture(t)
	if _, _, err := periodWrite(t, fx.legalFixtureState, fx.periods, periodOwner(fx.legalFixtureState), fx.member, periodCmd(t, period.ActionClose, "2026-10", 1, "")); err != nil {
		t.Fatal(err)
	}
	lastOctober := time.Date(2026, 11, 1, 3, 59, 59, 999999999, time.UTC)
	firstNovember := time.Date(2026, 11, 1, 4, 0, 0, 0, time.UTC)
	// The UTC day of the last October instant is already November: a gate
	// that attributed in UTC would admit this write into an open period.
	if utc, err := orgtime.AttributeReportingDay(lastOctober, orgtime.ReportingZones{Organization: "UTC"}); err != nil || utc.Day != "2026-11-01" {
		t.Fatal("UTC hazard fixture", err)
	}
	for _, c := range []struct {
		at               time.Time
		day, month, zone string
		state            period.State
		source           string
		revision         int64
		writable         bool
		unit             string
	}{
		{lastOctober, "2026-10-31", "2026-10", "America/Cuiaba", period.Locked, "organization", 2, false, ""},
		{firstNovember, "2026-11-01", "2026-11", "America/Cuiaba", period.Open, "organization", 1, true, ""},
		{time.Date(2026, 10, 1, 4, 0, 0, 0, time.UTC), "2026-10-01", "2026-10", "America/Cuiaba", period.Locked, "organization", 2, false, ""},
		{time.Date(2026, 10, 1, 3, 59, 59, 999999999, time.UTC), "2026-09-30", "2026-09", "America/Cuiaba", period.Open, "organization", 1, true, ""},
	} {
		fact, err := periodAttribute(t, fx.legalFixtureState, c.at, c.unit)
		if err != nil || fact.Day != c.day || fact.Month != c.month || fact.State != c.state || fact.Revision != c.revision || fact.Zone != c.zone || fact.ZoneSource != c.source || fact.Writable() != c.writable || fact.Validate() != nil || fact.CheckedAt.IsZero() {
			t.Fatalf("boundary %s attributed to %s %s %s (%v)", c.at.Format(time.RFC3339Nano), fact.Day, fact.State, fact.Zone, err)
		}
	}
	// The unit zone decides when supplied (America/Sao_Paulo, UTC-3).
	south := periodUnit(t, fx.legalFixtureState, "America/Sao_Paulo")
	between := time.Date(2026, 11, 1, 3, 30, 0, 0, time.UTC)
	if f, err := periodAttribute(t, fx.legalFixtureState, between, south); err != nil || f.Day != "2026-11-01" || f.State != period.Open || f.ZoneSource != "unit" || f.Zone != "America/Sao_Paulo" {
		t.Fatal("unit zone not applied", f.Day, err)
	}
	if f, err := periodAttribute(t, fx.legalFixtureState, between, ""); err != nil || f.Day != "2026-10-31" || f.State != period.Locked {
		t.Fatal("organization zone not applied", f.Day, err)
	}
	// VAL-19: the fueling of unit Sul at 2026-11-01T02:30Z is 2026-10-31.
	if f, err := periodAttribute(t, fx.legalFixtureState, time.Date(2026, 11, 1, 2, 30, 0, 0, time.UTC), south); err != nil || f.Day != "2026-10-31" || f.State != period.Locked || f.Writable() {
		t.Fatal("VAL-19 reporting day", f.Day, err)
	}
	// An unknown or foreign unit never falls back to the organization zone.
	if _, err := periodAttribute(t, fx.legalFixtureState, between, onboardingID(t)); !errors.Is(err, reviewapp.ErrNotFound) {
		t.Fatal("unknown unit fell back", err)
	}
	other := legalFixtureFor(t, fx.f, fx.onboarding, fx.actor, fx.flags)
	foreign := periodUnit(t, other, "America/Sao_Paulo")
	if _, err := periodAttribute(t, fx.legalFixtureState, between, foreign); !errors.Is(err, reviewapp.ErrNotFound) {
		t.Fatal("foreign unit attributed", err)
	}
	if _, err := periodAttribute(t, fx.legalFixtureState, between, "not-a-unit"); !errors.Is(err, period.ErrInvalid) {
		t.Fatal("malformed unit accepted", err)
	}
	if _, err := periodAttribute(t, fx.legalFixtureState, time.Time{}, ""); !errors.Is(err, period.ErrInvalid) {
		t.Fatal("zero instant attributed", err)
	}
	// Day form: half-open boundaries of the locked month.
	for day, want := range map[string]period.State{"2026-10-01": period.Locked, "2026-10-31": period.Locked, "2026-11-01": period.Open, "2026-09-30": period.Open} {
		if f := periodGate(t, fx.legalFixtureState, day); f.State != want || f.Day != day || f.Zone != "" {
			t.Fatal("day gate", day, f.State)
		}
	}
	err := database.WithOrgTx(t.Context(), fx.f.Runtime, fx.scope, func(tx pgx.Tx) error {
		_, err := reviewpg.PeriodGate{}.StateForDayTx(t.Context(), tx, fx.scope, "2026-10-32")
		return err
	})
	if !errors.Is(err, period.ErrInvalid) {
		t.Fatal("invalid day accepted", err)
	}
	// The gate refuses a transaction scoped to another organization.
	err = database.WithOrgTx(t.Context(), fx.f.Runtime, other.scope, func(tx pgx.Tx) error {
		_, err := reviewpg.PeriodGate{}.StateForDayTx(t.Context(), tx, fx.scope, "2026-10-01")
		return err
	})
	if !errors.Is(err, reviewapp.ErrUnavailable) {
		t.Fatal("gate read under another organization's scope", err)
	}
	if _, err := (reviewpg.PeriodGate{}).StateForDayTx(t.Context(), nil, fx.scope, "2026-10-01"); !errors.Is(err, reviewapp.ErrUnavailable) {
		t.Fatal("gate ran without a transaction", err)
	}
}

// Negative "race admits locked write" (B-BR-EMP-214): real concurrent
// transactions with lock-wait barriers. Exactly one ordering wins.
func TestPeriodGateRaceCloseVersusRecordWrite(t *testing.T) {
	fx := periodGateFixture(t)
	occurred := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()

	t.Run("write holds the gate, close waits and locks after the write", func(t *testing.T) {
		gated, proceed := make(chan int, 1), make(chan struct{})
		writeDone := make(chan periodConsumerResult, 1)
		go func() {
			writeDone <- periodConsumerWrite(ctx, fx.legalFixtureState, occurred, "", func(pid int) { gated <- pid; <-proceed })
		}()
		holder := <-gated
		closeDone, closed := make(chan error, 1), make(chan struct{})
		go func() {
			_, _, err := fx.periods.TransitionPeriod(ctx, fx.scope, periodOwner(fx.legalFixtureState), fx.member, onboardingID(t), periodCmd(t, period.ActionClose, "2026-10", 1, ""))
			closeDone <- err
			close(closed)
		}()
		periodAwaitWaiter(t, ctx, fx.legalFixtureState, holder, closed)
		close(proceed)
		write := <-writeDone
		if err := <-closeDone; err != nil {
			t.Fatal("close after the write", err)
		}
		if write.err != nil || !write.written || write.fact.State != period.Open {
			t.Fatal("write holding the gate was not admitted", write.err)
		}
		var ordered bool
		if err := fx.f.Admin.QueryRow(t.Context(), `SELECT (SELECT max(written_at) FROM synthetic_period_records WHERE org_id=$1) < (SELECT recorded_at FROM org_finance_period_transitions WHERE org_id=$1 AND period_month='2026-10-01' AND revision=2)`, fx.scope.OrgID().String()).Scan(&ordered); err != nil || !ordered {
			t.Fatal("the lock was recorded before the admitted write committed", err)
		}
	})

	t.Run("close holds the root, write waits and sees LOCKED", func(t *testing.T) {
		reason := "Synthetic reopening before the race"
		if _, _, err := periodWrite(t, fx.legalFixtureState, fx.periods, periodOwner(fx.legalFixtureState), fx.member, periodCmd(t, period.ActionReopen, "2026-10", 2, reason)); err != nil {
			t.Fatal(err)
		}
		// A proven reopening admits a write of the same day.
		if w := periodConsumerWrite(ctx, fx.legalFixtureState, occurred, "", nil); w.err != nil || !w.written || w.fact.State != period.Reopened || w.fact.Reopening == nil || w.fact.Reopening.Reason != reason {
			t.Fatal("reopened period refused a write", w.err)
		}
		before := periodRecordCount(t, fx.legalFixtureState)
		blocker, err := fx.f.Admin.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = blocker.Rollback(context.Background()) }()
		var blockerPID int
		if err := blocker.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&blockerPID); err != nil {
			t.Fatal(err)
		}
		// Hold the head table: the re-lock stops after it holds the root.
		if _, err := blocker.Exec(ctx, `LOCK TABLE org_finance_periods IN SHARE MODE`); err != nil {
			t.Fatal(err)
		}
		relockDone, relocked := make(chan error, 1), make(chan struct{})
		go func() {
			_, _, err := fx.periods.TransitionPeriod(ctx, fx.scope, periodOwner(fx.legalFixtureState), fx.member, onboardingID(t), periodCmd(t, period.ActionClose, "2026-10", 3, ""))
			relockDone <- err
			close(relocked)
		}()
		commandPID := periodAwaitWaiter(t, ctx, fx.legalFixtureState, blockerPID, relocked)
		writeDone, written := make(chan periodConsumerResult, 1), make(chan struct{})
		go func() {
			writeDone <- periodConsumerWrite(ctx, fx.legalFixtureState, occurred, "", nil)
			close(written)
		}()
		// The record transaction waits on the command's root lock.
		periodAwaitWaiter(t, ctx, fx.legalFixtureState, commandPID, written)
		if err := blocker.Rollback(ctx); err != nil {
			t.Fatal(err)
		}
		if err := <-relockDone; err != nil {
			t.Fatal("re-lock holding the root", err)
		}
		write := <-writeDone
		if write.err != nil || write.written || write.fact.State != period.Locked || write.fact.Revision != 4 || write.fact.Writable() {
			t.Fatal("a write was admitted into a period LOCKED before it committed", write.err)
		}
		if periodRecordCount(t, fx.legalFixtureState) != before {
			t.Fatal("refused write left a record")
		}
	})

	t.Run("unsynchronized writers never commit after the lock", func(t *testing.T) {
		const writers = 6
		november := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
		start := make(chan struct{})
		results := make(chan periodConsumerResult, writers)
		closeDone := make(chan error, 1)
		for range writers {
			go func() { <-start; results <- periodConsumerWrite(ctx, fx.legalFixtureState, november, "", nil) }()
		}
		go func() {
			<-start
			_, _, err := fx.periods.TransitionPeriod(ctx, fx.scope, periodOwner(fx.legalFixtureState), fx.member, onboardingID(t), periodCmd(t, period.ActionClose, "2026-11", 1, ""))
			closeDone <- err
		}()
		close(start)
		if err := <-closeDone; err != nil {
			t.Fatal(err)
		}
		admitted := 0
		for range writers {
			r := <-results
			switch {
			case r.err != nil:
				t.Fatal(r.err)
			case r.written:
				admitted++
				if r.fact.State != period.Open {
					t.Fatal("write admitted from a non-open fact")
				}
			case r.fact.State != period.Locked || r.fact.Revision != 2:
				t.Fatal("refused write did not see the lock")
			}
		}
		var late, count int
		if err := fx.f.Admin.QueryRow(t.Context(), `SELECT count(*) FILTER(WHERE r.written_at>=x.recorded_at),count(*) FROM synthetic_period_records r JOIN org_finance_period_transitions x ON x.org_id=r.org_id AND x.period_month='2026-11-01' AND x.revision=2 WHERE r.org_id=$1 AND r.reporting_day>='2026-11-01' AND r.reporting_day<'2026-12-01'`, fx.scope.OrgID().String()).Scan(&late, &count); err != nil || late != 0 || count != admitted {
			t.Fatal("a write committed into November after it was locked", late, count, admitted, err)
		}
		t.Logf("unsynchronized race: %d of %d writes admitted before the lock", admitted, writers)
	})
}

func periodRecordCount(t *testing.T, fx legalFixtureState) int {
	t.Helper()
	var n int
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT count(*) FROM synthetic_period_records WHERE org_id=$1`, fx.scope.OrgID().String()).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// BUC-EMP-109 / VAL-19 end to end: the gate's facts drive the amendment
// kernel (BR-AMD-004) inside a test-only record transaction.
func TestPeriodGateFeedsAmendmentKernel(t *testing.T) {
	fx := periodGateFixture(t)
	south := periodUnit(t, fx.legalFixtureState, "America/Sao_Paulo")
	fueled := time.Date(2026, 11, 1, 2, 30, 0, 0, time.UTC)
	record := amend.Facts{RecordID: onboardingID(t), State: amend.StateApproved, Effective: 1, Latest: 1}
	decide := func(intent amend.Intent) (amend.Decision, error) {
		t.Helper()
		var decision amend.Decision
		var decided error
		err := database.WithOrgTx(t.Context(), fx.f.Runtime, fx.scope, func(tx pgx.Tx) error {
			gate := reviewpg.PeriodGate{}
			fact, err := gate.StateForOccurrenceTx(t.Context(), tx, fx.scope, fueled, south)
			if err != nil {
				return err
			}
			facts := record
			facts.ReportingDay = fact.Day
			if facts.Period, err = fact.Amendment(); err != nil {
				return err
			}
			request := amend.Request{Intent: intent, Expected: 1, Base: 1, ActorID: fx.actor.AccountID, At: fact.CheckedAt, Reason: "Synthetic VAL-19 volume correction", Authority: amend.Authority{Amend: true, Void: true}}
			switch intent {
			case amend.IntentAmend:
				request.Revision = 2
			case amend.IntentAdjust:
				posting, err := gate.StateForDayTx(t.Context(), tx, fx.scope, "2026-11-14")
				if err != nil {
					return err
				}
				p, err := posting.Amendment()
				if err != nil {
					return err
				}
				request.PostingDay, request.PostingPeriod = posting.Day, &p
			}
			decision, decided = amend.Decide(facts, request)
			return nil
		})
		if err != nil {
			t.Fatal("record transaction", err)
		}
		return decision, decided
	}
	owner := periodOwner(fx.legalFixtureState)
	if d, err := decide(amend.IntentAmend); err != nil || d.Outcome != amend.OutcomeAmendment || d.ReportingDay != "2026-10-31" {
		t.Fatal("amendment in an open October", err)
	}
	if _, _, err := periodWrite(t, fx.legalFixtureState, fx.periods, owner, fx.member, periodCmd(t, period.ActionClose, "2026-10", 1, "")); err != nil {
		t.Fatal(err)
	}
	// (a) PERIOD_LOCKED and (c) the adjustment into the open November.
	if _, err := decide(amend.IntentAmend); !errors.Is(err, amend.ErrPeriodLocked) {
		t.Fatal("amendment into a locked October", err)
	}
	if _, err := decide(amend.IntentVoid); !errors.Is(err, amend.ErrPeriodLocked) {
		t.Fatal("void in a locked October", err)
	}
	if d, err := decide(amend.IntentAdjust); err != nil || d.Outcome != amend.OutcomeAdjustment || d.PostingDay != "2026-11-14" {
		t.Fatal("adjustment into the open period", err)
	}
	// (b) after an authorized reopening with a reason the amendment proceeds.
	if _, _, err := periodWrite(t, fx.legalFixtureState, fx.periods, owner, fx.member, periodCmd(t, period.ActionReopen, "2026-10", 2, "Synthetic VAL-19 reopening")); err != nil {
		t.Fatal(err)
	}
	if d, err := decide(amend.IntentAmend); err != nil || d.Outcome != amend.OutcomeAmendment {
		t.Fatal("amendment after authorized reopening", err)
	}
	if _, err := decide(amend.IntentAdjust); !errors.Is(err, amend.ErrPeriodNotLocked) {
		t.Fatal("adjustment while the period is writable", err)
	}
	if _, _, err := periodWrite(t, fx.legalFixtureState, fx.periods, owner, fx.member, periodCmd(t, period.ActionClose, "2026-10", 3, "")); err != nil {
		t.Fatal(err)
	}
	if _, err := decide(amend.IntentAmend); !errors.Is(err, amend.ErrPeriodLocked) {
		t.Fatal("amendment after re-lock", err)
	}
}
