//go:build integration

package adapters_test

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"testing"
	"time"

	fleetpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/adapters"
	fa "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	meter "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain/meter"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	orgpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters"
	auditpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/adapters"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	trackingpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/tracking/adapters"
	trackingapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/tracking/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/tracking/domain"
	trippg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/jackc/pgx/v5"
)

// EMP-GPS-006 on real PostGIS (B-BR-EMP-491..494, EDR-318/319): the distance
// port composed from the real recorded-track authority, the real trip store,
// the GPS-005 summaries, the METER-006 usage port and the PRIV-005 access log.

const startReading int64 = 184_320_000

// distanceStore composes the port exactly as a route owner would.
func (x trackingState) distanceStore(t *testing.T) *trackingpg.DistanceStore {
	t.Helper()
	catalog, err := audit.NewCatalog(trackingpg.AuditDefinitions())
	if err != nil {
		t.Fatal(err)
	}
	return &trackingpg.DistanceStore{Pool: x.f.Runtime, Authority: &orgpg.TrackAuthority{Memberships: x.legalFixtureState.store.Memberships, Roles: x.legalFixtureState.store.Roles,
		Capabilities: x.flags, Structure: &orgpg.ResourceStructure{Kind: orgpg.TripStructure, Pool: x.f.Runtime}},
		Trips: &trippg.Store{}, Meters: x.meters, Access: auditpg.AccessLog{Runner: auditpg.NewStore(x.f.Runtime, catalog)}, NewID: jobs.NewUUIDv4}
}

func distanceActor(p matrixPerson) trackingapp.TrackActor {
	return trackingapp.TrackActor{AccountID: p.account, SessionID: p.session}
}

func readDistances(ctx context.Context, store *trackingpg.DistanceStore, scope kernel.OrgScope, p matrixPerson, trip string) (trackingapp.DistanceView, error) {
	return store.ReadDistances(ctx, scope, distanceActor(p), trackingapp.DistanceRequest{TripID: trip, MembershipID: p.member})
}

// finishWith finishes the fixture trip at the instant with a final reading,
// or without one when value is negative.
func (x trackingState) finishWith(t *testing.T, at time.Time, value int64) {
	t.Helper()
	d := x.syncFixtureState
	op := x.finishOp(t, d, x.trip, at, func(_, p map[string]any) {
		if value < 0 {
			p["reading"], p["missing_reading_reason"] = nil, "Painel apagado na chegada"
			return
		}
		p["reading"] = x.reading(t, strconv.FormatInt(value, 10))
	})
	ack, err := x.tripState.process(t, d, op)
	readings := 1
	if value < 0 {
		readings = 0
	}
	x.tripState.accepted(t, op, ack, err, 2, readings)
}

// trackAndProcess records two precise runs separated by a 200 s gap, ends
// the session at the trip's finish and runs the processing job; it returns
// the session and the independently computed GPS distance.
func (x trackingState) trackAndProcess(t *testing.T, finish func()) (string, int64) {
	t.Helper()
	d := x.syncFixtureState
	session := onboardingID(t)
	x.startTracking(t, d, session, 1, "PRECISE", 2)
	first := westward(1, 2001, 20, -500000000, 25340)
	second := westward(2201, 4001, 20, first[len(first)-1].lon-10*25340, 25340)
	x.trackBatch(t, d, session, 1, first)
	x.trackBatch(t, d, session, 2, second)
	finish()
	ended := x.eventOp(t, d, trackingapp.EndOperationType, session, x.at(4010), "TRIP_COMPLETED")
	ack, err := x.process(t, d, ended)
	x.accepted(t, ended, ack, err, 3, trackingCounts{transitions: 1})
	if found, err := x.dispatcher().RunOnce(t.Context()); !found || err != nil {
		t.Fatal("processing job not run", found, err)
	}
	return session, runDistance(first, second)
}

