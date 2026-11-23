package http

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/subscription/adapters/appstore"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/subscription/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/subscription/domain"
	"github.com/go-chi/chi/v5"
)

type fakeAppleClient struct {
	notice    appstore.Notification
	decodeErr error
	target    string
	targetErr error
}

func (f fakeAppleClient) DecodeNotification(string) (appstore.Notification, error) {
	return f.notice, f.decodeErr
}
func (f fakeAppleClient) NotificationTarget(appstore.Notification) (string, error) {
	return f.target, f.targetErr
}

type receiptProbe struct {
	notes []app.Notification
	err   error
}

func (r *receiptProbe) ForJob(context.Context, string) (app.StoredToken, error) {
	return app.StoredToken{}, domain.ErrNotFound
}
func (r *receiptProbe) MarkAcknowledged(context.Context, string, int64) error { return nil }
func (r *receiptProbe) ReceiveNotification(_ context.Context, n app.Notification) (bool, error) {
	r.notes = append(r.notes, n)
	return false, r.err
}

func postApple(h AppStoreHandler, body, contentType string) *httptest.ResponseRecorder {
	router := chi.NewRouter()
	h.RegisterRoutes(router)
	req := httptest.NewRequest(http.MethodPost, "/v1/premium/app-store-notifications", strings.NewReader(body))
	req.Header.Set("Content-Type", contentType)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func TestAppStoreNotificationSchedulesRefreshOfBoundPurchase(t *testing.T) {
	store := &receiptProbe{}
	notice := appstore.Notification{NotificationType: "DID_RENEW", NotificationUUID: "002E14D5-51F5-4503-B5A8-C3A1AF68EB20", Version: "2.0"}
	h := AppStoreHandler{Client: fakeAppleClient{notice: notice, target: "2000000123456789"}, Store: store, Environment: "production", BundleID: "com.anpfuel.app"}
	w := postApple(h, `{"signedPayload":"a.b.c"}`, "application/json")
	if w.Code != 200 || len(store.notes) != 1 {
		t.Fatalf("got %d %v", w.Code, store.notes)
	}
	n := store.notes[0]
	want := domain.StoreTokenDigest(domain.ProviderAppStore, "production", "com.anpfuel.app", "2000000123456789")
	if n.Provider != domain.ProviderAppStore || n.Digest != want || n.MessageID != "app_store:002e14d5-51f5-4503-b5a8-c3a1af68eb20" || len(n.PayloadHash) != 64 {
		t.Fatalf("notification %+v", n)
	}
	again := postApple(h, `{"signedPayload":"a.b.d"}`, "application/json")
	if again.Code != 200 || store.notes[1].PayloadHash != n.PayloadHash {
		t.Fatal("a re-signed retry must keep the receipt hash")
	}
}

func TestAppStoreNotificationRefusals(t *testing.T) {
	notice := appstore.Notification{NotificationType: "DID_RENEW", NotificationUUID: "002e14d5-51f5-4503-b5a8-c3a1af68eb20", Version: "2.0"}
	cases := []struct {
		name   string
		client fakeAppleClient
		body   string
		ctype  string
		storeE error
		status int
		stored bool
	}{
		{"bad signature", fakeAppleClient{decodeErr: errors.New("chain")}, `{"signedPayload":"x"}`, "application/json", nil, 401, false},
		{"other app", fakeAppleClient{notice: notice, targetErr: domain.ErrInvalid}, `{"signedPayload":"x"}`, "application/json", nil, 400, false},
		{"test notice", fakeAppleClient{notice: notice}, `{"signedPayload":"x"}`, "application/json", nil, 200, false},
		{"not json", fakeAppleClient{notice: notice, target: "1"}, `signedPayload=x`, "application/x-www-form-urlencoded", nil, 400, false},
		{"empty", fakeAppleClient{notice: notice, target: "1"}, `{}`, "application/json", nil, 400, false},
		{"conflict", fakeAppleClient{notice: notice, target: "1"}, `{"signedPayload":"x"}`, "application/json", domain.ErrConflict, 409, true},
		{"quota", fakeAppleClient{notice: notice, target: "1"}, `{"signedPayload":"x"}`, "application/json", domain.ErrQuota, 429, true},
		{"database", fakeAppleClient{notice: notice, target: "1"}, `{"signedPayload":"x"}`, "application/json", domain.ErrUnavailable, 503, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			store := &receiptProbe{err: c.storeE}
			h := AppStoreHandler{Client: c.client, Store: store, Environment: "production", BundleID: "com.anpfuel.app"}
			w := postApple(h, c.body, c.ctype)
			if w.Code != c.status || (len(store.notes) > 0) != c.stored || w.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("got %d stored=%d", w.Code, len(store.notes))
			}
		})
	}
	if w := postApple(AppStoreHandler{}, `{"signedPayload":"x"}`, "application/json"); w.Code != 503 {
		t.Fatalf("unconfigured handler: %d", w.Code)
	}
}
