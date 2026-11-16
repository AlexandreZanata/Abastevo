//go:build integration

package adapters_test

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	fleetpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/adapters"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	meter "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain/meter"
	orgpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters"
	auditpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/adapters"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/jackc/pgx/v5"
)

// segmentFixture is correctionFixture with an optional modulus on the
// installed odometer: validation on, the decision store composed.
func segmentFixture(t *testing.T, opening int64, base time.Time, modulus *int64) (syncFixtureState, *fleetpg.MeterStore, app.MeterInstallResult) {
	t.Helper()
	fx := syncFixture(t)
	assets := fleetStore(t, fx.legalFixtureState)
	asset, _, err := fleetWrite(t, fx.legalFixtureState, assets, fleetCommand(t, fleetCreateInput()))
	if err != nil {
		t.Fatal(err)
	}
	store := meterStore(t, fx.legalFixtureState)
	store.Authority = &orgpg.MeterAuthority{Memberships: fx.legalFixtureState.store.Memberships, Roles: fx.legalFixtureState.store.Roles, Capabilities: fx.flags, Devices: fx.devices}
	input := meterInstallInput(t, asset.AssetID, base)
	input.Modulus = modulus
	if input.Value, err = meter.NewValue(meter.Odometer, opening, modulus); err != nil {
		t.Fatal(err)
	}
	installed, _, err := meterInstall(t, fx.legalFixtureState, store, meterInstallCommand(t, input))
	if err != nil {
		t.Fatal(err)
	}
	policy := meter.DefaultPlausibility()
	store.Plausibility = &policy
	catalog, err := audit.NewCatalog(fleetpg.MeterAuditDefinitions())
	if err != nil {
		t.Fatal(err)
	}
	store.Audit = auditpg.NewStore(fx.f.Runtime, catalog)
	store.Parents = correctionParents{}
	return fx, store, installed
}

