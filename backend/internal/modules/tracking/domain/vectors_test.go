package domain_test

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	track "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/tracking/domain"
	trip "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/domain"
)

const (
	corpusPath = "../../../../../contracts/testdata/enterprise/tracking/session-v1.psv"
	corpusHash = "4ddac666121fe1183e067dd65589a57c3f97a82e230e3091454ce9b889f9d39e"
	corpusSize = 359
)

type fields map[string]string

var (
	errMalformedFixture = errors.New("fixture: malformed grammar")
	baseInstant         = time.Date(2026, 11, 15, 10, 0, 0, 0, time.UTC)
	frozenCounts        = map[string]int{"ENUM": 64, "OPEN": 55, "START": 53, "RESUME": 30, "SUSPEND": 23, "INTERRUPT": 28,
		"REDUCE": 13, "END": 23, "FOLLOW": 22, "COLLECT": 22, "QUALITY": 26}
	errorCodes = []string{"INVALID", "SCOPE", "FORBIDDEN", "TRACKING_OFF", "PERSONAL_TRIP", "IMPLICIT_START",
		"SESSION_EXISTS", "INVALID_TRANSITION", "TRIP_STATE", "OUT_OF_ORDER"}
	tripKeys    = "prior scope trip trip_org driver driver_org trip_state"
	allowedArgs = map[string]string{
		"ENUM":    "kind value_hex",
		"OPEN":    "scope org session session_org trip trip_org driver driver_org trip_state author author_org origin policy at existing omit",
		"START":   tripKeys + " author author_org origin permission notification at omit",
		"RESUME":  tripKeys + " author author_org origin permission notification at omit",
		"SUSPEND": tripKeys + " cause author author_org at omit", "INTERRUPT": tripKeys + " cause author author_org at omit",
		"REDUCE": tripKeys + " cause author author_org at omit", "END": tripKeys + " cause author author_org at omit",
		"FOLLOW": tripKeys + " at omit", "COLLECT": tripKeys + " omit",
		"QUALITY": "expected gap fixes coarse"}
	openOmittable    = map[string]bool{"session": true, "trip": true, "driver": true, "state": true, "author": true, "origin": true, "policy": true, "at": true}
	actionOmittable  = map[string]bool{"trip": true, "driver": true, "state": true, "author": true, "origin": true, "permission": true, "at": true}
	eventOmittable   = map[string]bool{"trip": true, "driver": true, "state": true, "author": true, "cause": true, "at": true}
	followOmittable  = map[string]bool{"trip": true, "driver": true, "state": true, "at": true}
	collectOmittable = map[string]bool{"trip": true, "driver": true, "state": true}
	defaultCause     = map[string]string{"SUSPEND": "TRIP_PAUSED", "INTERRUPT": "PROCESS_STOPPED", "REDUCE": "PRECISION_REDUCED", "END": "TRIP_COMPLETED"}
	aliases          = map[string]int{"A": 1, "B": 2, "S": 11, "S2": 12, "T": 21, "T2": 22, "M": 31, "M2": 32}
)

func uuid(n int) string { return fmt.Sprintf("9b000000-0000-4000-8000-%012d", n) }

func fixtureID(alias string) string {
	if n, ok := aliases[alias]; ok {
		return uuid(n)
	}
	return alias
}

func alias(id string) string {
	for name, n := range aliases {
		if uuid(n) == id {
			return name
		}
	}
	return id
}

func (f fields) get(key, other string) string {
	if v, ok := f[key]; ok {
		return v
	}
	return other
}

func fieldMap(raw string) (fields, error) {
	out := fields{}
	for _, part := range strings.Split(raw, ";") {
		k, v, ok := strings.Cut(part, "=")
		if !ok || strings.TrimSpace(k) == "" {
			return nil, errMalformedFixture
		}
		if _, has := out[k]; has {
			return nil, errMalformedFixture
		}
		out[k] = v
	}
	return out, nil
}

