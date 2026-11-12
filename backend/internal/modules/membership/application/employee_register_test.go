package application_test

import (
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/adapters"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/domain"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
)

func TestEmployeeRegisterPortsFailClosedAndRequireCurrentDecision(t *testing.T) {
	org, _ := kernel.ParseOrgID("7b0e2a54-0000-4000-8000-000000000301")
	scope, _ := kernel.NewOrgScope(org)
	id := "7b0e2a54-0000-4000-8000-000000000501"
	member := "7b0e2a54-0000-4000-8000-000000000601"
	actor := app.EmployeeActor{AccountID: "7b0e2a54-0000-4000-8000-000000000401", SessionID: "7b0e2a54-0000-4000-8000-000000000402"}
	c, e := app.NewEmployeeWrite(app.EmployeeWriteInput{Kind: app.CreateEmployee, OperationID: "7b0e2a54-0000-4000-8000-000000001002", Profile: d.EmployeeProfile{DisplayName: "Synthetic Employee"}})
	if e != nil {
		t.Fatal(e)
	}
	store := &adapters.EmployeeStore{}
	if r, replay, e := store.WriteEmployee(t.Context(), scope, actor, member, id, c); e != app.ErrRegisterUnavailable || replay || r != (app.EmployeeResult{}) {
		t.Fatal("unavailable storage acknowledged")
	}
	for _, f := range []func() error{
		func() error { _, e := store.FindEmployee(t.Context(), scope, id); return e },
		func() error { _, e := store.ReadEmployee(t.Context(), scope, actor, member, id); return e },
		func() error {
			_, e := store.ListEmployees(t.Context(), scope, actor, member, app.EmployeeFilter{Limit: 1})
			return e
		},
		func() error {
			_, e := store.MatchEmployees(t.Context(), scope, actor, member, app.EmployeeMatch{Profile: d.EmployeeProfile{DisplayName: "Synthetic"}, Limit: 1})
			return e
		},
	} {
		if e := f(); e != app.ErrRegisterUnavailable {
			t.Fatal("port exposed provisional state", e)
		}
	}
	service := app.EmployeeService{Enabled: true, Repository: store}
	if _, _, e := service.Write(t.Context(), scope, actor, access.Decision{}, c); e != app.ErrEmployeeUnauthenticated {
		t.Fatal("fabricated decision")
	}
	if _, e := service.Read(t.Context(), scope, actor, access.Decision{}, id); e != app.ErrEmployeeUnauthenticated {
		t.Fatal("fabricated read decision")
	}
	if _, e := service.List(t.Context(), scope, actor, access.Decision{}, app.EmployeeFilter{Limit: 1}); e != app.ErrEmployeeUnauthenticated {
		t.Fatal("fabricated list decision")
	}
	if _, e := service.Matches(t.Context(), scope, actor, access.Decision{}, app.EmployeeMatch{Profile: d.EmployeeProfile{DisplayName: "Synthetic"}, Limit: 1}); e != app.ErrEmployeeUnauthenticated {
		t.Fatal("fabricated review decision")
	}
	if _, _, e := (app.EmployeeService{}).Write(t.Context(), scope, actor, access.Decision{}, c); e != app.ErrRegisterDisabled {
		t.Fatal("enabled by default")
	}
	v, _ := c.Snapshot()
	v.AllowPossibleDuplicate = true
	ack, e := app.NewEmployeeWrite(v)
	if e != nil || ack.Digest(scope) == c.Digest(scope) {
		t.Fatal("explicit review acknowledgement omitted from intent")
	}
}
