package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const (
	testOperator = "7b0e2a54-0000-4000-8000-0000000e0001"
	testOrg      = "7b0e2a54-0000-4000-8000-0000000e0002"
	testOp       = "7b0e2a54-0000-4000-8000-0000000e0003"
)

// B-BR-EMP-560/561: every malformed command is refused before any I/O.
func TestParseEnterpriseCapabilityArgs(t *testing.T) {
	none := func(string) string { return "" }
	set := func(extra ...string) []string {
		return append([]string{"capability", "set", "--org", testOrg, "--capability", "organizations", "--operation-id", testOp, "--expected-revision", "0", "--operator", testOperator}, extra...)
	}
	got, err := parseEnterpriseArgs(set("--enabled"), none)
	if err != nil || got.action != "set" || got.operator != testOperator || got.org.OrgID().String() != testOrg ||
		got.change.Capability != "organizations" || !got.change.Enabled || got.change.OperationID != testOp || got.change.ExpectedRevision != 0 {
		t.Fatal("valid enable refused", got, err)
	}
	if got, err := parseEnterpriseArgs(append(set("--disabled")[:9], "3", "--operator", testOperator, "--disabled"), none); err != nil || got.change.Enabled || got.change.ExpectedRevision != 3 {
		t.Fatal("valid disable refused", got, err)
	}
	env := func(key string) string {
		if key == "ANPFUEL_OPERATOR_ID" {
			return testOperator
		}
		return ""
	}
	show := []string{"capability", "show", "--org", testOrg, "--capability", "organizations"}
	if got, err := parseEnterpriseArgs(show, env); err != nil || got.action != "show" || got.operator != testOperator || got.change.Capability != "organizations" {
		t.Fatal("show with the operator from the environment refused", got, err)
	}
	upper := strings.ToUpper(testOrg)
	for name, args := range map[string][]string{
		"no subcommand":           {},
		"unknown group":           {"capabilities", "set"},
		"unknown action":          {"capability", "toggle", "--org", testOrg},
		"anonymous":               {"capability", "show", "--org", testOrg, "--capability", "organizations"},
		"operator not a uuid":     append(show, "--operator", "op-1"),
		"missing org":             {"capability", "show", "--capability", "organizations", "--operator", testOperator},
		"uppercase org":           {"capability", "show", "--org", upper, "--capability", "organizations", "--operator", testOperator},
		"zero org":                {"capability", "show", "--org", "00000000-0000-0000-0000-000000000000", "--capability", "organizations", "--operator", testOperator},
		"unknown capability":      {"capability", "show", "--org", testOrg, "--capability", "fleet", "--operator", testOperator},
		"missing capability":      {"capability", "show", "--org", testOrg, "--operator", testOperator},
		"show with a state flag":  append(append([]string{}, show...), "--operator", testOperator, "--enabled"),
		"show with an operation":  append(append([]string{}, show...), "--operator", testOperator, "--operation-id", testOp),
		"set without a state":     set(),
		"set with both states":    set("--enabled", "--disabled"),
		"set without operation":   {"capability", "set", "--org", testOrg, "--capability", "organizations", "--expected-revision", "0", "--operator", testOperator, "--enabled"},
		"set with a bad op":       {"capability", "set", "--org", testOrg, "--capability", "organizations", "--operation-id", "x", "--expected-revision", "0", "--operator", testOperator, "--enabled"},
		"set without revision":    {"capability", "set", "--org", testOrg, "--capability", "organizations", "--operation-id", testOp, "--operator", testOperator, "--enabled"},
		"set with a negative rev": {"capability", "set", "--org", testOrg, "--capability", "organizations", "--operation-id", testOp, "--expected-revision", "-1", "--operator", testOperator, "--enabled"},
		"set with a text rev":     {"capability", "set", "--org", testOrg, "--capability", "organizations", "--operation-id", testOp, "--expected-revision", "one", "--operator", testOperator, "--enabled"},
		"set with a reason":       set("--enabled", "--reason", "pilot"),
		"positional arguments":    set("--enabled", "extra"),
	} {
		if _, err := parseEnterpriseArgs(args, none); err == nil {
			t.Fatal("invalid capability command accepted:", name)
		}
	}
	if err := run([]string{"enterprise", "capability", "show", "--org", testOrg, "--capability", "organizations"}, none); err == nil || strings.Contains(err.Error(), "unknown command") {
		t.Fatal("enterprise command not dispatched or anonymous accepted", err)
	}
}

// B-BR-EMP-562: the production api image ships the operator tool at /ops.
func TestProductionAPIImageShipsTheOperatorTool(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "infra", "docker", "Dockerfile"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if !regexp.MustCompile(`go build -trimpath -ldflags="-s -w" -o /out/ops \./cmd/ops`).MatchString(text) {
		t.Fatal("the builder does not compile cmd/ops")
	}
	api := text[strings.Index(text, "AS api"):]
	api = api[:strings.Index(api, "AS worker")]
	if !strings.Contains(api, "COPY --from=builder --chown=65532:65532 /out/ops /ops") || !strings.Contains(api, "USER 65532:65532") || !strings.Contains(api, `ENTRYPOINT ["/api"]`) {
		t.Fatal("the api image does not ship /ops as the non-root user with the api entrypoint")
	}
}
