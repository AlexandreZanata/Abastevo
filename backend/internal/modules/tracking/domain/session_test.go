package domain_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"go/parser"
	"go/token"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	track "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/tracking/domain"
	trip "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/domain"
)

func must[T any](v T, e error) T {
	if e != nil {
		panic(e)
	}
	return v
}

func guard(t *testing.T) {
	t.Helper()
	if r := recover(); r != nil {
		t.Fatal(r)
	}
}

func scopeOf(t *testing.T, n int) kernel.OrgScope {
	t.Helper()
	return must(kernel.NewOrgScope(must(kernel.ParseOrgID(uuid(n)))))
}

type world struct {
	scope  kernel.OrgScope
	facts  track.TripFacts
	driver trip.MembershipRef
}

func newWorld(t *testing.T) world {
	t.Helper()
	s := scopeOf(t, 1)
	driver := must(trip.NewMembershipRef(s, uuid(31)))
	return world{scope: s, driver: driver,
		facts: track.TripFacts{Ref: must(trip.NewTripRef(s, uuid(21))), Driver: driver, State: trip.InProgress}}
}

func at(m int) time.Time { return baseInstant.Add(time.Duration(m) * time.Minute) }

func (w world) open(t *testing.T) track.Session {
	t.Helper()
	return must(track.Open(w.scope, track.OpenInput{Session: must(track.NewSessionRef(w.scope, uuid(11))), Trip: w.facts,
		Author: w.driver, Origin: track.OriginTripScreen, Policy: track.PolicyExpected, At: at(0)}))
}

func (w world) action(p track.Permission, m int) track.DriverAction {
	return track.DriverAction{Trip: w.facts, Author: w.driver, Origin: track.OriginTripScreen,
		Device: track.Device{Permission: p, NotificationVisible: true}, At: at(m)}
}

func (w world) event(state trip.State, c track.Cause, m int) track.Event {
	f := w.facts
	f.State = state
	return track.Event{Trip: f, Cause: c, Author: w.driver, At: at(m)}
}

func (w world) withState(state trip.State) track.TripFacts {
	f := w.facts
	f.State = state
	return f
}

// BUC-EMP-214: start, pause with the trip, resume on the trip screen, finish.
func TestTrackedTripJourney(t *testing.T) {
	defer guard(t)
	w := newWorld(t)
	s := w.open(t)
	if s.State() != track.NotStarted || s.Cause() != track.CauseNotStarted || s.Policy() != track.PolicyExpected ||
		s.Trip() != w.facts.Ref || s.Owner() != w.driver || s.Reference().ID() != uuid(11) {
		t.Fatal("opening did not bind the trip and its driver")
	}
	if _, ok := must2(s.Collecting(w.scope, w.facts)); ok {
		t.Fatal("a session that never started collects")
	}
	s = must(s.Start(w.scope, w.action(track.PermissionPrecise, 10)))
	if p, ok := must2(s.Collecting(w.scope, w.facts)); !ok || p != track.PermissionPrecise {
		t.Fatal("a recording session does not collect precise fixes")
	}
	paused := must(s.Follow(w.scope, w.withState(trip.Paused), at(20)))
	if paused.State() != track.Suspended || paused.Cause() != track.CauseTripPaused {
		t.Fatal("pausing the trip did not suspend the session")
	}
	same := must(paused.Follow(w.scope, w.facts, at(30)))
	if same.State() != track.Suspended || len(same.History()) != len(paused.History()) {
		t.Fatal("resuming the trip resumed recording by itself")
	}
	resumed := must(paused.Resume(w.scope, w.action(track.PermissionApproximate, 30)))
	done := must(resumed.Follow(w.scope, w.withState(trip.Completed), at(90)))
	if done.State() != track.Ended || done.Cause() != track.CauseTripCompleted || !done.Coarse() || done.Precision() != "" {
		t.Fatal("completing the trip did not end the session")
	}
	periods := done.RecordingPeriods()
	if len(periods) != 2 || periods[0].Precision() != track.PermissionPrecise || periods[1].Precision() != track.PermissionApproximate {
		t.Fatal("recording periods lost their precision", len(periods))
	}
	if end, ok := periods[1].End(); !ok || !end.Equal(at(90)) || !periods[0].Start().Equal(at(10)) {
		t.Fatal("recording period bounds changed")
	}
	h := done.History()
	if len(h) != 5 || h[4].Actor() != track.ActorTrip || h[3].Permission() != track.PermissionApproximate || !h[4].At().Equal(at(90)) {
		t.Fatal("history does not record every transition")
	}
	h[0] = track.Transition{}
	if done.History()[0].State() != track.NotStarted {
		t.Fatal("history accessor exposed the session's own storage")
	}
	q := must(track.Classify(track.TrackEvidence{ExpectedSeconds: 20400, GapSeconds: 1020, AcceptedFixes: 2, Coarse: done.Coarse()}))
	if permille, ok := q.Coverage(); q.Quality() != track.QualityApproximate || !ok || permille != 950 {
		t.Fatal("a coarse period did not cap the quality")
	}
}

