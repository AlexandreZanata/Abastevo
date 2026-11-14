package domain

import (
	"encoding/json"
	"fmt"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"strings"
	"testing"
	"time"
)

func invitationScope(t *testing.T) kernel.OrgScope {
	t.Helper()
	id, e := kernel.ParseOrgID("7b0e2a54-0000-4000-8000-000000009001")
	if e != nil {
		t.Fatal(e)
	}
	s, e := kernel.NewOrgScope(id)
	if e != nil {
		t.Fatal(e)
	}
	return s
}
func pendingInvitation(t *testing.T) (kernel.OrgScope, InvitationSnapshot) {
	s := invitationScope(t)
	at := time.Date(2026, 11, 12, 12, 0, 0, 0, time.UTC)
	v := InvitationSnapshot{OrgID: s.OrgID(), ID: "7b0e2a54-0000-4000-8000-000000009002", EmployeeID: "7b0e2a54-0000-4000-8000-000000009003", MembershipID: "7b0e2a54-0000-4000-8000-000000009004", IssuerMembershipID: "7b0e2a54-0000-4000-8000-000000009005", OperationID: "7b0e2a54-0000-4000-8000-000000009006", Revision: 1, SettingsRevision: 1, ValidityDays: 7, State: InvitationPending, CreatedAt: at, ChangedAt: at, ExpiresAt: at.Add(168 * time.Hour), Grants: []InvitationGrant{{Role: "DRIVER", Scope: "SELF"}}}
	return s, v
}
func TestInvitationExpiryAndPrivateRolePlan(t *testing.T) {
	s, v := pendingInvitation(t)
	at := v.CreatedAt
	invite, e := RehydrateInvitation(s, v)
	if e != nil {
		t.Fatal(e)
	}
	if !invite.UsableAt(s, at) || invite.UsableAt(s, v.ExpiresAt) || invite.UsableAt(s, at.Add(-time.Nanosecond)) {
		t.Fatal("expiry/source boundary")
	}
	for _, grants := range [][]InvitationGrant{{{Role: "OWNER", Scope: "SELF"}}, {{Role: "DRIVER", Scope: "SELF"}, {Role: "DRIVER", Scope: "SELF"}}, {{Role: "STATION_ADMIN", Scope: "ORG"}}} {
		if _, e := CanonicalInvitationGrants(grants); e == nil {
			t.Fatal("invalid grant plan")
		}
	}
	v.ExpiresAt = v.ExpiresAt.Add(time.Second)
	if _, e := RehydrateInvitation(s, v); e == nil {
		t.Fatal("client chosen validity")
	}
}

