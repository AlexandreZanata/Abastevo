//go:build integration

package adapters_test

import (
	"errors"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgbilling/adapters"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgbilling/domain"
	capability "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestSubscriptionLedgerRetainsAuthorityAndSerializesRevisions(t *testing.T) {
	f := orgtest.New(t)
	ctx := t.Context()
	role := pgx.Identifier{f.RuntimeRole}.Sanitize()
	if _, err := f.Admin.Exec(ctx, "GRANT SELECT ON enterprise_plan_catalog TO "+role+
		"; GRANT SELECT,INSERT ON org_billing_subscription_events TO "+role); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Admin.Exec(ctx, `INSERT INTO enterprise_plan_catalog
(product_code,plan_code,revision,display_name,capabilities,active_vehicles)
VALUES('fleet','synthetic',1,'Synthetic plan',ARRAY['organizations'],3)`); err != nil {
		t.Fatal(err)
	}
	known, err := capability.NewCatalog([]string{"organizations"})
	if err != nil {
		t.Fatal(err)
	}
	ledger := adapters.SubscriptionLedger{Capabilities: known}
	a, b := f.Organizations[0], f.Organizations[1]
	scopeA, scopeB := a.Scope(t), b.Scope(t)
	key := d.PlanKey{Product: "fleet", Plan: "synthetic", Revision: 1}
	at := time.Date(2026, 11, 21, 10, 0, 0, 0, time.UTC)
	period := d.Period{Start: at, End: at.Add(time.Hour * 24)}
	seed := d.LifecycleEvent{
		Scope: scopeA, SubscriptionID: a.RecordID, Kind: d.ConfirmSettlement, Source: d.VerifiedBilling,
		At: at, Reference: "synthetic-first-settlement", Period: period,
	}
	// The reviewed offer and binding exist and the serving role can read the
	// proof tables, so only the missing settlement receipt refuses the opening.
	proof := f.PrepareSubscriptionOpening(t, key, seed)
	err = database.WithOrgTx(ctx, f.Runtime, scopeA, func(tx pgx.Tx) error {
		_, err := ledger.ApplyTx(ctx, tx, key, seed)
		return err
	})
	var openingError *pgconn.PgError
	if !errors.As(err, &openingError) || openingError.Code != "23514" ||
		openingError.Message != "subscription opening proof unavailable" {
		t.Fatalf("first paid term committed without settlement proof: %v", err)
	}
	apply := func(scope kernel.OrgScope, event d.LifecycleEvent) (d.Transition, error) {
		var transition d.Transition
		err := database.WithOrgTx(ctx, f.Runtime, scope, func(tx pgx.Tx) error {
			var err error
			transition, err = ledger.ApplyTx(ctx, tx, key, event)
			if err == nil && event.ExpectedRevision == 0 {
				return proof(tx)
			}
			return err
		})
		return transition, err
	}
	if _, err := apply(scopeB, seed); err == nil {
		t.Fatal("a foreign transaction entered billing history")
	}
	if tr, err := apply(scopeA, seed); err != nil || tr.To != d.Active || tr.Revision != 1 {
		t.Fatalf("verified settlement was not recorded: %+v %v", tr, err)
	}
	failed := d.LifecycleEvent{
		Scope: scopeA, SubscriptionID: a.RecordID, ExpectedRevision: 1, Kind: d.ReportPastDue,
		Source: d.VerifiedBilling, At: at.Add(time.Hour), Reference: "synthetic-overdue",
	}
	if tr, err := apply(scopeA, failed); err != nil || tr.To != d.PastDue || tr.Revision != 2 {
		t.Fatalf("verified overdue state missing: %+v %v", tr, err)
	}

	paid := seed
	paid.ExpectedRevision, paid.At, paid.Reference = 2, at.Add(2*time.Hour), "synthetic-restoration"
	outcomes := make(chan error, 2)
	for range 2 {
		go func() {
			_, err := apply(scopeA, paid)
			outcomes <- err
		}()
	}
	var applied, stale int
	for range 2 {
		switch err := <-outcomes; {
		case err == nil:
			applied++
		case errors.Is(err, d.ErrStaleSubscription):
			stale++
		default:
			t.Fatalf("unexpected concurrent settlement outcome: %v", err)
		}
	}
	if applied != 1 || stale != 1 {
		t.Fatalf("settlement transition applied %d times with %d conflicts", applied, stale)
	}

	// These assertions run with the same limited login as the repository.
	if err := database.WithOrgTx(ctx, f.Runtime, scopeA, func(tx pgx.Tx) error {
		var count int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM org_billing_subscription_events
WHERE org_id=$1 AND subscription_id=$2`, a.ID, a.RecordID).Scan(&count); err != nil {
			return err
		}
		if count != 3 {
			t.Fatalf("expected exactly three retained decisions, found %d", count)
		}
		var from, state, source, reason string
		if err := tx.QueryRow(ctx, `SELECT from_state,state,event_source,evidence_reference
FROM org_billing_subscription_events WHERE org_id=$1 AND subscription_id=$2 AND revision=3`,
			a.ID, a.RecordID).Scan(&from, &state, &source, &reason); err != nil {
			return err
		}
		if from != "PAST_DUE" || state != "ACTIVE" || source != "VERIFIED_BILLING" || reason != paid.Reference {
			t.Fatal("restoration lost its source or prior state")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := database.WithOrgTx(ctx, f.Runtime, scopeB, func(tx pgx.Tx) error {
		var count int
		if err := tx.QueryRow(ctx, "SELECT count(*) FROM org_billing_subscription_events").Scan(&count); err != nil {
			return err
		}
		if count != 0 {
			t.Fatal("foreign organization saw subscription history")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	_, err = f.Admin.Exec(ctx, `INSERT INTO org_billing_subscription_events
(org_id,subscription_id,revision,product_code,plan_code,plan_revision,from_state,state,prior_state,
 event_kind,event_source,evidence_reference,occurred_at,term_start,term_end)
SELECT org_id,subscription_id,revision+2,product_code,plan_code,plan_revision,state,'PAST_DUE','',
 'REPORT_PAST_DUE','VERIFIED_BILLING','synthetic-skipped-revision',occurred_at,term_start,term_end
FROM org_billing_subscription_events WHERE org_id=$1 AND subscription_id=$2 AND revision=3`, a.ID, a.RecordID)
	var refusal *pgconn.PgError
	if !errors.As(err, &refusal) || refusal.Code != "23514" {
		t.Fatalf("database accepted a skipped history revision: %v", err)
	}
	_, err = f.Admin.Exec(ctx, "UPDATE org_billing_subscription_events SET evidence_reference='rewritten'")
	if !errors.As(err, &refusal) || refusal.Code != "55000" {
		t.Fatalf("database allowed old billing evidence to be rewritten: %v", err)
	}

	entitlements := adapters.EntitlementStore{Pool: f.Runtime, Capabilities: known, Now: func() time.Time { return at.Add(5 * time.Hour) }}
	expect := func(scope kernel.OrgScope, governed, allowed bool) {
		t.Helper()
		bound, enabled, err := entitlements.PlanCapability(ctx, scope, "organizations")
		if err != nil || bound != governed || enabled != allowed {
			t.Fatalf("scoped entitlement: got (%t,%t) %v want (%t,%t)", bound, enabled, err, governed, allowed)
		}
	}
	expect(scopeA, true, true)
	expect(scopeB, false, false)
	finish := d.LifecycleEvent{
		Scope: scopeA, SubscriptionID: a.RecordID, ExpectedRevision: 3, Kind: d.ConfirmCancellation,
		Source: d.VerifiedBilling, At: at.Add(3 * time.Hour), Reference: "synthetic-cancellation",
	}
	if transition, err := apply(scopeA, finish); err != nil || transition.To != d.Cancelled {
		t.Fatalf("verified cancellation: %+v %v", transition, err)
	}
	expect(scopeA, true, false)

}
