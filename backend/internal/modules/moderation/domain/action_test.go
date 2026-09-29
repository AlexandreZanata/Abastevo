package domain

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func validActionParams() ActionParams {
	return ActionParams{
		ID:      "a0000000-0000-4000-8000-000000000001",
		CaseID:  "c0000000-0000-4000-8000-000000000001",
		ActorID: "op-7", Action: ActionReview,
		Reason: "triage: conflicting reports", OccurredAt: time.Now(),
	}
}

func TestNewActionValid(t *testing.T) {
	a, evt, err := NewAction(validActionParams())
	if err != nil {
		t.Fatalf("valid action rejected: %v", err)
	}
	if a.ActorID != "op-7" || a.PolicyVersion != PolicyV1 {
		t.Errorf("action = %+v", a)
	}
	if evt.ActionID != a.ID || evt.Name() != "ModerationActionRecorded" {
		t.Errorf("event = %+v", evt)
	}
}

func TestNewActionRequiresOperatorAndReason(t *testing.T) {
	p := validActionParams()
	p.ActorID = "  "
	if _, _, err := NewAction(p); !errors.Is(err, ErrUnauthorized) {
		t.Errorf("anonymous operator = %v", err)
	}
	p = validActionParams()
	p.Reason = ""
	if _, _, err := NewAction(p); !errors.Is(err, ErrBadReason) {
		t.Errorf("empty reason = %v", err)
	}
	p = validActionParams()
	p.Reason = strings.Repeat("r", MaxReasonChars+1)
	if _, _, err := NewAction(p); !errors.Is(err, ErrBadReason) {
		t.Errorf("long reason = %v", err)
	}
	p = validActionParams()
	p.Action = "BAN"
	if _, _, err := NewAction(p); !errors.Is(err, ErrUnknownAction) {
		t.Errorf("unknown action = %v", err)
	}
}

func TestAllowedForTarget(t *testing.T) {
	if !AllowedForTarget(ActionInvalidate, TargetObservation) {
		t.Error("invalidate observation refused")
	}
	if AllowedForTarget(ActionInvalidate, TargetContributor) {
		t.Error("invalidate contributor allowed")
	}
	if !AllowedForTarget(ActionBlock, TargetContributor) {
		t.Error("block contributor refused")
	}
	if AllowedForTarget(ActionBlock, TargetObservation) {
		t.Error("block observation allowed")
	}
	for _, a := range []string{ActionReview, ActionResolve, ActionDismiss} {
		for _, target := range []string{TargetObservation, TargetDispute, TargetContributor, TargetEvidence} {
			if !AllowedForTarget(a, target) {
				t.Errorf("%s on %s refused", a, target)
			}
		}
	}
	if AllowedForTarget("BAN", TargetObservation) {
		t.Error("unknown action allowed")
	}
}

func TestNextStatus(t *testing.T) {
	cases := []struct {
		action, from, want string
	}{
		{ActionReview, StatusOpen, StatusInReview},
		{ActionInvalidate, StatusOpen, StatusResolved},
		{ActionInvalidate, StatusInReview, StatusResolved},
		{ActionBlock, StatusInReview, StatusResolved},
		{ActionResolve, StatusOpen, StatusResolved},
		{ActionDismiss, StatusOpen, StatusRejected},
		{ActionDismiss, StatusInReview, StatusRejected},
	}
	for _, tc := range cases {
		got, err := NextStatus(tc.action, tc.from)
		if err != nil || got != tc.want {
			t.Errorf("%s from %s = %q, %v; want %q", tc.action, tc.from, got, err, tc.want)
		}
	}
	if _, err := NextStatus(ActionReview, StatusInReview); !errors.Is(err, ErrBadTransition) {
		t.Errorf("re-review = %v", err)
	}
	if _, err := NextStatus(ActionResolve, StatusResolved); !errors.Is(err, ErrTerminalCase) {
		t.Errorf("resolved reopen = %v", err)
	}
	if _, err := NextStatus(ActionDismiss, StatusRejected); !errors.Is(err, ErrTerminalCase) {
		t.Errorf("rejected reopen = %v", err)
	}
}
