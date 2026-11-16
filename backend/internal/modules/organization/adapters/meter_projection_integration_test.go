//go:build integration

package adapters_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	fleetpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/adapters"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	meter "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain/meter"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
)

func grantProjectionRebuild(t *testing.T, fx syncFixtureState) {
	t.Helper()
	if _, err := fx.f.Admin.Exec(t.Context(), "GRANT EXECUTE ON FUNCTION org_meter_current_rebuild(uuid) TO "+pgx.Identifier{fx.f.RuntimeRole}.Sanitize()); err != nil {
		t.Fatal(err)
	}
}

// projection is the organization's stored projection as canonical JSON.
func projection(t *testing.T, fx syncFixtureState) string {
	t.Helper()
	var rows string
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT COALESCE(jsonb_agg(to_jsonb(x) ORDER BY meter_id),'[]')::text FROM org_meter_current_values x WHERE org_id=$1`, fx.scope.OrgID().String()).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	return rows
}

func usage(t *testing.T, fx syncFixtureState, store *fleetpg.MeterStore, asset string, from, to time.Time) (app.MeterUsage, error) {
	t.Helper()
	return store.UsageBetween(t.Context(), fx.scope, asset, meter.Odometer, from, to)
}

// AC1 and "Done when": the projection maintained in the transaction of
// every fact (intake, a correction, a rollover, a replacement) equals the one
// rebuilt from the readings, values and revisions, and a second rebuild
// changes nothing; the usage across every boundary names its anchors.
func TestMeterProjectionRebuildEqualsIncremental(t *testing.T) {
	base := time.Now().UTC().Truncate(time.Second).Add(-10 * 24 * time.Hour)
	fx, store, installed := segmentFixture(t, 1_000_000, base, int64p(2_000_000))
	grantProjectionRebuild(t, fx)
	r1, _ := validationRecord(t, fx, store, installed, base.Add(24*time.Hour), 1_200_000, readingStateRow{state: "VALID"})
	r2, _ := validationRecord(t, fx, store, installed, base.Add(30*time.Hour), 1_150_000, readingStateRow{state: "SUSPECT", flags: "READING_BELOW_PREVIOUS"})
	fix := withOperation(decision(installed, r2, 1, app.MeterSupersede, "Dígitos trocados"), onboardingID(t))
	fix.Value = odometerValue(t, 1_250_000)
	if _, _, err := decide(t, fx, store, fix); err != nil {
		t.Fatal("correction refused", err)
	}
	validationRecord(t, fx, store, installed, base.Add(48*time.Hour), 1_900_000, readingStateRow{state: "VALID"})
	r4, _ := validationRecord(t, fx, store, installed, base.Add(72*time.Hour), 100_000, readingStateRow{state: "SUSPECT", flags: "POSSIBLE_ROLLOVER"})
	if _, _, err := decide(t, fx, store, withOperation(decision(installed, r4, 1, app.MeterConfirmRollover, "Painel de sete dígitos"), onboardingID(t))); err != nil {
		t.Fatal("rollover refused", err)
	}
	validationRecord(t, fx, store, app.MeterInstallResult{AssetID: installed.AssetID, MeterID: installed.MeterID, SegmentID: openSegment(t, fx, installed.MeterID)}, base.Add(96*time.Hour), 300_000, readingStateRow{state: "VALID"})
	replaced, _, err := change(t, fx, store, replaceInput(t, installed, 2, base.Add(120*time.Hour), int64p(350_000), 0, nil))
	if err != nil {
		t.Fatal("replacement refused", err)
	}
	next := app.MeterInstallResult{AssetID: installed.AssetID, MeterID: replaced.NewMeterID, SegmentID: replaced.OpenedSegmentID}
	r6, _ := validationRecord(t, fx, store, next, base.Add(144*time.Hour), 50_000, readingStateRow{state: "VALID"})
	incremental := projection(t, fx)
	var coherent bool
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT count(*)=2 AND bool_and(revision=org_meter_current_revision(org_id,meter_id))
 AND bool_or(meter_id=$2 AND segment_id IS NULL AND reading_id IS NULL) AND bool_or(meter_id=$3 AND reading_id=$4 AND value=50000)
 FROM org_meter_current_values WHERE org_id=$1`, fx.scope.OrgID().String(), installed.MeterID, replaced.NewMeterID, r6).Scan(&coherent); err != nil || !coherent {
		t.Fatal("incremental projection does not count every fact or name the current reading", incremental, err)
	}
	for i := range 2 {
		n, err := fleetpg.RebuildMeterProjection(t.Context(), fx.f.Runtime, fx.scope)
		if err != nil || n != 2 {
			t.Fatal("rebuild", i, n, err)
		}
		if rebuilt := projection(t, fx); rebuilt != incremental {
			t.Fatalf("AC1: rebuild %d differs from the incremental projection\n%s\n%s", i, incremental, rebuilt)
		}
	}
	answer, err := usage(t, fx, store, installed.AssetID, base.Add(24*time.Hour), base.Add(144*time.Hour))
	if err != nil || !answer.Known() || answer.Usage != 1_200_000 || answer.Start == nil || answer.Start.ReadingID != r1 || answer.Start.Value != 1_200_000 ||
		answer.End == nil || answer.End.ReadingID != r6 || answer.Method != app.MeterUsageMethod || answer.Revision < 2 {
		t.Fatalf("AC2 usage across the rollover and the replacement %+v %v", answer, err)
	}
	var revisions int64
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT sum(revision) FROM org_meter_current_values WHERE org_id=$1`, fx.scope.OrgID().String()).Scan(&revisions); err != nil || answer.Revision != revisions {
		t.Fatal("the answer does not name the projection revision", answer.Revision, revisions, err)
	}
}

func openSegment(t *testing.T, fx syncFixtureState, meterID string) string {
	t.Helper()
	var id string
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT id::text FROM org_meter_segments WHERE org_id=$1 AND meter_id=$2 AND closed_at IS NULL`, fx.scope.OrgID().String(), meterID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

