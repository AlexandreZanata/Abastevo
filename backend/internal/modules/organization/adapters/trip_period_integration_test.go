//go:build integration

package adapters_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	accountpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	reviewpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/review/adapters"
	reviewapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/review/application"
	review "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/review/domain"
	period "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/review/domain/period"
	tripapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/application"
	tripdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/enterprisesuite"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// tripPeriodState is the EMP-TRIP-004 review fixture with the real APPR-006
// period store; a FINANCE member closes and reopens months (BUC-EMP-181).
type tripPeriodState struct {
	tripReviewState
	periods *reviewpg.PeriodStore
	finance tripReviewer
}

func tripPeriodFixture(t *testing.T) tripPeriodState {
	t.Helper()
	x := tripPeriodState{tripReviewState: tripReviewFixture(t)}
	periodGrant(t, x.f)
	x.periods = periodStoreFor(t, x.legalFixtureState)
	x.finance = x.reviewerFor(t, "FINANCE", "ORG", "")
	return x
}

// transition never calls t.Fatal, so it can run inside a goroutine.
func (x tripPeriodState) transition(ctx context.Context, by tripReviewer, command reviewapp.PeriodCommand, id string) (reviewapp.PeriodAck, error) {
	ack, _, err := x.periods.TransitionPeriod(ctx, x.scope, reviewapp.PeriodActor{AccountID: by.actor.AccountID, SessionID: by.actor.SessionID}, by.member, id, command)
	return ack, err
}

func (x tripPeriodState) lock(t *testing.T, month string, expected int64) reviewapp.PeriodAck {
	t.Helper()
	ack, err := x.transition(t.Context(), x.finance, periodCmd(t, period.ActionClose, month, expected, ""), onboardingID(t))
	if err != nil || ack.State != period.Locked {
		t.Fatal("finance could not lock", month, err)
	}
	return ack
}

func (x tripPeriodState) reopen(t *testing.T, month string, expected int64, reason string) reviewapp.PeriodAck {
	t.Helper()
	ack, err := x.transition(t.Context(), x.finance, periodCmd(t, period.ActionReopen, month, expected, reason), onboardingID(t))
	if err != nil || ack.State != period.Reopened {
		t.Fatal("finance could not reopen", month, err)
	}
	return ack
}

// month is the APPR-006 month of a trip's stored start reporting day.
func (x tripPeriodState) month(t *testing.T, trip string) string {
	t.Helper()
	var month string
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT to_char(reporting_day,'YYYY-MM') FROM org_trips WHERE org_id=$1 AND id=$2`, x.scope.OrgID().String(), trip).Scan(&month); err != nil {
		t.Fatal(err)
	}
	return month
}

// submittedTrip is driver d's completed and submitted trip at the instant.
func (x tripPeriodState) submittedTrip(t *testing.T, d syncFixtureState, at time.Time) string {
	t.Helper()
	trip, _ := x.completed(t, d, at)
	x.submitted(t, d, trip, at.Add(90*time.Minute))
	return trip
}

// plantBoundary rewrites, with triggers off for the planted row only, the
// start attribution of a real submitted trip to a month boundary: the start
// instant 2026-11-01T02:30:00Z is November in UTC and on the device
// (Asia/Tokyo, +09:00) but its reporting day in the stored zone
// America/Sao_Paulo (-03:00) is 2026-10-31, as the 000220 rule attributes it.
// The receipt of every decision below is later still (EDR-182, EDR-232).
func (x tripPeriodState) plantBoundary(t *testing.T, trip string) {
	t.Helper()
	if err := pgx.BeginFunc(t.Context(), x.f.Admin, func(tx pgx.Tx) error {
		if _, err := tx.Exec(t.Context(), `SET LOCAL session_replication_role=replica`); err != nil {
			return err
		}
		_, err := tx.Exec(t.Context(), `UPDATE org_trips SET occurred_at='2026-11-01 02:30:00+00',occurred_submicro=0,entry_zone='Asia/Tokyo',device_offset_seconds=32400,
 reporting_zone='America/Sao_Paulo',reporting_day=('2026-11-01 02:30:00+00'::timestamptz AT TIME ZONE 'America/Sao_Paulo')::date WHERE org_id=$1 AND id=$2`, x.scope.OrgID().String(), trip)
		return err
	}); err != nil {
		t.Fatal("boundary trip fixture", err)
	}
	var day, utc, device string
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT reporting_day::text,(occurred_at AT TIME ZONE 'UTC')::date::text,(occurred_at AT TIME ZONE entry_zone)::date::text FROM org_trips WHERE org_id=$1 AND id=$2`,
		x.scope.OrgID().String(), trip).Scan(&day, &utc, &device); err != nil || day != "2026-10-31" || utc != "2026-11-01" || device != "2026-11-01" {
		t.Fatal("boundary trip attribution", day, utc, device, err)
	}
}

