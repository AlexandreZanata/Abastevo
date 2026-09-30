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
