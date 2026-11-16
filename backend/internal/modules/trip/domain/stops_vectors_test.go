package domain_test

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"

	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	trip "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/domain"
)

// EMP-TRIP-010: the shared stops-v1 oracle (planned stops and the observed
// stop and leg derivation), consumed byte for byte by Kotlin.
const (
	stopsCorpusPath = "../../../../../contracts/testdata/enterprise/trip/stops-v1.psv"
	stopsCorpusHash = "128d45271fef1b7f8b5c53003fdc96a74f6cd506d68f6869eea7d53d01105ae5"
)

var (
	stopsCounts     = map[string]int{"PLANNED": 21, "OBSERVE": 33}
	stopsErrorCodes = []string{"INVALID"}
	stopsArgs       = map[string]string{"PLANNED": "stops_hex", "OBSERVE": "start end facts"}
	stopsIssues     = []trip.StopIssue{trip.ArrivalMissing, trip.DepartureMissing}
	stopAliases     = map[string]int{"C": 91, "E1": 81, "E2": 82, "E3": 83, "E4": 84, "E5": 85, "E6": 86, "E7": 87, "E8": 88, "E9": 89}
)

func stopID(raw string) string {
	if n, ok := stopAliases[raw]; ok {
		return uuid(n)
	}
	return fixtureID(raw)
}

func stopAlias(id string) string {
	for name, n := range stopAliases {
		if uuid(n) == id {
			return name
		}
	}
	return alias(id)
}

func plannedFields(t *testing.T, raw string) (fields, error) {
	t.Helper()
	var places []string
	if raw != "?" {
		places = []string{}
		if raw != "" {
			for _, part := range strings.Split(raw, ",") {
				places = append(places, mustHex(t, part))
			}
		}
	}
	stops, e := trip.NewPlannedStops(places)
	if e != nil {
		return nil, e
	}
	if got := stops.Places(); stops.Len() != len(places) || !slices.Equal(got, places) {
		t.Fatal("planned stops changed their places", got, places)
	}
	if len(places) > 0 {
		places[0] = "changed by the caller"
		if stops.Places()[0] == places[0] {
			t.Fatal("planned stops share the caller's slice")
		}
	}
	point := func(p trip.PlannedPoint) string {
		if p.Kind == trip.PointStop {
			return "STOP" + strconv.Itoa(p.Stop)
		}
		return string(p.Kind)
	}
	legs := make([]string, 0)
	for i, leg := range stops.Legs() {
		if leg.Sequence != i+1 {
			t.Fatal("planned leg sequence", leg.Sequence, i+1)
		}
		legs = append(legs, point(leg.From)+">"+point(leg.To))
	}
	return fields{"count": strconv.Itoa(stops.Len()), "legs": strings.Join(legs, ",")}, nil
}

func stopFact(t *testing.T, raw string, parts int) trip.StopFact {
	t.Helper()
	p := strings.Split(raw, "/")
	if len(p) != parts {
		t.Fatal("malformed fixture stop fact", raw)
	}
	at, e := orgtime.ParseInstant(p[len(p)-1])
	if e != nil {
		t.Fatal("malformed fixture instant", e)
	}
	if parts == 2 {
		return trip.StopFact{Mark: trip.MarkStart, ID: stopID(p[0]), At: at}
	}
	return trip.StopFact{Mark: trip.StopMark(p[0]), ID: stopID(p[1]), At: at}
}

func boundary(f *trip.StopFact) string {
	if f == nil {
		return "-"
	}
	return string(f.Mark) + "/" + stopAlias(f.ID)
}

func issue(v trip.StopIssue) string {
	if v == "" {
		return "-"
	}
	return string(v)
}

