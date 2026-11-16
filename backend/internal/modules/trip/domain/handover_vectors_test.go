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

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain/meter"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	trip "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/domain"
)

// EMP-TRIP-012 handover oracle (B-BR-EMP-394/395, EDR-254), shared with Kotlin.
const (
	handoverCorpusPath = "../../../../../contracts/testdata/enterprise/trip/handover-v1.psv"
	handoverCorpusHash = "5dd11b0510c63fc8081e6c7a4b743e2a5af580a03ec233092cc3573768bd456a"
	handoverCorpusSize = 87
)

var (
	handoverCounts     = map[string]int{"HANDOVER": 47, "RESPONSIBLE": 12, "ACT": 28}
	handoverErrorCodes = []string{"INVALID", "SCOPE", "INCOMPLETE", "FORBIDDEN", "INVALID_TRANSITION", "CONFLICT", "EVENT_BEFORE_START", "EVENT_AFTER_FINISH"}
	handoverKeys       = map[string]string{
		"HANDOVER":    "scope history recorder recorder_org receiver receiver_org receiver_employee receiver_employee_org expected reason_hex at omit again",
		"RESPONSIBLE": "scope history at",
		"ACT":         "scope history act by act_at expected",
	}
	handoverOmittable = map[string]bool{"recorder": true, "receiver": true, "receiver_employee": true, "reason": true, "at": true}
	handoverAliases   = map[string]int{"M3": 43, "D3": 33}
	handoverEmployee  = map[string]string{"M": "D", "M2": "D2", "M3": "D3"}
	handoverReason    = "Troca de turno"
)

func handoverID(raw string) string {
	if n, ok := handoverAliases[raw]; ok {
		return uuid(n)
	}
	return fixtureID(raw)
}

func handoverAlias(id string) string {
	for name, n := range handoverAliases {
		if uuid(n) == id {
			return name
		}
	}
	return alias(id)
}

func handoverInstant(raw string) (time.Time, error) {
	at, err := orgtime.ParseInstant(raw)
	if err != nil {
		return time.Time{}, trip.ErrInvalid
	}
	return at, nil
}

func handoverMember(t *testing.T, org, raw string) trip.MembershipRef {
	t.Helper()
	ref, err := trip.NewMembershipRef(vectorScope(org), handoverID(raw))
	if err != nil {
		t.Fatal("fixture membership", raw, err)
	}
	return ref
}

// handoverStep applies one fixture history step at 12:i0Z: PAUSE, RESUME,
// CANCEL and FINISH (a stated missing final reading) by the current
// responsible member; H:<member> a handover to that member and its employee
// recorded by M3 at the current revision.
func handoverStep(t *testing.T, tr trip.Trip, scope kernel.OrgScope, i int, step string) (trip.Trip, error) {
	t.Helper()
	at := fmt.Sprintf("2026-11-14T12:%d0:00Z", i+1)
	declared, err := declaration(at, "America/Cuiaba", "-14400")
	if err != nil {
		t.Fatal(err)
	}
	author := tr.Responsible()
	switch {
	case strings.HasPrefix(step, "H:"):
		member := strings.TrimPrefix(step, "H:")
		employee, ok := handoverEmployee[member]
		if !ok {
			t.Fatal("unknown handover fixture member", member)
		}
		receiver := handoverMember(t, "A", member)
		staff, err := trip.NewEmployeeRef(vectorScope("A"), handoverID(employee))
		if err != nil {
			t.Fatal(err)
		}
		reason := handoverReason
		instant, _ := handoverInstant(at)
		return tr.Handover(scope, trip.HandoverInput{Recorder: handoverMember(t, "A", "M3"), Receiver: receiver, ReceiverEmployee: staff,
			Expected: tr.Revision(), Reason: &reason, At: instant})
	case step == "FINISH":
		return tr.Finish(scope, trip.FinishInput{Author: author, Final: handoverAbsent(t), Declared: declared})
	case step == "PAUSE", step == "RESUME", step == "CANCEL":
		in := trip.LifecycleInput{Author: author, Expected: tr.Revision(), Declared: declared}
		if step == "CANCEL" {
			reason := "Cliente cancelou"
			in.Reason = &reason
		}
		return apply(tr, trip.Command(step), scope, in)
	}
	t.Fatal("unknown handover history step", step)
	return trip.Trip{}, nil
}

func handoverAbsent(t *testing.T) trip.MeterObservation {
	t.Helper()
	missing, err := meter.NewMissing("Sem leitura final")
	if err != nil {
		t.Fatal(err)
	}
	o, err := trip.AbsentReading(missing)
	if err != nil {
		t.Fatal(err)
	}
	return o
}