// AC2..AC5 and the negatives: anchors and no interpolation, no reading
// before an instant, a gap, reversed instants, another kind, another
// organization, and a correction raising the revision.
func TestMeterUsageAnchorsGapsAndRefusals(t *testing.T) {
	base := time.Now().UTC().Truncate(time.Second).Add(-10 * 24 * time.Hour)
	fx, store, installed := segmentFixture(t, 1_000_000, base, nil)
	r1, _ := validationRecord(t, fx, store, installed, base.Add(24*time.Hour), 1_200_000, readingStateRow{state: "VALID"})
	if a, err := usage(t, fx, store, installed.AssetID, base.Add(12*time.Hour), base.Add(24*time.Hour)); err != nil || !a.Known() || a.Usage != 200_000 ||
		a.Start.ReadingID != installed.OpeningReadingID || a.End.ReadingID != r1 {
		t.Fatalf("AC4: anchors, never an interpolated value %+v %v", a, err)
	}
	if a, err := usage(t, fx, store, installed.AssetID, base.Add(-time.Hour), base.Add(24*time.Hour)); err != nil || a.Known() || a.Reason != meter.NoReadingBefore || a.Start != nil || a.End == nil || a.End.ReadingID != r1 || a.Usage != 0 {
		t.Fatalf("AC3: no anchor before the start %+v %v", a, err)
	}
	if _, err := usage(t, fx, store, installed.AssetID, base.Add(24*time.Hour), base.Add(12*time.Hour)); !errors.Is(err, meter.ErrInvalid) {
		t.Fatal("reversed instants", err)
	}
	if _, err := store.UsageBetween(t.Context(), fx.scope, installed.AssetID, meter.HourMeter, base, base.Add(24*time.Hour)); !errors.Is(err, app.ErrNotFound) {
		t.Fatal("an asset without a meter of the kind", err)
	}
	other, _, err := fx.onboarding.Create(t.Context(), fx.actor, onboardingCommand(t, onboardingID(t)))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UsageBetween(t.Context(), scopeFor(t, other.OrgID.String()), installed.AssetID, meter.Odometer, base, base.Add(24*time.Hour)); !errors.Is(err, app.ErrNotFound) {
		t.Fatal("usage across organizations", err)
	}
	low, _ := validationRecord(t, fx, store, installed, base.Add(30*time.Hour), 1_100_000, readingStateRow{state: "SUSPECT", flags: "READING_BELOW_PREVIOUS"})
	before, err := usage(t, fx, store, installed.AssetID, base, base.Add(30*time.Hour))
	if err != nil || before.End.ReadingID != r1 {
		t.Fatal("suspect reading used as an anchor", before, err)
	}
	if _, _, err := decide(t, fx, store, withOperation(decision(installed, low, 1, app.MeterVoid, "Leitura do ativo errado"), onboardingID(t))); err != nil {
		t.Fatal(err)
	}
	after, err := usage(t, fx, store, installed.AssetID, base, base.Add(30*time.Hour))
	if err != nil || after.Revision != before.Revision+1 || after.Usage != before.Usage {
		t.Fatalf("AC5: a decision did not raise the revision exactly once %+v %+v %v", before, after, err)
	}
	replaced, _, err := change(t, fx, store, replaceInput(t, installed, 1, base.Add(36*time.Hour), nil, 0, nil))
	if err != nil {
		t.Fatal(err)
	}
	r3, _ := validationRecord(t, fx, store, app.MeterInstallResult{AssetID: installed.AssetID, MeterID: replaced.NewMeterID, SegmentID: replaced.OpenedSegmentID}, base.Add(48*time.Hour), 10_000, readingStateRow{state: "VALID"})
	if a, err := usage(t, fx, store, installed.AssetID, base.Add(24*time.Hour), base.Add(48*time.Hour)); err != nil || a.Reason != meter.MeterGap || a.Start.ReadingID != r1 || a.End.ReadingID != r3 {
		t.Fatalf("usage across an unknown final value %+v %v", a, err)
	}
}

