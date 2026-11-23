package http

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/subscription/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/subscription/domain"
)

type playReceipts struct{ notes []app.Notification }

func (p *playReceipts) ForJob(context.Context, string) (app.StoredToken, error) {
	return app.StoredToken{}, domain.ErrNotFound
}
func (p *playReceipts) MarkAcknowledged(context.Context, string, int64) error { return nil }
func (p *playReceipts) ReceiveNotification(_ context.Context, n app.Notification) (bool, error) {
	p.notes = append(p.notes, n)
	return false, nil
}

func postPlay(t *testing.T, event map[string]any) (*httptest.ResponseRecorder, *playReceipts) {
	t.Helper()
	now := time.Now().UTC()
	event["version"] = "1.0"
	event["packageName"] = "com.anpfuel.app"
	event["eventTimeMillis"] = strconv.FormatInt(now.UnixMilli(), 10)
	data, _ := json.Marshal(event)
	raw, _ := json.Marshal(map[string]any{"subscription": "projects/p/subscriptions/rtdn", "message": map[string]any{
		"messageId": "m-1", "publishTime": now.Format(time.RFC3339Nano), "data": base64.StdEncoding.EncodeToString(data)}})
	store := &playReceipts{}
	h := ProviderHandler{Store: store, Environment: "production", PackageName: "com.anpfuel.app", Subscription: "projects/p/subscriptions/rtdn",
		Now: func() time.Time { return now }, Verify: func(context.Context, string) error { return nil }}
	r := httptest.NewRequest("POST", "/v1/premium/provider-notifications", strings.NewReader(string(raw)))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Authorization", "Bearer proof")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w, store
}

func TestPlayVoidedSubscriptionSchedulesRefresh(t *testing.T) {
	w, store := postPlay(t, map[string]any{"voidedPurchaseNotification": map[string]any{"purchaseToken": "voided-token", "orderId": "GPA.1", "productType": 1, "refundType": 1}})
	if w.Code != 204 || len(store.notes) != 1 {
		t.Fatalf("voided subscription: %d %v", w.Code, store.notes)
	}
	n := store.notes[0]
	if n.Provider != domain.ProviderPlay || n.Digest != domain.TokenDigest("production", "com.anpfuel.app", "voided-token") {
		t.Fatalf("notification %+v", n)
	}
}

func TestPlayIgnoresOneTimeEventsAndRejectsAmbiguousEnvelopes(t *testing.T) {
	for name, event := range map[string]map[string]any{
		"voided one-time": {"voidedPurchaseNotification": map[string]any{"purchaseToken": "t", "productType": 2}},
		"one-time":        {"oneTimeProductNotification": map[string]any{"version": "1.0", "notificationType": 1, "purchaseToken": "t", "sku": "x"}},
		"test":            {"testNotification": map[string]any{"version": "1.0"}},
	} {
		if w, store := postPlay(t, event); w.Code != 204 || len(store.notes) != 0 {
			t.Fatalf("%s: %d %v", name, w.Code, store.notes)
		}
	}
	both := map[string]any{
		"subscriptionNotification":   map[string]any{"version": "1.0", "notificationType": 2, "purchaseToken": "t"},
		"voidedPurchaseNotification": map[string]any{"purchaseToken": "t", "productType": 1},
	}
	if w, store := postPlay(t, both); w.Code != 400 || len(store.notes) != 0 {
		t.Fatalf("ambiguous envelope: %d", w.Code)
	}
	if w, store := postPlay(t, map[string]any{}); w.Code != 400 || len(store.notes) != 0 {
		t.Fatalf("empty envelope: %d", w.Code)
	}
}
