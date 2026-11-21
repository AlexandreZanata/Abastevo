package main

import (
	"slices"
	"testing"
)

func TestEnterpriseTrialCommandRequiresReviewedGrantAndAccountableOperator(t *testing.T) {
	org, grant, operation := "7b0e2a54-0000-4000-8000-0000000e0111", "7b0e2a54-0000-4000-8000-0000000e0121", "7b0e2a54-0000-4000-8000-0000000e0131"
	args := []string{"start", "--org", org, "--grant-id", grant, "--operation-id", operation}
	operatorEnv := func(name string) string {
		if name == "ANPFUEL_OPERATOR_ID" {
			return testOperator
		}
		return ""
	}
	in, err := parseTrialArgs(args, operatorEnv)
	if err != nil || in.scope.OrgID().String() != org || in.grant != grant || in.operationID != operation || in.operator != testOperator {
		t.Fatalf("reviewed trial input was not preserved: %+v %v", in, err)
	}
	for _, attempted := range [][]string{
		args[:3],
		append(slices.Clone(args), "--price", "100"),
		append(slices.Clone(args), "--days", "10"),
		append(slices.Clone(args), "--operator", "someone"),
	} {
		if _, err := parseTrialArgs(attempted, operatorEnv); err == nil {
			t.Fatalf("unapproved trial command accepted: %v", attempted)
		}
	}
	if _, err := parseTrialArgs(args, func(string) string { return "" }); err == nil {
		t.Fatal("unattributed operator could open a trial")
	}
}
