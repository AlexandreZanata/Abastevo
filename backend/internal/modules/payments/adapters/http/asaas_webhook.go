package http

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"mime"
	"net/http"
	"strings"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
	"github.com/go-chi/chi/v5"
)

const (
	defaultAsaasWebhookBody = 1 << 20
	maxAsaasTokenLength     = 255
)

type AsaasWebhookHandler struct {
	Inbox                app.InboxRecorder
	WebhookToken         string
	PreviousWebhookToken string
	APIKey               string
	Environment          domain.Environment
	Merchant             string
	RequireTLS           bool
	MaxBodyBytes         int64
}

func (h AsaasWebhookHandler) RegisterRoutes(r chi.Router) {
	r.Post("/v1/payments/asaas/webhook", h.handle)
}

func (h AsaasWebhookHandler) handle(w http.ResponseWriter, r *http.Request) {
	if !h.configured() {
		asaasWebhookError(w, r, http.StatusServiceUnavailable, "payments.asaas-webhook-unavailable")
		return
	}
	if h.RequireTLS && r.TLS == nil && !strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")), "https") {
		asaasWebhookError(w, r, http.StatusForbidden, "payments.asaas-webhook-tls")
		return
	}
	typ, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || typ != "application/json" {
		asaasWebhookError(w, r, http.StatusBadRequest, "payments.asaas-content-type")
		return
	}
	body, err := httpapi.ReadBody(r, h.maxBody())
	if errors.Is(err, httpapi.ErrBodyTooLarge) {
		asaasWebhookError(w, r, http.StatusRequestEntityTooLarge, "payments.asaas-body-too-large")
		return
	}
	if err != nil {
		asaasWebhookError(w, r, http.StatusBadRequest, "payments.asaas-body")
		return
	}
	token := r.Header.Values("asaas-access-token")
	if len(token) != 1 || !h.validToken(token[0]) {
		asaasWebhookError(w, r, http.StatusUnauthorized, "payments.asaas-token")
		return
	}
	var event asaasEvent
	if err := json.Unmarshal(body, &event); err != nil {
		asaasWebhookError(w, r, http.StatusBadRequest, "payments.asaas-event")
		return
	}
	if event.ID == "" || event.Event == "" || len(event.ID) > 255 || len(event.Event) > 128 ||
		event.Account.ID != h.Merchant {
		asaasWebhookError(w, r, http.StatusBadRequest, "payments.asaas-binding")
		return
	}
	resourceType, resourceID := asaasResource(event)
	digest := sha256.Sum256(body)
	_, err = h.Inbox.RecordInbox(r.Context(), app.InboxEvent{
		Provider: domain.ProviderAsaas, Environment: h.Environment, Merchant: h.Merchant,
		EventID: event.ID, PayloadHash: hex.EncodeToString(digest[:]),
		EventType: event.Event, ResourceType: resourceType, ResourceID: resourceID,
	})
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrConflict):
			asaasWebhookError(w, r, http.StatusConflict, "payments.asaas-duplicate-content")
		case errors.Is(err, domain.ErrInvalid):
			asaasWebhookError(w, r, http.StatusBadRequest, "payments.asaas-event-invalid")
		default:
			asaasWebhookError(w, r, http.StatusServiceUnavailable, "payments.asaas-inbox-unavailable")
		}
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
}

func (h AsaasWebhookHandler) configured() bool {
	if h.Inbox == nil || h.APIKey == "" || h.Merchant == "" ||
		(h.Environment != domain.EnvironmentTest && h.Environment != domain.EnvironmentProduction) {
		return false
	}
	if !validAsaasWebhookToken(h.WebhookToken) || h.WebhookToken == h.APIKey {
		return false
	}
	if h.PreviousWebhookToken != "" &&
		(!validAsaasWebhookToken(h.PreviousWebhookToken) || h.PreviousWebhookToken == h.APIKey) {
		return false
	}
	return true
}

func (h AsaasWebhookHandler) validToken(value string) bool {
	if value == "" {
		return false
	}
	if subtle.ConstantTimeCompare([]byte(value), []byte(h.WebhookToken)) == 1 {
		return true
	}
	if h.PreviousWebhookToken != "" && subtle.ConstantTimeCompare([]byte(value), []byte(h.PreviousWebhookToken)) == 1 {
		return true
	}
	return false
}

func (h AsaasWebhookHandler) maxBody() int64 {
	if h.MaxBodyBytes > 0 {
		return h.MaxBodyBytes
	}
	return defaultAsaasWebhookBody
}

func validAsaasWebhookToken(value string) bool {
	if len(value) < 32 || len(value) > maxAsaasTokenLength {
		return false
	}
	for _, r := range value {
		if r <= 32 || r > 126 {
			return false
		}
	}
	return true
}

type asaasResourceRef struct {
	ID string `json:"id"`
}

type asaasEvent struct {
	ID          string `json:"id"`
	Event       string `json:"event"`
	DateCreated string `json:"dateCreated"`
	Account     struct {
		ID string `json:"id"`
	} `json:"account"`
	Payment            *asaasResourceRef `json:"payment"`
	Authorization      *asaasResourceRef `json:"authorization"`
	PaymentInstruction *asaasResourceRef `json:"paymentInstruction"`
}

func asaasResource(event asaasEvent) (string, string) {
	switch {
	case event.Payment != nil && event.Payment.ID != "":
		return "payment", event.Payment.ID
	case event.Authorization != nil && event.Authorization.ID != "":
		return "authorization", event.Authorization.ID
	case event.PaymentInstruction != nil && event.PaymentInstruction.ID != "":
		return "payment_instruction", event.PaymentInstruction.ID
	default:
		return "event", event.ID
	}
}

func asaasWebhookError(w http.ResponseWriter, r *http.Request, status int, code string) {
	httpapi.WriteError(w, r, status, code, "Asaas webhook refused", nil)
}
