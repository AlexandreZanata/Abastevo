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
	"time"

	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	trip "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/domain"
)

const (
	missionCorpusPath = "../../../../../contracts/testdata/enterprise/trip/mission-v3.psv"
	missionCorpusHash = "6b29bcbf3359066c553e371e1c054d2dba56632d99a1b05cc258fad45e1ef18c"
)

var (
	missionCounts = map[string]int{"STATE": 20, "COMMAND": 15, "RESPONSE": 8, "TRANSITION": 62, "CREATE": 29, "RESTORE": 34, "ASSIGN": 23, "RESPOND": 32, "CANCEL": 15,
		"LOAD": 8, "PLAN_VALUE": 50, "PLAN": 21}
	// EMP-TRIP-023 (EDR-265): the mission table no longer answers
	// UNSUPPORTED_COMMAND; every refused pair is INVALID_TRANSITION.
	missionErrorCodes = []string{"INVALID", "SCOPE", "FORBIDDEN", "CONFLICT", "INVALID_TRANSITION"}
	missionStored     = "scope id id_org unit unit_org asset asset_org title_hex starts ends state revision assignee assignee_org member member_org answer answer_member answer_reason_hex"
	missionArgs       = map[string]string{"STATE": "value_hex", "COMMAND": "value_hex", "RESPONSE": "value_hex", "TRANSITION": "state command",
		"CREATE":     "scope id id_org unit unit_org asset asset_org title_hex starts ends omit",
		"RESTORE":    missionStored,
		"ASSIGN":     missionStored + " to to_org to_member to_member_org expected",
		"RESPOND":    missionStored + " by by_org kind_hex reason_hex expected",
		"CANCEL":     missionStored + " reason_hex expected",
		"LOAD":       "value_hex",
		"PLAN_VALUE": "scope " + missionPlanArgs,
		"PLAN":       missionStored + " " + missionPlanArgs + " expected"}
	missionPlanArgs = "dest_text_hex dest_unit dest_unit_org plan_reason_hex project project_org load cargo_hex passengers"
	missionAliases  = map[string]int{"MI": 91, "U": 95, "U2": 96, "CC": 97}
)

func missionID(raw string) string {
	if n, ok := missionAliases[raw]; ok {
		return uuid(n)
	}
	return fixtureID(raw)
}

func missionLabel(id string) string {
	for name, n := range missionAliases {
		if uuid(n) == id {
			return name
		}
	}
	return alias(id)
}

func missionInstant(t *testing.T, raw string) time.Time {
	t.Helper()
	at, e := orgtime.ParseInstant(raw)
	if e != nil {
		t.Fatal("malformed fixture instant", raw, e)
	}
	return at
}

// missionContent builds identity, unit, asset, title and window in that order;
// the first construction error is the result.
func missionContent(t *testing.T, f fields, omit map[string]bool) (trip.MissionInput, error) {
	t.Helper()
	var in trip.MissionInput
	var e error
	if !omit["id"] {
		if in.Reference, e = trip.NewMissionRef(vectorScope(f.get("id_org", "A")), missionID(f.get("id", "MI"))); e != nil {
			return in, e
		}
	}
	if !omit["unit"] {
		if in.Unit, e = trip.NewUnitRef(vectorScope(f.get("unit_org", "A")), missionID(f.get("unit", "U"))); e != nil {
			return in, e
		}
	}
	if raw := f.get("asset", "V"); raw != "?" {
		asset, e := trip.NewMissionAssetRef(vectorScope(f.get("asset_org", "A")), missionID(raw))
		if e != nil {
			return in, e
		}
		in.Asset = &asset
	}
	in.Title = mustHex(t, f.get("title_hex", hex.EncodeToString([]byte("Entrega no cliente"))))
	in.Window = trip.MissionWindow{From: missionInstant(t, f.get("starts", "2026-11-20T11:00:00Z")), To: missionInstant(t, f.get("ends", "2026-11-20T15:00:00Z"))}
	return in, nil
}

