package domain

import (
	"math"
	"time"
)

// Track processing rules (EMP-GPS-005, B-BR-EMP-461..466, BR-GPS-017,
// BR-GPS-023..026, §10.2/§10.4, EDR-298..301). Pure functions of stored
// fixes, the session's state spans and the trip's running intervals; raw
// fixes are never edited, nothing is invented across a gap.
const (
	// MethodVersion names the whole processing method of a summary.
	MethodVersion = "gps-track-v1"
	// DistanceMethod names the GPS distance rule.
	DistanceMethod = "haversine-r6371008.8-step10-v1"
	// GapThreshold is the longest interval inside a recording period
	// without an accepted fix that is not a gap (BR-GPS-024).
	GapThreshold = 180 * time.Second
	// MaxPreciseAccuracyMetres bounds a precise fix (BR-GPS-017).
	MaxPreciseAccuracyMetres = 50
	// MaxSpeedMetresPerSecond bounds the implied speed from the previous
	// accepted precise fix (BR-GPS-017).
	MaxSpeedMetresPerSecond = 70
	// MinStepMetres is the shortest counted step (BR-GPS-025).
	MinStepMetres = 10.0
	// EarthRadiusMetres is the sphere of §10.4 (TrackRecordingPolicy).
	EarthRadiusMetres = 6_371_008.8
	// SimplifyToleranceMetres and MaxGeometryPoints are the §10.2 storage
	// defaults of the simplified geometry.
	SimplifyToleranceMetres = 20
	MaxGeometryPoints       = 1000
	// MaxToleranceMetres bounds the tolerance doubling.
	MaxToleranceMetres = 20 * 1024 * 1024
	// MaxProcessedFixes bounds one session's input.
	MaxProcessedFixes = 1_000_000
)

// Detector names who detected a gap.
type Detector string

const (
	DetectedByDevice Detector = "DEVICE"
	DetectedByServer Detector = "SERVER"
)

// GapReason is the closed gap vocabulary: SIGNAL_LOST (no accepted fix for
// more than GapThreshold inside a recording period) and every session cause
// that leaves the trip running without recording.
type GapReason string

const GapSignalLost GapReason = "SIGNAL_LOST"

// GapReasonOf maps the cause of a non-recording session state to its gap
// reason; PRECISION_REDUCED keeps recording and is never a gap.
func GapReasonOf(c Cause) (GapReason, bool) {
	switch c {
	case CauseNotStarted, CausePermissionDenied, CauseNotificationBlocked, CauseTripPaused, CauseLongStationary, CausePermissionRevoked, CauseProcessStopped,
		CauseDeviceRestarted, CauseStoppedByDriver, CauseOwnerChanged, CauseTripCompleted, CauseTripCancelled, CauseMembershipEnded, CauseDeviceRevoked:
		return GapReason(c), true
	}
	return "", false
}

// ParseGapReason accepts only an exact gap reason.
func ParseGapReason(raw string) (GapReason, error) {
	if raw == string(GapSignalLost) {
		return GapSignalLost, nil
	}
	if r, ok := GapReasonOf(Cause(raw)); ok {
		return r, nil
	}
	return "", ErrInvalid
}

// Interval is one half-open instant interval.
type Interval struct {
	From, To time.Time
}

// Span is one half-open interval of a session state. Recording spans carry
// their precision; the others carry the gap reason and its detector.
type Span struct {
	From, To  time.Time
	Recording bool
	Precision Permission
	Reason    GapReason
	Detector  Detector
}

// TrackFix is one stored fix as processed (positions in 10⁻⁷ degrees).
type TrackFix struct {
	redacted
	At       time.Time
	Lat, Lon int32
	Accuracy int32
	Coarse   bool
}

// TrackInput is everything one summary is derived from.
type TrackInput struct {
	redacted
	// Running are the trip's IN_PROGRESS intervals inside the processing
	// window, sorted and disjoint.
	Running []Interval
	// Opened and Ended bound the session; States cover [Opened, Ended)
	// contiguously. Running time before Opened is a NOT_STARTED gap detected
	// by the server; after Ended a gap with EndReason and EndDetector.
	Opened, Ended time.Time
	States        []Span
	EndReason     GapReason
	EndDetector   Detector
	// Fixes are the stored fixes of the session in instant order.
	Fixes []TrackFix
}

