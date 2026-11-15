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
	"slices"
	"strconv"
	"strings"
	"testing"

	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	trip "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/domain"
)

const (
	logbookCorpusPath = "../../../../../contracts/testdata/enterprise/trip/logbook-v1.psv"
	logbookCorpusHash = "051b1a3a6947d8b7b620be5316ca6747b6f4fed86cde3ce7f7c9eb19171ef614"
)

var (
	logbookCounts     = map[string]int{"TYPE": 27, "RECORD": 66, "ORDER": 10}
	logbookErrorCodes = []string{"INVALID", "SCOPE", "FORBIDDEN", "UNSUPPORTED_EVENT", "EVENT_BEFORE_START", "EVENT_AFTER_FINISH"}
	logbookArgs       = map[string]string{"TYPE": "value_hex",
		"RECORD": "finished at fin_at ev_scope ev_id ev_id_org ev_author ev_author_org ev_type_hex ev_text_hex ev_at ev_entry ev_offset ev_omit",
		"ORDER":  "items"}
	logbookOmittable = map[string]bool{"id": true, "author": true, "declaration": true}
	eventAliases     = map[string]int{"E1": 81, "E2": 82, "E3": 83}
)

func logbookID(raw string) string {
	if n, ok := eventAliases[raw]; ok {
		return uuid(n)
	}
	return fixtureID(raw)
}

func logbookAlias(id string) string {
	for name, n := range eventAliases {
		if uuid(n) == id {
			return name
		}
	}
	return alias(id)
}

func mustHex(t *testing.T, raw string) string {
	t.Helper()
	v, e := decodeHex(raw)
	if e != nil {
		t.Fatal(e)
	}
	return v
}

// logbookTrip builds the trip-v1 START (and FINISH when finished=true) with
// only the start and finish instants overridable.
func logbookTrip(t *testing.T, f fields) (trip.Trip, error) {
	t.Helper()
	args := fields{}
	for _, key := range []string{"at", "fin_at"} {
		if v, ok := f[key]; ok {
			args[key] = v
		}
	}
	switch f.get("finished", "false") {
	case "true":
		return vectorFinish(t, args)
	case "false":
		return vectorStart(t, startFields(args))
	}
	t.Fatal("malformed finished fixture flag")
	return trip.Trip{}, nil
}

func recordFields(t *testing.T, f fields) (fields, error) {
	t.Helper()
	tr, e := logbookTrip(t, f)
	if e != nil {
		t.Fatal("logbook fixture trip", e)
	}
	before := tripFields(tr)
	omit := omitted(t, f["ev_omit"], logbookOmittable)
	var in trip.LogbookInput
	if !omit["id"] {
		if in.Reference, e = trip.NewLogbookRef(vectorScope(f.get("ev_id_org", "A")), logbookID(f.get("ev_id", "E1"))); e != nil {
			return nil, e
		}
	}
	if !omit["author"] {
		if in.Author, e = trip.NewMembershipRef(vectorScope(f.get("ev_author_org", "A")), fixtureID(f.get("ev_author", "M"))); e != nil {
			return nil, e
		}
	}
	in.Type = mustHex(t, f.get("ev_type_hex", hex.EncodeToString([]byte("NOTE"))))
	if raw := f.get("ev_text_hex", hex.EncodeToString([]byte("Cliente ausente"))); raw != "?" {
		text := mustHex(t, raw)
		in.Text = &text
	}
	if !omit["declaration"] {
		if in.Declared, e = declaration(f.get("ev_at", "2026-11-14T13:00:00Z"), f.get("ev_entry", "America/Cuiaba"), f.get("ev_offset", "-14400")); e != nil {
			t.Fatal("malformed fixture declaration", e)
		}
	}
	entry, e := tr.Log(vectorScope(f.get("ev_scope", "A")), in)
	if !reflect.DeepEqual(tripFields(tr), before) {
		t.Fatal("a logbook event changed its trip")
	}
	if e != nil {
		return nil, e
	}
	text := "?"
	if v := entry.Text(); v != nil {
		text = hex.EncodeToString([]byte(*v))
	}
	d := entry.Declared()
	return fields{"trip": alias(entry.Trip().ID()), "id": logbookAlias(entry.Reference().ID()), "type": string(entry.Type()), "text_hex": text,
		"author": alias(entry.Author().ID()), "at": orgtime.FormatInstant(d.OccurredAt), "entry": d.EntryZone,
		"offset": strconv.FormatInt(d.DeviceOffsetSeconds, 10), "trip_state": string(tr.State())}, nil
}