func decodeHex(raw string) (string, error) {
	v, e := hex.DecodeString(raw)
	if e != nil || raw != strings.ToLower(raw) || !utf8.Valid(v) {
		return "", errMalformedFixture
	}
	return string(v), nil
}

func omitted(t *testing.T, raw string, allowed map[string]bool) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	if raw == "" {
		return out
	}
	for _, key := range strings.Split(raw, ",") {
		if !allowed[key] || out[key] {
			t.Fatal("unknown or repeated omit fixture field", key)
		}
		out[key] = true
	}
	return out
}

// vectorScope leaves a malformed or non-canonical organization as the zero
// scope so that the domain itself, not the fixture reader, refuses it.
func vectorScope(raw string) kernel.OrgScope {
	id, e := kernel.ParseOrgID(fixtureID(raw))
	if e != nil || id.String() != fixtureID(raw) {
		return kernel.OrgScope{}
	}
	s, _ := kernel.NewOrgScope(id)
	return s
}

// vectorInstant reads a whole-minute offset from the fixture base, or "zero".
func vectorInstant(t *testing.T, raw string) time.Time {
	t.Helper()
	if raw == "zero" {
		return time.Time{}
	}
	n, e := strconv.ParseInt(raw, 10, 64)
	if e != nil {
		t.Fatal("malformed fixture instant", raw)
	}
	return baseInstant.Add(time.Duration(n) * time.Minute)
}

func minute(at time.Time) string {
	return strconv.FormatInt(int64(at.Sub(baseInstant)/time.Minute), 10)
}

func boolField(t *testing.T, raw string) bool {
	t.Helper()
	switch raw {
	case "true":
		return true
	case "false":
		return false
	}
	t.Fatal("malformed fixture flag", raw)
	return false
}

// derivedTripState is the documented default trip state of a command.
func derivedTripState(op, cause string) string {
	switch {
	case op == "SUSPEND" && cause == "TRIP_PAUSED":
		return "PAUSED"
	case op == "END" && cause == "TRIP_COMPLETED":
		return "COMPLETED"
	case op == "END" && cause == "TRIP_CANCELLED":
		return "CANCELLED"
	}
	return "IN_PROGRESS"
}

// commandFields applies the documented command defaults, then the overrides.
func commandFields(op string, args fields) fields {
	f := fields{"scope": "A", "trip": "T", "trip_org": "A", "driver": "M", "driver_org": "A", "author": "M", "author_org": "A",
		"origin": "TRIP_SCREEN", "permission": "PRECISE", "notification": "true", "cause": defaultCause[op], "at": "100"}
	for k, v := range args {
		f[k] = v
	}
	f["trip_state"] = f.get("trip_state", derivedTripState(op, f["cause"]))
	return f
}

func vectorFacts(t *testing.T, f fields, omit map[string]bool) (track.TripFacts, error) {
	t.Helper()
	var facts track.TripFacts
	var e error
	if !omit["trip"] {
		if facts.Ref, e = trip.NewTripRef(vectorScope(f["trip_org"]), fixtureID(f["trip"])); e != nil {
			return facts, e
		}
	}
	if !omit["driver"] {
		if facts.Driver, e = trip.NewMembershipRef(vectorScope(f["driver_org"]), fixtureID(f["driver"])); e != nil {
			return facts, e
		}
	}
	if !omit["state"] {
		if facts.State, e = trip.ParseState(f["trip_state"]); e != nil {
			return facts, e
		}
	}
	return facts, nil
}

func vectorAuthor(f fields, omit map[string]bool) (trip.MembershipRef, error) {
	if omit["author"] {
		return trip.MembershipRef{}, nil
	}
	return trip.NewMembershipRef(vectorScope(f["author_org"]), fixtureID(f["author"]))
}

func openFields(args fields) fields {
	f := fields{"org": "A", "session": "S", "trip": "T", "driver": "M", "trip_state": "IN_PROGRESS", "author": "M",
		"origin": "TRIP_SCREEN", "policy": "EXPECTED", "at": "0", "existing": "none"}
	for k, v := range args {
		f[k] = v
	}
	for _, key := range []string{"scope", "session_org", "trip_org", "driver_org", "author_org"} {
		f[key] = f.get(key, f["org"])
	}
	return f
}

