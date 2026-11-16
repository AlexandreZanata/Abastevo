//go:build integration

package main

import (
	"bytes"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	capdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
)

func capabilityCommand(t *testing.T, org, action string, extra ...string) enterpriseArgs {
	t.Helper()
	args := append([]string{"capability", action, "--org", org, "--capability", "organizations", "--operator", testOperator}, extra...)
	parsed, err := parseEnterpriseArgs(args, func(string) string { return "" })
	if err != nil {
		t.Fatal("synthetic command refused", args, err)
	}
	return parsed
}

func setCommand(t *testing.T, org, operation string, enabled bool, expected int64) enterpriseArgs {
	t.Helper()
	state := "--disabled"
	if enabled {
		state = "--enabled"
	}
	return capabilityCommand(t, org, "set", "--operation-id", operation, "--expected-revision", strconv.FormatInt(expected, 10), state)
}

type capabilityFacts struct{ rows, enabledEvents, disabledEvents int }

// facts reads, as the migration owner, the stored switch rows and the audit
// events of one organization: what the command wrote, independently of it.
func facts(t *testing.T, admin *pgxpool.Pool, org string) capabilityFacts {
	t.Helper()
	var f capabilityFacts
	if err := admin.QueryRow(t.Context(), `SELECT
		(SELECT count(*) FROM org_capabilities WHERE org_id=$1),
		(SELECT count(*) FROM org_audit_events WHERE org_id=$1 AND action='capability.enabled'),
		(SELECT count(*) FROM org_audit_events WHERE org_id=$1 AND action='capability.disabled')`, org).Scan(&f.rows, &f.enabledEvents, &f.disabledEvents); err != nil {
		t.Fatal(err)
	}
	return f
}

