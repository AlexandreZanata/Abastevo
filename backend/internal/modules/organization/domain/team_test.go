package domain

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
)

func teamScope(t *testing.T, id string) kernel.OrgScope {
	t.Helper()
	org, e := kernel.ParseOrgID(id)
	if e != nil {
		t.Fatal(e)
	}
	s, e := kernel.NewOrgScope(org)
	if e != nil {
		t.Fatal(e)
	}
	return s
}
func TestTeamStructureAndRetainedLinkPeriods(t *testing.T) {
	scope := teamScope(t, "7b0e2a54-0000-4000-8000-000000000301")
	foreign := teamScope(t, "7b0e2a54-0000-4000-8000-000000000302")
	team, e := NewTeam(scope, "7b0e2a54-0000-4000-8000-000000000b01", TeamProfile{Name: " Synthetic Crew "})
	if e != nil {
		t.Fatal(e)
	}
	employee := TeamEmployee{OrgID: scope.OrgID(), ID: "7b0e2a54-0000-4000-8000-000000000501", Active: true}
	if _, e := team.MemberChange(scope, 1, TeamEmployee{OrgID: foreign.OrgID(), ID: employee.ID, Active: true}, true); e != ErrScope {
		t.Fatal("foreign employee", e)
	}
	if _, e := team.MemberChange(scope, 1, TeamEmployee{OrgID: scope.OrgID(), ID: employee.ID}, true); e != ErrConditions {
		t.Fatal("inactive employee", e)
	}
	changed, e := team.MemberChange(scope, 1, employee, true)
	if e != nil {
		t.Fatal(e)
	}
	snap, _ := changed.Snapshot(scope)
	if snap.Revision != 2 {
		t.Fatal("link revision")
	}
	if _, e := changed.Archive(scope, 2, StructureUsage{Employees: 1}); e != ErrStructureInUse {
		t.Fatal("active employee ignored", e)
	}
	archived, e := changed.Archive(scope, 2, StructureUsage{})
	if e != nil {
		t.Fatal(e)
	}
	if _, e := archived.MemberChange(scope, 3, employee, true); e != ErrTransition {
		t.Fatal("archived admission", e)
	}
	correction, e := archived.Change(scope, 3, TeamProfile{Name: "Synthetic corrected"})
	if e != nil {
		t.Fatal(e)
	}
	restored, e := correction.Restore(scope, 4)
	if e != nil {
		t.Fatal(e)
	}
	if _, e := restored.Change(scope, 4, TeamProfile{Name: "stale"}); e != ErrConflict {
		t.Fatal("stale", e)
	}
	if _, e := restored.Snapshot(foreign); e != ErrScope {
		t.Fatal("foreign snapshot", e)
	}
	now := time.Date(2026, 11, 12, 12, 0, 0, 0, time.UTC)
	link, e := NewTeamLink(scope, "7b0e2a54-0000-4000-8000-000000000c01", team, employee, now)
	if e != nil {
		t.Fatal(e)
	}
	ended, e := link.End(scope, now.Add(time.Second), TeamLinkEmployeeInactive)
	if e != nil {
		t.Fatal(e)
	}
	if _, e := ended.End(scope, now.Add(2*time.Second), TeamLinkRemoved); e != ErrTransition {
		t.Fatal("ended period rewritten", e)
	}
	history, _ := ended.Snapshot(scope)
	if history.EndReason != TeamLinkEmployeeInactive || history.EndedAt.IsZero() {
		t.Fatal("lost closure")
	}
	for _, v := range []any{team, snap, link, history} {
		raw, _ := json.Marshal(v)
		if strings.Contains(fmt.Sprintf("%+v %s", v, raw), "Synthetic") {
			t.Fatal("private formatting")
		}
	}
}
