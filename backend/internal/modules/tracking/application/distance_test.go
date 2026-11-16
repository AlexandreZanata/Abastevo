package application_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	fleetapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	meter "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain/meter"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/tracking/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/tracking/domain"
	tripapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/application"
	trip "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/domain"
)

// EMP-GPS-006 application oracle (B-BR-EMP-491..494, EDR-318/319).

type usageCall struct {
	asset    string
	kind     meter.Kind
	from, to time.Time
}

// fakeMeters is the METER-006 port: it records every question.
type fakeMeters struct {
	calls  []usageCall
	answer fleetapp.MeterUsage
	err    error
}

func (f *fakeMeters) UsageBetween(_ context.Context, _ kernel.OrgScope, asset string, kind meter.Kind, from, to time.Time) (fleetapp.MeterUsage, error) {
	f.calls = append(f.calls, usageCall{asset, kind, from, to})
	return f.answer, f.err
}

var (
	tripStart  = time.Date(2026, 11, 16, 8, 0, 0, 123, time.UTC)
	tripFinish = tripStart.Add(5*time.Hour + 40*time.Minute)
	assetID    = policyID(81)
)

// distanceTrip is a stored trip of the asset; finish adds its finish.
func distanceTrip(state trip.State, finish bool) tripapp.Record {
	r := tripapp.Record{ID: policyID(80), AssetID: assetID, Declared: orgtime.Declaration{OccurredAt: tripStart}, State: state}
	if finish {
		r.Finish = &tripapp.FinishRecord{Declared: orgtime.Declaration{OccurredAt: tripFinish}}
	}
	return r
}

func anchor(id int, value int64, at time.Time) *fleetapp.MeterUsageAnchor {
	return &fleetapp.MeterUsageAnchor{ReadingID: policyID(id), Value: value, OccurredAt: at}
}

// usageAnswer is the port's answer for the trip's window.
func usageAnswer(reason meter.Insufficient, usage int64, start, end *fleetapp.MeterUsageAnchor) fleetapp.MeterUsage {
	return fleetapp.MeterUsage{AssetID: assetID, Kind: meter.Odometer, From: tripStart, To: tripFinish, Reason: reason, Usage: usage, Start: start, End: end,
		Method: fleetapp.MeterUsageMethod, Revision: 7}
}

func readOdometer(t *testing.T, record tripapp.Record, meters *fakeMeters) (app.OdometerDistance, error) {
	t.Helper()
	return app.ReadOdometer(t.Context(), policyScope(t, 1), record, meters)
}

// The meter port is asked only for a finished trip's own window, for the
// asset's odometer; without a finish the reason is stated without asking.
func TestReadOdometerAsksOnlyForAFinishedTripsWindow(t *testing.T) {
	closed := distanceTrip(trip.Completed, false)
	closed.Closure = &tripapp.ClosureRecord{ClosedAt: tripFinish}
	for name, c := range map[string]struct {
		record tripapp.Record
		reason domain.DistanceReason
	}{
		"in progress": {distanceTrip(trip.InProgress, false), domain.ReasonTripNotFinished},
		"paused":      {distanceTrip(trip.Paused, false), domain.ReasonTripNotFinished},
		"cancelled":   {distanceTrip(trip.Cancelled, false), domain.ReasonTripCancelled},
		"closure":     {closed, domain.ReasonNoFinalReading},
	} {
		meters := &fakeMeters{answer: usageAnswer("", 290_000, anchor(1, 184_320_000, tripStart), anchor(2, 184_610_000, tripFinish))}
		got, err := readOdometer(t, c.record, meters)
		if err != nil || len(meters.calls) != 0 {
			t.Fatal(name, "asked the meter port", err, len(meters.calls))
		}
		want := domain.DistanceSource{Kind: domain.KindOdometer, Status: domain.SourceInsufficient, Reason: c.reason}
		if got.Source != want || got.AssetID != assetID || got.From != nil || got.To != nil || got.Start != nil || got.End != nil || got.Method != "" || got.Revision != 0 {
			t.Fatalf("%s: %+v", name, got.Source)
		}
	}
	meters := &fakeMeters{answer: usageAnswer("", 290_000, anchor(1, 184_320_000, tripStart), anchor(2, 184_610_000, tripFinish))}
	got, err := readOdometer(t, distanceTrip(trip.Completed, true), meters)
	if err != nil || len(meters.calls) != 1 || meters.calls[0] != (usageCall{assetID, meter.Odometer, tripStart, tripFinish}) {
		t.Fatal("the trip window was not asked exactly once", err, meters.calls)
	}
	if got.Source != (domain.DistanceSource{Kind: domain.KindOdometer, Status: domain.SourceOK, Metres: 290_000}) || got.Method != fleetapp.MeterUsageMethod || got.Revision != 7 ||
		got.From == nil || !got.From.Equal(tripStart) || got.To == nil || !got.To.Equal(tripFinish) {
		t.Fatalf("measured odometer %+v", got.Source)
	}
	if got.Start == nil || got.Start.ReadingID != policyID(1) || got.Start.Value != 184_320_000 || !got.Start.OccurredAt.Equal(tripStart) ||
		got.End == nil || got.End.ReadingID != policyID(2) || got.End.Value != 184_610_000 || !got.End.OccurredAt.Equal(tripFinish) {
		t.Fatal("the anchors are not named", got.Start, got.End)
	}
	if _, err := readOdometer(t, distanceTrip(trip.InProgress, true), meters); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("an in-progress trip with a finish was read", err)
	}
	if _, err := app.ReadOdometer(t.Context(), policyScope(t, 1), distanceTrip(trip.Completed, true), nil); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("a missing meter port answered", err)
	}
}