// B-BR-EMP-560/561, BUC-EMP-287: the operator command through the audited
// capability service on real PostGIS, as the role provisioned only by
// infra/db/runtime-role.sql, and refused as the migration owner.
func TestEnterpriseCapabilityCommandOnTheRuntimeRole(t *testing.T) {
	f := orgtest.New(t)
	ctx := t.Context()
	role, secret := f.ProvisionScriptRole(t)
	pool := f.ScriptRuntimePool(t, role, secret, 4)
	org, other := f.Organizations[0].ID, f.Organizations[1].ID
	opA, opB, opC := "7b0e2a54-0000-4000-8000-0000000e00a1", "7b0e2a54-0000-4000-8000-0000000e00b1", "7b0e2a54-0000-4000-8000-0000000e00c1"

	shown, err := executeCapability(ctx, pool, capabilityCommand(t, org, "show"))
	if err != nil || shown.enabled || shown.revision != 0 || shown.replayed || shown.org != org || shown.capability != "organizations" {
		t.Fatal("absent switch must read as disabled at revision 0", shown, err)
	}

	t.Run("the migration owner is refused before any read or write", func(t *testing.T) {
		for _, in := range []enterpriseArgs{capabilityCommand(t, org, "show"), setCommand(t, org, opA, true, 0)} {
			if _, err := executeCapability(ctx, f.Admin, in); !errors.Is(err, errUnsafeLogin) {
				t.Fatal("owner login accepted", err)
			}
		}
		if got := facts(t, f.Admin, org); got != (capabilityFacts{}) {
			t.Fatal("refused owner command wrote", got)
		}
	})

	enabled, err := executeCapability(ctx, pool, setCommand(t, org, opA, true, 0))
	if err != nil || !enabled.enabled || enabled.revision != 1 || enabled.replayed {
		t.Fatal("first enable", enabled, err)
	}
	if got := facts(t, f.Admin, org); got != (capabilityFacts{rows: 1, enabledEvents: 1}) {
		t.Fatal("enable did not write the state and one audit event", got)
	}
	var actor, reason, target string
	if err := f.Admin.QueryRow(ctx, `SELECT actor_id::text, reason_code, target_id::text FROM org_audit_events WHERE org_id=$1 AND operation_id=$2`, org, opA).Scan(&actor, &reason, &target); err != nil ||
		actor != testOperator || reason != "operator.request" || target != org {
		t.Fatal("audit event attribution", actor, reason, target, err)
	}

	again, err := executeCapability(ctx, pool, setCommand(t, org, opA, true, 0))
	if err != nil || !again.replayed || !again.enabled || again.revision != 1 {
		t.Fatal("same operation and command must replay", again, err)
	}
	if got := facts(t, f.Admin, org); got != (capabilityFacts{rows: 1, enabledEvents: 1}) {
		t.Fatal("replay wrote again", got)
	}
	if _, err := executeCapability(ctx, pool, setCommand(t, org, opA, false, 1)); !errors.Is(err, capdomain.ErrReused) {
		t.Fatal("same operation with another command accepted", err)
	}
	if _, err := executeCapability(ctx, pool, setCommand(t, org, opB, false, 0)); !errors.Is(err, capdomain.ErrConflict) {
		t.Fatal("stale expected revision accepted", err)
	}
	if _, err := executeCapability(ctx, pool, setCommand(t, org, opB, false, 7)); !errors.Is(err, capdomain.ErrConflict) {
		t.Fatal("future expected revision accepted", err)
	}
	if got := facts(t, f.Admin, org); got != (capabilityFacts{rows: 1, enabledEvents: 1}) {
		t.Fatal("refused changes wrote", got)
	}
	if current, err := executeCapability(ctx, pool, capabilityCommand(t, org, "show")); err != nil || !current.enabled || current.revision != 1 {
		t.Fatal("refusals changed the state", current, err)
	}

	disabled, err := executeCapability(ctx, pool, setCommand(t, org, opC, false, 1))
	if err != nil || disabled.enabled || disabled.revision != 2 || disabled.replayed {
		t.Fatal("disable", disabled, err)
	}
	if got := facts(t, f.Admin, org); got != (capabilityFacts{rows: 1, enabledEvents: 1, disabledEvents: 1}) {
		t.Fatal("disable did not write one audit event", got)
	}

	unknown := "7b0e2a54-0000-4000-8000-0000000e0fff"
	for _, in := range []enterpriseArgs{capabilityCommand(t, unknown, "show"), setCommand(t, unknown, "7b0e2a54-0000-4000-8000-0000000e00d1", true, 0)} {
		if _, err := executeCapability(ctx, pool, in); !errors.Is(err, errOrganizationNotFound) {
			t.Fatal("unknown organization not refused as not found", err)
		}
	}
	if got := facts(t, f.Admin, unknown); got != (capabilityFacts{}) {
		t.Fatal("unknown organization received rows", got)
	}
	if got := facts(t, f.Admin, other); got != (capabilityFacts{}) {
		t.Fatal("another organization was affected", got)
	}
	if view, err := executeCapability(ctx, pool, capabilityCommand(t, other, "show")); err != nil || view.enabled || view.revision != 0 {
		t.Fatal("another organization's switch changed", view, err)
	}

	// The real entry: configuration from the environment, the runtime DSN,
	// the operator from ANPFUEL_OPERATOR_ID and one bounded output line.
	dsn, err := url.Parse(f.AdminDSN)
	if err != nil {
		t.Fatal(err)
	}
	dsn.User = url.UserPassword(role, secret)
	t.Setenv("ANPFUEL_ENV", "development")
	t.Setenv("ANPFUEL_DATABASE_URL", dsn.String())
	env := func(key string) string {
		if key == "ANPFUEL_OPERATOR_ID" {
			return testOperator
		}
		return ""
	}
	var out bytes.Buffer
	if err := runEnterprise([]string{"capability", "set", "--org", other, "--capability", "organizations", "--enabled", "--operation-id", "7b0e2a54-0000-4000-8000-0000000e00e1", "--expected-revision", "0"}, env, &out); err != nil {
		t.Fatal("entry point", err)
	}
	want := "organization " + other + " capability organizations enabled=true revision=1 replayed=false\n"
	if out.String() != want || strings.Contains(out.String(), secret) || strings.Contains(out.String(), role) {
		t.Fatalf("entry output %q", out.String())
	}
	if got := facts(t, f.Admin, other); got != (capabilityFacts{rows: 1, enabledEvents: 1}) {
		t.Fatal("entry point did not write through the audited service", got)
	}
	if got := facts(t, f.Admin, org); got != (capabilityFacts{rows: 1, enabledEvents: 1, disabledEvents: 1}) {
		t.Fatal("the first organization changed", got)
	}
	t.Setenv("ANPFUEL_DATABASE_URL", f.AdminDSN)
	out.Reset()
	if err := runEnterprise([]string{"capability", "show", "--org", org, "--capability", "organizations"}, env, &out); !errors.Is(err, errUnsafeLogin) || out.Len() != 0 {
		t.Fatal("entry point accepted the owner DSN", err, out.String())
	}
}