func odometerValue(t *testing.T, v int64) meter.Value {
	t.Helper()
	value, err := meter.NewValue(meter.Odometer, v, nil)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func boundaryDeclaration(at time.Time) orgtime.Declaration {
	return orgtime.Declaration{OccurredAt: at, EntryZone: "America/Cuiaba", DeviceOffsetSeconds: -14400}
}

func replaceInput(t *testing.T, installed app.MeterInstallResult, segments int64, at time.Time, final *int64, opening int64, modulus *int64) app.MeterChangeInput {
	t.Helper()
	in := app.MeterChangeInput{OperationID: onboardingID(t), AssetID: installed.AssetID, MeterID: installed.MeterID, ExpectedSegments: segments, Change: app.MeterReplace,
		Declared: boundaryDeclaration(at), Label: "Synthetic replacement odometer", Modulus: modulus, Opening: odometerValue(t, opening)}
	if final != nil {
		v := odometerValue(t, *final)
		in.Final = &v
	}
	return in
}

func resetInput(t *testing.T, installed app.MeterInstallResult, segments int64, at time.Time, before, after int64) app.MeterChangeInput {
	t.Helper()
	return app.MeterChangeInput{OperationID: onboardingID(t), AssetID: installed.AssetID, MeterID: installed.MeterID, ExpectedSegments: segments, Change: app.MeterReset,
		Declared: boundaryDeclaration(at), Before: odometerValue(t, before), After: odometerValue(t, after)}
}

// change never calls t.Fatal, so it is safe in a goroutine.
func change(_ *testing.T, fx syncFixtureState, store *fleetpg.MeterStore, in app.MeterChangeInput) (app.MeterBoundaryResult, bool, error) {
	command, err := app.NewMeterChangeCommand(in)
	if err != nil {
		return app.MeterBoundaryResult{}, false, err
	}
	var ids app.MeterChangeIDs
	for _, p := range []*string{&ids.Segment, &ids.Meter, &ids.OpeningReading, &ids.RemovalReading} {
		if *p, err = jobs.NewUUIDv4(); err != nil {
			return app.MeterBoundaryResult{}, false, err
		}
	}
	return store.ChangeMeter(context.Background(), fx.scope, fleetActor(fx.legalFixtureState), fx.member, ids, command)
}

func int64p(v int64) *int64 { return &v }

func segmentCount(t *testing.T, fx syncFixtureState, meterID string) int64 {
	t.Helper()
	var n int64
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT count(*) FROM org_meter_segments WHERE org_id=$1 AND meter_id=$2`, fx.scope.OrgID().String(), meterID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// storedTrack builds the pure track of one asset and kind from the stored
// chain and the readings currently VALID, counted in the segment
// org_meter_reading_segment derives (B-BR-EMP-481/482).
func storedTrack(t *testing.T, fx syncFixtureState, asset string) meter.Track {
	t.Helper()
	org := fx.scope.OrgID().String()
	rows, err := fx.f.Admin.Query(t.Context(), `SELECT s.id::text,s.meter_id::text,s.kind,s.opened_at+make_interval(secs=>s.opened_submicro/1e9::float8),
 CASE s.kind WHEN 'ROLLOVER' THEN 0 ELSE(SELECT r.value FROM org_meter_readings r WHERE(r.org_id,r.meter_id,r.id)=(s.org_id,s.meter_id,s.opening_reading_id)) END,
 s.closed_at,s.closing_value
FROM org_meter_segments s JOIN org_meters m ON(m.org_id,m.id)=(s.org_id,s.meter_id) WHERE m.org_id=$1 AND m.asset_id=$2 AND m.kind='ODOMETER'
ORDER BY m.installed_at,m.installed_submicro,m.id,s.opened_at,s.opened_submicro`, org, asset)
	if err != nil {
		t.Fatal(err)
	}
	var spans []meter.Span
	for rows.Next() {
		var s meter.Span
		var reason string
		var closed *time.Time
		if err := rows.Scan(&s.ID, &s.Meter, &reason, &s.OpenedAt, &s.Open, &closed, &s.Close); err != nil {
			t.Fatal(err)
		}
		s.Reason, s.OpenedAt = meter.SegmentReason(reason), s.OpenedAt.UTC()
		if closed != nil {
			s.Closed, s.ClosedAt = true, closed.UTC()
		}
		spans = append(spans, s)
	}
	if rows.Err() != nil {
		t.Fatal(rows.Err())
	}
	rows, err = fx.f.Admin.Query(t.Context(), `SELECT r.id::text,org_meter_reading_segment(r.org_id,r.meter_id,r.id)::text,r.value,r.occurred_at,r.received_at
FROM org_meter_readings r JOIN org_meters m ON(m.org_id,m.id)=(r.org_id,r.meter_id) WHERE m.org_id=$1 AND m.asset_id=$2 AND org_meter_reading_state(r.org_id,r.meter_id,r.id)='VALID'`, org, asset)
	if err != nil {
		t.Fatal(err)
	}
	var anchors []meter.Anchor
	for rows.Next() {
		var a meter.Anchor
		if err := rows.Scan(&a.ID, &a.Segment, &a.Value, &a.OccurredAt, &a.ReceivedAt); err != nil {
			t.Fatal(err)
		}
		a.OccurredAt, a.ReceivedAt = a.OccurredAt.UTC(), a.ReceivedAt.UTC()
		anchors = append(anchors, a)
	}
	if rows.Err() != nil {
		t.Fatal(rows.Err())
	}
	track, err := meter.NewTrack(meter.Odometer, spans, anchors)
	if err != nil {
		t.Fatal("stored chain is not a valid track", err)
	}
	return track
}

// AC1 and example C through the command: the replacement records the final
// value as the METER_REMOVAL closing reading, removes the old meter, installs
// the new one at 0 and stores the implied usage; a reading of 412 km on the
// new meter gives 1 944 000 m cumulative usage and 754 000 m since R4′.
func TestMeterSegmentExampleCReplacementThroughTheStore(t *testing.T) {
	base := time.Now().UTC().Truncate(time.Second).Add(-10 * 24 * time.Hour)
	fx, store, installed := segmentFixture(t, 183_000_000, base, nil)
	validationRecord(t, fx, store, installed, base.Add(47*time.Hour), 183_400_000, readingStateRow{state: "VALID"})
	validationRecord(t, fx, store, installed, base.Add(95*time.Hour), 183_932_000, readingStateRow{state: "VALID"})
	r4, _ := validationRecord(t, fx, store, installed, base.Add(120*time.Hour), 184_190_000, readingStateRow{state: "VALID"})
	at := base.Add(169 * time.Hour)
	in := replaceInput(t, installed, 1, at, int64p(184_532_000), 0, nil)
	result, replay, err := change(t, fx, store, in)
	if err != nil || replay {
		t.Fatal("replacement refused", err)
	}
	if result.Change != app.MeterReplace || result.MeterID != installed.MeterID || result.ClosedSegmentID != installed.SegmentID || !result.At.Equal(at) || result.Close == nil || *result.Close != 184_532_000 ||
		result.Implied == nil || *result.Implied != 342_000 || result.PreviousReadingID != r4 || result.Open != 0 || result.Segments != 1 || result.NewMeterID == "" || result.ClosingReadingID == "" {
		t.Fatalf("replacement result %+v", result)
	}
	org := fx.scope.OrgID().String()
	var stored bool
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT
 EXISTS(SELECT 1 FROM org_meters WHERE org_id=$1 AND id=$2 AND removed_at=$3 AND removal_operation_id=$4)
 AND EXISTS(SELECT 1 FROM org_meter_readings WHERE org_id=$1 AND meter_id=$2 AND id=$5 AND origin='METER_REMOVAL' AND value=184532000 AND occurred_at=$3 AND segment_id=$6 AND operation_id=$4)
 AND EXISTS(SELECT 1 FROM org_meter_segments WHERE org_id=$1 AND id=$6 AND closed_at=$3 AND closing_kind='REMOVAL' AND closing_value=184532000 AND closing_reading_id=$5 AND closing_previous_reading_id=$7 AND closing_implied_usage=342000)
 AND EXISTS(SELECT 1 FROM org_meters m JOIN org_meter_segments s ON(s.org_id,s.meter_id)=(m.org_id,m.id) JOIN org_meter_readings r ON(r.org_id,r.meter_id,r.id)=(s.org_id,s.meter_id,s.opening_reading_id)
  WHERE m.org_id=$1 AND m.id=$8 AND m.kind='ODOMETER' AND m.installed_at=$3 AND m.created_operation_id=$4 AND m.removed_at IS NULL AND s.id=$9 AND s.kind='INSTALLATION' AND s.closed_at IS NULL
  AND r.id=$10 AND r.origin='METER_INSTALLATION' AND r.value=0 AND r.author_membership_id=$11)
 AND(SELECT count(*) FROM org_audit_events WHERE org_id=$1 AND operation_id=$4 AND action='meter.replaced' AND target_type='meter' AND target_id=$2)=1`,
		org, installed.MeterID, at, in.OperationID, result.ClosingReadingID, installed.SegmentID, r4, result.NewMeterID, result.OpenedSegmentID, result.OpeningReadingID, fx.member).Scan(&stored); err != nil || !stored {
		t.Fatal("replacement rows, removal, closing reading, new meter or audit event missing", err)
	}
	next := app.MeterInstallResult{AssetID: installed.AssetID, MeterID: result.NewMeterID, SegmentID: result.OpenedSegmentID}
	r8, _ := validationRecord(t, fx, store, next, base.Add(198*time.Hour), 412_000, readingStateRow{state: "VALID"})
	track := storedTrack(t, fx, installed.AssetID)
	if f, err := track.Cumulative(base.Add(198 * time.Hour)); err != nil || !f.Known() || f.Value != 1_944_000 || f.Equivalent != 184_944_000 || f.To != r8 {
		t.Fatalf("AC1 cumulative usage %+v %v", f, err)
	}
	if f, err := track.Between(base.Add(120*time.Hour), base.Add(198*time.Hour)); err != nil || !f.Known() || f.Value != 754_000 || f.From != r4 || f.To != r8 {
		t.Fatalf("AC1 usage across the replacement %+v %v", f, err)
	}
	// Replay returns the first result; another digest is refused.
	first := result
	again, replay, err := change(t, fx, store, in)
	if err != nil || !replay || !reflect.DeepEqual(again, first) {
		t.Fatal("replay changed the first result", err)
	}
	reused := in
	reused.Label = "Another synthetic label"
	if _, _, err := change(t, fx, store, reused); !errors.Is(err, app.ErrOperationReused) {
		t.Fatal("same operation with another digest", err)
	}
	// The removed meter has no segment in service; a second odometer cannot
	// be installed while the new one is in service.
	if _, _, err := change(t, fx, store, replaceInput(t, installed, 1, at.Add(time.Hour), int64p(184_600_000), 0, nil)); !errors.Is(err, app.ErrNotFound) {
		t.Fatal("replacing a removed meter", err)
	}
	install := meterInstallInput(t, installed.AssetID, at.Add(time.Hour))
	install.ExpectedAssetRevision = 2
	if _, _, err := meterInstall(t, fx.legalFixtureState, store, meterInstallCommand(t, install)); !errors.Is(err, meter.ErrAlreadyInService) {
		t.Fatal("a second odometer installed while one is in service", err)
	}
	if latestReadingID(t, fx, store, next) != r8 {
		t.Fatal("the new meter's current value is not its latest reading")
	}
}

