package domain

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
)

func TestEmployeeProfileMinimalPrivateAndSeparateLifecycle(t *testing.T) {
	org, _ := kernel.ParseOrgID("10000000-0000-4000-8000-000000000001")
	scope, _ := kernel.NewOrgScope(org)
	other, _ := kernel.ParseOrgID("20000000-0000-4000-8000-000000000001")
	foreign, _ := kernel.NewOrgScope(other)
	e, err := NewEmployee(scope, "10000000-0000-4000-8000-000000000002")
	if err != nil {
		t.Fatal(err)
	}
	pending, err := RehydrateEmployeeRecord(scope, EmployeeRecordSnapshot{Employee: e})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pending.ChangeProfile(scope, 1, EmployeeProfile{DisplayName: "Synthetic Person"}); err != ErrTransition {
		t.Fatal("pending change", err)
	}
	complete, err := pending.CompleteProfile(scope, 1, EmployeeProfile{DisplayName: "  Synthetic Person  ", WorkEmail: " TEAM@example.invalid ", WorkPhone: "+5511999999999"})
	if err != nil {
		t.Fatal(err)
	}
	v, err := complete.Snapshot(scope)
	if err != nil || v.ProfileRevision != 1 || v.Profile.DisplayName != "Synthetic Person" || v.Profile.WorkEmail != "team@example.invalid" {
		t.Fatal("completion", err)
	}
	identity, _ := v.Employee.Snapshot(scope)
	if identity.Revision != 1 || identity.State != EmployeeActive {
		t.Fatal("profile changed lifecycle")
	}
	if _, err = complete.CompleteProfile(scope, 1, v.Profile); err != ErrTransition {
		t.Fatal("repeat completion", err)
	}
	if _, err = complete.ChangeProfile(scope, 2, v.Profile); err != ErrConflict {
		t.Fatal("stale", err)
	}
	changed, err := complete.ChangeProfile(scope, 1, EmployeeProfile{DisplayName: "New Synthetic Name"})
	if err != nil {
		t.Fatal(err)
	}
	after, _ := changed.Snapshot(scope)
	if after.ProfileRevision != 2 {
		t.Fatal("revision")
	}
	if _, err = changed.Snapshot(foreign); err != ErrScope {
		t.Fatal("foreign", err)
	}
	for _, value := range []any{v, v.Profile, complete} {
		raw, _ := json.Marshal(value)
		if strings.Contains(fmt.Sprintf("%+v %s", value, raw), "Synthetic") {
			t.Fatal("private formatter")
		}
	}
}
func TestEmployeeProfileRejectsExcessAndCredentialShapes(t *testing.T) {
	for _, v := range []EmployeeProfile{{}, {DisplayName: "x\nname"}, {DisplayName: strings.Repeat("a", 201)}, {DisplayName: "Name", RegistrationNumber: strings.Repeat("r", 65)}, {DisplayName: "Name", WorkEmail: "a@@b"}, {DisplayName: "Name", WorkEmail: "a@b c"}, {DisplayName: "Name", WorkPhone: "11999999999"}, {DisplayName: "Name", WorkPhone: "+0012345678"}, {DisplayName: "Name", HomeUnitID: "foreign"}, {DisplayName: "Name", JobTitle: strings.Repeat("j", 121)}} {
		if _, err := CanonicalEmployeeProfile(v); err != ErrInvalid {
			t.Fatal("invalid profile accepted", err)
		}
	}
}