func must2[A, B any](a A, b B, e error) (A, B) {
	if e != nil {
		panic(e)
	}
	return a, b
}

// Go-only zero values and tokens that the fixture can never construct.
func TestZeroValuesAndUnparsedTokensAreRefused(t *testing.T) {
	defer guard(t)
	w := newWorld(t)
	s := w.open(t)
	var zero track.Session
	if zero.State() != "" || zero.Cause() != "" || zero.Precision() != "" || zero.Coarse() || zero.History() != nil || zero.RecordingPeriods() != nil {
		t.Fatal("an unconstructed session reports a state")
	}
	checks := []struct {
		name string
		err  error
		want error
	}{
		{"zero session start", second(zero.Start(w.scope, w.action(track.PermissionPrecise, 10))), track.ErrInvalid},
		{"zero session event", second(zero.End(w.scope, w.event(trip.Completed, track.CauseTripCompleted, 10))), track.ErrInvalid},
		{"zero session follow", second(zero.Follow(w.scope, w.facts, at(10))), track.ErrInvalid},
		{"zero session collect", third(zero.Collecting(w.scope, w.facts)), track.ErrInvalid},
		{"zero scope before zero session", second(zero.Start(kernel.OrgScope{}, w.action(track.PermissionPrecise, 10))), track.ErrScope},
		{"zero scope reference", second(track.NewSessionRef(kernel.OrgScope{}, uuid(11))), track.ErrScope},
		{"non-canonical reference", second(track.NewSessionRef(w.scope, strings.ToUpper(uuid(11)))), track.ErrInvalid},
		{"unconstructed existing", second(track.Open(w.scope, openWith(w, func(in *track.OpenInput) { in.Existing = &zero }))), track.ErrInvalid},
		{"unparsed origin", second(track.Open(w.scope, openWith(w, func(in *track.OpenInput) { in.Origin = "screen" }))), track.ErrInvalid},
		{"unparsed policy", second(track.Open(w.scope, openWith(w, func(in *track.OpenInput) { in.Policy = "on" }))), track.ErrInvalid},
		{"unparsed trip state", second(track.Open(w.scope, openWith(w, func(in *track.OpenInput) { in.Trip.State = "RUNNING" }))), track.ErrInvalid},
		{"year above profile", second(track.Open(w.scope, openWith(w, func(in *track.OpenInput) {
			in.At = time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)
		}))), track.ErrInvalid},
		{"unparsed permission", second(s.Start(w.scope, w.action("FINE", 10))), track.ErrInvalid},
		{"unparsed cause", second(s.Suspend(w.scope, w.event(trip.Paused, "PAUSE", 10))), track.ErrInvalid},
	}
	for _, c := range checks {
		if !errors.Is(c.err, c.want) {
			t.Fatal(c.name, c.err)
		}
	}
	if _, e := track.Classify(track.TrackEvidence{ExpectedSeconds: track.MaxExpectedSeconds + 1}); !errors.Is(e, track.ErrInvalid) {
		t.Fatal("expected time above the bound accepted")
	}
	for _, raw := range []string{"", " ", "ended", "PAUSED"} {
		if _, e := track.ParseState(raw); !errors.Is(e, track.ErrInvalid) {
			t.Fatal("state token accepted", raw)
		}
	}
	if !track.Ended.Terminal() || track.Recording.Terminal() || track.State("X").Terminal() {
		t.Fatal("terminal state set changed")
	}
}

