//go:build integration

package adapters_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/adapters"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	auditpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/adapters"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	cappg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/adapters"
	cap "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/domain"
	"github.com/jackc/pgx/v5"
)

type pilotRoot struct{}

func (pilotRoot) Resolve(_ context.Context, s kernel.OrgScope, _ app.Subject) (d.ResourceFacts, error) {
	return d.ResourceFacts{OrgID: s.OrgID(), ID: s.OrgID().String()}, nil
}

type pilotProof struct{}

func (pilotProof) Confirm(context.Context, app.Subject, app.Policy) error { return nil }

type pilotRecords struct{}

func (pilotRecords) Check(context.Context, kernel.OrgScope, app.Subject, app.Policy, d.ResourceFacts, []d.Rule) error {
	return nil
}

func TestPilotReadsCurrentScopedFlagsWithoutGrantingRoles(t *testing.T) {
	f, a, b := fixture(t)
	activate(t, f, a, 601, 701, "OWNER")
	activate(t, f, a, 602, 702, "DRIVER")
	activate(t, f, b, 603, 703, "OWNER")
	role := pgx.Identifier{f.RuntimeRole}.Sanitize()
	if _, err := f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT,UPDATE ON org_capabilities TO "+role+"; GRANT SELECT,INSERT ON org_audit_events TO "+role); err != nil {
		t.Fatal(err)
	}
	catalog, err := cap.NewCatalog([]string{"identity"})
	if err != nil {
		t.Fatal(err)
	}
	auditCatalog, err := audit.NewCatalog(cappg.AuditDefinitions())
	if err != nil {
		t.Fatal(err)
	}
	flags := cappg.NewStore(f.Runtime, catalog, auditpg.NewStore(f.Runtime, auditCatalog))
	gate := adapters.PilotEntitlements{Enabled: true, Catalog: catalog, States: flags}
	current := adapters.NewStore(f.Runtime)
	service := app.Authorization{Source: current, Capabilities: gate, Records: pilotRecords{}, Now: time.Now}
	policy := app.Policy{Permission: "idn.employee.manage", Capability: "identity", Effect: app.ConfigurationWrite}
	check := func(scope kernel.OrgScope, account int, p app.Policy) error {
		ctx, finish := app.BeginRequest(t.Context())
		defer finish()
		pending, err := service.Prepare(ctx, scope, app.Actor{AccountID: key(account), Channel: "android", ContributorID: key(901), DeviceKeyID: key(902)}, p, pilotRoot{})
		if err != nil {
			return err
		}
		_, err = service.Complete(ctx, pending, pilotProof{})
		return err
	}
	set := func(scope kernel.OrgScope, operation, account int, enabled bool, revision int64) {
		t.Helper()
		if _, err := flags.Set(t.Context(), scope, cap.Actor{AccountID: key(account)}, cap.Change{OperationID: key(operation), Capability: "identity", Enabled: enabled, ExpectedRevision: revision}); err != nil {
			t.Fatal(err)
		}
	}
	if err := check(a, 701, policy); err != app.ErrEntitlementRequired {
		t.Fatal("absent capability admitted new work", err)
	}
	set(b, 1001, 703, true, 0)
	if err := check(a, 701, policy); err != app.ErrEntitlementRequired {
		t.Fatal("foreign flag admitted A", err)
	}
	if err := check(b, 703, policy); err != nil {
		t.Fatal("B's own flag refused", err)
	}
	set(a, 1002, 701, true, 0)
	if err := check(a, 701, policy); err != nil {
		t.Fatal(err)
	}
	if err := check(a, 702, policy); !errors.Is(err, d.ErrPermissionDenied) {
		t.Fatal("enabled flag granted DRIVER a management role", err)
	}
	set(a, 1003, 701, false, 1)
	if err := check(a, 701, policy); err != app.ErrEntitlementRequired {
		t.Fatal("stale flag admitted new work", err)
	}
	read := policy
	read.Permission = "idn.employee.read"
	read.Effect = app.Read
	if err := check(a, 701, read); err != nil {
		t.Fatal("rollback hid existing authorized records", err)
	}
	if err := check(b, 703, policy); err != nil {
		t.Fatal("A rollback changed B", err)
	}
	set(a, 1004, 701, true, 2)
	if err := check(a, 701, policy); err != nil {
		t.Fatal("re-enable did not resolve current state", err)
	}
	// Entry acceptance is not the business transaction's lock. A later disable
	// must still stop that transaction before any mutation.
	set(a, 1005, 701, false, 3)
	mutations := 0
	if err := flags.Run(t.Context(), a, "identity", cap.Write, func(context.Context, pgx.Tx) error { mutations++; return nil }); !errors.Is(err, cap.ErrEntitlementRequired) || mutations != 0 {
		t.Fatal("disable raced past transaction guard", mutations, err)
	}
	for _, s := range []kernel.OrgScope{a, b} {
		rows, err := current.Candidates(t.Context(), s, key(601))
		if err != nil {
			t.Fatal(err)
		}
		if s == a && len(rows) != 1 {
			t.Fatal("capability rollback changed assignment history")
		}
	}
	gate.States = cappg.NewStore(nil, catalog, nil)
	service.Capabilities = gate
	if err := check(a, 701, read); err != app.ErrUnavailable {
		t.Fatal("missing storage silently admitted read", err)
	}
	service.Capabilities = adapters.PilotEntitlements{Catalog: catalog, States: flags}
	if err := check(a, 701, read); err != app.ErrEntitlementRequired {
		t.Fatal("default-off surface admitted read", err)
	}
}