func missionMember(f fields, key, orgKey, fallback string) (*trip.MembershipRef, error) {
	raw := f.get(key, fallback)
	if raw == "?" {
		return nil, nil
	}
	org := "A"
	if orgKey != "" {
		org = f.get(orgKey, "A")
	}
	ref, e := trip.NewMembershipRef(vectorScope(org), missionID(raw))
	return &ref, e
}

// missionRestore restores the fixture's stored mission (B-BR-EMP-353) in the
// given scope; commands restore in organization A and act in `scope`.
func missionRestore(t *testing.T, f fields, scope string) (trip.Mission, error) {
	t.Helper()
	in, e := missionContent(t, f, nil)
	if e != nil {
		return trip.Mission{}, e
	}
	s := trip.MissionSnapshot{Reference: in.Reference, Unit: in.Unit, Asset: in.Asset, Title: in.Title, Window: in.Window, State: trip.MissionState(f.get("state", "ASSIGNED"))}
	revision, err := strconv.ParseInt(f.get("revision", "2"), 10, 64)
	if err != nil {
		t.Fatal("malformed fixture revision", err)
	}
	s.Revision = revision
	employee, member := f.get("assignee", "D"), f.get("member", "M")
	if employee != "?" || member != "?" {
		var a trip.MissionAssignee
		if employee != "?" {
			if a.Employee, e = trip.NewEmployeeRef(vectorScope(f.get("assignee_org", "A")), missionID(employee)); e != nil {
				return trip.Mission{}, e
			}
		}
		if member != "?" {
			if a.Member, e = trip.NewMembershipRef(vectorScope(f.get("member_org", "A")), missionID(member)); e != nil {
				return trip.Mission{}, e
			}
		}
		s.Assignee = &a
	}
	if kind := f.get("answer", "?"); kind != "?" {
		by, e := missionMember(f, "answer_member", "", "M")
		if e != nil {
			return trip.Mission{}, e
		}
		answer := trip.MissionAnswer{Kind: trip.MissionResponse(kind)}
		if by != nil {
			answer.Member = *by
		}
		if raw := f.get("answer_reason_hex", "?"); raw != "?" {
			reason := mustHex(t, raw)
			answer.Reason = &reason
		}
		s.Answer = &answer
	}
	return trip.RestoreMission(vectorScope(scope), s)
}

func missionFields(m trip.Mission) fields {
	s := m.Snapshot()
	out := fields{"id": missionLabel(s.Reference.ID()), "unit": missionLabel(s.Unit.ID()), "asset": "?", "title_hex": hex.EncodeToString([]byte(s.Title)),
		"starts": orgtime.FormatInstant(s.Window.From), "ends": orgtime.FormatInstant(s.Window.To), "state": string(s.State), "revision": strconv.FormatInt(s.Revision, 10),
		"assignee": "?", "member": "?", "answer": "?", "answer_member": "?", "answer_reason_hex": "?"}
	if s.Asset != nil {
		out["asset"] = missionLabel(s.Asset.ID())
	}
	if s.Assignee != nil {
		out["assignee"], out["member"] = missionLabel(s.Assignee.Employee.ID()), missionLabel(s.Assignee.Member.ID())
	}
	if s.Answer != nil {
		out["answer"], out["answer_member"] = string(s.Answer.Kind), missionLabel(s.Answer.Member.ID())
		if s.Answer.Reason != nil {
			out["answer_reason_hex"] = hex.EncodeToString([]byte(*s.Answer.Reason))
		}
	}
	return out
}

func expected(t *testing.T, f fields) int64 {
	t.Helper()
	v, e := strconv.ParseInt(f.get("expected", "2"), 10, 64)
	if e != nil {
		t.Fatal("malformed fixture expected revision", e)
	}
	return v
}

