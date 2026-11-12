package application_test

import (
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
	"testing"
)

func TestTeamCommandsRejectFabricatedAuthorityAndUnavailablePeriods(t *testing.T) {
	org, _ := kernel.ParseOrgID("7b0e2a54-0000-4000-8000-000000000301")
	scope, _ := kernel.NewOrgScope(org)
	actor := app.CompanyActor{AccountID: "7b0e2a54-0000-4000-8000-000000000401", SessionID: "7b0e2a54-0000-4000-8000-000000000402"}
	input := app.TeamWriteInput{Kind: app.CreateTeam, OperationID: "7b0e2a54-0000-4000-8000-000000001007", Profile: d.TeamProfile{Name: "Synthetic Crew"}}
	c, e := app.NewTeamWrite(input)
	if e != nil {
		t.Fatal(e)
	}
	if r, replay, e := (app.TeamService{Enabled: true}).Write(t.Context(), scope, actor, access.Decision{}, c); e != app.ErrUnauthenticated || replay || r != (app.TeamResult{}) {
		t.Fatal("forged team authority", e)
	}
	if _, _, e := (app.TeamService{}).Write(t.Context(), scope, actor, access.Decision{}, c); e != app.ErrSurfaceDisabled {
		t.Fatal("team enabled by default", e)
	}
	input.EmployeeID = "7b0e2a54-0000-4000-8000-000000000501"
	if _, e := app.NewTeamWrite(input); e != d.ErrInvalid {
		t.Fatal("creation accepted employee field")
	}
	for _, kind := range []app.TeamWriteKind{app.AddTeamEmployee, app.RemoveTeamEmployee} {
		if _, e := app.NewTeamWrite(app.TeamWriteInput{Kind: kind, OperationID: input.OperationID, TeamID: "7b0e2a54-0000-4000-8000-000000000b01", ExpectedRevision: 1}); e != d.ErrInvalid {
			t.Fatal("missing employee")
		}
	}
	store := &adapters.TeamStore{}
	if _, e := store.ListTeamLinks(t.Context(), scope, actor, "7b0e2a54-0000-4000-8000-000000000601", "7b0e2a54-0000-4000-8000-000000000b01", app.TeamLinkFilter{Limit: 1}); e != app.ErrUnavailable {
		t.Fatal("unavailable history read", e)
	}
	if _, e := (app.TeamService{Enabled: true, Repository: store}).Links(t.Context(), scope, actor, access.Decision{}, "7b0e2a54-0000-4000-8000-000000000b01", app.TeamLinkFilter{Limit: 1}); e != app.ErrUnauthenticated {
		t.Fatal("forged history authority", e)
	}
}
