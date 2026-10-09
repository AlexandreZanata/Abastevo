//go:build integration

package adapters

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	dbmigrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
)

type integClock struct {
	mu  sync.Mutex
	now int64
}

func (c *integClock) NowUnix() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *integClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now += int64(d / time.Second)
}

type integMail struct {
	mu   sync.Mutex
	sent int
}

func (m *integMail) SendCode(_ context.Context, _, _ string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent++
	return nil
}

func (m *integMail) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sent
}

func testUUID(n int) string {
	return fmt.Sprintf("aaaaaaaa-1111-4111-8111-%012d", n)
}

func freshService(t *testing.T) (*application.Service, *integClock, *integMail) {
	t.Helper()
	adminDSN := os.Getenv("ANPFUEL_TEST_DATABASE_URL")
	if adminDSN == "" {
		adminDSN = "postgres://anpfuel:anpfuel@127.0.0.1:5434/anpfuel?sslmode=disable"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatalf("integration database unreachable at %s: %v (start it: docker compose -f infra/compose.dev.yml up -d db)", adminDSN, err)
	}
	defer admin.Close(ctx)
	name := fmt.Sprintf("account_test_%d", time.Now().UnixNano())
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
	applied, err := migrate.Apply(ctx, dsn, dbmigrations.Files)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	found := false
	for _, v := range applied {
		if v == "000020" {
			found = true
		}
	}
	if !found {
		t.Fatalf("accounts migration missing from %v", applied)
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)

	clock := &integClock{now: 1_700_000_000}
	mail := &integMail{}
	codes := []string{"482916", "111111", "222222", "333333", "444444", "555555", "666666"}
	var mu sync.Mutex
	ci, gi := 0, 0
	nextCode := func() (string, error) {
		mu.Lock()
		defer mu.Unlock()
		c := codes[ci%len(codes)]
		ci++
		return c, nil
	}
	nextID := func() (string, error) {
		mu.Lock()
		defer mu.Unlock()
		gi++
		return testUUID(gi), nil
	}
	var tn int
	nextToken := func() (string, error) {
		mu.Lock()
		defer mu.Unlock()
		tn++
		return fmt.Sprintf("tok-%d", tn), nil
	}
	var an int
	nextAlias := func() (string, error) {
		mu.Lock()
		defer mu.Unlock()
		an++
		return fmt.Sprintf("alias-%d", an), nil
	}
	svc := &application.Service{
		Clock:    clock,
		Hasher:   domain.SHA256Hasher{},
		Mail:     mail,
		Store:    NewPGStore(pool),
		CodeGen:  nextCode,
		TokenGen: nextToken,
		AliasGen: nextAlias,
		KeyGen:   domain.GenerateAccountKey,
		IDGen:    nextID,
	}
	return svc, clock, mail
}

func pgRequest(t *testing.T, svc *application.Service, address string) {
	t.Helper()
	if err := svc.RequestCode(context.Background(), address); err != nil {
		t.Fatalf("RequestCode: %v", err)
	}
}

func TestPGValidConsumeRoundTrip(t *testing.T) {
	svc, _, _ := freshService(t)
	ctx := context.Background()
	pgRequest(t, svc, "case-01@example.invalid")
	got, err := svc.ConsumeCode(ctx, "case-01@example.invalid", "482916")
	if err != nil {
		t.Fatalf("ConsumeCode: %v", err)
	}
	if !got.Created || got.Account.Status != domain.StatusActive || got.Session.AccessToken == "" {
		t.Errorf("signup must create an active sessioned account: %+v", got)
	}
	pgRequest(t, svc, "case-01@example.invalid")
	again, err := svc.ConsumeCode(ctx, "case-01@example.invalid", "111111")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if again.Created || again.Account.ID != got.Account.ID {
		t.Error("known address must log into the same account")
	}
}

func TestPGConcurrentConsumeAdmitsExactlyOne(t *testing.T) {
	svc, _, _ := freshService(t)
	ctx := context.Background()
	pgRequest(t, svc, "race@example.invalid")
	const racers = 16
	var wg sync.WaitGroup
	errs := make([]error, racers)
	for i := 0; i < racers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = svc.ConsumeCode(ctx, "race@example.invalid", "482916")
		}(i)
	}
	wg.Wait()
	ok, consumed := 0, 0
	for _, err := range errs {
		switch {
		case err == nil:
			ok++
		case errors.Is(err, domain.ErrCodeConsumed):
			consumed++
		default:
			t.Errorf("unexpected consume error: %v", err)
		}
	}
	if ok != 1 || consumed != racers-1 {
		t.Errorf("concurrent consume: %d ok, %d consumed (want 1/%d)", ok, consumed, racers-1)
	}
}

