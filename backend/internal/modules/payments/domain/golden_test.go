package domain

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

const goldenFixture = "../../../../../contracts/testdata/payments/offer-policy-v1.json"

type goldenFile struct {
	Status string       `json:"status"`
	Cases  []goldenCase `json:"cases"`
}

type goldenCase struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	Offer   string `json:"offer"`
	Channel string `json:"channel"`

	SettledAt  string  `json:"settled_at"`
	CurrentEnd *string `json:"current_end"`
	AnchorDay  int     `json:"anchor_day"`

	PeriodStart string `json:"period_start"`
	PeriodEnd   string `json:"period_end"`
	CheckedAt   string `json:"checked_at"`

	PaymentAmountCentavos int64   `json:"payment_amount_centavos"`
	RefundsCentavos       []int64 `json:"refunds_centavos"`
	ConfirmedAt           string  `json:"confirmed_at"`
	DisputeStatus         string  `json:"dispute_status"`

	SameOrder              bool    `json:"same_order"`
	SettledAmountsCentavos []int64 `json:"settled_amounts_centavos"`
	PeriodsAlreadyFunded   int     `json:"periods_already_funded"`
	SettledAmountCentavos  int64   `json:"settled_amount_centavos"`
	SettledCurrency        string  `json:"settled_currency"`

	OldSource       string `json:"old_source"`
	NewSource       string `json:"new_source"`
	OldCancellation string `json:"old_cancellation"`
	RequestedAt     string `json:"requested_at"`

	RequestedFeature string `json:"requested_feature"`
	RequestedTerm    string `json:"requested_term"`

	Expected struct {
		Start                       string  `json:"start"`
		End                         string  `json:"end"`
		Reason                      string  `json:"reason"`
		Access                      *bool   `json:"access"`
		RefundableRemainingCentavos *int64  `json:"refundable_remaining_centavos"`
		PeriodsFunded               *int    `json:"periods_funded"`
		ExcessCentavos              *int64  `json:"excess_centavos"`
		NewDebitAllowed             *bool   `json:"new_debit_allowed"`
		FirstDebitAt                *string `json:"first_debit_at"`
		SaleEnabled                 *bool   `json:"sale_enabled"`
	} `json:"expected"`
}

func TestGoldenPolicyFixture(t *testing.T) {
	path := filepath.Clean(goldenFixture)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var file goldenFile
	if err := json.Unmarshal(data, &file); err != nil {
		t.Fatal(err)
	}
	if file.Status != "SYNTHETIC_TEST_ONLY" {
		t.Fatalf("fixture status = %s", file.Status)
	}
	seen := map[string]bool{}
	for _, tc := range file.Cases {
		tc := tc
		if seen[tc.ID] {
			t.Fatalf("duplicate fixture id %s", tc.ID)
		}
		seen[tc.ID] = true
		t.Run(tc.ID, func(t *testing.T) {
			switch tc.Kind {
			case "period":
				checkGoldenPeriod(t, tc)
			case "access":
				checkGoldenAccess(t, tc)
			case "refund":
				checkGoldenRefund(t, tc)
			case "dispute":
				checkGoldenDispute(t, tc)
			case "excess":
				checkGoldenExcess(t, tc)
			case "amount":
				checkGoldenAmount(t, tc)
			case "switch":
				checkGoldenSwitch(t, tc)
			case "sale":
				checkGoldenSale(t, tc)
			default:
				t.Fatalf("unknown fixture kind %q", tc.Kind)
			}
		})
	}
	if len(seen) != 26 {
		t.Fatalf("fixture cases = %d want 26", len(seen))
	}
}

func checkGoldenPeriod(t *testing.T, tc goldenCase) {
	t.Helper()
	var current time.Time
	if tc.CurrentEnd != nil {
		current = fixtureTime(t, *tc.CurrentEnd)
	}
	period, err := NextPeriod(fixtureTime(t, tc.SettledAt), current, tc.AnchorDay)
	if err != nil {
		t.Fatal(err)
	}
	if fixtureFormat(period.Start) != tc.Expected.Start ||
		fixtureFormat(period.End) != tc.Expected.End ||
		period.Reason != tc.Expected.Reason {
		t.Fatalf("period = %+v", period)
	}
}

func checkGoldenAccess(t *testing.T, tc goldenCase) {
	t.Helper()
	entitlement, _ := entitlementWithGrant(t, fixtureTime(t, tc.PeriodStart), fixtureTime(t, tc.PeriodEnd), tc.ID)
	access := entitlement.Evaluate(fixtureTime(t, tc.CheckedAt), true)
	if tc.Expected.Access == nil || access.Granted() != *tc.Expected.Access {
		t.Fatalf("access = %+v", access)
	}
	if tc.Expected.Reason != "" && access.Reason != tc.Expected.Reason {
		t.Fatalf("reason = %s want %s", access.Reason, tc.Expected.Reason)
	}
}

