package adapters

import (
	"context"
	"errors"
	"testing"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/domain"
)

type muxVerifier struct {
	calls int
	err   error
}

func (m *muxVerifier) VerifySettlement(_ context.Context, event app.InboxRecord) (app.SettlementCandidate, error) {
	m.calls++
	if m.err != nil {
		return app.SettlementCandidate{}, m.err
	}
	return app.SettlementCandidate{RemotePaymentID: "pay_mux"}, nil
}

func TestPAY19SettlementMuxFailsClosedPerProvider(t *testing.T) {
	stripe, asaas := &muxVerifier{}, &muxVerifier{}
	mux := SettlementMux{Stripe: stripe, Asaas: asaas}
	event := func(provider domain.Provider) app.InboxRecord {
		return app.InboxRecord{InboxEvent: app.InboxEvent{Provider: provider}}
	}
	if _, err := mux.Verify(context.Background(), event(domain.ProviderStripe)); err != nil || stripe.calls != 1 || asaas.calls != 0 {
		t.Fatalf("stripe calls=%d asaas calls=%d err=%v", stripe.calls, asaas.calls, err)
	}
	if _, err := mux.Verify(context.Background(), event(domain.ProviderAsaas)); err != nil || asaas.calls != 1 || stripe.calls != 1 {
		t.Fatalf("stripe calls=%d asaas calls=%d err=%v", stripe.calls, asaas.calls, err)
	}
	bare := SettlementMux{}
	if _, err := bare.Verify(context.Background(), event(domain.ProviderStripe)); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatalf("unconfigured stripe err=%v", err)
	}
	if _, err := bare.Verify(context.Background(), event(domain.ProviderAsaas)); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatalf("unconfigured asaas err=%v", err)
	}
	if _, err := mux.Verify(context.Background(), event("")); !errors.Is(err, app.ErrIgnored) {
		t.Fatalf("impossible provider err=%v", err)
	}
}
