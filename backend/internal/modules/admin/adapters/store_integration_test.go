//go:build integration

package adapters

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	migrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	qadmin "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/admin"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/admin/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/admin/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
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
	name := fmt.Sprintf("admin_test_%d", time.Now().UnixNano())
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

func syntheticKey(fill byte) string {
	return domain.KeyPrefix + base64.RawURLEncoding.EncodeToString([]byte(strings.Repeat(string(fill), domain.KeyRandom)))
}

func hashIP(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func loginInput(key string, now time.Time, ip string) app.LoginInput {
	presented := domain.HashKey(key)
	return app.LoginInput{
		PresentedHash: presented,
		IPHash:        hashIP(ip),
		RequestID:     "synthetic-request",
		Now:           now,
		Verify: func() (string, bool) {
			matched, ok := domain.MatchKey(presented, []domain.ConfiguredKey{{
				Hash: domain.HashKey(syntheticKey('k')), Fingerprint: domain.Fingerprint(domain.HashKey(syntheticKey('k'))),
			}})
			return matched.Fingerprint, ok
		},
	}
}

func acceptedStoreInput(key string, now time.Time, ip string) app.LoginInput {
	in := loginInput(key, now, ip)
	in.Verify = func() (string, bool) {
		hash := domain.HashKey(key)
		return domain.Fingerprint(hash), true
	}
	return in
}

// TestLogin_ADM03_ADM05_IssuesSession covers a successful login, the stored
// session material and the same-session authentication path.
func TestLogin_ADM03_ADM05_IssuesSession(t *testing.T) {
	pool := freshDB(t)
	store := NewStore(pool)
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	key := syntheticKey('a')
	result, err := store.Login(context.Background(), acceptedStoreInput(key, now, "198.51.100.1"))
	if err != nil {
		t.Fatal(err)
	}
	if !domain.ValidToken(result.Token) || !domain.ValidToken(result.CSRFToken) || result.Token == result.CSRFToken {
		t.Fatal("invalid session material")
	}
	if !result.ExpiresAt.Equal(now.Add(domain.AbsoluteLifetime)) {
		t.Fatal("absolute lifetime mismatch")
	}
	session, err := store.Authenticate(context.Background(), result.Token, now.Add(time.Minute))
	if err != nil || session.ID == "" || session.KeyFingerprint != result.Fingerprint {
		t.Fatalf("session=%+v err=%v", session, err)
	}
	var stored string
	if err := pool.QueryRow(context.Background(), "SELECT token_hash FROM admin_sessions").Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored == result.Token || strings.Contains(stored, result.Token) {
		t.Fatal("raw session token persisted")
	}
	var audits int
	if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM admin_audit_events WHERE action='admin.login'").Scan(&audits); err != nil || audits != 1 {
		t.Fatalf("login audit=%d err=%v", audits, err)
	}
}

// TestLogin_ADM04_RateLimitPerIP is S5: the sixth failure in one IP window is
// refused with a bounded Retry-After.
func TestLogin_ADM04_RateLimitPerIP(t *testing.T) {
	pool := freshDB(t)
	store := NewStore(pool)
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	wrong := syntheticKey('x')
	for i := 0; i < 5; i++ {
		if _, err := store.Login(context.Background(), loginInput(wrong, now, "198.51.100.2")); !errors.Is(err, domain.ErrDenied) {
			t.Fatalf("failure %d err=%v", i+1, err)
		}
	}
	_, err := store.Login(context.Background(), loginInput(wrong, now.Add(time.Second), "198.51.100.2"))
	var limited *domain.RateLimitedError
	if !errors.As(err, &limited) || limited.RetryAfter <= 0 || limited.RetryAfter > ipWindow {
		t.Fatalf("sixth failure err=%v retry=%v", err, limited)
	}
	var failures int
	if err := pool.QueryRow(context.Background(), "SELECT failures FROM admin_login_attempts WHERE scope='ip' AND subject=$1", hashIP("198.51.100.2")).Scan(&failures); err != nil || failures != 5 {
		t.Fatalf("ip failures=%d err=%v", failures, err)
	}
}

