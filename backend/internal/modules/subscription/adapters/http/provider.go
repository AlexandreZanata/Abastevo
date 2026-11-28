package http

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/subscription/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/subscription/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
	"github.com/go-chi/chi/v5"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type ProviderHandler struct {
	Verify                                 func(context.Context, string) error
	Store                                  app.JobStore
	Environment, PackageName, Subscription string
	Now                                    func() time.Time
}

func (h ProviderHandler) RegisterRoutes(r chi.Router) {
	r.Post("/v1/premium/provider-notifications", h.ServeHTTP)
}
func (h ProviderHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	fail := func(status int, code string) {
		httpapi.WriteError(w, r, status, "subscription."+code, "subscription notification refused", nil)
	}
	if h.Verify == nil || h.Store == nil || h.Subscription == "" || h.PackageName == "" || (h.Environment != "test" && h.Environment != "production") {
		fail(503, "provider-disabled")
		return
	}
	auth := r.Header.Values("Authorization")
	if len(auth) != 1 || !strings.HasPrefix(auth[0], "Bearer ") {
		fail(401, "auth-required")
		return
	}
	if err := h.Verify(r.Context(), strings.TrimPrefix(auth[0], "Bearer ")); err != nil {
		fail(401, "auth-required")
		return
	}
	typ, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || typ != "application/json" {
		fail(400, "invalid")
		return
	}
	raw, err := httpapi.ReadBody(r, 16384)
	if err != nil {
		fail(400, "invalid")
		return
	}
	var envelope struct {
		Subscription    string `json:"subscription"`
		DeliveryAttempt int    `json:"deliveryAttempt,omitempty"`
		Message         struct {
			ID             string            `json:"messageId"`
			IDAlias        *string           `json:"message_id,omitempty"`
			Data           string            `json:"data"`
			Published      string            `json:"publishTime"`
			PublishedAlias *string           `json:"publish_time,omitempty"`
			Attributes     map[string]string `json:"attributes,omitempty"`
			OrderingKey    string            `json:"orderingKey,omitempty"`
		} `json:"message"`
	}
	if httpapi.DecodeJSON(raw, &envelope) != nil || envelope.Subscription != h.Subscription || len(envelope.Message.ID) == 0 || len(envelope.Message.ID) > 128 {
		fail(400, "invalid")
		return
	}
	// Wrapped Pub/Sub messages can repeat these two fields for compatibility.
	// Keep one message identity and one publish time for deduplication and age checks.
	if msg := envelope.Message; (msg.IDAlias != nil && *msg.IDAlias != msg.ID) ||
		(msg.PublishedAlias != nil && *msg.PublishedAlias != msg.Published) {
		fail(400, "invalid")
		return
	}
	for _, c := range envelope.Message.ID {
		if c <= 32 || c > 126 {
			fail(400, "invalid")
			return
		}
	}
	now := time.Now()
	if h.Now != nil {
		now = h.Now()
	}
	published, err := time.Parse(time.RFC3339Nano, envelope.Message.Published)
	if err != nil || published.After(now.Add(time.Minute)) {
		fail(400, "invalid")
		return
	}
	if now.Sub(published) > 10*time.Minute {
		w.WriteHeader(204)
		return
	}
	data, err := base64.StdEncoding.DecodeString(envelope.Message.Data)
	if err != nil || len(data) > 12288 {
		fail(400, "invalid")
		return
	}
	var event struct {
		Version      string `json:"version"`
		Package      string `json:"packageName"`
		At           string `json:"eventTimeMillis"`
		Subscription *struct {
			Version string `json:"version"`
			Type    int    `json:"notificationType"`
			Token   string `json:"purchaseToken"`
			Product string `json:"subscriptionId,omitempty"`
		} `json:"subscriptionNotification,omitempty"`
		Test *struct {
			Version string `json:"version"`
		} `json:"testNotification,omitempty"`
		// Voided purchases carry refunds and chargebacks (productType 1 is a
		// subscription); one-time products are not sold, so they are ignored.
		Voided *struct {
			Token       string `json:"purchaseToken"`
			ProductType int    `json:"productType"`
		} `json:"voidedPurchaseNotification,omitempty"`
		OneTime *struct{} `json:"oneTimeProductNotification,omitempty"`
	}
	if json.Unmarshal(data, &event) != nil || event.Version != "1.0" || event.Package != h.PackageName {
		fail(400, "invalid")
		return
	}
	at, err := strconv.ParseInt(event.At, 10, 64)
	if err != nil || at <= 0 || at > now.Add(time.Minute).UnixMilli() {
		fail(400, "invalid")
		return
	}
	kinds := 0
	for _, present := range []bool{event.Subscription != nil, event.Test != nil, event.Voided != nil, event.OneTime != nil} {
		if present {
			kinds++
		}
	}
	if kinds != 1 {
		fail(400, "invalid")
		return
	}
	if event.Test != nil || event.OneTime != nil || (event.Voided != nil && event.Voided.ProductType != 1) {
		w.WriteHeader(204)
		return
	}
	token := ""
	if event.Voided != nil {
		token = event.Voided.Token
	} else {
		if event.Subscription.Version != "1.0" || event.Subscription.Type <= 0 {
			fail(400, "invalid")
			return
		}
		token = event.Subscription.Token
	}
	if !domain.ValidToken(token) {
		fail(400, "invalid")
		return
	}
	sum := sha256.Sum256(data)
	_, err = h.Store.ReceiveNotification(r.Context(), app.Notification{Provider: domain.ProviderPlay, Environment: h.Environment, PackageName: h.PackageName, MessageID: envelope.Message.ID, Digest: domain.TokenDigest(h.Environment, h.PackageName, token), PayloadHash: hex.EncodeToString(sum[:])})
	if err != nil {
		switch err {
		case domain.ErrConflict:
			fail(409, "conflict")
		case domain.ErrQuota:
			w.Header().Set("Retry-After", "60")
			fail(429, "quota")
		default:
			fail(503, "unavailable")
		}
		return
	}
	w.WriteHeader(204)
}
