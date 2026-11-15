//go:build integration

package adapters_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	permission "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	orgpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters"
	auditpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/adapters"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	syncpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	review "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/review/domain"
	trippg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/adapters"
	tripapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/application"
	tripdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	closureReason  = "Motorista sem sinal; viagem encerrada pela supervisão"
	closureMissing = "Leitura final não registrada"
)

// tripClosureState is the EMP-TRIP-015 fixture (intake with start, finish,
// submit and logbook owners, review and amendment store) with the real
// EMP-TRIP-014 closure store.
type tripClosureState struct {
	tripAmendState
	closures *trippg.ClosureStore
}

func tripClosureStoreOf(t *testing.T, fx legalFixtureState, pool *pgxpool.Pool) *trippg.ClosureStore {
	t.Helper()
	catalog, err := audit.NewCatalog(trippg.ClosureAuditDefinitions())
	if err != nil {
		t.Fatal(err)
	}
	return &trippg.ClosureStore{Pool: pool, Authority: &orgpg.TripReviewAuthority{Memberships: fx.store.Memberships, Roles: fx.store.Roles, Capabilities: fx.flags,
		Structure: &orgpg.ResourceStructure{Kind: orgpg.TripStructure, Pool: pool}}, Audit: auditpg.NewStore(pool, catalog)}
}

func tripClosureFixture(t *testing.T) tripClosureState {
	t.Helper()
	x := tripClosureState{tripAmendState: tripAmendFixture(t)}
	x.closures = tripClosureStoreOf(t, x.legalFixtureState, x.f.Runtime)
	return x
}

// close never calls t.Fatal, so it is safe inside a goroutine.
func (x tripClosureState) close(scope kernel.OrgScope, r tripReviewer, trip string, revision int64, reason, missing, operation string) (tripapp.CloseResult, bool, error) {
	command, err := tripapp.NewCloseCommand(tripapp.CloseInput{OperationID: operation, TripID: trip, ExpectedRevision: revision, Reason: reason, MissingReason: missing})
	if err != nil {
		return tripapp.CloseResult{}, false, err
	}
	return x.closures.CloseTrip(context.Background(), scope, r.actor, r.member, command)
}

func (x tripClosureState) open(t *testing.T, r tripReviewer) tripapp.OpenTripPage {
	t.Helper()
	page, err := x.closures.ListOpenTrips(context.Background(), x.scope, r.actor, r.member, tripapp.QueueFilter{Limit: 10})
	if err != nil {
		t.Fatal("open-trip list", err)
	}
	return page
}

// running is member d's accepted trip start at the given instant.
func (x tripClosureState) running(t *testing.T, d syncFixtureState, at time.Time, edit func(envelope, payload map[string]any)) string {
	t.Helper()
	start := x.startOp(t, d, at, edit)
	ack, err := x.process(t, d, start)
	readings := 1
	if edit != nil {
		readings = 0
	}
	x.accepted(t, start, ack, err, 1, readings)
	return tripOf(start)
}

type tripClosureFacts struct {
	state                                   string
	closures, finishes, endReadings, audits int
}