// Gap is one derived gap.
type Gap struct {
	From, To time.Time
	Reason   GapReason
	Detector Detector
}

// Point is one geometry vertex in 10⁻⁷ degrees; it prints redacted.
type Point struct {
	redacted
	Lat, Lon int32
}

// TrackResult is one derived summary.
type TrackResult struct {
	redacted
	ExpectedSeconds, GapSeconds                      int64
	Accepted, CoarseAccepted                         int
	RejectedAccuracy, RejectedSpeed, RejectedOutside int
	CoarsePeriod                                     bool
	Gaps                                             []Gap
	Classification                                   Classification
	DistanceMetres                                   *int64
	Lines                                            [][]Point
	ToleranceMetres                                  int
}

// Metres is the great-circle length between two fixes on the §10.4 sphere.
func Metres(a, b Point) float64 {
	const k = math.Pi / 180 / 1e7
	p1, p2 := float64(a.Lat)*k, float64(b.Lat)*k
	dp, dl := p2-p1, float64(b.Lon-a.Lon)*k
	h := math.Pow(math.Sin(dp/2), 2) + math.Cos(p1)*math.Cos(p2)*math.Pow(math.Sin(dl/2), 2)
	return 2 * EarthRadiusMetres * math.Asin(math.Sqrt(math.Min(1, h)))
}

func validPoint(p Point) bool {
	return p.Lat >= -900_000_000 && p.Lat <= 900_000_000 && p.Lon >= -1_800_000_000 && p.Lon <= 1_800_000_000
}

func (in TrackInput) validate() error {
	for i, r := range in.Running {
		if !r.From.Before(r.To) || i > 0 && r.From.Before(in.Running[i-1].To) {
			return ErrInvalid
		}
	}
	if in.Opened.IsZero() || in.Ended.Before(in.Opened) {
		return ErrInvalid
	}
	if _, err := ParseGapReason(string(in.EndReason)); err != nil || in.EndReason == GapSignalLost || in.EndDetector != DetectedByDevice && in.EndDetector != DetectedByServer {
		return ErrInvalid
	}
	next := in.Opened
	for _, s := range in.States {
		if !s.From.Before(s.To) || !s.From.Equal(next) {
			return ErrInvalid
		}
		next = s.To
		if s.Recording {
			if s.Precision != PermissionPrecise && s.Precision != PermissionApproximate || s.Reason != "" || s.Detector != "" {
				return ErrInvalid
			}
			continue
		}
		if _, err := ParseGapReason(string(s.Reason)); err != nil || s.Reason == GapSignalLost || s.Detector != DetectedByDevice && s.Detector != DetectedByServer || s.Precision != "" {
			return ErrInvalid
		}
	}
	if !next.Equal(in.Ended) || len(in.Fixes) > MaxProcessedFixes {
		return ErrInvalid
	}
	for i, f := range in.Fixes {
		if f.At.IsZero() || i > 0 && !f.At.After(in.Fixes[i-1].At) || !validPoint(Point{Lat: f.Lat, Lon: f.Lon}) || f.Accuracy < 0 {
			return ErrInvalid
		}
	}
	return nil
}

// clip intersects two half-open intervals.
func clip(a, b Interval) (Interval, bool) {
	from, to := a.From, a.To
	if b.From.After(from) {
		from = b.From
	}
	if b.To.Before(to) {
		to = b.To
	}
	return Interval{from, to}, from.Before(to)
}

// piece is one part of the window with its session state.
type piece struct {
	Interval
	span Span
}

