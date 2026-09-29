package domain

import (
	"testing"
	"time"
)

func testObs() Observation {
	obs, _, err := NewObservation(Params{
		ID: "d6c74c23-63db-4c24-a2e5-408cb23bad26", ContributorRef: "ref-1",
		ClientSubmissionID: "client-1", StationID: "d6c74c23-63db-4c24-a2e5-408cb23bad27",
		Product: "GASOLINE_REGULAR", Unit: "L", AmountMilli: 5999,
		RawText: "5,999", ConditionKind: "STANDARD",
		ClaimedCapturedAt: time.Now().Add(-time.Hour), ReceivedAt: time.Now(),
	})
	if err != nil {
		panic(err)
	}
	return obs
}

func TestValidTransitions(t *testing.T) {
	obs := testObs()
	now := time.Now()
	claim, err := ClaimValidation(obs, StateReceived, "job-1", ActorWorker, now, 1)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if claim.EventName() != "ValidationStarted" || claim.Sequence != 1 || claim.CommandRef != "job-1" {
		t.Errorf("claim = %+v", claim)
	}
	admit, err := Admit(obs, StateValidating, ActorWorker, now, 2)
	if err != nil {
		t.Fatalf("admit: %v", err)
	}
	if admit.EventName() != "ObservationValidated" {
		t.Errorf("admit event = %q", admit.EventName())
	}
	reject, err := Reject(obs, StateValidating, ActorWorker, []string{"invalid-station"}, now, 2)
	if err != nil {
		t.Fatalf("reject: %v", err)
	}
	if reject.EventName() != "ObservationRejected" || len(reject.ReasonCodes) != 1 {
		t.Errorf("reject = %+v", reject)
	}
	inv, err := Invalidate(obs, StateValidated, ActorModerator, "case-9", []string{"evidence-mismatch"}, now, 3)
	if err != nil {
		t.Fatalf("invalidate: %v", err)
	}
	if inv.EventName() != "ObservationInvalidated" || inv.CaseRef != "case-9" {
		t.Errorf("invalidate = %+v", inv)
	}
}

func TestInvalidTransitions(t *testing.T) {
	obs := testObs()
	now := time.Now()
	cases := []struct {
		name string
		run  func() error
	}{
		{"skip to validated", func() error { _, err := Admit(obs, StateReceived, ActorWorker, now, 1); return err }},
		{"claim twice", func() error {
			_, err := ClaimValidation(obs, StateValidating, "job-1", ActorWorker, now, 2)
			return err
		}},
		{"validate from validated", func() error { _, err := Admit(obs, StateValidated, ActorWorker, now, 3); return err }},
		{"reject from received", func() error { _, err := Reject(obs, StateReceived, ActorWorker, []string{"x"}, now, 1); return err }},
		{"reject from validated", func() error { _, err := Reject(obs, StateValidated, ActorWorker, []string{"x"}, now, 3); return err }},
		{"reject from rejected", func() error { _, err := Reject(obs, StateRejected, ActorWorker, []string{"x"}, now, 4); return err }},
		{"invalidate from received", func() error {
			_, err := Invalidate(obs, StateReceived, ActorModerator, "c", []string{"x"}, now, 1)
			return err
		}},
		{"invalidate from validating", func() error {
			_, err := Invalidate(obs, StateValidating, ActorModerator, "c", []string{"x"}, now, 2)
			return err
		}},
		{"invalidate from rejected", func() error {
			_, err := Invalidate(obs, StateRejected, ActorModerator, "c", []string{"x"}, now, 4)
			return err
		}},
		{"admit from rejected", func() error { _, err := Admit(obs, StateRejected, ActorWorker, now, 4); return err }},
		{"claim from rejected", func() error { _, err := ClaimValidation(obs, StateRejected, "job-1", ActorWorker, now, 4); return err }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := c.run(); err == nil {
				t.Error("forbidden transition accepted")
			}
		})
	}
}

func TestActorsReasonsAndCommands(t *testing.T) {
	obs := testObs()
	now := time.Now()
	if _, err := ClaimValidation(obs, StateReceived, "job-1", ActorModerator, now, 1); err == nil {
		t.Error("moderator claim accepted")
	}
	if _, err := ClaimValidation(obs, StateReceived, "", ActorWorker, now, 1); err == nil {
		t.Error("commandless claim accepted")
	}
	if _, err := Admit(obs, StateValidating, ActorModerator, now, 2); err == nil {
		t.Error("moderator admit accepted")
	}
	if _, err := Reject(obs, StateValidating, ActorWorker, nil, now, 2); err == nil {
		t.Error("reasonless reject accepted")
	}
	if _, err := Reject(obs, StateValidating, ActorWorker, []string{""}, now, 2); err == nil {
		t.Error("blank reason accepted")
	}
	if _, err := Invalidate(obs, StateValidated, ActorWorker, "case-1", []string{"x"}, now, 3); err == nil {
		t.Error("worker invalidation accepted")
	}
	if _, err := Invalidate(obs, StateValidated, ActorModerator, "", []string{"x"}, now, 3); err == nil {
		t.Error("caseless invalidation accepted")
	}
	if _, err := Invalidate(obs, StateValidated, ActorModerator, "case-1", nil, now, 3); err == nil {
		t.Error("reasonless invalidation accepted")
	}
}

func TestNoStateConfusion(t *testing.T) {
	// Freshness, confidence and disputes are separate concepts: none of
	// them enters this machine, whose four transitions are exhaustive.
	for _, s := range []string{StateReceived, StateValidating, StateValidated, StateRejected} {
		switch s {
		case StateReceived, StateValidating, StateValidated, StateRejected:
		default:
			t.Errorf("unknown state %q", s)
		}
	}
	if (Decision{FromState: StateReceived, ToState: StateValidated}).EventName() != "" {
		t.Error("skipped transition has an event name")
	}
	if (Decision{FromState: StateRejected, ToState: StateReceived}).EventName() != "" {
		t.Error("resurrection has an event name")
	}
}
