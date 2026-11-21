//go:build integration

package adapters_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	billing "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgbilling/adapters"
	capability "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/domain"
	paypg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/adapters"
	payments "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/application"
	pay "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
	"github.com/jackc/pgx/v5"
)

func TestCompanyAsaasSettlesOnlyTheReviewedScopedCharge(t *testing.T) {
	for _, channel := range []pay.Channel{pay.ChannelPixManual, pay.ChannelPixAutomatic} {
		t.Run(string(channel), func(t *testing.T) { verifyCompanyAsaas(t, channel) })
	}
}

func verifyCompanyAsaas(t *testing.T, channel pay.Channel) {
	t.Helper()
	f := orgtest.New(t)
	ctx := t.Context()
	org, another := f.Organizations[0], f.Organizations[1]
	role := pgx.Identifier{f.RuntimeRole}.Sanitize()
	if _, err := f.Admin.Exec(ctx, "GRANT SELECT ON enterprise_plan_catalog,enterprise_billing_offers,org_billing_charge_bindings TO "+role+
		"; GRANT SELECT,INSERT ON org_billing_subscription_events,org_billing_settlement_receipts TO "+role+
		"; GRANT SELECT,INSERT,UPDATE ON payment_inbox,job_queue TO "+role); err != nil {
		t.Fatal(err)
	}
	known, err := capability.NewCatalog([]string{"organizations"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Admin.Exec(ctx, `INSERT INTO enterprise_plan_catalog
(product_code,plan_code,revision,display_name,capabilities,active_vehicles)
VALUES('fleet','synthetic',1,'Synthetic company',ARRAY['organizations'],4)`); err != nil {
		t.Fatal(err)
	}
	const merchant, customer, resource = "synthetic-merchant", "cus_company_synthetic", "pay_company_synthetic"
	if _, err := f.Admin.Exec(ctx, `INSERT INTO enterprise_billing_offers
(id,product_code,plan_code,plan_revision,channel,environment,merchant,provider_offer_ref,amount_centavos,term,approval_ref)
VALUES($1,'fleet','synthetic',1,$2,'test',$3,'synthetic-company-offer',4567,'P1M','synthetic-approval')`,
		org.ParentID, string(channel), merchant); err != nil {
		t.Fatal(err)
	}
	reference := "abv_org:" + org.ID + ":" + org.ParentID
	contract := resource
	if channel == pay.ChannelPixAutomatic {
		contract = "pix-automatic-authorization-synthetic"
	}
	observed := time.Date(2026, time.November, 21, 10, 0, 0, 0, time.UTC)
	termStart, termEnd := observed.Add(-time.Hour), observed.AddDate(0, 1, 0).Add(-time.Hour)
	if _, err := f.Admin.Exec(ctx, `INSERT INTO org_billing_charge_bindings
(org_id,provider,environment,merchant,resource_ref,subscription_id,offer_id,customer_ref,contract_ref,term_start,term_end,asaas_external_reference)
VALUES($1,'asaas','test',$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		org.ID, merchant, resource, org.RecordID, org.ParentID, customer, contract, termStart, termEnd, reference); err != nil {
		t.Fatal(err)
	}
	proof := map[string]any{
		"id": resource, "customer": customer, "externalReference": reference, "billingType": "PIX", "status": "RECEIVED",
		"value": json.Number("45.67"), "paymentDate": observed.Format(time.DateOnly),
	}
	if channel == pay.ChannelPixAutomatic {
		proof["pixAutomaticAuthorizationId"] = contract
	}
	probe := &settlementProbe{}
	store := paypg.NewStore(f.Runtime)
	receivedAt := observed
	checkedAt := observed.Add(time.Hour)
	intake := billing.BillingIntake{Pool: f.Runtime, Ledger: billing.SubscriptionLedger{Capabilities: known},
		Now: func() time.Time { return checkedAt }}
	client := &http.Client{Transport: stripeRoundTrip(func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodGet || request.URL.Path != "/v3/payments/"+resource {
			t.Fatalf("unexpected provider request: %s %s", request.Method, request.URL.Path)
		}
		body, err := json.Marshal(proof)
		if err != nil {
			t.Fatal(err)
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header),
			Body: io.NopCloser(bytes.NewReader(body)), Request: request}, nil
	})}
	processor := billing.AsaasSettlement{Resources: paypg.NewAsaasProvider("synthetic", pay.EnvironmentTest, client),
		Bindings: intake, Record: intake, Personal: probe, Inbox: store, Merchant: merchant,
		Environment: pay.EnvironmentTest, AutomaticAllowed: true}
	apply := func(label, eventType, want string) {
		t.Helper()
		event := payments.InboxEvent{Provider: pay.ProviderAsaas, Environment: pay.EnvironmentTest, Merchant: merchant,
			EventID: label, EventType: eventType, ResourceType: "payment", ResourceID: resource, PayloadHash: strings.Repeat("a", 64)}
		if _, err := store.RecordInbox(ctx, event); err != nil {
			t.Fatal(err)
		}
		if err := processor.Process(ctx, payments.InboxRecord{InboxEvent: event, ReceivedAt: receivedAt, State: "PENDING"}); err != nil {
			t.Fatal(err)
		}
		var state string
		if err := f.Admin.QueryRow(ctx, "SELECT state FROM payment_inbox WHERE event_id=$1", label).Scan(&state); err != nil || state != want {
			t.Fatalf("unexpected inbox result for %s: %s %v", label, state, err)
		}
	}
	proof["externalReference"] = "abv_org:" + another.ID + ":" + another.ParentID
	apply("synthetic-wrong-company", "PAYMENT_RECEIVED", "CONFLICT")
	proof["externalReference"] = reference
	proof["billingType"] = "CREDIT_CARD"
	apply("synthetic-wrong-payment-method", "PAYMENT_RECEIVED", "CONFLICT")
	proof["billingType"] = "PIX"
	proof["status"] = "CONFIRMED"
	apply("synthetic-is-unsettled", "PAYMENT_RECEIVED", "CONFLICT")
	proof["status"] = "RECEIVED"
	proof["originalValue"] = json.Number("49.00")
	apply("synthetic-is-adjusted", "PAYMENT_RECEIVED", "CONFLICT")
	delete(proof, "originalValue")
	proof["value"] = json.Number("45.66")
	apply("synthetic-underpayment", "PAYMENT_RECEIVED", "CONFLICT")
	proof["value"] = json.Number("45.67")
	if channel == pay.ChannelPixAutomatic {
		proof["pixAutomaticAuthorizationId"] = "unrelated-authorization"
		apply("synthetic-unrelated-authorization", "PAYMENT_RECEIVED", "CONFLICT")
		proof["pixAutomaticAuthorizationId"] = contract
	}
	proof["externalReference"] = "personal-order"
	apply("synthetic-personal", "PAYMENT_RECEIVED", "PENDING")
	proof["externalReference"] = reference
	var inserted int
	if err := f.Admin.QueryRow(ctx, "SELECT count(*) FROM org_billing_settlement_receipts").Scan(&inserted); err != nil ||
		inserted != 0 || probe.personal != 1 {
		t.Fatalf("refused or personal payment activated a company term: receipts=%d personal=%d err=%v", inserted, probe.personal, err)
	}
	lateAt := termEnd.Add(time.Hour)
	receivedAt, checkedAt = lateAt, lateAt.Add(time.Minute)
	apply("synthetic-first-observation-after-term", "PAYMENT_RECEIVED", "CONFLICT")
	receivedAt, checkedAt = observed, observed.Add(time.Hour)
	apply("synthetic-settled", "PAYMENT_RECEIVED", "PROCESSED")
	apply("synthetic-redelivery-with-new-id", "PAYMENT_RECEIVED", "PROCESSED")
	receivedAt, checkedAt = lateAt, lateAt.Add(time.Minute)
	apply("synthetic-redelivery-after-term", "PAYMENT_RECEIVED", "PROCESSED")
	proof["paymentDate"] = observed.AddDate(0, 0, 1).Format(time.DateOnly)
	apply("synthetic-redelivery-changed-day", "PAYMENT_RECEIVED", "CONFLICT")
	proof["paymentDate"] = observed.Format(time.DateOnly)
	proof["value"] = json.Number("45.66")
	apply("synthetic-redelivery-changed-amount", "PAYMENT_RECEIVED", "CONFLICT")
	var receipts, events int
	var paidOn string
	var recorded time.Time
	if err := f.Admin.QueryRow(ctx, `SELECT
 (SELECT count(*) FROM org_billing_settlement_receipts WHERE org_id=$1),
 (SELECT count(*) FROM org_billing_subscription_events WHERE org_id=$1),
 provider_paid_on::text,settled_at
FROM org_billing_settlement_receipts WHERE org_id=$1 AND resource_ref=$2`, org.ID, resource).Scan(&receipts, &events, &paidOn, &recorded); err != nil ||
		receipts != 1 || events != 1 || paidOn != observed.Format(time.DateOnly) || !recorded.Equal(observed) {
		t.Fatalf("verified Pix did not retain one exact scoped receipt: receipts=%d events=%d date=%s err=%v", receipts, events, paidOn, err)
	}
}
