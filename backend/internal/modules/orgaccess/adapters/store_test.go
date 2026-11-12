package adapters_test

import (
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/adapters"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
)

func assignment(t *testing.T, s kernel.OrgScope, id, member string) d.Assignment {
	t.Helper()
	a, err := d.RehydrateAssignment(s, d.AssignmentSnapshot{OrgID: s.OrgID(), ID: id, MembershipID: member, AssignedBy: member, Role: d.OperationsStaff, Scope: d.SelfScope, TargetID: member, ValidFrom: time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC), Reason: "synthetic role", Revision: 1})
	if err != nil {
		t.Fatal(err)
	}
	return a
}
func TestRepositoryRefusesInvalidInputsAndUnavailablePorts(t *testing.T) {
	id, err := kernel.ParseOrgID("7b0e2a54-0000-4000-8000-000000000301")
	if err != nil {
		t.Fatal(err)
	}
	scope, err := kernel.NewOrgScope(id)
	if err != nil {
		t.Fatal(err)
	}
	assignmentID, memberID := "7b0e2a54-0000-4000-8000-000000000701", "7b0e2a54-0000-4000-8000-000000000601"
	a := assignment(t, scope, assignmentID, memberID)
	for _, s := range []*adapters.Store{nil, adapters.NewStore(nil)} {
		var repo app.Repository = s
		var joined app.TransactionRepository = s
		for _, read := range []func() (d.Assignment, error){func() (d.Assignment, error) { return repo.Get(t.Context(), scope, assignmentID) }, func() (d.Assignment, error) { return joined.GetTx(t.Context(), nil, scope, assignmentID) }} {
			if got, err := read(); err != app.ErrUnavailable || !reflect.DeepEqual(got, d.Assignment{}) {
				t.Fatal("unavailable port returned a role or unexpected error")
			}
		}
		for _, read := range []func() ([]d.Assignment, error){func() ([]d.Assignment, error) { return repo.Candidates(t.Context(), scope, memberID) }, func() ([]d.Assignment, error) { return joined.CandidatesTx(t.Context(), nil, scope, memberID) }} {
			if got, err := read(); err != app.ErrUnavailable || got != nil {
				t.Fatal("unavailable port returned candidates or unexpected error")
			}
		}
		for _, write := range []func() (bool, error){func() (bool, error) { return repo.Create(t.Context(), scope, a) }, func() (bool, error) { return joined.CreateTx(t.Context(), nil, scope, a) }} {
			if changed, err := write(); changed || err != app.ErrUnavailable {
				t.Fatal(changed, err)
			}
		}
		for _, write := range []func() (d.Assignment, bool, error){func() (d.Assignment, bool, error) { return repo.End(t.Context(), scope, assignmentID, 1) }, func() (d.Assignment, bool, error) { return joined.EndTx(t.Context(), nil, scope, assignmentID, 1) }} {
			if got, changed, err := write(); changed || err != app.ErrUnavailable || !reflect.DeepEqual(got, d.Assignment{}) {
				t.Fatal("unavailable end returned success or a role")
			}
		}
		if _, err := repo.Get(t.Context(), kernel.OrgScope{}, assignmentID); err != d.ErrAssignmentScope {
			t.Fatal(err)
		}
		if _, err := repo.Get(t.Context(), scope, "bad"); err != d.ErrAssignmentInvalid {
			t.Fatal(err)
		}
		if _, err := repo.Candidates(t.Context(), scope, "bad"); err != d.ErrAssignmentInvalid {
			t.Fatal(err)
		}
		if _, err := joined.CandidatesTx(t.Context(), nil, scope, "bad"); err != d.ErrAssignmentInvalid {
			t.Fatal(err)
		}
		if _, err := repo.Create(t.Context(), scope, d.Assignment{}); err != d.ErrAssignmentScope {
			t.Fatal(err)
		}
		if _, err := joined.CreateTx(t.Context(), nil, scope, d.Assignment{}); err != d.ErrAssignmentScope {
			t.Fatal(err)
		}
		ended, err := a.End(scope, 1, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if _, err := repo.Create(t.Context(), scope, ended); err != d.ErrAssignmentInvalid {
			t.Fatal(err)
		}
		for _, revision := range []int64{0, math.MaxInt64} {
			if _, _, err := repo.End(t.Context(), scope, assignmentID, revision); err != d.ErrAssignmentInvalid {
				t.Fatal(err)
			}
			if _, _, err := joined.EndTx(t.Context(), nil, scope, assignmentID, revision); err != d.ErrAssignmentInvalid {
				t.Fatal(err)
			}
		}
	}
}
