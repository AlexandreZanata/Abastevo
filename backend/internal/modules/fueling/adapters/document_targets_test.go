package adapters_test

import (
	"context"
	"errors"
	"testing"

	docapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/document/application"
	docdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/document/domain"
	fuelpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fueling/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
)

// EMP-FUEL-011 (EDR-297): the FUELING target is governed by the owner's own
// record permission, never a document key; every other kind is not its own.
func TestFuelingDocumentTargetsPermissionAndKinds(t *testing.T) {
	targets := fuelpg.FuelingDocumentTargets{}
	permission, err := targets.DocumentTargetPermission(docdomain.Fueling)
	if err != nil || permission != "fuel.fueling.record" {
		t.Fatal("FUELING target permission", permission, err)
	}
	for _, kind := range []docdomain.TargetKind{docdomain.Expense, docdomain.MeterReading, docdomain.Asset, docdomain.Incident, docdomain.Trip} {
		if _, err := targets.DocumentTargetPermission(kind); !errors.Is(err, docapp.ErrInvalid) {
			t.Fatal("foreign kind accepted", kind, err)
		}
	}
	org, err := kernel.ParseOrgID("7b0e2a54-0000-4000-8000-000000009502")
	if err != nil {
		t.Fatal(err)
	}
	scope, err := kernel.NewOrgScope(org)
	if err != nil {
		t.Fatal(err)
	}
	// Without the caller's scoped transaction nothing is read.
	if _, err := targets.ResolveDocumentTargetTx(context.Background(), nil, scope, docdomain.Fueling, "7b0e2a54-0000-4000-8000-000000009711"); !errors.Is(err, docapp.ErrUnavailable) {
		t.Fatal("resolved without a transaction", err)
	}
	// A registry entry exists: the DOC-003 policy is the owner's write policy.
	policy, resolver, err := docapp.Targets{docdomain.Fueling: targets}.Policy(docdomain.Fueling, access.OperationalWrite)
	if err != nil || resolver == nil || policy.Permission != "fuel.fueling.record" {
		t.Fatal("registry policy", err, policy.Permission)
	}
}
