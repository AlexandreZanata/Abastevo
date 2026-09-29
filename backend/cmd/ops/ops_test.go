package main

import (
	"os"
	"strings"
	"testing"
	"time"

	moderationdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/moderation/domain"
)

func getenvNone(string) string { return "" }

func TestResolveOperatorID(t *testing.T) {
	if got, err := resolveOperatorID("op-7", getenvNone); err != nil || got != "op-7" {
		t.Errorf("flag operator = %q, %v", got, err)
	}
	env := func(k string) string {
		if k == "ANPFUEL_OPERATOR_ID" {
			return "op-env"
		}
		return ""
	}
	if got, err := resolveOperatorID("", env); err != nil || got != "op-env" {
		t.Errorf("env operator = %q, %v", got, err)
	}
	if _, err := resolveOperatorID("", getenvNone); err == nil {
		t.Error("anonymous operator accepted")
	}
	// Explicit flag wins over environment.
	if got, err := resolveOperatorID("op-flag", env); err != nil || got != "op-flag" {
		t.Errorf("flag precedence = %q, %v", got, err)
	}
}

func TestParseActArgs(t *testing.T) {
	a, err := parseActArgs([]string{"--case", "c1", "--action", "REVIEW", "--reason", "triage"})
	if err != nil {
		t.Fatalf("parse = %v", err)
	}
	if a.caseID != "c1" || a.action != "REVIEW" || a.reason != "triage" {
		t.Errorf("args = %+v", a)
	}
	if _, err := parseActArgs([]string{"--action", "REVIEW"}); err == nil {
		t.Error("missing --case accepted")
	}
	if _, err := parseActArgs([]string{"--case", "c1"}); err == nil {
		t.Error("missing --action accepted")
	}
}

func TestEvidenceAccessAllowed(t *testing.T) {
	mkCase := func(status, evidence string) moderationdomain.Case {
		c, _, err := moderationdomain.NewCase(moderationdomain.CaseParams{
			ID:         "c0000000-0000-4000-8000-000000000001",
			TargetType: moderationdomain.TargetObservation,
			TargetID:   "b0000000-0000-4000-8000-000000000001",
			Priority:   moderationdomain.PriorityP2, Reason: "r",
			EvidenceID: evidence,
			OpenedAt:   time.Now(),
		})
		if err != nil {
			t.Fatal(err)
		}
		c.Status = status
		return c
	}
	bound := "e0000000-0000-4000-8000-000000000001"
	if err := evidenceAccessAllowed(mkCase(moderationdomain.StatusOpen, bound), bound); err != nil {
		t.Errorf("bound open case refused: %v", err)
	}
	if err := evidenceAccessAllowed(mkCase(moderationdomain.StatusInReview, bound), bound); err != nil {
		t.Errorf("bound triaged case refused: %v", err)
	}
	// Foreign evidence through an open case refuses: no fishing.
	if err := evidenceAccessAllowed(mkCase(moderationdomain.StatusOpen, bound), "e0000000-0000-4000-8000-000000000002"); err == nil {
		t.Error("foreign evidence allowed")
	}
	// Closed cases refuse even bound evidence.
	if err := evidenceAccessAllowed(mkCase(moderationdomain.StatusResolved, bound), bound); err == nil {
		t.Error("closed case allowed")
	}
	// Cases without a bound reference refuse everything.
	if err := evidenceAccessAllowed(mkCase(moderationdomain.StatusOpen, ""), bound); err == nil {
		t.Error("unbound case allowed")
	}
	if err := evidenceAccessAllowed(mkCase(moderationdomain.StatusOpen, bound), ""); err == nil {
		t.Error("empty evidence allowed")
	}
}

func TestRunRejectsUnknownCommand(t *testing.T) {
	if err := run([]string{"launch-missiles"}, getenvNone); err == nil {
		t.Error("unknown command accepted")
	} else if !strings.Contains(err.Error(), "usage") {
		t.Errorf("error hides usage: %v", err)
	}
	if err := run(nil, getenvNone); err == nil {
		t.Error("empty invocation accepted")
	}
	if err := run([]string{"privacy", "launch"}, getenvNone); err == nil {
		t.Error("unknown privacy command accepted")
	}
}

func TestParseEraseArgs(t *testing.T) {
	e, err := parseEraseArgs([]string{"--contributor", "c1", "--reason", "owner request"})
	if err != nil {
		t.Fatalf("parse = %v", err)
	}
	if e.contributor != "c1" || e.reason != "owner request" {
		t.Errorf("args = %+v", e)
	}
	if e.clientKey == "" {
		t.Error("client key not defaulted")
	}
	if _, err := parseEraseArgs([]string{"--contributor", "c1"}); err == nil {
		t.Error("missing --reason accepted")
	}
	if _, err := parseEraseArgs([]string{"--reason", "r"}); err == nil {
		t.Error("missing --contributor accepted")
	}
}

func TestParseReplayArgs(t *testing.T) {
	r, err := parseReplayArgs([]string{"--contributor", "c1"})
	if err != nil {
		t.Fatalf("parse = %v", err)
	}
	if r.contributor != "c1" {
		t.Errorf("args = %+v", r)
	}
	if _, err := parseReplayArgs(nil); err == nil {
		t.Error("missing --contributor accepted")
	}
}

func TestStuckRuleBoundary(t *testing.T) {
	now := time.Now()
	if firing, _, _ := stuckRule(0, now.Add(-time.Hour), now); firing {
		t.Error("empty queue fired")
	}
	if firing, _, _ := stuckRule(3, time.Time{}, now); firing {
		t.Error("zero oldest fired")
	}
	if firing, detail, _ := stuckRule(3, now.Add(-6*time.Minute), now); !firing || detail == "" {
		t.Errorf("stuck queue silent: %v %q", firing, detail)
	}
	if firing, _, _ := stuckRule(3, now.Add(-time.Minute), now); firing {
		t.Error("fresh queue fired")
	}
}

func TestDeadRule(t *testing.T) {
	if firing, _, _ := deadRule(0); firing {
		t.Error("clean queue fired")
	}
	if firing, detail, _ := deadRule(2); !firing || detail != "dead=2" {
		t.Errorf("dead queue silent: %v %q", firing, detail)
	}
}

func TestBackupRule(t *testing.T) {
	now := time.Now()
	missing := "/nonexistent/manifest.json"
	if _, _, err := backupRule(missing, now); err == nil {
		t.Error("missing manifest counted as fresh")
	}
	fresh, err := os.CreateTemp("", "manifest-*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove(fresh.Name()) }()
	if firing, _, err := backupRule(fresh.Name(), now); err != nil || firing {
		t.Errorf("fresh manifest fired: %v %v", firing, err)
	}
	old, err := os.CreateTemp("", "manifest-*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove(old.Name()) }()
	past := now.Add(-30 * time.Hour)
	if err := os.Chtimes(old.Name(), past, past); err != nil {
		t.Fatal(err)
	}
	if firing, detail, err := backupRule(old.Name(), now); err != nil || !firing || detail == "" {
		t.Errorf("stale manifest silent: %v %q %v", firing, detail, err)
	}
}
