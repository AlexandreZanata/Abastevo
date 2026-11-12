package application_test

import (
	"errors"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
)

func TestOperatingUnitCommandsPreserveDefaultIntentAndRejectAuthorityFabrication(t *testing.T) {
	id, err := kernel.ParseOrgID("7b0e2a54-0000-4000-8000-000000000301")
	if err != nil {
		t.Fatal(err)
	}
	scope, err := kernel.NewOrgScope(id)
	if err != nil {
		t.Fatal(err)
	}
	input := app.UnitWriteInput{Kind: app.CreateUnit, OperationID: "7b0e2a54-0000-4000-8000-000000001005", Profile: d.UnitProfile{Name: " Synthetic North ", Code: " N01 "}}
	command, err := app.NewUnitWrite(input)
	if err != nil {
		t.Fatal(err)
	}
	value, err := command.Snapshot()
	if err != nil || value.Profile.Name != "Synthetic North" || value.Profile.TimeZone != "" {
		t.Fatal("default time-zone intent lost", err)
	}
	input.Profile.TimeZone = "UTC"
	explicit, err := app.NewUnitWrite(input)
	if err != nil || explicit.Digest(scope) == command.Digest(scope) {
		t.Fatal("omitted zone incorrectly equals explicit UTC", err)
	}
	actor := app.CompanyActor{AccountID: "7b0e2a54-0000-4000-8000-000000000201", SessionID: "7b0e2a54-0000-4000-8000-000000000202"}
	if result, replay, err := (app.UnitService{Enabled: true}).Write(t.Context(), scope, actor, access.Decision{}, command); !errors.Is(err, app.ErrUnauthenticated) || replay || result != (app.UnitResult{}) {
		t.Fatal("forged decision reached storage", err)
	}
	if _, _, err := (app.UnitService{}).Write(t.Context(), scope, actor, access.Decision{}, command); !errors.Is(err, app.ErrSurfaceDisabled) {
		t.Fatal("default enabled", err)
	}
	for _, kind := range []app.UnitWriteKind{"UNKNOWN", app.ArchiveUnit, app.RestoreUnit, app.ChangeUnit} {
		input.Kind = kind
		if _, err := app.NewUnitWrite(input); !errors.Is(err, d.ErrInvalid) {
			t.Fatal("invalid unit command accepted", kind, err)
		}
	}
	if !errors.Is(app.UnitInUse(d.UnitUsage{Assets: 1}), d.ErrStructureInUse) || !errors.Is(app.UnitInUse(d.UnitUsage{}), app.ErrUnavailable) || !errors.Is(app.DuplicateUnit("unknown"), app.ErrUnavailable) {
		t.Fatal("unsafe unit refusal details")
	}
}