// B-BR-EMP-484 refusals: each is VALIDATION_FAILED (or CONFLICT for a stale
// segment count) and changes nothing.
func TestMeterSegmentReplacementRefusals(t *testing.T) {
	base := time.Now().UTC().Truncate(time.Second).Add(-10 * 24 * time.Hour)
	fx, store, installed := segmentFixture(t, 1_000_000, base, int64p(900_000_000))
	validationRecord(t, fx, store, installed, base.Add(24*time.Hour), 1_200_000, readingStateRow{state: "VALID"})
	later, _ := validationRecord(t, fx, store, installed, base.Add(72*time.Hour), 1_100_000, readingStateRow{state: "SUSPECT", flags: "READING_BELOW_PREVIOUS"})
	at := base.Add(48 * time.Hour)
	hour := resetInput(t, installed, 1, base.Add(96*time.Hour), 1_300_000, 0)
	hour.Before, _ = meter.NewValue(meter.HourMeter, 1_300_000, nil)
	hour.After, _ = meter.NewValue(meter.HourMeter, 0, nil)
	for name, c := range map[string]struct {
		in   app.MeterChangeInput
		want error
	}{
		"final below the latest valid reading": {replaceInput(t, installed, 1, base.Add(96*time.Hour), int64p(1_199_999), 0, nil), meter.ErrInvalid},
		"final not below the old modulus":      {replaceInput(t, installed, 1, base.Add(96*time.Hour), int64p(900_000_000), 0, nil), meter.ErrInvalid},
		"opening not below the new modulus":    {replaceInput(t, installed, 1, base.Add(96*time.Hour), int64p(1_300_000), 1_000, int64p(1_000)), meter.ErrInvalid},
		"a live reading after the instant":     {replaceInput(t, installed, 1, at, int64p(1_300_000), 0, nil), meter.ErrInvalid},
		"instant at the opening":               {replaceInput(t, installed, 1, base, int64p(1_300_000), 0, nil), meter.ErrInvalid},
		"instant after the database clock":     {replaceInput(t, installed, 1, time.Now().UTC().Add(time.Hour), int64p(1_300_000), 0, nil), meter.ErrInvalid},
		"stale segment count":                  {replaceInput(t, installed, 2, base.Add(96*time.Hour), int64p(1_300_000), 0, nil), app.ErrConflict},
		"foreign meter": {func() app.MeterChangeInput {
			in := replaceInput(t, installed, 1, base.Add(96*time.Hour), int64p(1_300_000), 0, nil)
			in.MeterID = onboardingID(t)
			return in
		}(), app.ErrNotFound},
		"hour values on an odometer": {hour, meter.ErrInvalid},
	} {
		t.Run(name, func(t *testing.T) {
			if _, _, err := change(t, fx, store, c.in); !errors.Is(err, c.want) {
				t.Fatalf("got %v want %v", err, c.want)
			}
			if segmentCount(t, fx, installed.MeterID) != 1 {
				t.Fatal("a refused command changed the chain")
			}
		})
	}
	if recordedState(t, fx, later) != "SUSPECT" {
		t.Fatal("refusals changed a reading")
	}
}