// finishReadings returns the stored start and final reading of the trip.
func (x trackingState) finishReadings(t *testing.T) (string, string) {
	t.Helper()
	var start, final string
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT t.reading_id::text,f.reading_id::text FROM org_trips t JOIN org_trip_finishes f ON(f.org_id,f.trip_id)=(t.org_id,t.id) WHERE t.org_id=$1 AND t.id=$2`,
		x.scope.OrgID().String(), x.trip).Scan(&start, &final); err != nil {
		t.Fatal(err)
	}
	return start, final
}

// correctFinal supersedes a reading through the real METER-004 decision of
// the organization's owner; safe in a goroutine (no t.Fatal).
func (x trackingState) correctFinal(reading string, value int64, operation, request string) error {
	catalog, err := audit.NewCatalog(fleetpg.MeterAuditDefinitions())
	if err != nil {
		return err
	}
	store := *x.meters
	store.Audit = auditpg.NewStore(x.f.Runtime, catalog)
	store.Parents = fleetpg.MeterParentSet{&trippg.Store{}}
	v, err := meter.NewValue(meter.Odometer, value, nil)
	if err != nil {
		return err
	}
	command, err := fa.NewMeterDecisionCommand(fa.MeterDecisionInput{OperationID: operation, AssetID: x.vehicle.AssetID, MeterID: x.vehicle.MeterID, ReadingID: reading,
		Decision: fa.MeterSupersede, Reason: "Dígitos trocados no fechamento", Value: v})
	if err != nil {
		return err
	}
	_, _, err = store.DecideMeterReading(context.Background(), x.scope, fleetActor(x.legalFixtureState), x.legalFixtureState.member, request, command)
	return err
}

func checkOdometer(t *testing.T, name string, got trackingapp.OdometerDistance, metres int64, start, end string, startAt, endAt time.Time) {
	t.Helper()
	if got.Source != (domain.DistanceSource{Kind: domain.KindOdometer, Status: domain.SourceOK, Metres: metres}) || got.Method != fa.MeterUsageMethod || got.Revision <= 0 ||
		got.Start == nil || got.Start.ReadingID != start || got.Start.Value != startReading || !got.Start.OccurredAt.Equal(startAt) ||
		got.End == nil || got.End.ReadingID != end || got.End.Value != startReading+metres || !got.End.OccurredAt.Equal(endAt) ||
		got.From == nil || !got.From.Equal(startAt) || got.To == nil || !got.To.Equal(endAt) {
		t.Fatalf("%s: odometer %+v %+v %+v", name, got.Source, got.Start, got.End)
	}
}

// BUC-EMP-250/251, B-BR-EMP-491..494 on real PostGIS: the four sources stay
// apart, missing data says why, the odometer is the basis, the divergence
// follows a reading correction, the GPS part is served only under
// gps.track.read and every served answer is recorded before it is returned.
func TestTripDistancesComposeTheMeterAndTheTrackUnderTheTrackGrant(t *testing.T) {
	x := trackingFixture(t)
	processingGrants(t, x)
	router := trackRouter(t, x)
	fx := x.deviceFixtureState
	x.inForce(t)
	store := x.distanceStore(t)
	person := func(role string) matrixPerson {
		p := matrixNewPerson(t, fx, router)
		matrixAssign(t, fx, p, role, "ORG", "ACTIVE")
		return p
	}
	owner, finance, auditor := person("OWNER"), person("FINANCE"), person("AUDITOR")
	read := func(p matrixPerson) trackingapp.DistanceView {
		t.Helper()
		v, err := readDistances(t.Context(), store, x.scope, p, x.trip)
		if err != nil {
			t.Fatal("distance read refused", err)
		}
		if v.TripID != x.trip || v.Method != domain.SourcesMethod ||
			v.Planned != (domain.DistanceSource{Kind: domain.KindPlanned, Status: domain.SourceInsufficient, Reason: domain.ReasonNoRoutePlan}) ||
			v.Estimate != (domain.DistanceSource{Kind: domain.KindEstimate, Status: domain.SourceInsufficient, Reason: domain.ReasonNoRouteCompute}) {
			t.Fatalf("answer shape %+v %+v", v.Planned, v.Estimate)
		}
		return v
	}
	accesses := func(p matrixPerson, issued, served int) {
		t.Helper()
		if i, s := x.trackAccesses(t, p.member, x.trip); i != issued || s != served {
			t.Fatal("access records", p.member, i, s, "want", issued, served)
		}
	}

	// In progress, untracked: every figure says why, nothing is zero.
	v := read(owner)
	if v.Odometer.Source != (domain.DistanceSource{Kind: domain.KindOdometer, Status: domain.SourceInsufficient, Reason: domain.ReasonTripNotFinished}) || v.Odometer.AssetID != x.vehicle.AssetID ||
		v.GPS.Source != (domain.DistanceSource{Kind: domain.KindGPS, Status: domain.SourceInsufficient, Reason: domain.ReasonNoTrack}) ||
		v.Travelled != (domain.Travelled{Status: domain.SourceInsufficient, Reason: domain.ReasonTripNotFinished}) ||
		v.Divergence.Status != domain.DivergenceNotEvaluated || v.Divergence.Reason != domain.ReasonNoOdometerDistance {
		t.Fatalf("in-progress trip %+v %+v %+v", v.Odometer.Source, v.GPS.Source, v.Travelled)
	}
	accesses(owner, 1, 1)

	session, gps := x.trackAndProcess(t, func() {
		// While recording: the session exists, nothing is processed yet.
		if v := read(owner); v.GPS.Source.Reason != domain.ReasonNotProcessed || v.GPS.SessionID == "" || v.GPS.Revision != 0 {
			t.Fatalf("recording session %+v", v.GPS.Source)
		}
		x.finishWith(t, x.at(4010), startReading+52_000+1_500)
	})
	accesses(owner, 2, 2)
	startID, finalID := x.finishReadings(t)
	// The final reading was typed before the distance was known: correct it
	// to exactly 1 500 m above the GPS distance (no signal) first.
	if err := x.correctFinal(finalID, startReading+gps+1_500, onboardingID(t), onboardingID(t)); err != nil {
		t.Fatal("first correction", err)
	}
	var corrected string
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT id::text FROM org_meter_readings WHERE org_id=$1 AND value=$2`, x.scope.OrgID().String(), startReading+gps+1_500).Scan(&corrected); err != nil {
		t.Fatal(err)
	}

	v = read(owner)
	checkOdometer(t, "finished", v.Odometer, gps+1_500, startID, corrected, x.tripAt, x.at(4010))
	if v.GPS.Source != (domain.DistanceSource{Kind: domain.KindGPS, Status: domain.SourceOK, Metres: gps, Quality: domain.QualityPrecise}) || v.GPS.SessionID != session ||
		v.GPS.Revision != 1 || v.GPS.Method != domain.MethodVersion || v.GPS.DistanceMethod != domain.DistanceMethod || v.GPS.CoveragePermille == nil || *v.GPS.CoveragePermille < 900 ||
		v.GPS.Gaps != 2 || v.GPS.ExpectedSeconds != 4010 || v.GPS.GapSeconds < 200 || v.GPS.ProcessedAt == nil {
		t.Fatalf("GPS source %+v cov=%v gaps=%d exp=%d gap=%d", v.GPS.Source, v.GPS.CoveragePermille, v.GPS.Gaps, v.GPS.ExpectedSeconds, v.GPS.GapSeconds)
	}
	if v.Travelled != (domain.Travelled{Status: domain.SourceOK, Basis: domain.BasisOdometer, Metres: gps + 1_500}) ||
		v.Divergence != (domain.Divergence{Status: domain.DivergenceNotRaised, Odometer: gps + 1_500, GPS: gps, Difference: 1_500, Thresholds: domain.DefaultThresholds()}) {
		t.Fatalf("basis and divergence %+v %+v", v.Travelled, v.Divergence)
	}
	accesses(owner, 3, 3)

	t.Run("trip-read-holder-without-track-grant-sees-no-gps", func(t *testing.T) {
		v := read(finance)
		if v.GPS.Source != (domain.DistanceSource{Kind: domain.KindGPS, Status: domain.SourceWithheld}) || v.GPS.SessionID != "" || v.GPS.Revision != 0 ||
			v.GPS.CoveragePermille != nil || v.GPS.Gaps != 0 || v.GPS.ProcessedAt != nil || v.GPS.Method != "" {
			t.Fatalf("withheld GPS shows evidence %+v", v.GPS.Source)
		}
		checkOdometer(t, "withheld", v.Odometer, gps+1_500, startID, corrected, x.tripAt, x.at(4010))
		if v.Travelled.Basis != domain.BasisOdometer || v.Divergence != (domain.Divergence{Status: domain.DivergenceNotEvaluated, Reason: domain.ReasonGPSWithheld, Thresholds: domain.DefaultThresholds()}) {
			t.Fatalf("withheld divergence %+v", v.Divergence)
		}
		accesses(finance, 0, 0)
	})
	t.Run("auditor-recorded-track-grant-is-recorded", func(t *testing.T) {
		if v := read(auditor); v.GPS.Source.Status != domain.SourceOK || v.GPS.Source.Metres != gps {
			t.Fatalf("auditor GPS %+v", v.GPS.Source)
		}
		accesses(auditor, 1, 1)
	})
	t.Run("a-reading-correction-raises-the-signal", func(t *testing.T) {
		before := read(owner)
		// 8 000 m above the GPS distance: more than 2 000 m and more than 10 %.
		if err := x.correctFinal(corrected, startReading+gps+8_000, onboardingID(t), onboardingID(t)); err != nil {
			t.Fatal("second correction", err)
		}
		var latest string
		if err := x.f.Admin.QueryRow(t.Context(), `SELECT id::text FROM org_meter_readings WHERE org_id=$1 AND value=$2`, x.scope.OrgID().String(), startReading+gps+8_000).Scan(&latest); err != nil {
			t.Fatal(err)
		}
		v := read(owner)
		checkOdometer(t, "corrected", v.Odometer, gps+8_000, startID, latest, x.tripAt, x.at(4010))
		if v.Odometer.Revision <= before.Odometer.Revision {
			t.Fatal("the projection revision did not move", before.Odometer.Revision, v.Odometer.Revision)
		}
		if v.Divergence != (domain.Divergence{Status: domain.DivergenceRaised, Odometer: gps + 8_000, GPS: gps, Difference: 8_000, Thresholds: domain.DefaultThresholds()}) ||
			v.Travelled.Metres != gps+8_000 || v.Travelled.Basis != domain.BasisOdometer || v.GPS.Source.Metres != gps {
			t.Fatalf("raised signal %+v %+v", v.Divergence, v.Travelled)
		}
	})
	t.Run("a-failed-access-record-serves-nothing", func(t *testing.T) {
		role := pgx.Identifier{x.f.RuntimeRole}.Sanitize()
		if _, err := x.f.Admin.Exec(t.Context(), "REVOKE INSERT ON org_sensitive_access_events FROM "+role); err != nil {
			t.Fatal(err)
		}
		issued, served := x.trackAccesses(t, owner.member, x.trip)
		if v, err := readDistances(t.Context(), store, x.scope, owner, x.trip); !errors.Is(err, trackingapp.ErrAccessUnrecorded) || v.TripID != "" || v.GPS.Source.Metres != 0 || v.Odometer.Source.Metres != 0 {
			t.Fatal("an answer was served without its access record", err, v.GPS.Source)
		}
		accesses(owner, issued, served)
		// Nothing location-derived is served to a reader without the grant.
		if v, err := readDistances(t.Context(), store, x.scope, finance, x.trip); err != nil || v.GPS.Source.Status != domain.SourceWithheld {
			t.Fatal("a withheld answer needs no access record", err)
		}
		if _, err := x.f.Admin.Exec(t.Context(), "GRANT INSERT ON org_sensitive_access_events TO "+role); err != nil {
			t.Fatal(err)
		}
		read(owner)
		accesses(owner, issued+1, served+1)
	})
	t.Run("foreign-unknown-and-unproven-reads-not-found", func(t *testing.T) {
		other := legalFixtureFor(t, x.f, x.onboarding, x.legalFixtureState.actor, x.flags)
		if _, _, err := other.onboarding.Repository.ActivateOrganization(t.Context(), other.scope, other.actor, other.member, onboardingID(t), 1); err != nil {
			t.Fatal(err)
		}
		stranger := matrixPerson{account: other.actor.AccountID, session: other.actor.SessionID, member: other.member}
		if _, err := readDistances(t.Context(), store, other.scope, stranger, x.trip); !errors.Is(err, trackingapp.ErrNotFound) {
			t.Fatal("another organization's trip was not NOT_FOUND", err)
		}
		if _, err := readDistances(t.Context(), store, x.scope, owner, onboardingID(t)); !errors.Is(err, trackingapp.ErrNotFound) {
			t.Fatal("an unknown trip was not NOT_FOUND", err)
		}
		if _, err := readDistances(t.Context(), store, x.scope, matrixPerson{account: owner.account, session: onboardingID(t), member: owner.member}, x.trip); !errors.Is(err, trackingapp.ErrTrackUnauthorized) {
			t.Fatal("an unproven session read", err)
		}
		if _, err := readDistances(t.Context(), store, x.scope, matrixPerson{account: owner.account, session: owner.session, member: finance.member}, x.trip); !errors.Is(err, trackingapp.ErrNotFound) {
			t.Fatal("another member's membership was accepted", err)
		}
		var foreign int
		if err := x.f.Admin.QueryRow(t.Context(), `SELECT count(*) FROM org_sensitive_access_events WHERE org_id=$1`, other.scope.OrgID().String()).Scan(&foreign); err != nil || foreign != 0 {
			t.Fatal("a refused read was recorded", foreign, err)
		}
	})
}