// existingSession builds the documented fixed existing session (S2).
func existingSession(t *testing.T, kind string) *track.Session {
	t.Helper()
	var s track.Session
	switch kind {
	case "none":
		return nil
	case "open":
		s = mustOpen(t, fields{"session": "S2"})
	case "ended":
		s = mustOpen(t, fields{"session": "S2"})
		var e error
		if s, e = runCommand(t, s, "END", commandFields("END", fields{"cause": "MEMBERSHIP_ENDED", "at": "0"})); e != nil {
			t.Fatal("existing fixture session did not end", e)
		}
	case "other_trip":
		s = mustOpen(t, fields{"session": "S2", "trip": "T2"})
	case "foreign":
		s = mustOpen(t, fields{"session": "S2", "org": "B"})
	default:
		t.Fatal("unknown existing fixture session", kind)
	}
	return &s
}

func mustOpen(t *testing.T, args fields) track.Session {
	t.Helper()
	s, e := vectorOpen(t, openFields(args))
	if e != nil {
		t.Fatal("fixture session did not open", e)
	}
	return s
}

func vectorOpen(t *testing.T, f fields) (track.Session, error) {
	t.Helper()
	omit := omitted(t, f["omit"], openOmittable)
	in := track.OpenInput{Existing: existingSession(t, f["existing"])}
	var e error
	if !omit["session"] {
		if in.Session, e = track.NewSessionRef(vectorScope(f["session_org"]), fixtureID(f["session"])); e != nil {
			return track.Session{}, e
		}
	}
	if in.Trip, e = vectorFacts(t, f, omit); e != nil {
		return track.Session{}, e
	}
	if in.Author, e = vectorAuthor(f, omit); e != nil {
		return track.Session{}, e
	}
	if !omit["origin"] {
		if in.Origin, e = track.ParseOrigin(f["origin"]); e != nil {
			return track.Session{}, e
		}
	}
	if !omit["policy"] {
		if in.Policy, e = track.ParsePolicy(f["policy"]); e != nil {
			return track.Session{}, e
		}
	}
	if !omit["at"] {
		in.At = vectorInstant(t, f["at"])
	}
	return track.Open(vectorScope(f["scope"]), in)
}

func driverAction(t *testing.T, f fields) (track.DriverAction, error) {
	t.Helper()
	omit := omitted(t, f["omit"], actionOmittable)
	var a track.DriverAction
	var e error
	if a.Trip, e = vectorFacts(t, f, omit); e != nil {
		return a, e
	}
	if a.Author, e = vectorAuthor(f, omit); e != nil {
		return a, e
	}
	if !omit["origin"] {
		if a.Origin, e = track.ParseOrigin(f["origin"]); e != nil {
			return a, e
		}
	}
	if !omit["permission"] {
		if a.Device.Permission, e = track.ParsePermission(f["permission"]); e != nil {
			return a, e
		}
	}
	a.Device.NotificationVisible = boolField(t, f["notification"])
	if !omit["at"] {
		a.At = vectorInstant(t, f["at"])
	}
	return a, nil
}

func event(t *testing.T, f fields) (track.Event, error) {
	t.Helper()
	omit := omitted(t, f["omit"], eventOmittable)
	var ev track.Event
	var e error
	if ev.Trip, e = vectorFacts(t, f, omit); e != nil {
		return ev, e
	}
	if ev.Author, e = vectorAuthor(f, omit); e != nil {
		return ev, e
	}
	if !omit["cause"] {
		if ev.Cause, e = track.ParseCause(f["cause"]); e != nil {
			return ev, e
		}
	}
	if !omit["at"] {
		ev.At = vectorInstant(t, f["at"])
	}
	return ev, nil
}