func orderFields(t *testing.T, raw string) fields {
	t.Helper()
	var items []trip.TimelineItem
	for _, part := range strings.Split(raw, ",") {
		p := strings.Split(part, "/")
		if len(p) != 3 || p[0] != "START" && p[0] != "EVENT" && p[0] != "FINISH" {
			t.Fatal("malformed fixture timeline item", part)
		}
		at, e := orgtime.ParseInstant(p[2])
		if e != nil {
			t.Fatal("malformed fixture instant", e)
		}
		items = append(items, trip.TimelineItem{Kind: trip.TimelineKind(p[0]), ID: logbookID(p[1]), At: at})
	}
	reversed := slices.Clone(items)
	slices.Reverse(reversed)
	trip.SortTimeline(items)
	trip.SortTimeline(reversed)
	if !slices.Equal(items, reversed) {
		t.Fatal("timeline order depends on delivery order")
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, string(item.Kind)+"/"+logbookAlias(item.ID))
	}
	return fields{"order": strings.Join(out, ",")}
}

func logbookErrorFields(e error) fields {
	for code, target := range map[string]error{"INVALID": trip.ErrInvalid, "SCOPE": trip.ErrScope, "FORBIDDEN": trip.ErrForbidden,
		"UNSUPPORTED_EVENT": trip.ErrUnsupportedEvent, "EVENT_BEFORE_START": trip.ErrEventBeforeStart, "EVENT_AFTER_FINISH": trip.ErrEventAfterFinish} {
		if errors.Is(e, target) {
			return fields{"error": code}
		}
	}
	return fields{"error": "UNEXPECTED"}
}

func runLogbookVector(t *testing.T, op string, f fields) (fields, error) {
	t.Helper()
	switch op {
	case "TYPE":
		v, e := trip.ParseLogbookType(mustHex(t, f["value_hex"]))
		if e != nil {
			return nil, e
		}
		status := "RESERVED"
		if v.Supported() {
			status = "SUPPORTED"
		}
		return fields{"type": string(v), "status": status}, nil
	case "RECORD":
		return recordFields(t, f)
	case "ORDER":
		return orderFields(t, f["items"]), nil
	}
	t.Fatal("unknown operation")
	return nil, nil
}

// EMP-TRIP-003: the shared logbook-v1 oracle, consumed byte for byte by Kotlin.
func TestSharedTripLogbookVectors(t *testing.T) {
	raw, e := os.ReadFile(logbookCorpusPath)
	if e != nil {
		t.Fatal(e)
	}
	if fmt.Sprintf("%x", sha256.Sum256(raw)) != logbookCorpusHash {
		t.Fatal("shared logbook corpus hash changed")
	}
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	scanner.Buffer(make([]byte, 4096), 1<<20)
	if !scanner.Scan() || scanner.Text() != "id|operation|args|expected" {
		t.Fatal("logbook corpus header")
	}
	ids, counts, codes, types := map[string]bool{}, map[string]int{}, map[string]bool{}, map[string]bool{}
	type row struct{ id, op, args, expected string }
	var rows []row
	for scanner.Scan() {
		parts := strings.Split(scanner.Text(), "|")
		if len(parts) != 4 || parts[0] == "" || ids[parts[0]] {
			t.Fatal("malformed or repeated logbook row", scanner.Text())
		}
		ids[parts[0]] = true
		counts[parts[1]]++
		args, e := fieldMap(parts[2])
		if e != nil {
			t.Fatal(parts[0], e)
		}
		allowed := strings.Fields(logbookArgs[parts[1]])
		for key := range args {
			if !slices.Contains(allowed, key) {
				t.Fatal("unknown logbook fixture argument", parts[0], key)
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
		if parts[1] == "TYPE" && want["type"] != "" {
			types[want["type"]] = true
		}
		rows = append(rows, row{parts[0], parts[1], parts[2], parts[3]})
	}
	if e := scanner.Err(); e != nil {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(counts, logbookCounts) || len(rows) != 103 {
		t.Fatal("logbook corpus counts", counts, len(rows))
	}
	for _, code := range logbookErrorCodes {
		if !codes[code] {
			t.Fatal("error code without a shared vector", code)
		}
	}
	if len(codes) != len(logbookErrorCodes) {
		t.Fatal("unexpected logbook error code", codes)
	}
	// Domain coverage is reported without stopping the vectors.
	for _, kind := range trip.LogbookTypes() {
		if !types[string(kind)] {
			t.Error("logbook type without a TYPE vector", kind)
		}
	}
	if len(types) != len(trip.LogbookTypes()) {
		t.Error("TYPE vectors name a type outside the domain vocabulary", len(types), len(trip.LogbookTypes()))
	}
	for _, r := range rows {
		t.Run(r.id, func(t *testing.T) {
			args, _ := fieldMap(r.args)
			want, _ := fieldMap(r.expected)
			got, e := runLogbookVector(t, r.op, args)
			if e != nil {
				got = logbookErrorFields(e)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("got %v want %v", got, want)
			}
		})
	}
}
