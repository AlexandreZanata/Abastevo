package adapters

import (
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/domain"
)

func TestCompanyStripeRefundUsesCanonicalChargeAndExactAmount(t *testing.T) {
	for _, tc := range []struct {
		name, currency string
		refunded       int
		full, want     bool
	}{
		{name: "partial", currency: "brl", refunded: 100},
		{name: "total", currency: "brl", refunded: 1723, full: true, want: true},
		{name: "foreign currency", currency: "usd", refunded: 1723, full: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			event := app.InboxRecord{InboxEvent: app.InboxEvent{Provider: domain.ProviderStripe,
				Environment: domain.EnvironmentTest, Merchant: "acct_synthetic", EventType: "charge.refunded",
				ResourceID: "ch_synthetic", ResourceType: "charge"}}
			provider := stripeTestProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v1/charges/ch_synthetic" || r.Method != http.MethodGet {
					t.Error("expected a read of the canonical charge")
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"id": "ch_synthetic", "paid": true, "captured": true,
					"currency": tc.currency, "amount_captured": 1723, "amount_refunded": tc.refunded, "refunded": tc.full})
			}))
			provider.Merchant = event.Merchant
			got, err := provider.VerifyCompanyRefund(t.Context(), event)
			if tc.currency != "brl" {
				if !errors.Is(err, domain.ErrBinding) {
					t.Fatalf("foreign currency was not refused: %v", err)
				}
			} else if err != nil || got.Full != tc.want || !got.Amount.Equal(domain.BRL(1723)) {
				t.Fatalf("unexpected canonical refund evidence: %+v / %v", got, err)
			}
		})
	}
}