// runCommand builds the command input in the documented order and applies it.
func runCommand(t *testing.T, s track.Session, op string, f fields) (track.Session, error) {
	t.Helper()
	scope := vectorScope(f["scope"])
	switch op {
	case "START", "RESUME":
		a, e := driverAction(t, f)
		if e != nil {
			return track.Session{}, e
		}
		if op == "START" {
			return s.Start(scope, a)
		}
		return s.Resume(scope, a)
	case "SUSPEND", "INTERRUPT", "REDUCE", "END":
		ev, e := event(t, f)
		if e != nil {
			return track.Session{}, e
		}
		switch op {
		case "SUSPEND":
			return s.Suspend(scope, ev)
		case "INTERRUPT":
			return s.Interrupt(scope, ev)
		case "REDUCE":
			return s.ReducePrecision(scope, ev)
		}
		return s.End(scope, ev)
	case "FOLLOW":
		omit := omitted(t, f["omit"], followOmittable)
		facts, e := vectorFacts(t, f, omit)
		if e != nil {
			return track.Session{}, e
		}
		var at time.Time
		if !omit["at"] {
			at = vectorInstant(t, f["at"])
		}
		return s.Follow(scope, facts, at)
	}
	t.Fatal("unknown session command", op)
	return track.Session{}, nil
}

// priorSession opens the default session at minute 0 and applies each prior
// step (CMD or CMD:ARG) at minutes 10, 20, ...; a failing step fails the loader.
func priorSession(t *testing.T, raw string) track.Session {
	t.Helper()
	s := mustOpen(t, fields{})
	if raw == "" {
		return s
	}
	for i, step := range strings.Split(raw, ",") {
		op, arg, _ := strings.Cut(step, ":")
		f := fields{"at": strconv.Itoa(10 * (i + 1))}
		switch op {
		case "START", "RESUME":
			switch arg {
			case "", "PRECISE":
			case "APPROXIMATE", "NONE":
				f["permission"] = arg
			case "BLOCKED":
				f["notification"] = "false"
			default:
				t.Fatal("unknown prior permission", step)
			}
		case "SUSPEND", "INTERRUPT", "END":
			f["cause"] = arg
		case "REDUCE":
			if arg != "" {
				t.Fatal("REDUCE takes no prior argument")
			}
		case "FOLLOW":
			f["trip_state"] = arg
		default:
			t.Fatal("unknown prior step", step)
		}
		var e error
		if s, e = runCommand(t, s, op, commandFields(op, f)); e != nil {
			t.Fatal("prior fixture step failed", step, e)
		}
	}
	return s
}

func optionalToken(raw string) string {
	if raw == "" {
		return "?"
	}
	return raw
}

func sessionFields(s track.Session) fields {
	var history, periods []string
	for _, h := range s.History() {
		history = append(history, fmt.Sprintf("%s:%s:%s:%s@%s", h.State(), optionalToken(string(h.Cause())), h.Actor(),
			optionalToken(string(h.Permission())), minute(h.At())))
	}
	for _, p := range s.RecordingPeriods() {
		end := "?"
		if at, ok := p.End(); ok {
			end = minute(at)
		}
		periods = append(periods, fmt.Sprintf("%s~%s~%s", minute(p.Start()), end, p.Precision()))
	}
	out := fields{"org": alias(s.Reference().OrganizationID().String()), "id": alias(s.Reference().ID()), "trip": alias(s.Trip().ID()),
		"owner": alias(s.Owner().ID()), "policy": string(s.Policy()), "state": string(s.State()), "cause": optionalToken(string(s.Cause())),
		"precision": optionalToken(string(s.Precision())), "coarse": strconv.FormatBool(s.Coarse()),
		"history": strings.Join(history, ","), "periods": "?"}
	if len(periods) > 0 {
		out["periods"] = strings.Join(periods, ",")
	}
	if s.Trip().OrganizationID() != s.Reference().OrganizationID() || s.Owner().OrganizationID() != s.Reference().OrganizationID() {
		out["org"] = "MIXED"
	}
	return out
}