func (x tripClosureState) closureFacts(t *testing.T, trip string) tripClosureFacts {
	t.Helper()
	var c tripClosureFacts
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT(SELECT state FROM org_trips WHERE org_id=$1 AND id=$2),(SELECT count(*) FROM org_trip_closures WHERE org_id=$1 AND trip_id=$2),
(SELECT count(*) FROM org_trip_finishes WHERE org_id=$1 AND trip_id=$2),(SELECT count(*) FROM org_meter_readings WHERE org_id=$1 AND origin='TRIP_END'),
(SELECT count(*) FROM org_audit_events WHERE org_id=$1 AND target_type='trip' AND target_id=$2 AND action='trip.trip.closed')`, x.scope.OrgID().String(), trip).Scan(&c.state, &c.closures, &c.finishes, &c.endReadings, &c.audits); err != nil {
		t.Fatal(err)
	}
	return c
}

// BUC-EMP-202 / JRN-07 / B-BR-EMP-399..403 on real PostGIS: a supervisor
// closes another member's open trip with a reason and the missing-reading
// reason; nothing is invented, the closure replays, the driver's later finish
// and events are held, the closer never decides the review and the missing
// final reading cannot be "corrected" into existence.
func TestTripClosureClosesAnotherMembersOpenTripWithoutInventingEvidence(t *testing.T) {
	x := tripClosureFixture(t)
	ctx := context.Background()
	d, at := x.assignedDriver(t)
	trip := x.running(t, d, at, nil)
	supervisor := x.reviewerFor(t, "SUPERVISOR", "ORG", "")
	page := x.open(t, supervisor)
	if len(page.Items) != 1 || page.NextID != "" {
		t.Fatal("open-trip list", len(page.Items))
	}
	if item := page.Items[0]; item.TripID != trip || item.AssetID != x.vehicle.AssetID || item.DriverEmployeeID != d.person.employee || item.AuthorMembershipID != d.source.MembershipID ||
		item.Revision != 1 || item.ReportingDay == "" || !item.StartedAt.Equal(at) {
		t.Fatal("open-trip list item", item.TripID == trip, item.Revision)
	}
	var readingsBefore int
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT count(*) FROM org_meter_readings WHERE org_id=$1`, x.scope.OrgID().String()).Scan(&readingsBefore); err != nil {
		t.Fatal(err)
	}
	op := onboardingID(t)
	result, replayed, err := x.close(x.scope, supervisor, trip, 1, closureReason, closureMissing, op)
	if err != nil || replayed {
		t.Fatal("the supervisor closure was refused", err)
	}
	if result.TripID != trip || result.Operational != tripdomain.Completed || result.Revision != 2 || result.Closure.CloserMembershipID != supervisor.member ||
		result.Closure.CloserEmployeeID != supervisor.employee || result.Closure.Reason != closureReason || result.Closure.MissingReason != closureMissing ||
		result.Closure.ClosedAt.IsZero() || result.UsageStatus != tripdomain.NotMeasured || result.UsageReason != tripdomain.ReasonReadingAbsent {
		t.Fatal("closure result", result.Operational, result.Revision, result.UsageStatus, result.UsageReason)
	}
	if c := x.closureFacts(t, trip); c != (tripClosureFacts{state: "COMPLETED", closures: 1, audits: 1}) {
		t.Fatalf("closure not stored exactly once without a finish or end reading: %+v", c)
	}
	var readingsAfter int
	var exact bool
	command, _ := tripapp.NewCloseCommand(tripapp.CloseInput{OperationID: op, TripID: trip, ExpectedRevision: 1, Reason: closureReason, MissingReason: closureMissing})
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT(SELECT count(*) FROM org_meter_readings WHERE org_id=$1),EXISTS(SELECT 1 FROM org_trip_closures c JOIN org_audit_events a ON(a.org_id,a.operation_id)=(c.org_id,c.operation_id)
 WHERE c.org_id=$1 AND c.trip_id=$2 AND c.operation_id=$3 AND c.request_hash=$4 AND c.closer_membership_id=$5 AND c.closer_employee_id=$6 AND c.reason=$7 AND c.missing_reading_reason=$8
 AND a.actor_id=$9 AND a.action='trip.trip.closed' AND a.request_hash=$4 AND strpos(to_jsonb(a)::text,'supervisão')=0 AND strpos(to_jsonb(a)::text,'registrada')=0)`,
		x.scope.OrgID().String(), trip, op, command.Digest(x.scope), supervisor.member, supervisor.employee, closureReason, closureMissing, supervisor.account).Scan(&readingsAfter, &exact); err != nil || !exact {
		t.Fatal("stored closure or its audit event is not exact or carries reason text", err)
	}
	if readingsAfter != readingsBefore {
		t.Fatal("a closure created a reading", readingsBefore, readingsAfter)
	}
	if page := x.open(t, supervisor); len(page.Items) != 0 {
		t.Fatal("a closed trip is still listed as open")
	}
	// Replay and reuse (B-BR-EMP-403).
	again, replayed, err := x.close(x.scope, supervisor, trip, 1, closureReason, closureMissing, op)
	if err != nil || !replayed || again.Closure.ClosedAt != result.Closure.ClosedAt || again.Revision != 2 || again.UsageReason != tripdomain.ReasonReadingAbsent {
		t.Fatal("closure replay", err, replayed)
	}
	if _, _, err := x.close(x.scope, supervisor, trip, 1, "Outro motivo", closureMissing, op); !errors.Is(err, tripapp.ErrOperationReused) {
		t.Fatal("another digest under the closure operation", err)
	}
	// A stale second closure, at either revision, is CONFLICT and writes nothing.
	for _, revision := range []int64{1, 2} {
		if _, _, err := x.close(x.scope, x.owner(), trip, revision, closureReason, closureMissing, onboardingID(t)); !errors.Is(err, tripapp.ErrClosureConflict) {
			t.Fatal("a second closure was not CONFLICT", revision, err)
		}
	}
	if c := x.closureFacts(t, trip); c != (tripClosureFacts{state: "COMPLETED", closures: 1, audits: 1}) {
		t.Fatalf("a refused closure changed state: %+v", c)
	}
	// The decision context shows the closure and the absent finish.
	owner := x.owner()
	view, err := x.read(owner, trip)
	if err != nil || view.Operational != tripdomain.Completed || view.FinishedAt != nil || view.Closure == nil || view.Closure.MissingReason != closureMissing ||
		view.Closure.Reason != closureReason || view.Closure.CloserMembershipID != supervisor.member || view.Review != tripdomain.ReviewDraft {
		t.Fatal("decision context of a closed trip", err)
	}
	// B-BR-EMP-402: the driver's later finish and events are held, never applied.
	finish := x.finishOp(t, d, trip, at.Add(time.Hour), nil)
	ack, err := x.process(t, d, finish)
	x.held(t, finish, ack, err, tripapp.TripAlreadyClosed)
	x.replays(t, d, finish, ack)
	var heldValue int64
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT reading_value FROM org_trip_held_finishes WHERE org_id=$1 AND operation_id=$2`, x.scope.OrgID().String(), finish.ID()).Scan(&heldValue); err != nil || heldValue != 184_610_000 {
		t.Fatal("the held finish lost its declared reading", heldValue, err)
	}
	x.heldEvent(t, d, x.eventOp(t, d, trip, "STOP", nil, at.Add(30*time.Minute)), tripapp.TripAlreadyClosed)
	x.heldEvent(t, d, x.eventOp(t, d, trip, "NOTE", "Chegou ao pátio", at.Add(5*time.Hour)), tripapp.TripAlreadyClosed)
	if c := x.closureFacts(t, trip); c != (tripClosureFacts{state: "COMPLETED", closures: 1, audits: 1}) {
		t.Fatalf("a late field fact completed the closed trip again: %+v", c)
	}
	// The closed trip no longer holds the vehicle; no end instant was invented.
	next := x.running(t, d, at.Add(3*time.Hour), absentReading)
	if page := x.open(t, supervisor); len(page.Items) != 1 || page.Items[0].TripID != next {
		t.Fatal("the next trip on the freed vehicle is not the only open trip")
	}
	// B-BR-EMP-401: the author submits; the closer never decides; another reviewer does.
	x.submitted(t, d, trip, at.Add(4*time.Hour))
	if _, _, err := x.decide(x.scope, supervisor, trip, 1, review.Approve, "", onboardingID(t)); !errors.Is(err, review.ErrSelfApproval) {
		t.Fatal("the closer decided the closed trip", err)
	}
	if _, _, err := x.decide(x.scope, supervisor, trip, 1, review.Return, "Informe a leitura", onboardingID(t)); !errors.Is(err, review.ErrSelfApproval) {
		t.Fatal("the closer returned the closed trip", err)
	}
	if entries, _ := x.history(t, x.scope, trip); entries != "SUBMIT:SUBMITTED:1" {
		t.Fatal("a refused closer decision wrote history", entries)
	}
	x.approve(t, owner, trip, 1)
	if view, err := x.read(owner, trip); err != nil || view.EffectiveUsage == nil || view.EffectiveUsage.Status != tripdomain.NotMeasured || view.EffectiveUsage.Reason != tripdomain.ReasonReadingAbsent {
		t.Fatal("the approved closed trip reported a distance", err)
	}
	// A final reading that never existed cannot be "corrected" into the trip.
	if _, _, err := x.propose(x.reviews, x.manager, trip, 1, nil, odometer(184_600_000), "Leitura final informada depois", onboardingID(t)); !errors.Is(err, tripapp.ErrInvalidAmendment) {
		t.Fatal("an amendment invented the missing final reading", err)
	}
	var amendments int
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT count(*) FROM org_trip_amendments WHERE org_id=$1 AND trip_id=$2`, x.scope.OrgID().String(), trip).Scan(&amendments); err != nil || amendments != 0 {
		t.Fatal("a refused amendment was stored", amendments, err)
	}
	_ = ctx
}

// B-BR-EMP-399/401 negatives: self-closure, another organization's and an
// unknown trip, uncovering structural grants, denied roles and the
// organization-level list.
func TestTripClosureRefusesSelfClosureForeignUncoveredAndDeniedRoles(t *testing.T) {
	x := tripClosureFixture(t)
	org := x.scope.OrgID().String()
	at := x.declaredNow(t)
	// The ADMIN (ORG, holding trip.trip.close_for_other) drives its own trip.
	own := x.running(t, x.syncFixtureState, at, absentReading)
	admin := tripReviewer{actor: tripapp.ReviewActor{AccountID: x.person.web.AccountID, SessionID: x.person.web.SessionID}, member: x.person.member, employee: x.person.employee, account: x.person.auth.Account.ID}
	if _, _, err := x.close(x.scope, admin, own, 1, closureReason, closureMissing, onboardingID(t)); !errors.Is(err, tripdomain.ErrSelfClosure) {
		t.Fatal("self-closure was not refused", err)
	}
	if c := x.closureFacts(t, own); c != (tripClosureFacts{state: "IN_PROGRESS"}) {
		t.Fatalf("a refused self-closure changed state: %+v", c)
	}
	d, _ := x.assignedDriver(t)
	sul := onboardingID(t)
	if err := unitSchemaExec(t, x.legalFixtureState, `INSERT INTO org_units(org_id,id,name,code,time_zone,state) VALUES($1,$2,'Synthetic Sul','SUL','UTC','ACTIVE')`, org, sul); err != nil {
		t.Fatal(err)
	}
	unchanged := func(name string) {
		t.Helper()
		if c := x.closureFacts(t, own); c != (tripClosureFacts{state: "IN_PROGRESS"}) {
			t.Fatalf("%s changed state: %+v", name, c)
		}
	}
	denied := func(name string, r tripReviewer) {
		t.Helper()
		if _, _, err := x.close(x.scope, r, own, 1, closureReason, closureMissing, onboardingID(t)); !errors.Is(err, permission.ErrPermissionDenied) {
			t.Fatal("closure not FORBIDDEN", name, err)
		}
		unchanged(name)
	}
	denied("auditor", x.reviewerFor(t, "AUDITOR", "ORG", ""))
	denied("finance", x.reviewerFor(t, "FINANCE", "ORG", ""))
	denied("approver", x.reviewerFor(t, "APPROVER", "ORG", ""))
	denied("driver", x.reviewerFor(t, "DRIVER", "SELF", ""))
	denied("supervisor of another unit", x.reviewerFor(t, "SUPERVISOR", "UNIT", sul))
	denied("fleet manager of another asset", x.reviewerFor(t, "FLEET_MANAGER", "ASSET", x.installed.AssetID))
	// The list is organization-level: a structural grant does not list it.
	norte := x.reviewerFor(t, "SUPERVISOR", "UNIT", x.unit)
	if _, err := x.closures.ListOpenTrips(context.Background(), x.scope, norte.actor, norte.member, tripapp.QueueFilter{Limit: 5}); !errors.Is(err, permission.ErrPermissionDenied) {
		t.Fatal("a UNIT grant listed the organization's open trips", err)
	}
	auditor := x.reviewerFor(t, "AUDITOR", "ORG", "")
	if _, err := x.closures.ListOpenTrips(context.Background(), x.scope, auditor.actor, auditor.member, tripapp.QueueFilter{Limit: 5}); !errors.Is(err, permission.ErrPermissionDenied) {
		t.Fatal("an AUDITOR listed the organization's open trips", err)
	}
	// Another organization's owner naming this trip, and an unknown trip.
	other := legalFixtureFor(t, x.f, x.onboarding, x.legalFixtureState.actor, x.flags)
	if _, _, err := other.onboarding.Repository.ActivateOrganization(t.Context(), other.scope, other.actor, other.member, onboardingID(t), 1); err != nil {
		t.Fatal(err)
	}
	stranger := tripReviewer{actor: tripapp.ReviewActor{AccountID: other.actor.AccountID, SessionID: other.actor.SessionID}, member: other.member}
	if _, _, err := x.close(other.scope, stranger, own, 1, closureReason, closureMissing, onboardingID(t)); !errors.Is(err, tripapp.ErrNotFound) {
		t.Fatal("another organization's trip was not NOT_FOUND", err)
	}
	if page, err := x.closures.ListOpenTrips(context.Background(), other.scope, stranger.actor, stranger.member, tripapp.QueueFilter{Limit: 5}); err != nil || len(page.Items) != 0 {
		t.Fatal("another organization listed this organization's open trips", err)
	}
	if _, _, err := x.close(x.scope, x.owner(), onboardingID(t), 1, closureReason, closureMissing, onboardingID(t)); !errors.Is(err, tripapp.ErrNotFound) {
		t.Fatal("an unknown trip was not NOT_FOUND", err)
	}
	unchanged("foreign and unknown closures")
	// The trip's stored unit covers it (BR-AUTH-005).
	if result, _, err := x.close(x.scope, norte, own, 1, closureReason, closureMissing, onboardingID(t)); err != nil || result.Operational != tripdomain.Completed {
		t.Fatal("a covering UNIT supervisor could not close the trip", err)
	}
	// A covering ASSET fleet manager closes another member's trip on the vehicle.
	trip := x.running(t, d, x.declaredNow(t), absentReading)
	vehicle := x.reviewerFor(t, "FLEET_MANAGER", "ASSET", x.vehicle.AssetID)
	if result, _, err := x.close(x.scope, vehicle, trip, 1, closureReason, closureMissing, onboardingID(t)); err != nil || result.Revision != 2 {
		t.Fatal("a covering ASSET fleet manager could not close the trip", err)
	}
}

type tripClosureOutcome struct {
	result tripapp.CloseResult
	ack    protocol.Acknowledgement
	err    error
}

// B-BR-EMP-402: a driver finish and a supervisor closure of one trip queued
// behind the held organization root resolve in arrival order: finish first
// leaves the closure CONFLICT with nothing written; closure first holds the
// finish TRIP_ALREADY_CLOSED. Two closures leave one.
func TestTripClosureRacesTheDriverFinishInBothOrders(t *testing.T) {
	for _, order := range []string{"finish-first", "closure-first", "two-closures"} {
		t.Run(order, func(t *testing.T) {
			x := tripClosureFixture(t)
			d, at := x.assignedDriver(t)
			trip := x.running(t, d, at, absentReading)
			supervisor := x.reviewerFor(t, "SUPERVISOR", "ORG", "")
			manager := x.reviewerFor(t, "FLEET_MANAGER", "ORG", "")
			finish := x.finishOp(t, d, trip, at.Add(time.Hour), absentReading)
			pool := x.f.RuntimePool(t, 4)
			config := tripAmendIntakeConfig(x.syncFixtureState)
			config.Pool = pool
			intake := syncpg.NewStore(config)
			x.closures = tripClosureStoreOf(t, x.legalFixtureState, pool)
			holder := make(chan int, 1)
			release := make(chan struct{})
			var once sync.Once
			free := func() { once.Do(func() { close(release) }) }
			defer free()
			done := make(chan error, 1)
			go func() {
				done <- database.WithOrgTx(context.Background(), x.f.Runtime, x.scope, func(tx pgx.Tx) error {
					if _, err := tx.Exec(context.Background(), `SELECT state FROM organizations WHERE org_id=$1 FOR UPDATE`, x.scope.OrgID().String()); err != nil {
						return err
					}
					var pid int
					if err := tx.QueryRow(context.Background(), `SELECT pg_backend_pid()`).Scan(&pid); err != nil {
						return err
					}
					holder <- pid
					<-release
					return nil
				})
			}()
			var pid int
			select {
			case pid = <-holder:
			case err := <-done:
				t.Fatal("holder failed", err)
			}
			outcomes := map[string]*tripClosureOutcome{"finish": {}, "closure": {}, "closure2": {}}
			var wg sync.WaitGroup
			runFinish := func() {
				wg.Go(func() {
					ack, err := intake.Process(context.Background(), d.source, finish, protocol.DefaultLimits(), x.finish)
					outcomes["finish"].ack, outcomes["finish"].err = ack, err
				})
			}
			runClosure := func(key string, r tripReviewer) {
				wg.Go(func() {
					result, _, err := x.close(x.scope, r, trip, 1, closureReason, closureMissing, onboardingID(t))
					outcomes[key].result, outcomes[key].err = result, err
				})
			}
			switch order {
			case "finish-first":
				runFinish()
				waitBlocked(t, x.syncFixtureState, pid, 1)
				runClosure("closure", supervisor)
			case "closure-first":
				runClosure("closure", supervisor)
				waitBlocked(t, x.syncFixtureState, pid, 1)
				runFinish()
			default:
				runClosure("closure", supervisor)
				waitBlocked(t, x.syncFixtureState, pid, 1)
				runClosure("closure2", manager)
			}
			waitBlocked(t, x.syncFixtureState, pid, 2)
			free()
			if err := <-done; err != nil {
				t.Fatal(err)
			}
			wg.Wait()
			c := x.closureFacts(t, trip)
			switch order {
			case "finish-first":
				if f := outcomes["finish"]; f.err != nil || f.ack.Result() != protocol.Accepted || *f.ack.Snapshot().EntityRevision != 2 {
					t.Fatal("the first finish was not accepted", f.err)
				}
				if !errors.Is(outcomes["closure"].err, tripapp.ErrClosureConflict) {
					t.Fatal("the closure after the finish was not CONFLICT", outcomes["closure"].err)
				}
				if c != (tripClosureFacts{state: "COMPLETED", finishes: 1}) {
					t.Fatalf("finish-first state: %+v", c)
				}
			case "closure-first":
				if err := outcomes["closure"].err; err != nil {
					t.Fatal("the first closure was refused", err)
				}
				if f := outcomes["finish"]; f.err != nil || f.ack.Result() != protocol.AcceptedForReview || f.ack.Code() != tripapp.TripAlreadyClosed {
					t.Fatal("the finish after the closure was not held", f.err, f.ack.Result(), f.ack.Code())
				}
				if c != (tripClosureFacts{state: "COMPLETED", closures: 1, audits: 1}) {
					t.Fatalf("closure-first state: %+v", c)
				}
			default:
				if outcomes["closure"].err != nil || !errors.Is(outcomes["closure2"].err, tripapp.ErrClosureConflict) {
					t.Fatal("two closures did not leave one", outcomes["closure"].err, outcomes["closure2"].err)
				}
				if c != (tripClosureFacts{state: "COMPLETED", closures: 1, audits: 1}) {
					t.Fatalf("two-closures state: %+v", c)
				}
			}
		})
	}
}

// B-BR-EMP-401..403 database guards: forced RLS, insert-only closures, a
// trip in progress, an active separated closer, publication with the audit
// event, the guarded completion, and the closed trip's logbook and review.
func TestTripClosureSchemaIsForcedRLSInsertOnlyAndGuarded(t *testing.T) {
	x := tripClosureFixture(t)
	org := x.scope.OrgID().String()
	d, at := x.assignedDriver(t)
	finished, _ := x.completed(t, x.syncFixtureState, at)
	running := x.running(t, d, at.Add(2*time.Hour), absentReading)
	supervisor := x.reviewerFor(t, "SUPERVISOR", "ORG", "")
	refusedWith := func(name string, err error, codes []string, message string) {
		t.Helper()
		var pe *pgconn.PgError
		if !errors.As(err, &pe) || !slices.Contains(codes, pe.Code) || message != "" && pe.Message != message {
			t.Fatal("database guard did not refuse", name, err)
		}
	}
	refused := func(name string, err error, code string) {
		t.Helper()
		refusedWith(name, err, []string{code}, "")
	}
	insert := func(trip, member, employee, op string) error {
		return unitSchemaExec(t, x.legalFixtureState, `INSERT INTO org_trip_closures(org_id,trip_id,operation_id,request_hash,closer_membership_id,closer_employee_id,reason,missing_reading_reason) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`,
			org, trip, op, strings.Repeat("a", 64), member, employee, closureReason, closureMissing)
	}
	refusedWith("finished trip", insert(finished, supervisor.member, supervisor.employee, onboardingID(t)), []string{"23514"}, "trip already closed")
	refusedWith("self closure by the author", insert(running, d.source.MembershipID, d.person.employee, onboardingID(t)), []string{"23514"}, "trip self closure refused")
	refusedWith("closer employee of another person", insert(running, supervisor.member, d.person.employee, onboardingID(t)), []string{"23514"}, "trip closer unavailable")
	// A well-formed closure without its audit event fails at commit.
	refusedWith("unaudited closure", insert(running, supervisor.member, supervisor.employee, onboardingID(t)), []string{"23503", "23514"}, "")
	refused("blank reason", unitSchemaExec(t, x.legalFixtureState, `INSERT INTO org_trip_closures(org_id,trip_id,operation_id,request_hash,closer_membership_id,closer_employee_id,reason,missing_reading_reason) VALUES($1,$2,$3,$4,$5,$6,' ',$7)`,
		org, running, onboardingID(t), strings.Repeat("a", 64), supervisor.member, supervisor.employee, closureMissing), "23514")
	// The completion stays guarded: no update without a finish or a closure.
	if _, err := x.f.Admin.Exec(t.Context(), "GRANT UPDATE,DELETE ON org_trips TO "+pgx.Identifier{x.f.RuntimeRole}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	refused("bare completion", unitSchemaExec(t, x.legalFixtureState, `UPDATE org_trips SET state='COMPLETED' WHERE org_id=$1 AND id=$2`, org, running), "55000")
	if c := x.closureFacts(t, running); c != (tripClosureFacts{state: "IN_PROGRESS"}) {
		t.Fatalf("a refused direct write left state: %+v", c)
	}
	if _, _, err := x.close(x.scope, supervisor, running, 1, closureReason, closureMissing, onboardingID(t)); err != nil {
		t.Fatal(err)
	}
	// The closer never opens the closed trip's review; another reviewer's
	// opening passes the guard and fails only at commit (no decision).
	x.submitted(t, d, running, at.Add(3*time.Hour))
	open := func(r tripReviewer) error {
		return unitSchemaExec(t, x.legalFixtureState, `INSERT INTO org_trip_review_entries(org_id,trip_id,sequence,kind,revision,state,reason,actor_membership_id,actor_employee_id,operation_id) VALUES($1,$2,2,'OPEN',1,'IN_REVIEW','',$3,$4,$5)`,
			org, running, r.member, r.employee, onboardingID(t))
	}
	refusedWith("closer opening the review", open(supervisor), []string{"23514"}, "trip self approval refused")
	refusedWith("another reviewer's unpublished opening", open(x.reviewerFor(t, "APPROVER", "ORG", "")), []string{"23514"}, "trip review opening without decision")
	// Insert-only even with table privileges; forced RLS.
	if _, err := x.f.Admin.Exec(t.Context(), "GRANT UPDATE,DELETE,TRUNCATE ON org_trip_closures TO "+pgx.Identifier{x.f.RuntimeRole}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{`UPDATE org_trip_closures SET reason='x' WHERE org_id=$1`, `DELETE FROM org_trip_closures WHERE org_id=$1`} {
		refused(statement, unitSchemaExec(t, x.legalFixtureState, statement, org), "55000")
	}
	refused("truncate", unitSchemaExec(t, x.legalFixtureState, `TRUNCATE org_trip_closures`), "55000")
	refused("delete the closed trip", unitSchemaExec(t, x.legalFixtureState, `DELETE FROM org_trips WHERE org_id=$1 AND id=$2`, org, running), "55000")
	var visible int
	if err := x.f.Runtime.QueryRow(t.Context(), `SELECT count(*) FROM org_trip_closures`).Scan(&visible); err != nil || visible != 0 {
		t.Fatal("closures visible without the organization setting", visible, err)
	}
	if err := database.WithOrgTx(t.Context(), x.f.Runtime, x.scope, func(tx pgx.Tx) error {
		return tx.QueryRow(t.Context(), `SELECT count(*) FROM org_trip_closures`).Scan(&visible)
	}); err != nil || visible != 1 {
		t.Fatal("closures in scope", visible, err)
	}
	var forced bool
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT relrowsecurity AND relforcerowsecurity FROM pg_class WHERE oid='org_trip_closures'::regclass`).Scan(&forced); err != nil || !forced {
		t.Fatal("closures are not forced-RLS", err)
	}
}
