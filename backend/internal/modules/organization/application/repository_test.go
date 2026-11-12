package application_test

import (
	"reflect"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
)

// Exercise the production adapter through the ports consumed by commands.
// An unavailable repository must never return a usable root or success flag.
func TestUnavailableRepositoryReturnsNoOrganizationOrWrite(t *testing.T) {
	id, err := kernel.ParseOrgID("7b0e2a54-0000-4000-8000-000000000301")
	if err != nil {
		t.Fatal(err)
	}
	scope, err := kernel.NewOrgScope(id)
	if err != nil {
		t.Fatal(err)
	}
	value, err := d.NewPending(scope, d.Profile{Name: "Synthetic company", TimeZone: "UTC", Kinds: []d.Kind{d.Other}})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := value.Snapshot(scope)
	if err != nil {
		t.Fatal(err)
	}
	snapshot.Profile.Name = "Updated synthetic company"
	next, err := value.ConfigurePending(scope, 1, snapshot.Profile)
	if err != nil {
		t.Fatal(err)
	}
	var repo app.Repository = adapters.NewStore(nil)
	got, err := repo.Get(t.Context(), scope)
	if err != app.ErrUnavailable || !reflect.DeepEqual(got, d.Organization{}) {
		t.Fatal("unavailable read returned a root or unexpected error")
	}
	for _, write := range []func() (bool, error){
		func() (bool, error) { return repo.Create(t.Context(), scope, value) },
		func() (bool, error) { return repo.Save(t.Context(), scope, 1, next) },
	} {
		if changed, err := write(); changed || err != app.ErrUnavailable {
			t.Fatal("unavailable write reported success or unexpected error")
		}
	}
	var joined app.TransactionRepository = adapters.NewStore(nil)
	got, err = joined.GetTx(t.Context(), nil, scope)
	if err != app.ErrUnavailable || !reflect.DeepEqual(got, d.Organization{}) {
		t.Fatal("missing transaction returned a root or unexpected error")
	}
	for _, write := range []func() (bool, error){
		func() (bool, error) { return joined.CreateTx(t.Context(), nil, scope, value) },
		func() (bool, error) { return joined.SaveTx(t.Context(), nil, scope, 1, next) },
	} {
		if changed, err := write(); changed || err != app.ErrUnavailable {
			t.Fatal("missing transaction reported success or unexpected error")
		}
	}
}