func enumFields(t *testing.T, f fields) (fields, error) {
	t.Helper()
	raw, e := decodeHex(f["value_hex"])
	if e != nil {
		t.Fatal(e)
	}
	switch f["kind"] {
	case "STATE":
		s, e := track.ParseState(raw)
		if e != nil {
			return nil, e
		}
		return fields{"value": string(s), "terminal": strconv.FormatBool(s.Terminal())}, nil
	case "PERMISSION":
		v, e := track.ParsePermission(raw)
		return fields{"value": string(v)}, e
	case "POLICY":
		v, e := track.ParsePolicy(raw)
		return fields{"value": string(v)}, e
	case "ORIGIN":
		v, e := track.ParseOrigin(raw)
		return fields{"value": string(v)}, e
	case "CAUSE":
		v, e := track.ParseCause(raw)
		return fields{"value": string(v)}, e
	case "ACTOR":
		v, e := track.ParseActor(raw)
		return fields{"value": string(v)}, e
	case "QUALITY":
		v, e := track.ParseQuality(raw)
		return fields{"value": string(v)}, e
	}
	t.Fatal("unknown enum fixture kind", f["kind"])
	return nil, nil
}

func qualityFields(t *testing.T, args fields) (fields, error) {
	t.Helper()
	f := fields{"expected": "20400", "gap": "1020", "fixes": "2160", "coarse": "false"}
	for k, v := range args {
		f[k] = v
	}
	var n [3]int64
	for i, key := range []string{"expected", "gap", "fixes"} {
		v, e := strconv.ParseInt(f[key], 10, 64)
		if e != nil {
			t.Fatal("malformed fixture count", key)
		}
		n[i] = v
	}
	c, e := track.Classify(track.TrackEvidence{ExpectedSeconds: n[0], GapSeconds: n[1], AcceptedFixes: n[2], Coarse: boolField(t, f["coarse"])})
	if e != nil {
		return nil, e
	}
	out := fields{"quality": string(c.Quality()), "coverage": "?"}
	if permille, ok := c.Coverage(); ok {
		out["coverage"] = strconv.FormatInt(permille, 10)
	}
	return out, nil
}

func runVector(t *testing.T, op string, args fields) (fields, error) {
	t.Helper()
	switch op {
	case "ENUM":
		return enumFields(t, args)
	case "QUALITY":
		return qualityFields(t, args)
	case "OPEN":
		s, e := vectorOpen(t, openFields(args))
		if e != nil {
			return nil, e
		}
		return sessionFields(s), nil
	}
	s := priorSession(t, args["prior"])
	before := sessionFields(s)
	defer func() {
		if !reflect.DeepEqual(before, sessionFields(s)) {
			t.Fatal("a command changed its input session")
		}
	}()
	if op == "COLLECT" {
		f := commandFields(op, args)
		facts, e := vectorFacts(t, f, omitted(t, f["omit"], collectOmittable))
		if e != nil {
			return nil, e
		}
		precision, ok, e := s.Collecting(vectorScope(f["scope"]), facts)
		if e != nil {
			return nil, e
		}
		return fields{"collecting": strconv.FormatBool(ok), "precision": optionalToken(string(precision))}, nil
	}
	next, e := runCommand(t, s, op, commandFields(op, args))
	if e != nil {
		return nil, e
	}
	return sessionFields(next), nil
}

func errorFields(e error) fields {
	for _, c := range []struct {
		err  error
		code string
	}{{track.ErrInvalid, "INVALID"}, {track.ErrScope, "SCOPE"}, {track.ErrForbidden, "FORBIDDEN"},
		{track.ErrTrackingOff, "TRACKING_OFF"}, {track.ErrPersonalTrip, "PERSONAL_TRIP"}, {track.ErrImplicitStart, "IMPLICIT_START"},
		{track.ErrSessionExists, "SESSION_EXISTS"}, {track.ErrInvalidTransition, "INVALID_TRANSITION"},
		{track.ErrTripState, "TRIP_STATE"}, {track.ErrOutOfOrder, "OUT_OF_ORDER"},
		{trip.ErrInvalid, "INVALID"}, {trip.ErrScope, "SCOPE"}} {
		if errors.Is(e, c.err) {
			return fields{"error": c.code}
		}
	}
	return fields{"error": "UNMAPPED:" + e.Error()}
}