// handoverTrip starts the default trip-v2 trip and applies the history.
func handoverTrip(t *testing.T, f fields) (trip.Trip, error) {
	t.Helper()
	start := startFields(fields{})
	tr, err := vectorStart(t, start)
	if err != nil {
		t.Fatal("fixture start", err)
	}
	raw, ok := f["history"]
	if !ok {
		return tr, nil
	}
	steps := strings.Split(raw, ",")
	if len(steps) > 9 {
		t.Fatal("history too long for the fixture clock")
	}
	scope := vectorScope("A")
	for i, step := range steps {
		if tr, err = handoverStep(t, tr, scope, i, step); err != nil {
			return trip.Trip{}, err
		}
	}
	return tr, nil
}

func handoverFacts(tr trip.Trip) string {
	parts := []string{}
	for _, h := range tr.Handovers() {
		parts = append(parts, fmt.Sprintf("%d:%s>%s/%s@%s", h.Revision(), handoverAlias(h.From().ID()), handoverAlias(h.To().ID()),
			handoverAlias(h.ReceiverEmployee().ID()), orgtime.FormatInstant(h.At())))
	}
	if len(parts) == 0 {
		return "?"
	}
	return strings.Join(parts, ",")
}

func handoverResult(tr trip.Trip) fields {
	out := fields{"state": string(tr.State()), "revision": strconv.FormatInt(tr.Revision(), 10), "responsible": handoverAlias(tr.Responsible().ID()),
		"start_author": handoverAlias(tr.Started().Author().ID()), "driver": handoverAlias(tr.Driver().ID()), "handovers": handoverFacts(tr),
		"recorder": "?", "reason_hex": "?"}
	if hs := tr.Handovers(); len(hs) > 0 {
		last := hs[len(hs)-1]
		out["recorder"], out["reason_hex"] = handoverAlias(last.Recorder().ID()), hex.EncodeToString([]byte(last.Reason()))
	}
	return out
}

func runHandover(t *testing.T, f fields) (fields, error) {
	t.Helper()
	tr, err := handoverTrip(t, f)
	if err != nil {
		return nil, err
	}
	g := fields{"scope": "A", "recorder": "M3", "recorder_org": "A", "receiver": "M2", "receiver_org": "A", "receiver_employee": "D2",
		"receiver_employee_org": "A", "expected": strconv.FormatInt(tr.Revision(), 10), "reason_hex": hex.EncodeToString([]byte(handoverReason)),
		"at": "2026-11-14T13:00:00Z"}
	for k, v := range g {
		f[k] = f.get(k, v)
	}
	omit := omitted(t, f["omit"], handoverOmittable)
	var in trip.HandoverInput
	if in.Expected, err = strconv.ParseInt(f["expected"], 10, 64); err != nil {
		t.Fatal("malformed expected fixture revision")
	}
	if !omit["recorder"] {
		if in.Recorder, err = trip.NewMembershipRef(vectorScope(f["recorder_org"]), handoverID(f["recorder"])); err != nil {
			return nil, err
		}
	}
	if !omit["receiver"] {
		if in.Receiver, err = trip.NewMembershipRef(vectorScope(f["receiver_org"]), handoverID(f["receiver"])); err != nil {
			return nil, err
		}
	}
	if !omit["receiver_employee"] {
		if in.ReceiverEmployee, err = trip.NewEmployeeRef(vectorScope(f["receiver_employee_org"]), handoverID(f["receiver_employee"])); err != nil {
			return nil, err
		}
	}
	if !omit["reason"] {
		text, err := decodeHex(f["reason_hex"])
		if err != nil {
			t.Fatal(err)
		}
		in.Reason = &text
	}
	if !omit["at"] {
		at, err := handoverInstant(f["at"])
		if err != nil {
			return nil, err
		}
		in.At = at
	}
	scope := vectorScope(f["scope"])
	before := handoverResult(tr)
	next, err := tr.Handover(scope, in)
	if !reflect.DeepEqual(handoverResult(tr), before) {
		t.Fatal("a handover changed its input trip")
	}
	if again := f.get("again", "false"); again != "true" && again != "false" {
		t.Fatal("malformed again fixture flag")
	} else if err == nil && again == "true" {
		next, err = next.Handover(scope, in)
	}
	if err != nil {
		return nil, err
	}
	return handoverResult(next), nil
}

func runResponsible(t *testing.T, f fields) (fields, error) {
	t.Helper()
	tr, err := handoverTrip(t, f)
	if err != nil {
		return nil, err
	}
	if raw, ok := f["at"]; ok {
		at, err := handoverInstant(raw)
		if err != nil {
			t.Fatal("malformed responsible fixture instant")
		}
		return fields{"responsible": handoverAlias(tr.ResponsibleAt(at).ID())}, nil
	}
	return fields{"responsible": handoverAlias(tr.Responsible().ID())}, nil
}

