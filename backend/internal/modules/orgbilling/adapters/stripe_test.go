package adapters_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgbilling/adapters"
	orgbilling "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgbilling/domain"
	payadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/adapters"
	payment "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/application"
	paydomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/domain"
)

type stripeRoundTrip func(*http.Request) (*http.Response, error)

func (transport stripeRoundTrip) RoundTrip(request *http.Request) (*http.Response, error) {
	return transport(request)
}

type settlementProbe struct {
	fact      orgbilling.CanonicalBillingFact
	recordErr error
	recorded  int
	personal  int
	marked    string
	reason    string
}

func (probe *settlementProbe) RecordSettlement(_ context.Context, notice orgbilling.BillingNotice, fact orgbilling.CanonicalBillingFact) (adapters.SettlementReceipt, error) {
	if notice != fact.Notice {
		return adapters.SettlementReceipt{}, orgbilling.ErrBillingBinding
	}
	probe.fact = fact
	probe.recorded++
	return adapters.SettlementReceipt{}, probe.recordErr
}

func (probe *settlementProbe) Process(_ context.Context, _ payment.InboxRecord) error {
	probe.personal++
	return nil
}

func (probe *settlementProbe) MarkInbox(_ context.Context, _ payment.InboxEvent, state, reason string) error {
	probe.marked, probe.reason = state, reason
	return nil
}

func TestCompanyStripeSettlementUsesCanonicalInvoice(t *testing.T) {
	const canonical = `{
	  "id": "in_synthetic", "livemode": false, "status": "paid", "currency": "brl",
	  "customer": "cus_company", "amount_paid": 1723, "amount_due": 1723,
	  "collection_method": "charge_automatically", "billing_reason": "subscription_cycle",
	  "status_transitions": {"paid_at": 1795255200},
	  "parent": {"type": "subscription_details", "subscription_details": {
	    "subscription": "sub_company", "metadata": {"abastevo_billing_scope": "organization",
	    "abastevo_organization_id": "df12742d-7361-4308-ad01-0167408e778a",
	    "abastevo_organization_subscription_id": "f47e25cb-6e1c-425d-89e3-44c3133fcb62"}}},
	  "lines": {"data": [{"currency": "brl", "amount": 1723, "quantity": 1,
	    "period": {"start": 1795255200, "end": 1797847200},
	    "parent": {"type": "subscription_item_details", "subscription_item_details": {"subscription": "sub_company"}},
	    "pricing": {"type": "price_details", "price_details": {"price": "price_company"}}}]},
	  "payments": {"data": [{"status": "paid", "is_default": true, "currency": "brl", "amount_paid": 1723,
	    "payment": {"type": "payment_intent", "payment_intent": "pi_company"}}]}
	}`
	const canonicalIntent = `{
	  "id": "pi_company", "livemode": false, "status": "succeeded", "amount_received": 1723,
	  "currency": "brl", "customer": "cus_company",
	  "latest_charge": {"id": "ch_company", "livemode": false, "paid": true, "captured": true,
	    "amount_captured": 1723, "currency": "brl", "payment_method_details": {"type": "card"}}
	}`
	event := payment.InboxRecord{InboxEvent: payment.InboxEvent{
		Provider: paydomain.ProviderStripe, Environment: paydomain.EnvironmentTest, Merchant: "acct_synthetic",
		EventID: "evt_synthetic", PayloadHash: strings.Repeat("a", 64), ResourceID: "in_synthetic",
		ResourceType: "invoice", EventType: "invoice.paid",
	}, State: "PENDING"}

	for _, tc := range []struct {
		name, old, replacement string
		recordErr              error
		marked                 string
		recorded, personal     int
		intentOverride         string
	}{
		{name: "settled", marked: "PROCESSED", recorded: 1},
		{name: "personal", old: `"organization"`, replacement: `"personal"`, personal: 1},
		{name: "unsettled invoice", old: `"status": "paid"`, replacement: `"status": "open"`, marked: "CONFLICT"},
		{name: "foreign invoice", old: `"in_synthetic"`, replacement: `"in_other"`, marked: "CONFLICT"},
		{name: "live invoice", old: `"livemode": false`, replacement: `"livemode": true`, marked: "CONFLICT"},
		{name: "noncard settlement", old: `"type": "payment_intent"`, replacement: `"type": "payment_record"`, marked: "CONFLICT"},
		{name: "proration", old: `{"subscription": "sub_company"}`, replacement: `{"subscription": "sub_company", "proration": true}`, marked: "CONFLICT"},
		{name: "wrong card source", intentOverride: strings.Replace(canonicalIntent, `"card"`, `"pix"`, 1), marked: "CONFLICT"},
		{name: "partially refunded", intentOverride: strings.Replace(canonicalIntent, `"paid": true`, `"paid": true, "amount_refunded": 1`, 1), marked: "CONFLICT"},
		{name: "foreign price", recordErr: orgbilling.ErrBillingBinding, marked: "CONFLICT", recorded: 1},
		{name: "temporary dependency failure", recordErr: paydomain.ErrUnavailable, recorded: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := canonical
			if tc.old != "" {
				body = strings.Replace(canonical, tc.old, tc.replacement, 1)
			}
			probe := &settlementProbe{recordErr: tc.recordErr}
			client := &http.Client{Transport: stripeRoundTrip(func(request *http.Request) (*http.Response, error) {
				if request.Method != http.MethodGet {
					t.Fatalf("unexpected canonical request method: %s", request.Method)
				}
				var response string
				switch request.URL.Path {
				case "/v1/invoices/in_synthetic":
					response = body
				case "/v1/payment_intents/pi_company":
					response = canonicalIntent
					if tc.intentOverride != "" {
						response = tc.intentOverride
					}
				default:
					t.Fatalf("unexpected canonical read: %s", request.URL.Path)
				}
				return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header),
					Body: io.NopCloser(strings.NewReader(response)), Request: request}, nil
			})}
			s := adapters.StripeSettlement{
				Resources: payadapters.NewStripeProvider("synthetic", paydomain.EnvironmentTest, client),
				Record:    probe, Personal: probe, Inbox: probe, Merchant: "acct_synthetic", Environment: paydomain.EnvironmentTest,
			}
			err := s.Process(t.Context(), event)
			if tc.recordErr == paydomain.ErrUnavailable {
				if !errors.Is(err, tc.recordErr) {
					t.Fatalf("pending event lost the retryable error: %v", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if probe.marked != tc.marked || probe.recorded != tc.recorded || probe.personal != tc.personal {
				t.Fatalf("wrong owner or acknowledgement: %+v", probe)
			}
			if tc.name == "settled" && (probe.fact.Channel != paydomain.ChannelCard ||
				probe.fact.Scope.OrgID().String() != "df12742d-7361-4308-ad01-0167408e778a" ||
				probe.fact.CustomerRef != "cus_company" || probe.fact.ContractRef != "sub_company" ||
				probe.fact.ProviderOfferRef != "price_company" || probe.fact.ChargeRef != "ch_company" ||
				!probe.fact.Gross.Equal(paydomain.BRL(1723)) || !probe.fact.At.Equal(time.Unix(1795255200, 0))) {
				t.Fatalf("canonical facts were not preserved: %+v", probe.fact)
			}
		})
	}
}
