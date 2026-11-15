package application_test

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
)

const (
	lateCorpusPath = "../../../../../contracts/testdata/enterprise/sync/late-intake-v1.psv"
	lateCorpusHash = "feb23b2d4c147569ac49977e060b5fdb110c3817cf54c457bb7652e244214dc8"
)

// LateVector is one parsed row of the shared late-intake oracle (EDR-197),
// consumed by this pure test and by the PostgreSQL function's PostGIS test.
type lateVector struct {
	id, operation          string
	end                    app.LateEnd
	ended, declared, recvd string
	claimed                string
	window                 int
	revision               int64
	expected               map[string]string
}

func pairs(raw string) (map[string]string, bool) {
	out := map[string]string{}
	for _, part := range strings.Split(raw, ";") {
		key, value, ok := strings.Cut(part, "=")
		if !ok || key == "" {
			return nil, false
		}
		if _, dup := out[key]; dup {
			return nil, false
		}
		out[key] = value
	}
	return out, true
}

func loadLateVectors(t *testing.T) []lateVector {
	t.Helper()
	raw, err := os.ReadFile(lateCorpusPath)
	if err != nil {
		t.Fatal(err)
	}
	if sum := sha256.Sum256(raw); hex.EncodeToString(sum[:]) != lateCorpusHash {
		t.Fatal("late-intake oracle changed without updating its pinned hash")
	}
	lines := strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n")
	if lines[0] != "id|operation|args|expected" {
		t.Fatal("late-intake oracle header")
	}
	seen := map[string]bool{}
	var out []lateVector
	for _, line := range lines[1:] {
		cols := strings.Split(line, "|")
		if len(cols) != 4 || cols[0] == "" || seen[cols[0]] || cols[1] != "DECIDE" && cols[1] != "EVIDENCE" {
			t.Fatal("late-intake oracle row grammar", line)
		}
		seen[cols[0]] = true
		args, ok := pairs(cols[2])
		expected, ok2 := pairs(cols[3])
		if !ok || !ok2 || len(args) != 7 {
			t.Fatal("late-intake oracle fields", line)
		}
		for _, key := range []string{"end", "ended", "window", "revision", "declared", "claimed", "received"} {
			if _, present := args[key]; !present {
				t.Fatal("late-intake oracle missing", key, line)
			}
		}
		window, err := strconv.Atoi(args["window"])
		revision, err2 := strconv.ParseInt(args["revision"], 10, 64)
		if err != nil || err2 != nil {
			t.Fatal("late-intake oracle number", line)
		}
		if _, isError := expected["error"]; isError && (len(expected) != 1 || expected["error"] != "INVALID") || !isError && len(expected) != 3 {
			t.Fatal("late-intake oracle expectation", line)
		}
		out = append(out, lateVector{id: cols[0], operation: cols[1], end: app.LateEnd(args["end"]), ended: args["ended"], declared: args["declared"], recvd: args["received"], claimed: args["claimed"], window: window, revision: revision, expected: expected})
	}
	return out
}

func lateInstant(t *testing.T, raw string) time.Time {
	t.Helper()
	if raw == "?" {
		return time.Time{}
	}
	at, err := orgtime.ParseInstant(raw)
	if err != nil {
		t.Fatal("late-intake oracle instant", raw)
	}
	return at
}

func TestLateIntakeOracleIsCompleteAndDecidesFromServerHistory(t *testing.T) {
	vectors := loadLateVectors(t)
	counts := map[string]int{}
	exercised := map[string]bool{}
	for _, v := range vectors {
		counts[v.operation]++
		t.Run(v.id, func(t *testing.T) {
			evidence := app.LateEvidence{LateAccess: app.LateAccess{End: v.end, EndedAt: lateInstant(t, v.ended)}, WindowDays: v.window, SettingsRevision: v.revision}
			facts := app.LateFacts{Received: lateInstant(t, v.recvd)}
			if v.declared != "?" {
				declared := lateInstant(t, v.declared)
				facts.Declared = &declared
			}
			if v.claimed != "?" {
				claimed := lateInstant(t, v.claimed)
				facts.Claimed = &claimed
			}
			got, err := app.DecideLate(evidence, facts)
			if v.expected["error"] == "INVALID" {
				exercised["INVALID"] = true
				if err == nil || got != (app.LateDecision{}) {
					t.Fatalf("invalid late evidence decided %+v", got)
				}
				return
			}
			want := app.LateDecision{Disposition: app.LateDisposition(v.expected["disposition"]), Result: v.expected["result"], Code: v.expected["code"]}
			if err != nil || got != want {
				t.Fatalf("late decision got %+v (%v), want %+v", got, err, want)
			}
			if got.Holds() != (got.Result == protocol.AcceptedForReview) {
				t.Fatal("hold predicate disagrees with result")
			}
			exercised[string(got.Disposition)], exercised[got.Code] = true, true
		})
	}
	if counts["DECIDE"] != 47 || counts["EVIDENCE"] != 7 || len(vectors) != 54 {
		t.Fatal("late-intake oracle counts", counts)
	}
	for _, name := range []string{"HOLD_CLAIM", "HOLD_DECLARED", "ENDED", "CLOSED", "INVALID", app.LateHeldCode, app.LateMembershipEndedCode, app.LateDeviceRevokedCode, app.LateWindowClosedCode} {
		if !exercised[name] {
			t.Fatal("late-intake oracle does not exercise", name)
		}
	}
}

// The declared instant is evidence: it can only choose review over refusal,
// never the window and never application (BR-OFFL-009, B-BR-EMP-303).
func TestLateIntakeDeclaredTimeNeverAppliesOrMovesTheWindow(t *testing.T) {
	end := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	evidence := app.LateEvidence{LateAccess: app.LateAccess{End: app.LateEndMembership, EndedAt: end}, WindowDays: 15}
	closed := end.Add(15*24*time.Hour + time.Microsecond)
	for _, declared := range []time.Time{time.Date(1, 1, 1, 0, 0, 0, 0, time.UTC), end.Add(-time.Nanosecond), closed, end.Add(20 * 24 * time.Hour)} {
		got, err := app.DecideLate(evidence, app.LateFacts{Declared: &declared, Received: closed})
		if err != nil || got.Disposition != app.LateClosed || got.Code != app.LateWindowClosedCode {
			t.Fatal("a declared instant reopened a closed window", declared, got, err)
		}
		for _, received := range []time.Time{end, end.Add(time.Hour), end.Add(15 * 24 * time.Hour)} {
			got, err := app.DecideLate(evidence, app.LateFacts{Declared: &declared, Received: received})
			if err != nil || got.Result == protocol.Accepted || got.Result != protocol.AcceptedForReview && got.Result != protocol.Rejected {
				t.Fatal("a late decision applied an operation", declared, got, err)
			}
		}
	}
}