// missionCommand runs one command on the restored mission and asserts that
// the input mission never changes.
func missionCommand(t *testing.T, f fields, run func(trip.Mission) (trip.Mission, error)) (fields, error) {
	t.Helper()
	m, e := missionRestore(t, f, "A")
	if e != nil {
		t.Fatal("mission fixture did not restore", e)
	}
	before := missionFields(m)
	next, e := run(m)
	if !reflect.DeepEqual(missionFields(m), before) {
		t.Fatal("a mission command changed its input mission")
	}
	if e != nil {
		return nil, e
	}
	return missionFields(next), nil
}

// missionPlanInput builds the PLAN_VALUE arguments; a reference constructor
// error is the result, as in missionContent.
func missionPlanInput(t *testing.T, f fields) (trip.PlanInput, error) {
	t.Helper()
	in := trip.PlanInput{Reason: mustHex(t, f.get("plan_reason_hex", hex.EncodeToString([]byte("Entrega de peças de reposição")))), Load: f.get("load", "CARGO")}
	if raw := f.get("dest_text_hex", hex.EncodeToString([]byte("Centro de distribuição Norte"))); raw != "?" {
		v := mustHex(t, raw)
		in.DestinationText = &v
	}
	if raw := f.get("dest_unit", "?"); raw != "?" {
		unit, e := trip.NewUnitRef(vectorScope(f.get("dest_unit_org", "A")), missionID(raw))
		if e != nil {
			return in, e
		}
		in.DestinationUnit = &unit
	}
	if raw := f.get("project", "CC"); raw != "?" {
		centre, e := trip.NewCostCentreRef(vectorScope(f.get("project_org", "A")), missionID(raw))
		if e != nil {
			return in, e
		}
		in.CostCentre = &centre
	}
	if raw := f.get("cargo_hex", hex.EncodeToString([]byte("Peças de reposição"))); raw != "?" {
		v := mustHex(t, raw)
		in.CargoDescription = &v
	}
	if raw := f.get("passengers", "?"); raw != "?" {
		n, e := strconv.Atoi(raw)
		if e != nil {
			t.Fatal("malformed fixture passenger count", raw)
		}
		in.Passengers = &n
	}
	return in, nil
}

func missionPlanFields(p trip.Plan) fields {
	s := p.Snapshot()
	out := fields{"dest_kind": "TEXT", "dest_text_hex": "?", "dest_unit": "?", "plan_reason_hex": hex.EncodeToString([]byte(s.Reason)), "project": "?", "load": string(s.Load),
		"cargo_hex": "?", "passengers": "?"}
	if s.DestinationText != nil {
		out["dest_text_hex"] = hex.EncodeToString([]byte(*s.DestinationText))
	}
	if s.DestinationUnit != nil {
		out["dest_kind"], out["dest_unit"] = "UNIT", missionLabel(s.DestinationUnit.ID())
	}
	if s.CostCentre != nil {
		out["project"] = missionLabel(s.CostCentre.ID())
	}
	if s.CargoDescription != nil {
		out["cargo_hex"] = hex.EncodeToString([]byte(*s.CargoDescription))
	}
	if s.Passengers != nil {
		out["passengers"] = strconv.Itoa(*s.Passengers)
	}
	return out
}

