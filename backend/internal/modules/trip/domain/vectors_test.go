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
	"unicode/utf8"

	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain/meter"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	trip "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/domain"
)

const (
	// trip-v2 supersedes trip-v1 (EMP-TRIP-011, EDR-252).
	corpusPath = "../../../../../contracts/testdata/enterprise/trip/trip-v2.psv"
	corpusHash = "43aecf00903fffaff2ab17c3530595c3647e4040953fe2ff5e28b104b0bef41e"
	corpusSize = 297
)

type fields map[string]string

var (
	errUnknownEnumSelector = errors.New("fixture: unknown enum namespace")
	errMalformedFixture    = errors.New("fixture: malformed grammar")
	wantCounts             = map[string]int{"ENUM": 49, "TRANSITION": 40, "STANDING": 14, "START": 57, "FINISH": 47, "PAUSE": 40, "RESUME": 16, "CANCEL": 22, "HISTORY": 12}
	errorCodes             = []string{"INVALID", "SCOPE", "INCOMPLETE", "FORBIDDEN", "INVALID_TRANSITION", "UNSUPPORTED_COMMAND", "FINISH_BEFORE_START", "CONFLICT", "BEFORE_START"}
	startKeys              = "scope org id id_org author author_org asset asset_org driver driver_org dep dep_org dep_asset dep_meter dep_segment " +
		"dep_reading dep_kind dep_value_kind dep_value dep_modulus dep_reason_hex at entry offset zone unit_zone omit"
	finishKeys = startKeys + " fin_scope fin_author fin_author_org fin fin_org fin_asset fin_meter fin_segment fin_reading fin_kind " +
		"fin_value_kind fin_value fin_modulus fin_reason_hex fin_at fin_entry fin_offset fin_omit again"
	commandKeys = startKeys + " history cmd_scope by by_org expected reason_hex cmd_at cmd_entry cmd_offset cmd_omit again"
	allowedArgs = map[string]string{"ENUM": "kind value_hex", "TRANSITION": "state command", "STANDING": "operational review",
		"START": startKeys, "FINISH": finishKeys, "PAUSE": commandKeys, "RESUME": commandKeys, "CANCEL": commandKeys, "HISTORY": startKeys + " history"}
	commandOmittable = map[string]bool{"author": true, "declaration": true}
	cancelReasonHex  = hex.EncodeToString([]byte("Cliente cancelou"))
	startOmittable   = map[string]bool{"id": true, "author": true, "asset": true, "driver": true, "departure": true, "declaration": true, "zone": true}
	finishOmittable  = map[string]bool{"author": true, "final": true, "declaration": true}
	aliases          = map[string]int{"A": 1, "B": 2, "T": 11, "T2": 12, "V": 21, "V2": 22, "D": 31, "D2": 32, "M": 41, "M2": 42,
		"MT": 51, "MT2": 52, "SG": 61, "SG2": 62, "R1": 71, "R2": 72, "R3": 73}
	departureReasonHex = hex.EncodeToString([]byte("Odometro ilegivel"))
	finalReasonHex     = hex.EncodeToString([]byte("Sem leitura final"))
)

func validateEnumSelector(raw string) error {
	switch raw {
	case "STATE", "COMMAND", "REVIEW":
		return nil
	}
	return errUnknownEnumSelector
}

func uuid(n int) string { return fmt.Sprintf("5e000000-0000-4000-8000-%012d", n) }

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

