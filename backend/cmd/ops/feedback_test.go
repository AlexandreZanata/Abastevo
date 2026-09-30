package main

import (
	"testing"
)

func TestParseFeedbackArgs(t *testing.T) {
	got, err := parseFeedbackArgs([]string{"--comment", "c1", "--case", "k9"})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !got.hide || got.comment != "c1" || got.caseID != "k9" {
		t.Errorf("hide defaults true with ids: %+v", got)
	}
	if _, err := parseFeedbackArgs([]string{"--comment", "c1"}); err == nil {
		t.Error("missing case must refuse (audit binding required)")
	}
	if _, err := parseFeedbackArgs([]string{"--case", "k9"}); err == nil {
		t.Error("missing comment must refuse")
	}
}

func TestRunFeedbackDispatch(t *testing.T) {
	if err := runFeedback([]string{}, getenvNone); err == nil {
		t.Error("empty feedback command must refuse")
	}
	if err := runFeedback([]string{"freeze", "--comment", "c1", "--case", "k9"}, getenvNone); err == nil {
		t.Error("unknown feedback command must refuse")
	}
}

func TestParseFeedbackExportEraseArgs(t *testing.T) {
	got, err := parseFeedbackExportArgs([]string{"--account", "a1"})
	if err != nil {
		t.Fatalf("export parse: %v", err)
	}
	if got.account != "a1" {
		t.Errorf("export account: %+v", got)
	}
	if _, err := parseFeedbackExportArgs([]string{}); err == nil {
		t.Error("missing account must refuse export")
	}
	erase, err := parseFeedbackEraseArgs([]string{"--account", "a1", "--reason", "owner request"})
	if err != nil {
		t.Fatalf("erase parse: %v", err)
	}
	if erase.account != "a1" || erase.reason != "owner request" {
		t.Errorf("erase args: %+v", erase)
	}
	if _, err := parseFeedbackEraseArgs([]string{"--account", "a1"}); err == nil {
		t.Error("missing reason must refuse erase")
	}
	if _, err := parseFeedbackEraseArgs([]string{"--reason", "x"}); err == nil {
		t.Error("missing account must refuse erase")
	}
}
