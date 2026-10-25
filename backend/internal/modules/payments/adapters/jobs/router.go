// Package jobs adapts durable payment inbox work to application services.
// The payment-inbox job is a wake-up: the router drains bounded pending
// inbox rows per configured channel and routes each event to exactly one
// service by provider and type, because all services share one inbox state
// and a second processor would only see an already-consumed event.
// Unknown authenticated events are retained minimally with no financial
// effect; anything a configured service cannot evaluate stays pending.
package jobs

import (
	"context"
	"errors"
	"strings"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
	platform "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
)

const (
	// KindPaymentInbox is the durable job kind enqueued by inbox intake.
	KindPaymentInbox = "payment-inbox"
	// drainLimit bounds one wake-up across all channels.
	drainLimit = 100
	// maxEventID bounds the informational job payload reference.
	maxEventID = 255
)

// Channel binds one provider inbox stream drained by the router.
type Channel struct {
	Provider    domain.Provider
	Environment domain.Environment
	Merchant    string
}

// EventProcessor consumes one pending inbox event exactly like the
// settlement, lifecycle, dispute and Pix Automático cycle services do.
type EventProcessor interface {
	Process(context.Context, app.InboxRecord) error
}

// InboxStore is the router's narrow inbox port: bounded pending listing
// plus terminal marking. Services own every other store effect.
type InboxStore interface {
	PendingInbox(context.Context, domain.Provider, domain.Environment, string, int) ([]app.InboxRecord, error)
	MarkInbox(context.Context, app.InboxEvent, string, string) error
}

// Router drains pending payment inbox rows to exactly one service each.
type Router struct {
	Store      InboxStore
	Settlement EventProcessor
	Lifecycle  EventProcessor
	Dispute    EventProcessor
	Cycles     EventProcessor
	Channels   []Channel
	Limit      int
}

func (r Router) limit() int {
	if r.Limit < 1 || r.Limit > drainLimit {
		return drainLimit
	}
	return r.Limit
}

// Drain processes bounded pending inbox rows per configured channel and
// reports how many reached a terminal owner. Unavailable or ambiguous
// dependencies abort with the count so far and the events stay pending;
// conflicts are already marked by their service before the error returns.
func (r Router) Drain(ctx context.Context) (int, error) {
	if r.Store == nil {
		return 0, domain.ErrUnavailable
	}
	processed := 0
	for _, channel := range r.Channels {
		events, err := r.Store.PendingInbox(ctx, channel.Provider, channel.Environment, channel.Merchant, r.limit())
		if err != nil {
			return processed, err
		}
		for _, event := range events {
			if event.State != "" && event.State != "PENDING" {
				continue
			}
			if err := r.route(ctx, event); err != nil {
				return processed, err
			}
			processed++
		}
	}
	return processed, nil
}

func (r Router) route(ctx context.Context, event app.InboxRecord) error {
	processor, ok := r.owner(event)
	if !ok {
		// Unknown authenticated event: retained minimally, no effect.
		return r.Store.MarkInbox(ctx, event.InboxEvent, "PROCESSED", "")
	}
	if processor == nil {
		return domain.ErrUnavailable
	}
	if err := processor.Process(ctx, event); err != nil {
		if errors.Is(err, app.ErrIgnored) {
			// Services that leave ignored events unmarked (cycles) still
			// converge here with no effect.
			return r.Store.MarkInbox(ctx, event.InboxEvent, "PROCESSED", "")
		}
		return err
	}
	return nil
}

// owner selects the single service for an event. The table mirrors the
// verifier matchers: settlement owns paid events, lifecycle owns
// subscription/checkout/failure events, dispute owns chargeback events and
// the cycle service owns Pix Automático mandate events.
func (r Router) owner(event app.InboxRecord) (EventProcessor, bool) {
	switch event.Provider {
	case domain.ProviderStripe:
		switch event.EventType {
		case "invoice.paid":
			return r.Settlement, true
		case "customer.subscription.updated", "customer.subscription.deleted",
			"invoice.payment_failed", "invoice.payment_action_required",
			"checkout.session.completed":
			return r.Lifecycle, true
		case "charge.dispute.created", "charge.dispute.updated", "charge.dispute.closed":
			return r.Dispute, true
		default:
			return nil, false
		}
	case domain.ProviderAsaas:
		switch {
		case event.EventType == "PAYMENT_CHARGEBACK_REQUESTED" ||
			event.EventType == "PAYMENT_CHARGEBACK_DISPUTE" ||
			event.EventType == "PAYMENT_AWAITING_CHARGEBACK_REVERSAL":
			return r.Dispute, true
		case event.EventType == "PAYMENT_RECEIVED" || strings.HasPrefix(event.EventType, "PAYMENT_"):
			return r.Settlement, true
		case strings.HasPrefix(event.EventType, "PIX_AUTOMATIC_RECURRING_"):
			return r.Cycles, true
		default:
			return nil, false
		}
	default:
		return nil, false
	}
}

// Handler executes one payment-inbox wake-up through the router. The payload
// carries only a versioned informational event reference; the bounded drain
// covers every pending row including missed or reordered webhooks.
type Handler struct {
	Router *Router
}

func (h Handler) Kind() string { return KindPaymentInbox }
func (h Handler) Version() int { return 1 }
func (h Handler) Handle(ctx context.Context, job platform.Job) error {
	var payload struct {
		Version int    `json:"version"`
		EventID string `json:"event_id"`
	}
	if httpapi.DecodeJSON(job.Payload, &payload) != nil || payload.Version != 1 ||
		payload.EventID == "" || len(payload.EventID) > maxEventID {
		return domain.ErrInvalid
	}
	if h.Router == nil {
		return domain.ErrUnavailable
	}
	_, err := h.Router.Drain(ctx)
	return err
}