func openWith(w world, change func(*track.OpenInput)) track.OpenInput {
	in := track.OpenInput{Session: must(track.NewSessionRef(w.scope, uuid(11))), Trip: w.facts, Author: w.driver,
		Origin: track.OriginTripScreen, Policy: track.PolicyExpected, At: at(0)}
	change(&in)
	return in
}

func second[A any](_ A, e error) error        { return e }
func third[A, B any](_ A, _ B, e error) error { return e }

// B-BR-EMP-425: the trip lifecycle can never read the session, because the
// trip packages do not import tracking (and Go forbids the import cycle).
func TestTripDoesNotDependOnTracking(t *testing.T) {
	for _, dir := range []string{"../../trip/domain", "../../trip/application", "../../trip/adapters"} {
		files, e := filepath.Glob(filepath.Join(dir, "*.go"))
		if e != nil || len(files) == 0 {
			t.Fatal("trip sources not found", dir, e)
		}
		for _, file := range files {
			f, e := parser.ParseFile(token.NewFileSet(), file, nil, parser.ImportsOnly)
			if e != nil {
				t.Fatal(e)
			}
			for _, imp := range f.Imports {
				if strings.Contains(imp.Path.Value, "/modules/tracking") {
					t.Fatal("the trip depends on tracking", file)
				}
			}
		}
	}
	if _, e := os.Stat("../../trip/domain/trip.go"); e != nil {
		t.Fatal(e)
	}
}

// Commands return new values; branches from one session never share history.
func TestConcurrentCommandsShareOneImmutableSession(t *testing.T) {
	defer guard(t)
	w := newWorld(t)
	base := must(must(w.open(t).Start(w.scope, w.action(track.PermissionPrecise, 10))).Suspend(w.scope,
		w.event(trip.Paused, track.CauseTripPaused, 20)))
	snapshot := sessionFields(base)
	results := make([]error, 32)
	var wg sync.WaitGroup
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			permission := track.PermissionPrecise
			if i%2 == 1 {
				permission = track.PermissionApproximate
			}
			next, e := base.Resume(w.scope, w.action(permission, 30+i))
			switch {
			case e != nil:
				results[i] = e
			case next.Precision() != permission || len(next.History()) != 4 || !next.History()[3].At().Equal(at(30+i)):
				results[i] = fmt.Errorf("branch %d lost its own resume", i)
			}
		}(i)
	}
	wg.Wait()
	for _, e := range results {
		if e != nil {
			t.Fatal(e)
		}
	}
	a := must(base.Resume(w.scope, w.action(track.PermissionPrecise, 40)))
	b := must(base.End(w.scope, w.event(trip.Cancelled, track.CauseTripCancelled, 50)))
	if a.History()[3].State() != track.Recording || b.History()[3].State() != track.Ended {
		t.Fatal("two branches share one history")
	}
	if fmt.Sprint(snapshot) != fmt.Sprint(sessionFields(base)) {
		t.Fatal("a command changed its input")
	}
}

func TestPrivateValuesAreRedacted(t *testing.T) {
	defer guard(t)
	w := newWorld(t)
	s := must(w.open(t).Start(w.scope, w.action(track.PermissionPrecise, 10)))
	values := []any{s, s.Reference(), s.History()[1], s.RecordingPeriods()[0], w.facts, w.action(track.PermissionPrecise, 10),
		w.event(trip.Paused, track.CauseTripPaused, 20), track.Device{Permission: track.PermissionPrecise},
		openWith(w, func(*track.OpenInput) {})}
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
		if a.Key == slog.TimeKey {
			return slog.Attr{}
		}
		return a
	}}))
	for _, v := range values {
		for _, format := range []string{"%v", "%+v", "%#v", "%s", "%q"} {
			assertRedacted(t, fmt.Sprintf(format, v))
		}
		raw, e := json.Marshal(v)
		if e != nil {
			t.Fatal(e)
		}
		assertRedacted(t, string(raw))
		logger.Info("tracking", "value", v)
	}
	assertRedacted(t, logs.String())
}

func assertRedacted(t *testing.T, out string) {
	t.Helper()
	if !strings.Contains(out, "redacted") {
		t.Fatal("value printed without redaction", out)
	}
	for _, secret := range []string{"9b000000", "2026", "PRECISE", "RECORDING", "TRIP_PAUSED"} {
		if strings.Contains(out, secret) {
			t.Fatal("private value leaked", out)
		}
	}
}