// pieces splits the running window by the session states: before the
// session opened NOT_STARTED (server), after it ended its end reason.
func pieces(in TrackInput) []piece {
	var out []piece
	for _, r := range in.Running {
		if v, ok := clip(r, Interval{r.From, in.Opened}); ok {
			out = append(out, piece{v, Span{Reason: GapReason(CauseNotStarted), Detector: DetectedByServer}})
		}
		for _, s := range in.States {
			if v, ok := clip(r, Interval{s.From, s.To}); ok {
				out = append(out, piece{v, s})
			}
		}
		if v, ok := clip(r, Interval{in.Ended, r.To}); ok {
			out = append(out, piece{v, Span{Reason: in.EndReason, Detector: in.EndDetector}})
		}
	}
	return out
}

func within(p piece, at time.Time) bool { return !at.Before(p.From) && at.Before(p.To) }

// ProcessTrack derives one summary (B-BR-EMP-461..465): per-fix filtering,
// gaps with reasons, coverage and quality, GPS distance and the simplified
// geometry. It never edits a fix, never bridges a gap and returns ErrInvalid
// for an inconsistent input.
func ProcessTrack(in TrackInput) (TrackResult, error) {
	if err := in.validate(); err != nil {
		return TrackResult{}, err
	}
	parts := pieces(in)
	var out TrackResult
	var expected, gapped time.Duration
	var gaps []Gap
	add := func(g Gap) {
		if n := len(gaps); n > 0 && gaps[n-1].To.Equal(g.From) && gaps[n-1].Reason == g.Reason && gaps[n-1].Detector == g.Detector {
			gaps[n-1].To = g.To
			return
		}
		gaps = append(gaps, g)
	}
	for _, p := range parts {
		expected += p.To.Sub(p.From)
		if p.span.Recording && p.span.Precision == PermissionApproximate {
			out.CoarsePeriod = true
		}
	}
	// Per-fix filter (BR-GPS-017) in instant order; the previous accepted
	// precise fix anchors the implied speed.
	type accepted struct {
		fix   TrackFix
		piece int
	}
	var kept []accepted
	var previous *TrackFix
	cursor := 0
	for _, f := range in.Fixes {
		for cursor < len(parts) && !f.At.Before(parts[cursor].To) {
			cursor++
		}
		if cursor == len(parts) || !within(parts[cursor], f.At) || !parts[cursor].span.Recording || f.Coarse != (parts[cursor].span.Precision == PermissionApproximate) {
			out.RejectedOutside++
			continue
		}
		if !f.Coarse {
			if f.Accuracy > MaxPreciseAccuracyMetres {
				out.RejectedAccuracy++
				continue
			}
			if previous != nil {
				metres := Metres(Point{Lat: previous.Lat, Lon: previous.Lon}, Point{Lat: f.Lat, Lon: f.Lon})
				if metres*1000 > MaxSpeedMetresPerSecond*float64(f.At.Sub(previous.At).Milliseconds()) {
					out.RejectedSpeed++
					continue
				}
			}
			fix := f
			previous = &fix
		} else {
			out.CoarseAccepted++
		}
		out.Accepted++
		kept = append(kept, accepted{f, cursor})
	}
	// Gaps (BR-GPS-024): every not-recording piece, and every interval of a
	// recording piece longer than GapThreshold without an accepted fix.
	k := 0
	var runs [][]Point
	for i, p := range parts {
		if !p.span.Recording {
			add(Gap{p.From, p.To, p.span.Reason, p.span.Detector})
			continue
		}
		last := p.From
		var run []Point
		flush := func() {
			if len(run) > 0 && p.span.Precision == PermissionPrecise {
				runs = append(runs, run)
			}
			run = nil
		}
		for ; k < len(kept) && kept[k].piece == i; k++ {
			f := kept[k].fix
			if f.At.Sub(last) > GapThreshold {
				add(Gap{last, f.At, GapSignalLost, DetectedByServer})
				flush()
			}
			run = append(run, Point{Lat: f.Lat, Lon: f.Lon})
			last = f.At
		}
		if p.To.Sub(last) > GapThreshold {
			add(Gap{last, p.To, GapSignalLost, DetectedByServer})
		}
		flush()
	}
	for _, g := range gaps {
		gapped += g.To.Sub(g.From)
	}
	out.Gaps = gaps
	out.ExpectedSeconds = int64(expected / time.Second)
	out.GapSeconds = int64((gapped + time.Second - 1) / time.Second)
	if out.GapSeconds > out.ExpectedSeconds {
		out.GapSeconds = out.ExpectedSeconds
	}
	c, err := Classify(TrackEvidence{ExpectedSeconds: out.ExpectedSeconds, GapSeconds: out.GapSeconds, AcceptedFixes: int64(out.Accepted), Coarse: out.CoarsePeriod})
	if err != nil {
		return TrackResult{}, err
	}
	out.Classification = c
	if c.Quality() == QualityInsufficient {
		return out, nil
	}
	// GPS distance (BR-GPS-025): consecutive accepted precise fixes of one
	// continuous run, a step counted only once it reaches MinStepMetres from
	// the last counted fix, never across a gap; floored to whole metres.
	total, measured := 0.0, false
	for _, run := range runs {
		if len(run) < 2 {
			continue
		}
		measured = true
		anchor := run[0]
		for _, p := range run[1:] {
			if step := Metres(anchor, p); step >= MinStepMetres {
				total += step
				anchor = p
			}
		}
	}
	if measured {
		metres := int64(math.Floor(total))
		out.DistanceMetres = &metres
	}
	out.Lines, out.ToleranceMetres = Simplify(runs)
	return out, nil
}

