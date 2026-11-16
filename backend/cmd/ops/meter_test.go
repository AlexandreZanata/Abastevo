package main

import (
	"strings"
	"testing"
)

// EMP-METER-006: the projection rebuild needs an operator and one canonical
// organization, and is reachable from the tool's dispatch.
func TestParseMeterArgs(t *testing.T) {
	env := func(map[string]string) func(string) string {
		return func(string) string { return "" }
	}
	const org = "7b0e2a54-0000-4000-8000-000000008001"
	got, err := parseMeterArgs([]string{"rebuild-projection", "--org", org, "--operator", "op-1"}, env(nil))
	if err != nil || got.operator != "op-1" || got.org.OrgID().String() != org {
		t.Fatal("valid rebuild refused", got, err)
	}
	withEnv := func(key string) string {
		if key == "ANPFUEL_OPERATOR_ID" {
			return "op-env"
		}
		return ""
	}
	if got, err := parseMeterArgs([]string{"rebuild-projection", "--org", org}, withEnv); err != nil || got.operator != "op-env" {
		t.Fatal("operator from the environment refused", err)
	}
	for name, args := range map[string][]string{
		"no subcommand":   {},
		"unknown":         {"rebuild", "--org", org, "--operator", "op-1"},
		"anonymous":       {"rebuild-projection", "--org", org},
		"missing org":     {"rebuild-projection", "--operator", "op-1"},
		"uppercase org":   {"rebuild-projection", "--org", strings.ToUpper(org), "--operator", "op-1"},
		"zero org":        {"rebuild-projection", "--org", "00000000-0000-0000-0000-000000000000", "--operator", "op-1"},
		"positional args": {"rebuild-projection", "--org", org, "--operator", "op-1", "extra"},
	} {
		if _, err := parseMeterArgs(args, env(nil)); err == nil {
			t.Fatal("invalid rebuild accepted", name)
		}
	}
	if err := run([]string{"meter", "rebuild-projection"}, env(nil)); err == nil || strings.Contains(err.Error(), "unknown command") {
		t.Fatal("meter command not dispatched", err)
	}
}
