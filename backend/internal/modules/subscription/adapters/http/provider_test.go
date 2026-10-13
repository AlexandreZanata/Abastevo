package http

import (
	"context"
	"encoding/base64"
	"encoding/json"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/subscription/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/subscription/domain"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

type inboxProbe struct{ calls int }

func (p *inboxProbe) ForJob(context.Context, string) (app.StoredToken, error) {
	return app.StoredToken{}, domain.ErrNotFound
}
func (p *inboxProbe) MarkAcknowledged(context.Context, string, int64) error { return nil }
func (p *inboxProbe) ReceiveNotification(context.Context, app.Notification) (bool, error) {
	p.calls++
	return false, nil
}
func TestProviderEnvelopeRequiresProofBeforePersistence(t *testing.T) {
	now := time.Now().UTC()
	event, _ := json.Marshal(map[string]any{"version": "1.0", "packageName": "com.anpfuel", "eventTimeMillis": strconv.FormatInt(now.UnixMilli(), 10), "subscriptionNotification": map[string]any{"version": "1.0", "notificationType": 2, "purchaseToken": "synthetic-token"}})
	for _, name := range []string{"valid", "unauthenticated", "forged", "duplicate-header", "wrong-subscription", "stale", "oversize", "duplicate-json", "disabled"} {
		t.Run(name, func(t *testing.T) {
			store := &inboxProbe{}
			h := ProviderHandler{Store: store, Environment: "test", PackageName: "com.anpfuel", Subscription: "projects/test/subscriptions/rtdn", Now: func() time.Time { return now }, Verify: func(_ context.Context, raw string) error {
				if raw != "trusted-proof" {
					return domain.ErrUnauthorized
				}
				return nil
			}}
			subscription := h.Subscription
			published := now.Format(time.RFC3339Nano)
			if name == "wrong-subscription" {
				subscription = "foreign"
			}
			if name == "stale" {
				published = now.Add(-11 * time.Minute).Format(time.RFC3339Nano)
			}
			raw, _ := json.Marshal(map[string]any{"subscription": subscription, "message": map[string]any{"messageId": "synthetic-id", "publishTime": published, "data": base64.StdEncoding.EncodeToString(event)}})
			body := string(raw)
			if name == "oversize" {
				body = strings.Repeat("x", 16385)
			}
			if name == "duplicate-json" {
				body = `{"subscription":"a","subscription":"b"}`
			}
			if name == "disabled" {
				h.Verify = nil
			}
			r := httptest.NewRequest("POST", "/v1/premium/provider-notifications", strings.NewReader(body))
			r.Header.Set("Content-Type", "application/json")
			if name != "unauthenticated" {
				r.Header.Set("Authorization", "Bearer trusted-proof")
			}
			if name == "forged" {
				r.Header.Set("Authorization", "Bearer forged")
			}
			if name == "duplicate-header" {
				r.Header.Add("Authorization", "Bearer trusted-proof")
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("private response cacheable")
			}
			if name == "valid" {
				if w.Code != 204 || store.calls != 1 {
					t.Fatal("valid receipt refused", w.Code)
				}
			} else if name == "stale" {
				if w.Code != 204 || store.calls != 0 {
					t.Fatal("late event retried/sent")
				}
			} else if w.Code < 400 || store.calls != 0 {
				t.Fatal("unproved envelope persisted", w.Code)
			}
			if strings.Contains(w.Body.String(), "synthetic-token") {
				t.Fatal("private provider token leaked")
			}
		})
	}
}
