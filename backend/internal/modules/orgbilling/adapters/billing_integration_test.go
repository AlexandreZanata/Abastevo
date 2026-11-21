//go:build integration

package adapters_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgbilling/adapters"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgbilling/domain"
	capability "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/domain"
	payment "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
	"github.com/jackc/pgx/v5"
)

func TestVerifiedCompanyBillingReceiptPreventsConcurrentDuplicateSettlement(t *testing.T) {
	for _, settleFirst := range []bool{false, true} {
		name := "simultaneous first deliveries"
		if settleFirst {
			name = "simultaneous replays"
		}
		t.Run(name, func(t *testing.T) { checkVerifiedCompanySettlement(t, settleFirst) })
	}
}

func checkVerifiedCompanySettlement(t *testing.T, settleFirst bool) {
	t.Helper()
	f := orgtest.New(t)
	ctx := t.Context()
	a, b := f.Organizations[0], f.Organizations[1]
	scopeA, scopeB := a.Scope(t), b.Scope(t)
	role := pgx.Identifier{f.RuntimeRole}.Sanitize()
	if _, err := f.Admin.Exec(ctx, "GRANT SELECT ON enterprise_plan_catalog, enterprise_billing_offers, org_billing_charge_bindings TO "+role+
		"; GRANT SELECT, INSERT ON org_billing_subscription_events, org_billing_settlement_receipts TO "+role); err != nil {
		t.Fatal(err)
	}
	known, err := capability.NewCatalog([]string{"organizations"})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := f.Admin.Exec(ctx, `INSERT INTO enterprise_plan_catalog
(product_code,plan_code,revision,display_name,capabilities,active_vehicles)
VALUES('fleet','synthetic',1,'Synthetic company',ARRAY['organizations'],3)`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Admin.Exec(ctx, `INSERT INTO enterprise_billing_offers
(id,product_code,plan_code,plan_revision,channel,environment,merchant,provider_offer_ref,amount_centavos,term,approval_ref)
VALUES($1,'fleet','synthetic',1,'stripe_card','test','synthetic-merchant','synthetic-offer',1723,'P1M','synthetic-review')`, a.ParentID); err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 11, 21, 10, 0, 0, 0, time.UTC)
	prior := d.Period{Start: at.AddDate(0, -1, 0), End: at}
	term := d.Period{Start: at, End: at.AddDate(0, 1, 0)}
	if _, err := f.Admin.Exec(ctx, `INSERT INTO org_billing_charge_bindings
(org_id,provider,environment,merchant,resource_ref,subscription_id,offer_id,customer_ref,contract_ref,term_start,term_end)
VALUES($1,'stripe','test','synthetic-merchant','synthetic-invoice',$2,$3,'synthetic-customer',
 'synthetic-contract',$4,$5)`, a.ID, a.RecordID, a.ParentID, term.Start, term.End); err != nil {
		t.Fatal(err)
	}

	ledger := adapters.SubscriptionLedger{Capabilities: known}
	key := d.PlanKey{Product: "fleet", Plan: "synthetic", Revision: 1}
	ready := d.LifecycleEvent{Scope: scopeA, SubscriptionID: a.RecordID, Kind: d.ConfirmSettlement, Source: d.VerifiedBilling,
		At: prior.Start, Reference: "synthetic-prior-settlement", Period: prior}
	pastDue := ready
	pastDue.Kind, pastDue.ExpectedRevision, pastDue.At = d.ReportPastDue, 1, at
	pastDue.Reference, pastDue.Period = "synthetic-overdue", d.Period{}
	for _, event := range []d.LifecycleEvent{ready, pastDue} {
		if err := database.WithOrgTx(ctx, f.Runtime, scopeA, func(tx pgx.Tx) error {
			_, err := ledger.ApplyTx(ctx, tx, key, event)
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}

	intake := adapters.BillingIntake{Pool: f.Runtime, Ledger: ledger, Now: func() time.Time { return at.Add(time.Hour) }}
	notice := d.BillingNotice{Provider: payment.ProviderStripe, Environment: payment.EnvironmentTest, Merchant: "synthetic-merchant",
		EventID: "synthetic-event", PayloadHash: strings.Repeat("a", 64), ResourceRef: "synthetic-invoice"}
	fact := d.CanonicalBillingFact{Notice: notice, Scope: scopeA, SubscriptionID: a.RecordID,
		CustomerRef: "synthetic-customer", ContractRef: "synthetic-contract", ProviderOfferRef: "synthetic-offer",
		Channel: payment.ChannelCard, ResourceRef: notice.ResourceRef, ChargeRef: "synthetic-payment",
		Outcome: d.BillingSettled, At: at, Gross: payment.BRL(1723), PaidTerm: term}

	foreign := fact
	foreign.Scope = scopeB
	if _, err := intake.RecordSettlement(ctx, notice, foreign); !errors.Is(err, d.ErrBillingBinding) {
		t.Fatalf("foreign scope reconciled the company's invoice: %v", err)
	}
	underpayment := fact
	underpayment.Gross = payment.BRL(1722)
	if _, err := intake.RecordSettlement(ctx, notice, underpayment); !errors.Is(err, payment.ErrAmountMismatch) {
		t.Fatalf("underpayment reconciled as an exact invoice: %v", err)
	}

	wantReplays := 1
	if settleFirst {
		first, err := intake.RecordSettlement(ctx, notice, fact)
		if err != nil || first.Replayed || first.Revision != 3 {
			t.Fatalf("first settlement did not activate the overdue term: %+v %v", first, err)
		}
		wantReplays = 2
	}

	type result struct {
		receipt adapters.SettlementReceipt
		err     error
	}
	replies := make(chan result, 2)
	for range 2 {
		go func() {
			receipt, err := intake.RecordSettlement(ctx, notice, fact)
			replies <- result{receipt, err}
		}()
	}
	var replays int
	for range 2 {
		out := <-replies
		if out.err != nil || out.receipt.Revision != 3 || !out.receipt.SettledAt.Equal(at) {
			t.Fatalf("concurrent intake did not converge: %+v", out)
		}
		if out.receipt.Replayed {
			replays++
		}
	}
	if replays != wantReplays {
		t.Fatalf("got %d replays, want %d", replays, wantReplays)
	}

	conflict := notice
	conflict.PayloadHash = strings.Repeat("b", 64)
	changedProof := fact
	changedProof.Notice = conflict
	if _, err := intake.RecordSettlement(ctx, conflict, changedProof); !errors.Is(err, payment.ErrConflict) {
		t.Fatalf("changed event payload reused a prior acknowledgement: %v", err)
	}
	redelivery := notice
	redelivery.EventID = "second-notice-same-charge"
	sameCharge := fact
	sameCharge.Notice = redelivery
	if receipt, err := intake.RecordSettlement(ctx, redelivery, sameCharge); err != nil || !receipt.Replayed || receipt.Revision != 3 {
		t.Fatalf("canonical charge was applied twice: %+v %v", receipt, err)
	}
	foreignAmount := sameCharge
	foreignAmount.Gross = payment.BRL(1722)
	if _, err := intake.RecordSettlement(ctx, redelivery, foreignAmount); !errors.Is(err, payment.ErrAmountMismatch) {
		t.Fatalf("mismatched facts replayed a previously verified charge: %v", err)
	}
	if err := database.WithOrgTx(ctx, f.Runtime, scopeA, func(tx pgx.Tx) error {
		var receipts, recoveries int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM org_billing_settlement_receipts`).Scan(&receipts); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM org_billing_subscription_events
WHERE from_state='PAST_DUE' AND state='ACTIVE' AND event_source='VERIFIED_BILLING'`).Scan(&recoveries); err != nil {
			return err
		}
		if receipts != 1 || recoveries != 1 {
			t.Fatalf("expected one audited settlement and one recovery, got %d receipts and %d recoveries", receipts, recoveries)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