// Named negative "absent basis reported as zero" on real PostGIS: a trip
// finished without its final reading has a known METER-006 usage of 0 m
// between its instants (the start reading is the latest reading at both),
// and the distance port reports NO_FINAL_READING instead; with a precise
// track the basis is the GPS distance, labelled GPS.
func TestTripDistanceWithoutAFinalReadingIsNeverZero(t *testing.T) {
	x := trackingFixture(t)
	processingGrants(t, x)
	router := trackRouter(t, x)
	fx := x.deviceFixtureState
	x.inForce(t)
	store := x.distanceStore(t)
	owner := matrixNewPerson(t, fx, router)
	matrixAssign(t, fx, owner, "OWNER", "ORG", "ACTIVE")
	session, gps := x.trackAndProcess(t, func() { x.finishWith(t, x.at(4010), -1) })
	trap, err := x.meters.UsageBetween(t.Context(), x.scope, x.vehicle.AssetID, meter.Odometer, x.tripAt, x.at(4010))
	if err != nil || !trap.Known() || trap.Usage != 0 || trap.Start == nil || trap.End == nil || trap.Start.ReadingID != trap.End.ReadingID {
		t.Fatal("the meter port no longer answers the absent final reading with the start reading", trap.Usage, trap.Reason, err)
	}
	v, err := readDistances(t.Context(), store, x.scope, owner, x.trip)
	if err != nil {
		t.Fatal(err)
	}
	if v.Odometer.Source != (domain.DistanceSource{Kind: domain.KindOdometer, Status: domain.SourceInsufficient, Reason: domain.ReasonNoFinalReading}) ||
		v.Odometer.Start == nil || v.Odometer.Start.ReadingID != trap.Start.ReadingID || v.Odometer.End != nil {
		t.Fatalf("absent final reading %+v %+v %+v", v.Odometer.Source, v.Odometer.Start, v.Odometer.End)
	}
	if v.GPS.Source != (domain.DistanceSource{Kind: domain.KindGPS, Status: domain.SourceOK, Metres: gps, Quality: domain.QualityPrecise}) || v.GPS.SessionID != session {
		t.Fatalf("GPS source %+v", v.GPS.Source)
	}
	if v.Travelled != (domain.Travelled{Status: domain.SourceOK, Basis: domain.BasisGPS, Metres: gps}) ||
		v.Divergence != (domain.Divergence{Status: domain.DivergenceNotEvaluated, Reason: domain.ReasonNoOdometerDistance, Thresholds: domain.DefaultThresholds()}) {
		t.Fatalf("por GPS basis %+v %+v", v.Travelled, v.Divergence)
	}
	// Without the track grant nothing remains: INSUFFICIENT_DATA, never 0.
	finance := matrixNewPerson(t, fx, router)
	matrixAssign(t, fx, finance, "FINANCE", "ORG", "ACTIVE")
	w, err := readDistances(t.Context(), store, x.scope, finance, x.trip)
	if err != nil || w.Travelled != (domain.Travelled{Status: domain.SourceInsufficient, Reason: domain.ReasonNoFinalReading}) {
		t.Fatalf("withheld without odometer %+v %v", w.Travelled, err)
	}
	if m, b, ok := w.Travelled.Value(); ok || m != 0 || b != "" {
		t.Fatal("an absent basis reads as a value", m, b)
	}
}

