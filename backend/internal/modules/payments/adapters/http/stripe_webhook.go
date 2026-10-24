// Package http exposes private payment HTTP adapters.
package http

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"mime"
	"net/http"
	"strings"
	"time"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
	"github.com/go-chi/chi/v5"
	"github.com/stripe/stripe-go/v87"
	"github.com/stripe/stripe-go/v87/webhook"
)

const (
	defaultStripeWebhookBody = 1 << 20
	defaultStripeTolerance   = 300 * time.Second
	maxStripeSignatureHeader = 4096
)

type StripeWebhookHandler struct {
	Inbox            app.InboxRecorder
	Secret           string
	Environment      domain.Environment
	Merchant         string
	ExpectedLivemode bool
	Tolerance        time.Duration
	MaxBodyBytes     int64
}

func (h StripeWebhookHandler) RegisterRoutes(r chi.Router) {
	r.Post("/v1/payments/stripe/webhook", h.handle)
}

func (h StripeWebhookHandler) handle(w http.ResponseWriter, r *http.Request) {
	if h.Inbox == nil || h.Secret == "" || !strings.HasPrefix(h.Merchant, "acct_") ||
		(h.Environment != domain.EnvironmentTest && h.Environment != domain.EnvironmentProduction) ||
		(h.Environment == domain.EnvironmentProduction) != h.ExpectedLivemode {
		webhookError(w, r, http.StatusServiceUnavailable, "payments.stripe-webhook-unavailable")
		return
	}
	typ, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || typ != "application/json" {
		webhookError(w, r, http.StatusBadRequest, "payments.stripe-content-type")
		return
	}
	body, err := httpapi.ReadBody(r, h.maxBody())
	if errors.Is(err, httpapi.ErrBodyTooLarge) {
		webhookError(w, r, http.StatusRequestEntityTooLarge, "payments.stripe-body-too-large")
		return
	}
	if err != nil {
		webhookError(w, r, http.StatusBadRequest, "payments.stripe-body")
		return
	}
	signature := strings.TrimSpace(r.Header.Get("Stripe-Signature"))
	if signature == "" || len(signature) > maxStripeSignatureHeader {
		webhookError(w, r, http.StatusBadRequest, "payments.stripe-signature")
		return
	}
	event, err := webhook.ConstructEventWithOptions(body, signature, h.Secret, webhook.ConstructEventOptions{
		Tolerance: h.tolerance(),
	})
	if err != nil {
		webhookError(w, r, http.StatusBadRequest, "payments.stripe-signature")
		return
	}
	if event.ID == "" || event.Type == "" || event.Livemode != h.ExpectedLivemode ||
		event.Account != "" || event.APIVersion != stripe.APIVersion {
		webhookError(w, r, http.StatusBadRequest, "payments.stripe-binding")
		return
	}
	resourceType, resourceID := eventResource(event)
	digest := sha256.Sum256(body)
	_, err = h.Inbox.RecordInbox(r.Context(), app.InboxEvent{
		Provider: domain.ProviderStripe, Environment: h.Environment, Merchant: h.Merchant,
		EventID: event.ID, PayloadHash: hex.EncodeToString(digest[:]),
		EventType: string(event.Type), ResourceType: resourceType, ResourceID: resourceID,
	})
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrConflict):
			webhookError(w, r, http.StatusConflict, "payments.stripe-duplicate-content")
		case errors.Is(err, domain.ErrInvalid):
			webhookError(w, r, http.StatusBadRequest, "payments.stripe-event-invalid")
		default:
			webhookError(w, r, http.StatusServiceUnavailable, "payments.stripe-inbox-unavailable")
		}
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
}

func (h StripeWebhookHandler) maxBody() int64 {
	if h.MaxBodyBytes > 0 {
		return h.MaxBodyBytes
	}
	return defaultStripeWebhookBody
}

func (h StripeWebhookHandler) tolerance() time.Duration {
	if h.Tolerance > 0 {
		return h.Tolerance
	}
	return defaultStripeTolerance
}

func eventResource(event stripe.Event) (string, string) {
	resourceType, resourceID := "event", event.ID
	if event.Data != nil && event.Data.Object != nil {
		if value, ok := event.Data.Object["object"].(string); ok && value != "" && len(value) <= 64 {
			resourceType = value
		}
		if value, ok := event.Data.Object["id"].(string); ok && value != "" && len(value) <= 255 {
			resourceID = value
		}
	}
	return resourceType, resourceID
}

func webhookError(w http.ResponseWriter, r *http.Request, status int, code string) {
	httpapi.WriteError(w, r, status, code, "Stripe webhook refused", nil)
}
