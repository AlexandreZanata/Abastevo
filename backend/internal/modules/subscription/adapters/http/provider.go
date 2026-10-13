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
		Subscription string `json:"subscription"`
		Message      struct {
			ID          string            `json:"messageId"`
			Data        string            `json:"data"`
			Published   string            `json:"publishTime"`
			Attributes  map[string]string `json:"attributes,omitempty"`
			OrderingKey string            `json:"orderingKey,omitempty"`
		} `json:"message"`
	}
	if httpapi.DecodeJSON(raw, &envelope) != nil || envelope.Subscription != h.Subscription || len(envelope.Message.ID) == 0 || len(envelope.Message.ID) > 128 {
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
	if event.Test != nil && event.Subscription == nil {
		w.WriteHeader(204)
		return
	}
	if event.Subscription == nil || event.Test != nil || event.Subscription.Version != "1.0" || event.Subscription.Type <= 0 || !domain.ValidToken(event.Subscription.Token) {
		fail(400, "invalid")
		return
	}
	sum := sha256.Sum256(data)
	_, err = h.Store.ReceiveNotification(r.Context(), app.Notification{Environment: h.Environment, PackageName: h.PackageName, MessageID: envelope.Message.ID, Digest: domain.TokenDigest(h.Environment, h.PackageName, event.Subscription.Token), PayloadHash: hex.EncodeToString(sum[:])})
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
