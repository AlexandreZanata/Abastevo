package jobs

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/domain"
	platform "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
)

type fakeInboxStore struct {
	mu         sync.Mutex
	pending    []app.InboxRecord
	marked     []string
	markErr    error
	pendingErr error
}

func (f *fakeInboxStore) PendingInbox(_ context.Context, provider domain.Provider, _ domain.Environment, _ string, _ int) ([]app.InboxRecord, error) {
	if f.pendingErr != nil {
		return nil, f.pendingErr
	}
	var out []app.InboxRecord
	for _, record := range f.pending {
		if record.Provider == provider {
			out = append(out, record)
		}
	}
	return out, nil
}

func (f *fakeInboxStore) MarkInbox(_ context.Context, event app.InboxEvent, state, _ string) error {
	if f.markErr != nil {
		return f.markErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.marked = append(f.marked, event.EventID+":"+state)
	return nil
}

type fakeProcessor struct {
	mu    sync.Mutex
	calls []string
	err   error
	after func(app.InboxRecord) error
}

func (f *fakeProcessor) Process(_ context.Context, event app.InboxRecord) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, event.EventID)
	if f.after != nil {
		return f.after(event)
	}
	return f.err
}

func pay19Event(provider domain.Provider, eventID, eventType string) app.InboxRecord {
	return app.InboxRecord{
		InboxEvent: app.InboxEvent{
			Provider: provider, Environment: domain.EnvironmentTest,
			Merchant: "merchant-test", EventID: eventID,
			PayloadHash: strings.Repeat("c", 64), EventType: eventType,
			ResourceType: "payment", ResourceID: "res-1",
		},
		State: "PENDING",
	}
}

func pay19Router(store *fakeInboxStore) (*Router, *fakeProcessor, *fakeProcessor, *fakeProcessor, *fakeProcessor) {
	settlement, lifecycle, dispute, cycles := &fakeProcessor{}, &fakeProcessor{}, &fakeProcessor{}, &fakeProcessor{}
	router := &Router{
		Store: store, Settlement: settlement, Lifecycle: lifecycle,
		Dispute: dispute, Cycles: cycles,
		Channels: []Channel{
			{Provider: domain.ProviderStripe, Environment: domain.EnvironmentTest, Merchant: "merchant-test"},
			{Provider: domain.ProviderAsaas, Environment: domain.EnvironmentTest, Merchant: "merchant-test"},
		},
	}
	return router, settlement, lifecycle, dispute, cycles
}

func TestPAY19RouterOwnsEveryKnownEventType(t *testing.T) {
	cases := map[string]struct {
		provider domain.Provider
		want     string
	}{
		"stripe-paid":         {domain.ProviderStripe, "settlement"},
		"stripe-subscription": {domain.ProviderStripe, "lifecycle"},
		"stripe-failed":       {domain.ProviderStripe, "lifecycle"},
		"stripe-checkout":     {domain.ProviderStripe, "lifecycle"},
		"stripe-dispute":      {domain.ProviderStripe, "dispute"},
		"asaas-received":      {domain.ProviderAsaas, "settlement"},
		"asaas-confirmed":     {domain.ProviderAsaas, "settlement"},
		"asaas-chargeback":    {domain.ProviderAsaas, "dispute"},
		"asaas-authorization": {domain.ProviderAsaas, "cycles"},
		"asaas-instruction":   {domain.ProviderAsaas, "cycles"},
		"asaas-eligibility":   {domain.ProviderAsaas, "cycles"},
	}
	types := map[string]string{
		"stripe-paid": "invoice.paid", "stripe-subscription": "customer.subscription.updated",
		"stripe-failed": "invoice.payment_failed", "stripe-checkout": "checkout.session.completed",
		"stripe-dispute": "charge.dispute.created", "asaas-received": "PAYMENT_RECEIVED",
		"asaas-confirmed": "PAYMENT_CONFIRMED", "asaas-chargeback": "PAYMENT_CHARGEBACK_REQUESTED",
		"asaas-authorization": "PIX_AUTOMATIC_RECURRING_AUTHORIZATION_ACTIVATED",
		"asaas-instruction":   "PIX_AUTOMATIC_RECURRING_PAYMENT_INSTRUCTION_SCHEDULED",
		"asaas-eligibility":   "PIX_AUTOMATIC_RECURRING_ELIGIBILITY_UPDATED",
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			store := &fakeInboxStore{pending: []app.InboxRecord{pay19Event(tc.provider, "evt-"+name, types[name])}}
			router, settlement, lifecycle, dispute, cycles := pay19Router(store)
			count, err := router.Drain(context.Background())
			if err != nil || count != 1 {
				t.Fatalf("count=%d err=%v", count, err)
			}
			owners := map[string]*fakeProcessor{
				"settlement": settlement, "lifecycle": lifecycle,
				"dispute": dispute, "cycles": cycles,
			}
			for owner, processor := range owners {
				want := 0
				if owner == tc.want {
					want = 1
				}
				if len(processor.calls) != want {
					t.Fatalf("owner=%s calls=%d want %d", owner, len(processor.calls), want)
				}
			}
			if len(store.marked) != 0 {
				t.Fatalf("owned event marked: %v", store.marked)
			}
		})
	}
}

