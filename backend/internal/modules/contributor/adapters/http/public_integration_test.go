//go:build integration

package http

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	migrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/contributor/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/contributor/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func freshDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("ANPFUEL_TEST_DATABASE_URL")
	if dsn == "" {
		t.Fatal("explicit disposable PostGIS DSN required")
	}
	u, err := url.Parse(dsn)
	if err != nil || u.Hostname() != "127.0.0.1" {
		t.Fatal("literal loopback test DSN required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal("disposable database unavailable")
	}
	name := fmt.Sprintf("contributor_http_test_%d", time.Now().UnixNano())
	if _, err = admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
		admin.Close(ctx)
		t.Fatal(err)
	}
	admin.Close(ctx)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		admin, err := pgx.Connect(ctx, dsn)
		if err != nil {
			t.Error("cleanup connection failed")
			return
		}
		defer admin.Close(ctx)
		if _, err = admin.Exec(ctx, "DROP DATABASE "+pgx.Identifier{name}.Sanitize()+" WITH (FORCE)"); err != nil {
			t.Error(err)
		}
	})
	u.Path = "/" + name
	if _, err = migrate.Apply(ctx, u.String(), migrations.Files); err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.New(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func newID(t *testing.T) string {
	t.Helper()
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		t.Fatal(err)
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	raw := hex.EncodeToString(b[:])
	return raw[0:8] + "-" + raw[8:12] + "-" + raw[12:16] + "-" + raw[16:20] + "-" + raw[20:32]
}

func seedPublic(t *testing.T, pool *pgxpool.Pool, name, visibility string, public bool, showRoles bool, roles []string, since time.Time, login string) string {
	t.Helper()
	accountID := newID(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, "INSERT INTO accounts(id,alias,status) VALUES($1,$2,'active')", accountID, "synthetic-"+accountID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO contributor_profiles(account_id,display_name,visibility,public_consent,show_roles_publicly) VALUES($1,$2,$3,$4,$5)`, accountID, name, visibility, public, showRoles); err != nil {
		t.Fatal(err)
	}
	for _, role := range roles {
		if _, err := pool.Exec(ctx, `INSERT INTO staff_assignments(id,account_id,role,state,start_at,end_at,reason) VALUES($1,$2,$3,'ACTIVE',$4,NULL,'synthetic staff assignment')`, newID(t), accountID, role, since); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := pool.Exec(ctx, `INSERT INTO entitlement_grants(id,account_id,tier,policy_version,state,start_at,end_at,reason) VALUES($1,$2,'CONTRIBUTOR',1,'ACTIVE',$3,NULL,'synthetic public grant')`, newID(t), accountID, since); err != nil {
		t.Fatal(err)
	}
	if login != "" {
		if _, err := pool.Exec(ctx, `INSERT INTO contributor_links(account_id,platform,handle) VALUES($1,'github',$2)`, accountID, login); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO contributor_github_links(account_id,github_login,linked_at) VALUES($1,$2,$3)`, accountID, strings.ToLower(login), since); err != nil {
			t.Fatal(err)
		}
	}
	return accountID
}