// TestLogin_ADM04_GlobalRateLimit is S6: after 50 failures the global block
// refuses even a correct key.
func TestLogin_ADM04_GlobalRateLimit(t *testing.T) {
	pool := freshDB(t)
	store := NewStore(pool)
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	wrong := syntheticKey('x')
	for i := 0; i < 50; i++ {
		ip := fmt.Sprintf("198.51.100.%d", i+10)
		if _, err := store.Login(context.Background(), loginInput(wrong, now, ip)); !errors.Is(err, domain.ErrDenied) {
			t.Fatalf("failure %d err=%v", i+1, err)
		}
	}
	correct := syntheticKey('c')
	_, err := store.Login(context.Background(), acceptedStoreInput(correct, now.Add(time.Second), "203.0.113.50"))
	var limited *domain.RateLimitedError
	if !errors.As(err, &limited) || limited.RetryAfter <= 0 || limited.RetryAfter > globalBlockDuration {
		t.Fatalf("correct key not blocked err=%v retry=%v", err, limited)
	}
}

// TestLogin_ADM04_IPWindowSlides: five failures just before a quarter-hour
// boundary still block right after it; the limit is a trailing 15 minutes.
func TestLogin_ADM04_IPWindowSlides(t *testing.T) {
	pool := freshDB(t)
	store := NewStore(pool)
	first := time.Date(2026, 10, 27, 12, 14, 59, 0, time.UTC)
	wrong := syntheticKey('x')
	for i := 0; i < 5; i++ {
		if _, err := store.Login(context.Background(), loginInput(wrong, first, "198.51.100.3")); !errors.Is(err, domain.ErrDenied) {
			t.Fatalf("failure %d err=%v", i+1, err)
		}
	}
	for _, at := range []time.Time{first.Add(time.Second), first.Add(ipWindow - time.Second)} {
		_, err := store.Login(context.Background(), loginInput(wrong, at, "198.51.100.3"))
		var limited *domain.RateLimitedError
		if !errors.As(err, &limited) || limited.RetryAfter <= 0 || limited.RetryAfter > ipWindow {
			t.Fatalf("at %s err=%v retry=%v", at, err, limited)
		}
	}
	if _, err := store.Login(context.Background(), loginInput(wrong, first.Add(ipWindow), "198.51.100.3")); !errors.Is(err, domain.ErrDenied) {
		t.Fatalf("after the trailing window err=%v", err)
	}
}

// TestLogin_ADM04_GlobalBlockSurvivesHourBoundary: 50 failures at 10:58
// keep every login refused at 11:00 and 11:12; the block is not tied to a
// clock-hour row and failures are counted over a trailing hour.
func TestLogin_ADM04_GlobalBlockSurvivesHourBoundary(t *testing.T) {
	pool := freshDB(t)
	store := NewStore(pool)
	at := time.Date(2026, 10, 27, 10, 58, 0, 0, time.UTC)
	wrong := syntheticKey('x')
	for i := 0; i < 49; i++ {
		ip := fmt.Sprintf("198.51.100.%d", i+10)
		if _, err := store.Login(context.Background(), loginInput(wrong, at, ip)); !errors.Is(err, domain.ErrDenied) {
			t.Fatalf("failure %d err=%v", i+1, err)
		}
	}
	// Forty-nine failures before the hour plus one after it reach the limit.
	if _, err := store.Login(context.Background(), loginInput(wrong, at.Add(2*time.Minute), "198.51.100.200")); !errors.Is(err, domain.ErrDenied) {
		t.Fatalf("fiftieth failure err=%v", err)
	}
	correct := syntheticKey('c')
	for _, when := range []time.Time{at.Add(2*time.Minute + time.Second), at.Add(14 * time.Minute)} {
		_, err := store.Login(context.Background(), acceptedStoreInput(correct, when, "203.0.113.50"))
		var limited *domain.RateLimitedError
		if !errors.As(err, &limited) || limited.RetryAfter <= 0 || limited.RetryAfter > globalBlockDuration {
			t.Fatalf("at %s correct key not blocked err=%v retry=%v", when, err, limited)
		}
	}
	if _, err := store.Login(context.Background(), acceptedStoreInput(correct, at.Add(time.Hour+3*time.Minute), "203.0.113.50")); err != nil {
		t.Fatalf("after the trailing hour err=%v", err)
	}
}

