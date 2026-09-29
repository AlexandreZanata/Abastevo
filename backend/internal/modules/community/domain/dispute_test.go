package domain

import (
	"errors"
	"testing"
	"time"
)

func validDisputeParams() DisputeParams {
	now := time.Now()
	return DisputeParams{
		ID:                  "e6c74c23-63db-4c24-a2e5-408cb23bad26",
		TargetObservationID: "d6c74c23-63db-4c24-a2e5-408cb23bad27",
		ContributorRef:      "tok-c2", ClientSubmissionID: "dsp-1",
		Reason: ReasonPriceChanged, ReplacementID: "d6c74c23-63db-4c24-a2e5-408cb23bad28",
		ReceivedAt: now,
	}
}

func TestNewDisputeHappyPath(t *testing.T) {
	d, evt, err := NewDispute(validDisputeParams())
	if err != nil {
		t.Fatalf("valid dispute rejected: %v", err)
	}
	if d.Reason != ReasonPriceChanged || d.Status != DisputeOpen {
		t.Errorf("dispute = %+v", d)
	}
	if d.PolicyVersion != PolicyV1 {
		t.Errorf("policy = %q", d.PolicyVersion)
	}
	if evt.DisputeID != d.ID || evt.Name() != "ObservationDisputed" {
		t.Errorf("event = %+v", evt)
	}
	// Self-reports are allowed: authors supersede through PRICE_CHANGED
	// with a replacement, while only self-confirmation is forbidden.
	p := validDisputeParams()
	if _, _, err := NewDispute(p); err != nil {
		t.Fatalf("report rejected: %v", err)
	}
}

func TestNewDisputeValidatesReason(t *testing.T) {
	for _, reason := range []string{
		ReasonPriceChanged, ReasonWrongStation, ReasonWrongProduct,
		ReasonWrongCondition, ReasonEvidenceMismatch, ReasonOther,
	} {
		p := validDisputeParams()
		p.Reason = reason
		if _, _, err := NewDispute(p); err != nil {
			t.Errorf("reason %q rejected: %v", reason, err)
		}
	}
	p := validDisputeParams()
	p.Reason = "WRONG_EVERYTHING"
	if _, _, err := NewDispute(p); !errors.Is(err, ErrUnknownReason) {
		t.Errorf("unknown reason = %v", err)
	}
	p = validDisputeParams()
	p.Reason = ""
	if _, _, err := NewDispute(p); !errors.Is(err, ErrUnknownReason) {
		t.Errorf("empty reason = %v", err)
	}
}

func TestNewDisputeRejectsSelfReplacement(t *testing.T) {
	// A replacement pointing at the disputed observation itself is
	// meaningless: reference a real replacement or omit it.
	p := validDisputeParams()
	p.ReplacementID = p.TargetObservationID
	if _, _, err := NewDispute(p); !errors.Is(err, ErrInvalidDispute) {
		t.Errorf("self replacement = %v", err)
	}
	p = validDisputeParams()
	p.ReplacementID = "not-a-uuid"
	if _, _, err := NewDispute(p); !errors.Is(err, ErrInvalidDispute) {
		t.Errorf("malformed replacement = %v", err)
	}
}

func TestNewDisputeRequiresIdentity(t *testing.T) {
	cases := map[string]func(*DisputeParams){
		"id":          func(p *DisputeParams) { p.ID = "" },
		"target":      func(p *DisputeParams) { p.TargetObservationID = "" },
		"contributor": func(p *DisputeParams) { p.ContributorRef = "" },
		"client-id":   func(p *DisputeParams) { p.ClientSubmissionID = "" },
		"clock":       func(p *DisputeParams) { p.ReceivedAt = time.Time{} },
	}
	for name, mutate := range cases {
		p := validDisputeParams()
		mutate(&p)
		if _, _, err := NewDispute(p); !errors.Is(err, ErrInvalidDispute) {
			t.Errorf("%s accepted: %v", name, err)
		}
	}
}
