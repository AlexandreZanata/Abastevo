package application_test

import (
	"reflect"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/adapters"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/domain"
)

func TestUnavailableRepositoryReturnsNoIdentityOrWrite(t *testing.T) {
	id, err := kernel.ParseOrgID("7b0e2a54-0000-4000-8000-000000000301")
	if err != nil {
		t.Fatal(err)
	}
	scope, err := kernel.NewOrgScope(id)
	if err != nil {
		t.Fatal(err)
	}
	employeeID, memberID, accountID := "7b0e2a54-0000-4000-8000-000000000501", "7b0e2a54-0000-4000-8000-000000000601", "7b0e2a54-0000-4000-8000-000000000401"
	employee, err := d.NewEmployee(scope, employeeID)
	if err != nil {
		t.Fatal(err)
	}
	member, err := d.NewInvited(scope, memberID, employee, d.ExistingAccount)
	if err != nil {
		t.Fatal(err)
	}
	ended, err := member.Transition(scope, 1, d.MembershipEnded, d.MembershipConditions{EndReason: d.AdmissionEnded})
	if err != nil {
		t.Fatal(err)
	}
	var repo app.Repository = adapters.NewStore(nil)
	var joined app.TransactionRepository = adapters.NewStore(nil)
	for _, read := range []func() (d.Employee, error){
		func() (d.Employee, error) { return repo.GetEmployee(t.Context(), scope, employeeID) },
		func() (d.Employee, error) { return joined.GetEmployeeTx(t.Context(), nil, scope, employeeID) },
	} {
		if got, err := read(); err != app.ErrUnavailable || !reflect.DeepEqual(got, d.Employee{}) {
			t.Fatal("unavailable read returned an employee or unexpected error")
		}
	}
	for _, read := range []func() (d.Membership, error){
		func() (d.Membership, error) { return repo.Get(t.Context(), scope, memberID) },
		func() (d.Membership, error) { return repo.ActiveByAccount(t.Context(), scope, accountID) },
		func() (d.Membership, error) { return joined.GetTx(t.Context(), nil, scope, memberID) },
		func() (d.Membership, error) { return joined.ActiveByAccountTx(t.Context(), nil, scope, accountID) },
	} {
		if got, err := read(); err != app.ErrUnavailable || !reflect.DeepEqual(got, d.Membership{}) {
			t.Fatal("unavailable read returned membership authority or unexpected error")
		}
	}
	for _, write := range []func() (bool, error){
		func() (bool, error) { return repo.CreateEmployee(t.Context(), scope, employee) },
		func() (bool, error) { return repo.Create(t.Context(), scope, member) },
		func() (bool, error) { return repo.Save(t.Context(), scope, 1, ended) },
		func() (bool, error) { return joined.CreateEmployeeTx(t.Context(), nil, scope, employee) },
		func() (bool, error) { return joined.CreateTx(t.Context(), nil, scope, member) },
		func() (bool, error) { return joined.SaveTx(t.Context(), nil, scope, 1, ended) },
	} {
		if changed, err := write(); changed || err != app.ErrUnavailable {
			t.Fatal("unavailable write reported success or unexpected error")
		}
	}
}