func (x tripPeriodState) decideBy(t *testing.T, r tripReviewer, trip string, revision int64, kind review.Kind, reason string) (tripapp.DecisionResult, bool, error) {
	t.Helper()
	return x.decide(x.scope, r, trip, revision, kind, reason, onboardingID(t))
}

// BUC-EMP-181 / B-BR-EMP-359..361 on real PostGIS: the decision gate is
// keyed on the stored start reporting day only; a reopening admits, a
// re-lock withdraws it; refusals write nothing; replays, reads, the queue and
// field intake are not gated; a locked month keeps its approved trip.
func TestTripPeriodLockGatesDecisionsOnTheStoredStartDay(t *testing.T) {
	x := tripPeriodFixture(t)
	d, at := x.assignedDriver(t)
	owner := x.owner()
	var trips []string
	for i := range 5 {
		trips = append(trips, x.submittedTrip(t, d, at.Add(time.Duration(2*i)*time.Hour)))
	}
	early, boundary, stale, current, later := trips[0], trips[1], trips[2], trips[3], trips[4]
	for _, trip := range []string{early, boundary, stale} {
		x.plantBoundary(t, trip)
	}
	month := x.month(t, current)
	if x.month(t, later) != month || month == "2026-10" {
		t.Fatal("the current trips must share a month other than the boundary month", month)
	}
	nothing := func(trip string) {
		t.Helper()
		if entries, audits := x.history(t, x.scope, trip); entries != "SUBMIT:SUBMITTED:1" || audits != 0 {
			t.Fatal("a refused decision wrote history", trip, entries, audits)
		}
	}
	// Wrong reporting zone: November (the UTC, device and receipt month of
	// the boundary trips) is locked; their stored day is October 31.
	november := x.lock(t, "2026-11", 1)
	if month != "2026-11" {
		x.lock(t, month, 1)
	}
	if _, _, err := x.decideBy(t, owner, current, 1, review.Approve, ""); !errors.Is(err, tripapp.ErrPeriodLocked) {
		t.Fatal("a decision in the locked month was not PERIOD_LOCKED", err)
	}
	nothing(current)
	approveOp := onboardingID(t)
	approved, replayed, err := x.decide(x.scope, owner, early, 1, review.Approve, "", approveOp)
	if err != nil || replayed || approved.State != review.Approved {
		t.Fatal("a trip of open October was refused by the November lock of its UTC/device/receipt day", err)
	}
	// Lock October: every decision of the boundary trip is refused, a stale
	// revision keeps its own code, and the approved trip stays approved.
	october := x.lock(t, "2026-10", 1)
	for _, c := range []struct {
		kind   review.Kind
		reason string
	}{{review.Approve, ""}, {review.Return, "Informe o motivo da parada"}, {review.Reject, "Viagem duplicada"}} {
		if _, _, err := x.decideBy(t, owner, boundary, 1, c.kind, c.reason); !errors.Is(err, tripapp.ErrPeriodLocked) {
			t.Fatal("a decision in locked October was not PERIOD_LOCKED", c.kind, err)
		}
	}
	if _, _, err := x.decideBy(t, owner, boundary, 2, review.Approve, ""); !errors.Is(err, review.ErrConflict) {
		t.Fatal("a stale revision in a locked month lost its CONFLICT code", err)
	}
	nothing(boundary)
	again, replayed, err := x.decide(x.scope, owner, early, 1, review.Approve, "", approveOp)
	if err != nil || !replayed || again != approved {
		t.Fatal("the replay of an approval recorded before the lock did not answer", err)
	}
	if entries, audits := x.history(t, x.scope, early); entries != "SUBMIT:SUBMITTED:1,OPEN:IN_REVIEW:1,APPROVE:APPROVED:1" || audits != 1 {
		t.Fatal("the lock changed the approved trip", entries, audits)
	}
	if view, err := x.read(owner, boundary); err != nil || view.Review != tripdomain.ReviewSubmitted || view.ReportingDay != "2026-10-31" {
		t.Fatal("the decision context of a locked trip is not readable", err)
	}
	if page, err := x.reviews.ListTripReviews(t.Context(), x.scope, owner.actor, owner.member, tripapp.QueueFilter{Limit: 10}); err != nil || len(page.Items) != 4 {
		t.Fatal("trips of locked months left the review queue", err, len(page.Items))
	}
	// Field intake is not gated: a new trip of the locked month is started,
	// finished and submitted (B-BR-EMP-360).
	field := x.submittedTrip(t, d, at.Add(12*time.Hour))
	if x.month(t, field) != month {
		t.Fatal("the field trip left the locked month")
	}
	nothing(field)
	// An authorised reopening with a reason admits the decision.
	reason := "Correção de viagens de outubro"
	reopened := x.reopen(t, "2026-10", october.Revision, reason)
	if view, err := x.read(owner, stale); err != nil || view.Review != tripdomain.ReviewSubmitted {
		t.Fatal("decision context during the reopening", err)
	}
	returned, _, err := x.decideBy(t, owner, boundary, 1, review.Return, "Informe o motivo da parada")
	if err != nil || returned.State != review.Returned {
		t.Fatal("a decision in reopened October was refused", err)
	}
	// Stale authority: the re-lock withdraws the reopening before the
	// decision prepared during it is received.
	relocked := x.lock(t, "2026-10", reopened.Revision)
	if relocked.Revision != 4 {
		t.Fatal("re-lock revision", relocked.Revision)
	}
	if _, _, err := x.decideBy(t, owner, stale, 1, review.Approve, ""); !errors.Is(err, tripapp.ErrPeriodLocked) {
		t.Fatal("a decision after the re-lock was admitted on a withdrawn reopening", err)
	}
	nothing(stale)
	// The histories keep the lock, the reasoned reopening, the re-lock and
	// the decisions in the order the root lock serialised them.
	var ordered bool
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT
 (SELECT string_agg(kind||':'||revision||':'||reason,',' ORDER BY revision) FROM org_finance_period_transitions WHERE org_id=$1 AND period_month='2026-10-01')=$4
 AND(SELECT recorded_at FROM org_trip_review_entries WHERE org_id=$1 AND trip_id=$2 AND kind='APPROVE')<(SELECT recorded_at FROM org_finance_period_transitions WHERE org_id=$1 AND period_month='2026-10-01' AND revision=2)
 AND(SELECT recorded_at FROM org_trip_review_entries WHERE org_id=$1 AND trip_id=$3 AND kind='RETURN') BETWEEN
  (SELECT recorded_at FROM org_finance_period_transitions WHERE org_id=$1 AND period_month='2026-10-01' AND revision=3)
  AND(SELECT recorded_at FROM org_finance_period_transitions WHERE org_id=$1 AND period_month='2026-10-01' AND revision=4)`,
		x.scope.OrgID().String(), early, boundary, "LOCK:2:,REOPEN:3:"+reason+",RELOCK:4:").Scan(&ordered); err != nil || !ordered {
		t.Fatal("period and decision histories are not kept in order", err)
	}
	if entries, audits := x.history(t, x.scope, boundary); entries != "SUBMIT:SUBMITTED:1,OPEN:IN_REVIEW:1,RETURN:RETURNED:1" || audits != 1 {
		t.Fatal("the decision made during the reopening was not kept", entries, audits)
	}
	if november.Revision != 2 {
		t.Fatal("november lock revision", november.Revision)
	}
}

// heldPeriodGate is the real APPR-006 gate that pauses the decision after
// its period read, while the decision holds the organization root.
type heldPeriodGate struct {
	reviewpg.PeriodGate
	hold func(pid int)
}

func (g heldPeriodGate) StateForDayTx(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, day string) (reviewapp.PeriodFact, error) {
	fact, err := g.PeriodGate.StateForDayTx(ctx, tx, scope, day)
	if err != nil {
		return fact, err
	}
	var pid int
	if err := tx.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&pid); err != nil {
		return reviewapp.PeriodFact{}, err
	}
	g.hold(pid)
	return fact, nil
}

type tripDecisionOutcome struct {
	result tripapp.DecisionResult
	err    error
}

// B-BR-EMP-359/362 "lock race": real concurrent transactions with lock-wait
// barriers in both orders; exactly the serialised order wins.
func TestTripPeriodLockRacesWithTheDecisionInBothOrders(t *testing.T) {
	x := tripPeriodFixture(t)
	d, at := x.assignedDriver(t)
	first, second := x.submittedTrip(t, d, at), x.submittedTrip(t, d, at.Add(2*time.Hour))
	month := x.month(t, first)
	owner := x.owner()
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	command := func(trip string, kind review.Kind) tripapp.DecisionCommand {
		t.Helper()
		c, err := tripapp.NewDecisionCommand(tripapp.DecisionInput{OperationID: onboardingID(t), TripID: trip, ExpectedRevision: 1, Decision: kind})
		if err != nil {
			t.Fatal(err)
		}
		return c
	}

	t.Run("decision holds the root, the lock waits and commits after it", func(t *testing.T) {
		gated, proceed := make(chan int, 1), make(chan struct{})
		held := *x.reviews
		held.Periods = heldPeriodGate{hold: func(pid int) { gated <- pid; <-proceed }}
		approve := command(first, review.Approve)
		decided := make(chan tripDecisionOutcome, 1)
		go func() {
			r, _, err := held.DecideTripReview(ctx, x.scope, owner.actor, owner.member, approve)
			decided <- tripDecisionOutcome{r, err}
		}()
		holder := <-gated
		lockCommand, lockID := periodCmd(t, period.ActionClose, month, 1, ""), onboardingID(t)
		lockDone, locked := make(chan error, 1), make(chan struct{})
		go func() {
			_, err := x.transition(ctx, x.finance, lockCommand, lockID)
			lockDone <- err
			close(locked)
		}()
		periodAwaitWaiter(t, ctx, x.legalFixtureState, holder, locked)
		close(proceed)
		outcome := <-decided
		if err := <-lockDone; err != nil {
			t.Fatal("the lock after the decision", err)
		}
		if outcome.err != nil || outcome.result.State != review.Approved {
			t.Fatal("the decision holding the gate was not admitted", outcome.err)
		}
		var ordered bool
		if err := x.f.Admin.QueryRow(t.Context(), `SELECT(SELECT recorded_at FROM org_trip_review_entries WHERE org_id=$1 AND trip_id=$2 AND kind='APPROVE')
 <(SELECT recorded_at FROM org_finance_period_transitions WHERE org_id=$1 AND period_month=($3||'-01')::date AND revision=2)`, x.scope.OrgID().String(), first, month).Scan(&ordered); err != nil || !ordered {
			t.Fatal("the lock was recorded before the admitted decision committed", err)
		}
		if entries, audits := x.history(t, x.scope, first); entries != "SUBMIT:SUBMITTED:1,OPEN:IN_REVIEW:1,APPROVE:APPROVED:1" || audits != 1 {
			t.Fatal("the locked month did not keep its approved trip", entries, audits)
		}
	})

	t.Run("re-lock holds the root, the decision waits and sees LOCKED", func(t *testing.T) {
		x.reopen(t, month, 2, "Revisão de viagens do mês")
		blocker, err := x.f.Admin.Begin(ctx)
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
		relockCommand, relockID := periodCmd(t, period.ActionClose, month, 3, ""), onboardingID(t)
		relockDone, relocked := make(chan error, 1), make(chan struct{})
		go func() {
			_, err := x.transition(ctx, x.finance, relockCommand, relockID)
			relockDone <- err
			close(relocked)
		}()
		commandPID := periodAwaitWaiter(t, ctx, x.legalFixtureState, blockerPID, relocked)
		approve := command(second, review.Approve)
		decided, done := make(chan tripDecisionOutcome, 1), make(chan struct{})
		go func() {
			r, _, err := x.reviews.DecideTripReview(ctx, x.scope, owner.actor, owner.member, approve)
			decided <- tripDecisionOutcome{r, err}
			close(done)
		}()
		// The decision waits on the command's organization root lock.
		periodAwaitWaiter(t, ctx, x.legalFixtureState, commandPID, done)
		if err := blocker.Rollback(ctx); err != nil {
			t.Fatal(err)
		}
		if err := <-relockDone; err != nil {
			t.Fatal("re-lock holding the root", err)
		}
		if outcome := <-decided; !errors.Is(outcome.err, tripapp.ErrPeriodLocked) {
			t.Fatal("a decision was admitted into a month LOCKED before it committed", outcome.err)
		}
		if entries, audits := x.history(t, x.scope, second); entries != "SUBMIT:SUBMITTED:1" || audits != 0 {
			t.Fatal("the refused decision left history", entries, audits)
		}
	})
}

// B-BR-EMP-362 / EDR-233: the database refuses an opening in a locked month
// whatever the handler, admits it in a reopened month, and refuses in-place
// rewrites of the trip, its review history and the period history.
func TestTripPeriodDatabaseRefusesLockedDecisionsAndRewrites(t *testing.T) {
	x := tripPeriodFixture(t)
	d, at := x.assignedDriver(t)
	trip := x.submittedTrip(t, d, at)
	approved := x.submittedTrip(t, d, at.Add(2*time.Hour))
	month := x.month(t, trip)
	owner := x.owner()
	if _, _, err := x.decideBy(t, owner, approved, 1, review.Approve, ""); err != nil {
		t.Fatal(err)
	}
	reviewer := x.reviewerFor(t, "SUPERVISOR", "ORG", "")
	org := x.scope.OrgID().String()
	locked := x.lock(t, month, 1)
	code := func(err error, want, message string) {
		t.Helper()
		var pe *pgconn.PgError
		if !errors.As(err, &pe) || pe.Code != want || message != "" && pe.Message != message {
			t.Fatalf("SQL refusal mismatch, want %s %q: %v", want, message, err)
		}
		if strings.Contains(pe.Message+pe.Detail, "Informe") {
			t.Fatal("SQL refusal leaked a private reason")
		}
	}
	open := `INSERT INTO org_trip_review_entries(org_id,trip_id,sequence,kind,revision,state,reason,actor_membership_id,actor_employee_id,operation_id)
VALUES($1,$2,2,'OPEN',1,'IN_REVIEW','',$3,$4,$5)`
	code(unitSchemaExec(t, x.legalFixtureState, open, org, trip, reviewer.member, reviewer.employee, onboardingID(t)), "55000", "trip reporting period locked")
	if entries, audits := x.history(t, x.scope, trip); entries != "SUBMIT:SUBMITTED:1" || audits != 0 {
		t.Fatal("a refused direct opening left history", entries)
	}
	// In a reopened month the period clause admits the row; the publication
	// guard still refuses an opening without its decision at commit.
	x.reopen(t, month, locked.Revision, "Revisão de viagens do mês")
	code(unitSchemaExec(t, x.legalFixtureState, open, org, trip, reviewer.member, reviewer.employee, onboardingID(t)), "23514", "trip review opening without decision")
	x.lock(t, month, locked.Revision+1)
	role := pgx.Identifier{x.f.RuntimeRole}.Sanitize()
	if _, err := x.f.Admin.Exec(t.Context(), "GRANT UPDATE,DELETE,TRUNCATE ON org_trips,org_trip_review_entries,org_finance_period_transitions,org_finance_periods TO "+role); err != nil {
		t.Fatal(err)
	}
	before := x.tripRow(t, approved)
	beforeHistory, beforeAudits := x.history(t, x.scope, approved)
	beforePeriods := periodPrivateSnapshot(t, x.legalFixtureState)
	// In-place rewrites of a locked month's approved facts.
	code(unitSchemaExec(t, x.legalFixtureState, `UPDATE org_trips SET reporting_day=reporting_day+40 WHERE org_id=$1 AND id=$2`, org, approved), "55000", "")
	code(unitSchemaExec(t, x.legalFixtureState, `UPDATE org_trip_review_entries SET state='REJECTED',kind='REJECT',reason='rewritten' WHERE org_id=$1 AND trip_id=$2 AND kind='APPROVE'`, org, approved), "55000", "")
	code(unitSchemaExec(t, x.legalFixtureState, `DELETE FROM org_trip_review_entries WHERE org_id=$1 AND trip_id=$2`, org, approved), "55000", "")
	code(unitSchemaExec(t, x.legalFixtureState, `DELETE FROM org_trips WHERE org_id=$1 AND id=$2`, org, approved), "55000", "")
	code(unitSchemaExec(t, x.legalFixtureState, `UPDATE org_finance_period_transitions SET reason='rewritten' WHERE org_id=$1`, org), "55000", "")
	code(unitSchemaExec(t, x.legalFixtureState, `UPDATE org_finance_periods SET state='REOPENED',revision=revision+1 WHERE org_id=$1`, org), "23514", "")
	code(unitSchemaExec(t, x.legalFixtureState, `DELETE FROM org_finance_periods WHERE org_id=$1`, org), "55000", "")
	if err := unitSchemaExec(t, x.legalFixtureState, `TRUNCATE org_trip_review_entries`); err == nil {
		t.Fatal("trip review history truncated")
	}
	afterHistory, afterAudits := x.history(t, x.scope, approved)
	if x.tripRow(t, approved) != before || afterHistory != beforeHistory || afterAudits != beforeAudits || periodPrivateSnapshot(t, x.legalFixtureState) != beforePeriods {
		t.Fatal("a refused rewrite changed the locked month")
	}
	if _, _, err := x.decideBy(t, owner, trip, 1, review.Approve, ""); !errors.Is(err, tripapp.ErrPeriodLocked) {
		t.Fatal("the re-locked month admitted a decision", err)
	}
}

// The composed production suite wires the gate: the actual company route
// answers 409 PERIOD_LOCKED and writes nothing (B-BR-EMP-359).
func TestTripPeriodLockThroughComposedCompanyHTTP(t *testing.T) {
	x := tripPeriodFixture(t)
	router := chi.NewRouter()
	runtimeAccounts := *x.accounts
	runtimeAccounts.Store = accountpg.NewPGStore(x.f.Runtime)
	if err := enterprisesuite.Register(router, enterprisesuite.Options{Enabled: true, Origin: deviceTestOrigin, CursorSecret: bytes.Repeat([]byte{31}, 32), Pool: x.f.Runtime, Accounts: &runtimeAccounts}); err != nil {
		t.Fatal("actual trip review composition failed", err)
	}
	fx := x.deviceFixtureState
	owner := matrixNewPerson(t, fx, router)
	matrixAssign(t, fx, owner, "OWNER", "ORG", "ACTIVE")
	d, at := x.assignedDriver(t)
	trip := x.submittedTrip(t, d, at)
	month := x.month(t, trip)
	locked := x.lock(t, month, 1)
	decide := "/v1/orgs/" + x.scope.OrgID().String() + "/trips/" + trip + "/review/decide"
	body := func(op string) map[string]any {
		return map[string]any{"schema_version": 1, "operation_id": op, "revision": 1, "decision": "APPROVE", "reason": ""}
	}
	matrixCall(t, fx, router, owner, decide, body(onboardingID(t)), 409, "PERIOD_LOCKED", "", nil)
	if entries, audits := x.history(t, x.scope, trip); entries != "SUBMIT:SUBMITTED:1" || audits != 0 {
		t.Fatal("the refused HTTP decision left history", entries, audits)
	}
	x.reopen(t, month, locked.Revision, "Revisão de viagens do mês")
	if approved := matrixCall(t, fx, router, owner, decide, body(onboardingID(t)), 200, "", "", nil); approved["review_state"] != "APPROVED" {
		t.Fatal("the reopened month refused the HTTP decision", approved)
	}
}