// A usage answer and a concurrent correction serialise on the meter lock in
// both arrival orders: the answer is wholly before or wholly after it.
func TestMeterUsageAndCorrectionSerialiseInBothOrders(t *testing.T) {
	for _, order := range []string{"correction-first", "usage-first"} {
		t.Run(order, func(t *testing.T) {
			base := time.Now().UTC().Truncate(time.Second).Add(-10 * 24 * time.Hour)
			fx, store, installed := segmentFixture(t, 1_000_000, base, nil)
			r1, _ := validationRecord(t, fx, store, installed, base.Add(24*time.Hour), 1_200_000, readingStateRow{state: "VALID"})
			fix := withOperation(decision(installed, r1, 1, app.MeterSupersede, "Dígitos trocados"), onboardingID(t))
			fix.Value = odometerValue(t, 1_100_000)
			before, err := usage(t, fx, store, installed.AssetID, base, base.Add(24*time.Hour))
			if err != nil {
				t.Fatal(err)
			}
			holder := make(chan int, 1)
			release := make(chan struct{})
			var once sync.Once
			free := func() { once.Do(func() { close(release) }) }
			defer free()
			done := make(chan error, 1)
			go func() {
				done <- database.WithOrgTx(context.Background(), fx.f.Runtime, fx.scope, func(tx pgx.Tx) error {
					if _, err := tx.Exec(context.Background(), `SELECT id FROM org_meters WHERE org_id=$1 AND id=$2 FOR UPDATE`, fx.scope.OrgID().String(), installed.MeterID); err != nil {
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
			var answer app.MeterUsage
			var usageErr, decideErr error
			var wg sync.WaitGroup
			ask := func() {
				answer, usageErr = store.UsageBetween(context.Background(), fx.scope, installed.AssetID, meter.Odometer, base, base.Add(24*time.Hour))
			}
			correct := func() { _, _, decideErr = decide(t, fx, store, fix) }
			first, second := correct, ask
			if order == "usage-first" {
				first, second = ask, correct
			}
			wg.Go(first)
			waitBlocked(t, fx, pid, 1)
			wg.Go(second)
			waitBlocked(t, fx, pid, 2)
			free()
			if err := <-done; err != nil {
				t.Fatal(err)
			}
			wg.Wait()
			if usageErr != nil || decideErr != nil {
				t.Fatal(usageErr, decideErr)
			}
			if order == "usage-first" {
				if answer.Usage != before.Usage || answer.Revision != before.Revision || answer.End.ReadingID != r1 {
					t.Fatalf("the earlier answer saw part of the later correction %+v %+v", before, answer)
				}
				return
			}
			if answer.Usage != 100_000 || answer.Revision <= before.Revision || answer.End.ReadingID == r1 {
				t.Fatalf("the later answer missed part of the earlier correction %+v %+v", before, answer)
			}
		})
	}
}

// The projection is written only through the database functions: the
// runtime role reads it under forced RLS and cannot write it, a refresh or
// rebuild for another organization is refused.
func TestMeterProjectionSchemaGuards(t *testing.T) {
	base := time.Now().UTC().Truncate(time.Second).Add(-10 * 24 * time.Hour)
	fx, _, installed := segmentFixture(t, 1_000_000, base, nil)
	grantProjectionRebuild(t, fx)
	org := fx.scope.OrgID().String()
	for name, sql := range map[string]string{
		"insert":  `INSERT INTO org_meter_current_values(org_id,meter_id,revision) VALUES($1,$2,1)`,
		"update":  `UPDATE org_meter_current_values SET revision=revision+1 WHERE org_id=$1 AND meter_id=$2`,
		"delete":  `DELETE FROM org_meter_current_values WHERE org_id=$1 AND meter_id=$2`,
		"refresh": `SELECT org_meter_current_refresh($1,$2)`,
	} {
		t.Run(name, func(t *testing.T) {
			fleetSQLCode(t, fleetSQL(t, fx.legalFixtureState, sql, org, installed.MeterID), "42501")
		})
	}
	var n int
	if err := fx.f.Runtime.QueryRow(t.Context(), `SELECT count(*) FROM org_meter_current_values`).Scan(&n); err != nil || n != 0 {
		t.Fatal("projection read without organization escaped forced RLS", err)
	}
	other, _, err := fx.onboarding.Create(t.Context(), fx.actor, onboardingCommand(t, onboardingID(t)))
	if err != nil {
		t.Fatal(err)
	}
	foreign := scopeFor(t, other.OrgID.String())
	err = database.WithOrgTx(t.Context(), fx.f.Runtime, foreign, func(tx pgx.Tx) error {
		if err := tx.QueryRow(t.Context(), `SELECT count(*) FROM org_meter_current_values`).Scan(&n); err != nil || n != 0 {
			t.Fatal("foreign organization sees the projection", err)
		}
		_, err := tx.Exec(t.Context(), `SELECT org_meter_current_rebuild($1)`, org)
		return err
	})
	fleetSQLCode(t, err, "42501")
}
