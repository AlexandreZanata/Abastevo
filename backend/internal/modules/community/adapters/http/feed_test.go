package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	communityread "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/community/adapters/read"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/community/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
)

func TestFeedAnonymousScopeCursorAndFailures(t *testing.T) {
	secret := []byte("synthetic-feed-cursor-secret-32bytes")
	query := "/v1/community/feed?state=MT&municipality_code=5107925&fuel_product=ETHANOL&limit=1"
	called := 0
	h := FeedHandler{Secrets: secret, Read: func(_ context.Context, f application.FeedFilter) ([]communityread.FeedItem, string, error) {
		called++
		if f.State != "MT" || f.Municipality != "5107925" || f.Product != "ETHANOL" {
			t.Fatalf("scope %+v", f)
		}
		return []communityread.FeedItem{{StationID: "d6c74c23-63db-4c24-a2e5-408cb23bad26", StationName: "Synthetic station", AmountMilliBrl: 3980, Source: "COMMUNITY"}}, application.FeedKey(time.Now(), "d6c74c23-63db-4c24-a2e5-408cb23bad26", 3980), nil
	}}
	r := chi.NewRouter()
	h.RegisterRoutes(r)
	request := func(path string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		return w
	}
	w := request(query)
	if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("anonymous %d %s", w.Code, w.Body.String())
	}
	var page struct {
		Next string `json:"next_cursor"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil || page.Next == "" {
		t.Fatal("missing cursor")
	}
	if request(query+"&cursor="+page.Next).Code != 200 {
		t.Fatal("valid pagination refused")
	}
	for _, path := range []string{strings.Replace(query, "5107925", "5103403", 1) + "&cursor=" + page.Next, query + "&sort=cheapest&cursor=" + page.Next, query + "&cursor=bad", strings.Replace(query, "limit=1", "limit=51", 1), strings.Replace(query, "ETHANOL", "OTHER", 1), query + "&sort=popular", "/v1/community/feed"} {
		before := called
		if request(path).Code != 400 || called != before {
			t.Fatalf("invalid request reached read: %s", path)
		}
	}
	for _, order := range []string{"best", "worst"} {
		w := request(query + "&sort=" + order)
		if w.Code != 200 || !strings.Contains(w.Body.String(), `"sort":"`+order+`"`) {
			t.Fatalf("order %s not echoed: %d %s", order, w.Code, w.Body.String())
		}
	}
	fh := httpapi.FilterHash("community-feed-v1", "MT", "5107925", "ETHANOL", "recent", "1")
	expired := httpapi.Seal(secret, fh, "{}", time.Now().Add(-16*time.Minute))
	if request(query+"&cursor="+expired).Code != 400 {
		t.Fatal("expired accepted")
	}
	h.Read = func(context.Context, application.FeedFilter) ([]communityread.FeedItem, string, error) {
		return nil, "", errors.New("private database details")
	}
	r = chi.NewRouter()
	h.RegisterRoutes(r)
	w = request(query)
	if w.Code != 503 || strings.Contains(w.Body.String(), "private database") {
		t.Fatal("failure not redacted")
	}
	// The public allowlist can never serialize these private fields.
	raw, _ := json.Marshal(communityread.FeedItem{})
	for _, field := range []string{"contributor_ref", "evidence_id", "lat", "lon", "email", "object_key"} {
		if strings.Contains(string(raw), `"`+field+`"`) {
			t.Fatalf("leaked %s", field)
		}
	}
}
