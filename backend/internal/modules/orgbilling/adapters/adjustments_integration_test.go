//go:build integration

package adapters_test

import (
	"context"
	"errors"
	"testing"
	"time"

	auditpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/adapters"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgbilling/adapters"
	billing "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgbilling/domain"
	capability "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestReviewedAdjustmentPostsOnceWithoutChangingSubscriptionOrAuthority(t *testing.T) {
	t.Setenv("ANPFUEL_TEST_RUNTIME_GRANTS", "script")
	ctx := t.Context()
	f := orgtest.New(t)
	a, b := f.Organizations[0], f.Organizations[1]
	scope := a.Scope(t)
	pool := f.RuntimePool(t, 3)
	id := func() string {
		t.Helper()
		value, err := jobs.NewUUIDv4()
		if err != nil {
			t.Fatal(err)
		}
		return value
	}
	cat, err := capability.NewCatalog([]string{"organizations"})
	if err != nil {
		t.Fatal(err)
	}
	key := billing.PlanKey{Product: "fleet", Plan: "reviewed-test", Revision: 1}
	if _, err := f.Admin.Exec(ctx, `INSERT INTO enterprise_plan_catalog
(product_code,plan_code,revision,display_name,capabilities)
VALUES('fleet','reviewed-test',1,'Synthetic adjustment plan',ARRAY['organizations'])`); err != nil {
		t.Fatal(err)
	}
	at := time.Now().UTC().Truncate(time.Microsecond)
	ledger := adapters.SubscriptionLedger{Capabilities: cat}
	opening := billing.LifecycleEvent{Scope: scope, SubscriptionID: a.RecordID, Kind: billing.ConfirmSettlement,
		Source: billing.VerifiedBilling, At: at, Reference: "synthetic-prior-confirmed-term", Period: billing.Period{Start: at, End: at.Add(24 * time.Hour)}}
	proof := f.PrepareSubscriptionOpening(t, key, opening)
	if err := database.WithOrgTx(ctx, pool, scope, func(tx pgx.Tx) error {
		if _, err := ledger.ApplyTx(ctx, tx, key, opening); err != nil {
			return err
		}
		return proof(tx)
	}); err != nil {
		t.Fatal(err)
	}
	approve := func(kind billing.AdjustmentKind, amount int64, from, until time.Time) string {
		t.Helper()
		approval := id()
		if _, err := f.Admin.Exec(ctx, `INSERT INTO org_billing_adjustment_approvals
(org_id,id,subscription_id,subscription_revision,operator_id,kind,amount_centavos,currency,approval_ref,reason_ref,valid_from,valid_until)
VALUES($1,$2,$3,1,$4,$5,$6,'BRL',$7,'synthetic-reason-review',$8,$9)`,
			a.ID, approval, a.RecordID, a.ParentID, kind, amount, "synthetic-review-"+approval, from, until); err != nil {
			t.Fatal(err)
		}
		return approval
	}
	credit := approve(billing.ServiceCredit, 2099, at.Add(-time.Minute), at.Add(time.Hour))
	debit := approve(billing.DebitCorrection, 150, at.Add(-time.Minute), at.Add(time.Hour))
	expired := approve(billing.ServiceCredit, 60, at.Add(-time.Hour), at.Add(-time.Minute))
	catalog, err := audit.NewCatalog(adapters.AdjustmentAuditDefinitions())
	if err != nil {
		t.Fatal(err)
	}
	audits := auditpg.NewStore(pool, catalog)
	store := adapters.AdjustmentStore{Pool: pool, Audit: audits}
	before := func() string {
		t.Helper()
		var snapshot string
		if err := f.Admin.QueryRow(ctx, `SELECT jsonb_build_array(
 (SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,subscription_id,revision) FROM org_billing_subscription_events x),
 (SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,id) FROM org_role_assignments x),
 (SELECT jsonb_agg(to_jsonb(x) ORDER BY id) FROM entitlement_grants x),
 (SELECT count(*) FROM org_billing_settlement_receipts))::text`).Scan(&snapshot); err != nil {
			t.Fatal(err)
		}
		return snapshot
	}()
	if _, err := store.Apply(ctx, b.Scope(t), credit, a.ParentID, id()); !errors.Is(err, adapters.ErrAdjustmentUnavailable) {
		t.Fatalf("foreign exact approval returned %v", err)
	}
	for _, bad := range []struct{ approval, operator string }{
		{credit, b.ParentID}, {expired, a.ParentID}, {id(), a.ParentID},
	} {
		if _, err := store.Apply(ctx, scope, bad.approval, bad.operator, id()); !errors.Is(err, adapters.ErrAdjustmentUnavailable) {
			t.Fatalf("unreviewed operator, approval or expired grant used: %v", err)
		}
	}
	type attempt struct {
		operation string
		receipt   adapters.AdjustmentReceipt
		err       error
	}
	results := make(chan attempt, 2)
	both := make(chan struct{})
	for _, op := range []string{id(), id()} {
		go func() {
			<-both
			r, err := store.Apply(ctx, scope, credit, a.ParentID, op)
			results <- attempt{operation: op, receipt: r, err: err}
		}()
	}
	close(both)
	first, second := <-results, <-results
	if first.err != nil {
		first, second = second, first
	}
	if first.err != nil || !errors.Is(second.err, adapters.ErrAdjustmentPosted) ||
		first.receipt.DeltaCentavos != -2099 || first.receipt.Replayed || first.receipt.SubscriptionID != a.RecordID {
		t.Fatalf("concurrent use did not preserve exactly one credit: %v / %v", first.err, second.err)
	}
	replay, err := store.Apply(ctx, scope, credit, a.ParentID, first.operation)
	expected := first.receipt
	expected.Replayed = true
	if err != nil || replay != expected {
		t.Fatalf("matching retry did not return the same posting: %v", err)
	}
	if _, err := store.Apply(ctx, scope, debit, a.ParentID, first.operation); !errors.Is(err, audit.ErrReused) {
		t.Fatalf("same operation was rebound to a second approval: %v", err)
	}

	incorrect := id()
	err = database.WithOrgTx(ctx, pool, scope, func(tx pgx.Tx) error {
		_, err := audits.RecordChangeTx(ctx, tx, scope, audit.Event{OperationID: incorrect, ActorID: a.ParentID,
			Action: "subs.adjustment.posted", TargetType: "subscription", TargetID: a.RecordID, ReasonCode: "debit_correction", RequestHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}, func(ctx context.Context, tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `INSERT INTO org_billing_adjustment_postings(org_id,operation_id,approval_id,delta_centavos)
VALUES($1,$2,$3,151)`, a.ID, incorrect, debit)
			return err
		})
		return err
	})
	var sqlErr *pgconn.PgError
	if !errors.As(err, &sqlErr) || sqlErr.Code != "23514" {
		t.Fatalf("database accepted a changed adjustment amount: %v", err)
	}
	corrected, err := store.Apply(ctx, scope, debit, a.ParentID, id())
	if err != nil || corrected.DeltaCentavos != 150 || corrected.Replayed {
		t.Fatalf("exact debit did not settle after the failed write: %v", err)
	}
	var after string
	var postings, audited int
	var net int64
	if err := f.Admin.QueryRow(ctx, `SELECT jsonb_build_array(
 (SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,subscription_id,revision) FROM org_billing_subscription_events x),
 (SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,id) FROM org_role_assignments x),
 (SELECT jsonb_agg(to_jsonb(x) ORDER BY id) FROM entitlement_grants x),
 (SELECT count(*) FROM org_billing_settlement_receipts))::text,
 (SELECT count(*) FROM org_billing_adjustment_postings WHERE org_id=$1),
 (SELECT count(*) FROM org_audit_events WHERE org_id=$1 AND action='subs.adjustment.posted'),
 (SELECT sum(delta_centavos)::bigint FROM org_billing_adjustment_postings WHERE org_id=$1)`, a.ID).Scan(&after, &postings, &audited, &net); err != nil || before != after || postings != 2 || audited != 2 || net != -1949 {
		t.Fatalf("billing adjustment isolation or accounting diverged: postings=%d audit=%d net=%d error=%v", postings, audited, net, err)
	}
}
