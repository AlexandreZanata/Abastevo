package domain_test

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
)

func assignmentScope(t *testing.T, raw string) kernel.OrgScope {
	t.Helper()
	id, err := kernel.ParseOrgID(raw)
	if err != nil {
		t.Fatal(err)
	}
	s, err := kernel.NewOrgScope(id)
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func TestAssignmentHalfOpenValidityAndTerminalEnd(t *testing.T) {
	s := assignmentScope(t, "7b0e2a54-0000-4000-8000-000000000301")
	now := time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)
	input := d.AssignmentSnapshot{OrgID: s.OrgID(), ID: "7b0e2a54-0000-4000-8000-000000000701", MembershipID: "7b0e2a54-0000-4000-8000-000000000601", AssignedBy: "7b0e2a54-0000-4000-8000-000000000602", Role: d.Driver, Scope: d.SelfScope, TargetID: "7b0e2a54-0000-4000-8000-000000000601", ValidFrom: now, ValidTo: now.Add(time.Hour), Reason: "synthetic access", Revision: 1}
	a, err := d.RehydrateAssignment(s, input)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		at   time.Time
		want bool
	}{{now.Add(-time.Nanosecond), false}, {now, true}, {now.Add(time.Hour - time.Nanosecond), true}, {now.Add(time.Hour), false}} {
		got, err := a.ValidAt(s, tc.at)
		if err != nil || got != tc.want {
			t.Fatal(tc.at, got, err)
		}
	}
	ended, err := a.End(s, 1, now.Add(-time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if valid, err := ended.ValidAt(s, now); valid || err != nil {
		t.Fatal(valid, err)
	}
	if _, err := ended.End(s, 2, now); err != d.ErrAssignmentEnded {
		t.Fatal(err)
	}
	if _, err := a.End(s, 2, now); err != d.ErrAssignmentConflict {
		t.Fatal(err)
	}
	foreign := assignmentScope(t, "7b0e2a54-0000-4000-8000-000000000302")
	if _, err := a.Snapshot(foreign); err != d.ErrAssignmentScope {
		t.Fatal(err)
	}
	if valid, err := a.ValidAt(kernel.OrgScope{}, now); valid || err != d.ErrAssignmentScope {
		t.Fatal(valid, err)
	}
	for _, v := range []any{a, input, ended} {
		encoded, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(encoded), input.ID) || strings.Contains(string(encoded), input.Reason) {
			t.Fatal("JSON leaked assignment")
		}
		if logged, ok := v.(slog.LogValuer); !ok || strings.Contains(logged.LogValue().String(), input.ID) {
			t.Fatal("slog leaked assignment")
		}
		for _, verb := range []string{"%v", "%+v", "%#v", "%s", "%d"} {
			if text := fmt.Sprintf(verb, v); strings.Contains(text, input.ID) || strings.Contains(text, input.MembershipID) || strings.Contains(text, input.Reason) {
				t.Fatal("assignment leaked private values")
			}
		}
	}
	if _, err := a.ValidAt(s, time.Time{}); err != d.ErrAssignmentInvalid {
		t.Fatal(err)
	}
	if _, err := a.End(foreign, 1, now); err != d.ErrAssignmentScope {
		t.Fatal(err)
	}
	if _, err := a.End(s, 1, time.Time{}); err != d.ErrAssignmentInvalid {
		t.Fatal(err)
	}
}

func TestAssignmentRejectsInvalidOrWidenedScope(t *testing.T) {
	s := assignmentScope(t, "7b0e2a54-0000-4000-8000-000000000301")
	now := time.Now().UTC()
	base := d.AssignmentSnapshot{OrgID: s.OrgID(), ID: "7b0e2a54-0000-4000-8000-000000000701", MembershipID: "7b0e2a54-0000-4000-8000-000000000601", AssignedBy: "7b0e2a54-0000-4000-8000-000000000602", Role: d.Owner, Scope: d.OrgScope, TargetID: s.OrgID().String(), ValidFrom: now, Reason: "synthetic access", Revision: 1}
	for _, change := range []func(*d.AssignmentSnapshot){
		func(v *d.AssignmentSnapshot) { v.ID = "bad" }, func(v *d.AssignmentSnapshot) { v.MembershipID = "" }, func(v *d.AssignmentSnapshot) { v.AssignedBy = "" },
		func(v *d.AssignmentSnapshot) { v.Role = "administrator" }, func(v *d.AssignmentSnapshot) { v.Scope = "UNIT" }, func(v *d.AssignmentSnapshot) { v.TargetID = v.MembershipID },
		func(v *d.AssignmentSnapshot) { v.Scope = d.SelfScope; v.TargetID = v.MembershipID }, func(v *d.AssignmentSnapshot) { v.Role = d.StationAdmin },
		func(v *d.AssignmentSnapshot) { v.ValidFrom = time.Time{} }, func(v *d.AssignmentSnapshot) { v.ValidTo = v.ValidFrom },
		func(v *d.AssignmentSnapshot) { v.ValidTo = v.ValidFrom.Add(-time.Second) }, func(v *d.AssignmentSnapshot) { v.Reason = "\n" }, func(v *d.AssignmentSnapshot) { v.Reason = strings.Repeat("x", 281) },
		func(v *d.AssignmentSnapshot) { v.Revision = 0 }, func(v *d.AssignmentSnapshot) { v.EndedAt = now; v.Revision = 1 },
	} {
		v := base
		change(&v)
		if _, err := d.RehydrateAssignment(s, v); err == nil {
			t.Fatal("invalid assignment accepted")
		}
	}
}
