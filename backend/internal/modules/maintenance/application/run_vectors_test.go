package application_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/maintenance/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/maintenance/domain/checklist"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
)

// EMP-CHECK-002 (B-BR-EMP-439): the shared check.run.complete oracle. The
// Kotlin builder produces the OK bytes; the Go parser must accept exactly
// them and refuse every other row with its class; PROTOCOL rows are refused
// by the envelope profile before any run parser.
const (
	runCorpusPath = "../../../../../contracts/testdata/enterprise/checklist/run-v1.psv"
	runCorpusHash = "fbbea99443fd5aadf49f7d55bf132b34208d2ee7e545188a078af56dff14a24e"
	runAccount    = "7b0e2a54-0000-4000-8000-000000009501"
	runOrigin     = "https://api.example.invalid"
	runOrg        = "7b0e2a54-0000-4000-8000-000000009502"
)

type runVector struct {
	id, outcome string
	args        map[string]string
	data        []byte
}

func loadRunVectors(t *testing.T) []runVector {
	t.Helper()
	raw, err := os.ReadFile(runCorpusPath)
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprintf("%x", sha256.Sum256(raw)) != runCorpusHash {
		t.Fatal("run corpus hash changed")
	}
	lines := strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n")
	if lines[0] != "id|operation|args|expected|bytes_hex" {
		t.Fatal("run corpus header")
	}
	seen := map[string]bool{}
	var out []runVector
	for _, line := range lines[1:] {
		cols := strings.Split(line, "|")
		if len(cols) != 5 || cols[0] == "" || seen[cols[0]] || cols[1] != "RUN" {
			t.Fatal("malformed, duplicate or unknown row", line)
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
		out = append(out, runVector{id: cols[0], outcome: cols[3], args: args, data: data})
	}
	return out
}

// expectedResponses decodes the answers argument: ANSWER[:hexnote][+photo].
func expectedResponses(t *testing.T, raw string) []checklist.Response {
	t.Helper()
	var out []checklist.Response
	for _, item := range strings.Split(raw, ",") {
		photo := strings.HasSuffix(item, "+photo")
		item = strings.TrimSuffix(item, "+photo")
		answer, note, _ := strings.Cut(item, ":")
		text, err := hex.DecodeString(note)
		if err != nil {
			t.Fatal("note", item)
		}
		out = append(out, checklist.Response{Answer: checklist.Answer(answer), Note: string(text), Photo: photo})
	}
	return out
}

func TestSharedRunOperationVectors(t *testing.T) {
	vectors := loadRunVectors(t)
	binding, err := protocol.NewBinding(runAccount, runOrigin, runOrg)
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for _, v := range vectors {
		counts[v.outcome]++
	}
	if len(vectors) != 56 || counts["OK"] != 15 || counts["MALFORMED"] != 37 || counts["UNSUPPORTED"] != 2 || counts["PROTOCOL"] != 2 || len(counts) != 4 {
		t.Fatalf("run corpus counts %v", counts)
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
			if op.ID() != v.args["op"] || op.Snapshot().Type != app.RunOperationType {
				t.Fatal("envelope identity")
			}
			command, err := app.ParseRun(op)
			switch v.outcome {
			case "OK":
				if err != nil {
					t.Fatal("produced operation refused by the intake parser", err)
				}
				in, err := command.Snapshot()
				if err != nil {
					t.Fatal(err)
				}
				version, _ := strconv.ParseInt(v.args["version"], 10, 64)
				policy, _ := strconv.ParseInt(v.args["policy"], 10, 64)
				want := expectedResponses(t, v.args["answers"])
				if in.RunID != v.args["run"] || in.AssetID != v.args["asset"] || string(in.Context) != v.args["context"] || in.TemplateID != v.args["template"] ||
					in.Version != version || in.PolicyRevision != policy || string(in.DeviceOutcome) != v.args["device"] || in.Acknowledged != (v.args["ack"] == "true") ||
					in.StartedAt.IsZero() || in.StartedAt.After(in.Declared.OccurredAt) || len(in.Responses) != len(want) || in.OperationID != op.ID() ||
					in.PayloadDigest != op.PayloadDigest() || in.AuthorMembershipID != op.Snapshot().MembershipID {
					t.Fatalf("parsed run differs: %+v", in)
				}
				for i := range want {
					if in.Responses[i].Answer != want[i].Answer || in.Responses[i].Note != want[i].Note || in.Responses[i].Photo != want[i].Photo {
						t.Fatal("answer", i)
					}
				}
			case "MALFORMED":
				if !errors.Is(err, app.ErrMalformed) {
					t.Fatal("expected MALFORMED", err)
				}
			case "UNSUPPORTED":
				if !errors.Is(err, app.ErrUnsupported) {
					t.Fatal("expected UNSUPPORTED", err)
				}
			}
		})
	}
}