// TestPublicContributors_CTB09_CTB10_CTB11 covers JSON allowlist, ordering,
// pagination, cache and CORS on the only public route.
func TestPublicContributors_CTB09_CTB10_CTB11(t *testing.T) {
	pool := freshDB(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	service := &application.Service{Store: adapters.NewStore(pool), Clock: func() time.Time { return now }, NewID: func() (string, error) { return newID(t), nil }}
	firstID := seedPublic(t, pool, "Ana", "PUBLIC", true, true, []string{"community_moderator"}, now.Add(-3*time.Hour), "octocat")
	seedPublic(t, pool, "Bia", "PUBLIC", true, false, []string{"support_agent"}, now.Add(-2*time.Hour), "")
	thirdID := seedPublic(t, pool, "Caio", "PUBLIC", true, false, nil, now.Add(-time.Hour), "")
	hiddenID := seedPublic(t, pool, "Hidden", "HIDDEN", true, false, nil, now.Add(-time.Hour), "")
	router := chi.NewRouter()
	PublicHandler{Service: service, CursorSecret: bytes.Repeat([]byte{9}, 32), AllowedOrigins: []string{"https://landing.example"}}.RegisterRoutes(router)
	router.Get("/v1/stations", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/v1/contributors?limit=2", nil)
	req.Header.Set("Origin", "https://landing.example")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if w.Header().Get("Cache-Control") != "public, max-age=300" || w.Header().Get("Access-Control-Allow-Origin") != "https://landing.example" {
		t.Fatalf("headers=%v", w.Header())
	}
	var page map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if len(page) != 2 || page["items"] == nil || page["next_cursor"] == nil {
		t.Fatalf("top-level keys=%v", page)
	}
	var items []map[string]json.RawMessage
	if err := json.Unmarshal(page["items"], &items); err != nil || len(items) != 2 {
		t.Fatalf("items=%s err=%v", page["items"], err)
	}
	allowed := map[string]bool{"display_name": true, "links": true, "github_login": true, "contributor_since": true, "roles": true}
	for _, item := range items {
		for key := range item {
			if !allowed[key] {
				t.Fatalf("public item leaked key %q", key)
			}
		}
	}
	for _, forbidden := range []string{firstID, hiddenID, "synthetic-", "@", "reason", "grant_id", "account_id", "alias"} {
		if strings.Contains(w.Body.String(), forbidden) {
			t.Fatalf("public body leaked %q: %s", forbidden, w.Body.String())
		}
	}
	var since string
	if err := json.Unmarshal(items[0]["contributor_since"], &since); err != nil || !regexp.MustCompile(`^[0-9]{4}-[0-9]{2}$`).MatchString(since) {
		t.Fatalf("contributor_since=%q err=%v", since, err)
	}
	var links []map[string]string
	if err := json.Unmarshal(items[0]["links"], &links); err != nil || len(links) != 1 || links[0]["url"] != "https://github.com/octocat" {
		t.Fatalf("links=%v err=%v", links, err)
	}
	type publicItem struct {
		DisplayName string   `json:"display_name"`
		Roles       []string `json:"roles"`
	}
	var typed []publicItem
	if err := json.Unmarshal(page["items"], &typed); err != nil {
		t.Fatal(err)
	}
	if len(typed) != 2 || typed[0].DisplayName != "Ana" || len(typed[0].Roles) != 1 || typed[0].Roles[0] != "Moderação da comunidade" {
		t.Fatalf("roles=%+v", typed)
	}
	if typed[1].DisplayName != "Bia" || len(typed[1].Roles) != 0 {
		t.Fatalf("hidden roles=%+v", typed[1])
	}
	var next string
	if err := json.Unmarshal(page["next_cursor"], &next); err != nil || next == "" {
		t.Fatalf("cursor=%q err=%v", next, err)
	}
	secondReq := httptest.NewRequest(http.MethodGet, "/v1/contributors?limit=2&cursor="+url.QueryEscape(next), nil)
	secondReq.Header.Set("Origin", "https://landing.example")
	second := httptest.NewRecorder()
	router.ServeHTTP(second, secondReq)
	if second.Code != http.StatusOK || !strings.Contains(second.Body.String(), "Caio") || strings.Contains(second.Body.String(), "Ana") {
		t.Fatalf("page2 status=%d body=%s", second.Code, second.Body.String())
	}
	_ = thirdID

	evil := httptest.NewRequest(http.MethodGet, "/v1/contributors", nil)
	evil.Header.Set("Origin", "https://evil.example")
	evilW := httptest.NewRecorder()
	router.ServeHTTP(evilW, evil)
	if evilW.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("disallowed origin received CORS")
	}
	other := httptest.NewRequest(http.MethodGet, "/v1/stations", nil)
	other.Header.Set("Origin", "https://landing.example")
	otherW := httptest.NewRecorder()
	router.ServeHTTP(otherW, other)
	if otherW.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("CORS leaked to another route")
	}
}
