package application_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	payadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/adapters"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/domain"
)

type reviewAuthTransport struct{ status int }

func (r reviewAuthTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: r.status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"error":{"type":"invalid_request_error","message":"synthetic provider credential failure for cus_review_sentinel"}}`)), Request: req}, nil
}

type reviewSettlementStore struct {
	app.SettlementStore
	marked []string
}

func (s *reviewSettlementStore) MarkInbox(_ context.Context, _ app.InboxEvent, state, _ string) error {
	s.marked = append(s.marked, state)
	return nil
}

type reviewLifecycleStore struct {
	app.LifecycleStore
	marked []string
}

func (s *reviewLifecycleStore) MarkInbox(_ context.Context, _ app.InboxEvent, state, _ string) error {
	s.marked = append(s.marked, state)
	return nil
}

func TestPAY22ReviewProviderAuthFailurePreservesPending(t *testing.T) {
	for _, status := range []int{401, 403} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			provider := payadapters.NewStripeProvider("sk_test_synthetic", domain.EnvironmentTest, &http.Client{Transport: reviewAuthTransport{status: status}})
			provider.Merchant = "acct_synthetic"
			t.Run("settlement", func(t *testing.T) {
				store := &reviewSettlementStore{}
				mux := payadapters.SettlementMux{}
				mux.Stripe = provider
				service := &app.SettlementService{Store: store, Verifier: mux}
				event := app.InboxRecord{InboxEvent: app.InboxEvent{Provider: domain.ProviderStripe, Environment: domain.EnvironmentTest, Merchant: "acct_synthetic", EventID: "evt_synthetic", EventType: "invoice.paid", ResourceType: "invoice", ResourceID: "in_synthetic"}, State: "PENDING"}
				err := service.Process(context.Background(), event)
				if len(store.marked) != 0 {
					t.Fatalf("provider HTTP %d must preserve PENDING; marked=%v err=%v", status, store.marked, err)
				}
				if !errors.Is(err, app.ErrProviderUnauthorized) {
					t.Fatalf("provider HTTP %d must stay recoverable, err=%v", status, err)
				}
			})
			t.Run("lifecycle", func(t *testing.T) {
				store := &reviewLifecycleStore{}
				service := &app.LifecycleService{Store: store, Verifier: provider}
				event := app.InboxRecord{InboxEvent: app.InboxEvent{Provider: domain.ProviderStripe, Environment: domain.EnvironmentTest, Merchant: "acct_synthetic", EventID: "evt_synthetic", EventType: "customer.subscription.updated", ResourceType: "subscription", ResourceID: "sub_synthetic"}, State: "PENDING"}
				err := service.Process(context.Background(), event)
				if len(store.marked) != 0 {
					t.Fatalf("provider HTTP %d must preserve PENDING; marked=%v err=%v", status, store.marked, err)
				}
				if !errors.Is(err, app.ErrProviderUnauthorized) {
					t.Fatalf("provider HTTP %d must stay recoverable, err=%v", status, err)
				}
			})
		})
	}
}
