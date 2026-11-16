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
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/tracking/application"
)

// EMP-GPS-002 (B-BR-EMP-497, EDR-322): the Android tracking producer's exact
// gps.session.* bytes. Every OK row must parse through the real protocol and
// ParseSession with exactly the row's fields; every MALFORMED row must be a
// well-formed envelope that ParseSession refuses; every PROTOCOL row is
// refused by the shared envelope profile itself.
const (
	sessionCorpusPath = "../../../../../contracts/testdata/enterprise/tracking/session-operations-v1.psv"
	sessionCorpusHash = "e0ab1391496c811440e107e2f89fc5035798b566d8b52dc67c78b985c1181416"
	vectorAccount     = "7b0e2a54-0000-4000-8000-000000009501"
	vectorOrigin      = "https://api.example.invalid"
	vectorOrg         = "7b0e2a54-0000-4000-8000-000000009502"
	vectorMember      = "7b0e2a54-0000-4000-8000-000000009503"
	vectorEnrollment  = "7b0e2a54-0000-4000-8000-000000009504"
)

type sessionVector struct {
	id, operation, outcome string
	args                   map[string]string
	data                   []byte
}

func loadSessionVectors(t *testing.T) []sessionVector {
	t.Helper()
	raw, err := os.ReadFile(sessionCorpusPath)
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprintf("%x", sha256.Sum256(raw)) != sessionCorpusHash {
		t.Fatal("tracking session operation corpus hash changed")
	}
	lines := strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n")
	if lines[0] != "id|operation|args|expected|bytes_hex" {
		t.Fatal("tracking session operation corpus header")
	}
	seen := map[string]bool{}
	var out []sessionVector
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
		out = append(out, sessionVector{id: cols[0], operation: cols[1], outcome: cols[3], args: args, data: data})
	}
	return out
}

var sessionKinds = map[string]string{"START": app.StartOperationType, "RESUME": app.ResumeOperationType, "SUSPEND": app.SuspendOperationType,
	"INTERRUPT": app.InterruptOperationType, "REDUCE": app.ReduceOperationType, "END": app.EndOperationType}

func TestSharedTrackingSessionOperationVectors(t *testing.T) {
	vectors := loadSessionVectors(t)
	binding, err := protocol.NewBinding(vectorAccount, vectorOrigin, vectorOrg)
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for _, v := range vectors {
		counts[v.operation+"/"+v.outcome]++
	}
	want := map[string]int{"START/OK": 9, "START/MALFORMED": 8, "START/PROTOCOL": 1, "RESUME/OK": 4, "RESUME/MALFORMED": 1,
		"SUSPEND/OK": 2, "SUSPEND/MALFORMED": 1, "INTERRUPT/OK": 7, "INTERRUPT/MALFORMED": 2, "REDUCE/OK": 1, "REDUCE/MALFORMED": 1,
		"END/OK": 2, "END/MALFORMED": 1, "END/PROTOCOL": 1}
	if len(vectors) != 41 || fmt.Sprint(counts) != fmt.Sprint(want) {
		t.Fatalf("tracking session operation corpus counts %v", counts)
	}
	// Every session operation type is produced by an OK row.
	produced := map[string]bool{}
	for _, v := range vectors {
		if v.outcome == "OK" {
			produced[sessionKinds[v.operation]] = true
		}
	}
	for _, kind := range app.SessionOperationTypes() {
		if !produced[kind] {
			t.Fatal("no OK row for", kind)
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
			case "MALFORMED":
				op, err = protocol.Seal(v.data, binding)
				if err != nil {
					t.Fatal("refusal rows must be well-formed envelopes", err)
				}
			default:
				t.Fatal("unknown outcome", v.outcome)
			}
			snapshot := op.Snapshot()
			if op.ID() != v.args["op"] || snapshot.MembershipID != vectorMember || snapshot.EnrollmentID != vectorEnrollment ||
				snapshot.Type != sessionKinds[v.operation] || snapshot.Entity.Type != app.EntityType || snapshot.Entity.ID != v.args["session"] {
				t.Fatal("envelope identity")
			}
			deps := []string{}
			for _, key := range []string{"dep", "dep2"} {
				if dep, ok := v.args[key]; ok {
					deps = append(deps, dep)
				}
			}
			if fmt.Sprint(snapshot.DependsOn) != fmt.Sprint(sortedCopy(deps)) {
				t.Fatal("dependencies", snapshot.DependsOn)
			}
			command, err := app.ParseSession(op)
			if v.outcome == "MALFORMED" {
				if !errors.Is(err, app.ErrMalformed) {
					t.Fatal("expected MALFORMED", err)
				}
				return
			}
			if err != nil {
				t.Fatal("produced operation refused by the intake parser", err)
			}
			s, err := command.Snapshot()
			if err != nil || s.Type != sessionKinds[v.operation] || s.OperationID != v.args["op"] || s.SessionID != v.args["session"] ||
				s.TripID != v.args["trip"] || s.AuthorMembershipID != vectorMember || s.PayloadDigest != op.PayloadDigest() {
				t.Fatal("parsed session operation identity differs from the row")
			}
			at := "2026-11-15T12:00:00.123Z"
			if raw, ok := v.args["at"]; ok {
				at = raw
			}
			if want, err := orgtime.ParseInstant(at); err != nil || !s.Declared.Equal(want) {
				t.Fatal("declared instant", s.Declared, err)
			}
			if v.operation == "START" || v.operation == "RESUME" {
				if string(s.Origin) != v.args["origin"] || string(s.Permission) != v.args["permission"] ||
					fmt.Sprint(s.NotificationVisible) != v.args["visible"] || s.Cause != "" {
					t.Fatal("parsed driver action differs from the row")
				}
				return
			}
			if string(s.Cause) != v.args["cause"] || s.Origin != "" || s.Permission != "" {
				t.Fatal("parsed event differs from the row")
			}
		})
	}
}

func sortedCopy(values []string) []string {
	out := append([]string(nil), values...)
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}