// AC4: a replacement with an unknown final value stores no closing reading
// and no implied usage; usage across it is INSUFFICIENT_DATA with METER_GAP
// while usage on either side stays known.
func TestMeterSegmentUnknownFinalValueMakesAGap(t *testing.T) {
	base := time.Now().UTC().Truncate(time.Second).Add(-10 * 24 * time.Hour)
	fx, store, installed := segmentFixture(t, 1_000_000, base, nil)
	r1, _ := validationRecord(t, fx, store, installed, base.Add(24*time.Hour), 1_200_000, readingStateRow{state: "VALID"})
	at := base.Add(48 * time.Hour)
	result, _, err := change(t, fx, store, replaceInput(t, installed, 1, at, nil, 0, nil))
	if err != nil || result.Close != nil || result.Implied != nil || result.ClosingReadingID != "" || result.PreviousReadingID != r1 {
		t.Fatalf("unknown final value %+v %v", result, err)
	}
	var stored bool
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT EXISTS(SELECT 1 FROM org_meter_segments WHERE org_id=$1 AND id=$2 AND closing_kind='REMOVAL' AND closing_value IS NULL AND closing_reading_id IS NULL AND closing_implied_usage IS NULL)
 AND NOT EXISTS(SELECT 1 FROM org_meter_readings WHERE org_id=$1 AND meter_id=$3 AND origin='METER_REMOVAL')`, fx.scope.OrgID().String(), installed.SegmentID, installed.MeterID).Scan(&stored); err != nil || !stored {
		t.Fatal("an unknown final value was invented", err)
	}
	next := app.MeterInstallResult{AssetID: installed.AssetID, MeterID: result.NewMeterID, SegmentID: result.OpenedSegmentID}
	r3, _ := validationRecord(t, fx, store, next, base.Add(72*time.Hour), 50_000, readingStateRow{state: "VALID"})
	track := storedTrack(t, fx, installed.AssetID)
	if f, err := track.Between(base.Add(24*time.Hour), base.Add(72*time.Hour)); err != nil || f.Known() || f.Reason != meter.MeterGap || f.From != r1 || f.To != r3 {
		t.Fatalf("AC4 usage across the gap %+v %v", f, err)
	}
	if f, err := track.Between(at, base.Add(72*time.Hour)); err != nil || !f.Known() || f.Value != 50_000 {
		t.Fatalf("usage after the gap %+v %v", f, err)
	}
	if f, err := track.Cumulative(base.Add(72 * time.Hour)); err != nil || f.Reason != meter.MeterGap {
		t.Fatalf("cumulative usage across the gap %+v %v", f, err)
	}
}

// AC2, AC3 and example D: the low reading is stored SUSPECT with
// POSSIBLE_ROLLOVER and nothing is applied; a plain confirmation is refused;
// the person's CONFIRM_ROLLOVER closes the segment at the modulus, opens the
// next at 0 with the reading, re-evaluates the later candidate and stores
// 170 000 m; the equivalent display is 1 000 120 000 m.
func TestMeterSegmentExampleDRolloverConfirmation(t *testing.T) {
	base := time.Now().UTC().Truncate(time.Second).Add(-10 * 24 * time.Hour)
	fx, store, installed := segmentFixture(t, 999_000_000, base, int64p(1_000_000_000))
	s1, _ := validationRecord(t, fx, store, installed, base.Add(24*time.Hour), 999_950_000, readingStateRow{state: "VALID"})
	s2, _ := validationRecord(t, fx, store, installed, base.Add(48*time.Hour), 120_000, readingStateRow{state: "SUSPECT", flags: "POSSIBLE_ROLLOVER"})
	s3, _ := validationRecord(t, fx, store, installed, base.Add(72*time.Hour), 130_000, readingStateRow{state: "SUSPECT", flags: "POSSIBLE_ROLLOVER"})
	if latestReadingID(t, fx, store, installed) != s1 || segmentCount(t, fx, installed.MeterID) != 1 {
		t.Fatal("AC3: a rollover was applied without a person's command")
	}
	if _, _, err := decide(t, fx, store, withOperation(decision(installed, s2, 1, app.MeterConfirm, "Synthetic confirmation"), onboardingID(t))); !errors.Is(err, meter.ErrInvalid) {
		t.Fatal("a plain confirmation applied a rollover", err)
	}
	in := withOperation(decision(installed, s2, 1, app.MeterConfirmRollover, "Painel de seis dígitos"), onboardingID(t))
	result, replay, err := decide(t, fx, store, in)
	if err != nil || replay {
		t.Fatal("rollover confirmation refused", err)
	}
	b := result.Boundary
	if result.Target.ReadingID != s2 || result.Target.State != meter.Valid || result.Target.Revision != 2 || b == nil || b.Change != app.MeterRollover || b.Close == nil || *b.Close != 1_000_000_000 ||
		b.Implied == nil || *b.Implied != 170_000 || b.Open != 0 || b.PreviousReadingID != s1 || b.ClosedSegmentID != installed.SegmentID || b.OpeningReadingID != s2 || b.Segments != 2 {
		t.Fatalf("rollover result %+v %+v", result, b)
	}
	if len(result.Reevaluated) != 1 || result.Reevaluated[0].ReadingID != s3 || result.Reevaluated[0].State != meter.Valid || result.Reevaluated[0].Revision != 2 {
		t.Fatalf("the later candidate was not re-evaluated in the new segment %+v", result.Reevaluated)
	}
	if rows := readingStates(t, fx, s2); len(rows) != 2 || rows[1].cause != "ROLLOVER" || rows[1].state != "VALID" || rows[1].operation != in.OperationID {
		t.Fatalf("rollover history %+v", rows)
	}
	if latestReadingID(t, fx, store, app.MeterInstallResult{AssetID: installed.AssetID, MeterID: installed.MeterID, SegmentID: b.OpenedSegmentID}) != s3 {
		t.Fatal("the current value did not move to the new segment")
	}
	track := storedTrack(t, fx, installed.AssetID)
	if f, err := track.Between(base.Add(24*time.Hour), base.Add(48*time.Hour)); err != nil || !f.Known() || f.Value != 170_000 || f.From != s1 || f.To != s2 {
		t.Fatalf("AC2 usage across the rollover %+v %v", f, err)
	}
	if f, err := track.Cumulative(base.Add(48 * time.Hour)); err != nil || f.Equivalent != 1_000_120_000 {
		t.Fatalf("example D equivalent display %+v %v", f, err)
	}
	first := result
	if again, replay, err := decide(t, fx, store, in); err != nil || !replay || !reflect.DeepEqual(again, first) {
		t.Fatal("rollover replay changed the first result", err)
	}
	// The confirmed reading opens the new segment: an anchor.
	if _, _, err := decide(t, fx, store, withOperation(decision(installed, s2, 2, app.MeterVoid, "Synthetic void"), onboardingID(t))); !errors.Is(err, meter.ErrInvalid) {
		t.Fatal("a segment anchor was decided", err)
	}
	// A meter without a modulus never confirms a rollover.
	fx2, store2, plain := segmentFixture(t, 1_000_000, base, nil)
	validationRecord(t, fx2, store2, plain, base.Add(24*time.Hour), 1_200_000, readingStateRow{state: "VALID"})
	low, _ := validationRecord(t, fx2, store2, plain, base.Add(48*time.Hour), 120_000, readingStateRow{state: "SUSPECT", flags: "READING_BELOW_PREVIOUS"})
	if _, _, err := decide(t, fx2, store2, withOperation(decision(plain, low, 1, app.MeterConfirmRollover, "Synthetic rollover"), onboardingID(t))); !errors.Is(err, meter.ErrInvalid) {
		t.Fatal("rollover confirmed on a meter without a modulus", err)
	}
}

// AC5 and BUC-EMP-246: a reset recorded before a low reading returns it to
// VALID in the new segment, where monotonicity is checked alone; refusals
// leave the chain unchanged.
func TestMeterSegmentResetReturnsLaterReadingsToValid(t *testing.T) {
	base := time.Now().UTC().Truncate(time.Second).Add(-10 * 24 * time.Hour)
	fx, store, installed := segmentFixture(t, 1_000_000, base, nil)
	r1, _ := validationRecord(t, fx, store, installed, base.Add(48*time.Hour), 1_200_000, readingStateRow{state: "VALID"})
	r2, _ := validationRecord(t, fx, store, installed, base.Add(97*time.Hour), 5_000, readingStateRow{state: "SUSPECT", flags: "READING_BELOW_PREVIOUS"})
	for name, c := range map[string]struct {
		in   app.MeterChangeInput
		want error
	}{
		"a valid reading after the instant": {resetInput(t, installed, 1, base.Add(40*time.Hour), 1_100_000, 0), meter.ErrInvalid},
		"before below the latest valid":     {resetInput(t, installed, 1, base.Add(96*time.Hour), 1_199_999, 0), meter.ErrInvalid},
		"a live reading at the instant":     {resetInput(t, installed, 1, base.Add(97*time.Hour), 1_500_000, 0), meter.ErrInvalid},
		"stale segment count":               {resetInput(t, installed, 2, base.Add(96*time.Hour), 1_500_000, 0), app.ErrConflict},
	} {
		t.Run(name, func(t *testing.T) {
			if _, _, err := change(t, fx, store, c.in); !errors.Is(err, c.want) {
				t.Fatalf("got %v want %v", err, c.want)
			}
			if segmentCount(t, fx, installed.MeterID) != 1 {
				t.Fatal("a refused reset changed the chain")
			}
		})
	}
	at := base.Add(96 * time.Hour)
	in := resetInput(t, installed, 1, at, 1_500_000, 0)
	result, _, err := change(t, fx, store, in)
	if err != nil || result.Change != app.MeterReset || result.Close == nil || *result.Close != 1_500_000 || result.Implied == nil || *result.Implied != 300_000 || result.PreviousReadingID != r1 ||
		result.Open != 0 || result.Segments != 2 || result.NewMeterID != "" || result.ClosingReadingID != "" {
		t.Fatalf("reset result %+v %v", result, err)
	}
	if len(result.Reevaluated) != 1 || result.Reevaluated[0].ReadingID != r2 || result.Reevaluated[0].State != meter.Valid {
		t.Fatalf("AC5: the low reading after the reset stayed suspect %+v", result.Reevaluated)
	}
	var stored bool
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT EXISTS(SELECT 1 FROM org_meter_segments s JOIN org_meter_readings r ON(r.org_id,r.meter_id,r.id)=(s.org_id,s.meter_id,s.opening_reading_id)
 WHERE s.org_id=$1 AND s.id=$2 AND s.kind='RESET' AND s.opened_at=$3 AND s.opened_operation_id=$4 AND r.origin='STANDALONE' AND r.method='TYPED' AND r.value=0 AND r.segment_id=s.id AND r.author_membership_id=$5 AND r.capture_kind='WEB_SESSION')
 AND org_meter_reading_segment($1,$6,$7)=$2
 AND(SELECT segment_id FROM org_meter_readings WHERE org_id=$1 AND id=$7)=$8`, fx.scope.OrgID().String(), result.OpenedSegmentID, at, in.OperationID, fx.member, installed.MeterID, r2, installed.SegmentID).Scan(&stored); err != nil || !stored {
		t.Fatal("reset segment, opening reading or derived segment missing", err)
	}
	open := app.MeterInstallResult{AssetID: installed.AssetID, MeterID: installed.MeterID, SegmentID: result.OpenedSegmentID}
	if latestReadingID(t, fx, store, open) != r2 {
		t.Fatal("the reset segment's reading is not current")
	}
	if f, err := storedTrack(t, fx, installed.AssetID).Cumulative(base.Add(97 * time.Hour)); err != nil || !f.Known() || f.Value != 505_000 {
		t.Fatalf("cumulative usage across the reset %+v %v", f, err)
	}
	// A reading now arriving for the closed segment names a segment out of service.
	if _, _, err := meterRecord(t, fx, store, fx.source, meterObservedCommand(t, installed, base.Add(98*time.Hour), 6_000)); err == nil {
		t.Fatal("a reading was recorded into a closed segment")
	}
}

