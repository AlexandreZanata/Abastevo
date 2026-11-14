package domain

import (
	"testing"
	"time"
)

func TestAdmissionConfirmationAndExactDeadline(t *testing.T) {
	for _, tc := range []struct {
		role        string
		force, want bool
	}{{"DRIVER", false, false}, {"OPERATIONS_STAFF", false, false}, {"DRIVER", true, true}, {"SUPERVISOR", false, true}, {"OWNER", false, true}} {
		got, e := InvitationNeedsConfirmation([]InvitationGrant{{Role: tc.role, Scope: "ORG"}}, tc.force)
		if e != nil || got != tc.want {
			t.Fatalf("confirmation for %s: %v", tc.role, e)
		}
	}
	if _, e := InvitationNeedsConfirmation([]InvitationGrant{{Role: "OTHER", Scope: "ORG"}}, false); e == nil {
		t.Fatal("unknown grant")
	}
	at := time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC)
	a, e := NewAdmission(AdmissionWaiting, at)
	if e != nil {
		t.Fatal(e)
	}
	if !a.CanConfirmAt(at.Add(7*24*time.Hour-time.Nanosecond)) || a.CanConfirmAt(at.Add(7*24*time.Hour)) || a.CanConfirmAt(at.Add(-time.Nanosecond)) {
		t.Fatal("deadline equality")
	}
	active, e := NewAdmission(AdmissionActive, at)
	if e != nil || active.CanConfirmAt(at) {
		t.Fatal("active reconfirmation")
	}
	if _, e := NewAdmission(AdmissionWaiting, time.Time{}); e == nil {
		t.Fatal("zero source time")
	}
	if _, e := NewAdmission("invented", at); e == nil {
		t.Fatal("unknown state")
	}
}
