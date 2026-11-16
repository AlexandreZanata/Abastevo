package domain_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/tracking/domain"
)

var pt0 = time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)

func pSec(n float64) time.Time { return pt0.Add(time.Duration(n * float64(time.Second))) }

// line returns precise fixes every step seconds from..to moving dlon e7 per
// fix along latitude -10°.
func pLine(from, to, step int, dlon int32) []domain.TrackFix {
	var out []domain.TrackFix
	lon := int32(-500000000)
	for s := from; s <= to; s += step {
		out = append(out, domain.TrackFix{At: pSec(float64(s)), Lat: -100000000, Lon: lon, Accuracy: 8})
		lon += dlon
	}
	return out
}

func pRecording(from, to float64, precision domain.Permission) domain.Span {
	return domain.Span{From: pSec(from), To: pSec(to), Recording: true, Precision: precision}
}

func pStopped(from, to float64, reason domain.GapReason, by domain.Detector) domain.Span {
	return domain.Span{From: pSec(from), To: pSec(to), Reason: reason, Detector: by}
}

// track builds a whole-trip input: the trip runs [0, end), the session opens
// at 0 and ends at end after one precise recording span.
func pTrack(end float64, fixes []domain.TrackFix) domain.TrackInput {
	return domain.TrackInput{Running: []domain.Interval{{From: pSec(0), To: pSec(end)}}, Opened: pSec(0), Ended: pSec(end),
		States: []domain.Span{pRecording(0, end, domain.PermissionPrecise)}, EndReason: domain.GapReason(domain.CauseTripCompleted), EndDetector: domain.DetectedByServer, Fixes: fixes}
}

// haversine is an independent oracle of the §10.4 sphere.
func pHaversine(a, b domain.TrackFix) float64 {
	rad := func(e7 int32) float64 { return float64(e7) / 1e7 * math.Pi / 180 }
	dp, dl := rad(b.Lat)-rad(a.Lat), rad(b.Lon)-rad(a.Lon)
	h := math.Sin(dp/2)*math.Sin(dp/2) + math.Cos(rad(a.Lat))*math.Cos(rad(b.Lat))*math.Sin(dl/2)*math.Sin(dl/2)
	return 2 * 6371008.8 * math.Asin(math.Sqrt(h))
}

