//go:build integration

package http

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	communityadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/community/adapters"
	communityapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/community/application"
	identityadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/adapters"
	identityauth "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/adapters/auth"
	identitydomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/domain"
	platformjobs "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
)

var testSecrets = []byte("test-cursor-secret-32-bytes-xxxx")

func testDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("ANPFUEL_TEST_DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://anpfuel:anpfuel@127.0.0.1:5434/anpfuel?sslmode=disable"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("integration database unreachable at %s: %v (start it: docker compose -f infra/compose.dev.yml up -d db)", dsn, err)
	}
	defer conn.Close(ctx)
	return dsn
}

// fixture wires the full stack exactly like cmd/api: real auth, quota,
// idempotency, attribution, store and enqueue closures.
func fixture(t *testing.T) (chi.Router, *pgxpool.Pool, string) {
	t.Helper()
	adminDSN := testDSN(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatalf("admin connect: %v", err)
	}
	defer admin.Close(ctx)
	name := fmt.Sprintf("community_http_test_%d", time.Now().UnixNano())
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
	station := "d6c74c23-63db-4c24-a2e5-408cb23bad26"
	if _, err := pool.Exec(ctx, `INSERT INTO directory_stations (id, display_name) VALUES ($1, 'Posto T')`, station); err != nil {
		t.Fatalf("station: %v", err)
	}
	store := communityadapters.NewStore(pool)
	registrar := identityadapters.NewRegistrar(pool)
	runner := identityadapters.NewRunner(pool)
	limiter := identityadapters.NewLimiter(pool, identitydomain.DefaultQuotaPolicy())
	verifier := &identityauth.Verifier{Pool: pool, Authority: "api.example.invalid"}
	newUUID := func() (string, error) {
		var b [16]byte
		if _, err := rand.Read(b[:]); err != nil {
			return "", err
		}
		b[6] = b[6]&0x0f | 0x40
		b[8] = b[8]&0x3f | 0x80
		hexed := fmt.Sprintf("%x", b)
		return hexed[0:8] + "-" + hexed[8:12] + "-" + hexed[12:16] + "-" + hexed[16:20] + "-" + hexed[20:32], nil
	}
	ports := communityapp.Ports{
		Clock: time.Now,
		NewID: newUUID,
		Attribution: func(ctx context.Context, contributorID string) (string, error) {
			return registrar.AttributionToken(ctx, contributorID)
		},
		CheckQuota: func(ctx context.Context, subject, operation string) (time.Duration, error) {
			return limiter.Check(ctx, subject, operation)
		},
		Idempotent: func(ctx context.Context, key communityapp.IdempotencyKey, body []byte, run func(ctx context.Context) (communityapp.Outcome, error)) (communityapp.Outcome, error) {
			out, err := runner.Do(ctx, identitydomain.IdempotencyKey{
				ContributorID: key.ContributorID, Method: key.Method, Route: key.Route, Key: key.Key,
			}, body, func(ctx context.Context, tx pgx.Tx) (int, []byte, error) {
				o, err := run(ctx)
				return o.StatusCode, o.Body, err
			})
			if err != nil {
				if errors.Is(err, identitydomain.ErrIdempotentConflict) {
					return communityapp.Outcome{}, communityapp.ErrConflict
				}
				return communityapp.Outcome{}, err
			}
			return communityapp.Outcome{StatusCode: out.StatusCode, Body: out.Response, Replayed: out.Replayed}, nil
		},
		EnqueueJob: func(ctx context.Context, tx pgx.Tx, kind string, payload []byte, dedupe string) error {
			_, err := platformjobs.Enqueue(ctx, tx, kind, payload, dedupe, 5, time.Time{})
			return err
		},
		Store: store,
	}
	r := chi.NewRouter()
	Handler{
		Authenticate: func(req *http.Request) (communityapp.Caller, error) {
			id, err := verifier.Verify(req.Context(), req)
			if err != nil {
				return communityapp.Caller{}, err
			}
			token, err := registrar.AttributionToken(req.Context(), id.ContributorID)
			if err != nil {
				return communityapp.Caller{}, err
			}
			return communityapp.Caller{ContributorID: id.ContributorID, Fingerprint: id.Fingerprint, KeyID: id.KeyID, Token: token}, nil
		},
		Submit: func(ctx context.Context, caller communityapp.Caller, key string, dto communityapp.SubmitDTO, body []byte) (communityapp.SubmitResult, bool, error) {
			return submitWithKernel(ctx, ports, caller, key, dto, body)
		},
		Status: func(ctx context.Context, caller communityapp.Caller, id string) (communityapp.StatusResult, error) {
			return communityapp.Status(ctx, ports, caller, id)
		},
		History: func(ctx context.Context, caller communityapp.Caller, limit int, after time.Time, afterID string, hasCursor bool) ([]communityapp.HistoryItem, string, error) {
			return communityapp.History(ctx, ports, caller, limit, after, afterID, hasCursor)
		},
		Secrets: testSecrets,
	}.RegisterRoutes(r)
	return r, pool, station
}