// TestLogin_ADM04_ConcurrentFailuresExact is S7: serialized counters admit
// exactly the five real comparisons and never lose an update.
func TestLogin_ADM04_ConcurrentFailuresExact(t *testing.T) {
	pool := freshDB(t)
	store := NewStore(pool)
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	wrong := syntheticKey('x')
	const attempts = 20
	var wg sync.WaitGroup
	errs := make(chan error, attempts)
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := store.Login(context.Background(), loginInput(wrong, now, "198.51.100.9"))
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	denied, limited := 0, 0
	for err := range errs {
		switch {
		case errors.Is(err, domain.ErrDenied):
			denied++
		case errors.Is(err, domain.ErrRateLimited):
			limited++
		default:
			t.Fatalf("unexpected error %v", err)
		}
	}
	if denied != ipFailureLimit || limited != attempts-ipFailureLimit {
		t.Fatalf("denied=%d limited=%d", denied, limited)
	}
	var failures int
	if err := pool.QueryRow(context.Background(), "SELECT failures FROM admin_login_attempts WHERE scope='ip' AND subject=$1", hashIP("198.51.100.9")).Scan(&failures); err != nil || failures != ipFailureLimit {
		t.Fatalf("counter=%d err=%v", failures, err)
	}
}

// TestAuthenticate_ADM05_IdleAndAbsoluteExpiry is S8 with an injected clock.
func TestAuthenticate_ADM05_IdleAndAbsoluteExpiry(t *testing.T) {
	pool := freshDB(t)
	store := NewStore(pool)
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	key := syntheticKey('d')
	idle, err := store.Login(context.Background(), acceptedStoreInput(key, now, "198.51.100.20"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Authenticate(context.Background(), idle.Token, now.Add(domain.IdleTimeout+time.Second)); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("idle session accepted err=%v", err)
	}
	absolute, err := store.Login(context.Background(), acceptedStoreInput(key, now, "198.51.100.21"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Authenticate(context.Background(), absolute.Token, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Authenticate(context.Background(), absolute.Token, now.Add(domain.AbsoluteLifetime+time.Second)); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("absolute session accepted err=%v", err)
	}
}

// TestLogout_ADM05_ADM10_ReuseAndAudit is S9: logout revokes and is audited.
func TestLogout_ADM05_ADM10_ReuseAndAudit(t *testing.T) {
	pool := freshDB(t)
	store := NewStore(pool)
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	result, err := store.Login(context.Background(), acceptedStoreInput(syntheticKey('e'), now, "198.51.100.30"))
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.Authenticate(context.Background(), result.Token, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	mutation := app.Mutation{
		SessionID: session.ID, Action: "admin.logout", TargetType: "admin_session",
		TargetID: session.ID, Reason: "synthetic logout reason", Key: "11111111-1111-4111-8111-111111111111",
		RequestHash: strings.Repeat("a", 64), Now: now,
	}
	if _, err := store.Logout(context.Background(), session, mutation); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Authenticate(context.Background(), result.Token, now.Add(2*time.Minute)); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("reused cookie accepted err=%v", err)
	}
	var audits int
	if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM admin_audit_events WHERE action='admin.logout'").Scan(&audits); err != nil || audits != 1 {
		t.Fatalf("logout audit=%d err=%v", audits, err)
	}
}

// TestRevokeAll_ADM07 is S10: two logins stay independent until revoke-all.
func TestRevokeAll_ADM07(t *testing.T) {
	pool := freshDB(t)
	store := NewStore(pool)
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	key := syntheticKey('f')
	first, err := store.Login(context.Background(), acceptedStoreInput(key, now, "198.51.100.40"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.Login(context.Background(), acceptedStoreInput(key, now.Add(time.Second), "198.51.100.41"))
	if err != nil {
		t.Fatal(err)
	}
	if first.Token == second.Token {
		t.Fatal("login reused a token")
	}
	if _, err := store.Authenticate(context.Background(), first.Token, now.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Authenticate(context.Background(), second.Token, now.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	session, err := store.Authenticate(context.Background(), first.Token, now.Add(3*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	mutation := app.Mutation{
		SessionID: session.ID, Action: "admin.sessions.revoke-all", TargetType: "admin_session",
		Reason: "synthetic revoke-all reason", Key: "22222222-2222-4222-8222-222222222222",
		RequestHash: strings.Repeat("b", 64), Now: now,
	}
	if _, err := store.RevokeAll(context.Background(), session, mutation); err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{first.Token, second.Token} {
		if _, err := store.Authenticate(context.Background(), token, now.Add(4*time.Minute)); !errors.Is(err, domain.ErrUnauthorized) {
			t.Fatalf("session survived revoke-all err=%v", err)
		}
	}
}

// TestMutate_ADM12_Idempotency is S16: replay returns the stored result and a
// changed request conflicts in one transaction.
func TestMutate_ADM12_Idempotency(t *testing.T) {
	pool := freshDB(t)
	store := NewStore(pool)
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	result, err := store.Login(context.Background(), acceptedStoreInput(syntheticKey('g'), now, "198.51.100.50"))
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.Authenticate(context.Background(), result.Token, now)
	if err != nil {
		t.Fatal(err)
	}
	body := []byte(`{"ok":true}`)
	mutation := app.Mutation{
		SessionID: session.ID, Action: "admin.test", TargetType: "synthetic",
		Reason: "synthetic mutation reason", Key: "33333333-3333-4333-8333-333333333333",
		RequestHash: strings.Repeat("c", 64), Now: now,
	}
	run := func(context.Context, *qadmin.Queries) (app.MutationResult, error) {
		return app.MutationResult{Status: 200, Body: body}, nil
	}
	first, err := store.mutate(context.Background(), mutation, run)
	if err != nil || first.Replayed || string(first.Body) != string(body) {
		t.Fatalf("first=%+v err=%v", first, err)
	}
	replay, err := store.mutate(context.Background(), mutation, run)
	if err != nil || !replay.Replayed || string(replay.Body) != string(body) {
		t.Fatalf("replay=%+v err=%v", replay, err)
	}
	changed := mutation
	changed.RequestHash = strings.Repeat("d", 64)
	if _, err := store.mutate(context.Background(), changed, run); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("changed body err=%v", err)
	}
	var audits int
	if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM admin_audit_events WHERE action='admin.test'").Scan(&audits); err != nil || audits != 1 {
		t.Fatalf("mutation audit=%d err=%v", audits, err)
	}
}

// TestAudit_ADM10_AppendOnly is S17: the audit table refuses UPDATE/DELETE.
func TestAudit_ADM10_AppendOnly(t *testing.T) {
	pool := freshDB(t)
	store := NewStore(pool)
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	if _, err := store.Login(context.Background(), loginInput(syntheticKey('x'), now, "198.51.100.60")); !errors.Is(err, domain.ErrDenied) {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := pool.Exec(ctx, "UPDATE admin_audit_events SET action='tampered'"); err == nil {
		t.Fatal("audit UPDATE accepted")
	}
	if _, err := pool.Exec(ctx, "DELETE FROM admin_audit_events"); err == nil {
		t.Fatal("audit DELETE accepted")
	}
}
