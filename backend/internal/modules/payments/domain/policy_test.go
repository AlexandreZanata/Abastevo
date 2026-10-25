package domain

import (
	"errors"
	"sort"
	"testing"
	"time"

	subdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/subscription/domain"
)

var testNow = time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)

func validOffer() Offer {
	return Offer{
		ID:           "premium.monthly",
		Family:       "premium",
		Term:         TermMonthly,
		Price:        BRL(1234),
		Capabilities: PremiumCapabilityList(),
		Channels: map[Channel]bool{
			ChannelCard:         true,
			ChannelPixManual:    true,
			ChannelPixAutomatic: true,
		},
		SaleEnabled: true,
	}
}

func enterpriseOffer() Offer {
	return Offer{
		ID:          "enterprise.regional_brief.v1",
		Family:      "enterprise",
		Term:        TermMonthly,
		Price:       BRL(0),
		Channels:    map[Channel]bool{ChannelCard: true, ChannelPixManual: true},
		SaleEnabled: false,
		Enterprise:  true,
	}
}

func testCatalog(t *testing.T) Catalog {
	t.Helper()
	catalog, err := NewCatalog("pay-catalog-v1", validOffer(), enterpriseOffer())
	if err != nil {
		t.Fatal(err)
	}
	return catalog
}

func testQuote(t *testing.T, channel Channel) Quote {
	t.Helper()
	quote, err := testCatalog(t).Quote(QuoteRequest{OfferID: "premium.monthly", Channel: channel}, testNow)
	if err != nil {
		t.Fatal(err)
	}
	return quote
}

func testOrder(t *testing.T, channel Channel) Order {
	t.Helper()
	order, err := NewOrder("order_1", "account_1", testQuote(t, channel), testNow)
	if err != nil {
		t.Fatal(err)
	}
	if err := order.MarkPending(testNow); err != nil {
		t.Fatal(err)
	}
	return order
}

func testAttempt(t *testing.T, order Order) Attempt {
	t.Helper()
	attempt, err := NewAttempt("attempt_1", order.ID, ProviderAsaas, "create_pix")
	if err != nil {
		t.Fatal(err)
	}
	if err := attempt.Dispatch(testNow); err != nil {
		t.Fatal(err)
	}
	if err := attempt.Succeed("remote_1", testNow); err != nil {
		t.Fatal(err)
	}
	return attempt
}

func testSettlement(t *testing.T, order Order, attempt Attempt, id, remote string, at time.Time) Settlement {
	t.Helper()
	settlement := Settlement{
		ID:              id,
		OrderID:         order.ID,
		AttemptID:       attempt.ID,
		AccountID:       order.AccountID,
		OfferID:         order.OfferID,
		Channel:         order.Channel,
		Provider:        attempt.Provider,
		Environment:     EnvironmentTest,
		MerchantID:      "merchant_1",
		RemotePaymentID: remote,
		ProviderStatus:  "RECEIVED",
		Amount:          order.Amount,
		Status:          SettlementSettled,
		Authority:       AuthorityProviderVerified,
		SettledAt:       at,
	}
	if err := settlement.Validate(); err != nil {
		t.Fatal(err)
	}
	return settlement
}

func testEntitlement(t *testing.T) Entitlement {
	t.Helper()
	entitlement, err := NewEntitlement("account_1", "premium.monthly")
	if err != nil {
		t.Fatal(err)
	}
	return entitlement
}

func applyGrant(t *testing.T, entitlement *Entitlement, order *Order, attempt Attempt, settlement Settlement) Grant {
	t.Helper()
	result, err := entitlement.ApplySettlement(validOffer(), order, &attempt, settlement, settlement.SettledAt.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if result.Grant == nil {
		t.Fatalf("no grant: %+v", result)
	}
	return *result.Grant
}

func testGrant(start, end time.Time) Grant {
	return Grant{
		ID:              "grant_1",
		OrderID:         "order_1",
		SettlementID:    "settlement_1",
		RemotePaymentID: "remote_1",
		Source:          ChannelPixManual,
		Paid:            BRL(1234),
		Start:           start,
		End:             end,
		AnchorDay:       start.In(time.FixedZone("BRT", -3*3600)).Day(),
		Capabilities:    PremiumCapabilityList(),
		State:           GrantActive,
		PolicyVersion:   PolicyVersion,
	}
}

func TestMoneyIsExactAndBounded(t *testing.T) {
	if got := BRL(1234).String(); got != "12.34" {
		t.Fatalf("decimal = %q", got)
	}
	if got := BRL(5).String(); got != "0.05" {
		t.Fatalf("decimal = %q", got)
	}
	if err := BRL(1234).Validate(); err != nil {
		t.Fatal(err)
	}
	for _, money := range []Money{{Centavos: 1, Currency: "USD"}, BRL(-1), BRL(MaxAmountCentavos + 1)} {
		if err := money.Validate(); err == nil {
			t.Fatalf("invalid money accepted: %+v", money)
		}
	}
	sum, err := BRL(1234).Add(BRL(766))
	if err != nil || !sum.Equal(BRL(2000)) {
		t.Fatalf("add = %+v, %v", sum, err)
	}
	if _, err := BRL(MaxAmountCentavos).Add(BRL(1)); err == nil {
		t.Fatal("overflow accepted")
	}
	if _, err := BRL(1).Sub(BRL(2)); err == nil {
		t.Fatal("negative accepted")
	}
	if _, err := BRL(1).Add(Money{Centavos: 1, Currency: "USD"}); err == nil {
		t.Fatal("currency mix accepted")
	}
	if !BRL(1234).Equal(BRL(1234)) || BRL(1234).Equal(BRL(1235)) {
		t.Fatal("equality")
	}
}

func TestCatalogQuoteRejectsTamperingAndUnfinishedSales(t *testing.T) {
	catalog := testCatalog(t)
	valid, err := catalog.Quote(QuoteRequest{OfferID: "premium.monthly", Channel: ChannelPixManual}, testNow)
	if err != nil {
		t.Fatal(err)
	}
	if valid.Amount != BRL(1234) || len(valid.Capabilities) != 7 {
		t.Fatalf("quote = %+v", valid)
	}
	valid.Capabilities[0] = "invented.capability"
	again, err := catalog.Quote(QuoteRequest{OfferID: "premium.monthly", Channel: ChannelPixManual}, testNow)
	if err != nil {
		t.Fatal(err)
	}
	if again.Capabilities[0] != "cloud.sync" {
		t.Fatal("caller mutated catalog capabilities")
	}
	caps := PremiumCapabilityList()
	caps[0] = "invented.capability"
	if PremiumCapabilityList()[0] != "cloud.sync" {
		t.Fatal("caller mutated global capabilities")
	}

	amount := BRL(1000)
	term := Term("P1Y")
	cases := []struct {
		name    string
		request QuoteRequest
		want    error
	}{
		{"unknown offer", QuoteRequest{OfferID: "unknown", Channel: ChannelPixManual}, ErrUnknownOffer},
		{"wrong amount", QuoteRequest{OfferID: "premium.monthly", Channel: ChannelPixManual, Amount: &amount}, ErrAmountMismatch},
		{"annual term", QuoteRequest{OfferID: "premium.monthly", Channel: ChannelPixManual, Term: term}, ErrTermUnsupported},
		{"trial", QuoteRequest{OfferID: "premium.monthly", Channel: ChannelPixManual, Feature: "trial"}, ErrFeatureUnsupported},
		{"android", QuoteRequest{OfferID: "premium.monthly", Channel: Channel("android_play")}, ErrChannelUnsupported},
		{"enterprise", QuoteRequest{OfferID: "enterprise.regional_brief.v1", Channel: ChannelPixManual}, ErrPriceUnresolved},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := catalog.Quote(tc.request, testNow); !errors.Is(err, tc.want) {
				t.Fatalf("err = %v want %v", err, tc.want)
			}
		})
	}

	disabled := validOffer()
	disabled.SaleEnabled = false
	disabledCatalog, err := NewCatalog("pay-catalog-v1", disabled)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := disabledCatalog.Quote(QuoteRequest{OfferID: disabled.ID, Channel: ChannelPixManual}, testNow); !errors.Is(err, ErrSaleDisabled) {
		t.Fatalf("disabled sale err = %v", err)
	}
	unpriced := disabled
	unpriced.Price = BRL(0)
	unpricedCatalog, err := NewCatalog("pay-catalog-v1", unpriced)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := unpricedCatalog.Quote(QuoteRequest{OfferID: unpriced.ID, Channel: ChannelPixManual}, testNow); !errors.Is(err, ErrPriceUnresolved) {
		t.Fatalf("unpriced sale err = %v", err)
	}

	if _, err := NewCatalog("pay-catalog-v1", validOffer(), validOffer()); err == nil {
		t.Fatal("duplicate offer accepted")
	}
}