func TestSharedReaderRejectsMalformedGrammar(t *testing.T) {
	for _, raw := range []string{"", "novalue", "=x", "a=1;a=2", " =1"} {
		if _, e := fieldMap(raw); !errors.Is(e, errMalformedFixture) {
			t.Fatal("malformed field map accepted", raw)
		}
	}
	for _, raw := range []string{"ff", "eda080", "4F", "4"} {
		if _, e := decodeHex(raw); !errors.Is(e, errMalformedFixture) {
			t.Fatal("malformed fixture hex accepted", raw)
		}
	}
	if got := fieldsOrFail(t, "kind=STATE;value_hex="); got["value_hex"] != "" {
		t.Fatal("empty fixture value lost")
	}
	if vectorScope("not-an-org").Validate() == nil || vectorScope("A").Validate() != nil {
		t.Fatal("fixture scope reader changed meaning")
	}
	if minute(vectorInstant(t, "-15")) != "-15" || !vectorInstant(t, "zero").IsZero() {
		t.Fatal("fixture instant reader changed meaning")
	}
}

func fieldsOrFail(t *testing.T, raw string) fields {
	t.Helper()
	f, e := fieldMap(raw)
	if e != nil {
		t.Fatal(e)
	}
	return f
}

func TestSharedTrackingSessionVectors(t *testing.T) {
	raw, e := os.ReadFile(corpusPath)
	if e != nil {
		t.Fatal(e)
	}
	if fmt.Sprintf("%x", sha256.Sum256(raw)) != corpusHash {
		t.Fatal("shared tracking corpus hash changed")
	}
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	scanner.Buffer(make([]byte, 4096), 1<<20)
	if !scanner.Scan() || scanner.Text() != "id|operation|args|expected" {
		t.Fatal("invalid shared corpus header")
	}
	counts, ids, codes := map[string]int{}, map[string]bool{}, map[string]bool{}
	for scanner.Scan() {
		p := strings.Split(scanner.Text(), "|")
		if len(p) != 4 || strings.TrimSpace(p[0]) == "" || ids[p[0]] {
			t.Fatal("malformed/duplicate shared case", p[0])
		}
		if _, ok := frozenCounts[p[1]]; !ok {
			t.Fatal("unknown shared operation", p[1])
		}
		ids[p[0]] = true
		counts[p[1]]++
		args, er := fieldMap(p[2])
		if er != nil {
			t.Fatal(p[0], er)
		}
		want, er := fieldMap(p[3])
		if er != nil {
			t.Fatal(p[0], er)
		}
		for k := range args {
			if !strings.Contains(" "+allowedArgs[p[1]]+" ", " "+k+" ") {
				t.Fatal("unknown fixture argument", p[0], k)
			}
		}
		if code, ok := want["error"]; ok {
			if len(want) != 1 {
				t.Fatal("error result carries output", p[0])
			}
			codes[code] = true
		}
		t.Run(p[0], func(t *testing.T) {
			got, err := runVector(t, p[1], args)
			if err != nil {
				got = errorFields(err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("shared mismatch: got %v expected %v", got, want)
			}
		})
	}
	if e = scanner.Err(); e != nil {
		t.Fatal(e)
	}
	if len(ids) != corpusSize || !reflect.DeepEqual(counts, frozenCounts) {
		t.Fatal("incomplete tracking corpus", len(ids), counts)
	}
	for _, code := range errorCodes {
		if !codes[code] {
			t.Fatal("error code without a shared vector", code)
		}
	}
	if len(codes) != len(errorCodes) {
		t.Fatal("undocumented error code in tracking corpus", codes)
	}
}