// B-BR-EMP-490 through the composed port: a distance read and a correction
// of the trip's final reading queued behind a held meter lock serialise in
// both orders; the answer names the readings and revision it used.
func TestTripDistanceAndReadingCorrectionSerialiseInBothOrders(t *testing.T) {
	for _, order := range []string{"correction-first", "read-first"} {
		t.Run(order, func(t *testing.T) {
			x := trackingFixture(t)
			processingGrants(t, x)
			router := trackRouter(t, x)
			fx := x.deviceFixtureState
			store := x.distanceStore(t)
			owner := matrixNewPerson(t, fx, router)
			matrixAssign(t, fx, owner, "OWNER", "ORG", "ACTIVE")
			x.finishWith(t, x.at(3600), startReading+290_000)
			startID, finalID := x.finishReadings(t)
			before, err := readDistances(t.Context(), store, x.scope, owner, x.trip)
			if err != nil {
				t.Fatal(err)
			}
			checkOdometer(t, "before", before.Odometer, 290_000, startID, finalID, x.tripAt, x.at(3600))
			operation, request := onboardingID(t), onboardingID(t)
			holder := make(chan int, 1)
			release := make(chan struct{})
			var once sync.Once
			free := func() { once.Do(func() { close(release) }) }
			defer free()
			done := make(chan error, 1)
			go func() {
				done <- database.WithOrgTx(context.Background(), x.f.Runtime, x.scope, func(tx pgx.Tx) error {
					if _, err := tx.Exec(context.Background(), `SELECT id FROM org_meters WHERE org_id=$1 AND id=$2 FOR UPDATE`, x.scope.OrgID().String(), x.vehicle.MeterID); err != nil {
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
			var answer trackingapp.DistanceView
			var readErr, correctErr error
			var wg sync.WaitGroup
			ask := func() { answer, readErr = readDistances(context.Background(), store, x.scope, owner, x.trip) }
			correct := func() { correctErr = x.correctFinal(finalID, startReading+280_000, operation, request) }
			first, second := correct, ask
			if order == "read-first" {
				first, second = ask, correct
			}
			wg.Go(first)
			waitBlocked(t, x.syncFixtureState, pid, 1)
			wg.Go(second)
			waitBlocked(t, x.syncFixtureState, pid, 2)
			free()
			if err := <-done; err != nil {
				t.Fatal(err)
			}
			wg.Wait()
			if readErr != nil || correctErr != nil {
				t.Fatal(readErr, correctErr)
			}
			if order == "read-first" {
				checkOdometer(t, "earlier answer", answer.Odometer, 290_000, startID, finalID, x.tripAt, x.at(3600))
				if answer.Odometer.Revision != before.Odometer.Revision {
					t.Fatal("the earlier answer saw part of the later correction", before.Odometer.Revision, answer.Odometer.Revision)
				}
				return
			}
			var latest string
			if err := x.f.Admin.QueryRow(t.Context(), `SELECT id::text FROM org_meter_readings WHERE org_id=$1 AND value=$2`, x.scope.OrgID().String(), startReading+280_000).Scan(&latest); err != nil {
				t.Fatal(err)
			}
			checkOdometer(t, "later answer", answer.Odometer, 280_000, startID, latest, x.tripAt, x.at(3600))
			if answer.Odometer.Revision <= before.Odometer.Revision {
				t.Fatal("the later answer missed part of the earlier correction", before.Odometer.Revision, answer.Odometer.Revision)
			}
		})
	}
}