func pProcess(t *testing.T, in domain.TrackInput) domain.TrackResult {
	t.Helper()
	r, err := domain.ProcessTrack(in)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func pCoverage(t *testing.T, r domain.TrackResult) int64 {
	t.Helper()
	c, ok := r.Classification.Coverage()
	if !ok {
		t.Fatal("coverage unknown")
	}
	return c
}

// §10.3 worked number: 12 670 × 10⁻⁷ degrees of longitude at 10° south.
// The specification's 138.9 m uses 109 629 m per degree (an ellipsoid); on
// the §10.4 sphere of 6 371 008.8 m it is 138.74 m (B-BR-EMP-464).
func TestMetresIsTheSectionSphere(t *testing.T) {
	a, b := domain.Point{Lat: -100000000, Lon: -500000000}, domain.Point{Lat: -100000000, Lon: -500012670}
	if m := domain.Metres(a, b); math.Abs(m-138.744) > 0.001 || domain.Metres(a, a) != 0 || domain.Metres(a, b) != domain.Metres(b, a) {
		t.Fatal("great-circle length", m)
	}
}

// BR-GPS-026 §10.4 worked example: 20 400 s in progress, a tunnel of 240 s
// and a phone restart of 780 s give 950 permille and PRECISE (B-BR-EMP-463).
func TestWorkedCoverageExampleYields950(t *testing.T) {
	var fixes []domain.TrackFix
	for _, f := range pLine(0, 20390, 10, 100) {
		s := f.At.Sub(pt0).Seconds()
		if s > 1000 && s < 1240 || s >= 5000 && s < 5780 {
			continue
		}
		fixes = append(fixes, f)
	}
	in := pTrack(20400, fixes)
	in.States = []domain.Span{pRecording(0, 5000, domain.PermissionPrecise), pStopped(5000, 5780, domain.GapReason(domain.CauseDeviceRestarted), domain.DetectedByDevice), pRecording(5780, 20400, domain.PermissionPrecise)}
	r := pProcess(t, in)
	if r.ExpectedSeconds != 20400 || r.GapSeconds != 1020 || pCoverage(t, r) != 950 || r.Classification.Quality() != domain.QualityPrecise {
		t.Fatal("worked example", r.ExpectedSeconds, r.GapSeconds, pCoverage(t, r))
	}
	want := []domain.Gap{{From: pSec(1000), To: pSec(1240), Reason: domain.GapSignalLost, Detector: domain.DetectedByServer},
		{From: pSec(5000), To: pSec(5780), Reason: domain.GapReason(domain.CauseDeviceRestarted), Detector: domain.DetectedByDevice}}
	if fmt.Sprint(pGapList(r.Gaps)) != fmt.Sprint(pGapList(want)) {
		t.Fatal("worked gaps", pGapList(r.Gaps))
	}
}

func pGapList(gaps []domain.Gap) []string {
	out := []string{}
	for _, g := range gaps {
		out = append(out, fmt.Sprintf("%v-%v:%s/%s", g.From.Sub(pt0).Seconds(), g.To.Sub(pt0).Seconds(), g.Reason, g.Detector))
	}
	return out
}

// BR-GPS-017 (B-BR-EMP-462): an outlier is never accepted, measured or
// drawn; accuracy above 50 m is refused at 51 and kept at 50.
func TestOutliersAndInaccurateFixesAreRejectedAndCounted(t *testing.T) {
	fixes := pLine(0, 100, 10, -12670)
	outlier := domain.TrackFix{At: pSec(55), Lat: -100000000 + 500000, Lon: fixes[5].Lon, Accuracy: 5}
	fixes = append(fixes[:6], append([]domain.TrackFix{outlier}, fixes[6:]...)...)
	fixes[2].Accuracy = 51
	fixes[3].Accuracy = 50
	r := pProcess(t, pTrack(110, fixes))
	if r.RejectedSpeed != 1 || r.RejectedAccuracy != 1 || r.RejectedOutside != 0 || r.Accepted != len(fixes)-2 {
		t.Fatal("filter counters", r.RejectedSpeed, r.RejectedAccuracy, r.Accepted)
	}
	clean := append(append([]domain.TrackFix{}, fixes[:2]...), fixes[3:6]...)
	clean = append(clean, fixes[7:]...)
	want := 0.0
	for i := 1; i < len(clean); i++ {
		want += pHaversine(clean[i-1], clean[i])
	}
	if r.DistanceMetres == nil || *r.DistanceMetres != int64(math.Floor(want)) {
		t.Fatal("an outlier entered the distance", *r.DistanceMetres, want)
	}
	for _, l := range r.Lines {
		for _, p := range l {
			if p.Lat != -100000000 {
				t.Fatal("an outlier entered the geometry", p)
			}
		}
	}
	// Exactly 70 m/s is accepted; above it the implied speed is refused.
	fast := []domain.TrackFix{{At: pSec(0), Lat: 0, Lon: 0, Accuracy: 5}, {At: pSec(10), Lat: 0, Lon: 62950, Accuracy: 5}, {At: pSec(20), Lat: 0, Lon: 62950 + 64000, Accuracy: 5}}
	if m := pHaversine(fast[0], fast[1]); m > 700 || m < 699 {
		t.Fatal("speed fixture", m)
	}
	if r := pProcess(t, pTrack(30, fast)); r.RejectedSpeed != 1 || r.Accepted != 2 {
		t.Fatal("speed bound", r.RejectedSpeed, r.Accepted)
	}
}

// BR-GPS-024/025 (B-BR-EMP-463/464): more than 180 s without an accepted
// fix inside a recording period is a SIGNAL_LOST gap that splits the run;
// exactly 180 s is not; nothing is measured or drawn across a gap.
func TestGapsAreNeverBridged(t *testing.T) {
	first := pLine(0, 100, 10, -12670)
	second := pLine(281, 400, 10, -12670)
	shift := second[0].Lon - first[len(first)-1].Lon
	for i := range second {
		second[i].Lon -= shift - 200000 // the vehicle moved about 2.2 km during the gap
	}
	r := pProcess(t, pTrack(410, append(append([]domain.TrackFix{}, first...), second...)))
	if len(r.Gaps) != 1 || r.Gaps[0].From != first[len(first)-1].At || r.Gaps[0].To != second[0].At || r.Gaps[0].Reason != domain.GapSignalLost || r.Gaps[0].Detector != domain.DetectedByServer {
		t.Fatal("signal lost gap", pGapList(r.Gaps))
	}
	want := 0.0
	for _, run := range [][]domain.TrackFix{first, second} {
		for i := 1; i < len(run); i++ {
			want += pHaversine(run[i-1], run[i])
		}
	}
	if r.DistanceMetres == nil || *r.DistanceMetres != int64(math.Floor(want)) || len(r.Lines) != 2 {
		t.Fatal("distance across the gap", *r.DistanceMetres, want, len(r.Lines))
	}
	bridged := want + pHaversine(first[len(first)-1], second[0])
	if *r.DistanceMetres >= int64(bridged) {
		t.Fatal("the gap was bridged")
	}
	// Exactly 180 s is one run and no gap.
	edge := append(pLine(0, 100, 10, -12670), pLine(280, 300, 10, -12670)...)
	for i := 11; i < len(edge); i++ {
		edge[i].Lon = edge[10].Lon + int32(i-10)*-12670
	}
	if r := pProcess(t, pTrack(300, edge)); len(r.Gaps) != 0 || len(r.Lines) != 1 {
		t.Fatal("180 s is not a gap", pGapList(r.Gaps), len(r.Lines))
	}
	// From the period start and until its end.
	late := pProcess(t, pTrack(500, pLine(200, 300, 10, -12670)))
	if fmt.Sprint(pGapList(late.Gaps)) != "[0-200:SIGNAL_LOST/SERVER 300-500:SIGNAL_LOST/SERVER]" {
		t.Fatal("leading and trailing gaps", pGapList(late.Gaps))
	}
}

// BR-GPS-025: a step counts only once it reaches 10 m from the last counted
// fix, so jitter adds nothing and a crawl still adds up; floored metres.
func TestStepsShorterThanTenMetresAreNotCounted(t *testing.T) {
	// About 5.5 m per fix along the meridian: every second fix is counted.
	var crawl []domain.TrackFix
	for i := 0; i <= 20; i++ {
		crawl = append(crawl, domain.TrackFix{At: pSec(float64(i * 10)), Lat: int32(-100000000 + i*500), Lon: -500000000, Accuracy: 5})
	}
	r := pProcess(t, pTrack(210, crawl))
	want := 10 * pHaversine(crawl[0], crawl[2])
	if r.DistanceMetres == nil || math.Abs(float64(*r.DistanceMetres)-want) > 1 || *r.DistanceMetres != int64(math.Floor(want)) {
		t.Fatal("crawl distance", *r.DistanceMetres, want)
	}
	// Jitter of 3 m around one point adds nothing.
	var jitter []domain.TrackFix
	for i := 0; i <= 20; i++ {
		jitter = append(jitter, domain.TrackFix{At: pSec(float64(i * 10)), Lat: int32(-100000000 + (i%2)*270), Lon: -500000000, Accuracy: 5})
	}
	if r := pProcess(t, pTrack(210, jitter)); r.DistanceMetres == nil || *r.DistanceMetres != 0 {
		t.Fatal("jitter counted", r.DistanceMetres)
	}
}

// BR-GPS-007/026 (B-BR-EMP-463/464): coarse periods cap the quality at
// APPROXIMATE and add no distance; a coarse-only track has no distance.
func TestCoarsePeriodsAreApproximateAndUnmeasured(t *testing.T) {
	fixes := pLine(0, 290, 10, -12670)
	for i := range fixes {
		if fixes[i].At.Sub(pt0) >= 150*time.Second {
			fixes[i].Coarse, fixes[i].Accuracy = true, 2000
		}
	}
	in := pTrack(300, fixes)
	in.States = []domain.Span{pRecording(0, 150, domain.PermissionPrecise), pRecording(150, 300, domain.PermissionApproximate)}
	r := pProcess(t, in)
	if r.Classification.Quality() != domain.QualityApproximate || !r.CoarsePeriod || r.CoarseAccepted != 15 || r.RejectedAccuracy != 0 {
		t.Fatal("coarse classification", r.Classification.Quality(), r.CoarseAccepted, r.RejectedAccuracy)
	}
	want := 0.0
	for i := 1; i < 15; i++ {
		want += pHaversine(fixes[i-1], fixes[i])
	}
	if r.DistanceMetres == nil || *r.DistanceMetres != int64(math.Floor(want)) || len(r.Lines) != 1 {
		t.Fatal("distance from coarse fixes", *r.DistanceMetres, want)
	}
	in.States = []domain.Span{pRecording(0, 300, domain.PermissionApproximate)}
	for i := range in.Fixes {
		in.Fixes[i].Coarse = true
	}
	if r := pProcess(t, in); r.DistanceMetres != nil || len(r.Lines) != 0 || r.ToleranceMetres != 0 || r.Classification.Quality() != domain.QualityApproximate {
		t.Fatal("coarse-only track measured", r.DistanceMetres, len(r.Lines))
	}
}

// B-BR-EMP-463: not-recording time is a gap with the session cause and its
// detector; paused time is not expected; before the session opened the
// server records NOT_STARTED; after an early end the end cause.
func TestNotRecordingTimeIsAGapWithItsReason(t *testing.T) {
	fixes := pLine(100, 900, 10, -12670)
	var kept []domain.TrackFix
	for _, f := range fixes {
		s := f.At.Sub(pt0).Seconds()
		if s >= 300 && s < 500 || s >= 600 && s < 700 {
			continue
		}
		kept = append(kept, f)
	}
	in := domain.TrackInput{
		Running: []domain.Interval{{From: pSec(0), To: pSec(300)}, {From: pSec(500), To: pSec(1000)}},
		Opened:  pSec(100), Ended: pSec(900),
		States: []domain.Span{pRecording(100, 300, domain.PermissionPrecise), pStopped(300, 500, domain.GapReason(domain.CauseTripPaused), domain.DetectedByServer),
			pRecording(500, 600, domain.PermissionPrecise), pStopped(600, 700, domain.GapReason(domain.CauseStoppedByDriver), domain.DetectedByDevice), pRecording(700, 900, domain.PermissionPrecise)},
		EndReason: domain.GapReason(domain.CauseMembershipEnded), EndDetector: domain.DetectedByServer, Fixes: kept,
	}
	r := pProcess(t, in)
	if fmt.Sprint(pGapList(r.Gaps)) != "[0-100:NOT_STARTED/SERVER 600-700:STOPPED_BY_DRIVER/DEVICE 900-1000:MEMBERSHIP_ENDED/SERVER]" {
		t.Fatal("not-recording gaps", pGapList(r.Gaps))
	}
	if r.ExpectedSeconds != 800 || r.GapSeconds != 300 || pCoverage(t, r) != 625 || r.Classification.Quality() != domain.QualityApproximate {
		t.Fatal("paused time counted", r.ExpectedSeconds, r.GapSeconds)
	}
	// A fix in a stopped period or outside the running time is OUTSIDE.
	in.Fixes = append(append([]domain.TrackFix{}, kept[:5]...), domain.TrackFix{At: pSec(150.5), Lat: -100000000, Lon: kept[4].Lon, Accuracy: 5})
	in.Fixes = append(in.Fixes, domain.TrackFix{At: pSec(650), Lat: -100000000, Lon: kept[4].Lon, Accuracy: 5})
	if r := pProcess(t, in); r.RejectedOutside != 1 || r.Accepted != 6 {
		t.Fatal("outside fix", r.RejectedOutside, r.Accepted)
	}
	// A coarse flag disagreeing with its period is OUTSIDE too.
	in.Fixes = []domain.TrackFix{{At: pSec(120), Lat: -100000000, Lon: -500000000, Coarse: true}}
	if r := pProcess(t, in); r.RejectedOutside != 1 || r.Accepted != 0 || r.Classification.Quality() != domain.QualityInsufficient || r.DistanceMetres != nil || r.Lines != nil {
		t.Fatal("mismatched precision", r.RejectedOutside, r.Accepted)
	}
}

// §6.2: fewer than two accepted fixes or no expected time is INSUFFICIENT
// and derives nothing.
func TestInsufficientTracksDeriveNothing(t *testing.T) {
	for name, in := range map[string]domain.TrackInput{
		"one fix":      pTrack(100, pLine(0, 0, 10, 0)),
		"no fix":       pTrack(100, nil),
		"low coverage": pTrack(1000, pLine(0, 300, 10, -12670)),
		"no time":      {Opened: pSec(0), Ended: pSec(0), EndReason: domain.GapReason(domain.CauseTripCancelled), EndDetector: domain.DetectedByDevice},
	} {
		r := pProcess(t, in)
		if r.Classification.Quality() != domain.QualityInsufficient || r.DistanceMetres != nil || r.Lines != nil || r.ToleranceMetres != 0 {
			t.Fatal("an insufficient track derived values", name)
		}
	}
}

// B-BR-EMP-465: Douglas–Peucker at 20 m keeps the end points and every turn
// farther than the tolerance, and the tolerance doubles until at most 1 000
// vertices remain.
func TestSimplifiedGeometryHasItsToleranceAndBound(t *testing.T) {
	straight := make([]domain.Point, 100)
	for i := range straight {
		straight[i] = domain.Point{Lat: -100000000, Lon: int32(-500000000 + i*1000)}
	}
	lines, tolerance := domain.Simplify([][]domain.Point{straight})
	if tolerance != 20 || len(lines) != 1 || len(lines[0]) != 2 || lines[0][0] != straight[0] || lines[0][1] != straight[99] {
		t.Fatal("straight line", tolerance, lines)
	}
	// A 30 m bump survives, a 10 m bump does not.
	bump := func(height int32) []domain.Point {
		out := []domain.Point{}
		for i := 0; i < 11; i++ {
			lat := int32(-100000000)
			if i == 5 {
				lat += height
			}
			out = append(out, domain.Point{Lat: lat, Lon: int32(-500000000 + i*100)})
		}
		return out
	}
	if l, _ := domain.Simplify([][]domain.Point{bump(2700)}); len(l[0]) != 3 {
		t.Fatal("a 30 m turn was dropped", l)
	}
	if l, _ := domain.Simplify([][]domain.Point{bump(900)}); len(l[0]) != 2 {
		t.Fatal("a 10 m wiggle was kept", l)
	}
	// 3 000 zig-zag vertices of ±60 m: the tolerance doubles until the
	// geometry fits 1 000 vertices.
	zig := make([]domain.Point, 3000)
	for i := range zig {
		zig[i] = domain.Point{Lat: int32(-100000000 + (i%2)*10800), Lon: int32(-500000000 + i*2000)}
	}
	lines, tolerance = domain.Simplify([][]domain.Point{zig})
	if total := len(lines[0]); total > domain.MaxGeometryPoints || tolerance <= 20 || tolerance%20 != 0 {
		t.Fatal("geometry bound", total, tolerance)
	}
	// Single fixes draw nothing; more runs than the bound allows draw nothing.
	if l, tol := domain.Simplify([][]domain.Point{{straight[0]}}); l != nil || tol != 0 {
		t.Fatal("a single fix drew a line")
	}
	many := make([][]domain.Point, 501)
	for i := range many {
		many[i] = straight[:2]
	}
	if l, tol := domain.Simplify(many); l != nil || tol != 0 {
		t.Fatal("501 runs drew a line")
	}
}

// ProcessTrack refuses an inconsistent input instead of guessing.
func TestProcessTrackRefusesInconsistentInput(t *testing.T) {
	good := pTrack(100, pLine(0, 90, 10, -12670))
	cases := map[string]func(*domain.TrackInput){
		"unsorted fixes":   func(in *domain.TrackInput) { in.Fixes[1], in.Fixes[2] = in.Fixes[2], in.Fixes[1] },
		"repeated instant": func(in *domain.TrackInput) { in.Fixes[2].At = in.Fixes[1].At },
		"latitude":         func(in *domain.TrackInput) { in.Fixes[0].Lat = 900000001 },
		"overlapping trip": func(in *domain.TrackInput) {
			in.Running = append(in.Running, domain.Interval{From: pSec(50), To: pSec(200)})
		},
		"empty interval":    func(in *domain.TrackInput) { in.Running[0].To = in.Running[0].From },
		"hole in states":    func(in *domain.TrackInput) { in.States[0].From = pSec(1) },
		"states past end":   func(in *domain.TrackInput) { in.Ended = pSec(90) },
		"recording reason":  func(in *domain.TrackInput) { in.States[0].Reason = domain.GapSignalLost },
		"unknown precision": func(in *domain.TrackInput) { in.States[0].Precision = domain.PermissionNone },
		"signal lost state": func(in *domain.TrackInput) {
			in.States[0] = pStopped(0, 100, domain.GapSignalLost, domain.DetectedByServer)
		},
		"precision reduced": func(in *domain.TrackInput) { in.EndReason = domain.GapReason(domain.CausePrecisionReduced) },
		"unknown detector":  func(in *domain.TrackInput) { in.EndDetector = "PHONE" },
		"negative accuracy": func(in *domain.TrackInput) { in.Fixes[0].Accuracy = -1 },
		"no opening":        func(in *domain.TrackInput) { in.Opened = time.Time{} },
	}
	for name, edit := range cases {
		in := good
		in.Fixes = append([]domain.TrackFix(nil), good.Fixes...)
		in.States = append([]domain.Span(nil), good.States...)
		in.Running = append([]domain.Interval(nil), good.Running...)
		edit(&in)
		if _, err := domain.ProcessTrack(in); !errors.Is(err, domain.ErrInvalid) {
			t.Fatal("an inconsistent input was processed", name, err)
		}
	}
	for _, raw := range []string{"SIGNAL_LOST", "NOT_STARTED", "DEVICE_REVOKED"} {
		if _, err := domain.ParseGapReason(raw); err != nil {
			t.Fatal("gap reason", raw)
		}
	}
	for _, raw := range []string{"PRECISION_REDUCED", "signal_lost", "PROVIDER_DISABLED", ""} {
		if _, err := domain.ParseGapReason(raw); err == nil {
			t.Fatal("unknown gap reason admitted", raw)
		}
	}
}

// No coordinate prints through fmt, JSON or slog (BR-GPS-039).
func TestProcessingValuesAreRedacted(t *testing.T) {
	in := pTrack(100, pLine(0, 90, 10, -12670))
	r := pProcess(t, in)
	for _, v := range []any{in, r, in.Fixes[0]} {
		var buf bytes.Buffer
		slog.New(slog.NewJSONHandler(&buf, nil)).Info("x", "v", v)
		raw, _ := json.Marshal(v)
		for _, s := range []string{fmt.Sprint(v), fmt.Sprintf("%+v", v), string(raw), buf.String()} {
			if strings.Contains(s, "500000000") || strings.Contains(s, "100000000") {
				t.Fatal("a coordinate printed", s)
			}
		}
	}
}

// VAL-28 server side: a permission revoked at 10:20:00 and resumed at
// 10:35:00 is a 900-second gap, a process killed at 11:00:00 and resumed at
// 11:06:00 a 360-second gap, with no invented fix; fixes every 10 s.
func TestVal28InterruptionsBecomeGaps(t *testing.T) {
	at := func(h, m int) float64 { return float64((h-10)*3600 + m*60) }
	var fixes []domain.TrackFix
	for _, f := range pLine(0, 7190, 10, -1000) {
		s := f.At.Sub(pt0).Seconds()
		if s >= at(10, 20) && s < at(10, 35) || s >= at(11, 0) && s < at(11, 6) {
			continue
		}
		fixes = append(fixes, f)
	}
	in := pTrack(7200, fixes)
	in.States = []domain.Span{pRecording(0, at(10, 20), domain.PermissionPrecise), pStopped(at(10, 20), at(10, 35), domain.GapReason(domain.CausePermissionRevoked), domain.DetectedByDevice),
		pRecording(at(10, 35), at(11, 0), domain.PermissionPrecise), pStopped(at(11, 0), at(11, 6), domain.GapReason(domain.CauseProcessStopped), domain.DetectedByDevice),
		pRecording(at(11, 6), 7200, domain.PermissionPrecise)}
	r := pProcess(t, in)
	if fmt.Sprint(pGapList(r.Gaps)) != "[1200-2100:PERMISSION_REVOKED/DEVICE 3600-3960:PROCESS_STOPPED/DEVICE]" || r.GapSeconds != 1260 || r.ExpectedSeconds != 7200 {
		t.Fatal("VAL-28 gaps", pGapList(r.Gaps), r.GapSeconds)
	}
	if r.RejectedOutside != 0 || r.Accepted != len(fixes) || pCoverage(t, r) != 825 || r.Classification.Quality() != domain.QualityApproximate {
		t.Fatal("VAL-28 counters", r.Accepted, pCoverage(t, r))
	}
}