// Concurrent replacement and reading intake on one meter serialise in both
// arrival orders behind the organization root lock: the replacement either
// measures from the committed reading, or the reading finds the meter
// removed and nothing is stored.
func TestMeterSegmentConcurrentReplacementAndIntakeInBothOrders(t *testing.T) {
	for _, order := range []string{"intake-first", "replacement-first"} {
		t.Run(order, func(t *testing.T) {
			base := time.Now().UTC().Truncate(time.Second).Add(-10 * 24 * time.Hour)
			fx, store, installed := segmentFixture(t, 1_000_000, base, nil)
			validationRecord(t, fx, store, installed, base.Add(24*time.Hour), 1_200_000, readingStateRow{state: "VALID"})
			reading := meterObservedCommand(t, installed, base.Add(36*time.Hour), 1_250_000)
			replacement := replaceInput(t, installed, 1, base.Add(48*time.Hour), int64p(1_300_000), 0, nil)
			holder := make(chan int, 1)
			release := make(chan struct{})
			var once sync.Once
			free := func() { once.Do(func() { close(release) }) }
			defer free()
			done := make(chan error, 1)
			go func() {
				done <- database.WithOrgTx(context.Background(), fx.f.Runtime, fx.scope, func(tx pgx.Tx) error {
					if _, err := tx.Exec(context.Background(), `SELECT state FROM organizations WHERE org_id=$1 FOR UPDATE`, fx.scope.OrgID().String()); err != nil {
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
			var intakeErr, replaceErr error
			var result app.MeterBoundaryResult
			var wg sync.WaitGroup
			intake := func() {
				err := database.WithOrgTx(context.Background(), fx.f.Runtime, fx.scope, func(tx pgx.Tx) error {
					_, _, err := store.RecordEmbedded(context.Background(), syncapp.Borrow(tx), fx.scope, app.MeterSource{Device: &fx.source}, fx.source.MembershipID, reading)
					return err
				})
				intakeErr = err
			}
			replace := func() { result, _, replaceErr = change(t, fx, store, replacement) }
			first, second := intake, replace
			if order == "replacement-first" {
				first, second = replace, intake
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
			if replaceErr != nil {
				t.Fatal("replacement failed", replaceErr)
			}
			in, _ := reading.Snapshot()
			var stored int
			if err := fx.f.Admin.QueryRow(t.Context(), `SELECT count(*) FROM org_meter_readings WHERE org_id=$1 AND id=$2`, fx.scope.OrgID().String(), in.ReadingID).Scan(&stored); err != nil {
				t.Fatal(err)
			}
			if order == "intake-first" {
				if intakeErr != nil || stored != 1 || result.PreviousReadingID != in.ReadingID || result.Implied == nil || *result.Implied != 50_000 {
					t.Fatalf("replacement did not measure from the committed reading %+v %v", result, intakeErr)
				}
				return
			}
			if !errors.Is(intakeErr, app.ErrNotFound) || stored != 0 || result.Implied == nil || *result.Implied != 100_000 {
				t.Fatalf("reading recorded on a removed meter %+v %v", result, intakeErr)
			}
		})
	}
}
