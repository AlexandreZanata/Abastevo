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
)

const (
	stateCorpusPath = "../../../../../contracts/testdata/enterprise/sync/state-intake-v1.psv"
	stateCorpusHash = "ecb5ec152d738be26caa644abe1c2f3700b3445e4c6e8a969084a4cdb5004d5c"
)

// The plan oracle (EDR-201) is consumed here and by org_sync_plan_disposition.
func TestStateIntakePlanOracleIsCompleteAndDecidesFromTheCapabilityRow(t *testing.T) {
	raw, err := os.ReadFile(stateCorpusPath)
	if err != nil {
		t.Fatal(err)
	}
	if sum := sha256.Sum256(raw); hex.EncodeToString(sum[:]) != stateCorpusHash {
		t.Fatal("state-intake oracle changed without updating its pinned hash")
	}
	lines := strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n")
	if lines[0] != "id|operation|args|expected" {
		t.Fatal("state-intake oracle header")
	}
	seen, exercised := map[string]bool{}, map[string]bool{}
	for _, line := range lines[1:] {
		cols := strings.Split(line, "|")
		if len(cols) != 4 || cols[0] == "" || seen[cols[0]] || cols[1] != "PLAN" {
			t.Fatal("state-intake oracle row grammar", line)
		}
		seen[cols[0]] = true
		args, ok := pairs(cols[2])
		expected, ok2 := pairs(cols[3])
		if !ok || !ok2 || len(args) != 5 {
			t.Fatal("state-intake oracle fields", line)
		}
		revision, err := strconv.ParseInt(args["revision"], 10, 64)
		if err != nil {
			t.Fatal("state-intake oracle revision", line)
		}
		t.Run(cols[0], func(t *testing.T) {
			state := app.PlanState{State: args["state"], Revision: revision, ChangedAt: lateInstant(t, args["changed"])}
			var declared *time.Time
			if args["declared"] != "?" {
				at := lateInstant(t, args["declared"])
				declared = &at
			}
			got, err := app.DecidePlan(state, declared, lateInstant(t, args["received"]))
			if expected["error"] == "INVALID" {
				exercised["INVALID"] = true
				if err == nil || got != "" {
					t.Fatal("invalid plan evidence decided", got)
				}
				return
			}
			if err != nil || got != expected["disposition"] {
				t.Fatalf("plan decision got %q (%v), want %q", got, err, expected["disposition"])
			}
			exercised[got] = true
			if got == app.StateHold && (expected["result"] != protocol.AcceptedForReview || expected["code"] != app.StatePlanCode) {
				t.Fatal("plan hold expectation drifted")
			}
		})
	}
	if len(seen) != 20 {
		t.Fatal("state-intake oracle count", len(seen))
	}
	for _, name := range []string{app.PlanApply, app.StateHold, app.PlanDeny, "INVALID"} {
		if !exercised[name] {
			t.Fatal("state-intake oracle does not exercise", name)
		}
	}
}

func TestStateIntakeDecisionsValidateTheirShape(t *testing.T) {
	at := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	for _, valid := range []app.StateDecision{{},
		{Condition: app.StateSuspension, Disposition: app.StateHold, Result: protocol.AcceptedForReview, Code: app.StateGraceCode},
		{Condition: app.StateSuspension, Disposition: app.StateRefuse, Result: protocol.Rejected, Code: app.StateSuspendedCode},
		{Condition: app.StatePlan, Disposition: app.StateHold, Result: protocol.AcceptedForReview, Code: app.StatePlanCode, Instant: at, Capability: "organizations", CapabilityRevision: 2},
	} {
		if valid.Validate() != nil {
			t.Fatal("valid state decision refused", valid.Condition, valid.Code)
		}
	}
	for _, invalid := range []app.StateDecision{{Code: app.StateGraceCode},
		{Condition: app.StateSuspension, Disposition: app.StateHold, Result: protocol.Accepted, Code: app.StateGraceCode},
		{Condition: app.StateSuspension, Disposition: app.StateRefuse, Result: protocol.Rejected, Code: app.StateGraceCode},
		{Condition: app.StatePlan, Disposition: app.StateRefuse, Result: protocol.Rejected, Code: app.StatePlanCode, Instant: at, Capability: "organizations", CapabilityRevision: 2},
		{Condition: app.StatePlan, Disposition: app.StateHold, Result: protocol.AcceptedForReview, Code: app.StatePlanCode, Capability: "organizations", CapabilityRevision: 2},
		{Condition: app.StatePlan, Disposition: app.StateHold, Result: protocol.AcceptedForReview, Code: app.StatePlanCode, Instant: at, Capability: "organizations", CapabilityRevision: 1},
	} {
		if invalid.Validate() == nil {
			t.Fatal("invalid state decision accepted", invalid.Condition, invalid.Code)
		}
	}
}
