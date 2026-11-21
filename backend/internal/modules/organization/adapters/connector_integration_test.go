//go:build integration

package adapters_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters"
	auditpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/adapters"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	cappg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/adapters"
	capability "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/domain"
	connpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgconnector/adapters"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgconnector/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgconnector/domain"
	"github.com/jackc/pgx/v5"
)

func TestReviewedConnectorLifecycleAndScopedUse(t *testing.T) {
	fx, _, _ := lifecycleFixture(t)
	ctx := t.Context()
	pool := fx.f.RuntimePool(t, 4)
	role := pgx.Identifier{fx.f.RuntimeRole}.Sanitize()
	if _, err := fx.f.Admin.Exec(ctx, "GRANT SELECT ON org_connector_approvals TO "+role+
		"; GRANT SELECT,INSERT,UPDATE ON org_connections TO "+role); err != nil {
		t.Fatal(err)
	}
	known, err := capability.NewCatalog([]string{"organizations", "integrations"})
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := audit.NewCatalog(append(connpg.AuditDefinitions(), cappg.AuditDefinitions()...))
	if err != nil {
		t.Fatal(err)
	}
	audits := auditpg.NewStore(pool, catalog)
	flags := cappg.NewStore(pool, known, audits)
	store := connpg.Store{Pool: pool, Audit: audits,
		Authority: adapters.ConnectorAuthority{Memberships: fx.store.Memberships, Roles: fx.store.Roles, Capabilities: flags}}
	access := connpg.Access{Pool: pool, Flags: flags}
	org := fx.scope.OrgID().String()
	at := lifecycleDBNow(t, fx)
	approval, ready, credential, asset := onboardingID(t), onboardingID(t), onboardingID(t), onboardingID(t)
	for _, fixture := range []struct {
		id       string
		revision int64
		decision string
		readyAt  any
	}{{approval, 1, "APPROVED", nil}, {ready, 1, "APPROVED", at}} {
		if _, err := fx.f.Admin.Exec(ctx, `INSERT INTO org_connector_approvals
(org_id,id,revision,kind,provider_code,resource_id,credential_ref,maximum_bytes,maximum_items,decision,decision_ref,ready_at)
VALUES($1,$2,$3,'TELEMATICS','synthetic-telematics',$4,$5,4096,3,$6,'synthetic-approved-contract',$7)`,
			org, fixture.id, fixture.revision, asset, credential, fixture.decision, fixture.readyAt); err != nil {
			t.Fatal(err)
		}
	}
	create := app.Change{Action: app.Configure, OperationID: onboardingID(t), ApprovalID: ready, ApprovalRevision: 1, Label: "Synthetic connection"}
	write := func(input app.Change) (app.Result, error) {
		return store.Change(ctx, fx.scope, fx.actor, fx.member, input)
	}
	if _, err := write(create); !errors.Is(err, capability.ErrEntitlementRequired) {
		t.Fatalf("connection was configured before the capability was approved: %v", err)
	}
	if _, err := flags.Set(ctx, fx.scope, capability.Actor{AccountID: fx.actor.AccountID}, capability.Change{
		OperationID: onboardingID(t), Capability: "integrations", Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	notReady := create
	notReady.OperationID, notReady.ApprovalID = onboardingID(t), approval
	configured, err := write(notReady)
	if err != nil || configured.Connection.State != domain.Configured {
		t.Fatalf("unready connector was not safe to configure: %+v %v", configured, err)
	}
	activation := app.Change{Action: app.Activate, OperationID: onboardingID(t), TargetID: configured.Connection.ID, ExpectedRevision: 1}
	if _, err := write(activation); !errors.Is(err, domain.ErrNotReady) {
		t.Fatalf("unready provider activated: %v", err)
	}
	type outcome struct {
		result app.Result
		err    error
	}
	out := make(chan outcome, 2)
	for range 2 {
		go func() { result, err := write(create); out <- outcome{result, err} }()
	}
	first, second := <-out, <-out
	if first.err != nil || second.err != nil || first.result.Replayed == second.result.Replayed ||
		first.result.Connection.ID != second.result.Connection.ID || first.result.Connection.State != domain.Configured {
		t.Fatalf("matching commands did not converge: %+v %+v", first, second)
	}
	id := first.result.Connection.ID
	body, err := json.Marshal(first.result)
	if err != nil || strings.Contains(string(body), credential) || strings.Contains(string(body), ready) {
		t.Fatal("connector metadata exposed a credential or approval reference")
	}
	stub := func(_ context.Context, _ pgx.Tx, review domain.Approval) error {
		if review.Credential.Reference() != credential || review.MaxBytes != 4096 || review.MaxItems != 3 ||
			strings.Contains(fmt.Sprintf("%+v %#v", review, review), credential) {
			t.Fatal("connector operation did not receive the exact safe binding")
		}
		return nil
	}
	if err := access.WithActive(ctx, fx.scope, domain.Telematics, id, asset, stub); !errors.Is(err, domain.ErrDenied) {
		t.Fatal("configured connection was usable before activation", err)
	}
	activate := app.Change{Action: app.Activate, OperationID: onboardingID(t), TargetID: id, ExpectedRevision: 1}
	active, err := write(activate)
	if err != nil || active.Connection.State != domain.Active || !active.Connection.ApprovalCurrent {
		t.Fatalf("ready connector did not activate: %+v %v", active, err)
	}
	if err := access.WithActive(ctx, fx.scope, domain.Telematics, id, asset, stub); err != nil {
		t.Fatal("approved active scope was refused", err)
	}
	for _, scope := range []struct {
		kind     domain.Kind
		resource string
	}{{domain.FuelCardStatement, asset}, {domain.Telematics, onboardingID(t)}} {
		if err := access.WithActive(ctx, fx.scope, scope.kind, id, scope.resource, stub); !errors.Is(err, domain.ErrDenied) {
			t.Fatalf("connector authorized unapproved use: %v", err)
		}
	}
	foreign := fx.f.Organizations[1].Scope(t)
	if err := access.WithActive(ctx, foreign, domain.Telematics, id, asset, stub); !errors.Is(err, domain.ErrDenied) {
		t.Fatal("foreign organization used an approval", err)
	}
	if _, err := fx.f.Admin.Exec(ctx, `INSERT INTO org_connector_approvals
(org_id,id,revision,kind,provider_code,resource_id,credential_ref,maximum_bytes,maximum_items,decision,decision_ref)
VALUES($1,$2,2,'TELEMATICS','synthetic-telematics',$3,$4,4096,3,'WITHDRAWN','synthetic-withdrawal')`, org, ready, asset, credential); err != nil {
		t.Fatal(err)
	}
	if err := access.WithActive(ctx, fx.scope, domain.Telematics, id, asset, stub); !errors.Is(err, domain.ErrDenied) {
		t.Fatal("withdrawn approval remained usable", err)
	}
	page, err := store.List(ctx, fx.scope, fx.actor, fx.member, "", 20)
	if err != nil || len(page.Connections) != 2 {
		t.Fatalf("could not see retained connection metadata: %+v %v", page, err)
	}
	for _, row := range page.Connections {
		if row.ID == id && row.ApprovalCurrent {
			t.Fatal("withdrawn registration was presented as currently approved")
		}
	}
	if _, err := flags.Set(ctx, fx.scope, capability.Actor{AccountID: fx.actor.AccountID}, capability.Change{
		OperationID: onboardingID(t), Capability: "integrations", ExpectedRevision: 1, Enabled: false,
	}); err != nil {
		t.Fatal(err)
	}
	disabled, err := write(app.Change{Action: app.Disable, OperationID: onboardingID(t), TargetID: id, ExpectedRevision: 2})
	if err != nil || disabled.Connection.State != domain.Disabled {
		t.Fatalf("manager could not disable a connection after the plan hold: %+v %v", disabled, err)
	}
	if err := access.WithActive(ctx, fx.scope, domain.Telematics, id, asset, stub); !errors.Is(err, domain.ErrDenied) {
		t.Fatal("disabled connector was admitted", err)
	}
	var changes int
	if err := fx.f.Admin.QueryRow(ctx, "SELECT count(*) FROM org_audit_events WHERE org_id=$1 AND action LIKE 'integ.connection.%'", org).Scan(&changes); err != nil || changes != 4 {
		t.Fatalf("connector changes were not audited once: changes=%d err=%v", changes, err)
	}
}
