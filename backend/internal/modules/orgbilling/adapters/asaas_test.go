package adapters_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgbilling/adapters"
	orgbilling "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgbilling/domain"
	payment "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/application"
	paydomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/domain"
)

type companyPixStub struct {
	proof payment.CompanyPixProof
	err   error
}

func (stub companyPixStub) RetrieveCompanyPix(context.Context, string) (payment.CompanyPixProof, error) {
	return stub.proof, stub.err
}

type asaasBindingStub struct{ binding orgbilling.BillingBinding }

func (stub asaasBindingStub) ReadAsaasBinding(context.Context, kernel.OrgScope, orgbilling.BillingNotice, string) (orgbilling.BillingBinding, error) {
	return stub.binding, nil
}

func asaasReceived() payment.InboxRecord {
	return payment.InboxRecord{InboxEvent: payment.InboxEvent{Provider: paydomain.ProviderAsaas, Environment: paydomain.EnvironmentTest,
		Merchant: "synthetic-merchant", EventID: "evt_synthetic", EventType: "PAYMENT_RECEIVED", ResourceType: "payment",
		ResourceID: "pay_synthetic", PayloadHash: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"},
		ReceivedAt: time.Date(2026, time.November, 21, 10, 0, 0, 0, time.UTC), State: "PENDING"}
}

// A deterministic provider answer must settle the inbox row: the payment
// router drains in order and stops at the first error, so a returned error
// would hold every later Asaas event behind this one.
func TestCompanyAsaasNeverHoldsTheInboxOnADeterministicProviderAnswer(t *testing.T) {
	for _, providerErr := range []error{paydomain.ErrNotFound, paydomain.ErrConflict} {
		probe := &settlementProbe{}
		processor := adapters.AsaasSettlement{Resources: companyPixStub{err: providerErr}, Bindings: asaasBindingStub{},
			Record: probe, Personal: probe, Inbox: probe, Merchant: "synthetic-merchant", Environment: paydomain.EnvironmentTest}
		if err := processor.Process(t.Context(), asaasReceived()); err != nil || probe.personal != 1 || probe.recorded != 0 {
			t.Fatalf("%v was not handed to the personal processor: err=%v personal=%d", providerErr, err, probe.personal)
		}
	}
	for _, transient := range []error{paydomain.ErrUnavailable, payment.ErrAmbiguous, payment.ErrProviderUnauthorized} {
		probe := &settlementProbe{}
		processor := adapters.AsaasSettlement{Resources: companyPixStub{err: transient}, Bindings: asaasBindingStub{},
			Record: probe, Personal: probe, Inbox: probe, Merchant: "synthetic-merchant", Environment: paydomain.EnvironmentTest}
		if err := processor.Process(t.Context(), asaasReceived()); !errors.Is(err, transient) || probe.personal != 0 || probe.marked != "" {
			t.Fatalf("transient %v was not retried: err=%v personal=%d marked=%q", transient, err, probe.personal, probe.marked)
		}
	}
}

func TestCompanyAsaasRefusesAutomaticPixWhileTheChannelIsDisabled(t *testing.T) {
	event := asaasReceived()
	reference := "abv_org:df12742d-7361-4308-ad01-0167408e778a:f47e25cb-6e1c-425d-89e3-44c3133fcb62"
	proof := payment.CompanyPixProof{ID: event.ResourceID, CustomerID: "cus_synthetic", ExternalReference: reference,
		Environment: paydomain.EnvironmentTest, Method: "PIX", Status: "RECEIVED", PaidOn: event.ReceivedAt.Add(-time.Hour),
		AuthorizationID: "pix-automatic-authorization-synthetic"}
	probe := &settlementProbe{}
	processor := adapters.AsaasSettlement{Resources: companyPixStub{proof: proof}, Bindings: asaasBindingStub{},
		Record: probe, Personal: probe, Inbox: probe, Merchant: "synthetic-merchant", Environment: paydomain.EnvironmentTest}
	if err := processor.Process(t.Context(), event); err != nil || probe.marked != "CONFLICT" || probe.recorded != 0 || probe.personal != 0 {
		t.Fatalf("disabled automatic Pix held the inbox or settled: err=%v marked=%q recorded=%d", err, probe.marked, probe.recorded)
	}
}

type asaasMissingBinding struct{}

func (asaasMissingBinding) ReadAsaasBinding(context.Context, kernel.OrgScope, orgbilling.BillingNotice, string) (orgbilling.BillingBinding, error) {
	return orgbilling.BillingBinding{}, orgbilling.ErrChargeBindingMissing
}

// Only a Stripe renewal waits for its binding. A company Pix charge is
// reviewed and bound before it is paid, so a payment without a binding is a
// terminal conflict and never a personal settlement.
func TestCompanyAsaasPaymentBeforeItsBindingIsATerminalConflict(t *testing.T) {
	event := asaasReceived()
	for name, authorization := range map[string]string{"manual": "", "automatic": "pix-automatic-authorization-synthetic"} {
		t.Run(name, func(t *testing.T) {
			proof := payment.CompanyPixProof{ID: event.ResourceID, CustomerID: "cus_synthetic",
				ExternalReference: "abv_org:df12742d-7361-4308-ad01-0167408e778a:f47e25cb-6e1c-425d-89e3-44c3133fcb62",
				Environment:       paydomain.EnvironmentTest, Method: "PIX", Status: "RECEIVED",
				PaidOn: event.ReceivedAt.Add(-time.Hour), Amount: paydomain.BRL(1723), AuthorizationID: authorization}
			probe := &settlementProbe{}
			processor := adapters.AsaasSettlement{Resources: companyPixStub{proof: proof}, Bindings: asaasMissingBinding{},
				Record: probe, Personal: probe, Inbox: probe, Merchant: "synthetic-merchant", Environment: paydomain.EnvironmentTest,
				AutomaticAllowed: true}
			if err := processor.Process(t.Context(), event); err != nil || probe.marked != "CONFLICT" ||
				probe.recorded != 0 || probe.personal != 0 || !probe.deferred.IsZero() {
				t.Fatalf("unbound company Pix was not a terminal conflict: err=%v probe=%+v", err, probe)
			}
		})
	}
}
