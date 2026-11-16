package application_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/maintenance/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/maintenance/domain/checklist"
)

// EDR-284/285: the run policies are fixed and equal to the existing ones.
func TestRunPoliciesAreFixed(t *testing.T) {
	if p := app.RunPolicy(app.RunWrite); p.Permission != "check.checklist.execute" || !p.Effect.Write() || p.Fresh {
		t.Fatal("run write policy", p)
	}
	if p := app.RunPolicy(app.RunReceipt); p.Permission != "check.checklist.execute" || p.Effect.Write() {
		t.Fatal("run receipt policy", p)
	}
	if app.RunCompanyPolicy(app.RunRead) != app.Policy(app.ApplicableRead) || app.RunCompanyPolicy(app.RunRead).Permission != "fleet.asset.read" {
		t.Fatal("run read is not the applicable read")
	}
	if p := app.RunCompanyPolicy(app.RunRelease); p != app.IncidentPolicy(app.IncidentDecide) || p.Permission != "check.incident.manage" || !p.Fresh {
		t.Fatal("run release is not the fresh incident handling policy", p)
	}
	if app.RunPolicy(0).Permission != "" || app.RunCompanyPolicy(0).Permission != "" {
		t.Fatal("unknown action has a policy")
	}
}

// B-BR-EMP-442: the release shape; state, revision and separation are the
// store's; the digest binds every field.
func TestReleaseCommandValidatesAndDigests(t *testing.T) {
	op, run := "9c000000-0000-4000-8000-000000004001", "9c000000-0000-4000-8000-000000004002"
	ok := app.ReleaseInput{OperationID: op, RunID: run, ExpectedRevision: 1, Reason: "Freios verificados"}
	command, err := app.NewReleaseCommand(ok)
	if err != nil {
		t.Fatal(err)
	}
	for name, edit := range map[string]func(*app.ReleaseInput){
		"operation": func(v *app.ReleaseInput) { v.OperationID = "x" },
		"run":       func(v *app.ReleaseInput) { v.RunID = strings.ToUpper(run) },
		"revision":  func(v *app.ReleaseInput) { v.ExpectedRevision = 0 },
		"above":     func(v *app.ReleaseInput) { v.ExpectedRevision = 3 },
		"blank":     func(v *app.ReleaseInput) { v.Reason = "   " },
		"empty":     func(v *app.ReleaseInput) { v.Reason = "" },
		"control":   func(v *app.ReleaseInput) { v.Reason = "a\nb" },
		"long":      func(v *app.ReleaseInput) { v.Reason = strings.Repeat("a", 501) },
	} {
		v := ok
		edit(&v)
		if _, err := app.NewReleaseCommand(v); !errors.Is(err, checklist.ErrInvalid) {
			t.Fatal("invalid release accepted", name, err)
		}
	}
	org, err := kernel.ParseOrgID("9c000000-0000-4000-8000-000000004003")
	if err != nil {
		t.Fatal(err)
	}
	scope, err := kernel.NewOrgScope(org)
	if err != nil {
		t.Fatal(err)
	}
	digest := command.Digest(scope)
	other := ok
	other.Reason = "Outro motivo"
	otherCommand, _ := app.NewReleaseCommand(other)
	if len(digest) != 64 || digest == otherCommand.Digest(scope) || command.Digest(kernel.OrgScope{}) != "" {
		t.Fatal("digest does not bind the reason or the scope")
	}
}
