//go:build integration

package adapters_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgbilling/adapters"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgbilling/domain"
	capability "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/domain"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/application"
	payment "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestVerifiedCompanyBillingReceiptPreventsConcurrentDuplicateSettlement(t *testing.T) {
	t.Run("simultaneous first deliveries", func(t *testing.T) { checkVerifiedCompanySettlement(t, false, false) })
	t.Run("simultaneous replays", func(t *testing.T) { checkVerifiedCompanySettlement(t, true, false) })
	t.Run("next-term downgrade", func(t *testing.T) { checkVerifiedCompanySettlement(t, false, true) })
}

func checkVerifiedCompanySettlement(t *testing.T, settleFirst, downgrade bool) {
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

	ledger := adapters.SubscriptionLedger{Capabilities: known}
	key := d.PlanKey{Product: "fleet", Plan: "synthetic", Revision: 1}
	priorPlan := key
	if downgrade {
		priorPlan.Plan = "synthetic-larger"
		if _, err := f.Admin.Exec(ctx, `INSERT INTO enterprise_plan_catalog
(product_code,plan_code,revision,display_name,capabilities,active_vehicles)
VALUES('fleet',$1,1,'Synthetic larger company',ARRAY['organizations'],10)`, priorPlan.Plan); err != nil {
			t.Fatal(err)
		}
	}
	ready := d.LifecycleEvent{Scope: scopeA, SubscriptionID: a.RecordID, Kind: d.ConfirmSettlement, Source: d.VerifiedBilling,
		At: prior.Start, Reference: "synthetic-prior-settlement", Period: prior}
	pastDue := ready
	pastDue.Kind, pastDue.ExpectedRevision, pastDue.At = d.ReportPastDue, 1, at
	pastDue.Reference, pastDue.Period = "synthetic-overdue", d.Period{}
	proof := f.PrepareSubscriptionOpening(t, priorPlan, ready)
	for _, event := range []d.LifecycleEvent{ready, pastDue} {
		if err := database.WithOrgTx(ctx, f.Runtime, scopeA, func(tx pgx.Tx) error {
			_, err := ledger.ApplyTx(ctx, tx, priorPlan, event)
			if err == nil && event.Kind == d.ConfirmSettlement {
				return proof(tx)
			}
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}

	if downgrade {
		unreceipted := ready
		unreceipted.ExpectedRevision, unreceipted.At, unreceipted.Period, unreceipted.PlanChange = 2, at, term, key
		err := database.WithOrgTx(ctx, f.Runtime, scopeA, func(tx pgx.Tx) error {
			_, err := ledger.ApplyTx(ctx, tx, key, unreceipted)
			return err
		})
		var constraint *pgconn.PgError
		if !errors.As(err, &constraint) || constraint.Code != "23514" ||
			constraint.Message != "subscription plan change receipt unavailable" {
			t.Fatalf("expected the plan-change receipt constraint to deny commit: %v", err)
		}
	}

	intake := adapters.BillingIntake{Pool: f.Runtime, Ledger: ledger, Now: func() time.Time { return at.Add(time.Hour) }}
	notice := d.BillingNotice{Provider: payment.ProviderStripe, Environment: payment.EnvironmentTest, Merchant: "synthetic-merchant",
		EventID: "synthetic-event", PayloadHash: strings.Repeat("a", 64), ResourceRef: "synthetic-invoice"}
	fact := d.CanonicalBillingFact{Notice: notice, Scope: scopeA, SubscriptionID: a.RecordID,
		CustomerRef: "synthetic-customer", ContractRef: "synthetic-contract", ProviderOfferRef: "synthetic-offer",
		Channel: payment.ChannelCard, ResourceRef: notice.ResourceRef, ChargeRef: "synthetic-payment",
		Outcome: d.BillingSettled, At: at, Gross: payment.BRL(1723), PaidTerm: term}

	if _, err := intake.RecordSettlement(ctx, notice, fact); !errors.Is(err, d.ErrChargeBindingMissing) {
		t.Fatalf("renewal without a binding did not return a retryable missing binding: %v", err)
	}
	if _, err := f.Admin.Exec(ctx, `INSERT INTO org_billing_charge_bindings
(org_id,provider,environment,merchant,resource_ref,subscription_id,offer_id,customer_ref,contract_ref,term_start,term_end)
VALUES($1,'stripe','test','synthetic-merchant','synthetic-invoice',$2,$3,'synthetic-customer',
 'synthetic-contract',$4,$5)`, a.ID, a.RecordID, a.ParentID, term.Start, term.End); err != nil {
		t.Fatal(err)
	}
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
	if _, err := f.Admin.Exec(ctx, `INSERT INTO org_billing_charge_bindings
(org_id,provider,environment,merchant,resource_ref,subscription_id,offer_id,customer_ref,contract_ref,term_start,term_end)
VALUES($1,'stripe','test','synthetic-merchant','different-invoice',$2,$3,'synthetic-customer',
 'synthetic-contract',$4,$5)`, a.ID, a.RecordID, a.ParentID, term.Start, term.End); err != nil {
		t.Fatal(err)
	}
	duplicateTerm := notice
	duplicateTerm.ResourceRef, duplicateTerm.EventID = "different-invoice", "different-provider-notice"
	otherCharge := fact
	otherCharge.Notice, otherCharge.ResourceRef, otherCharge.ChargeRef = duplicateTerm, duplicateTerm.ResourceRef, "different-canonical-charge"
	if _, err := intake.RecordSettlement(ctx, duplicateTerm, otherCharge); !errors.Is(err, payment.ErrConflict) {
		t.Fatalf("a different charge produced a second receipt for an already paid term: %v", err)
	}
	foreignAmount := sameCharge
	foreignAmount.Gross = payment.BRL(1722)
	if _, err := intake.RecordSettlement(ctx, redelivery, foreignAmount); !errors.Is(err, payment.ErrAmountMismatch) {
		t.Fatalf("mismatched facts replayed a previously verified charge: %v", err)
	}

	if _, err := f.Admin.Exec(ctx, `INSERT INTO org_billing_charge_bindings
(org_id,provider,environment,merchant,resource_ref,subscription_id,offer_id,customer_ref,contract_ref,term_start,term_end)
VALUES($1,'stripe','test','synthetic-merchant','second-company-invoice',$2,$3,'second-company-customer',
 'second-company-contract',$4,$5)`, b.ID, b.RecordID, a.ParentID, term.Start, term.End); err != nil {
		t.Fatal(err)
	}
	secondCompany := fact
	secondCompany.Scope, secondCompany.SubscriptionID = scopeB, b.RecordID
	secondCompany.CustomerRef, secondCompany.ContractRef = "second-company-customer", "second-company-contract"
	secondCompany.ResourceRef = "second-company-invoice"
	secondCompany.Notice.ResourceRef = secondCompany.ResourceRef
	for _, collision := range []struct{ event, charge string }{
		{"second-company-event", fact.ChargeRef},
		{notice.EventID, "second-company-charge"},
	} {
		attempt := secondCompany
		attempt.Notice.EventID, attempt.ChargeRef = collision.event, collision.charge
		if receipt, err := intake.RecordSettlement(ctx, attempt.Notice, attempt); !errors.Is(err, payment.ErrConflict) || receipt != (adapters.SettlementReceipt{}) {
			t.Fatalf("consumed provider identity did not yield a terminal private conflict: %+v %v", receipt, err)
		}
	}
	if err := database.WithOrgTx(ctx, f.Runtime, scopeB, func(tx pgx.Tx) error {
		var changes int
		if err := tx.QueryRow(ctx, `SELECT (SELECT count(*) FROM org_billing_subscription_events)
 + (SELECT count(*) FROM org_billing_settlement_receipts)`).Scan(&changes); err != nil {
			return err
		}
		if changes != 0 {
			t.Fatalf("provider identity collision changed the second company's billing: %d", changes)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	secondCompany.Notice.EventID, secondCompany.ChargeRef = "second-company-event", "second-company-charge"
	if receipt, err := intake.RecordSettlement(ctx, secondCompany.Notice, secondCompany); err != nil || receipt.Replayed || receipt.Revision != 1 {
		t.Fatalf("second company could not settle an independent charge: %+v %v", receipt, err)
	}

	if err := database.WithOrgTx(ctx, f.Runtime, scopeA, func(tx pgx.Tx) error {
		var receipts, recoveries int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM org_billing_settlement_receipts WHERE applied_revision=3`).Scan(&receipts); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM org_billing_subscription_events
WHERE from_state='PAST_DUE' AND state='ACTIVE' AND event_source='VERIFIED_BILLING'`).Scan(&recoveries); err != nil {
			return err
		}
		if receipts != 1 || recoveries != 1 {
			t.Fatalf("expected one audited settlement and one recovery, got %d receipts and %d recoveries", receipts, recoveries)
		}
		var originalPlan, currentPlan string
		if err := tx.QueryRow(ctx, `SELECT
 (SELECT plan_code FROM org_billing_subscription_events WHERE subscription_id=$1 AND revision=2),
 (SELECT plan_code FROM org_billing_subscription_events WHERE subscription_id=$1 AND revision=3)`, a.RecordID).Scan(&originalPlan, &currentPlan); err != nil {
			return err
		}
		if originalPlan != priorPlan.Plan || currentPlan != key.Plan {
			t.Fatalf("recorded plan history changed: %s → %s", originalPlan, currentPlan)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	// A provider read proves the exact reversal. Open, won, mismatched
	// merchant and historical reversals cannot change the active term.
	event := app.InboxEvent{Provider: notice.Provider, Environment: notice.Environment, Merchant: notice.Merchant,
		EventID: "synthetic-reversal", PayloadHash: strings.Repeat("d", 64), ResourceID: "synthetic-dispute"}
	dispute := app.DisputeFact{Provider: event.Provider, Environment: event.Environment, Merchant: event.Merchant,
		OriginalChargeID: fact.ChargeRef, Amount: payment.BRL(1723), State: payment.DisputePending}
	for _, state := range []payment.DisputeState{payment.DisputePending, payment.DisputeWon} {
		dispute.State = state
		if handled, err := intake.ObserveCompanyDispute(ctx, event, dispute); !handled || err != nil {
			t.Fatalf("nonfinal company dispute was misrouted: %t %v", handled, err)
		}
	}
	assertState := func(scope kernel.OrgScope, want string, revision int64) {
		t.Helper()
		if err := database.WithOrgTx(ctx, f.Runtime, scope, func(tx pgx.Tx) error {
			var state string
			var got int64
			if err := tx.QueryRow(ctx, "SELECT state,revision FROM org_billing_subscription_events ORDER BY revision DESC LIMIT 1").Scan(&state, &got); err != nil {
				return err
			}
			if state != want || got != revision {
				t.Fatalf("company state/revision = %s/%d, want %s/%d", state, got, want, revision)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	assertState(scopeA, "ACTIVE", 3)
	dispute.State, dispute.Amount = payment.DisputeLost, payment.BRL(1724)
	if handled, err := intake.ObserveCompanyDispute(ctx, event, dispute); !handled || !errors.Is(err, payment.ErrBinding) {
		t.Fatalf("chargeback above the reviewed charge was admitted: %t %v", handled, err)
	}
	dispute.Amount = payment.BRL(1723)
	dispute.Merchant = "foreign-merchant"
	foreignEvent := event
	foreignEvent.Merchant = dispute.Merchant
	if handled, err := intake.ObserveCompanyDispute(ctx, foreignEvent, dispute); handled || err != nil {
		t.Fatalf("foreign merchant resolved the company charge: %t %v", handled, err)
	}
	dispute.Merchant = event.Merchant
	assertState(scopeA, "ACTIVE", 3)
	assertState(scopeB, "ACTIVE", 1)

	for range 2 {
		go func() {
			handled, err := intake.ObserveCompanyDispute(ctx, event, dispute)
			if !handled && err == nil {
				err = errors.New("company dispute escaped into personal processing")
			}
			replies <- result{err: err}
		}()
	}
	for range 2 {
		if outcome := <-replies; outcome.err != nil {
			t.Fatal(outcome.err)
		}
	}
	assertState(scopeA, "PAST_DUE", 4)
	assertState(scopeB, "ACTIVE", 1)

	refundEvent := app.InboxRecord{InboxEvent: event}
	refundEvent.EventID, refundEvent.ResourceID, refundEvent.EventType = "synthetic-refund-event", secondCompany.ChargeRef, "charge.refunded"
	refundProbe := &settlementProbe{}
	refund := adapters.CompanyRefund{Intake: intake, Inbox: refundProbe}
	refund.Stripe = verifiedCompanyRefund{ChargeID: secondCompany.ChargeRef, Amount: payment.BRL(1723)}
	if err := refund.Process(ctx, refundEvent); err != nil || refundProbe.marked != "PROCESSED" {
		t.Fatalf("partial refund was not acknowledged: %v", err)
	}
	assertState(scopeB, "ACTIVE", 1)
	refund.Stripe = verifiedCompanyRefund{ChargeID: secondCompany.ChargeRef, Amount: payment.BRL(1723), Full: true}
	if err := refund.Process(ctx, refundEvent); err != nil || refundProbe.marked != "PROCESSED" {
		t.Fatalf("full refund was not acknowledged: %v", err)
	}
	assertState(scopeB, "PAST_DUE", 2)
}

type verifiedCompanyRefund app.ChargeRefundFact

func (v verifiedCompanyRefund) VerifyCompanyRefund(_ context.Context, _ app.InboxRecord) (app.ChargeRefundFact, error) {
	return app.ChargeRefundFact(v), nil
}