func TestPAY19RouterKeepsUnknownEventsWithoutEffect(t *testing.T) {
	store := &fakeInboxStore{pending: []app.InboxRecord{
		pay19Event(domain.ProviderStripe, "evt-unknown-stripe", "synthetic.unknown"),
		pay19Event(domain.ProviderAsaas, "evt-unknown-asaas", "SYNTHETIC_UNKNOWN"),
	}}
	router, settlement, lifecycle, dispute, cycles := pay19Router(store)
	count, err := router.Drain(context.Background())
	if err != nil || count != 2 {
		t.Fatalf("count=%d err=%v", count, err)
	}
	for _, processor := range []*fakeProcessor{settlement, lifecycle, dispute, cycles} {
		if len(processor.calls) != 0 {
			t.Fatalf("unknown event reached a processor: %v", processor.calls)
		}
	}
	if len(store.marked) != 2 {
		t.Fatalf("marked=%v", store.marked)
	}
	for _, mark := range store.marked {
		if !strings.HasSuffix(mark, ":PROCESSED") {
			t.Fatalf("mark=%q", mark)
		}
	}
}

func TestPAY19RouterConvergesUnmarkedIgnores(t *testing.T) {
	store := &fakeInboxStore{pending: []app.InboxRecord{
		pay19Event(domain.ProviderAsaas, "evt-ignore", "PIX_AUTOMATIC_RECURRING_AUTHORIZATION_ACTIVATED"),
	}}
	router, _, _, _, cycles := pay19Router(store)
	cycles.err = app.ErrIgnored
	count, err := router.Drain(context.Background())
	if err != nil || count != 1 {
		t.Fatalf("count=%d err=%v", count, err)
	}
	if len(store.marked) != 1 || store.marked[0] != "evt-ignore:PROCESSED" {
		t.Fatalf("marked=%v", store.marked)
	}
}

func TestPAY19RouterPreservesPendingOnOutage(t *testing.T) {
	store := &fakeInboxStore{pending: []app.InboxRecord{
		pay19Event(domain.ProviderAsaas, "evt-outage", "PAYMENT_RECEIVED"),
	}}
	router, settlement, _, _, _ := pay19Router(store)
	settlement.err = domain.ErrUnavailable
	count, err := router.Drain(context.Background())
	if !errors.Is(err, domain.ErrUnavailable) || count != 0 {
		t.Fatalf("count=%d err=%v", count, err)
	}
	if len(store.marked) != 0 {
		t.Fatalf("outage marked: %v", store.marked)
	}
}

func TestPAY19HandlerValidatesEnvelope(t *testing.T) {
	handler := Handler{Router: &Router{}}
	for _, raw := range []string{
		`{"version":2,"event_id":"evt"}`,
		`{"version":1}`,
		`{"version":1,"event_id":""}`,
		`{"version":1,"event_id":"` + strings.Repeat("e", 256) + `"}`,
		`{"version":1,"event_id":"evt","token":"forbidden"}`,
		`not-json`,
	} {
		if err := handler.Handle(context.Background(), platform.Job{Payload: []byte(raw)}); err == nil {
			t.Fatalf("invalid envelope accepted: %s", raw)
		}
	}
	if handler.Kind() != KindPaymentInbox || handler.Version() != 1 {
		t.Fatal("handler identity changed")
	}
	if err := handler.Handle(context.Background(), platform.Job{Payload: []byte(`{"version":1,"event_id":"evt-wakeup"}`)}); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatalf("nil store err=%v", err)
	}
}