func runAct(t *testing.T, f fields) (fields, error) {
	t.Helper()
	tr, err := handoverTrip(t, f)
	if err != nil {
		return nil, err
	}
	scope := vectorScope("A")
	by := handoverMember(t, "A", f["by"])
	declared, err := declaration(f.get("act_at", "2026-11-14T14:00:00Z"), "America/Cuiaba", "-14400")
	if err != nil {
		t.Fatal(err)
	}
	expected := tr.Revision()
	if raw, ok := f["expected"]; ok {
		if expected, err = strconv.ParseInt(raw, 10, 64); err != nil {
			t.Fatal("malformed expected fixture revision")
		}
	}
	var next trip.Trip
	switch f["act"] {
	case "PAUSE", "RESUME", "CANCEL":
		in := trip.LifecycleInput{Author: by, Expected: expected, Declared: declared}
		if f["act"] == "CANCEL" {
			reason := "Cliente cancelou"
			in.Reason = &reason
		}
		next, err = apply(tr, trip.Command(f["act"]), scope, in)
	case "FINISH":
		next, err = tr.Finish(scope, trip.FinishInput{Author: by, Final: handoverAbsent(t), Declared: declared})
	case "LOG":
		ref, e := trip.NewLogbookRef(scope, uuid(81))
		if e != nil {
			t.Fatal(e)
		}
		text := "Parada"
		entry, e := tr.Log(scope, trip.LogbookInput{Reference: ref, Author: by, Type: "NOTE", Text: &text, Declared: declared})
		if e != nil {
			return nil, e
		}
		return fields{"logged": "true", "author": handoverAlias(entry.Author().ID())}, nil
	default:
		t.Fatal("unknown act fixture", f["act"])
	}
	if err != nil {
		return nil, err
	}
	out := fields{"state": string(next.State()), "revision": strconv.FormatInt(next.Revision(), 10)}
	if end, ok := next.Finished(); ok {
		out["finish_author"] = handoverAlias(end.Author().ID())
	}
	return out, nil
}

func handoverErrorFields(e error) fields {
	var incomplete *trip.IncompleteError
	if errors.As(e, &incomplete) {
		names := make([]string, 0, len(incomplete.Missing()))
		for _, m := range incomplete.Missing() {
			names = append(names, string(m))
		}
		return fields{"error": "INCOMPLETE", "missing": strings.Join(names, ",")}
	}
	for code, target := range map[string]error{"INVALID": trip.ErrInvalid, "SCOPE": trip.ErrScope, "FORBIDDEN": trip.ErrForbidden,
		"INVALID_TRANSITION": trip.ErrInvalidTransition, "CONFLICT": trip.ErrConflict, "EVENT_BEFORE_START": trip.ErrEventBeforeStart,
		"EVENT_AFTER_FINISH": trip.ErrEventAfterFinish} {
		if errors.Is(e, target) {
			return fields{"error": code}
		}
	}
	return fields{"error": "UNEXPECTED"}
}

func TestSharedHandoverVectors(t *testing.T) {
	raw, err := os.ReadFile(handoverCorpusPath)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	if hex.EncodeToString(sum[:]) != handoverCorpusHash {
		t.Fatal("handover corpus hash drift", hex.EncodeToString(sum[:]))
	}
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	scanner.Buffer(make([]byte, 64*1024), 1<<20)
	if !scanner.Scan() || scanner.Text() != "id|operation|args|expected" {
		t.Fatal("handover corpus header drift")
	}
	counts, ids, codes := map[string]int{}, map[string]bool{}, map[string]bool{}
	for scanner.Scan() {
		parts := strings.Split(scanner.Text(), "|")
		if len(parts) != 4 || parts[0] == "" || ids[parts[0]] {
			t.Fatal("malformed or duplicate handover vector", scanner.Text())
		}
		ids[parts[0]] = true
		op := parts[1]
		allowed, ok := handoverKeys[op]
		if !ok {
			t.Fatal("unknown handover operation", op)
		}
		counts[op]++
		args, err := fieldMap(parts[2])
		want, err2 := fieldMap(parts[3])
		if err != nil || err2 != nil {
			t.Fatal("malformed handover vector fields", parts[0])
		}
		for k := range args {
			if !strings.Contains(" "+allowed+" ", " "+k+" ") {
				t.Fatal("unknown handover argument", parts[0], k)
			}
		}
		if code, ok := want["error"]; ok {
			codes[code] = true
		}
		t.Run(parts[0], func(t *testing.T) {
			var got fields
			var e error
			switch op {
			case "HANDOVER":
				got, e = runHandover(t, args)
			case "RESPONSIBLE":
				got, e = runResponsible(t, args)
			case "ACT":
				got, e = runAct(t, args)
			}
			if e != nil {
				got = handoverErrorFields(e)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("got %v want %v", got, want)
			}
		})
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if len(ids) != handoverCorpusSize || !reflect.DeepEqual(counts, handoverCounts) {
		t.Fatal("handover corpus count drift", len(ids), counts)
	}
	for _, code := range handoverErrorCodes {
		if !codes[code] {
			t.Fatal("handover error code without a vector", code)
		}
	}
	if len(codes) != len(handoverErrorCodes) {
		t.Fatal("undocumented handover error code", codes)
	}
}
