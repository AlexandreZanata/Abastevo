//go:build integration

package adapters_test

import (
	"errors"
	"testing"
	"time"

	billing "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgbilling/adapters"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgbilling/domain"
	capstore "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/adapters"
	capability "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
	"github.com/jackc/pgx/v5"
)

func TestSubscriptionWriteGateUsesCurrentStateWithoutPilotFlag(t *testing.T) {
	f := orgtest.New(t)
	pool := f.RuntimePool(t, 2)
	ctx := t.Context()
	a, b := f.Organizations[0], f.Organizations[1]
	scopeA, scopeB := a.Scope(t), b.Scope(t)
	role := pgx.Identifier{f.RuntimeRole}.Sanitize()
	if _, err := f.Admin.Exec(ctx, "GRANT SELECT ON enterprise_plan_catalog TO "+role+
		"; GRANT SELECT, INSERT ON org_billing_subscription_events TO "+role+
		"; GRANT SELECT, UPDATE ON org_capabilities TO "+role); err != nil {
		t.Fatal(err)
	}
	known, err := capability.NewCatalog([]string{"organizations"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Admin.Exec(ctx, `INSERT INTO enterprise_plan_catalog
(product_code,plan_code,revision,display_name,capabilities)
VALUES('fleet','synthetic',1,'Synthetic company',ARRAY['organizations'])`); err != nil {
		t.Fatal(err)
	}
	at := time.Now().UTC().Truncate(time.Second)
	key := d.PlanKey{Product: "fleet", Plan: "synthetic", Revision: 1}
	ledger := billing.SubscriptionLedger{Capabilities: known}
	active := d.LifecycleEvent{
		Scope: scopeA, SubscriptionID: a.RecordID, Kind: d.ConfirmSettlement, Source: d.VerifiedBilling,
		At: at, Reference: "synthetic-settlement", Period: d.Period{Start: at, End: at.Add(time.Hour)},
	}
	proof := f.PrepareSubscriptionOpening(t, key, active)
	if err := database.WithOrgTx(ctx, pool, scopeA, func(tx pgx.Tx) error {
		if _, err := ledger.ApplyTx(ctx, tx, key, active); err != nil {
			return err
		}
		return proof(tx)
	}); err != nil {
		t.Fatal(err)
	}
	plans := billing.EntitlementStore{Pool: pool, Capabilities: known, Now: func() time.Time { return at.Add(time.Minute) }}
	flags := capstore.NewStore(pool, known, nil)
	flags.PlanWrites = plans
	if err := database.WithOrgTx(ctx, pool, scopeA, func(tx pgx.Tx) error {
		return flags.GuardWrite(ctx, tx, scopeA, "organizations")
	}); err != nil {
		t.Fatalf("an eligible company requires an unrelated pilot flag: %v", err)
	}
	if err := database.WithOrgTx(ctx, pool, scopeB, func(tx pgx.Tx) error {
		return flags.GuardWrite(ctx, tx, scopeB, "organizations")
	}); !errors.Is(err, capability.ErrEntitlementRequired) {
		t.Fatalf("new write used a foreign company's plan: %v", err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, "SELECT set_config('anpfuel.org_id',$1,true)", a.ID); err != nil {
		t.Fatal(err)
	}
	suspend := d.LifecycleEvent{
		Scope: scopeA, SubscriptionID: a.RecordID, Kind: d.SuspendAccess, Source: d.ReviewedOperator,
		ExpectedRevision: 1, At: at.Add(time.Second), Reference: "synthetic-suspension",
	}
	if _, err := ledger.ApplyTx(ctx, tx, key, suspend); err != nil {
		t.Fatal(err)
	}
	if err := database.WithOrgTx(ctx, pool, scopeA, func(probe pgx.Tx) error {
		var available bool
		if err := probe.QueryRow(ctx, `SELECT pg_try_advisory_xact_lock_shared(
hashtextextended('organization-entitlement:' || $1::text,0))`, a.ID).Scan(&available); err != nil {
			return err
		}
		if available {
			t.Fatal("write could evaluate stale subscription before suspension committed")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	if _, err := f.Admin.Exec(ctx, `INSERT INTO org_capabilities(org_id,capability,enabled)
VALUES($1,'organizations',true)`, a.ID); err != nil {
		t.Fatal(err)
	}
	if err := database.WithOrgTx(ctx, pool, scopeA, func(tx pgx.Tx) error {
		return flags.GuardWrite(ctx, tx, scopeA, "organizations")
	}); !errors.Is(err, capability.ErrEntitlementRequired) {
		t.Fatalf("write bypassed suspension using stale or pilot authority: %v", err)
	}
	if err := database.WithOrgTx(ctx, pool, scopeA, func(tx pgx.Tx) error {
		return flags.GuardRetainedExport(ctx, tx, scopeA, "organizations")
	}); err != nil {
		t.Fatalf("subscription suspension blocked a retained export: %v", err)
	}
	if err := database.WithOrgTx(ctx, pool, scopeB, func(tx pgx.Tx) error {
		return flags.GuardRetainedExport(ctx, tx, scopeB, "organizations")
	}); !errors.Is(err, capability.ErrEntitlementRequired) {
		t.Fatalf("pilot-only organization bypassed its absent flag on export: %v", err)
	}
	if _, err := f.Admin.Exec(ctx, `UPDATE org_capabilities SET enabled=false
WHERE org_id=$1 AND capability='organizations'`, a.ID); err != nil {
		t.Fatal(err)
	}
	if err := database.WithOrgTx(ctx, pool, scopeA, func(tx pgx.Tx) error {
		return flags.GuardRetainedExport(ctx, tx, scopeA, "organizations")
	}); !errors.Is(err, capability.ErrEntitlementRequired) {
		t.Fatalf("retained export bypassed an explicit operator denial: %v", err)
	}
}
