package meter_test

import (
	"bufio"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain/meter"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
)

type fields map[string]string

func (f fields) get(key, fallback string) string {
	if value, ok := f[key]; ok {
		return value
	}
	return fallback
}
func (f fields) clone() fields {
	out := fields{}
	for k, v := range f {
		out[k] = v
	}
	return out
}
func parseFields(t *testing.T, raw string) fields {
	t.Helper()
	out := fields{}
	for _, part := range strings.Split(raw, ";") {
		k, v, ok := strings.Cut(part, "=")
		if !ok || strings.TrimSpace(k) == "" {
			t.Fatal("malformed vector field")
		}
		if _, exists := out[k]; exists {
			t.Fatal("duplicate vector field")
		}
		out[k] = v
	}
	return out
}
func fixtureID(alias string) string {
	n, ok := map[string]int{"A": 1, "B": 2, "X": 11, "Y": 12, "M": 21, "N": 22, "S": 31, "T": 32, "E": 41, "F": 42, "D": 51, "Q": 52, "P": 61}[alias]
	if !ok && len(alias) > 1 {
		offset, found := map[byte]int{'O': 100, 'R': 200, 'F': 300}[alias[0]]
		if found {
			suffix, err := strconv.Atoi(alias[1:])
			if err == nil && suffix >= 0 {
				n, ok = offset+suffix, true
			}
		}
	}
	if ok {
		return fmt.Sprintf("8c000000-0000-4000-8000-%012d", n)
	}
	if alias == "?" {
		return ""
	}
	return alias
}
func fixtureAlias(id string) string {
	for _, alias := range []string{"A", "B", "X", "Y", "M", "N", "S", "T", "E", "F", "D", "Q", "P"} {
		if fixtureID(alias) == id {
			return alias
		}
	}
	for _, prefix := range []string{"O", "R", "F"} {
		for i := 0; i < 100; i++ {
			alias := prefix + strconv.Itoa(i)
			if fixtureID(alias) == id {
				return alias
			}
		}
	}
	return id
}
func vectorScope(alias string) (kernel.OrgScope, error) {
	id, err := kernel.ParseOrgID(fixtureID(alias))
	if err != nil {
		return kernel.OrgScope{}, meter.ErrScope
	}
	s, err := kernel.NewOrgScope(id)
	if err != nil {
		return kernel.OrgScope{}, meter.ErrScope
	}
	return s, nil
}
func decoded(raw string) (string, error) {
	b, err := hex.DecodeString(raw)
	if err != nil || raw != strings.ToLower(raw) || !utf8.Valid(b) {
		return "", meter.ErrInvalid
	}
	return string(b), nil
}
func canonicalInteger(raw string) (int64, error) {
	if raw == "" || len(raw) > 1 && raw[0] == '0' {
		return 0, meter.ErrInvalid
	}
	for i := 0; i < len(raw); i++ {
		if raw[i] < '0' || raw[i] > '9' {
			return 0, meter.ErrInvalid
		}
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, meter.ErrInvalid
	}
	return n, nil
}
func modulus(f fields) (*int64, error) {
	raw, ok := f["modulus"]
	if !ok || raw == "?" {
		return nil, nil
	}
	n, err := canonicalInteger(raw)
	if err != nil {
		return nil, err
	}
	return &n, nil
}
func vectorSegment(f fields) (kernel.OrgScope, meter.SegmentRef, error) {
	org := f.get("org", "A")
	s, err := vectorScope(org)
	if err != nil {
		return s, meter.SegmentRef{}, err
	}
	assetScope, err := vectorScope(f.get("asset_org", org))
	if err != nil {
		return s, meter.SegmentRef{}, err
	}
	assetID, err := fleet.ParseAssetID(fixtureID(f.get("asset", "X")))
	if err != nil {
		return s, meter.SegmentRef{}, meter.ErrInvalid
	}
	asset, err := fleet.NewAssetRef(assetScope, assetID)
	if err != nil {
		return s, meter.SegmentRef{}, meter.ErrScope
	}
	meterScope, err := vectorScope(f.get("meter_org", org))
	if err != nil {
		return s, meter.SegmentRef{}, err
	}
	mod, err := modulus(f)
	if err != nil {
		return s, meter.SegmentRef{}, err
	}
	m, err := meter.NewMeterRef(meterScope, asset, fixtureID(f.get("meter", "M")), meter.Kind(f.get("kind", "ODOMETER")), mod)
	if err != nil {
		return s, meter.SegmentRef{}, err
	}
	segmentScope, err := vectorScope(f.get("segment_org", org))
	if err != nil {
		return s, meter.SegmentRef{}, err
	}
	seg, err := meter.NewSegmentRef(segmentScope, m, fixtureID(f.get("segment", "S")))
	return s, seg, err
}
func vectorReading(f fields) (meter.Reading, error) {
	s, seg, err := vectorSegment(f)
	if err != nil {
		return meter.Reading{}, err
	}
	ref, err := meter.NewReadingRef(s, seg, fixtureID(f.get("id", "R1")))
	if err != nil {
		return meter.Reading{}, err
	}
	v, err := meter.ParseBase(meter.Kind(f.get("value_kind", string(seg.Meter().Kind()))), f.get("value", "0"), nil)
	if err != nil {
		return meter.Reading{}, err
	}
	memberScope, err := vectorScope(f.get("member_org", f.get("org", "A")))
	if err != nil {
		return meter.Reading{}, err
	}
	member, err := meter.NewMembershipRef(memberScope, fixtureID(f.get("member", "E")))
	if err != nil {
		return meter.Reading{}, err
	}
	deviceScope, err := vectorScope(f.get("device_org", f.get("org", "A")))
	if err != nil {
		return meter.Reading{}, err
	}
	device, err := meter.NewDeviceRef(deviceScope, fixtureID(f.get("device", "D")))
	if err != nil {
		return meter.Reading{}, err
	}
	opScope, err := vectorScope(f.get("operation_org", f.get("org", "A")))
	if err != nil {
		return meter.Reading{}, err
	}
	op, err := meter.NewOperationRef(opScope, fixtureID(f.get("operation", "O1")))
	if err != nil {
		return meter.Reading{}, err
	}
	var photo *meter.PhotoRef
	if f.get("photo", "?") != "?" {
		photoScope, e := vectorScope(f.get("photo_org", f.get("org", "A")))
		if e != nil {
			return meter.Reading{}, e
		}
		p, e := meter.NewPhotoRef(photoScope, fixtureID(f["photo"]))
		if e != nil {
			return meter.Reading{}, e
		}
		photo = &p
	}
	var declared *orgtime.Declaration
	if raw := f.get("occurred", "2026-11-02T00:00:00Z"); raw != "?" {
		at, e := orgtime.ParseInstant(raw)
		if e != nil {
			return meter.Reading{}, meter.ErrInvalid
		}
		offset, e := strconv.ParseInt(f.get("offset", "0"), 10, 64)
		if e != nil {
			return meter.Reading{}, meter.ErrInvalid
		}
		declared = &orgtime.Declaration{OccurredAt: at, EntryZone: f.get("entry", "UTC"), DeviceOffsetSeconds: offset}
	}
	received, err := orgtime.ParseInstant(f.get("received", "2026-11-03T00:00:00Z"))
	if err != nil {
		return meter.Reading{}, meter.ErrInvalid
	}
	return meter.NewReading(s, meter.ReadingInput{Reference: ref, Value: v, Declared: declared, ReceivedAt: received, Author: member, Device: device, Operation: op, Origin: meter.Origin(f.get("origin", "STANDALONE")), Method: meter.Method(f.get("method", "TYPED")), Photo: photo, State: meter.State(f.get("state", "VALID"))})
}
func prefixed(f fields, prefix string) fields {
	out := f.clone()
	for key, v := range f {
		if strings.HasPrefix(key, prefix) {
			out[strings.TrimPrefix(key, prefix)] = v
		}
	}
	return out
}
func vectorOpening(f fields) (meter.Reading, error) {
	out := f.clone()
	out["id"] = "R0"
	out["operation"] = "O0"
	out["value"] = "0"
	out["state"] = "VALID"
	out["occurred"] = "2026-11-01T00:00:00Z"
	out["received"] = "2026-11-01T00:01:00Z"
	return vectorReading(prefixed(out, "opening_"))
}
func tupleReadings(t *testing.T, f fields, key string) ([]meter.Reading, error) {
	t.Helper()
	if f[key] == "" {
		return nil, nil
	}
	var out []meter.Reading
	for _, tuple := range strings.Split(f[key], "~") {
		parts := strings.Split(tuple, ",")
		if len(parts) != 6 {
			t.Fatal("malformed reading tuple")
		}
		in := prefixed(f, "reading_")
		for i, k := range []string{"id", "operation", "value", "occurred", "received", "state"} {
			in[k] = parts[i]
		}
		r, err := vectorReading(in)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, nil
}
func vectorFact(f fields, id, op, reasonHex string) (meter.FactInput, error) {
	in := prefixed(f, "fact_")
	org := in.get("org", "A")
	opScope, err := vectorScope(org)
	if err != nil {
		return meter.FactInput{}, err
	}
	operation, err := meter.NewOperationRef(opScope, fixtureID(op))
	if err != nil {
		return meter.FactInput{}, err
	}
	ms, err := vectorScope(in.get("member_org", org))
	if err != nil {
		return meter.FactInput{}, err
	}
	member, err := meter.NewMembershipRef(ms, fixtureID(in.get("member", "E")))
	if err != nil {
		return meter.FactInput{}, err
	}
	ds, err := vectorScope(in.get("device_org", org))
	if err != nil {
		return meter.FactInput{}, err
	}
	device, err := meter.NewDeviceRef(ds, fixtureID(in.get("device", "D")))
	if err != nil {
		return meter.FactInput{}, err
	}
	received, err := orgtime.ParseInstant(in.get("received", "2026-11-14T00:00:00Z"))
	if err != nil {
		return meter.FactInput{}, meter.ErrInvalid
	}
	why, err := decoded(reasonHex)
	if err != nil {
		return meter.FactInput{}, err
	}
	return meter.FactInput{ID: fixtureID(id), Operation: operation, Author: member, Device: device, ReceivedAt: received, Reason: why}, nil
}
func vectorSeries(t *testing.T, f fields) (kernel.OrgScope, meter.Series, error) {
	t.Helper()
	s, seg, err := vectorSegment(f)
	if err != nil {
		return s, meter.Series{}, err
	}
	opening, err := vectorOpening(f)
	if err != nil {
		return s, meter.Series{}, err
	}
	series, err := meter.NewSeries(s, seg, opening)
	if err != nil {
		return s, meter.Series{}, err
	}
	rows, err := tupleReadings(t, f, "readings")
	if err != nil {
		return s, meter.Series{}, err
	}
	for _, row := range rows {
		series, err = series.Append(s, row)
		if err != nil {
			return s, meter.Series{}, err
		}
	}
	candidates, err := tupleReadings(t, f, "candidates")
	if err != nil {
		return s, meter.Series{}, err
	}
	candidateMap := map[string]meter.Reading{}
	for _, r := range candidates {
		candidateMap[r.Snapshot().Reference.ID()] = r
	}
	if f["actions"] != "" {
		for _, action := range strings.Split(f["actions"], "~") {
			parts := strings.Split(action, ",")
			if len(parts) < 5 {
				t.Fatal("malformed action")
			}
			expected := 5
			if parts[0] == "SUPERSEDE" {
				expected = 6
			} else if parts[0] != "VOID" {
				t.Fatal("unknown vector action")
			}
			if len(parts) != expected {
				t.Fatal("malformed action arity")
			}
			fact, er := vectorFact(f, parts[1], parts[2], parts[len(parts)-1])
			if er != nil {
				return s, meter.Series{}, er
			}
			_, targetSegment, er := vectorSegment(prefixed(f, "fact_"))
			if er != nil {
				return s, meter.Series{}, er
			}
			target, er := meter.NewReadingRef(s, targetSegment, fixtureID(parts[3]))
			if er != nil {
				return s, meter.Series{}, er
			}
			if parts[0] == "VOID" {
				series, err = series.Void(s, target, fact)
			} else {
				r, ok := candidateMap[fixtureID(parts[4])]
				if !ok {
					return s, meter.Series{}, meter.ErrInvalid
				}
				series, err = series.Supersede(s, target, r, fact)
			}
			if err != nil {
				return s, meter.Series{}, err
			}
		}
	}
	return s, series, nil
}
func valueMap(value meter.Value) fields {
	return fields{"kind": string(value.Kind()), "unit": value.Kind().Unit(), "value": strconv.FormatInt(value.Count(), 10)}
}
func readingMap(r meter.Reading) fields {
	in := r.Snapshot()
	seg := in.Reference.Segment()
	m := seg.Meter()
	photo := "?"
	if in.Photo != nil {
		photo = fixtureAlias(in.Photo.ID())
	}
	return fields{"org": fixtureAlias(m.Asset().OrganizationID().String()), "asset": fixtureAlias(m.Asset().ID().Value()), "meter": fixtureAlias(m.ID()), "segment": fixtureAlias(seg.ID()), "id": fixtureAlias(in.Reference.ID()), "member": fixtureAlias(in.Author.ID()), "device": fixtureAlias(in.Device.ID()), "operation": fixtureAlias(in.Operation.ID()), "photo": photo, "kind": string(in.Value.Kind()), "unit": in.Value.Kind().Unit(), "value": strconv.FormatInt(in.Value.Count(), 10), "state": string(in.State), "occurred": orgtime.FormatInstant(in.Declared.OccurredAt), "received": orgtime.FormatInstant(in.ReceivedAt), "entry": in.Declared.EntryZone, "offset": strconv.FormatInt(in.Declared.DeviceOffsetSeconds, 10), "origin": string(in.Origin), "method": string(in.Method)}
}
func runVector(t *testing.T, op string, f fields) (fields, error) {
	t.Helper()
	switch op {
	case "VALUE":
		mod, err := modulus(f)
		if err != nil {
			return nil, err
		}
		raw, err := decoded(f["text_hex"])
		if err != nil {
			return nil, err
		}
		kind := meter.Kind(f.get("kind", "ODOMETER"))
		minutes, err := decoded(f.get("minutes_hex", ""))
		if err != nil {
			return nil, err
		}
		v, err := meter.ParseEntry(kind, meter.EntryForm(f["form"]), raw, minutes, mod)
		return valueMap(v), err
	case "METER":
		s, seg, err := vectorSegment(f)
		if err != nil {
			return nil, err
		}
		r, err := vectorOpening(f)
		if err != nil {
			return nil, err
		}
		if _, err = meter.NewSeries(s, seg, r); err != nil {
			return nil, err
		}
		mod := "?"
		if n := seg.Meter().Modulus(); n != nil {
			mod = strconv.FormatInt(*n, 10)
		}
		return fields{"kind": string(seg.Meter().Kind()), "unit": seg.Meter().Kind().Unit(), "modulus": mod, "opening_value": strconv.FormatInt(r.Snapshot().Value.Count(), 10), "opening_state": string(r.Snapshot().State)}, nil
	case "OPEN_SET":
		s, err := vectorScope(f.get("request_org", f.get("org", "A")))
		if err != nil {
			return nil, err
		}
		var meters []meter.MeterRef
		var kinds []string
		if f["meters"] != "" {
			for _, tuple := range strings.Split(f["meters"], "~") {
				p := strings.Split(tuple, ",")
				if len(p) != 3 {
					t.Fatal("malformed open-set tuple")
				}
				in := f.clone()
				in["meter"], in["asset"], in["kind"] = p[0], p[1], p[2]
				_, seg, e := vectorSegment(in)
				if e != nil {
					return nil, e
				}
				meters = append(meters, seg.Meter())
				kinds = append(kinds, fixtureAlias(seg.Meter().Asset().ID().Value())+":"+string(seg.Meter().Kind()))
			}
		}
		if err = meter.ValidateOpenSet(s, meters); err != nil {
			return nil, err
		}
		sort.Strings(kinds)
		return fields{"count": strconv.Itoa(len(meters)), "kinds": strings.Join(kinds, ",")}, nil
	case "READING":
		r, err := vectorReading(f)
		if err != nil {
			return nil, err
		}
		return readingMap(r), nil
	case "PRESENCE":
		kind := meter.Kind(f.get("kind", "ODOMETER"))
		if kind.Unit() == "" {
			return nil, meter.ErrInvalid
		}
		if f["value"] == "?" {
			why, err := decoded(f.get("reason_hex", ""))
			if err != nil {
				return nil, err
			}
			missing, err := meter.NewMissing(why)
			if err != nil {
				return nil, err
			}
			return fields{"kind": string(kind), "presence": "MISSING", "value": "?", "reason_hex": hex.EncodeToString([]byte(missing.Reason()))}, nil
		}
		v, err := meter.ParseBase(kind, f["value"], nil)
		if err != nil {
			return nil, err
		}
		return fields{"kind": string(kind), "presence": "OBSERVED", "value": strconv.FormatInt(v.Count(), 10), "reason_hex": ""}, nil
	case "HISTORY", "USAGE":
		s, series, err := vectorSeries(t, f)
		if err != nil {
			return nil, err
		}
		if op == "USAGE" {
			start, e := orgtime.ParseInstant(f["start"])
			if e != nil {
				return nil, meter.ErrInvalid
			}
			end, e := orgtime.ParseInstant(f["end"])
			if e != nil {
				return nil, meter.ErrInvalid
			}
			u, e := series.Usage(s, start, end)
			if e != nil {
				return nil, e
			}
			status, delta := "INSUFFICIENT_DATA", "?"
			if n, ok := u.Delta(); ok {
				status = "OK"
				delta = strconv.FormatInt(n, 10)
			}
			from, to := "?", "?"
			if r := u.From(); r != nil {
				from = fixtureAlias(r.Snapshot().Reference.ID())
			}
			if r := u.To(); r != nil {
				to = fixtureAlias(r.Snapshot().Reference.ID())
			}
			return fields{"kind": string(u.Kind()), "unit": u.Kind().Unit(), "status": status, "usage": delta, "from": from, "to": to, "reason": u.Reason()}, nil
		}
		at, e := orgtime.ParseInstant(f.get("at", "2026-11-30T00:00:00Z"))
		if e != nil {
			return nil, meter.ErrInvalid
		}
		ordered, e := series.Ordered(s)
		if e != nil {
			return nil, e
		}
		var order, states, values []string
		for _, r := range ordered {
			snapshot := r.Snapshot()
			alias := fixtureAlias(snapshot.Reference.ID())
			state, er := series.State(s, snapshot.Reference)
			if er != nil {
				return nil, er
			}
			order = append(order, alias)
			states = append(states, alias+":"+string(state))
			values = append(values, alias+":"+strconv.FormatInt(snapshot.Value.Count(), 10))
		}
		latest, e := series.Latest(s, at)
		if e != nil {
			return nil, e
		}
		last, value := "?", "?"
		if latest != nil {
			last = fixtureAlias(latest.Snapshot().Reference.ID())
			value = strconv.FormatInt(latest.Snapshot().Value.Count(), 10)
		}
		facts, e := series.Facts(s)
		if e != nil {
			return nil, e
		}
		return fields{"order": strings.Join(order, ","), "states": strings.Join(states, ","), "values": strings.Join(values, ","), "latest": last, "latest_value": value, "count": strconv.Itoa(len(ordered)), "facts": strconv.Itoa(len(facts))}, nil
	}
	t.Fatal("unknown vector operation")
	return nil, nil
}
func errorFields(err error) fields {
	code := ""
	switch {
	case errors.Is(err, meter.ErrInvalid):
		code = "INVALID"
	case errors.Is(err, meter.ErrScope):
		code = "SCOPE"
	case errors.Is(err, meter.ErrAlreadySuperseded):
		code = "READING_ALREADY_SUPERSEDED"
	case errors.Is(err, meter.ErrOperationReused):
		code = "OPERATION_ID_REUSED"
	case errors.Is(err, meter.ErrAlreadyInService):
		code = "METER_ALREADY_IN_SERVICE"
	default:
		code = "UNEXPECTED"
	}
	return fields{"error": code}
}

func TestSharedMeterVectors(t *testing.T) {
	file, err := os.Open("../../../../../../contracts/testdata/enterprise/meter/meter-v1.psv")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 4096), 1<<20)
	if !scanner.Scan() || scanner.Text() != "id|operation|args|expected" {
		t.Fatal("wrong vector header")
	}
	counts := map[string]int{}
	expectedCounts := map[string]int{"VALUE": 99, "METER": 14, "OPEN_SET": 8, "READING": 54, "PRESENCE": 11, "HISTORY": 56, "USAGE": 19}
	ids := map[string]bool{}
	total := 0
	allowed := map[string]string{"VALUE": "kind form minutes_hex modulus text_hex", "METER": "org asset asset_org meter meter_org segment segment_org kind modulus opening_id opening_operation opening_value opening_value_kind opening_state opening_occurred opening_received opening_member opening_device opening_entry opening_offset opening_origin opening_method", "OPEN_SET": "org request_org meters", "READING": "org asset asset_org meter meter_org segment segment_org id member member_org device device_org operation operation_org photo photo_org kind value_kind value state occurred received entry offset origin method modulus", "PRESENCE": "kind value reason_hex", "HISTORY": "org kind readings candidates actions at opening_value opening_state reading_org reading_asset reading_meter reading_segment reading_kind fact_org fact_member_org fact_device_org fact_meter fact_segment", "USAGE": "org kind readings candidates actions start end opening_value opening_state reading_org reading_asset reading_meter reading_segment reading_kind fact_org fact_member_org fact_device_org fact_meter fact_segment"}
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Split(line, "|")
		if len(parts) != 4 || strings.TrimSpace(parts[0]) == "" || ids[parts[0]] {
			t.Fatal("malformed or duplicate vector")
		}
		ids[parts[0]] = true
		operation := parts[1]
		if _, known := expectedCounts[operation]; !known {
			t.Fatal("unknown operation")
		}
		counts[operation]++
		total++
		args, want := parseFields(t, parts[2]), parseFields(t, parts[3])
		for k := range args {
			if !strings.Contains(" "+allowed[operation]+" ", " "+k+" ") {
				t.Fatal("unknown vector argument")
			}
		}
		t.Run(parts[0], func(t *testing.T) {
			got, e := runVector(t, operation, args)
			if e != nil {
				got = errorFields(e)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("vector mismatch: got %v, expected %v", got, want)
			}
		})
	}
	if err = scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if total != 261 || !reflect.DeepEqual(counts, expectedCounts) {
		t.Fatalf("incomplete corpus: %d %v", total, counts)
	}
}