func checkGoldenRefund(t *testing.T, tc goldenCase) {
	t.Helper()
	entitlement, grant := entitlementWithGrant(t, fixtureTime(t, tc.PeriodStart), fixtureTime(t, tc.PeriodEnd), tc.ID)
	confirmed := fixtureTime(t, tc.ConfirmedAt)
	for index, amount := range tc.RefundsCentavos {
		refund, err := entitlement.ReserveRefund(grant.ID, fmt.Sprintf("refund_%s_%d", tc.ID, index), BRL(amount), confirmed)
		if err != nil {
			t.Fatal(err)
		}
		if err := entitlement.ConfirmRefund(grant.ID, refund.ID, confirmed); err != nil {
			t.Fatal(err)
		}
	}
	access := entitlement.Evaluate(fixtureTime(t, tc.CheckedAt), true)
	if tc.Expected.Access == nil || access.Granted() != *tc.Expected.Access {
		t.Fatalf("access = %+v", access)
	}
	if tc.Expected.RefundableRemainingCentavos != nil {
		remaining := entitlement.grantByID(grant.ID).Refundable().Centavos
		if remaining != *tc.Expected.RefundableRemainingCentavos {
			t.Fatalf("refundable = %d want %d", remaining, *tc.Expected.RefundableRemainingCentavos)
		}
	}
}

func checkGoldenDispute(t *testing.T, tc goldenCase) {
	t.Helper()
	entitlement, grant := entitlementWithGrant(t, fixtureTime(t, tc.PeriodStart), fixtureTime(t, tc.PeriodEnd), tc.ID)
	checked := fixtureTime(t, tc.CheckedAt)
	opened := checked.Add(-time.Hour)
	if _, err := entitlement.OpenDispute(grant.ID, "dispute_"+tc.ID, BRL(1234), opened); err != nil {
		t.Fatal(err)
	}
	target := DisputeUnderReview
	if tc.DisputeStatus == "CONFIRMED" {
		target = DisputeLost
	}
	if err := entitlement.UpdateDispute(grant.ID, "dispute_"+tc.ID, target, checked.Add(-30*time.Minute)); err != nil {
		t.Fatal(err)
	}
	access := entitlement.Evaluate(checked, true)
	if tc.Expected.Access == nil || access.Granted() != *tc.Expected.Access {
		t.Fatalf("access = %+v", access)
	}
}

func checkGoldenExcess(t *testing.T, tc goldenCase) {
	t.Helper()
	entitlement := testEntitlement(t)
	fundedByCase := 0
	if tc.SameOrder {
		order, attempt, first := fixturePurchase(t, &entitlement, tc.ID+"_1", testNow.Add(time.Minute))
		applyGrant(t, &entitlement, &order, attempt, first)
		fundedByCase = 1
		second := first
		second.ID = "settlement_" + tc.ID + "_2"
		second.RemotePaymentID = "remote_" + tc.ID + "_2"
		second.SettledAt = testNow.Add(2 * time.Minute)
		result, err := entitlement.ApplySettlement(validOffer(), &order, &attempt, second, testNow.Add(3*time.Minute))
		if err != nil || !result.Excess {
			t.Fatalf("second result=%+v err=%v", result, err)
		}
	} else {
		for index := 0; index < tc.PeriodsAlreadyFunded; index++ {
			order, attempt, settlement := fixturePurchase(t, &entitlement, fmt.Sprintf("%s_%d", tc.ID, index), testNow.Add(time.Duration(index+1)*2*time.Hour))
			applyGrant(t, &entitlement, &order, attempt, settlement)
		}
		before := len(entitlement.Grants)
		order, attempt, settlement := fixturePurchase(t, &entitlement, tc.ID+"_extra", testNow.Add(time.Duration(tc.PeriodsAlreadyFunded+2)*2*time.Hour))
		result, err := entitlement.ApplySettlement(validOffer(), &order, &attempt, settlement, settlement.SettledAt.Add(time.Hour))
		if err != nil || !result.Excess {
			t.Fatalf("extra result=%+v err=%v", result, err)
		}
		fundedByCase = len(entitlement.Grants) - before
	}
	if tc.Expected.PeriodsFunded != nil && fundedByCase != *tc.Expected.PeriodsFunded {
		t.Fatalf("periods funded = %d want %d", fundedByCase, *tc.Expected.PeriodsFunded)
	}
	if tc.Expected.ExcessCentavos != nil {
		var total int64
		for _, settlement := range entitlement.Excess {
			total += settlement.Amount.Centavos
		}
		if total != *tc.Expected.ExcessCentavos {
			t.Fatalf("excess = %d want %d", total, *tc.Expected.ExcessCentavos)
		}
	}
	if tc.Expected.Access != nil {
		checked := testNow.Add(5 * time.Hour)
		if len(entitlement.Grants) > 0 {
			last := entitlement.Grants[len(entitlement.Grants)-1]
			if last.Start.After(checked) {
				checked = last.Start.Add(time.Minute)
			}
		}
		access := entitlement.Evaluate(checked, true)
		if access.Granted() != *tc.Expected.Access {
			t.Fatalf("access = %+v", access)
		}
	}
}

