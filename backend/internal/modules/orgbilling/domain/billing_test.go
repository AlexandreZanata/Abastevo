package domain_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgbilling/domain"
	payment "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/domain"
)

func TestVerifiedBillingFactBindsExactApprovedOffer(t *testing.T) {
	org := scope(t, orgID)
	at := time.Date(2026, 11, 21, 10, 0, 0, 0, time.UTC)
	for _, channel := range []payment.Channel{payment.ChannelCard, payment.ChannelPixManual, payment.ChannelPixAutomatic} {
		t.Run(string(channel), func(t *testing.T) {
			plan := d.PlanKey{Product: "fleet", Plan: "synthetic", Revision: 1}
			offer := d.BillingOffer{
				Plan: plan, Channel: channel, Environment: payment.EnvironmentTest,
				Merchant: "synthetic-merchant", ProviderOfferRef: "synthetic-offer",
				Price: payment.BRL(1723), ApprovalRef: "synthetic-reviewed-offer", Term: payment.TermMonthly,
			}
			binding := d.BillingBinding{
				Scope: org, SubscriptionID: subsID, Offer: offer,
				ProviderCustomerRef: "synthetic-customer", ProviderContractRef: "synthetic-contract",
				ExpectedTerm: d.Period{Start: at, End: at.AddDate(0, 1, 0)},
			}
			notice := d.BillingNotice{
				Provider: channel.Provider(), Environment: payment.EnvironmentTest, Merchant: offer.Merchant,
				EventID: "synthetic-event-1", PayloadHash: strings.Repeat("a", 64), ResourceRef: "synthetic-resource",
			}
			fact := d.CanonicalBillingFact{
				Notice: notice, Scope: org, SubscriptionID: subsID,
				CustomerRef: binding.ProviderCustomerRef, ContractRef: binding.ProviderContractRef,
				ProviderOfferRef: offer.ProviderOfferRef, Channel: channel, ResourceRef: notice.ResourceRef,
				ChargeRef: "synthetic-charge", Outcome: d.BillingSettled, At: at, Gross: offer.Price,
				PaidTerm: d.Period{Start: at, End: at.AddDate(0, 1, 0)},
			}
			if channel.Provider() == payment.ProviderAsaas {
				binding.AsaasExternalRef = "abv_org:" + orgID + ":" + subsID
				fact.AsaasExternalRef, fact.ProviderPaidOn = binding.AsaasExternalRef, at.Truncate(24*time.Hour)
			}

			event, err := binding.SettlementEvent(notice, fact, 4, at)
			if err != nil || event.Source != d.VerifiedBilling || event.Scope != org || event.SubscriptionID != subsID ||
				event.Period != fact.PaidTerm || event.ExpectedRevision != 4 || len(event.Reference) != 64 {
				t.Fatalf("exact canonical fact did not map: %+v %v", event, err)
			}
			for _, alter := range []func(*d.CanonicalBillingFact){
				func(f *d.CanonicalBillingFact) { f.Scope = scope(t, "7b0e2a54-0000-4000-8000-000000000099") },
				func(f *d.CanonicalBillingFact) { f.Notice.Environment = payment.EnvironmentProduction },
				func(f *d.CanonicalBillingFact) { f.Notice.Merchant = "another-merchant" },
				func(f *d.CanonicalBillingFact) { f.ProviderOfferRef = "another-offer" },
				func(f *d.CanonicalBillingFact) { f.CustomerRef = "different-customer" },
				func(f *d.CanonicalBillingFact) { f.ContractRef = "different-contract" },
				func(f *d.CanonicalBillingFact) { f.ResourceRef = "different-resource" },
				func(f *d.CanonicalBillingFact) { f.Gross = payment.BRL(1722) },
				func(f *d.CanonicalBillingFact) { f.Gross.Currency = "USD" },
				func(f *d.CanonicalBillingFact) { f.PaidTerm.End = f.PaidTerm.End.Add(time.Hour) },
			} {
				bad := fact
				alter(&bad)
				if _, err := binding.SettlementEvent(notice, bad, 4, at); err == nil {
					t.Fatal("mismatched provider or offer fact activated the subscription")
				}
			}
			unpaid := fact
			unpaid.Outcome = d.BillingUnsettled
			if _, err := binding.SettlementEvent(notice, unpaid, 4, at); !errors.Is(err, d.ErrBillingUnsettled) {
				t.Fatalf("unsettled event supplied a lifecycle transition: %v", err)
			}
			future := fact
			future.PaidTerm = d.Period{Start: at.AddDate(0, 0, 1), End: at.AddDate(0, 1, 1)}
			futureBinding := binding
			futureBinding.ExpectedTerm = future.PaidTerm
			if _, err := futureBinding.SettlementEvent(notice, future, 4, at); !errors.Is(err, d.ErrBillingFutureTerm) {
				t.Fatalf("future term affected the current subscription: %v", err)
			}
			effective, err := futureBinding.SettlementEvent(notice, future, 4, future.PaidTerm.Start)
			if err != nil || !effective.At.Equal(future.PaidTerm.Start) {
				t.Fatalf("future term did not start at its recorded boundary: %v", err)
			}
			binding.Offer.ApprovalRef = ""
			if _, err := binding.SettlementEvent(notice, fact, 4, at); !errors.Is(err, d.ErrOfferUnapproved) {
				t.Fatalf("unapproved offer entered billing: %v", err)
			}
		})
	}
}
