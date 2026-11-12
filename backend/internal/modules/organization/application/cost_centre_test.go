package application_test

import (
	"errors"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
)

func TestCostCentreCommandsCanonicalizeAndRejectForgedAuthority(t *testing.T) {
	id, err := kernel.ParseOrgID("7b0e2a54-0000-4000-8000-000000000301")
	if err != nil {
		t.Fatal(err)
	}
	scope, err := kernel.NewOrgScope(id)
	if err != nil {
		t.Fatal(err)
	}
	input := app.CostCentreWriteInput{Kind: app.CreateCostCentre, OperationID: "7b0e2a54-0000-4000-8000-000000001006", Profile: d.CostCentreProfile{Name: " Synthetic centre ", Code: " CODE "}}
	command, err := app.NewCostCentreWrite(input)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := command.Snapshot()
	if err != nil || snapshot.Profile.Name != "Synthetic centre" || snapshot.Profile.Code != "CODE" {
		t.Fatal("canonical cost-centre command", err)
	}
	input.Profile = snapshot.Profile
	normalized, err := app.NewCostCentreWrite(input)
	if err != nil || normalized.Digest(scope) != command.Digest(scope) {
		t.Fatal("normalized code/name changed intent", err)
	}
	snapshot.Profile.Code = "Changed copy"
	if again, _ := command.Snapshot(); again.Profile.Code != "CODE" {
		t.Fatal("command snapshot mutated original")
	}
	actor := app.CompanyActor{AccountID: "7b0e2a54-0000-4000-8000-000000000201", SessionID: "7b0e2a54-0000-4000-8000-000000000202"}
	if result, replay, err := (app.CostCentreService{Enabled: true}).Write(t.Context(), scope, actor, access.Decision{}, command); !errors.Is(err, app.ErrUnauthenticated) || replay || result != (app.CostCentreResult{}) {
		t.Fatal("forged cost-centre authority reached storage", err)
	}
	if _, _, err := (app.CostCentreService{}).Write(t.Context(), scope, actor, access.Decision{}, command); !errors.Is(err, app.ErrSurfaceDisabled) {
		t.Fatal("cost-centre service default enabled", err)
	}
	for _, kind := range []app.CostCentreWriteKind{"UNKNOWN", app.ChangeCostCentre, app.ArchiveCostCentre, app.RestoreCostCentre} {
		input.Kind = kind
		if _, err := app.NewCostCentreWrite(input); !errors.Is(err, d.ErrInvalid) {
			t.Fatal("invalid cost-centre operation accepted", kind, err)
		}
	}
	for _, filter := range []app.CostCentreFilter{{}, {Limit: 101}, {Limit: 1, AfterID: "unknown"}, {Limit: 1, State: "DELETED"}} {
		if filter.Validate() == nil {
			t.Fatal("unbounded cost-centre filter accepted")
		}
	}
}
