package application_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/application"
)

// EMP-TRIP-005 (B-BR-EMP-375, EDR-245): the Android producer's exact operation
// bytes, v2 with the EMP-TRIP-019 checklist-run references (B-BR-EMP-511,
// EDR-331; v1 is kept unconsumed). Every OK row must parse through the real protocol and intake parsers
// with exactly the row's fields; every refused row must be refused by the same
// parser with the same class.
const (
	operationsCorpusPath = "../../../../../contracts/testdata/enterprise/trip/trip-operations-v2.psv"
	operationsCorpusHash = "08e0c63235a470ce11756f9700f99d29ec640cf03e8803e3e845e6341dda04ef"
	vectorAccount        = "7b0e2a54-0000-4000-8000-000000009501"
	vectorOrigin         = "https://api.example.invalid"
	vectorOrg            = "7b0e2a54-0000-4000-8000-000000009502"
)

type operationVector struct {
	id, operation, outcome string
	args                   map[string]string
	data                   []byte
}

func loadOperationVectors(t *testing.T) []operationVector {
	t.Helper()
	raw, err := os.ReadFile(operationsCorpusPath)
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprintf("%x", sha256.Sum256(raw)) != operationsCorpusHash {
		t.Fatal("trip operation corpus hash changed")
	}
	lines := strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n")
	if lines[0] != "id|operation|args|expected|bytes_hex" {
		t.Fatal("trip operation corpus header")
	}
	seen := map[string]bool{}
	var out []operationVector
	for _, line := range lines[1:] {
		cols := strings.Split(line, "|")
		if len(cols) != 5 || cols[0] == "" || seen[cols[0]] {
			t.Fatal("malformed or duplicate row", line)
		}
		seen[cols[0]] = true
		args := map[string]string{}
		for _, field := range strings.Split(cols[2], ";") {
			key, value, ok := strings.Cut(field, "=")
			if !ok || key == "" {
				t.Fatal("malformed field", cols[0], field)
			}
			if _, dup := args[key]; dup {
				t.Fatal("duplicate field", cols[0], key)
			}
			args[key] = value
		}
		data, err := hex.DecodeString(cols[4])
		if err != nil || len(data) == 0 {
			t.Fatal("row bytes", cols[0], err)
		}
		out = append(out, operationVector{id: cols[0], operation: cols[1], outcome: cols[3], args: args, data: data})
	}
	return out
}

func vectorText(t *testing.T, v operationVector, key string) *string {
	t.Helper()
	raw, ok := v.args[key]
	if !ok {
		return nil
	}
	decoded, err := hex.DecodeString(raw)
	if err != nil {
		t.Fatal("text field", v.id, key)
	}
	text := string(decoded)
	return &text
}

func sameText(a, b *string) bool { return (a == nil) == (b == nil) && (a == nil || *a == *b) }