func TestPGExpiryAttemptsEnumerationQuota(t *testing.T) {
	svc, clock, mail := freshService(t)
	ctx := context.Background()

	pgRequest(t, svc, "exp@example.invalid")
	clock.advance(601 * time.Second)
	if _, err := svc.ConsumeCode(ctx, "exp@example.invalid", "482916"); !errors.Is(err, domain.ErrCodeExpired) {
		t.Errorf("expired must fail closed, got %v", err)
	}

	pgRequest(t, svc, "lock@example.invalid")
	for i := 0; i < 4; i++ {
		if _, err := svc.ConsumeCode(ctx, "lock@example.invalid", "000000"); !errors.Is(err, domain.ErrCodeUnknown) {
			t.Fatalf("wrong guess must be unknown, got %v", err)
		}
	}
	if _, err := svc.ConsumeCode(ctx, "lock@example.invalid", "000000"); !errors.Is(err, domain.ErrCodeAttemptsExhausted) {
		t.Errorf("5th guess must lock, got %v", err)
	}

	pgRequest(t, svc, "known@example.invalid")
	if _, err := svc.ConsumeCode(ctx, "known@example.invalid", "000000"); !errors.Is(err, domain.ErrCodeUnknown) {
		t.Errorf("wrong code must be unknown")
	}
	if _, err := svc.ConsumeCode(ctx, "nobody@example.invalid", "000000"); !errors.Is(err, domain.ErrCodeUnknown) {
		t.Errorf("unknown address must share code-unknown")
	}

	addr := "quota@example.invalid"
	for i := 0; i < 5; i++ {
		if err := svc.RequestCode(ctx, addr); err != nil {
			t.Fatal(err)
		}
		clock.advance(61 * time.Second)
	}
	before := mail.count()
	if err := svc.RequestCode(ctx, addr); err != nil {
		t.Fatalf("over-quota must look successful, got %v", err)
	}
	if got := mail.count(); got != before {
		t.Error("over-quota request must not issue")
	}
}

func TestPGRefreshReuseRevokesFamily(t *testing.T) {
	svc, clock, _ := freshService(t)
	ctx := context.Background()
	pgRequest(t, svc, "case-07@example.invalid")
	auth, err := svc.ConsumeCode(ctx, "case-07@example.invalid", "482916")
	if err != nil {
		t.Fatal(err)
	}
	rotated, err := svc.Refresh(ctx, auth.Session.FamilyID, auth.Session.RefreshToken)
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if _, err := svc.Refresh(ctx, auth.Session.FamilyID, auth.Session.RefreshToken); !errors.Is(err, domain.ErrSessionReuse) {
		t.Errorf("superseded refresh must be reuse, got %v", err)
	}
	if _, err := svc.Refresh(ctx, auth.Session.FamilyID, rotated.RefreshToken); !errors.Is(err, domain.ErrSessionRevoked) {
		t.Errorf("reuse must revoke the family, got %v", err)
	}

	pgRequest(t, svc, "case-08@example.invalid")
	auth2, err := svc.ConsumeCode(ctx, "case-08@example.invalid", "111111")
	if err != nil {
		t.Fatal(err)
	}
	clock.advance(31 * 24 * time.Hour)
	if _, err := svc.Refresh(ctx, auth2.Session.FamilyID, auth2.Session.RefreshToken); !errors.Is(err, domain.ErrSessionExpired) {
		t.Errorf("31-day-old family must expire, got %v", err)
	}
}

func TestPGParallelSignupLinksWinner(t *testing.T) {
	svc, clock, _ := freshService(t)
	ctx := context.Background()
	if err := svc.RequestCode(ctx, "same@example.invalid"); err != nil {
		t.Fatal(err)
	}
	clock.advance(61 * time.Second)
	if err := svc.RequestCode(ctx, "same@example.invalid"); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make([]application.AuthResult, 2)
	errs := make([]error, 2)
	for i, code := range []string{"482916", "111111"} {
		wg.Add(1)
		go func(i int, code string) {
			defer wg.Done()
			results[i], errs[i] = svc.ConsumeCode(ctx, "same@example.invalid", code)
		}(i, code)
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatalf("parallel signup must succeed on both codes, got %v", err)
		}
	}
	if results[0].Account.ID != results[1].Account.ID {
		t.Error("parallel signups must link the same account")
	}
}

func TestPGReplaySupersededCodeIsConsumed(t *testing.T) {
	svc, clock, _ := freshService(t)
	ctx := context.Background()
	pgRequest(t, svc, "sup@example.invalid")
	if _, err := svc.ConsumeCode(ctx, "sup@example.invalid", "482916"); err != nil {
		t.Fatalf("first consume: %v", err)
	}
	clock.advance(61 * time.Second)
	pgRequest(t, svc, "sup@example.invalid")
	if _, err := svc.ConsumeCode(ctx, "sup@example.invalid", "482916"); !errors.Is(err, domain.ErrCodeConsumed) {
		t.Errorf("replayed first code must be code-consumed, got %v", err)
	}
	if _, err := svc.ConsumeCode(ctx, "sup@example.invalid", "111111"); err != nil {
		t.Errorf("live second code must consume, got %v", err)
	}
}