func TestOrderAndAttemptTransitions(t *testing.T) {
	quote := testQuote(t, ChannelPixManual)
	order, err := NewOrder("order_1", "account_1", quote, testNow)
	if err != nil {
		t.Fatal(err)
	}
	if err := order.MarkPaid("settlement_1", testNow); !errors.Is(err, ErrTransition) {
		t.Fatalf("redirect-only payment err = %v", err)
	}
	if err := order.MarkPending(testNow); err != nil {
		t.Fatal(err)
	}
	if err := order.MarkPaid("settlement_1", testNow.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := order.MarkPaid("settlement_1", testNow.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := order.MarkPaid("settlement_2", testNow.Add(3*time.Minute)); !errors.Is(err, ErrConflict) {
		t.Fatalf("second settlement err = %v", err)
	}

	expiring, err := NewOrder("order_2", "account_1", testQuote(t, ChannelPixManual), testNow)
	if err != nil {
		t.Fatal(err)
	}
	late := testNow.Add(QuoteTTL)
	if err := expiring.MarkPending(testNow.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	expiring, err = NewOrder("order_2", "account_1", testQuote(t, ChannelPixManual), testNow)
	if err != nil {
		t.Fatal(err)
	}
	if err := expiring.Expire(late); err != nil {
		t.Fatal(err)
	}
	if err := expiring.MarkPaid("settlement_late", late); err != nil {
		t.Fatalf("late payment rejected: %v", err)
	}

	canceled, err := NewOrder("order_3", "account_1", testQuote(t, ChannelPixManual), testNow)
	if err != nil {
		t.Fatal(err)
	}
	if err := canceled.Cancel(testNow); err != nil {
		t.Fatal(err)
	}
	if err := canceled.MarkPending(testNow); !errors.Is(err, ErrTransition) {
		t.Fatalf("canceled order pending err = %v", err)
	}

	attempt, err := NewAttempt("attempt_1", "order_1", ProviderAsaas, "create_pix")
	if err != nil {
		t.Fatal(err)
	}
	if err := attempt.Succeed("remote_1", testNow); !errors.Is(err, ErrTransition) {
		t.Fatalf("success before dispatch err = %v", err)
	}
	if err := attempt.Dispatch(testNow); err != nil {
		t.Fatal(err)
	}
	if err := attempt.Succeed("remote_1", testNow.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := attempt.Succeed("remote_1", testNow.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := attempt.Succeed("remote_2", testNow.Add(3*time.Second)); !errors.Is(err, ErrConflict) {
		t.Fatalf("changed remote id err = %v", err)
	}

	unknown, _ := NewAttempt("attempt_2", "order_1", ProviderAsaas, "create_pix")
	if err := unknown.Dispatch(testNow); err != nil {
		t.Fatal(err)
	}
	if err := unknown.Unknown("", testNow.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := unknown.ResolveSuccess("remote_2", testNow.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	if unknown.State != AttemptSucceeded {
		t.Fatal("unknown not resolved")
	}

	failed, _ := NewAttempt("attempt_3", "order_1", ProviderAsaas, "create_pix")
	if err := failed.Dispatch(testNow); err != nil {
		t.Fatal(err)
	}
	if err := failed.Unknown("", testNow.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := failed.ResolveFailure(testNow.Add(2 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if failed.State != AttemptFailed {
		t.Fatal("unknown failure not resolved")
	}

	quarantined, _ := NewAttempt("attempt_4", "order_1", ProviderAsaas, "create_pix")
	if err := quarantined.Dispatch(testNow); err != nil {
		t.Fatal(err)
	}
	if err := quarantined.Unknown("remote_4", testNow.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := quarantined.Quarantine(testNow.Add(2 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if quarantined.State != AttemptQuarantined {
		t.Fatal("unknown not quarantined")
	}
}

func TestStateMachinePermutations(t *testing.T) {
	checkMachine(t, "order", []OrderState{OrderQuoted, OrderPending, OrderPaid, OrderExpired, OrderCanceled},
		map[OrderState][]OrderState{
			OrderQuoted:   {OrderPending, OrderExpired, OrderCanceled},
			OrderPending:  {OrderPaid, OrderExpired, OrderCanceled},
			OrderExpired:  {OrderPaid},
			OrderPaid:     {},
			OrderCanceled: {},
		}, canTransitionOrder)
	checkMachine(t, "attempt", []AttemptState{AttemptCreated, AttemptDispatched, AttemptSucceeded, AttemptFailed, AttemptUnknown, AttemptQuarantined, AttemptCanceled},
		map[AttemptState][]AttemptState{
			AttemptCreated:     {AttemptDispatched, AttemptCanceled},
			AttemptDispatched:  {AttemptSucceeded, AttemptFailed, AttemptUnknown},
			AttemptUnknown:     {AttemptSucceeded, AttemptFailed, AttemptQuarantined},
			AttemptSucceeded:   {},
			AttemptFailed:      {},
			AttemptQuarantined: {},
			AttemptCanceled:    {},
		}, canTransitionAttempt)
	checkMachine(t, "refund", []RefundState{RefundRequested, RefundPending, RefundSucceeded, RefundFailed, RefundDenied},
		map[RefundState][]RefundState{
			RefundRequested: {RefundPending, RefundSucceeded, RefundFailed, RefundDenied},
			RefundPending:   {RefundSucceeded, RefundFailed},
			RefundSucceeded: {},
			RefundFailed:    {},
			RefundDenied:    {},
		}, canTransitionRefund)
	checkMachine(t, "dispute", []DisputeState{DisputePending, DisputeUnderReview, DisputeWon, DisputeLost, DisputeClosed},
		map[DisputeState][]DisputeState{
			DisputePending:     {DisputeUnderReview, DisputeWon, DisputeLost, DisputeClosed},
			DisputeUnderReview: {DisputeWon, DisputeLost, DisputeClosed},
			DisputeWon:         {},
			DisputeLost:        {},
			DisputeClosed:      {},
		}, canTransitionDispute)
	checkMachine(t, "contract", []ContractState{ContractPending, ContractActive, ContractPastDue, ContractCanceled, ContractExpired},
		map[ContractState][]ContractState{
			ContractPending:  {ContractActive, ContractCanceled, ContractExpired},
			ContractActive:   {ContractPastDue, ContractCanceled, ContractExpired},
			ContractPastDue:  {ContractActive, ContractCanceled, ContractExpired},
			ContractCanceled: {},
			ContractExpired:  {},
		}, canTransitionContract)
	checkMachine(t, "mandate", []MandateState{MandateCreated, MandateActive, MandateRefused, MandateCanceled, MandateExpired},
		map[MandateState][]MandateState{
			MandateCreated:  {MandateActive, MandateRefused, MandateCanceled, MandateExpired},
			MandateActive:   {MandateCanceled, MandateExpired},
			MandateRefused:  {},
			MandateCanceled: {},
			MandateExpired:  {},
		}, canTransitionMandate)

	grant := testGrant(testNow, testNow.Add(time.Hour))
	if !grant.State.Valid() {
		t.Fatal("grant state")
	}
	grant.revoke()
	if grant.State != GrantRevoked {
		t.Fatal("grant not revoked")
	}
}

func checkMachine[S ~string](t *testing.T, name string, states []S, allowed map[S][]S, can func(S, S) bool) {
	t.Helper()
	for _, from := range states {
		for _, to := range states {
			want := false
			for _, target := range allowed[from] {
				if target == to {
					want = true
					break
				}
			}
			if got := can(from, to); got != want {
				t.Fatalf("%s %s -> %s = %v want %v", name, from, to, got, want)
			}
		}
	}
}

func TestSettlementCannotGrantWithoutVerifiedAuthority(t *testing.T) {
	order := testOrder(t, ChannelPixManual)
	attempt := testAttempt(t, order)
	base := testSettlement(t, order, attempt, "settlement_1", "remote_1", testNow.Add(time.Minute))

	for _, authority := range []SettlementAuthority{AuthorityRedirect, AuthorityAuthorization, AuthorityManual, AuthorityUnknown} {
		t.Run(string(authority), func(t *testing.T) {
			entitlement := testEntitlement(t)
			settlement := base
			settlement.Authority = authority
			result, err := entitlement.ApplySettlement(validOffer(), &order, &attempt, settlement, testNow.Add(2*time.Minute))
			if !errors.Is(err, ErrBinding) || result.Grant != nil || len(entitlement.Grants) != 0 {
				t.Fatalf("authority %s result=%+v err=%v", authority, result, err)
			}
		})
	}
	for _, status := range []SettlementStatus{SettlementPending, SettlementUnknown} {
		t.Run(string(status), func(t *testing.T) {
			entitlement := testEntitlement(t)
			settlement := base
			settlement.Status = status
			if _, err := entitlement.ApplySettlement(validOffer(), &order, &attempt, settlement, testNow.Add(2*time.Minute)); !errors.Is(err, ErrBinding) {
				t.Fatalf("status %s err=%v", status, err)
			}
			if len(entitlement.Grants) != 0 {
				t.Fatal("granted from non-settled state")
			}
		})
	}

	entitlement := testEntitlement(t)
	quotedOrder, _ := NewOrder("order_quoted", "account_1", testQuote(t, ChannelPixManual), testNow)
	if _, err := entitlement.ApplySettlement(validOffer(), &quotedOrder, &attempt, base, testNow.Add(2*time.Minute)); !errors.Is(err, ErrBinding) && !errors.Is(err, ErrTransition) {
		t.Fatalf("redirect-only order err=%v", err)
	}
	if len(entitlement.Grants) != 0 {
		t.Fatal("quoted order granted")
	}

	notSucceeded, _ := NewAttempt("attempt_dispatch", order.ID, ProviderAsaas, "create_pix")
	if err := notSucceeded.Dispatch(testNow); err != nil {
		t.Fatal(err)
	}
	settlement := base
	settlement.AttemptID = notSucceeded.ID
	if _, err := entitlement.ApplySettlement(validOffer(), &order, &notSucceeded, settlement, testNow.Add(2*time.Minute)); !errors.Is(err, ErrBinding) {
		t.Fatalf("authorization-only err=%v", err)
	}
	if len(entitlement.Grants) != 0 {
		t.Fatal("authorization-only granted")
	}

	mismatch := base
	mismatch.Amount = BRL(1000)
	if _, err := entitlement.ApplySettlement(validOffer(), &order, &attempt, mismatch, testNow.Add(2*time.Minute)); !errors.Is(err, ErrBinding) {
		t.Fatalf("amount mismatch err=%v", err)
	}
	future := base
	future.SettledAt = testNow.Add(time.Hour)
	if _, err := entitlement.ApplySettlement(validOffer(), &order, &attempt, future, testNow.Add(time.Minute)); !errors.Is(err, ErrInvalid) {
		t.Fatalf("future settlement err=%v", err)
	}
	if len(entitlement.Grants) != 0 {
		t.Fatal("invalid settlement granted")
	}

	tampered := validOffer()
	tampered.Capabilities = append([]string(nil), tampered.Capabilities...)
	tampered.Capabilities[0] = "invented.capability"
	if _, err := entitlement.ApplySettlement(tampered, &order, &attempt, base, testNow.Add(2*time.Minute)); !errors.Is(err, ErrBinding) {
		t.Fatalf("tampered capabilities err=%v", err)
	}

	result, err := entitlement.ApplySettlement(validOffer(), &order, &attempt, base, testNow.Add(2*time.Minute))
	if err != nil || result.Grant == nil || order.State != OrderPaid {
		t.Fatalf("valid settlement result=%+v err=%v order=%s", result, err, order.State)
	}
	if !entitlement.Evaluate(testNow.Add(3*time.Minute), true).Granted() {
		t.Fatal("valid settlement did not grant")
	}
	if err := entitlement.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestDuplicateAndExcessSettlementsDoNotDoubleGrant(t *testing.T) {
	entitlement := testEntitlement(t)
	order := testOrder(t, ChannelPixManual)
	attempt := testAttempt(t, order)
	first := testSettlement(t, order, attempt, "settlement_1", "remote_1", testNow.Add(time.Minute))
	grant := applyGrant(t, &entitlement, &order, attempt, first)

	replay, err := entitlement.ApplySettlement(validOffer(), &order, &attempt, first, testNow.Add(2*time.Minute))
	if err != nil || replay.Grant == nil || replay.Grant.ID != grant.ID || len(entitlement.Grants) != 1 {
		t.Fatalf("idempotent replay result=%+v err=%v grants=%d", replay, err, len(entitlement.Grants))
	}

	secondAttempt, _ := NewAttempt("attempt_2", "order_2", ProviderAsaas, "create_pix")
	if err := secondAttempt.Dispatch(testNow); err != nil {
		t.Fatal(err)
	}
	if err := secondAttempt.Succeed("remote_2", testNow); err != nil {
		t.Fatal(err)
	}
	secondOrder, _ := NewOrder("order_2", "account_1", testQuote(t, ChannelPixManual), testNow)
	if err := secondOrder.MarkPending(testNow); err != nil {
		t.Fatal(err)
	}
	second := testSettlement(t, secondOrder, secondAttempt, "settlement_2", "remote_2", testNow.Add(2*time.Hour))
	result, err := entitlement.ApplySettlement(validOffer(), &secondOrder, &secondAttempt, second, testNow.Add(3*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if result.Grant == nil || len(entitlement.Grants) != 2 {
		t.Fatalf("early renewal not granted: %+v", result)
	}

	thirdOrder, _ := NewOrder("order_3", "account_1", testQuote(t, ChannelPixManual), testNow)
	if err := thirdOrder.MarkPending(testNow); err != nil {
		t.Fatal(err)
	}
	thirdAttempt, _ := NewAttempt("attempt_3", thirdOrder.ID, ProviderAsaas, "create_pix")
	if err := thirdAttempt.Dispatch(testNow); err != nil {
		t.Fatal(err)
	}
	if err := thirdAttempt.Succeed("remote_3", testNow); err != nil {
		t.Fatal(err)
	}
	third := testSettlement(t, thirdOrder, thirdAttempt, "settlement_3", "remote_3", testNow.Add(3*time.Hour))
	result, err = entitlement.ApplySettlement(validOffer(), &thirdOrder, &thirdAttempt, third, testNow.Add(4*time.Hour))
	if err != nil || !result.Excess || result.Reason != ResultFutureExists {
		t.Fatalf("third settlement result=%+v err=%v", result, err)
	}
	if len(entitlement.Grants) != 2 || len(entitlement.Excess) != 1 {
		t.Fatalf("grants=%d excess=%d", len(entitlement.Grants), len(entitlement.Excess))
	}
	if err := entitlement.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestRefundsAndDisputesAffectOnlyMatchedGrant(t *testing.T) {
	entitlement := testEntitlement(t)
	order := testOrder(t, ChannelPixManual)
	attempt := testAttempt(t, order)
	settlement := testSettlement(t, order, attempt, "settlement_1", "remote_1", testNow.Add(time.Minute))
	grant := applyGrant(t, &entitlement, &order, attempt, settlement)

	partial, err := entitlement.ReserveRefund(grant.ID, "refund_1", BRL(500), testNow.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if err := entitlement.ConfirmRefund(grant.ID, partial.ID, testNow.Add(2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if !entitlement.Evaluate(testNow.Add(3*time.Hour), true).Granted() {
		t.Fatal("partial refund revoked access")
	}
	grantPtr := entitlement.grantByID(grant.ID)
	if !grantPtr.Refundable().Equal(BRL(734)) {
		t.Fatalf("refundable = %+v", grantPtr.Refundable())
	}

	if _, err := entitlement.ReserveRefund(grant.ID, "refund_over", BRL(735), testNow.Add(4*time.Hour)); !errors.Is(err, ErrRefundBounds) {
		t.Fatalf("over-refund err=%v", err)
	}
	if _, err := entitlement.ReserveRefund(grant.ID, "refund_1", BRL(1), testNow.Add(4*time.Hour)); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate refund err=%v", err)
	}

	failed, err := entitlement.ReserveRefund(grant.ID, "refund_failed", BRL(100), testNow.Add(4*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if err := entitlement.MarkRefundPending(grant.ID, failed.ID, testNow.Add(4*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := entitlement.ReserveRefund(grant.ID, "refund_blocked", BRL(700), testNow.Add(4*time.Hour)); !errors.Is(err, ErrRefundBounds) {
		t.Fatalf("pending refund not reserved: %v", err)
	}
	if err := entitlement.FailRefund(grant.ID, failed.ID, testNow.Add(4*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if !entitlement.grantByID(grant.ID).Refundable().Equal(BRL(734)) {
		t.Fatal("failed refund did not release reservation")
	}

	full, err := entitlement.ReserveRefund(grant.ID, "refund_2", BRL(734), testNow.Add(5*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if err := entitlement.MarkRefundPending(grant.ID, full.ID, testNow.Add(6*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := entitlement.ConfirmRefund(grant.ID, full.ID, testNow.Add(7*time.Hour)); err != nil {
		t.Fatal(err)
	}
	access := entitlement.Evaluate(testNow.Add(8*time.Hour), true)
	if access.Granted() || access.State != AccessRevoked {
		t.Fatalf("full refund access = %+v", access)
	}

	futureEntitlement := testEntitlement(t)
	currentOrder := testOrder(t, ChannelPixManual)
	currentAttempt := testAttempt(t, currentOrder)
	current := testSettlement(t, currentOrder, currentAttempt, "settlement_current", "remote_current", testNow.Add(time.Minute))
	applyGrant(t, &futureEntitlement, &currentOrder, currentAttempt, current)

	futureOrder, _ := NewOrder("order_future", "account_1", testQuote(t, ChannelPixManual), testNow)
	if err := futureOrder.MarkPending(testNow); err != nil {
		t.Fatal(err)
	}
	futureAttempt, _ := NewAttempt("attempt_future", futureOrder.ID, ProviderAsaas, "create_pix")
	if err := futureAttempt.Dispatch(testNow); err != nil {
		t.Fatal(err)
	}
	if err := futureAttempt.Succeed("remote_future", testNow); err != nil {
		t.Fatal(err)
	}
	futureSettlement := testSettlement(t, futureOrder, futureAttempt, "settlement_future", "remote_future", testNow.Add(2*time.Hour))
	futureGrant := applyGrant(t, &futureEntitlement, &futureOrder, futureAttempt, futureSettlement)

	refund, err := futureEntitlement.ReserveRefund(futureGrant.ID, "refund_future", BRL(1234), testNow.Add(3*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if err := futureEntitlement.ConfirmRefund(futureGrant.ID, refund.ID, testNow.Add(4*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if !futureEntitlement.Evaluate(testNow.Add(5*time.Hour), true).Granted() {
		t.Fatal("future refund revoked current access")
	}

	disputeEntitlement := testEntitlement(t)
	disputeOrder := testOrder(t, ChannelPixManual)
	disputeAttempt := testAttempt(t, disputeOrder)
	disputeSettlement := testSettlement(t, disputeOrder, disputeAttempt, "settlement_dispute", "remote_dispute", testNow.Add(time.Minute))
	disputeGrant := applyGrant(t, &disputeEntitlement, &disputeOrder, disputeAttempt, disputeSettlement)
	if _, err := disputeEntitlement.OpenDispute(disputeGrant.ID, "dispute_1", BRL(1234), testNow.Add(2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if !disputeEntitlement.Evaluate(testNow.Add(3*time.Hour), true).Granted() {
		t.Fatal("pending dispute revoked access")
	}
	if err := disputeEntitlement.UpdateDispute(disputeGrant.ID, "dispute_1", DisputeUnderReview, testNow.Add(4*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if !disputeEntitlement.Evaluate(testNow.Add(5*time.Hour), true).Granted() {
		t.Fatal("under-review dispute revoked access")
	}
	if err := disputeEntitlement.UpdateDispute(disputeGrant.ID, "dispute_1", DisputeLost, testNow.Add(6*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if disputeEntitlement.Evaluate(testNow.Add(7*time.Hour), true).Granted() {
		t.Fatal("lost dispute kept access")
	}

	wonEntitlement := testEntitlement(t)
	wonOrder := testOrder(t, ChannelPixManual)
	wonAttempt := testAttempt(t, wonOrder)
	wonSettlement := testSettlement(t, wonOrder, wonAttempt, "settlement_won", "remote_won", testNow.Add(time.Minute))
	wonGrant := applyGrant(t, &wonEntitlement, &wonOrder, wonAttempt, wonSettlement)
	if _, err := wonEntitlement.OpenDispute(wonGrant.ID, "dispute_won", BRL(1234), testNow.Add(2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := wonEntitlement.UpdateDispute(wonGrant.ID, "dispute_won", DisputeWon, testNow.Add(3*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if !wonEntitlement.Evaluate(testNow.Add(4*time.Hour), true).Granted() {
		t.Fatal("won dispute revoked access")
	}
}

func TestContractsAndSourceSwitching(t *testing.T) {
	card, err := NewContract("contract_card", "account_1", "premium.monthly", ChannelCard, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := card.Activate(testNow); err != nil {
		t.Fatal(err)
	}
	if card.State != ContractActive {
		t.Fatal("card contract not active")
	}

	pix, err := NewContract("contract_pix", "account_1", "premium.monthly", ChannelPixAutomatic, "mandate_1")
	if err != nil {
		t.Fatal(err)
	}
	if err := pix.Activate(testNow); !errors.Is(err, ErrBinding) {
		t.Fatalf("inactive mandate activation err=%v", err)
	}
	if err := pix.Mandate.Activate(testNow); err != nil {
		t.Fatal(err)
	}
	if err := pix.Activate(testNow); err != nil {
		t.Fatal(err)
	}
	if pix.State != ContractActive || pix.Mandate.State != MandateActive {
		t.Fatal("pix contract not active")
	}
	if err := pix.Cancel(testNow); err != nil {
		t.Fatal(err)
	}
	if pix.Mandate.State != MandateCanceled {
		t.Fatal("canceled contract left mandate active")
	}
	if err := pix.Activate(testNow); !errors.Is(err, ErrTransition) {
		t.Fatalf("canceled contract reactivated: %v", err)
	}

	expiring, _ := NewContract("contract_exp", "account_2", "premium.monthly", ChannelPixAutomatic, "mandate_2")
	if err := expiring.Mandate.Activate(testNow); err != nil {
		t.Fatal(err)
	}
	if err := expiring.Activate(testNow); err != nil {
		t.Fatal(err)
	}
	if err := expiring.Expire(testNow.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if expiring.Mandate.State != MandateExpired {
		t.Fatal("expired contract left mandate active")
	}

	if _, err := NewContract("bad", "account_1", "premium.monthly", ChannelPixManual, ""); !errors.Is(err, ErrChannelUnsupported) {
		t.Fatalf("manual contract err=%v", err)
	}
	if _, err := NewContract("bad", "account_1", "premium.monthly", ChannelPixAutomatic, ""); !errors.Is(err, ErrInvalid) {
		t.Fatalf("missing mandate err=%v", err)
	}
	if _, err := NewContract("bad", "account_1", "premium.monthly", ChannelCard, "mandate"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("card mandate err=%v", err)
	}

	inconsistent := pix
	inconsistent.State = ContractActive
	inconsistent.Mandate.State = MandateCreated
	if err := inconsistent.Validate(); err == nil {
		t.Fatal("inconsistent mandate state accepted")
	}

	switchCases := []struct {
		name   string
		req    SwitchRequest
		allow  bool
		reason string
	}{
		{"confirmed card to pix", SwitchRequest{OldSource: ChannelCard, NewSource: ChannelPixAutomatic, OldCancellation: CancellationConfirmed, CurrentEnd: testNow.Add(30 * 24 * time.Hour), RequestedAt: testNow}, true, SwitchNextTerm},
		{"unknown card to pix", SwitchRequest{OldSource: ChannelCard, NewSource: ChannelPixAutomatic, OldCancellation: CancellationUnknown, CurrentEnd: testNow.Add(30 * 24 * time.Hour), RequestedAt: testNow}, false, SwitchOldCancellation},
		{"pending card to pix", SwitchRequest{OldSource: ChannelCard, NewSource: ChannelPixAutomatic, OldCancellation: CancellationPending, CurrentEnd: testNow.Add(30 * 24 * time.Hour), RequestedAt: testNow}, false, SwitchOldCancellation},
		{"manual to card", SwitchRequest{OldSource: ChannelPixManual, NewSource: ChannelCard, OldCancellation: CancellationNotApplicable, CurrentEnd: testNow.Add(30 * 24 * time.Hour), RequestedAt: testNow}, true, SwitchNextTerm},
		{"pix to card", SwitchRequest{OldSource: ChannelPixAutomatic, NewSource: ChannelCard, OldCancellation: CancellationConfirmed, CurrentEnd: testNow.Add(30 * 24 * time.Hour), RequestedAt: testNow}, true, SwitchNextTerm},
		{"expired switch immediate", SwitchRequest{OldSource: ChannelCard, NewSource: ChannelPixAutomatic, OldCancellation: CancellationConfirmed, CurrentEnd: testNow.Add(-time.Hour), RequestedAt: testNow}, true, SwitchImmediate},
	}
	for _, tc := range switchCases {
		t.Run(tc.name, func(t *testing.T) {
			decision, err := DecideSwitch(tc.req)
			if err != nil {
				t.Fatal(err)
			}
			if decision.Allowed != tc.allow || decision.Reason != tc.reason {
				t.Fatalf("decision = %+v", decision)
			}
			if tc.allow && decision.FirstDebitAt.IsZero() {
				t.Fatal("missing first debit")
			}
		})
	}
	if _, err := DecideSwitch(SwitchRequest{OldSource: ChannelCard, NewSource: ChannelCard, OldCancellation: CancellationConfirmed, RequestedAt: testNow}); !errors.Is(err, ErrConflict) {
		t.Fatalf("same source err=%v", err)
	}
	if _, err := DecideSwitch(SwitchRequest{OldSource: ChannelPixManual, NewSource: ChannelCard, OldCancellation: CancellationConfirmed, RequestedAt: testNow}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("manual cancellation err=%v", err)
	}

	one, _ := NewContract("contract_a", "account_1", "premium.monthly", ChannelCard, "")
	_ = one.Activate(testNow)
	two, _ := NewContract("contract_b", "account_1", "premium.monthly", ChannelPixAutomatic, "mandate_b")
	_ = two.Mandate.Activate(testNow)
	_ = two.Activate(testNow)
	if err := ValidateFundingSources([]Contract{one, two}); !errors.Is(err, ErrConflict) {
		t.Fatalf("parallel sources err=%v", err)
	}
	two.Cancel(testNow)
	if err := ValidateFundingSources([]Contract{one, two}); err != nil {
		t.Fatal(err)
	}
}

func TestPlayBehaviorAndFreeExitsPreserved(t *testing.T) {
	now := testNow
	snapshot := subdomain.Snapshot{State: subdomain.Active, VerifiedAt: now, ValidUntil: now.Add(time.Hour)}
	status := subdomain.Evaluate(&snapshot, now, true)
	if len(status.Capabilities) != len(PremiumCapabilityList()) {
		t.Fatalf("play capabilities = %v", status.Capabilities)
	}
	if !status.RetainedDataAccess {
		t.Fatal("play data access not retained")
	}
	play := append([]string(nil), status.Capabilities...)
	payment := PremiumCapabilityList()
	sort.Strings(play)
	sort.Strings(payment)
	for i := range play {
		if play[i] != payment[i] {
			t.Fatalf("capability mismatch at %d: %s != %s", i, play[i], payment[i])
		}
	}
	for _, snapshot := range []*subdomain.Snapshot{nil, {State: subdomain.Revoked, VerifiedAt: now, ValidUntil: now.Add(time.Hour)}} {
		if !subdomain.Evaluate(snapshot, now, true).RetainedDataAccess {
			t.Fatal("free exit lost")
		}
	}
}

func TestEntitlementValidateRejectsOverlapAndDuplicateEffects(t *testing.T) {
	base := testGrant(testNow, testNow.Add(time.Hour))
	entitlement := Entitlement{AccountID: "account_1", OfferID: "premium.monthly", PolicyVersion: PolicyVersion, Grants: []Grant{base}}
	if err := entitlement.Validate(); err != nil {
		t.Fatal(err)
	}
	overlap := base
	overlap.ID = "grant_2"
	overlap.OrderID = "order_2"
	overlap.SettlementID = "settlement_2"
	overlap.RemotePaymentID = "remote_2"
	overlap.Start = testNow.Add(30 * time.Minute)
	overlap.End = testNow.Add(2 * time.Hour)
	entitlement.Grants = append(entitlement.Grants, overlap)
	if err := entitlement.Validate(); !errors.Is(err, ErrConflict) {
		t.Fatalf("overlap err=%v", err)
	}
	entitlement.Grants = entitlement.Grants[:1]
	entitlement.Excess = []Settlement{testSettlement(t, testOrder(t, ChannelPixManual), testAttempt(t, testOrder(t, ChannelPixManual)), "settlement_1", "remote_x", testNow)}
	if err := entitlement.Validate(); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate excess err=%v", err)
	}
}

func TestAccessStates(t *testing.T) {
	entitlement := testEntitlement(t)
	entitlement.Grants = []Grant{testGrant(testNow.Add(time.Hour), testNow.Add(2*time.Hour))}
	if got := entitlement.Evaluate(testNow, true); got.State != AccessFuture || got.Granted() {
		t.Fatalf("future = %+v", got)
	}
	if got := entitlement.Evaluate(testNow, false); got.State != AccessNone || got.Reason != ReasonInactive || !got.RetainedDataAccess {
		t.Fatalf("inactive = %+v", got)
	}
	if got := entitlement.Evaluate(time.Time{}, true); got.Reason != ReasonInvalidTime {
		t.Fatalf("invalid time = %+v", got)
	}
	empty := testEntitlement(t)
	if got := empty.Evaluate(testNow, true); got.State != AccessNone || got.Reason != ReasonNoPurchase || !got.RetainedDataAccess {
		t.Fatalf("empty = %+v", got)
	}
}

func TestIdentifiersAndBoundaries(t *testing.T) {
	for _, value := range []string{"", "has space", "line\nbreak", string(make([]byte, MaxText+1))} {
		if validText(value, MaxText) {
			t.Fatalf("invalid text accepted: %q", value)
		}
	}
	if !validText("account_1", MaxText) {
		t.Fatal("valid text refused")
	}
	if err := (ChannelCard).Validate(); err != nil {
		t.Fatal(err)
	}
	if err := (Channel("android_play")).Validate(); !errors.Is(err, ErrChannelUnsupported) {
		t.Fatalf("channel err=%v", err)
	}
	if ChannelCard.Provider() != ProviderStripe || ChannelPixManual.Provider() != ProviderAsaas {
		t.Fatal("provider mapping")
	}
	if ChannelPixManual.Recurring() || !ChannelCard.Recurring() || !ChannelPixAutomatic.Recurring() {
		t.Fatal("recurring mapping")
	}
	if err := (Environment("staging")).Validate(); err == nil {
		t.Fatal("invalid environment accepted")
	}
	for _, offer := range []Offer{validOffer(), enterpriseOffer()} {
		if err := offer.Validate(); err != nil {
			t.Fatalf("offer %s: %v", offer.ID, err)
		}
	}
	badEnterprise := enterpriseOffer()
	badEnterprise.Capabilities = []string{"enterprise.private"}
	if err := badEnterprise.Validate(); err == nil {
		t.Fatal("enterprise capability accepted")
	}
	badEnterprise = enterpriseOffer()
	badEnterprise.SaleEnabled = true
	badEnterprise.Price = BRL(1234)
	if err := badEnterprise.Validate(); !errors.Is(err, ErrSaleDisabled) {
		t.Fatalf("enterprise sale err=%v", err)
	}
}

func TestDuplicateContractSourceAndMandateTransitions(t *testing.T) {
	m, _ := NewContract("contract_1", "account_1", "premium.monthly", ChannelPixAutomatic, "mandate_1")
	if err := m.Mandate.Refuse(testNow); err != nil {
		t.Fatal(err)
	}
	if err := m.Mandate.Activate(testNow); !errors.Is(err, ErrTransition) {
		t.Fatalf("refused mandate activated: %v", err)
	}
	if err := m.Activate(testNow); !errors.Is(err, ErrBinding) {
		t.Fatalf("refused mandate contract activated: %v", err)
	}
	if err := m.Mandate.Cancel(testNow); !errors.Is(err, ErrTransition) {
		t.Fatalf("refused mandate canceled: %v", err)
	}
	if err := m.Expire(testNow); err != nil {
		t.Fatalf("pending contract expire: %v", err)
	}
}

func TestSettlementBindingRejectsMismatchedAuthority(t *testing.T) {
	order := testOrder(t, ChannelPixManual)
	attempt := testAttempt(t, order)
	base := testSettlement(t, order, attempt, "settlement_1", "remote_1", testNow.Add(time.Minute))
	entitlement := testEntitlement(t)

	cases := []struct {
		name   string
		mutate func(*Settlement)
	}{
		{"provider channel", func(s *Settlement) { s.Channel = ChannelCard }},
		{"provider", func(s *Settlement) { s.Provider = ProviderStripe }},
		{"currency", func(s *Settlement) { s.Amount = Money{Centavos: 1234, Currency: "USD"} }},
		{"attempt", func(s *Settlement) { s.AttemptID = "attempt_other" }},
		{"merchant", func(s *Settlement) { s.MerchantID = "" }},
		{"remote", func(s *Settlement) { s.RemotePaymentID = "" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			settlement := base
			tc.mutate(&settlement)
			if _, err := entitlement.ApplySettlement(validOffer(), &order, &attempt, settlement, testNow.Add(2*time.Minute)); !errors.Is(err, ErrBinding) && !errors.Is(err, ErrInvalid) {
				t.Fatalf("err = %v", err)
			}
			if len(entitlement.Grants) != 0 {
				t.Fatal("invalid binding granted")
			}
		})
	}

	wrongAttempt := attempt
	wrongAttempt.Provider = ProviderStripe
	if _, err := entitlement.ApplySettlement(validOffer(), &order, &wrongAttempt, base, testNow.Add(2*time.Minute)); !errors.Is(err, ErrBinding) {
		t.Fatalf("wrong attempt provider err=%v", err)
	}

	wrongAmount := order
	wrongAmount.Amount = BRL(1000)
	if _, err := entitlement.ApplySettlement(validOffer(), &wrongAmount, &attempt, base, testNow.Add(2*time.Minute)); !errors.Is(err, ErrBinding) {
		t.Fatalf("wrong order amount err=%v", err)
	}
}

func TestExcessRemoteReplayIsRejected(t *testing.T) {
	entitlement := testEntitlement(t)
	order := testOrder(t, ChannelPixManual)
	attempt := testAttempt(t, order)
	first := testSettlement(t, order, attempt, "settlement_1", "remote_1", testNow.Add(time.Minute))
	applyGrant(t, &entitlement, &order, attempt, first)

	secondOrder, _ := NewOrder("order_2", "account_1", testQuote(t, ChannelPixManual), testNow)
	_ = secondOrder.MarkPending(testNow)
	secondAttempt, _ := NewAttempt("attempt_2", secondOrder.ID, ProviderAsaas, "create_pix")
	_ = secondAttempt.Dispatch(testNow)
	_ = secondAttempt.Succeed("remote_2", testNow)
	second := testSettlement(t, secondOrder, secondAttempt, "settlement_2", "remote_2", testNow.Add(2*time.Hour))
	if result, err := entitlement.ApplySettlement(validOffer(), &secondOrder, &secondAttempt, second, testNow.Add(3*time.Hour)); err != nil || result.Grant == nil {
		t.Fatalf("second result=%+v err=%v", result, err)
	}

	excessOrder, _ := NewOrder("order_3", "account_1", testQuote(t, ChannelPixManual), testNow)
	_ = excessOrder.MarkPending(testNow)
	excessAttempt, _ := NewAttempt("attempt_3", excessOrder.ID, ProviderAsaas, "create_pix")
	_ = excessAttempt.Dispatch(testNow)
	_ = excessAttempt.Succeed("remote_3", testNow)
	excess := testSettlement(t, excessOrder, excessAttempt, "settlement_3", "remote_3", testNow.Add(4*time.Hour))
	if result, err := entitlement.ApplySettlement(validOffer(), &excessOrder, &excessAttempt, excess, testNow.Add(5*time.Hour)); err != nil || !result.Excess {
		t.Fatalf("excess result=%+v err=%v", result, err)
	}

	replayOrder, _ := NewOrder("order_4", "account_1", testQuote(t, ChannelPixManual), testNow)
	_ = replayOrder.MarkPending(testNow)
	replayAttempt, _ := NewAttempt("attempt_4", replayOrder.ID, ProviderAsaas, "create_pix")
	_ = replayAttempt.Dispatch(testNow)
	_ = replayAttempt.Succeed("remote_3", testNow)
	replay := testSettlement(t, replayOrder, replayAttempt, "settlement_4", "remote_3", testNow.Add(6*time.Hour))
	if _, err := entitlement.ApplySettlement(validOffer(), &replayOrder, &replayAttempt, replay, testNow.Add(7*time.Hour)); !errors.Is(err, ErrConflict) {
		t.Fatalf("remote replay err=%v", err)
	}
	if len(entitlement.Excess) != 1 {
		t.Fatalf("excess = %d", len(entitlement.Excess))
	}
}

func TestDecidePlanChangeNextTermOnly(t *testing.T) {
	current := validOffer()
	target := current
	target.ID = "premium.monthly.v2"
	target.Price = BRL(1490)
	target.Capabilities = append([]string(nil), current.Capabilities...)
	decision, err := DecidePlanChange(PlanChangeRequest{
		Current: current, Target: target,
		CurrentEnd: testNow.Add(10 * 24 * time.Hour), RequestedAt: testNow,
	})
	if err != nil || !decision.Allowed || decision.Reason != ChangeNextTerm ||
		!decision.EffectiveAt.Equal(testNow.Add(10*24*time.Hour)) {
		t.Fatalf("decision=%+v err=%v", decision, err)
	}

	decision, err = DecidePlanChange(PlanChangeRequest{
		Current: current, Target: target,
		CurrentEnd: testNow.Add(-time.Hour), RequestedAt: testNow,
	})
	if err != nil || !decision.Allowed || decision.Reason != ChangeImmediate ||
		!decision.EffectiveAt.Equal(testNow) {
		t.Fatalf("expired decision=%+v err=%v", decision, err)
	}
}

func TestDecidePlanChangeRefusals(t *testing.T) {
	current := validOffer()
	higher := current
	higher.ID = "premium.monthly.v2"
	higher.Price = BRL(1490)

	cases := []struct {
		name    string
		request PlanChangeRequest
		reason  string
		wantErr error
	}{
		{"proration", PlanChangeRequest{Current: current, Target: higher, CurrentEnd: testNow.Add(time.Hour), RequestedAt: testNow, Proration: true}, ChangeUnsupportedProration, nil},
		{"same", PlanChangeRequest{Current: current, Target: current, CurrentEnd: testNow.Add(time.Hour), RequestedAt: testNow}, ChangeNoop, nil},
		{"family", PlanChangeRequest{Current: current, Target: Offer{ID: "family.v2", Family: "other", Term: TermMonthly, Price: BRL(1490), Capabilities: PremiumCapabilityList(), Channels: map[Channel]bool{ChannelCard: true}, SaleEnabled: true}, CurrentEnd: testNow.Add(time.Hour), RequestedAt: testNow}, ChangeUnsupportedFamily, nil},
		{"disabled", PlanChangeRequest{Current: current, Target: Offer{ID: "premium.off", Family: "premium", Term: TermMonthly, Price: BRL(1490), Capabilities: PremiumCapabilityList(), Channels: map[Channel]bool{ChannelCard: true}, SaleEnabled: false}, CurrentEnd: testNow.Add(time.Hour), RequestedAt: testNow}, ChangeUnsupportedOffer, nil},
		{"annual", PlanChangeRequest{Current: current, Target: Offer{ID: "premium.annual", Family: "premium", Term: Term("P1Y"), Price: BRL(1490), Capabilities: PremiumCapabilityList(), Channels: map[Channel]bool{ChannelCard: true}, SaleEnabled: true}, CurrentEnd: testNow.Add(time.Hour), RequestedAt: testNow}, "", ErrTermUnsupported},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			decision, err := DecidePlanChange(tc.request)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err=%v", err)
				}
				return
			}
			if err != nil || decision.Allowed || decision.Reason != tc.reason {
				t.Fatalf("decision=%+v err=%v", decision, err)
			}
		})
	}
	if _, err := DecidePlanChange(PlanChangeRequest{Current: current, Target: higher}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("zero time err=%v", err)
	}
}

func approvedEnterpriseOffer() Offer {
	return Offer{
		ID: "enterprise.regional_brief.approved", Family: "enterprise", Term: TermMonthly,
		Price: BRL(9900), Capabilities: []string{},
		Channels:    map[Channel]bool{ChannelCard: true, ChannelPixManual: true, ChannelPixAutomatic: true},
		SaleEnabled: true,
		Enterprise:  true,
	}
}

func TestDecideEnterpriseBindingApprovesExactSelfPurchase(t *testing.T) {
	decision, err := DecideEnterpriseBinding(EnterpriseBindingRequest{
		Offer: approvedEnterpriseOffer(), AccountID: "11111111-1111-4111-8111-111111111111",
		PayerID: "11111111-1111-4111-8111-111111111111", Quantity: 1, Approved: true,
	})
	if err != nil || !decision.Allowed || decision.Reason != EnterpriseBound {
		t.Fatalf("decision=%+v err=%v", decision, err)
	}
}

func TestDecideEnterpriseBindingRefusals(t *testing.T) {
	approved := approvedEnterpriseOffer()
	account := "11111111-1111-4111-8111-111111111111"
	other := "22222222-2222-4222-8222-222222222222"
	withCaps := approved
	withCaps.Capabilities = []string{"cloud.sync"}
	wrongFamily := approved
	wrongFamily.Family = "premium"
	premiumOffer := validOffer()
	cases := []struct {
		name    string
		request EnterpriseBindingRequest
		reason  string
		wantErr error
	}{
		{"foreign-payer", EnterpriseBindingRequest{Offer: approved, AccountID: account, PayerID: other, Quantity: 1, Approved: true}, EnterpriseForeignPayer, nil},
		{"entity-substitution", EnterpriseBindingRequest{Offer: approved, AccountID: other, PayerID: other, Quantity: 1, Approved: true}, "", nil},
		{"zero-quantity", EnterpriseBindingRequest{Offer: approved, AccountID: account, PayerID: account, Quantity: 0, Approved: true}, EnterpriseQuantity, nil},
		{"seat-override", EnterpriseBindingRequest{Offer: approved, AccountID: account, PayerID: account, Quantity: 5, Approved: true}, EnterpriseQuantity, nil},
		{"unapproved", EnterpriseBindingRequest{Offer: enterpriseOffer(), AccountID: account, PayerID: account, Quantity: 1, Approved: false}, EnterpriseUnavailable, nil},
		{"unapproved-approved-flag", EnterpriseBindingRequest{Offer: approved, AccountID: account, PayerID: account, Quantity: 1, Approved: false}, EnterpriseUnavailable, nil},
		{"wrong-family", EnterpriseBindingRequest{Offer: wrongFamily, AccountID: account, PayerID: account, Quantity: 1, Approved: true}, EnterpriseMismatch, nil},
		{"premium-offer", EnterpriseBindingRequest{Offer: premiumOffer, AccountID: account, PayerID: account, Quantity: 1, Approved: true}, EnterpriseMismatch, nil},
		{"capabilities-leak", EnterpriseBindingRequest{Offer: withCaps, AccountID: account, PayerID: account, Quantity: 1, Approved: true}, "", ErrInvalid},
		{"bad-identity", EnterpriseBindingRequest{Offer: approved, AccountID: "", PayerID: account, Quantity: 1, Approved: true}, "", ErrInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			decision, err := DecideEnterpriseBinding(tc.request)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err=%v want %v", err, tc.wantErr)
				}
				return
			}
			if tc.reason == "" {
				// entity-substitution is equivalent to a self purchase of another
				// account: allowed only when payer and billed entity coincide.
				if err != nil || !decision.Allowed {
					t.Fatalf("decision=%+v err=%v", decision, err)
				}
				return
			}
			if err != nil || decision.Allowed || decision.Reason != tc.reason {
				t.Fatalf("decision=%+v err=%v", decision, err)
			}
		})
	}
}

func TestEnterpriseOfferNeverCarriesAppCapabilities(t *testing.T) {
	for _, capability := range PremiumCapabilityList() {
		candidate := approvedEnterpriseOffer()
		candidate.Capabilities = []string{capability}
		if _, err := DecideEnterpriseBinding(EnterpriseBindingRequest{
			Offer: candidate, AccountID: "11111111-1111-4111-8111-111111111111",
			PayerID: "11111111-1111-4111-8111-111111111111", Quantity: 1, Approved: true,
		}); err == nil {
			t.Fatalf("premium capability %q accepted on enterprise offer", capability)
		}
	}
	if got := approvedEnterpriseOffer(); len(got.Capabilities) != 0 {
		t.Fatal("approved enterprise fixture must carry no app capabilities")
	}
}