func TestSharedTripOperationVectors(t *testing.T) {
	vectors := loadOperationVectors(t)
	binding, err := protocol.NewBinding(vectorAccount, vectorOrigin, vectorOrg)
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for _, v := range vectors {
		counts[v.operation+"/"+v.outcome]++
	}
	want := map[string]int{"START/OK": 10, "START/MALFORMED": 17, "START/PROTOCOL": 2, "EVENT/OK": 7, "EVENT/MALFORMED": 7, "EVENT/UNSUPPORTED": 3,
		"FINISH/OK": 6, "FINISH/MALFORMED": 4, "FINISH/PROTOCOL": 1, "SUBMIT/OK": 2, "SUBMIT/MALFORMED": 1, "SUBMIT/PROTOCOL": 1}
	if len(vectors) != 61 || fmt.Sprint(counts) != fmt.Sprint(want) {
		t.Fatalf("trip operation corpus counts %v", counts)
	}
	for _, v := range vectors {
		t.Run(v.id, func(t *testing.T) {
			var op protocol.Operation
			if v.outcome == "OK" {
				op, err = protocol.ParseOperation(v.data, binding)
				if err != nil || !bytes.Equal(op.CanonicalBytes(), v.data) {
					t.Fatal("produced bytes are not canonical server operations", err)
				}
			} else if v.outcome == "PROTOCOL" {
				if _, err := protocol.Seal(v.data, binding); err == nil {
					t.Fatal("the envelope profile accepted a protocol refusal row")
				}
				return
			} else {
				op, err = protocol.Seal(v.data, binding)
				if err != nil {
					t.Fatal("refusal rows must be well-formed envelopes", err)
				}
			}
			snapshot := op.Snapshot()
			if op.ID() != v.args["op"] || snapshot.MembershipID != "7b0e2a54-0000-4000-8000-000000009503" || snapshot.EnrollmentID != "7b0e2a54-0000-4000-8000-000000009504" {
				t.Fatal("envelope identity")
			}
			deps := []string{}
			if dep, ok := v.args["dep"]; ok {
				deps = append(deps, dep)
			}
			if fmt.Sprint(snapshot.DependsOn) != fmt.Sprint(deps) {
				t.Fatal("dependencies", snapshot.DependsOn)
			}
			refs := []string{}
			for _, r := range snapshot.SoftReferences {
				refs = append(refs, r.Type+":"+r.ID)
			}
			want := []string{}
			if run, ok := v.args["run"]; ok {
				want = append(want, app.ChecklistRunReferenceType+":"+run)
			}
			if fmt.Sprint(refs) != fmt.Sprint(want) {
				t.Fatal("soft references", refs)
			}
			err := checkOperationVector(t, v, op)
			switch v.outcome {
			case "OK":
				if err != nil {
					t.Fatal("produced operation refused by the intake parser", err)
				}
			case "MALFORMED":
				if !errors.Is(err, app.ErrMalformed) {
					t.Fatal("expected MALFORMED", err)
				}
			case "UNSUPPORTED":
				if !errors.Is(err, app.ErrUnsupported) {
					t.Fatal("expected UNSUPPORTED", err)
				}
			default:
				t.Fatal("unknown outcome", v.outcome)
			}
		})
	}
}

// checkOperationVector parses the row with its real intake parser and, when
// accepted, proves every parsed field equals the row's arguments.
func checkOperationVector(t *testing.T, v operationVector, op protocol.Operation) error {
	t.Helper()
	readingMatches := func(r *app.Reading) bool {
		if _, ok := v.args["meter"]; !ok {
			return r == nil
		}
		return r != nil && r.MeterID == v.args["meter"] && r.SegmentID == v.args["segment"] && r.ReadingID == v.args["reading"] &&
			string(r.Value.Kind()) == v.args["kind"] && fmt.Sprint(r.Value.Count()) == v.args["value"]
	}
	clockMatches := func(e app.Event) bool {
		if _, ok := v.args["boot"]; !ok {
			return e.Clock == nil
		}
		return e.Clock != nil && e.Clock.BootID() == v.args["boot"] && fmt.Sprint(e.Clock.ElapsedMillis()) == v.args["elapsed"]
	}
	switch v.operation {
	case "START":
		command, err := app.ParseStart(op)
		if err != nil {
			return err
		}
		s, err := command.Snapshot()
		if err != nil || s.TripID != v.args["trip"] || s.AssetID != v.args["asset"] || s.DriverID != v.args["driver"] || s.DepartureRunID != v.args["run"] || s.MissionID != "" ||
			!readingMatches(s.Reading) || !sameText(s.MissingReason, vectorText(t, v, "reason")) || !clockMatches(s.Event) {
			t.Fatal("parsed start differs from the row")
		}
	case "EVENT":
		command, err := app.ParseEvent(op)
		if err != nil {
			return err
		}
		s, err := command.Snapshot()
		if err != nil || s.EventID != v.args["event"] || s.TripID != v.args["trip"] || string(s.Type) != v.args["type"] || !sameText(s.Text, vectorText(t, v, "text")) {
			t.Fatal("parsed event differs from the row")
		}
	case "FINISH":
		command, err := app.ParseFinish(op)
		if err != nil {
			return err
		}
		s, err := command.Snapshot()
		if err != nil || op.Snapshot().Entity.ID != v.args["trip"] || s.ReturnRunID != v.args["run"] || !readingMatches(s.Reading) || !sameText(s.MissingReason, vectorText(t, v, "reason")) || !clockMatches(s.Event) {
			t.Fatal("parsed finish differs from the row")
		}
	case "SUBMIT":
		command, err := app.ParseSubmit(op)
		if err != nil {
			return err
		}
		s, err := command.Snapshot()
		if err != nil || s.TripID != v.args["trip"] || fmt.Sprint(s.Revision) != v.args["revision"] {
			t.Fatal("parsed submission differs from the row")
		}
	default:
		t.Fatal("unknown operation", v.operation)
	}
	return nil
}
