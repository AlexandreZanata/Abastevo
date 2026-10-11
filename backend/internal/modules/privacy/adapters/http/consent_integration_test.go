//go:build integration

package http

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	dbmigrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	privacyadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/privacy/adapters"
	privacyapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/privacy/application"
	privacydomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/privacy/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
)

// CS09C: consent HTTP against real disposable PostGIS with the real
// application commands and guarded store. Authentication is injected
// (proof verification itself is cmd/api's unchanged verifier,
// exercised by the community signed-proof suites, not duplicated
// here): the fixture proves transport parsing, owner binding, error
// mapping and revision convergence end to end over the router.

type consentTerms struct{}

func (consentTerms) CurrentVersion() string { return privacydomain.TrainingTermsV1 }
func (consentTerms) CurrentDigest() string  { return "digest-v1" }

func consentTestDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("ANPFUEL_TEST_DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://anpfuel:anpfuel@127.0.0.1:5434/anpfuel?sslmode=disable"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("integration database unreachable at %s: %v", dsn, err)
	}
	defer conn.Close(ctx)
	return dsn
}

func consentFixture(t *testing.T) chi.Router {
	t.Helper()
	adminDSN := consentTestDSN(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatalf("admin connect: %v", err)
	}
	defer admin.Close(ctx)
	name := fmt.Sprintf("privacy_consent_http_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
		t.Fatalf("create database: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		admin, err := pgx.Connect(ctx, adminDSN)
		if err != nil {
			t.Errorf("admin connect for drop: %v", err)
			return
		}
		defer admin.Close(ctx)
		if _, err := admin.Exec(ctx, "DROP DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
			t.Errorf("drop database: %v", err)
		}
	})
	u, err := url.Parse(adminDSN)
	if err != nil {
		t.Fatalf("parse dsn: %v", err)
	}
	u.Path = "/" + name
	dsn := u.String()
	if _, err := migrate.Apply(ctx, dsn, dbmigrations.Files); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)
	ports := privacyapp.ConsentPorts{
		Clock:       time.Now,
		Attribution: func(_ context.Context, contributorID string) (string, error) { return "tok-" + contributorID, nil },
		Store:       privacyadapters.NewConsentStore(pool),
		Terms:       consentTerms{},
	}
	h := Handler{
		Authenticate: func(r *http.Request) (privacyapp.Caller, error) {
			if r.Header.Get("Authorization") == "" {
				return privacyapp.Caller{}, privacyapp.ErrUnauthorized
			}
			return privacyapp.Caller{ContributorID: "c1"}, nil
		},
		GrantConsent: func(ctx context.Context, caller privacyapp.Caller, dto privacyapp.GrantDTO) (privacyapp.ConsentResult, error) {
			return privacyapp.GrantConsent(ctx, ports, caller, dto)
		},
		RevokeConsent: func(ctx context.Context, caller privacyapp.Caller, dto privacyapp.RevokeDTO) (privacyapp.ConsentResult, error) {
			return privacyapp.RevokeConsent(ctx, ports, caller, dto)
		},
		ReadConsent: func(ctx context.Context, caller privacyapp.Caller) (privacyapp.ConsentView, error) {
			return privacyapp.ReadConsent(ctx, ports, caller)
		},
	}
	r := chi.NewRouter()
	h.RegisterRoutes(r)
	return r
}

func doConsent(t *testing.T, r chi.Router, method, target, body string, auth bool) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	if auth {
		req.Header.Set("Authorization", "Bearer test-proof")
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestConsentHTTPEndToEnd(t *testing.T) {
	r := consentFixture(t)
	grant := `{"terms_version":"training-terms-v1","terms_digest":"digest-v1","revision":1}`

	w := doConsent(t, r, http.MethodPost, "/v1/privacy/training-consent/grant", grant, true)
	if w.Code != http.StatusOK {
		t.Fatalf("grant = %d: %s", w.Code, w.Body.String())
	}
	var receipt map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &receipt); err != nil {
		t.Fatal(err)
	}
	if receipt["state"] != privacydomain.StateGranted || receipt["revision"] != 1.0 || receipt["replayed"] != false {
		t.Fatalf("receipt = %v", receipt)
	}

	// Lost response replays end to end: same revision converges.
	w = doConsent(t, r, http.MethodPost, "/v1/privacy/training-consent/grant", grant, true)
	if w.Code != http.StatusOK {
		t.Fatalf("replay = %d: %s", w.Code, w.Body.String())
	}
	if err := json.Unmarshal(w.Body.Bytes(), &receipt); err != nil {
		t.Fatal(err)
	}
	if receipt["replayed"] != true {
		t.Fatalf("replay receipt = %v", receipt)
	}

	// Owner read shows the grant; anonymous read refuses.
	w = doConsent(t, r, http.MethodGet, "/v1/privacy/training-consent", "", true)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", w.Code, w.Body.String())
	}
	var view map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if view["granted"] != true || view["terms_digest"] != "digest-v1" {
		t.Fatalf("view = %v", view)
	}
	w = doConsent(t, r, http.MethodGet, "/v1/privacy/training-consent", "", false)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous status = %d", w.Code)
	}

	// Revocation stops selection with the deletion signal.
	w = doConsent(t, r, http.MethodPost, "/v1/privacy/training-consent/revoke", `{"revision":2}`, true)
	if w.Code != http.StatusOK {
		t.Fatalf("revoke = %d: %s", w.Code, w.Body.String())
	}
	if err := json.Unmarshal(w.Body.Bytes(), &receipt); err != nil {
		t.Fatal(err)
	}
	if receipt["state"] != privacydomain.StateRevoked || receipt["delete_owned_training_assets"] != true {
		t.Fatalf("revoke receipt = %v", receipt)
	}

	// A stale offline grant queued at the revoked revision refuses.
	w = doConsent(t, r, http.MethodPost, "/v1/privacy/training-consent/grant",
		`{"terms_version":"training-terms-v1","terms_digest":"digest-v1","revision":2}`, true)
	if w.Code != http.StatusConflict {
		t.Fatalf("stale grant = %d: %s", w.Code, w.Body.String())
	}

	// Outdated terms refuse without touching the stored revocation.
	w = doConsent(t, r, http.MethodPost, "/v1/privacy/training-consent/grant",
		`{"terms_version":"training-terms-v0","terms_digest":"digest-v0","revision":3}`, true)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("old terms = %d: %s", w.Code, w.Body.String())
	}
	w = doConsent(t, r, http.MethodGet, "/v1/privacy/training-consent", "", true)
	if err := json.Unmarshal(w.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if view["granted"] != false && view["granted"] != nil {
		t.Fatalf("revocation resurrected: %v", view)
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("Cache-Control = %q", w.Header().Get("Cache-Control"))
	}
}