func checkGoldenAmount(t *testing.T, tc goldenCase) {
	t.Helper()
	catalog := testCatalog(t)
	money := Money{Centavos: tc.SettledAmountCentavos, Currency: tc.SettledCurrency}
	if _, err := catalog.Quote(QuoteRequest{OfferID: "premium.monthly", Channel: ChannelPixManual, Amount: &money}, testNow); !errors.Is(err, ErrAmountMismatch) {
		t.Fatalf("amount err = %v", err)
	}
	if tc.Expected.PeriodsFunded == nil || *tc.Expected.PeriodsFunded != 0 ||
		tc.Expected.ExcessCentavos == nil || *tc.Expected.ExcessCentavos != 0 {
		t.Fatal("fixture expectation")
	}
}

func checkGoldenSwitch(t *testing.T, tc goldenCase) {
	t.Helper()
	decision, err := DecideSwitch(SwitchRequest{
		OldSource:       Channel(tc.OldSource),
		NewSource:       Channel(tc.NewSource),
		OldCancellation: CancellationState(tc.OldCancellation),
		CurrentEnd:      fixtureTime(t, tc.CurrentEndForSwitch()),
		RequestedAt:     fixtureTime(t, tc.RequestedAt),
	})
	if err != nil {
		t.Fatal(err)
	}
	if tc.Expected.NewDebitAllowed == nil || decision.Allowed != *tc.Expected.NewDebitAllowed {
		t.Fatalf("decision = %+v", decision)
	}
	if tc.Expected.FirstDebitAt != nil {
		if fixtureFormat(decision.FirstDebitAt) != *tc.Expected.FirstDebitAt {
			t.Fatalf("first debit = %s want %s", fixtureFormat(decision.FirstDebitAt), *tc.Expected.FirstDebitAt)
		}
	} else if !decision.FirstDebitAt.IsZero() {
		t.Fatalf("unexpected first debit = %s", fixtureFormat(decision.FirstDebitAt))
	}
	if decision.Reason != tc.Expected.Reason {
		t.Fatalf("reason = %s want %s", decision.Reason, tc.Expected.Reason)
	}
}

func (c goldenCase) CurrentEndForSwitch() string {
	if c.CurrentEnd != nil {
		return *c.CurrentEnd
	}
	return ""
}

func checkGoldenSale(t *testing.T, tc goldenCase) {
	t.Helper()
	var catalog Catalog
	var request QuoteRequest
	switch tc.ID {
	case "premium-production-price-unresolved":
		offer := validOffer()
		offer.SaleEnabled = false
		offer.Price = BRL(0)
		catalog, _ = NewCatalog("pay-catalog-v1", offer)
		request = QuoteRequest{OfferID: offer.ID, Channel: ChannelPixManual}
	case "enterprise-deliverable-not-implemented":
		catalog, _ = NewCatalog("pay-catalog-v1", enterpriseOffer())
		request = QuoteRequest{OfferID: "enterprise.regional_brief.v1", Channel: ChannelPixManual}
	default:
		catalog = testCatalog(t)
		request = QuoteRequest{
			OfferID: "premium.monthly",
			Channel: ChannelPixManual,
			Feature: tc.RequestedFeature,
			Term:    Term(tc.RequestedTerm),
		}
		if tc.Channel != "" {
			request.Channel = Channel(tc.Channel)
		}
	}
	quote, err := catalog.Quote(request, testNow)
	if err == nil {
		t.Fatalf("sale unexpectedly enabled: %+v", quote)
	}
	if tc.Expected.SaleEnabled != nil && *tc.Expected.SaleEnabled {
		t.Fatal("fixture expects enabled sale")
	}
}

func entitlementWithGrant(t *testing.T, start, end time.Time, id string) (Entitlement, Grant) {
	t.Helper()
	entitlement := testEntitlement(t)
	grant := testGrant(start, end)
	grant.ID = "grant_" + id
	grant.OrderID = "order_" + id
	grant.SettlementID = "settlement_" + id
	grant.RemotePaymentID = "remote_" + id
	entitlement.Grants = []Grant{grant}
	if err := entitlement.Validate(); err != nil {
		t.Fatal(err)
	}
	return entitlement, grant
}

func fixturePurchase(t *testing.T, entitlement *Entitlement, id string, settledAt time.Time) (Order, Attempt, Settlement) {
	t.Helper()
	order, err := NewOrder("order_"+id, "account_1", testQuote(t, ChannelPixManual), testNow)
	if err != nil {
		t.Fatal(err)
	}
	if err := order.MarkPending(testNow); err != nil {
		t.Fatal(err)
	}
	attempt, err := NewAttempt("attempt_"+id, order.ID, ProviderAsaas, "create_pix")
	if err != nil {
		t.Fatal(err)
	}
	if err := attempt.Dispatch(testNow); err != nil {
		t.Fatal(err)
	}
	if err := attempt.Succeed("remote_"+id, testNow); err != nil {
		t.Fatal(err)
	}
	settlement := testSettlement(t, order, attempt, "settlement_"+id, "remote_"+id, settledAt)
	return order, attempt, settlement
}

func fixtureTime(t *testing.T, value string) time.Time {
	t.Helper()
	if value == "" {
		return time.Time{}
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatal(err)
	}
	return parsed.UTC()
}

func fixtureFormat(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}
