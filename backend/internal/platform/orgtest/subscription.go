//go:build integration

package orgtest

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"

	billing "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgbilling/domain"
	"github.com/jackc/pgx/v5"
)

// PrepareSubscriptionOpening installs only synthetic review inputs. The
// returned function writes evidence in the test's ordinary lifecycle
// transaction. Callers still exercise their usual ledger and authorization.
func (f *Fixture) PrepareSubscriptionOpening(t *testing.T, plan billing.PlanKey, event billing.LifecycleEvent) func(pgx.Tx) error {
	t.Helper()
	ctx := t.Context()
	org := event.Scope.OrgID().String()
	id := "a0000000-0000-4000-8000-" + randomHex(t, 6)
	role := pgx.Identifier{f.RuntimeRole}.Sanitize()
	switch event.Kind {
	case billing.StartTrial:
		if _, err := f.Admin.Exec(ctx, "GRANT SELECT ON org_billing_trial_grants TO "+role+
			"; GRANT SELECT, INSERT ON org_audit_events TO "+role); err != nil {
			t.Fatal(err)
		}
		if _, err := f.Admin.Exec(ctx, `INSERT INTO org_billing_trial_grants
(org_id,id,product_code,plan_code,plan_revision,eligible_from,eligible_until,duration_seconds,approval_ref)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, org, event.SubscriptionID, plan.Product, plan.Plan, plan.Revision,
			event.At.Add(-time.Minute), event.At.Add(time.Minute), int64(event.Period.End.Sub(event.Period.Start)/time.Second), event.Reference); err != nil {
			t.Fatal(err)
		}
		hash := sha256.Sum256([]byte(org + ":" + event.SubscriptionID))
		return func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `INSERT INTO org_audit_events
(org_id,operation_id,actor_id,action,target_type,target_id,request_hash)
VALUES($1,$2,$3,'billing.trial.started','subscription',$4,$5)`,
				org, id, f.Organizations[0].ParentID, event.SubscriptionID, hex.EncodeToString(hash[:]))
			return err
		}
	case billing.ConfirmSettlement:
		if _, err := f.Admin.Exec(ctx, "GRANT SELECT ON org_billing_charge_bindings, enterprise_billing_offers TO "+role+
			"; GRANT SELECT, INSERT ON org_billing_settlement_receipts TO "+role); err != nil {
			t.Fatal(err)
		}
		if _, err := f.Admin.Exec(ctx, `INSERT INTO enterprise_billing_offers
(id,product_code,plan_code,plan_revision,channel,environment,merchant,provider_offer_ref,amount_centavos,term,approval_ref)
VALUES($1,$2,$3,$4,'stripe_card','test','synthetic-opening-merchant','synthetic-opening-offer',1723,'P1M','synthetic-opening-proof')`,
			id, plan.Product, plan.Plan, plan.Revision); err != nil {
			t.Fatal(err)
		}
		if _, err := f.Admin.Exec(ctx, `INSERT INTO org_billing_charge_bindings
(org_id,provider,environment,merchant,resource_ref,subscription_id,offer_id,customer_ref,contract_ref,term_start,term_end)
VALUES($1,'stripe','test','synthetic-opening-merchant',$2,$3,$4,'synthetic-opening-customer','synthetic-opening-contract',$5,$6)`,
			org, "resource-"+id, event.SubscriptionID, id, event.Period.Start, event.Period.End); err != nil {
			t.Fatal(err)
		}
		return func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `INSERT INTO org_billing_settlement_receipts
(org_id,provider,environment,merchant,event_id,payload_hash,resource_ref,charge_ref,subscription_id,applied_revision,settled_at)
VALUES($1,'stripe','test','synthetic-opening-merchant',$2,repeat('a',64),$3,$4,$5,1,$6)`,
				org, "event-"+id, "resource-"+id, "charge-"+id, event.SubscriptionID, event.At)
			return err
		}
	default:
		t.Fatalf("fixture cannot open a subscription with %q", event.Kind)
		return nil
	}
}
