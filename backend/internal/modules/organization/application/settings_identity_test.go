package application_test

import (
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
)

func TestSettingsIntentIncludesInvitationConfirmation(t *testing.T) {
	id, _ := kernel.ParseOrgID("7b0e2a54-0000-4000-8000-000000000301")
	scope, _ := kernel.NewOrgScope(id)
	values, err := d.DefaultSettingsValues("America/Cuiaba", "7b0e2a54-0000-4000-8000-000000000601")
	if err != nil {
		t.Fatal(err)
	}
	input := app.SettingsWriteInput{OperationID: "7b0e2a54-0000-4000-8000-000000001001", ExpectedRevision: 1, Values: values, Reason: "Synthetic complete settings intent"}
	first, err := app.NewSettingsWrite(input)
	if err != nil {
		t.Fatal(err)
	}
	input.Values.Policies.RequireInvitationConfirmation = !input.Values.Policies.RequireInvitationConfirmation
	second, err := app.NewSettingsWrite(input)
	if err != nil {
		t.Fatal(err)
	}
	if first.Digest(scope) == second.Digest(scope) {
		t.Fatal("distinct invitation confirmation policies have identical immutable intent")
	}
}
