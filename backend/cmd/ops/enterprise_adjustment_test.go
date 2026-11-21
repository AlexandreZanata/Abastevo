package main

import (
	"errors"
	"testing"
)

func TestAdjustmentCommandAcceptsOnlyExistingReviewedApproval(t *testing.T) {
	const org = "7b0e2a54-0000-4000-8000-000000000001"
	const approval = "7b0e2a54-0000-4000-8000-000000000002"
	const operation = "7b0e2a54-0000-4000-8000-000000000003"
	const operator = "7b0e2a54-0000-4000-8000-000000000004"
	env := func(key string) string {
		if key == "ANPFUEL_OPERATOR_ID" {
			return operator
		}
		return ""
	}
	args := []string{"post", "--org", org, "--approval-id", approval, "--operation-id", operation}
	parsed, err := parseAdjustmentArgs(args, env)
	if err != nil || parsed.scope.OrgID().String() != org || parsed.approval != approval || parsed.operation != operation || parsed.operator != operator {
		t.Fatalf("reviewed command did not preserve its scope: %v", err)
	}
	if _, err := parseAdjustmentArgs(append(args, "--amount-centavos", "4567"), env); !errors.Is(err, errAdjustmentUsage) {
		t.Fatalf("command accepted an operator-selected amount: %v", err)
	}
	if _, err := parseAdjustmentArgs(args, func(string) string { return "" }); err == nil {
		t.Fatal("command accepted an anonymous operator")
	}
}
