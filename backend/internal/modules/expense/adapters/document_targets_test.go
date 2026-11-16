package adapters_test

import (
	"context"
	"errors"
	"testing"

	docapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/document/application"
	docdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/document/domain"
	expensepg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/expense/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
)

// EMP-EXP-004 (EDR-338): the EXPENSE target is governed by the owner's own
// record permission, never a document key; every other kind is not its own.
func TestExpenseDocumentTargetsPermissionAndKinds(t *testing.T) {
	targets := expensepg.ExpenseDocumentTargets{}
	permission, err := targets.DocumentTargetPermission(docdomain.Expense)
	if err != nil || permission != "exp.expense.record" {
		t.Fatal("EXPENSE target permission", permission, err)
	}
	for _, kind := range []docdomain.TargetKind{docdomain.Fueling, docdomain.MeterReading, docdomain.Asset, docdomain.Incident, docdomain.Trip, docdomain.WorkOrder} {
		if _, err := targets.DocumentTargetPermission(kind); !errors.Is(err, docapp.ErrInvalid) {
			t.Fatal("foreign kind accepted", kind, err)
		}
	}
	org, err := kernel.ParseOrgID("7b0e2a54-0000-4000-8000-000000009503")
	if err != nil {
		t.Fatal(err)
	}
	scope, err := kernel.NewOrgScope(org)
	if err != nil {
		t.Fatal(err)
	}
	// Without the caller's scoped transaction nothing is read.
	if _, err := targets.ResolveDocumentTargetTx(context.Background(), nil, scope, docdomain.Expense, "7b0e2a54-0000-4000-8000-000000009712"); !errors.Is(err, docapp.ErrUnavailable) {
		t.Fatal("resolved without a transaction", err)
	}
	// A registry entry exists: the DOC-003 policy is the owner's write policy.
	policy, resolver, err := docapp.Targets{docdomain.Expense: targets}.Policy(docdomain.Expense, access.OperationalWrite)
	if err != nil || resolver == nil || policy.Permission != "exp.expense.record" {
		t.Fatal("registry policy", err, policy.Permission)
	}
}
