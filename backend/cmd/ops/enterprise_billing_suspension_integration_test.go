//go:build integration

package main

import (
	"context"
	"crypto/rand"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	billingpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgbilling/adapters"
	billing "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgbilling/domain"
	capability "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
	"github.com/jackc/pgx/v5"
)

func TestReviewedSuspensionOnlyConsumesCurrentPastDueRevision(t *testing.T) {
	f := orgtest.New(t)
	ctx := t.Context()
	org, other := f.Organizations[0], f.Organizations[1]
	scope := org.Scope(t)
	now := time.Now().UTC().Truncate(time.Second).Add(-time.Minute)
	plan := billing.PlanKey{Product: "fleet", Plan: "synthetic", Revision: 1}
	if _, err := f.Admin.Exec(ctx, `INSERT INTO enterprise_plan_catalog
(product_code,plan_code,revision,display_name,capabilities)
VALUES('fleet','synthetic',1,'Synthetic suspension plan',ARRAY['organizations'])`); err != nil {
		t.Fatal(err)
	}
	caps, err := capability.NewCatalog([]string{"organizations"})
	if err != nil {
		t.Fatal(err)
	}
	ledger := billingpg.SubscriptionLedger{Capabilities: caps}
	opening := billing.LifecycleEvent{Scope: scope, SubscriptionID: org.RecordID, Kind: billing.ConfirmSettlement,
		Source: billing.VerifiedBilling, Reference: "synthetic-settlement", At: now, Period: billing.Period{Start: now, End: now.Add(time.Hour)}}
	proof := f.PrepareSubscriptionOpening(t, plan, opening)
	role, secret := f.ProvisionScriptRole(t)
	serving := f.ScriptRuntimePool(t, role, secret, 1)
	if err := database.WithOrgTx(ctx, serving, scope, func(tx pgx.Tx) error {
		if _, err := ledger.ApplyTx(ctx, tx, plan, opening); err != nil {
			return err
		}
		if err := proof(tx); err != nil {
			return err
		}
		_, err := ledger.ApplyTx(ctx, tx, plan, billing.LifecycleEvent{Scope: scope, SubscriptionID: org.RecordID,
			ExpectedRevision: 1, Kind: billing.ReportPastDue, Source: billing.VerifiedBilling,
			Reference: "synthetic-confirmed-reversal", At: now.Add(time.Second)})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Admin.Exec(ctx, `INSERT INTO org_billing_suspension_approvals
(org_id,id,subscription_id,expected_revision,operator_id,decision_ref,valid_from,valid_until)
VALUES($1,$2,$3,2,$4,'synthetic-reviewed-suspension',$5,$6),
      ($1,$7,$3,2,$4,'synthetic-expired-suspension',$5,$8)`, org.ID, org.ParentID,
		org.RecordID, testOperator, now.Add(-time.Hour), now.Add(time.Hour), other.ParentID, now); err != nil {
		t.Fatal(err)
	}
	in := reviewedBillingArgs{scope: scope, approval: org.ParentID, operation: testOp, operator: testOperator}
	if _, _, err := executeBillingSuspension(ctx, serving, in); err == nil {
		t.Fatal("serving role consumed an operator review")
	}

	script, err := os.ReadFile("../../../infra/db/billing-suspension-role.sql")
	if err != nil {
		t.Fatal(err)
	}
	operatorRole := "emp_suspend_" + strings.ToLower(rand.Text())
	operatorSecret := rand.Text()
	name := pgx.Identifier{operatorRole}.Sanitize()
	if _, err := f.Admin.Exec(ctx, "CREATE ROLE "+name+" LOGIN NOINHERIT PASSWORD '"+operatorSecret+"'"); err != nil {
		t.Fatal("could not create disposable suspension login")
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := f.Admin.Exec(ctx, "DROP OWNED BY "+name+"; DROP ROLE "+name); err != nil {
			t.Error(err)
		}
	})
	rendered := strings.NewReplacer(`:'billing_suspension_role'`, "'"+operatorRole+"'", `:"billing_suspension_role"`, name).Replace(string(script))
	if _, err := f.Admin.Exec(ctx, rendered); err != nil {
		t.Fatal(err)
	}
	pool := f.ScriptRuntimePool(t, operatorRole, operatorSecret, 3)
	for _, denied := range []reviewedBillingArgs{
		{scope: other.Scope(t), approval: in.approval, operation: in.operation, operator: in.operator},
		{scope: scope, approval: in.approval, operation: in.operation, operator: testOrg},
		{scope: scope, approval: other.ParentID, operation: in.operation, operator: in.operator},
	} {
		if _, _, err := executeBillingSuspension(ctx, pool, denied); !errors.Is(err, errBillingSuspensionUnavailable) {
			t.Fatalf("foreign, expired or wrong-operator review was admitted: %v", err)
		}
	}
	type outcome struct {
		revision int64
		replayed bool
		err      error
	}
	replies := make(chan outcome, 2)
	for range 2 {
		go func() {
			revision, replayed, err := executeBillingSuspension(ctx, pool, in)
			replies <- outcome{revision, replayed, err}
		}()
	}
	first, second := <-replies, <-replies
	if first.err != nil || second.err != nil || first.revision != 3 || second.revision != 3 || first.replayed == second.replayed {
		t.Fatalf("reviewed suspension did not converge: %+v / %+v", first, second)
	}
	in.operation = testOrg
	if _, _, err := executeBillingSuspension(ctx, pool, in); !errors.Is(err, errBillingSuspensionUnavailable) {
		t.Fatalf("stale review could suspend again: %v", err)
	}
	var count int64
	if err := f.Admin.QueryRow(ctx, `SELECT count(*) FROM org_audit_events
WHERE action='subs.billing.suspended'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("suspension did not retain one audit row: %d / %v", count, err)
	}
}