func submitWithKernel(ctx context.Context, ports communityapp.Ports, caller communityapp.Caller, key string, dto communityapp.SubmitDTO, body []byte) (communityapp.SubmitResult, bool, error) {
	res, err := communityapp.Submit(ctx, ports, caller, "POST", "/v1/observations", key, body, dto)
	if err != nil {
		return communityapp.SubmitResult{}, false, err
	}
	return res, res.Replayed, nil
}

// callerKey enrolls a fresh contributor and returns signing material.
type callerKey struct {
	priv *ecdsa.PrivateKey
	fp   string
}

func enrollCaller(t *testing.T, pool *pgxpool.Pool) *callerKey {
	t.Helper()
	ctx := context.Background()
	reg := identityadapters.NewRegistrar(pool)
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := priv.PublicKey.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	enc := base64.RawURLEncoding.EncodeToString
	x, y := enc(raw[1:33]), enc(raw[33:65])
	thumb := `{"crv":"P-256","kty":"EC","x":"` + x + `","y":"` + y + `"}`
	sum := sha256.Sum256([]byte(thumb))
	fp := fmt.Sprintf("fp:%x", sum)
	ch, err := reg.IssueChallenge(ctx, fp, identitydomain.PurposeRegister)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	lines := []string{
		`"@method": POST`,
		`"@authority": test.invalid`,
		`"@path": /v1/contributors`,
		`"@query": `,
		fmt.Sprintf(`"created": %d`, now.Add(-time.Minute).Unix()),
		fmt.Sprintf(`"expires": %d`, now.Add(4*time.Minute).Unix()),
		`"keyid": "` + fp + `"`,
		`"nonce": "` + ch.Nonce + `"`,
	}
	joined := ""
	for i, l := range lines {
		if i > 0 {
			joined += "\n"
		}
		joined += l
	}
	digest := sha512.Sum512([]byte(joined))
	rr, ss, err := ecdsa.Sign(rand.Reader, priv, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	sig := append(rr.FillBytes(make([]byte, 32)), ss.FillBytes(make([]byte, 32))...)
	if _, err := reg.Register(ctx, identitydomain.RegistrationRequest{
		JWKX: x, JWKY: y, Challenge: ch,
		BaseLines: lines, Signature: base64.RawURLEncoding.EncodeToString(sig),
		VerifiedAt: now,
	}); err != nil {
		t.Fatalf("enroll: %v", err)
	}
	return &callerKey{priv: priv, fp: fp}
}

func signedRequest(t *testing.T, pool *pgxpool.Pool, c *callerKey, method, target, body string) *http.Request {
	t.Helper()
	ctx := context.Background()
	reg := identityadapters.NewRegistrar(pool)
	ch, err := reg.IssueChallenge(ctx, c.fp, identitydomain.PurposeSign)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	u, err := url.Parse(target)
	if err != nil {
		t.Fatal(err)
	}
	lines := []string{
		`"@method": ` + method,
		`"@authority": api.example.invalid`,
		`"@path": ` + u.EscapedPath(),
		`"@query": ` + u.RawQuery,
	}
	ct := ""
	if body != "" {
		ct = "application/json"
		sum := sha512.Sum512([]byte(body))
		lines = append(lines,
			`"content-type": `+ct,
			`"content-digest": "sha-512=:`+base64.StdEncoding.EncodeToString(sum[:])+`:"`,
		)
	}
	lines = append(lines,
		fmt.Sprintf(`"created": %d`, now.Add(-time.Minute).Unix()),
		fmt.Sprintf(`"expires": %d`, now.Add(4*time.Minute).Unix()),
		`"keyid": "`+c.fp+`"`,
		`"nonce": "`+ch.Nonce+`"`,
	)
	joined := ""
	for i, l := range lines {
		if i > 0 {
			joined += "\n"
		}
		joined += l
	}
	digest := sha512.Sum512([]byte(joined))
	r, s, err := ecdsa.Sign(rand.Reader, c.priv, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	sig := append(r.FillBytes(make([]byte, 32)), s.FillBytes(make([]byte, 32))...)
	var reader io.Reader
	if body != "" {
		reader = bytes.NewBufferString(body)
	}
	req := httptest.NewRequest(method, target, reader)
	if body != "" {
		req.Header.Set("Content-Type", ct)
	}
	req.Header.Set("Signature", base64.RawURLEncoding.EncodeToString(sig))
	req.Header.Set("Signature-Created", fmt.Sprint(now.Add(-time.Minute).Unix()))
	req.Header.Set("Signature-Expires", fmt.Sprint(now.Add(4*time.Minute).Unix()))
	req.Header.Set("Signature-Keyid", c.fp)
	req.Header.Set("Signature-Nonce", ch.Nonce)
	return req
}

func doRequest(t *testing.T, r chi.Router, req *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func submitBody(station, submission string) string {
	return fmt.Sprintf(`{"client_submission_id":%q,"station_id":%q,"fuel_product":"GASOLINE_REGULAR",`+
		`"price":{"amount_milli_brl":5999,"currency":"BRL","unit":"L"},`+
		`"condition":{"kind":"STANDARD","qualifier_id":null}}`, submission, station)
}

func TestSubmitEnvelope(t *testing.T) {
	r, pool, station := fixture(t)
	c := enrollCaller(t, pool)
	req := signedRequest(t, pool, c, http.MethodPost, "/v1/observations", submitBody(station, "sub-1"))
	req.Header.Set("Idempotency-Key", "cmd-1")
	w := doRequest(t, r, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d: %s", w.Code, w.Body.String())
	}
	if cc := w.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("cache = %q", cc)
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["validation_state"] != "RECEIVED" {
		t.Errorf("state = %v (must never claim publication)", body["validation_state"])
	}
	for _, k := range []string{"id", "received_at", "status_url"} {
		if body[k] == nil || body[k] == "" {
			t.Errorf("receipt missing %s: %v", k, body)
		}
	}
	if !strings.HasSuffix(body["status_url"].(string), body["id"].(string)) {
		t.Errorf("status_url = %v", body["status_url"])
	}
	// Retry with the same key and body converges on the same id.
	req2 := signedRequest(t, pool, c, http.MethodPost, "/v1/observations", submitBody(station, "sub-1"))
	req2.Header.Set("Idempotency-Key", "cmd-1")
	w2 := doRequest(t, r, req2)
	if w2.Code != http.StatusCreated {
		t.Fatalf("retry = %d: %s", w2.Code, w2.Body.String())
	}
	var b2 map[string]any
	_ = json.Unmarshal(w2.Body.Bytes(), &b2)
	if b2["id"] != body["id"] {
		t.Errorf("retry id = %v, want %v", b2["id"], body["id"])
	}
	// Changed body under the same key conflicts.
	req3 := signedRequest(t, pool, c, http.MethodPost, "/v1/observations", submitBody(station, "sub-2"))
	req3.Header.Set("Idempotency-Key", "cmd-1")
	w3 := doRequest(t, r, req3)
	if w3.Code != http.StatusConflict {
		t.Fatalf("conflict = %d: %s", w3.Code, w3.Body.String())
	}
}

func TestSubmitValidation(t *testing.T) {
	r, pool, station := fixture(t)
	c := enrollCaller(t, pool)
	// Missing key, malformed bodies, unknown fields, float amount.
	cases := []struct {
		name   string
		key    string
		body   string
		status int
	}{
		{"missing key", "", submitBody(station, "s1"), http.StatusBadRequest},
		{"unknown field", "k1", `{"client_submission_id":"s1","bogus":1,"station_id":"` + station + `","fuel_product":"GASOLINE_REGULAR","price":{"amount_milli_brl":5999,"currency":"BRL","unit":"L"},"condition":{"kind":"STANDARD"}}`, http.StatusBadRequest},
		{"float amount", "k2", strings.Replace(submitBody(station, "s1"), "5999", "5999.0", 1), http.StatusBadRequest},
		{"bad enum", "k3", strings.Replace(submitBody(station, "s1"), "GASOLINE_REGULAR", "JET_A1", 1), http.StatusBadRequest},
		{"bad uuid", "k4", strings.Replace(submitBody(station, "s1"), station, "nope", 1), http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := signedRequest(t, pool, c, http.MethodPost, "/v1/observations", tc.body)
			if tc.key != "" {
				req.Header.Set("Idempotency-Key", tc.key)
			}
			w := doRequest(t, r, req)
			if w.Code != tc.status {
				t.Errorf("status = %d, want %d: %s", w.Code, tc.status, w.Body.String())
			}
		})
	}
	// Unauthenticated request fails closed.
	unsigned := httptest.NewRequest(http.MethodPost, "/v1/observations", strings.NewReader(submitBody(station, "s9")))
	unsigned.Header.Set("Idempotency-Key", "k9")
	if w := doRequest(t, r, unsigned); w.Code != http.StatusUnauthorized {
		t.Errorf("unsigned = %d", w.Code)
	}
}

func TestStatusAndHistory(t *testing.T) {
	r, pool, station := fixture(t)
	c := enrollCaller(t, pool)
	other := enrollCaller(t, pool)
	post := func(sub, key string) string {
		req := signedRequest(t, pool, c, http.MethodPost, "/v1/observations", submitBody(station, sub))
		req.Header.Set("Idempotency-Key", key)
		w := doRequest(t, r, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("submit %s = %d: %s", sub, w.Code, w.Body.String())
		}
		var body map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &body)
		return body["id"].(string)
	}
	id1 := post("sub-1", "cmd-1")
	id2 := post("sub-2", "cmd-2")
	_ = id2
	get := func(caller *callerKey, target string) *httptest.ResponseRecorder {
		return doRequest(t, r, signedRequest(t, pool, caller, http.MethodGet, target, ""))
	}
	w := get(c, "/v1/observations/"+id1)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", w.Code, w.Body.String())
	}
	if cc := w.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("cache = %q", cc)
	}
	var body map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if body["validation_state"] != "RECEIVED" {
		t.Errorf("state = %v", body["validation_state"])
	}
	if obs, _ := body["observation"].(map[string]any); obs["id"] != id1 {
		t.Errorf("observation = %v", body["observation"])
	}
	// Foreign and unknown observations share one 404 (no oracle).
	if w := get(other, "/v1/observations/"+id1); w.Code != http.StatusNotFound {
		t.Errorf("foreign = %d", w.Code)
	}
	if w := get(c, "/v1/observations/d6c74c23-63db-4c24-a2e5-408cb23bad26"); w.Code != http.StatusNotFound {
		t.Errorf("unknown = %d", w.Code)
	}
	if w := get(c, "/v1/observations/not-a-uuid"); w.Code != http.StatusBadRequest && w.Code != http.StatusNotFound {
		t.Errorf("malformed = %d", w.Code)
	}
	// History walk with sealed cursors.
	w = get(c, "/v1/contributors/me/observations?limit=1")
	if w.Code != http.StatusOK {
		t.Fatalf("history = %d: %s", w.Code, w.Body.String())
	}
	var h1 map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &h1)
	if len(h1["items"].([]any)) != 1 {
		t.Fatalf("page one = %v", h1)
	}
	next, _ := h1["next_cursor"].(string)
	if next == "" {
		t.Fatal("missing next cursor")
	}
	w = get(c, "/v1/contributors/me/observations?limit=1&cursor="+url.QueryEscape(next))
	var h2 map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &h2)
	if len(h2["items"].([]any)) != 1 {
		t.Fatalf("page two = %v", h2)
	}
	if h1["items"].([]any)[0].(map[string]any)["id"] == h2["items"].([]any)[0].(map[string]any)["id"] {
		t.Error("cursor did not advance")
	}
	// Tampered cursor fails closed.
	w = get(c, "/v1/contributors/me/observations?limit=1&cursor="+url.QueryEscape(next+"x"))
	if w.Code != http.StatusBadRequest {
		t.Errorf("tampered = %d", w.Code)
	}
	// History never leaks across contributors.
	w = get(other, "/v1/contributors/me/observations?limit=10")
	var ho map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &ho)
	if len(ho["items"].([]any)) != 0 {
		t.Errorf("cross-contributor leak: %v", ho)
	}
}
