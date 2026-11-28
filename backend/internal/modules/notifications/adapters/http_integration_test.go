//go:build integration

package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	notificationshttp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/notifications/adapters/http"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/notifications/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/notifications/domain"
)

// The owner API over the real store: list pages with a sealed cursor, the
// signed read marks only the caller's rows, and a replay answers the same.
func TestOwnerAPIOnRealStore(t *testing.T) {
	pool := freshDB(t)
	s := NewStore(pool)
	owner, other := newAccount(t, pool, "active"), newAccount(t, pool, "active")
	family := uuid(t)
	at := time.Date(2026, 11, 28, 12, 0, 0, 0, time.UTC)
	reply := replyEvent(t, owner, "44444444-4444-4444-8444-444444444444", at)
	decided, err := domain.NewContributionDecision(owner, false, domain.Contribution{
		ObservationID: "55555555-5555-4555-8555-555555555555", StationID: "66666666-6666-4666-8666-666666666666",
		StationName: "Synthetic Station", Fuel: "ETHANOL", Reason: "evidence-timeout",
	}, at.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range []domain.Event{reply, decided, replyEvent(t, other, uuid(t), at)} {
		if err := notify(t, pool, s, e, true); err != nil {
			t.Fatal(err)
		}
	}
	signed := true
	h := notificationshttp.Handler{
		Service:      application.Service{Inbox: s, Clock: func() time.Time { return at.Add(time.Hour) }},
		CursorSecret: []byte("0123456789abcdef0123456789abcdef"),
		Origin:       "api.test",
		Clock:        func() time.Time { return at.Add(time.Hour) },
		Sessions: func(_ context.Context, f, a string) (string, error) {
			if f != family || a != "synthetic" {
				return "", errors.New("no session")
			}
			return owner, nil
		},
		SignedOwner: func(*http.Request, string) (string, string, error) {
			if !signed {
				return "", "", errors.New("no proof")
			}
			return "contributor", "key", nil
		},
	}
	router := chi.NewRouter()
	h.RegisterRoutes(router)
	call := func(path, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		return w
	}
	session := `"family_id":"` + family + `","access_token":"synthetic"`

	first := call("/v1/notifications/list", `{`+session+`,"limit":1}`)
	t.Logf("list page 1: %s", first.Body.String())
	var page struct {
		Items []struct {
			NotificationID string          `json:"notification_id"`
			Kind           string          `json:"kind"`
			CreatedAt      string          `json:"created_at"`
			Read           bool            `json:"read"`
			Payload        json.RawMessage `json:"payload"`
		} `json:"items"`
		NextBefore *string `json:"next_before"`
		Unread     int64   `json:"unread"`
	}
	if err := json.Unmarshal(first.Body.Bytes(), &page); err != nil || first.Code != 200 {
		t.Fatalf("list %d %s", first.Code, first.Body.String())
	}
	if len(page.Items) != 1 || page.Items[0].Kind != domain.KindContributionRejected || page.Items[0].CreatedAt != "2026-11-28T12:01:00Z" || page.Unread != 2 || page.NextBefore == nil {
		t.Fatalf("first page %+v", page)
	}
	newest := page.Items[0].NotificationID
	second := call("/v1/notifications/list", `{`+session+`,"limit":1,"before":"`+*page.NextBefore+`"}`)
	page.Items, page.NextBefore = nil, nil
	if err := json.Unmarshal(second.Body.Bytes(), &page); err != nil || len(page.Items) != 1 || page.Items[0].Kind != domain.KindForumReply || page.NextBefore != nil {
		t.Fatalf("second page %d %s", second.Code, second.Body.String())
	}

	var foreign string
	if err := pool.QueryRow(context.Background(), "SELECT id::text FROM account_notifications WHERE account_id=$1", other).Scan(&foreign); err != nil {
		t.Fatal(err)
	}
	readBody := `{` + session + `,"operation_id":"` + uuid(t) + `","notification_ids":["` + newest + `","` + foreign + `"]}`
	for range 2 {
		w := call("/v1/notifications/read", readBody)
		t.Logf("read request: %s -> %d %s", readBody, w.Code, w.Body.String())
		if w.Code != 200 || w.Body.String() != `{"marked":1}` || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("read %d %s", w.Code, w.Body.String())
		}
	}
	if count(t, pool, "account_id=$1 AND read_at IS NULL", other) != 1 {
		t.Fatal("a foreign id was marked")
	}
	signed = false
	if w := call("/v1/notifications/read", `{`+session+`,"operation_id":"`+uuid(t)+`","all":true}`); w.Code != 401 {
		t.Fatalf("unsigned read %d", w.Code)
	}
	if count(t, pool, "account_id=$1 AND read_at IS NULL", owner) != 1 {
		t.Fatal("an unsigned write changed state")
	}
}