// Simplify reduces each precise run of at least two fixes with
// Douglas–Peucker at SimplifyToleranceMetres, doubling the tolerance until
// the whole geometry holds at most MaxGeometryPoints; it returns no line and
// tolerance 0 when even the run end points exceed the bound or no run has
// two fixes (EDR-300).
func Simplify(runs [][]Point) ([][]Point, int) {
	var lines [][]Point
	for _, r := range runs {
		if len(r) >= 2 {
			lines = append(lines, r)
		}
	}
	if len(lines) == 0 || 2*len(lines) > MaxGeometryPoints {
		return nil, 0
	}
	for tolerance := SimplifyToleranceMetres; tolerance <= MaxToleranceMetres; tolerance *= 2 {
		out, total := make([][]Point, 0, len(lines)), 0
		for _, line := range lines {
			s := douglasPeucker(line, float64(tolerance))
			out = append(out, s)
			total += len(s)
		}
		if total <= MaxGeometryPoints {
			return out, tolerance
		}
	}
	return nil, 0
}

// douglasPeucker keeps the end points and every vertex farther than the
// tolerance from the chord of its interval, on a local equirectangular
// projection in metres around the line's first point.
func douglasPeucker(line []Point, tolerance float64) []Point {
	n := len(line)
	if n <= 2 {
		return append([]Point(nil), line...)
	}
	const k = math.Pi / 180 / 1e7
	cos := math.Cos(float64(line[0].Lat) * k)
	xy := make([][2]float64, n)
	for i, p := range line {
		xy[i] = [2]float64{float64(p.Lon-line[0].Lon) * k * cos * EarthRadiusMetres, float64(p.Lat-line[0].Lat) * k * EarthRadiusMetres}
	}
	keep := make([]bool, n)
	keep[0], keep[n-1] = true, true
	stack := [][2]int{{0, n - 1}}
	for len(stack) > 0 {
		s := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		farthest, index := -1.0, -1
		for i := s[0] + 1; i < s[1]; i++ {
			if d := segmentDistance(xy[i], xy[s[0]], xy[s[1]]); d > farthest {
				farthest, index = d, i
			}
		}
		if index >= 0 && farthest > tolerance {
			keep[index] = true
			stack = append(stack, [2]int{s[0], index}, [2]int{index, s[1]})
		}
	}
	out := make([]Point, 0, n)
	for i, p := range line {
		if keep[i] {
			out = append(out, p)
		}
	}
	return out
}

func segmentDistance(p, a, b [2]float64) float64 {
	dx, dy := b[0]-a[0], b[1]-a[1]
	length := dx*dx + dy*dy
	t := 0.0
	if length > 0 {
		t = ((p[0]-a[0])*dx + (p[1]-a[1])*dy) / length
		t = math.Max(0, math.Min(1, t))
	}
	return math.Hypot(p[0]-a[0]-t*dx, p[1]-a[1]-t*dy)
}
