package application_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/expense/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
)

// EMP-EXP-002 (B-BR-EMP-447, EDR-289): the Android producer's exact
// expense.submit bytes. Every OK row parses through the real protocol and
// intake parser with exactly the row's fields; every refused row is refused
// by the same parser with the same class.
const (
	submitCorpusPath = "../../../../../contracts/testdata/enterprise/expense/expense-submit-v1.psv"
	submitCorpusHash = "5c69de3c71bdeb9495ddef9ae9a39ae6597f0e791cdf1500abebdffc100de6e6"
	vectorAccount    = "9e5b0000-0000-4000-8000-000000009501"
	vectorOrigin     = "https://api.example.invalid"
	vectorOrg        = "9e5b0000-0000-4000-8000-000000009502"
	vectorMember     = "9e5b0000-0000-4000-8000-000000009503"
	vectorEnrollment = "9e5b0000-0000-4000-8000-000000009504"
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
		t.Fatal("expense submit corpus hash changed")
	}
	lines := strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n")
	if lines[0] != "id|operation|args|expected|bytes_hex" {
		t.Fatal("expense submit corpus header")
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
			if !ok || key == "" || !slices.Contains([]string{"op", "expense", "asset", "category", "paid", "company", "payer", "trip", "at", "zone", "offset"}, key) {
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

func TestSharedExpenseSubmitVectors(t *testing.T) {
	vectors := loadSubmitVectors(t)
	binding, err := protocol.NewBinding(vectorAccount, vectorOrigin, vectorOrg)
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	categories := map[string]bool{}
	for _, v := range vectors {
		counts[v.outcome]++
		if v.outcome == "OK" {
			categories[v.args["category"]] = true
		}
	}
	if len(vectors) != 40 || fmt.Sprint(counts) != fmt.Sprint(map[string]int{"MALFORMED": 28, "OK": 7, "PROTOCOL": 1, "UNSUPPORTED": 4}) {
		t.Fatalf("expense submit corpus counts %v", counts)
	}
	// Every v1 direct category is produced by an accepted row.
	for _, category := range app.DirectCategories() {
		if !categories[category] {
			t.Fatal("direct category without an accepted vector", category)
		}
	}
	for _, v := range vectors {
		t.Run(v.id, func(t *testing.T) {
			var op protocol.Operation
			switch v.outcome {
			case "OK":
				op, err = protocol.ParseOperation(v.data, binding)
				if err != nil || !bytes.Equal(op.CanonicalBytes(), v.data) {
					t.Fatal("produced bytes are not canonical server operations", err)
				}
			case "PROTOCOL":
				if _, err := protocol.Seal(v.data, binding); err == nil {
					t.Fatal("the envelope profile accepted a protocol refusal row")
				}
				return
			default:
				op, err = protocol.Seal(v.data, binding)
				if err != nil {
					t.Fatal("refusal rows must be well-formed envelopes", err)
				}
			}
			snapshot := op.Snapshot()
			if op.ID() != v.args["op"] || snapshot.MembershipID != vectorMember || snapshot.EnrollmentID != vectorEnrollment || snapshot.Entity.ID != v.args["expense"] {
				t.Fatal("envelope identity")
			}
			command, err := app.ParseOperation(op)
			switch v.outcome {
			case "OK":
				if err != nil {
					t.Fatal("produced operation refused by the intake parser", err)
				}
				s, err := command.Snapshot()
				at := v.args["at"]
				if at == "" {
					at = "2026-11-15T12:00:00.123Z"
				}
				instant, perr := orgtime.ParseInstant(at)
				if err != nil || perr != nil || s.OperationID != v.args["op"] || s.PayloadDigest != op.PayloadDigest() || s.ExpenseID != v.args["expense"] || s.AssetID != v.args["asset"] ||
					s.AuthorMembershipID != vectorMember || s.Category != v.args["category"] || fmt.Sprint(s.Paid) != v.args["paid"] || fmt.Sprint(s.Company) != v.args["company"] ||
					s.Payer != v.args["payer"] || !s.Declared.OccurredAt.Equal(instant) {
					t.Fatal("parsed expense differs from the row", err)
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
