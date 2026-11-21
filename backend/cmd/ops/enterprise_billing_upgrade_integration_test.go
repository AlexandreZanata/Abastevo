//go:build integration

package main

import (
	"io/fs"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jackc/pgx/v5"

	migrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	billingpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgbilling/adapters"
	billing "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgbilling/domain"
	capability "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/domain"
	payment "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
)

func TestReviewedChargeBindingUpgradePreservesPriorSettlement(t *testing.T) {
	const migrationName = "000289_organization_billing_charge_approvals.sql"
	prior := fstest.MapFS{}
	names, err := fs.Glob(migrations.Files, "*.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range append(names, "schema_min_supported") {
		if name != "schema_min_supported" && name >= migrationName {
			continue
		}
		raw, err := fs.ReadFile(migrations.Files, name)
		if err != nil {
			t.Fatal(err)
		}
		prior[name] = &fstest.MapFile{Data: raw}
	}
	f := orgtest.NewWithMigrations(t, prior)
	ctx, org := t.Context(), f.Organizations[0]
	term := billing.Period{Start: time.Date(2026, 11, 21, 10, 0, 0, 0, time.UTC), End: time.Date(2026, 12, 21, 10, 0, 0, 0, time.UTC)}
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := f.Admin.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO enterprise_plan_catalog(product_code,plan_code,revision,display_name,capabilities,active_vehicles)
           VALUES('fleet','synthetic',1,'Synthetic plan',ARRAY['organizations'],3)`)
	exec(`INSERT INTO enterprise_billing_offers
           (id,product_code,plan_code,plan_revision,channel,environment,merchant,provider_offer_ref,amount_centavos,term,approval_ref)
           VALUES($1,'fleet','synthetic',1,'stripe_card','test','synthetic-merchant','synthetic-offer',1723,'P1M','synthetic-review')`, org.ParentID)
	exec(`INSERT INTO org_billing_charge_bindings
           (org_id,provider,environment,merchant,resource_ref,subscription_id,offer_id,customer_ref,contract_ref,term_start,term_end)
           VALUES($1,'stripe','test','synthetic-merchant','synthetic-invoice',$2,$3,'synthetic-customer','synthetic-contract',$4,$5)`,
		org.ID, org.RecordID, org.ParentID, term.Start, term.End)
	role := pgx.Identifier{f.RuntimeRole}.Sanitize()
	exec("GRANT SELECT ON enterprise_plan_catalog, enterprise_billing_offers, org_billing_charge_bindings TO " + role +
		"; GRANT SELECT, INSERT ON org_billing_subscription_events, org_billing_settlement_receipts TO " + role)
	known, err := capability.NewCatalog([]string{"organizations"})
	if err != nil {
		t.Fatal(err)
	}
	intake := billingpg.BillingIntake{Pool: f.Runtime, Ledger: billingpg.SubscriptionLedger{Capabilities: known},
		Now: func() time.Time { return term.Start.Add(time.Hour) }}
	notice := billing.BillingNotice{Provider: payment.ProviderStripe, Environment: payment.EnvironmentTest,
		Merchant: "synthetic-merchant", ResourceRef: "synthetic-invoice", EventID: "synthetic-event", PayloadHash: strings.Repeat("a", 64)}
	fact := billing.CanonicalBillingFact{Notice: notice, Scope: org.Scope(t), SubscriptionID: org.RecordID, CustomerRef: "synthetic-customer",
		ContractRef: "synthetic-contract", ProviderOfferRef: "synthetic-offer", Channel: payment.ChannelCard, ResourceRef: notice.ResourceRef,
		ChargeRef: "synthetic-charge", Outcome: billing.BillingSettled, At: term.Start, Gross: payment.BRL(1723), PaidTerm: term}
	initial, err := intake.RecordSettlement(ctx, notice, fact)
	if err != nil || initial.Revision != 1 || initial.Replayed {
		t.Fatalf("pre-upgrade settlement: %+v / %v", initial, err)
	}
	proof := func() string {
		t.Helper()
		var row string
		if err := f.Admin.QueryRow(ctx, `SELECT jsonb_build_array(to_jsonb(b)-'approval_id'-'binding_operation_id',to_jsonb(r))::text
            FROM org_billing_charge_bindings b JOIN org_billing_settlement_receipts r
            USING (org_id,provider,environment,merchant,resource_ref,subscription_id)`).Scan(&row); err != nil {
			t.Fatal(err)
		}
		return row
	}
	before := proof()
	raw, err := fs.ReadFile(migrations.Files, migrationName)
	if err != nil {
		t.Fatal(err)
	}
	prior[migrationName] = &fstest.MapFile{Data: append(slices.Clone(raw), []byte("\nSELECT 1/0;\n")...)}
	if _, err := migrate.Apply(ctx, f.AdminDSN, prior); err == nil || !strings.Contains(err.Error(), "SQLSTATE 22012") {
		t.Fatalf("expected injected migration failure: %v", err)
	}
	var approvalsAbsent bool
	if err := f.Admin.QueryRow(ctx, `SELECT to_regclass('public.org_billing_charge_approvals') IS NULL`).Scan(&approvalsAbsent); err != nil ||
		!approvalsAbsent || proof() != before {
		t.Fatalf("failed migration changed billing proof or left its approvals table: %v / %v", approvalsAbsent, err)
	}
	prior[migrationName] = &fstest.MapFile{Data: raw}
	if applied, err := migrate.Apply(ctx, f.AdminDSN, prior); err != nil || !slices.Equal(applied, []string{"000289"}) || proof() != before {
		t.Fatalf("migration recovery changed billing proof: %v / %v", applied, err)
	}
	if receipt, err := intake.RecordSettlement(ctx, notice, fact); err != nil || !receipt.Replayed ||
		receipt.Revision != initial.Revision || !receipt.SettledAt.Equal(initial.SettledAt) {
		t.Fatalf("existing settlement lost its original receipt: %+v / %v", receipt, err)
	}
}
