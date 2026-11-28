package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	forumapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/application"
	notificationsdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/notifications/domain"
)

// B-BR-NOTIF-03: the production router mounts both notification routes,
// OpenAPI documents them, and neither answers without a live session.
func TestNotificationRoutesAreComposedDocumentedAndPrivate(t *testing.T) {
	appleCompositionEnv(t, map[string]string{
		"ANPFUEL_GOOGLE_NATIVE_AUDIENCE":    "",
		"ANPFUEL_GOOGLE_AUTHORIZED_PARTIES": "",
	})
	spec, err := openapi3.NewLoader().LoadFromFile(filepath.Join("..", "..", "..", "contracts", "openapi", "v1.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	routes := []string{"/v1/notifications/list", "/v1/notifications/read"}
	stopped := errors.New("notification route composition checkpoint")
	err = runWithRouterCheck(func(router chi.Router) error {
		seen := map[string]bool{}
		if err := chi.Walk(router, func(method, path string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
			if strings.HasPrefix(path, "/v1/notifications/") {
				if method != http.MethodPost || seen[path] {
					t.Errorf("unexpected notification route %s %s", method, path)
				}
				seen[path] = true
			}
			return nil
		}); err != nil {
			return err
		}
		for _, path := range routes {
			if !seen[path] {
				t.Errorf("production router misses POST %s", path)
				continue
			}
			item := spec.Paths.Find(path)
			if item == nil || item.Post == nil {
				t.Errorf("OpenAPI missing POST %s", path)
				continue
			}
			for body, want := range map[string]int{
				`{}`: http.StatusBadRequest,
				`{"family_id":"11111111-1111-4111-8111-111111111111","access_token":"synthetic","operation_id":"cccccccc-cccc-4ccc-8ccc-cccccccccccc","all":true}`: http.StatusUnauthorized,
			} {
				if path == "/v1/notifications/list" && want == http.StatusUnauthorized {
					body = `{"family_id":"11111111-1111-4111-8111-111111111111","access_token":"synthetic"}`
				}
				req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, req)
				if rec.Code != want || item.Post.Responses.Status(want) == nil || rec.Header().Get("Cache-Control") != "no-store" {
					t.Errorf("%s %s returned %d (cache %q), want documented %d", path, body, rec.Code, rec.Header().Get("Cache-Control"), want)
				}
			}
		}
		return stopped
	})
	if !errors.Is(err, stopped) {
		t.Fatalf("check the production composition: %v", err)
	}
}

type recordingNotifier struct{ events []notificationsdomain.Event }

func (r *recordingNotifier) Notify(_ context.Context, _ pgx.Tx, e notificationsdomain.Event) error {
	r.events = append(r.events, e)
	return nil
}

// The composition glue maps every forum fact onto the forum.reply payload
// and leaves the self-reply rule to the notifications domain.
func TestForumReplyNoticeGlue(t *testing.T) {
	rec := &recordingNotifier{}
	notify := forumReplyNotices(rec)
	notice := forumapp.ReplyNotice{
		PostID: "33333333-3333-4333-8333-333333333333", PostTitle: "Etanol em Cuiabá",
		PostAuthorAccountID: "11111111-1111-4111-8111-111111111111",
		CommentID:           "44444444-4444-4444-8444-444444444444", CommentAuthorAccountID: "22222222-2222-4222-8222-222222222222",
		AuthorName: "Motorista MT",
	}
	if err := notify(context.Background(), nil, notice); err != nil || len(rec.events) != 1 {
		t.Fatalf("notice %v %d", err, len(rec.events))
	}
	got := rec.events[0]
	want := `{"topic_id":"33333333-3333-4333-8333-333333333333","topic_title":"Etanol em Cuiabá","comment_id":"44444444-4444-4444-8444-444444444444","author_name":"Motorista MT"}`
	if got.AccountID != notice.PostAuthorAccountID || got.Kind != "forum.reply" || string(got.Payload) != want {
		t.Fatalf("event %+v %s", got, got.Payload)
	}
	notice.CommentAuthorAccountID = notice.PostAuthorAccountID
	if err := notify(context.Background(), nil, notice); err != nil || len(rec.events) != 1 {
		t.Fatalf("self reply notified: %v %d", err, len(rec.events))
	}
}
