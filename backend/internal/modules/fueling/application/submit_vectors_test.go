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

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fueling/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
)

// EMP-FUEL-011 (B-BR-EMP-455, EDR-294): the Android producer's exact
// fueling.submit bytes. Every OK row must parse through the real protocol and
// intake parsers with exactly the row's fields; every refused row must be
// refused by the same parser with the same class.
const (
	submitCorpusPath = "../../../../../contracts/testdata/enterprise/fueling/fueling-submit-v1.psv"
	submitCorpusHash = "67068cfba629ff0fe6ec6299fb2c892bcc2c6e001424ebb0a012d6a1ecf5f0b8"
	submitAccount    = "7b0e2a54-0000-4000-8000-000000009501"
	submitOrigin     = "https://api.example.invalid"
	submitOrg        = "7b0e2a54-0000-4000-8000-000000009502"
)

type submitVector struct {
	id, outcome string
	args        map[string]string
	data        []byte
}

func loadSubmitVectors(t *testing.T) []submitVector {
	t.Helper()
	raw, err := os.ReadFile(submitCorpusPath)
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprintf("%x", sha256.Sum256(raw)) != submitCorpusHash {
		t.Fatal("fueling submit corpus hash changed")
	}
	lines := strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n")
	if lines[0] != "id|operation|args|expected|bytes_hex" {
		t.Fatal("fueling submit corpus header")
	}
	seen := map[string]bool{}
	var out []submitVector
	for _, line := range lines[1:] {
		cols := strings.Split(line, "|")
		if len(cols) != 5 || cols[0] == "" || seen[cols[0]] || cols[1] != "SUBMIT" {
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
		out = append(out, submitVector{id: cols[0], outcome: cols[3], args: args, data: data})
	}
	return out
}

func submitText(t *testing.T, v submitVector, key string) *string {
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

func optionalArg(v submitVector, key string) *string {
	value, ok := v.args[key]
	if !ok {
		return nil
	}
	return &value
}

func sameOptional(a, b *string) bool { return (a == nil) == (b == nil) && (a == nil || *a == *b) }

func int64Text(p *int64) *string {
	if p == nil {
		return nil
	}
	text := fmt.Sprint(*p)
	return &text
}

func TestSharedFuelingSubmitVectors(t *testing.T) {
	vectors := loadSubmitVectors(t)
	binding, err := protocol.NewBinding(submitAccount, submitOrigin, submitOrg)
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for _, v := range vectors {
		counts[v.outcome]++
	}
	want := map[string]int{"OK": 15, "MALFORMED": 39, "UNSUPPORTED": 5}
	if len(vectors) != 59 || fmt.Sprint(counts) != fmt.Sprint(want) {
		t.Fatalf("fueling submit corpus counts %v", counts)
	}
	for _, v := range vectors {
		t.Run(v.id, func(t *testing.T) {
			var op protocol.Operation
			if v.outcome == "OK" {
				op, err = protocol.ParseOperation(v.data, binding)
				if err != nil || !bytes.Equal(op.CanonicalBytes(), v.data) {
					t.Fatal("produced bytes are not canonical server operations", err)
				}
			} else {
				op, err = protocol.Seal(v.data, binding)
				if err != nil {
					t.Fatal("refusal rows must be well-formed envelopes", err)
				}
			}
			snapshot := op.Snapshot()
			if op.ID() != v.args["op"] || snapshot.MembershipID != "7b0e2a54-0000-4000-8000-000000009503" ||
				snapshot.EnrollmentID != "7b0e2a54-0000-4000-8000-000000009504" || len(snapshot.DependsOn) != 0 {
				t.Fatal("envelope identity")
			}
			command, err := app.ParseOperation(op)
			switch v.outcome {
			case "OK":
				if err != nil {
					t.Fatal("produced operation refused by the intake parser", err)
				}
				checkSubmitVector(t, v, command)
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

// checkSubmitVector proves every parsed field equals the row's arguments.
func checkSubmitVector(t *testing.T, v submitVector, command app.Command) {
	t.Helper()
	s, err := command.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	c := s.Candidate.Snapshot()
	amounts := c.Amounts.Snapshot()
	if s.FuelingID != v.args["fueling"] || s.AssetID != v.args["asset"] || s.ResponsibleID != v.args["responsible"] ||
		string(c.Quantity.Product().Code()) != v.args["product"] || string(c.Quantity.Product().Unit()) != v.args["unit"] ||
		fmt.Sprint(c.Quantity.Amount()) != v.args["quantity"] || fmt.Sprint(amounts.Paid) != v.args["paid"] ||
		!sameOptional(int64Text(amounts.Discount), optionalArg(v, "discount")) || !sameOptional(int64Text(amounts.UnitPrice), optionalArg(v, "price")) ||
		!sameOptional(amounts.SourcePriceText, submitText(t, v, "price_text")) || string(c.Fill) != v.args["fill"] ||
		string(c.Payer) != v.args["payer"] || string(c.Payment) != v.args["payment"] ||
		!sameOptional(s.MissingReason, submitText(t, v, "reason")) || !sameOptional(s.PlaceName, submitText(t, v, "place_name")) {
		t.Fatal("parsed fueling differs from the row")
	}
	if _, ok := v.args["meter"]; ok {
		r := s.Reading
		if r == nil || r.MeterID != v.args["meter"] || r.SegmentID != v.args["segment"] || r.ReadingID != v.args["reading"] ||
			string(r.Value.Kind()) != v.args["kind"] || fmt.Sprint(r.Value.Count()) != v.args["value"] {
			t.Fatal("parsed reading differs from the row")
		}
	} else if s.Reading != nil {
		t.Fatal("unexpected reading")
	}
}
