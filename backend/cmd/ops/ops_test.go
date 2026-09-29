package main

import (
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
}
