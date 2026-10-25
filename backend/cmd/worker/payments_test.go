package main

import (
	"context"
	"errors"
	"testing"
	"time"

	paydomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/config"
	platform "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
)

func TestPAY19WorkerInboxIsolatedAndFailClosed(t *testing.T) {
	handler := composePaymentInbox(config.PaymentsConfig{}, nil, time.Now)
	if handler.Kind() != "payment-inbox" || handler.Version() != 1 {
		t.Fatalf("handler=%s v%d", handler.Kind(), handler.Version())
	}
	// No configured channels: the wake-up drains nothing and never touches
	// the database.
	if err := handler.Handle(context.Background(), platform.Job{
		Payload: []byte(`{"version":1,"event_id":"evt-synthetic"}`),
	}); err != nil {
		t.Fatalf("empty drain err=%v", err)
	}
	for _, raw := range []string{
		`{"version":2,"event_id":"evt"}`,
		`{"version":1}`,
		`{"version":1,"event_id":""}`,
	} {
		if err := handler.Handle(context.Background(), platform.Job{Payload: []byte(raw)}); err == nil {
			t.Fatalf("invalid envelope accepted: %s", raw)
		}
	}
	if err := handler.Handle(context.Background(), platform.Job{}); !errors.Is(err, paydomain.ErrInvalid) {
		t.Fatalf("empty payload err=%v", err)
	}
}