func observeFields(t *testing.T, f fields) (fields, error) {
	t.Helper()
	start := stopFact(t, f["start"], 2)
	var end *trip.StopFact
	if raw, ok := f["end"]; ok {
		e := stopFact(t, raw, 3)
		end = &e
	}
	var facts []trip.StopFact
	if raw, ok := f["facts"]; ok {
		for _, part := range strings.Split(raw, ",") {
			facts = append(facts, stopFact(t, part, 3))
		}
	}
	got, e := trip.ObserveStops(start, end, facts)
	if e != nil {
		return nil, e
	}
	// The delivery order never changes the derivation (B-BR-EMP-324).
	reversed := slices.Clone(facts)
	slices.Reverse(reversed)
	again, e := trip.ObserveStops(start, end, reversed)
	if e != nil || !reflect.DeepEqual(got, again) {
		t.Fatal("observed stops depend on the delivery order", e)
	}
	stops := make([]string, 0, len(got.Stops))
	for i, s := range got.Stops {
		if s.Sequence != i+1 {
			t.Fatal("observed stop sequence", s.Sequence, i+1)
		}
		stops = append(stops, fmt.Sprintf("%d:%s:%s:%s", s.Sequence, boundary(s.Arrival), boundary(s.Departure), issue(s.Issue)))
	}
	legs := make([]string, 0, len(got.Legs))
	for i, l := range got.Legs {
		if l.Sequence != i+1 {
			t.Fatal("observed leg sequence", l.Sequence, i+1)
		}
		legs = append(legs, fmt.Sprintf("%d:%s:%s:%s", l.Sequence, boundary(l.Departure), boundary(l.Arrival), issue(l.Issue)))
	}
	after := make([]string, 0, len(got.AfterEnd))
	for i := range got.AfterEnd {
		after = append(after, boundary(&got.AfterEnd[i]))
	}
	return fields{"stops": strings.Join(stops, ","), "legs": strings.Join(legs, ","), "after_end": strings.Join(after, ",")}, nil
}

func stopsErrorFields(e error) fields {
	if errors.Is(e, trip.ErrInvalid) {
		return fields{"error": "INVALID"}
	}
	return fields{"error": "UNEXPECTED"}
}

func runStopsVector(t *testing.T, op string, f fields) (fields, error) {
	t.Helper()
	switch op {
	case "PLANNED":
		return plannedFields(t, f["stops_hex"])
	case "OBSERVE":
		return observeFields(t, f)
	}
	t.Fatal("unknown operation")
	return nil, nil
}

// EMP-TRIP-010: the shared stops-v1 oracle, consumed byte for byte by Kotlin.
func TestSharedTripStopsVectors(t *testing.T) {
	raw, e := os.ReadFile(stopsCorpusPath)
	if e != nil {
		t.Fatal(e)
	}
	if fmt.Sprintf("%x", sha256.Sum256(raw)) != stopsCorpusHash {
		t.Fatal("shared stops corpus hash changed")
	}
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	scanner.Buffer(make([]byte, 4096), 1<<20)
	if !scanner.Scan() || scanner.Text() != "id|operation|args|expected" {
		t.Fatal("stops corpus header")
	}
	ids, counts, codes, issues := map[string]bool{}, map[string]int{}, map[string]bool{}, map[string]bool{}
	type row struct{ id, op, args, expected string }
	var rows []row
	for scanner.Scan() {
		parts := strings.Split(scanner.Text(), "|")
		if len(parts) != 4 || parts[0] == "" || ids[parts[0]] {
			t.Fatal("malformed or repeated stops row", scanner.Text())
		}
		ids[parts[0]] = true
		counts[parts[1]]++
		args, e := fieldMap(parts[2])
		if e != nil {
			t.Fatal(parts[0], e)
		}
		allowed := strings.Fields(stopsArgs[parts[1]])
		for key := range args {
			if !slices.Contains(allowed, key) {
				t.Fatal("unknown stops fixture argument", parts[0], key)
			}
		}
		want, e := fieldMap(parts[3])
		if e != nil {
			t.Fatal(parts[0], e)
		}
		if code, ok := want["error"]; ok {
			if len(want) != 1 {
				t.Fatal("error rows carry only the code", parts[0])
			}
			codes[code] = true
		}
		for _, v := range stopsIssues {
			if strings.Contains(want["stops"]+want["legs"], string(v)) {
				issues[string(v)] = true
			}
		}
		rows = append(rows, row{parts[0], parts[1], parts[2], parts[3]})
	}
	if e := scanner.Err(); e != nil {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(counts, stopsCounts) || len(rows) != 54 {
		t.Fatal("stops corpus counts", counts, len(rows))
	}
	for _, code := range stopsErrorCodes {
		if !codes[code] {
			t.Fatal("error code without a shared vector", code)
		}
	}
	if len(codes) != len(stopsErrorCodes) {
		t.Fatal("unexpected stops error code", codes)
	}
	if len(issues) != len(stopsIssues) {
		t.Fatal("an issue without a shared vector", issues)
	}
	for _, r := range rows {
		t.Run(r.id, func(t *testing.T) {
			args, _ := fieldMap(r.args)
			want, _ := fieldMap(r.expected)
			got, e := runStopsVector(t, r.op, args)
			if e != nil {
				got = stopsErrorFields(e)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("got %v want %v", got, want)
			}
		})
	}
}
