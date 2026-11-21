//go:build integration

package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	billingpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgbilling/adapters"
	billing "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgbilling/domain"
	capability "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/domain"
	payment "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
)

func TestReviewedChargeBindingCommandAndSettlement(t *testing.T) {
	f := orgtest.New(t)
	ctx := t.Context()
	a, b := f.Organizations[0], f.Organizations[1]
	start := time.Now().UTC().Truncate(time.Second).Add(-time.Hour)
	end := start.AddDate(0, 1, 0)
	stripeOffer, asaasOffer := testOp, testOrg
	ready, wrongOrg, expired, pix, malformed, closed := a.ParentID, b.ParentID, a.RecordID, "7b0e2a54-0000-4000-8000-0000000e0009",
		"7b0e2a54-0000-4000-8000-0000000e0010", "7b0e2a54-0000-4000-8000-0000000e0011"
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := f.Admin.Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`UPDATE organizations SET has_snapshot=true, display_name='Synthetic company', time_zone='UTC',
       kind_tags='{}', state='PENDING_SETUP', suspension_reason='', revision=revision+1`)
	exec(`INSERT INTO enterprise_plan_catalog(product_code,plan_code,revision,display_name,capabilities,active_vehicles)
       VALUES('fleet','synthetic',1,'Synthetic plan',ARRAY['organizations'],3)`)
	exec(`INSERT INTO enterprise_billing_offers
       (id,product_code,plan_code,plan_revision,channel,environment,merchant,provider_offer_ref,amount_centavos,term,approval_ref)
       VALUES($1,'fleet','synthetic',1,'stripe_card','test','synthetic-merchant','synthetic-offer',1723,'P1M','synthetic-stripe-review'),
             ($2,'fleet','synthetic',1,'asaas_pix_manual','test','synthetic-merchant','synthetic-pix-offer',1723,'P1M','synthetic-pix-review')`,
		stripeOffer, asaasOffer)
	approve := func(org orgtest.Organization, id, offer, resource, contract, asaasRef string, expires time.Time) {
		t.Helper()
		exec(`INSERT INTO org_billing_charge_approvals
           (org_id,id,operator_id,subscription_id,offer_id,resource_ref,customer_ref,contract_ref,asaas_external_reference,
            term_start,term_end,approval_ref,valid_from,valid_until)
           VALUES($1,$2,$3,$4,$5,$6,'synthetic-customer',$7,$8,$9,$10,$11,$12,$13)`,
			org.ID, id, testOperator, org.RecordID, offer, resource, contract, asaasRef, start, end, "synthetic-review-"+id, start, expires)
	}
	approve(a, ready, stripeOffer, "synthetic-invoice", "synthetic-contract", "", end)
	approve(b, wrongOrg, stripeOffer, "synthetic-invoice", "synthetic-contract", "", end)
	approve(a, expired, stripeOffer, "synthetic-expired", "synthetic-contract", "", start.Add(time.Minute))
	pixRef := "abv_org:" + a.ID + ":" + pix
	approve(a, pix, asaasOffer, "synthetic-pix", "synthetic-pix", pixRef, end)
	approve(a, malformed, asaasOffer, "synthetic-invalid", "synthetic-invalid", "abv_org:"+b.ID+":"+pix, end)
	approve(b, closed, stripeOffer, "synthetic-closed", "synthetic-contract", "", end)

	script, err := os.ReadFile("../../../infra/db/billing-binding-role.sql")
	if err != nil {
		t.Fatal(err)
	}
	provision := func(role string) error {
		_, err := f.Admin.Exec(ctx, strings.NewReplacer(`:'billing_binding_role'`, "'"+role+"'",
			`:"billing_binding_role"`, pgx.Identifier{role}.Sanitize()).Replace(string(script)))
		return err
	}
	role := "emp_bind_" + strings.ToLower(rand.Text())
	secret := rand.Text()
	name := pgx.Identifier{role}.Sanitize()
	exec("CREATE ROLE " + name + " LOGIN NOINHERIT PASSWORD '" + secret + "'")
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := f.Admin.Exec(cleanup, "DROP OWNED BY "+name+"; DROP ROLE "+name); err != nil {
			t.Error(err)
		}
	})
	for range 2 {
		if err := provision(role); err != nil {
			t.Fatalf("dedicated login provisioning: %v", err)
		}
	}
	servingRole, servingSecret := f.ProvisionScriptRole(t)
	serving := f.ScriptRuntimePool(t, servingRole, servingSecret, 2)
	// A failed multi-statement provisioning attempt must roll back the open
	// transaction. Query the restriction from an explicitly borrowed session.
	conn, err := f.Admin.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, provisionErr := conn.Exec(ctx, strings.NewReplacer(`:'billing_binding_role'`, "'"+servingRole+"'",
		`:"billing_binding_role"`, pgx.Identifier{servingRole}.Sanitize()).Replace(string(script)))
	_, rollbackErr := conn.Exec(ctx, "ROLLBACK")
	conn.Release()
	if provisionErr == nil || rollbackErr != nil {
		t.Fatalf("serving login must be refused by operator provisioning: %v / %v", provisionErr, rollbackErr)
	}
	dsn, err := url.Parse(f.AdminDSN)
	if err != nil {
		t.Fatal(err)
	}
	dsn.User = url.UserPassword(role, secret)
	t.Setenv("ANPFUEL_DATABASE_URL", dsn.String())
	t.Setenv("ANPFUEL_ENTERPRISE_ENABLED", "true")
	t.Setenv("ANPFUEL_ENTERPRISE_BILLING_ENABLED", "true")
	args := func(org, approvalID, operation, operator string) []string {
		return []string{"billing", "bind", "--org", org, "--approval-id", approvalID,
			"--operation-id", operation, "--operator", operator}
	}
	var out bytes.Buffer
	run := func(org, approvalID, operation, operator string) error {
		out.Reset()
		return runEnterprise(args(org, approvalID, operation, operator), os.Getenv, &out)
	}
	assertRun := func(org, approval, operation, operator string, want error) {
		t.Helper()
		if err := run(org, approval, operation, operator); !errors.Is(err, want) || out.Len() != 0 {
			t.Fatalf("expected refusal %v without acknowledgement, got %v / %q", want, err, out.String())
		}
	}
	t.Setenv("ANPFUEL_ENTERPRISE_BILLING_ENABLED", "false")
	if err := run(a.ID, ready, testOp, testOperator); err == nil || err.Error() != "enterprise billing is disabled" || out.Len() != 0 {
		t.Fatalf("disabled billing accepted binding: %v / %q", err, out.String())
	}
	t.Setenv("ANPFUEL_ENTERPRISE_BILLING_ENABLED", "true")
	assertRun(a.ID, wrongOrg, testOp, testOperator, errChargeReviewUnavailable)
	assertRun(a.ID, ready, testOp, testOrg, errChargeReviewUnavailable)
	assertRun(a.ID, expired, testOp, testOperator, errChargeReviewUnavailable)
	assertRun(a.ID, malformed, testOp, testOperator, errChargeReviewUnavailable)
	if err := run(a.ID, ready, testOp, testOperator); err != nil ||
		out.String() != "organization "+a.ID+" subscription "+a.RecordID+" operation="+testOp+" replayed=false\n" {
		t.Fatalf("first command: %v / %q", err, out.String())
	}
	if err := run(a.ID, ready, testOp, testOperator); err != nil || !strings.HasSuffix(out.String(), "replayed=true\n") {
		t.Fatalf("matching replay: %v / %q", err, out.String())
	}
	assertRun(a.ID, pix, testOp, testOperator, audit.ErrReused)
	assertRun(a.ID, ready, testOrg, testOperator, errChargeBindingConflict)
	assertRun(b.ID, wrongOrg, testOp, testOperator, errChargeBindingConflict)
	exec(`UPDATE organizations SET state='CLOSED', revision=revision+1 WHERE org_id=$1`, b.ID)
	assertRun(b.ID, closed, testOp, testOperator, errChargeReviewUnavailable)

	pool := f.ScriptRuntimePool(t, role, secret, 2)
	in, err := parseReviewedBillingArgs(args(a.ID, pix, testOrg, testOperator)[1:], os.Getenv,
		"enterprise billing bind", "bind", errBillingBindUsage)
	if err != nil {
		t.Fatal(err)
	}
	type outcome struct {
		receipt chargeBindingReceipt
		err     error
	}
	results := make(chan outcome, 2)
	for range 2 {
		go func() {
			receipt, err := executeBillingBind(ctx, pool, in)
			results <- outcome{receipt, err}
		}()
	}
	first, second := <-results, <-results
	if first.err != nil || second.err != nil || first.receipt.replayed == second.receipt.replayed ||
		first.receipt.subscription != a.RecordID || second.receipt.subscription != a.RecordID {
		t.Fatalf("concurrent Pix binding must commit once and replay once: %+v / %+v", first, second)
	}
	if _, err := executeBillingBind(ctx, f.Admin, in); !errors.Is(err, database.ErrUnsafeOrgRole) {
		t.Fatalf("migration owner accepted as operator: %v", err)
	}
	if _, err := executeBillingBind(ctx, serving, in); !hasBillingSQLState(err, "42501") {
		t.Fatalf("serving login can reach private reviews: %v", err)
	}
	if err := database.WithOrgTx(ctx, pool, a.Scope(t), func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO org_billing_charge_approvals(org_id,id) VALUES($1,$2)`, a.ID, closed)
		return err
	}); !hasBillingSQLState(err, "42501") {
		t.Fatalf("binding login can create its own approval: %v", err)
	}
	if err := database.WithOrgTx(ctx, pool, a.Scope(t), func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO org_billing_charge_bindings
			(org_id,provider,environment,merchant,resource_ref,subscription_id,offer_id,customer_ref,contract_ref,term_start,term_end)
			VALUES($1,'stripe','test','synthetic-merchant','synthetic-unreviewed',$2,$3,'synthetic-customer','synthetic-contract',$4,$5)`,
			a.ID, a.RecordID, stripeOffer, start, end)
		return err
	}); !hasBillingSQLState(err, "42501") {
		t.Fatalf("binding login can insert an unreviewed binding: %v", err)
	}
	if err := database.WithOrgTx(ctx, pool, a.Scope(t), func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO org_audit_events(org_id,operation_id,actor_id,action,target_type,target_id,request_hash)
            VALUES($1,$2,$3,'capability.enabled','subscription',$4,$5)`, a.ID, closed, testOperator, a.RecordID, strings.Repeat("a", 64))
		return err
	}); !hasBillingSQLState(err, "42501") {
		t.Fatalf("binding login can write unrelated audit: %v", err)
	}
	if err := database.WithOrgTx(ctx, pool, a.Scope(t), func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO org_audit_events(org_id,operation_id,actor_id,action,target_type,target_id,request_hash)
            VALUES($1,$2,$3,'subs.billing.charge.bound','subscription',$4,$5)`, a.ID, closed, testOperator, a.RecordID, strings.Repeat("a", 64))
		return err
	}); !hasBillingSQLState(err, "23514") {
		t.Fatalf("binding audit can commit without its effect: %v", err)
	}

	var bindings, auditRows, paymentRows, lifecycleRows int
	if err := f.Admin.QueryRow(ctx, `SELECT (SELECT count(*) FROM org_billing_charge_bindings),
        (SELECT count(*) FROM org_audit_events WHERE action='subs.billing.charge.bound'),
        (SELECT count(*) FROM org_billing_settlement_receipts), (SELECT count(*) FROM org_billing_subscription_events)`).
		Scan(&bindings, &auditRows, &paymentRows, &lifecycleRows); err != nil || bindings != 2 || auditRows != 2 || paymentRows != 0 || lifecycleRows != 0 {
		t.Fatalf("binding/audit atomicity and no settlement: %d / %d / %d / %d / %v", bindings, auditRows, paymentRows, lifecycleRows, err)
	}
	known, err := capability.NewCatalog([]string{"organizations"})
	if err != nil {
		t.Fatal(err)
	}
	intake := billingpg.BillingIntake{Pool: serving, Ledger: billingpg.SubscriptionLedger{Capabilities: known},
		Now: func() time.Time { return start.Add(time.Hour) }}
	notice := billing.BillingNotice{Provider: payment.ProviderStripe, Environment: payment.EnvironmentTest,
		Merchant: "synthetic-merchant", ResourceRef: "synthetic-invoice", EventID: "synthetic-event", PayloadHash: strings.Repeat("a", 64)}
	fact := billing.CanonicalBillingFact{Notice: notice, Scope: a.Scope(t), SubscriptionID: a.RecordID, CustomerRef: "synthetic-customer",
		ContractRef: "synthetic-contract", ProviderOfferRef: "synthetic-offer", Channel: payment.ChannelCard, ResourceRef: notice.ResourceRef,
		ChargeRef: "synthetic-settled-charge", Outcome: billing.BillingSettled, At: start, Gross: payment.BRL(1723),
		PaidTerm: billing.Period{Start: start, End: end}}
	if receipt, err := intake.RecordSettlement(ctx, notice, fact); err != nil || receipt.Replayed || receipt.Revision != 1 {
		t.Fatalf("serving login failed to recognize the operator binding: %+v / %v", receipt, err)
	}
}

func hasBillingSQLState(err error, code string) bool {
	var cause *pgconn.PgError
	return errors.As(err, &cause) && cause.Code == code
}
