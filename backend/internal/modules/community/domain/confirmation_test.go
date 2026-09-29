package domain

import (
	"errors"
	"testing"
	"time"
)

func validConfirmationParams() ConfirmationParams {
	now := time.Now()
	return ConfirmationParams{
		ID: "c6c74c23-63db-4c24-a2e5-408cb23bad26", ObservationID: "d6c74c23-63db-4c24-a2e5-408cb23bad27",
		ContributorRef: "tok-c2", AuthorRef: "tok-c1",
		ClientSubmissionID: "cfm-1", ReceivedAt: now,
	}
}

func TestNewConfirmationHappyPath(t *testing.T) {
	c, evt, err := NewConfirmation(validConfirmationParams())
	if err != nil {
		t.Fatalf("valid confirmation rejected: %v", err)
	}
	if c.ObservationID == "" || c.ContributorRef != "tok-c2" {
		t.Errorf("confirmation = %+v", c)
	}
	if c.PolicyVersion != PolicyV1 {
		t.Errorf("policy = %q", c.PolicyVersion)
	}
	if evt.ConfirmationID != c.ID || evt.ObservationID != c.ObservationID || evt.Name() != "PriceConfirmed" {
		t.Errorf("event = %+v", evt)
	}
}

func TestNewConfirmationRejectsSelf(t *testing.T) {
	// B-BR-006: no self-confirmation. The author ref arrives as a
	// server-resolved fact beside the contributor, never from the client.
	p := validConfirmationParams()
	p.ContributorRef = "tok-c1"
	if _, _, err := NewConfirmation(p); !errors.Is(err, ErrSelfConfirmation) {
		t.Errorf("self-confirmation = %v", err)
	}
}

func TestNewConfirmationRequiresIdentity(t *testing.T) {
	cases := map[string]func(*ConfirmationParams){
		"id":          func(p *ConfirmationParams) { p.ID = "" },
		"observation": func(p *ConfirmationParams) { p.ObservationID = " " },
		"contributor": func(p *ConfirmationParams) { p.ContributorRef = "" },
		"author":      func(p *ConfirmationParams) { p.AuthorRef = "" },
		"client-id":   func(p *ConfirmationParams) { p.ClientSubmissionID = "" },
		"clock":       func(p *ConfirmationParams) { p.ReceivedAt = time.Time{} },
	}
	for name, mutate := range cases {
		p := validConfirmationParams()
		mutate(&p)
		if _, _, err := NewConfirmation(p); !errors.Is(err, ErrInvalidConfirmation) {
			t.Errorf("%s accepted: %v", name, err)
		}
	}
}
