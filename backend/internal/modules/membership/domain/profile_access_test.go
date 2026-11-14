package domain

import (
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"testing"
)

func TestEmployeeReadAccessStateIsDescriptiveAndScoped(t *testing.T) {
	id, _ := kernel.ParseOrgID("10000000-0000-4000-8000-000000000001")
	scope, _ := kernel.NewOrgScope(id)
	employee, _ := NewEmployee(scope, "10000000-0000-4000-8000-000000000002")
	for _, state := range []EmployeeAccessState{"", "NONE", "INVITED", "ACTIVE", "SUSPENDED"} {
		record, e := RehydrateEmployeeRecord(scope, EmployeeRecordSnapshot{Employee: employee, AccessState: state})
		if e != nil {
			t.Fatal(e)
		}
		view, e := record.Snapshot(scope)
		if e != nil || view.AccessState != state {
			t.Fatal("lost descriptive access state", e)
		}
		identity, _ := view.Employee.Snapshot(scope)
		if identity.State != EmployeeActive {
			t.Fatal("access metadata changed employee lifecycle")
		}
	}
	for _, state := range []EmployeeAccessState{"ENDED", "UNKNOWN", "active", "OWNER"} {
		if _, e := RehydrateEmployeeRecord(scope, EmployeeRecordSnapshot{Employee: employee, AccessState: state}); e != ErrInvalid {
			t.Fatal("invalid access state", e)
		}
	}
	otherID, _ := kernel.ParseOrgID("20000000-0000-4000-8000-000000000001")
	other, _ := kernel.NewOrgScope(otherID)
	record, _ := RehydrateEmployeeRecord(scope, EmployeeRecordSnapshot{Employee: employee, AccessState: "ACTIVE"})
	if _, e := record.Snapshot(other); e != ErrScope {
		t.Fatal("foreign view", e)
	}
}