func TestSharedReaderRejectsMalformedGrammar(t *testing.T) {
	for _, raw := range []string{"", "UNKNOWN", "state", "STATE ", "STANDING"} {
		if !errors.Is(validateEnumSelector(raw), errUnknownEnumSelector) {
			t.Fatal("malformed fixture namespace reached domain dispatch")
		}
	}
	for _, raw := range []string{"STATE", "COMMAND", "REVIEW"} {
		if validateEnumSelector(raw) != nil {
			t.Fatal("documented fixture namespace refused")
		}
	}
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

func optional(f fields, key string) *string {
	v, ok := f[key]
	if !ok || v == "?" {
		return nil
	}
	return &v
}

func optionalInt(f fields, key string) (*int64, error) {
	raw := optional(f, key)
	if raw == nil {
		return nil, nil
	}
	n, e := strconv.ParseInt(*raw, 10, 64)
	if e != nil {
		return nil, errMalformedFixture
	}
	return &n, nil
}

// vectorObservation builds departure ("dep") or final ("fin") evidence.
func vectorObservation(t *testing.T, f fields, prefix string) (trip.MeterObservation, error) {
	t.Helper()
	switch f[prefix] {
	case "ABSENT":
		reason, e := decodeHex(f[prefix+"_reason_hex"])
		if e != nil {
			t.Fatal(e)
		}
		m, e := meter.NewMissing(reason)
		if e != nil {
			return trip.MeterObservation{}, e
		}
		return trip.AbsentReading(m)
	case "READING":
	default:
		t.Fatal("unknown evidence fixture mode", f[prefix])
	}
	s := vectorScope(f[prefix+"_org"])
	id, e := fleet.ParseAssetID(fixtureID(f[prefix+"_asset"]))
	if e != nil {
		return trip.MeterObservation{}, e
	}
	asset, e := fleet.NewAssetRef(s, id)
	if e != nil {
		return trip.MeterObservation{}, e
	}
	modulus, e := optionalInt(f, prefix+"_modulus")
	if e != nil {
		t.Fatal(e)
	}
	m, e := meter.NewMeterRef(s, asset, fixtureID(f[prefix+"_meter"]), meter.Kind(f[prefix+"_kind"]), modulus)
	if e != nil {
		return trip.MeterObservation{}, e
	}
	seg, e := meter.NewSegmentRef(s, m, fixtureID(f[prefix+"_segment"]))
	if e != nil {
		return trip.MeterObservation{}, e
	}
	ref, e := meter.NewReadingRef(s, seg, fixtureID(f[prefix+"_reading"]))
	if e != nil {
		return trip.MeterObservation{}, e
	}
	value, e := meter.ParseBase(meter.Kind(f[prefix+"_value_kind"]), f[prefix+"_value"], nil)
	if e != nil {
		return trip.MeterObservation{}, e
	}
	return trip.ObservedReading(ref, value)
}

func declaration(at, entry, offset string) (*orgtime.Declaration, error) {
	instant, e := orgtime.ParseInstant(at)
	if e != nil {
		return nil, trip.ErrInvalid
	}
	seconds, e := strconv.ParseInt(offset, 10, 64)
	if e != nil {
		return nil, trip.ErrInvalid
	}
	return &orgtime.Declaration{OccurredAt: instant, EntryZone: entry, DeviceOffsetSeconds: seconds}, nil
}

// startFields applies the documented fixture defaults, then the overrides.
func startFields(args fields) fields {
	f := fields{"org": "A", "id": "T", "author": "M", "asset": "V", "driver": "D", "dep": "READING", "dep_meter": "MT",
		"dep_segment": "SG", "dep_reading": "R1", "dep_kind": "ODOMETER", "dep_value": "184320000", "dep_reason_hex": departureReasonHex,
		"at": "2026-11-14T12:00:00Z", "entry": "America/Cuiaba", "offset": "-14400", "zone": "America/Cuiaba", "unit_zone": "?"}
	for k, v := range args {
		f[k] = v
	}
	org := f["org"]
	for _, key := range []string{"scope", "id_org", "author_org", "asset_org", "driver_org", "dep_org"} {
		f[key] = f.get(key, org)
	}
	f["dep_asset"] = f.get("dep_asset", f["asset"])
	f["dep_value_kind"] = f.get("dep_value_kind", f["dep_kind"])
	f["dep_modulus"] = f.get("dep_modulus", "?")
	return f
}

func vectorStart(t *testing.T, f fields) (trip.Trip, error) {
	t.Helper()
	omit := omitted(t, f["omit"], startOmittable)
	var in trip.StartInput
	var e error
	if !omit["id"] {
		if in.Reference, e = trip.NewTripRef(vectorScope(f["id_org"]), fixtureID(f["id"])); e != nil {
			return trip.Trip{}, e
		}
	}
	if !omit["author"] {
		if in.Author, e = trip.NewMembershipRef(vectorScope(f["author_org"]), fixtureID(f["author"])); e != nil {
			return trip.Trip{}, e
		}
	}
	if !omit["asset"] {
		id, er := fleet.ParseAssetID(fixtureID(f["asset"]))
		if er != nil {
			return trip.Trip{}, er
		}
		if in.Asset, e = fleet.NewAssetRef(vectorScope(f["asset_org"]), id); e != nil {
			return trip.Trip{}, e
		}
	}
	if !omit["driver"] {
		if in.Driver, e = trip.NewEmployeeRef(vectorScope(f["driver_org"]), fixtureID(f["driver"])); e != nil {
			return trip.Trip{}, e
		}
	}
	if !omit["departure"] {
		if in.Departure, e = vectorObservation(t, f, "dep"); e != nil {
			return trip.Trip{}, e
		}
	}
	if !omit["declaration"] {
		if in.Declared, e = declaration(f["at"], f["entry"], f["offset"]); e != nil {
			return trip.Trip{}, e
		}
	}
	if !omit["zone"] {
		in.Zones = orgtime.ReportingZones{Organization: f["zone"], Unit: optional(f, "unit_zone")}
	}
	if raw, ok := f["boot_hex"]; ok {
		c, e := vectorClock(t, raw, f["mono"])
		if e != nil {
			return trip.Trip{}, e
		}
		in.Clock = &c
	}
	return trip.Start(vectorScope(f["scope"]), in)
}

func finishFields(f fields) fields {
	g := fields{"fin_scope": f["scope"], "fin_author": "M", "fin_author_org": f["org"], "fin": "READING", "fin_org": f["org"],
		"fin_asset": f["asset"], "fin_meter": f["dep_meter"], "fin_segment": f["dep_segment"], "fin_reading": "R2",
		"fin_kind": f["dep_kind"], "fin_value": "184610000", "fin_modulus": f["dep_modulus"], "fin_reason_hex": finalReasonHex,
		"fin_at": "2026-11-14T15:00:00Z", "fin_entry": "America/Cuiaba", "fin_offset": "-14400"}
	for k, v := range g {
		f[k] = f.get(k, v)
	}
	f["fin_value_kind"] = f.get("fin_value_kind", f["fin_kind"])
	return f
}

// vectorFinishInput builds the fixture finish input of finish fields f.
func vectorFinishInput(t *testing.T, f fields) (trip.FinishInput, error) {
	t.Helper()
	omit := omitted(t, f["fin_omit"], finishOmittable)
	var in trip.FinishInput
	var e error
	if !omit["author"] {
		if in.Author, e = trip.NewMembershipRef(vectorScope(f["fin_author_org"]), fixtureID(f["fin_author"])); e != nil {
			return in, e
		}
	}
	if !omit["final"] {
		if in.Final, e = vectorObservation(t, f, "fin"); e != nil {
			return in, e
		}
	}
	if !omit["declaration"] {
		if in.Declared, e = declaration(f["fin_at"], f["fin_entry"], f["fin_offset"]); e != nil {
			return in, e
		}
	}
	if raw, ok := f["fin_boot_hex"]; ok {
		c, e := vectorClock(t, raw, f["fin_mono"])
		if e != nil {
			return in, e
		}
		in.Clock = &c
	}
	return in, nil
}

func vectorFinish(t *testing.T, args fields) (trip.Trip, error) {
	t.Helper()
	f := finishFields(startFields(args))
	started, e := vectorStart(t, f)
	if e != nil {
		return trip.Trip{}, e
	}
	in, e := vectorFinishInput(t, f)
	if e != nil {
		return trip.Trip{}, e
	}
	before := tripFields(started)
	scope := vectorScope(f["fin_scope"])
	next, e := started.Finish(scope, in)
	if !reflect.DeepEqual(tripFields(started), before) {
		t.Fatal("finish changed its input trip")
	}
	if again := f.get("again", "false"); again != "true" && again != "false" {
		t.Fatal("malformed again fixture flag")
	} else if e == nil && again == "true" {
		next, e = next.Finish(scope, in)
	}
	return next, e
}

func evidenceField(o trip.MeterObservation) string {
	if r := o.Reading(); r != nil {
		v, _ := o.Value()
		return alias(r.ID()) + ":" + string(v.Kind()) + ":" + strconv.FormatInt(v.Count(), 10)
	}
	if a := o.Absence(); a != nil {
		return "ABSENT:" + hex.EncodeToString([]byte(a.Reason()))
	}
	return "?"
}

func tripFields(tr trip.Trip) fields {
	start, rep := tr.Started(), tr.Reporting()
	out := fields{"ready": "true", "org": alias(tr.OrganizationID().String()), "id": alias(tr.Reference().ID()),
		"asset": alias(tr.Asset().ID().Value()), "driver": alias(tr.Driver().ID()), "state": string(tr.State()),
		"start_author": alias(start.Author().ID()), "start_at": orgtime.FormatInstant(start.Declared().OccurredAt),
		"start_entry": start.Declared().EntryZone, "start_offset": strconv.FormatInt(start.Declared().DeviceOffsetSeconds, 10),
		"departure": evidenceField(start.Evidence()), "day": rep.Day, "zone": rep.Zone, "reporting_source": rep.Source,
		"finish_author": "?", "finish_at": "?", "finish_entry": "?", "finish_offset": "?", "final": "?"}
	if end, ok := tr.Finished(); ok {
		out["finish_author"] = alias(end.Author().ID())
		out["finish_at"] = orgtime.FormatInstant(end.Declared().OccurredAt)
		out["finish_entry"] = end.Declared().EntryZone
		out["finish_offset"] = strconv.FormatInt(end.Declared().DeviceOffsetSeconds, 10)
		out["final"] = evidenceField(end.Evidence())
	}
	usage := tr.MeterUsage()
	out["usage"], out["usage_reason"], out["usage_kind"], out["usage_delta"] = string(usage.Status()), "?", "?", "?"
	if usage.Reason() != "" {
		out["usage_reason"] = string(usage.Reason())
	}
	if delta, ok := usage.Delta(); ok {
		out["usage_kind"], out["usage_delta"] = string(usage.Kind()), strconv.FormatInt(delta, 10)
	}
	return out
}

func runVector(t *testing.T, op string, f fields) (fields, error) {
	t.Helper()
	switch op {
	case "ENUM":
		raw, e := decodeHex(f["value_hex"])
		if e != nil {
			t.Fatal(e)
		}
		switch f["kind"] {
		case "STATE":
			v, e := trip.ParseState(raw)
			if e != nil {
				return nil, e
			}
			return fields{"value": string(v), "terminal": strconv.FormatBool(v.Terminal())}, nil
		case "COMMAND":
			v, e := trip.ParseCommand(raw)
			return fields{"value": string(v)}, e
		case "REVIEW":
			v, e := trip.ParseReviewState(raw)
			return fields{"value": string(v)}, e
		}
		t.Fatal("unknown fixture enum namespace")
	case "TRANSITION":
		s, e := trip.ParseState(f["state"])
		if e != nil {
			return nil, e
		}
		c, e := trip.ParseCommand(f["command"])
		if e != nil {
			return nil, e
		}
		next, e := trip.Transition(s, c)
		return fields{"state": string(next)}, e
	case "STANDING":
		s, e := trip.ParseState(f["operational"])
		if e != nil {
			return nil, e
		}
		r, e := trip.ParseReviewState(f["review"])
		if e != nil {
			return nil, e
		}
		st, e := trip.NewStanding(s, r)
		if e != nil {
			return nil, e
		}
		return fields{"operational": string(st.Operational()), "review": string(st.Review()),
			"completed": strconv.FormatBool(st.Completed()), "approved": strconv.FormatBool(st.Approved())}, nil
	case "START":
		tr, e := vectorStart(t, startFields(f))
		if e != nil {
			return nil, e
		}
		return tripFields(tr), nil
	case "FINISH":
		tr, e := vectorFinish(t, f)
		if e != nil {
			return nil, e
		}
		return tripFields(tr), nil
	case "PAUSE", "RESUME", "CANCEL":
		return vectorCommand(t, trip.Command(op), f)
	case "HISTORY":
		g := startFields(f)
		tr, e := vectorStart(t, g)
		if e != nil {
			return nil, e
		}
		if tr, e = vectorHistory(t, tr, g); e != nil {
			return nil, e
		}
		out := tripFields(tr)
		out["revision"], out["steps"] = strconv.FormatInt(tr.Revision(), 10), "?"
		var steps []string
		for _, l := range tr.Lifecycle() {
			steps = append(steps, string(l.Command())+"/"+strconv.FormatInt(l.Revision(), 10))
		}
		if _, ok := tr.Finished(); ok {
			steps = append(steps, "FINISH/"+strconv.FormatInt(tr.Revision(), 10))
		}
		if len(steps) != 0 {
			out["steps"] = strings.Join(steps, ",")
		}
		return out, nil
	}
	t.Fatal("unknown operation")
	return nil, nil
}

// errorFields maps the trip vocabulary; fixture-built meter and fleet
// references keep their own INVALID/SCOPE meaning.
func errorFields(e error) fields {
	var incomplete *trip.IncompleteError
	if errors.As(e, &incomplete) {
		names := make([]string, 0, len(incomplete.Missing()))
		for _, m := range incomplete.Missing() {
			names = append(names, string(m))
		}
		return fields{"error": "INCOMPLETE", "missing": strings.Join(names, ",")}
	}
	for code, targets := range map[string][]error{"INVALID": {trip.ErrInvalid, meter.ErrInvalid, fleet.ErrInvalid},
		"SCOPE": {trip.ErrScope, meter.ErrScope, fleet.ErrScope}, "FORBIDDEN": {trip.ErrForbidden},
		"INVALID_TRANSITION": {trip.ErrInvalidTransition}, "UNSUPPORTED_COMMAND": {trip.ErrUnsupportedCommand},
		"FINISH_BEFORE_START": {trip.ErrFinishBeforeStart}, "CONFLICT": {trip.ErrConflict}, "BEFORE_START": {trip.ErrBeforeStart}} {
		for _, target := range targets {
			if errors.Is(e, target) {
				return fields{"error": code}
			}
		}
	}
	return fields{"error": "UNEXPECTED"}
}

func TestSharedTripVectors(t *testing.T) {
	raw, e := os.ReadFile(corpusPath)
	if e != nil {
		t.Fatal(e)
	}
	if fmt.Sprintf("%x", sha256.Sum256(raw)) != corpusHash {
		t.Fatal("shared corpus hash changed")
	}
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	scanner.Buffer(make([]byte, 4096), 1<<20)
	if !scanner.Scan() || scanner.Text() != "id|operation|args|expected" {
		t.Fatal("invalid shared corpus header")
	}
	counts, ids, codes, pairs := map[string]int{}, map[string]bool{}, map[string]bool{}, map[string]bool{}
	for scanner.Scan() {
		p := strings.Split(scanner.Text(), "|")
		if len(p) != 4 || strings.TrimSpace(p[0]) == "" || ids[p[0]] {
			t.Fatal("malformed/duplicate shared case")
		}
		if _, ok := wantCounts[p[1]]; !ok {
			t.Fatal("unknown shared operation")
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
		if p[1] == "ENUM" && validateEnumSelector(args["kind"]) != nil {
			t.Fatal("malformed fixture enum namespace")
		}
		for k := range args {
			if !strings.Contains(" "+allowedArgs[p[1]]+" ", " "+k+" ") {
				t.Fatal("unknown fixture argument", p[0], k)
			}
		}
		if code, ok := want["error"]; ok {
			_, listed := want["missing"]
			if len(want) != 1 && !(code == "INCOMPLETE" && listed && len(want) == 2) {
				t.Fatal("error result carries output", p[0])
			}
			codes[code] = true
		}
		if p[1] == "TRANSITION" {
			if _, e := trip.ParseState(args["state"]); e == nil {
				if _, e := trip.ParseCommand(args["command"]); e == nil {
					pairs[args["state"]+":"+args["command"]] = true
				}
			}
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
	if len(ids) != corpusSize || !reflect.DeepEqual(counts, wantCounts) {
		t.Fatal("incomplete corpus", len(ids), counts)
	}
	if len(pairs) != 35 {
		t.Fatal("the state and command table is not complete", len(pairs))
	}
	for _, code := range errorCodes {
		if !codes[code] {
			t.Fatal("error code without a shared vector", code)
		}
	}
	if len(codes) != len(errorCodes) {
		t.Fatal("undocumented error code in corpus", codes)
	}
}

// apply runs one lifecycle command on a trip.
func apply(tr trip.Trip, cmd trip.Command, scope kernel.OrgScope, in trip.LifecycleInput) (trip.Trip, error) {
	switch cmd {
	case trip.CommandPause:
		return tr.Pause(scope, in)
	case trip.CommandResume:
		return tr.Resume(scope, in)
	case trip.CommandCancel:
		return tr.Cancel(scope, in)
	}
	return trip.Trip{}, errMalformedFixture
}

// vectorHistory applies the fixture history steps (step i at 12:i0Z by M of
// the trip organization at the current revision; FINISH with its v1 defaults).
func vectorHistory(t *testing.T, tr trip.Trip, f fields) (trip.Trip, error) {
	t.Helper()
	raw, ok := f["history"]
	if !ok {
		return tr, nil
	}
	steps := strings.Split(raw, ",")
	if len(steps) > 9 {
		t.Fatal("history too long for the fixture clock")
	}
	scope := vectorScope(f["scope"])
	for i, step := range steps {
		var e error
		switch step {
		case "FINISH":
			g := fields{}
			for k, v := range f {
				g[k] = v
			}
			in, e := vectorFinishInput(t, finishFields(g))
			if e != nil {
				t.Fatal("fixture finish step", e)
			}
			tr, e = tr.Finish(scope, in)
			if e != nil {
				return trip.Trip{}, e
			}
			continue
		case "PAUSE", "RESUME", "CANCEL":
		default:
			t.Fatal("unknown history step", step)
		}
		author, e := trip.NewMembershipRef(vectorScope(f["org"]), fixtureID("M"))
		if e != nil {
			t.Fatal(e)
		}
		at, e := declaration(fmt.Sprintf("2026-11-14T12:%d0:00Z", i+1), "America/Cuiaba", "-14400")
		if e != nil {
			t.Fatal(e)
		}
		in := trip.LifecycleInput{Author: author, Expected: tr.Revision(), Declared: at}
		if step == "CANCEL" {
			reason := "Cliente cancelou"
			in.Reason = &reason
		}
		if tr, e = apply(tr, trip.Command(step), scope, in); e != nil {
			return trip.Trip{}, e
		}
	}
	return tr, nil
}

// vectorCommand builds the trip and its history, then applies the command.
func vectorCommand(t *testing.T, cmd trip.Command, args fields) (fields, error) {
	t.Helper()
	f := startFields(args)
	tr, e := vectorStart(t, f)
	if e != nil {
		return nil, e
	}
	if tr, e = vectorHistory(t, tr, f); e != nil {
		return nil, e
	}
	reason := "?"
	if cmd == trip.CommandCancel {
		reason = cancelReasonHex
	}
	g := fields{"cmd_scope": f["scope"], "by": "M", "by_org": f["org"], "expected": strconv.FormatInt(tr.Revision(), 10), "reason_hex": reason,
		"cmd_at": "2026-11-14T14:00:00Z", "cmd_entry": "America/Cuiaba", "cmd_offset": "-14400"}
	for k, v := range g {
		f[k] = f.get(k, v)
	}
	omit := omitted(t, f["cmd_omit"], commandOmittable)
	in := trip.LifecycleInput{}
	if in.Expected, e = strconv.ParseInt(f["expected"], 10, 64); e != nil {
		t.Fatal("malformed expected fixture revision")
	}
	if !omit["author"] {
		if in.Author, e = trip.NewMembershipRef(vectorScope(f["by_org"]), fixtureID(f["by"])); e != nil {
			return nil, e
		}
	}
	if raw := f["reason_hex"]; raw != "?" {
		text, e := decodeHex(raw)
		if e != nil {
			t.Fatal(e)
		}
		in.Reason = &text
	}
	if !omit["declaration"] {
		if in.Declared, e = declaration(f["cmd_at"], f["cmd_entry"], f["cmd_offset"]); e != nil {
			return nil, e
		}
	}
	scope := vectorScope(f["cmd_scope"])
	before := tripFields(tr)
	next, e := apply(tr, cmd, scope, in)
	if !reflect.DeepEqual(tripFields(tr), before) {
		t.Fatal("a lifecycle command changed its input trip")
	}
	if again := f.get("again", "false"); again != "true" && again != "false" {
		t.Fatal("malformed again fixture flag")
	} else if e == nil && again == "true" {
		next, e = apply(next, cmd, scope, in)
	}
	if e != nil {
		return nil, e
	}
	facts := next.Lifecycle()
	if len(facts) == 0 {
		t.Fatal("an accepted command left no lifecycle fact")
	}
	last := facts[len(facts)-1]
	out := tripFields(next)
	out["revision"], out["previous"] = strconv.FormatInt(next.Revision(), 10), string(last.From())
	out["command_author"], out["command_at"] = alias(last.Author().ID()), orgtime.FormatInstant(last.Declared().OccurredAt)
	out["command_entry"], out["command_offset"] = last.Declared().EntryZone, strconv.FormatInt(last.Declared().DeviceOffsetSeconds, 10)
	out["command_reason_hex"] = "?"
	if r := last.Reason(); r != nil {
		out["command_reason_hex"] = hex.EncodeToString([]byte(*r))
	}
	if last.Command() != cmd || last.To() != next.State() || last.Revision() != next.Revision() {
		t.Fatal("the last lifecycle fact is not the applied command")
	}
	return out, nil
}