func TestInvitationRetainedHistoryRejectsInvalidSources(t *testing.T) {
	scope, p := pendingInvitation(t)
	for _, state := range []InvitationState{InvitationExpired, InvitationSuperseded, InvitationRevoked, InvitationAccepted, InvitationDeclined} {
		v := p
		v.State = state
		v.Revision = 2
		v.ChangedAt = v.ExpiresAt
		v.Cause = string(state)
		if state == InvitationAccepted {
			v.Cause = "PERSON_ACCEPTED"
		}
		if state == InvitationDeclined {
			v.Cause = "PERSON_DECLINED"
		}
		if state == InvitationSuperseded {
			v.Cause = "REPLACED"
		}
		if state == InvitationRevoked {
			v.Cause = "MANAGER_REVOKED"
		}
		if _, e := RehydrateInvitation(scope, v); e != nil {
			t.Fatal("retained terminal value", state, e)
		}
	}
	for _, source := range []struct {
		cause, kind string
		id          string
	}{{"CONTACT_CHANGED", "PROFILE", p.EmployeeID}, {"EMPLOYEE_UNAVAILABLE", "EMPLOYEE", p.EmployeeID}, {"ISSUER_REDUCED", "MEMBERSHIP", p.IssuerMembershipID}, {"RESERVATION_ENDED", "MEMBERSHIP", p.MembershipID}, {"ORGANIZATION_ARCHIVED", "ORGANIZATION", scope.OrgID().String()}} {
		v := p
		v.State = InvitationRevoked
		v.Revision = 2
		v.Cause = source.cause
		v.SourceType = source.kind
		v.SourceID = source.id
		v.SourceRevision = 2
		if _, e := RehydrateInvitation(scope, v); e != nil {
			t.Fatal("causal source", e)
		}
		v.SourceID = p.OperationID
		if _, e := RehydrateInvitation(scope, v); e == nil {
			t.Fatal("unrelated source accepted")
		}
	}
	for name, mutate := range map[string]func(*InvitationSnapshot){
		"id": func(v *InvitationSnapshot) { v.ID = "invalid" }, "employee": func(v *InvitationSnapshot) { v.EmployeeID = "" }, "member": func(v *InvitationSnapshot) { v.MembershipID = "00000000-0000-0000-0000-000000000000" }, "issuer": func(v *InvitationSnapshot) { v.IssuerMembershipID = "INVALID" }, "operation": func(v *InvitationSnapshot) { v.OperationID = " " },
		"state": func(v *InvitationSnapshot) { v.State = "UNKNOWN" }, "revision": func(v *InvitationSnapshot) { v.Revision = 0 }, "settings": func(v *InvitationSnapshot) { v.SettingsRevision = 0 }, "days zero": func(v *InvitationSnapshot) { v.ValidityDays = 0 }, "days upper": func(v *InvitationSnapshot) { v.ValidityDays = 31 }, "creation": func(v *InvitationSnapshot) { v.CreatedAt = time.Time{} }, "expiry": func(v *InvitationSnapshot) { v.ExpiresAt = time.Time{} }, "change": func(v *InvitationSnapshot) { v.ChangedAt = v.CreatedAt.Add(-time.Second) }, "pending cause": func(v *InvitationSnapshot) { v.Cause = "EXPIRED" }, "pending revision": func(v *InvitationSnapshot) { v.Revision = 2 }, "source partial": func(v *InvitationSnapshot) { v.SourceType = "EMPLOYEE" }, "source unknown": func(v *InvitationSnapshot) {
			v.State = InvitationRevoked
			v.Revision = 2
			v.Cause = "EMPLOYEE_UNAVAILABLE"
			v.SourceID = v.EmployeeID
			v.SourceType = "OTHER"
			v.SourceRevision = 2
		}, "expired early": func(v *InvitationSnapshot) { v.State = InvitationExpired; v.Revision = 2; v.Cause = "EXPIRED" }, "replacement cause": func(v *InvitationSnapshot) { v.State = InvitationSuperseded; v.Revision = 2; v.Cause = "EXPIRED" }, "revocation cause": func(v *InvitationSnapshot) { v.State = InvitationRevoked; v.Revision = 2; v.Cause = "UNKNOWN" }, "missing cause": func(v *InvitationSnapshot) { v.State = InvitationRevoked; v.Revision = 2 },
	} {
		t.Run(name, func(t *testing.T) {
			v := p
			mutate(&v)
			if _, e := RehydrateInvitation(scope, v); e == nil {
				t.Fatal("invalid history accepted")
			}
		})
	}
	if _, e := RehydrateInvitation(kernel.OrgScope{}, p); e != ErrScope {
		t.Fatal("missing scope")
	}
}
func TestInvitationPlansAndFormattingArePrivateCopies(t *testing.T) {
	scope, v := pendingInvitation(t)
	value, e := RehydrateInvitation(scope, v)
	if e != nil {
		t.Fatal(e)
	}
	v.Grants[0].Role = "OWNER"
	s, _ := value.Snapshot(scope)
	if s.Grants[0].Role != "DRIVER" {
		t.Fatal("input alias")
	}
	s.Grants[0].Role = "ADMIN"
	s, _ = value.Snapshot(scope)
	if s.Grants[0].Role != "DRIVER" {
		t.Fatal("snapshot alias")
	}
	raw, e := json.Marshal(value)
	if e != nil {
		t.Fatal(e)
	}
	for _, render := range []string{string(raw), fmt.Sprintf("%+v", value), fmt.Sprintf("%#v", value), value.LogValue().String()} {
		if strings.Contains(render, s.ID) || strings.Contains(render, s.EmployeeID) {
			t.Fatal("private identity leaked")
		}
	}
	for _, g := range [][]InvitationGrant{nil, make([]InvitationGrant, 11), {{Role: "DRIVER", Scope: "UNIT"}}, {{Role: "UNKNOWN", Scope: "SELF"}}} {
		if _, e := CanonicalInvitationGrants(g); e == nil {
			t.Fatal("unsupported plan")
		}
	}
	sorted, e := CanonicalInvitationGrants([]InvitationGrant{{Role: "DRIVER", Scope: "SELF"}, {Role: "ADMIN", Scope: "ORG"}})
	if e != nil || sorted[0].Role != "ADMIN" {
		t.Fatal("unstable plan")
	}
}