// Named negative "absent basis as zero" (EDR-319): a usage that is not
// measured between the trip's own readings is a reason, never a number.
func TestReadOdometerNeverTurnsAnAbsentReadingIntoZero(t *testing.T) {
	earlier := tripStart.Add(-48 * time.Hour)
	middle := tripStart.Add(2 * time.Hour)
	missing := func(r domain.DistanceReason) domain.DistanceSource {
		return domain.DistanceSource{Kind: domain.KindOdometer, Status: domain.SourceInsufficient, Reason: r}
	}
	cases := []struct {
		name       string
		answer     fleetapp.MeterUsage
		err        error
		want       domain.DistanceSource
		start, end bool
	}{
		// The finish carried no reading: the latest reading at the finish is
		// the start reading, so the port answers 0 m.
		{"finish without its reading", usageAnswer("", 0, anchor(1, 184_320_000, tripStart), anchor(1, 184_320_000, tripStart)), nil, missing(domain.ReasonNoFinalReading), true, false},
		// A mid-trip reading would give a partial distance.
		{"finish reading voided, mid-trip reading", usageAnswer("", 120_000, anchor(1, 184_320_000, tripStart), anchor(3, 184_440_000, middle)), nil, missing(domain.ReasonNoFinalReading), true, false},
		{"start reading not valid", usageAnswer("", 400_000, anchor(4, 184_210_000, earlier), anchor(2, 184_610_000, tripFinish)), nil, missing(domain.ReasonNoStartReading), false, true},
		{"neither reading", usageAnswer("", 10, anchor(4, 184_210_000, earlier), anchor(4, 184_210_010, earlier)), nil, missing(domain.ReasonNoStartReading), false, false},
		{"no reading before the start", usageAnswer(meter.NoReadingBefore, 0, nil, anchor(2, 184_610_000, tripFinish)), nil, missing(domain.ReasonNoReadingBefore), false, true},
		{"meter replaced with an unknown final value", usageAnswer(meter.MeterGap, 0, anchor(1, 184_320_000, tripStart), anchor(5, 1_000, tripFinish)), nil, missing(domain.ReasonMeterGap), true, true},
		{"no odometer on the asset", fleetapp.MeterUsage{}, fleetapp.ErrNotFound, missing(domain.ReasonNoMeter), false, false},
		{"projection not built", fleetapp.MeterUsage{}, fleetapp.ErrUnavailable, missing(domain.ReasonMeterUnavailable), false, false},
		{"measured zero", usageAnswer("", 0, anchor(1, 184_320_000, tripStart), anchor(2, 184_320_000, tripFinish)), nil,
			domain.DistanceSource{Kind: domain.KindOdometer, Status: domain.SourceOK, Metres: 0}, true, true},
	}
	for _, c := range cases {
		meters := &fakeMeters{answer: c.answer, err: c.err}
		got, err := readOdometer(t, distanceTrip(trip.Completed, true), meters)
		if err != nil || got.Source != c.want {
			t.Fatalf("%s: %+v %v", c.name, got.Source, err)
		}
		if (got.Start != nil) != c.start || (got.End != nil) != c.end {
			t.Fatalf("%s: an anchor that is not the trip's reading was named %v %v", c.name, got.Start, got.End)
		}
		if got.Start != nil && !got.Start.OccurredAt.Equal(tripStart) || got.End != nil && !got.End.OccurredAt.Equal(tripFinish) {
			t.Fatalf("%s: anchor instants", c.name)
		}
		if c.err == nil && (got.Method != fleetapp.MeterUsageMethod || got.Revision != 7) {
			t.Fatalf("%s: the port's method and revision are not stated", c.name)
		}
	}
	// Any other answer of the port is an outage, never a figure.
	for name, c := range map[string]struct {
		answer fleetapp.MeterUsage
		err    error
	}{
		"invalid instants": {fleetapp.MeterUsage{}, meter.ErrInvalid},
		"store outage":     {fleetapp.MeterUsage{}, errors.New("synthetic outage")},
		"another asset": {func() fleetapp.MeterUsage {
			u := usageAnswer("", 1, anchor(1, 1, tripStart), anchor(2, 2, tripFinish))
			u.AssetID = policyID(99)
			return u
		}(), nil},
		"hour meter seconds": {func() fleetapp.MeterUsage {
			u := usageAnswer("", 1, anchor(1, 1, tripStart), anchor(2, 2, tripFinish))
			u.Kind = meter.HourMeter
			return u
		}(), nil},
		"another window": {func() fleetapp.MeterUsage {
			u := usageAnswer("", 1, anchor(1, 1, tripStart), anchor(2, 2, tripFinish))
			u.To = tripFinish.Add(time.Second)
			return u
		}(), nil},
		"unversioned": {func() fleetapp.MeterUsage {
			u := usageAnswer("", 1, anchor(1, 1, tripStart), anchor(2, 2, tripFinish))
			u.Method = ""
			return u
		}(), nil},
		"negative usage":        {usageAnswer("", -1, anchor(1, 1, tripStart), anchor(2, 2, tripFinish)), nil},
		"above the ceiling":     {usageAnswer("", domain.MaxDistanceMetres+1, anchor(1, 1, tripStart), anchor(2, 2, tripFinish)), nil},
		"unknown reason":        {usageAnswer("ESTIMATED", 0, anchor(1, 1, tripStart), anchor(2, 2, tripFinish)), nil},
		"unknown with a number": {usageAnswer(meter.MeterGap, 5, anchor(1, 1, tripStart), anchor(2, 2, tripFinish)), nil},
		"known without anchors": {usageAnswer("", 5, nil, nil), nil},
		"negative revision": {func() fleetapp.MeterUsage {
			u := usageAnswer("", 1, anchor(1, 1, tripStart), anchor(2, 2, tripFinish))
			u.Revision = -1
			return u
		}(), nil},
	} {
		meters := &fakeMeters{answer: c.answer, err: c.err}
		if got, err := readOdometer(t, distanceTrip(trip.Completed, true), meters); !errors.Is(err, app.ErrUnavailable) || got.Source.Metres != 0 {
			t.Fatal(name, "became a figure", got.Source, err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	meters := &fakeMeters{err: context.Canceled}
	if _, err := app.ReadOdometer(ctx, policyScope(t, 1), distanceTrip(trip.Completed, true), meters); !errors.Is(err, context.Canceled) {
		t.Fatal("a cancelled read was not reported as cancelled", err)
	}
}

func coverage(v int64) *int64 { return &v }
func method(v string) *string { return &v }

// preciseSummary is a PRECISE revision 2 with 950 permille and two gaps.
func preciseSummary(metres int64) *app.SummaryFacts {
	return &app.SummaryFacts{Revision: 2, Method: domain.MethodVersion, Quality: domain.QualityPrecise, CoveragePermille: coverage(950), ExpectedSeconds: 20_400,
		GapSeconds: 1_020, Gaps: 2, DistanceMetres: &metres, DistanceMethod: method(domain.DistanceMethod), ProcessedAt: tripFinish.Add(time.Minute)}
}

// The GPS source says why there is no GPS distance and never invents one.
func TestTrackOfStatesWhyThereIsNoGPSDistance(t *testing.T) {
	session := policyID(90)
	gps := func(status domain.SourceStatus, reason domain.DistanceReason, metres int64, q domain.Quality) domain.DistanceSource {
		return domain.DistanceSource{Kind: domain.KindGPS, Status: status, Reason: reason, Metres: metres, Quality: q}
	}
	withheld, err := app.TrackOf(app.DistanceFacts{Trip: distanceTrip(trip.Completed, true), Withheld: true})
	if err != nil || withheld.Source != gps(domain.SourceWithheld, "", 0, "") || withheld.SessionID != "" || withheld.Revision != 0 || withheld.CoveragePermille != nil ||
		withheld.Gaps != 0 || withheld.ProcessedAt != nil || withheld.Method != "" || withheld.DistanceMethod != "" || withheld.ExpectedSeconds != 0 || withheld.GapSeconds != 0 {
		t.Fatalf("a withheld source shows evidence %+v %v", withheld.Source, err)
	}
	none, err := app.TrackOf(app.DistanceFacts{Trip: distanceTrip(trip.Completed, true)})
	if err != nil || none.Source != gps(domain.SourceInsufficient, domain.ReasonNoTrack, 0, "") || none.SessionID != "" {
		t.Fatal("no session", none.Source, err)
	}
	pending, err := app.TrackOf(app.DistanceFacts{Trip: distanceTrip(trip.InProgress, false), SessionID: session})
	if err != nil || pending.Source != gps(domain.SourceInsufficient, domain.ReasonNotProcessed, 0, "") || pending.SessionID != session || pending.Revision != 0 {
		t.Fatal("an unprocessed session", pending.Source, err)
	}
	insufficient := &app.SummaryFacts{Revision: 1, Method: domain.MethodVersion, Quality: domain.QualityInsufficient, CoveragePermille: coverage(420), ExpectedSeconds: 1_000,
		GapSeconds: 580, Gaps: 1, ProcessedAt: tripFinish}
	short, err := app.TrackOf(app.DistanceFacts{Trip: distanceTrip(trip.Completed, true), SessionID: session, Summary: insufficient})
	if err != nil || short.Source != gps(domain.SourceInsufficient, domain.ReasonNoGPSDistance, 0, domain.QualityInsufficient) || short.Revision != 1 ||
		*short.CoveragePermille != 420 || short.Gaps != 1 || short.GapSeconds != 580 || short.ExpectedSeconds != 1_000 || short.DistanceMethod != "" || short.Method != domain.MethodVersion {
		t.Fatalf("a summary without a distance %+v %v", short.Source, err)
	}
	precise, err := app.TrackOf(app.DistanceFacts{Trip: distanceTrip(trip.Completed, true), SessionID: session, Summary: preciseSummary(171_900)})
	if err != nil || precise.Source != gps(domain.SourceOK, "", 171_900, domain.QualityPrecise) || precise.SessionID != session || precise.Revision != 2 || *precise.CoveragePermille != 950 ||
		precise.Gaps != 2 || precise.GapSeconds != 1_020 || precise.DistanceMethod != domain.DistanceMethod || precise.ProcessedAt == nil || !precise.ProcessedAt.Equal(tripFinish.Add(time.Minute)) {
		t.Fatalf("a precise summary %+v %v", precise.Source, err)
	}
	partial := preciseSummary(120_000)
	partial.Quality, partial.CoveragePermille = domain.QualityApproximate, coverage(700)
	if got, err := app.TrackOf(app.DistanceFacts{Trip: distanceTrip(trip.Completed, true), SessionID: session, Summary: partial}); err != nil ||
		got.Source != gps(domain.SourceOK, "", 120_000, domain.QualityApproximate) || *got.CoveragePermille != 700 {
		t.Fatal("a partial distance keeps its quality and coverage", got.Source, err)
	}
	refused := map[string]app.DistanceFacts{
		"withheld with a session":        {Withheld: true, SessionID: session},
		"withheld with a summary":        {Withheld: true, Summary: preciseSummary(1)},
		"summary without a session":      {Summary: preciseSummary(1)},
		"another method":                 {SessionID: session, Summary: func() *app.SummaryFacts { s := preciseSummary(1); s.Method = "gps-track-v0"; return s }()},
		"distance without its method":    {SessionID: session, Summary: func() *app.SummaryFacts { s := preciseSummary(1); s.DistanceMethod = nil; return s }()},
		"method without a distance":      {SessionID: session, Summary: func() *app.SummaryFacts { s := preciseSummary(1); s.DistanceMetres = nil; return s }()},
		"insufficient with a distance":   {SessionID: session, Summary: func() *app.SummaryFacts { s := preciseSummary(1); s.Quality = domain.QualityInsufficient; return s }()},
		"unknown quality":                {SessionID: session, Summary: func() *app.SummaryFacts { s := preciseSummary(1); s.Quality = "GOOD"; return s }()},
		"coverage without expected time": {SessionID: session, Summary: func() *app.SummaryFacts { s := preciseSummary(1); s.ExpectedSeconds, s.GapSeconds = 0, 0; return s }()},
		"expected time without coverage": {SessionID: session, Summary: func() *app.SummaryFacts { s := preciseSummary(1); s.CoveragePermille = nil; return s }()},
		"gaps above expected time":       {SessionID: session, Summary: func() *app.SummaryFacts { s := preciseSummary(1); s.GapSeconds = 30_000; return s }()},
		"negative gap count":             {SessionID: session, Summary: func() *app.SummaryFacts { s := preciseSummary(1); s.Gaps = -1; return s }()},
		"revision zero":                  {SessionID: session, Summary: func() *app.SummaryFacts { s := preciseSummary(1); s.Revision = 0; return s }()},
		"negative distance":              {SessionID: session, Summary: preciseSummary(-1)},
		"above the ceiling":              {SessionID: session, Summary: preciseSummary(domain.MaxDistanceMetres + 1)},
	}
	for name, facts := range refused {
		facts.Trip = distanceTrip(trip.Completed, true)
		if got, err := app.TrackOf(facts); !errors.Is(err, app.ErrUnavailable) || got.Source.Metres != 0 {
			t.Fatal("inconsistent track facts accepted:", name, got.Source)
		}
	}
}

func measured(metres int64) app.OdometerDistance {
	from, to := tripStart, tripFinish
	return app.OdometerDistance{Source: domain.DistanceSource{Kind: domain.KindOdometer, Status: domain.SourceOK, Metres: metres}, AssetID: assetID, From: &from, To: &to,
		Start: &app.OdometerAnchor{ReadingID: policyID(1), Value: 184_320_000, OccurredAt: tripStart}, End: &app.OdometerAnchor{ReadingID: policyID(2), Value: 184_320_000 + metres, OccurredAt: tripFinish},
		Method: fleetapp.MeterUsageMethod, Revision: 7}
}

func unmeasured(r domain.DistanceReason) app.OdometerDistance {
	return app.OdometerDistance{Source: domain.DistanceSource{Kind: domain.KindOdometer, Status: domain.SourceInsufficient, Reason: r}, AssetID: assetID}
}

// The composed answer keeps the four sources apart, states the basis and
// evaluates the divergence on the evidence only (B-BR-EMP-491..493).
func TestComposeDistancesKeepsTheSourcesApart(t *testing.T) {
	session := policyID(90)
	facts := app.DistanceFacts{Trip: distanceTrip(trip.Completed, true), SessionID: session, Summary: preciseSummary(158_300)}
	view, err := app.ComposeDistances(facts, measured(186_400), domain.DefaultThresholds())
	if err != nil {
		t.Fatal(err)
	}
	if view.TripID != policyID(80) || view.Method != domain.SourcesMethod || view.Odometer.Source.Metres != 186_400 || view.GPS.Source.Metres != 158_300 || view.GPS.SessionID != session {
		t.Fatalf("composed sources %+v %+v", view.Odometer.Source, view.GPS.Source)
	}
	if view.Planned != (domain.DistanceSource{Kind: domain.KindPlanned, Status: domain.SourceInsufficient, Reason: domain.ReasonNoRoutePlan}) ||
		view.Estimate != (domain.DistanceSource{Kind: domain.KindEstimate, Status: domain.SourceInsufficient, Reason: domain.ReasonNoRouteCompute}) {
		t.Fatal("a planned distance or estimate was invented", view.Planned, view.Estimate)
	}
	if view.Travelled != (domain.Travelled{Status: domain.SourceOK, Basis: domain.BasisOdometer, Metres: 186_400}) ||
		view.Divergence != (domain.Divergence{Status: domain.DivergenceRaised, Odometer: 186_400, GPS: 158_300, Difference: 28_100, Thresholds: domain.DefaultThresholds()}) {
		t.Fatalf("§10.5 case 2 %+v %+v", view.Travelled, view.Divergence)
	}
	// §10.5 case 5: no final reading, precise track: basis por GPS.
	view, err = app.ComposeDistances(app.DistanceFacts{Trip: distanceTrip(trip.Completed, true), SessionID: session, Summary: preciseSummary(171_900)}, unmeasured(domain.ReasonNoFinalReading), domain.DefaultThresholds())
	if err != nil || view.Travelled != (domain.Travelled{Status: domain.SourceOK, Basis: domain.BasisGPS, Metres: 171_900}) || view.Divergence.Reason != domain.ReasonNoOdometerDistance {
		t.Fatalf("por GPS %+v %+v %v", view.Travelled, view.Divergence, err)
	}
	// Withheld: the odometer stays the basis, nothing about the track shows.
	view, err = app.ComposeDistances(app.DistanceFacts{Trip: distanceTrip(trip.Completed, true), Withheld: true}, measured(186_400), domain.DefaultThresholds())
	if err != nil || view.GPS.Source.Status != domain.SourceWithheld || view.Travelled.Basis != domain.BasisOdometer || view.Divergence.Reason != domain.ReasonGPSWithheld {
		t.Fatalf("withheld %+v %+v %v", view.GPS.Source, view.Divergence, err)
	}
	view, err = app.ComposeDistances(app.DistanceFacts{Trip: distanceTrip(trip.Completed, true), Withheld: true}, unmeasured(domain.ReasonNoFinalReading), domain.DefaultThresholds())
	if err != nil || view.Travelled != (domain.Travelled{Status: domain.SourceInsufficient, Reason: domain.ReasonNoFinalReading}) {
		t.Fatalf("withheld without odometer %+v %v", view.Travelled, err)
	}
	if m, _, ok := view.Travelled.Value(); ok || m != 0 {
		t.Fatal("an absent basis reads as a value")
	}
	for name, c := range map[string]struct {
		facts    app.DistanceFacts
		odometer app.OdometerDistance
	}{
		"another asset's odometer": {facts, func() app.OdometerDistance { o := measured(1); o.AssetID = policyID(99); return o }()},
		"odometer as gps":          {facts, func() app.OdometerDistance { o := measured(1); o.Source.Kind = domain.KindGPS; return o }()},
		"inconsistent track":       {app.DistanceFacts{Trip: distanceTrip(trip.Completed, true), Summary: preciseSummary(1)}, measured(1)},
	} {
		if _, err := app.ComposeDistances(c.facts, c.odometer, domain.DefaultThresholds()); !errors.Is(err, app.ErrUnavailable) {
			t.Fatal("an inconsistent composition was answered:", name, err)
		}
	}
	if _, err := app.ComposeDistances(facts, measured(1), domain.Thresholds{}); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("thresholds outside their ranges were used", err)
	}
}

type distanceRepository struct {
	calls   int
	scope   kernel.OrgScope
	actor   app.TrackActor
	request app.DistanceRequest
	view    app.DistanceView
	err     error
}

func (r *distanceRepository) ReadDistances(_ context.Context, s kernel.OrgScope, a app.TrackActor, q app.DistanceRequest) (app.DistanceView, error) {
	r.calls++
	r.scope, r.actor, r.request = s, a, q
	return r.view, r.err
}

// The port admits only a composed service, a valid company proof and a
// canonical request before its repository is asked.
func TestDistanceServiceGate(t *testing.T) {
	valid := app.DistanceRequest{TripID: policyID(80), MembershipID: policyID(3)}
	if valid.Validate() != nil {
		t.Fatal("a valid request is refused")
	}
	for name, r := range map[string]app.DistanceRequest{
		"no trip": {MembershipID: policyID(3)}, "no member": {TripID: policyID(80)}, "upper trip": {TripID: strings.ToUpper(policyID(80)), MembershipID: policyID(3)},
		"nil uuid": {TripID: "00000000-0000-0000-0000-000000000000", MembershipID: policyID(3)}, "spaced member": {TripID: policyID(80), MembershipID: " " + policyID(3)},
	} {
		if !errors.Is(r.Validate(), app.ErrMalformed) {
			t.Fatal("an invalid request was admitted", name)
		}
	}
	actor := app.TrackActor{AccountID: policyID(1), SessionID: policyID(2)}
	repository := &distanceRepository{view: app.DistanceView{TripID: policyID(80)}}
	s := policyScope(t, 1)
	if _, err := (app.DistanceService{Repository: repository}).Read(t.Context(), s, actor, valid); !errors.Is(err, app.ErrDistanceDisabled) {
		t.Fatal("a disabled port read", err)
	}
	if _, err := (app.DistanceService{Enabled: true}).Read(t.Context(), s, actor, valid); !errors.Is(err, app.ErrDistanceDisabled) {
		t.Fatal("a port without a repository read", err)
	}
	service := app.DistanceService{Enabled: true, Repository: repository}
	if _, err := service.Read(t.Context(), s, app.TrackActor{AccountID: policyID(1)}, valid); !errors.Is(err, app.ErrTrackUnauthorized) {
		t.Fatal("an actor without a session was admitted", err)
	}
	if _, err := service.Read(t.Context(), s, actor, app.DistanceRequest{TripID: policyID(80)}); !errors.Is(err, app.ErrMalformed) {
		t.Fatal("a request without a membership was admitted", err)
	}
	if repository.calls != 0 {
		t.Fatal("a refused read reached the repository")
	}
	view, err := service.Read(t.Context(), s, actor, valid)
	if err != nil || view.TripID != policyID(80) || repository.calls != 1 || repository.scope != s || repository.actor != actor || repository.request != valid {
		t.Fatal("a valid read was not delegated once", err, repository.calls)
	}
	repository.err = app.ErrAccessUnrecorded
	if _, err := service.Read(t.Context(), s, actor, valid); !errors.Is(err, app.ErrAccessUnrecorded) {
		t.Fatal("a repository refusal was hidden", err)
	}
}

// Distance answers print redacted like every tracking value.
func TestDistanceValuesAreRedacted(t *testing.T) {
	view, err := app.ComposeDistances(app.DistanceFacts{Trip: distanceTrip(trip.Completed, true), SessionID: policyID(90), Summary: preciseSummary(158_300)}, measured(186_400), domain.DefaultThresholds())
	if err != nil || view.Odometer.Start == nil {
		t.Fatal("no composed answer to print", err)
	}
	values := []any{view, view.Odometer, view.GPS, *view.Odometer.Start, app.DistanceRequest{TripID: policyID(80), MembershipID: policyID(3)},
		app.DistanceFacts{Trip: distanceTrip(trip.Completed, true), SessionID: policyID(90)}, *preciseSummary(158_300)}
	for _, v := range values {
		var out bytes.Buffer
		slog.New(slog.NewJSONHandler(&out, nil)).Info("value", "v", v)
		raw, _ := json.Marshal(v)
		for _, s := range []string{fmt.Sprintf("%v %+v %#v %s", v, v, v, v), string(raw), out.String()} {
			for _, secret := range []string{policyID(80), policyID(90), policyID(3), "158300", "186400", "184320000"} {
				if strings.Contains(s, secret) {
					t.Fatalf("%T leaked %q", v, secret)
				}
			}
		}
	}
}
