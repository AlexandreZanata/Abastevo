package http

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"mime"
	"net/http"
	"strings"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/subscription/adapters/appstore"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/subscription/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/subscription/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
	"github.com/go-chi/chi/v5"
)

// AppStoreNotifications resolves a verified notification to its purchase.
type AppStoreNotifications interface {
	DecodeNotification(signedPayload string) (appstore.Notification, error)
	NotificationTarget(appstore.Notification) (string, error)
}

// AppStoreHandler receives App Store Server Notifications V2. The signature
// chain to Apple's root is the authentication; the notification only
// schedules a refresh of an already bound purchase (B-BR-STORE-07).
type AppStoreHandler struct {
	Client                AppStoreNotifications
	Store                 app.JobStore
	Environment, BundleID string
}

func (h AppStoreHandler) RegisterRoutes(r chi.Router) {
	r.Post("/v1/premium/app-store-notifications", h.ServeHTTP)
}

func (h AppStoreHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	fail := func(status int, code string) {
		httpapi.WriteError(w, r, status, "subscription."+code, "subscription notification refused", nil)
	}
	if h.Client == nil || h.Store == nil || h.BundleID == "" || (h.Environment != "test" && h.Environment != "production") {
		fail(503, "provider-disabled")
		return
	}
	typ, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || typ != "application/json" {
		fail(400, "invalid")
		return
	}
	raw, err := httpapi.ReadBody(r, 64<<10)
	if err != nil {
		fail(400, "invalid")
		return
	}
	var body struct {
		SignedPayload string `json:"signedPayload"`
	}
	if httpapi.DecodeJSON(raw, &body) != nil || body.SignedPayload == "" {
		fail(400, "invalid")
		return
	}
	notice, err := h.Client.DecodeNotification(body.SignedPayload)
	if err != nil {
		fail(401, "auth-required")
		return
	}
	original, err := h.Client.NotificationTarget(notice)
	if err != nil {
		fail(400, "invalid")
		return
	}
	if original == "" {
		w.WriteHeader(http.StatusOK)
		return
	}
	// A retry keeps notificationUUID but may be signed again, so the receipt
	// hash covers the stable meaning, not the signature bytes.
	sum := sha256.Sum256([]byte(notice.NotificationUUID + "\x00" + notice.NotificationType + "\x00" + notice.Subtype + "\x00" + original))
	_, err = h.Store.ReceiveNotification(r.Context(), app.Notification{
		Provider: domain.ProviderAppStore, Environment: h.Environment, PackageName: h.BundleID,
		MessageID:   "app_store:" + strings.ToLower(notice.NotificationUUID),
		Digest:      domain.StoreTokenDigest(domain.ProviderAppStore, h.Environment, h.BundleID, original),
		PayloadHash: hex.EncodeToString(sum[:]),
	})
	switch {
	case err == nil:
		w.WriteHeader(http.StatusOK)
	case errors.Is(err, domain.ErrConflict):
		fail(409, "conflict")
	case errors.Is(err, domain.ErrQuota):
		w.Header().Set("Retry-After", "60")
		fail(429, "quota")
	default:
		fail(503, "unavailable")
	}
}