func runMissionVector(t *testing.T, op string, f fields) (fields, error) {
	t.Helper()
	switch op {
	case "STATE":
		s, e := trip.ParseMissionState(mustHex(t, f["value_hex"]))
		if e != nil {
			return nil, e
		}
		return fields{"state": string(s), "terminal": strconv.FormatBool(s.Terminal())}, nil
	case "COMMAND":
		c, e := trip.ParseMissionCommand(mustHex(t, f["value_hex"]))
		if e != nil {
			return nil, e
		}
		return fields{"command": string(c)}, nil
	case "RESPONSE":
		r, e := trip.ParseMissionResponse(mustHex(t, f["value_hex"]))
		if e != nil {
			return nil, e
		}
		return fields{"response": string(r)}, nil
	case "TRANSITION":
		s, e := trip.ParseMissionState(f["state"])
		if e != nil {
			return nil, e
		}
		c, e := trip.ParseMissionCommand(f["command"])
		if e != nil {
			return nil, e
		}
		next, e := trip.MissionTransition(s, c)
		if e != nil {
			return nil, e
		}
		return fields{"state": string(next)}, nil
	case "CREATE":
		in, e := missionContent(t, f, omitted(t, f["omit"], map[string]bool{"id": true, "unit": true}))
		if e != nil {
			return nil, e
		}
		m, e := trip.NewMission(vectorScope(f.get("scope", "A")), in)
		if e != nil {
			return nil, e
		}
		return missionFields(m), nil
	case "RESTORE":
		m, e := missionRestore(t, f, f.get("scope", "A"))
		if e != nil {
			return nil, e
		}
		return missionFields(m), nil
	case "ASSIGN":
		return missionCommand(t, f, func(m trip.Mission) (trip.Mission, error) {
			var to trip.MissionAssignee
			var e error
			if raw := f.get("to", "D2"); raw != "?" {
				if to.Employee, e = trip.NewEmployeeRef(vectorScope(f.get("to_org", "A")), missionID(raw)); e != nil {
					return trip.Mission{}, e
				}
			}
			if raw := f.get("to_member", "M2"); raw != "?" {
				if to.Member, e = trip.NewMembershipRef(vectorScope(f.get("to_member_org", "A")), missionID(raw)); e != nil {
					return trip.Mission{}, e
				}
			}
			return m.Assign(vectorScope(f.get("scope", "A")), expected(t, f), to)
		})
	case "RESPOND":
		return missionCommand(t, f, func(m trip.Mission) (trip.Mission, error) {
			var by trip.MembershipRef
			if raw := f.get("by", "M"); raw != "?" {
				ref, e := trip.NewMembershipRef(vectorScope(f.get("by_org", "A")), missionID(raw))
				if e != nil {
					return trip.Mission{}, e
				}
				by = ref
			}
			var reason *string
			if raw := f.get("reason_hex", "?"); raw != "?" {
				v := mustHex(t, raw)
				reason = &v
			}
			return m.Respond(vectorScope(f.get("scope", "A")), expected(t, f), by, mustHex(t, f.get("kind_hex", hex.EncodeToString([]byte("ACCEPT")))), reason)
		})
	case "CANCEL":
		return missionCommand(t, f, func(m trip.Mission) (trip.Mission, error) {
			return m.Cancel(vectorScope(f.get("scope", "A")), expected(t, f), mustHex(t, f.get("reason_hex", hex.EncodeToString([]byte("Cliente cancelou")))))
		})
	case "LOAD":
		l, e := trip.ParseMissionLoad(mustHex(t, f["value_hex"]))
		if e != nil {
			return nil, e
		}
		return fields{"load": string(l)}, nil
	case "PLAN_VALUE":
		in, e := missionPlanInput(t, f)
		if e != nil {
			return nil, e
		}
		p, e := trip.NewPlan(vectorScope(f.get("scope", "A")), in)
		if e != nil {
			return nil, e
		}
		return missionPlanFields(p), nil
	case "PLAN":
		var plan trip.Plan
		out, e := missionCommand(t, f, func(m trip.Mission) (trip.Mission, error) {
			in, e := missionPlanInput(t, f)
			if e != nil {
				return trip.Mission{}, e
			}
			next, p, e := m.Plan(vectorScope(f.get("scope", "A")), expected(t, f), in)
			plan = p
			return next, e
		})
		if e != nil {
			return nil, e
		}
		for k, v := range missionPlanFields(plan) {
			out[k] = v
		}
		return out, nil
	}
	t.Fatal("unknown operation")
	return nil, nil
}

func missionErrorFields(e error) fields {
	for code, target := range map[string]error{"INVALID": trip.ErrInvalid, "SCOPE": trip.ErrScope, "FORBIDDEN": trip.ErrForbidden, "CONFLICT": trip.ErrConflict,
		"INVALID_TRANSITION": trip.ErrInvalidTransition, "UNSUPPORTED_COMMAND": trip.ErrUnsupportedCommand} {
		if errors.Is(e, target) {
			return fields{"error": code}
		}
	}
	return fields{"error": "UNEXPECTED"}
}

// EMP-TRIP-008/009/023: the shared mission-v3 oracle (superseding v2 and
// v1), consumed byte for byte by Kotlin.
func TestSharedTripMissionVectors(t *testing.T) {
	raw, e := os.ReadFile(missionCorpusPath)
	if e != nil {
		t.Fatal(e)
	}
	if fmt.Sprintf("%x", sha256.Sum256(raw)) != missionCorpusHash {
		t.Fatal("shared mission corpus hash changed")
	}
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	scanner.Buffer(make([]byte, 4096), 1<<20)
	if !scanner.Scan() || scanner.Text() != "id|operation|args|expected" {
		t.Fatal("mission corpus header")
	}
	ids, counts, codes := map[string]bool{}, map[string]int{}, map[string]bool{}
	pairs := map[string]bool{}
	enums := map[string]map[string]bool{"STATE": {}, "COMMAND": {}, "RESPONSE": {}, "LOAD": {}}
	type row struct{ id, op, args, expected string }
	var rows []row
	for scanner.Scan() {
		parts := strings.Split(scanner.Text(), "|")
		if len(parts) != 4 || parts[0] == "" || ids[parts[0]] {
			t.Fatal("malformed or repeated mission row", scanner.Text())
		}
		ids[parts[0]] = true
		counts[parts[1]]++
		args, e := fieldMap(parts[2])
		if e != nil {
			t.Fatal(parts[0], e)
		}
		allowed := strings.Fields(missionArgs[parts[1]])
		for key := range args {
			if !slices.Contains(allowed, key) {
				t.Fatal("unknown mission fixture argument", parts[0], key)
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
		if parts[1] == "TRANSITION" {
			pairs[args["state"]+"/"+args["command"]] = true
		}
		for _, key := range []string{"state", "command", "response", "load"} {
			if v, ok := want[key]; ok && enums[parts[1]] != nil {
				enums[parts[1]][v] = true
			}
		}
		rows = append(rows, row{parts[0], parts[1], parts[2], parts[3]})
	}
	if e := scanner.Err(); e != nil {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(counts, missionCounts) || len(rows) != 317 {
		t.Fatal("mission corpus counts", counts, len(rows))
	}
	for _, code := range missionErrorCodes {
		if !codes[code] {
			t.Fatal("error code without a shared vector", code)
		}
	}
	if len(codes) != len(missionErrorCodes) {
		t.Fatal("unexpected mission error code", codes)
	}
	// Domain coverage is reported without stopping the vectors.
	for _, s := range trip.MissionStates() {
		if !enums["STATE"][string(s)] {
			t.Error("mission state without a STATE vector", s)
		}
		for _, c := range trip.MissionCommands() {
			if !pairs[string(s)+"/"+string(c)] {
				t.Error("state/command pair without a TRANSITION vector", s, c)
			}
		}
	}
	for _, c := range trip.MissionCommands() {
		if !enums["COMMAND"][string(c)] {
			t.Error("mission command without a COMMAND vector", c)
		}
	}
	for _, r := range trip.MissionResponses() {
		if !enums["RESPONSE"][string(r)] {
			t.Error("mission response without a RESPONSE vector", r)
		}
	}
	for _, l := range trip.MissionLoads() {
		if !enums["LOAD"][string(l)] {
			t.Error("mission load without a LOAD vector", l)
		}
	}
	if len(enums["STATE"]) != 8 || len(enums["COMMAND"]) != 7 || len(enums["RESPONSE"]) != 2 || len(enums["LOAD"]) != 3 || len(trip.MissionLoads()) != 3 {
		t.Error("enum vectors name a token outside the domain vocabulary", enums)
	}
	for _, r := range rows {
		t.Run(r.id, func(t *testing.T) {
			args, _ := fieldMap(r.args)
			want, _ := fieldMap(r.expected)
			got, e := runMissionVector(t, r.op, args)
			if e != nil {
				got = missionErrorFields(e)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("got %v want %v", got, want)
			}
		})
	}
}
