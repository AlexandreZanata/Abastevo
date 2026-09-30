package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
)

type fakeClock struct {
	mu  sync.Mutex
	now int64
}

func (f *fakeClock) NowUnix() int64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.now
}

func (f *fakeClock) advance(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.now += int64(d / time.Second)
}

type stubMail struct {
	mu   sync.Mutex
	sent []string
	fail bool
}

func (m *stubMail) SendCode(_ context.Context, address, code string) error {
	if m.fail {
		return errors.New("account: mail delivery failed")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent = append(m.sent, address+"\x00"+code)
	return nil
}

func (m *stubMail) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.sent)
}

func counter(prefix string) func() (string, error) {
	var n int
	var mu sync.Mutex
	return func() (string, error) {
		mu.Lock()
		defer mu.Unlock()
		n++
		return fmt.Sprintf("%s-%d", prefix, n), nil
	}
}

func testService() (*Service, *fakeClock, *stubMail) {
	clock := &fakeClock{now: 1_700_000_000}
	mail := &stubMail{}
	codes := []string{"482916", "111111", "222222", "333333", "444444", "555555", "666666"}
	var ci int
	var mu sync.Mutex
	svc := &Service{
		Clock:  clock,
		Hasher: domain.SHA256Hasher{},
		Mail:   mail,
		Store:  NewMemStore(),
		CodeGen: func() (string, error) {
			mu.Lock()
			defer mu.Unlock()
			c := codes[ci%len(codes)]
			ci++
			return c, nil
		},
		TokenGen: counter("tok"),
		AliasGen: counter("alias"),
		IDGen:    counter("id"),
	}
	return svc, clock, mail
}

func request(t *testing.T, svc *Service, address string) {
	t.Helper()
	if err := svc.RequestCode(context.Background(), address); err != nil {
		t.Fatalf("RequestCode: %v", err)
	}
}

func TestValidConsumeCreatesAccountAndSession(t *testing.T) {
	svc, _, _ := testService()
	ctx := context.Background()
	request(t, svc, "case-01@example.invalid")

	got, err := svc.ConsumeCode(ctx, "case-01@example.invalid", "482916")
	if err != nil {
		t.Fatalf("ConsumeCode: %v", err)
	}
	if !got.Created || got.Account.Status != domain.StatusActive {
		t.Errorf("signup must create an active account: %+v", got.Account)
	}
	if got.Session.AccessToken == "" || got.Session.RefreshToken == "" {
		t.Error("session must carry both tokens")
	}
	if !strings.HasPrefix(got.Account.Alias, "alias-") {
		t.Errorf("alias must be opaque, got %q", got.Account.Alias)
	}

	request(t, svc, "case-01@example.invalid")
	again, err := svc.ConsumeCode(ctx, "case-01@example.invalid", "111111")
	if err != nil {
		t.Fatalf("second login: %v", err)
	}
	if again.Created || again.Account.ID != got.Account.ID {
		t.Error("known address must log into the same account, not create one")
	}
}

func TestReplayExpiredAndAttempts(t *testing.T) {
	svc, clock, _ := testService()
	ctx := context.Background()
	request(t, svc, "case-02@example.invalid")

	if _, err := svc.ConsumeCode(ctx, "case-02@example.invalid", "482916"); err != nil {
		t.Fatalf("first consume: %v", err)
	}
	if _, err := svc.ConsumeCode(ctx, "case-02@example.invalid", "482916"); !errors.Is(err, domain.ErrCodeConsumed) {
		t.Errorf("replay must be code-consumed, got %v", err)
	}

	request(t, svc, "case-03@example.invalid")
	clock.advance(601 * time.Second)
	if _, err := svc.ConsumeCode(ctx, "case-03@example.invalid", "111111"); !errors.Is(err, domain.ErrCodeExpired) {
		t.Errorf("601s-old code must be expired, got %v", err)
	}

	request(t, svc, "case-04@example.invalid")
	for i := 0; i < 4; i++ {
		if _, err := svc.ConsumeCode(ctx, "case-04@example.invalid", "000000"); !errors.Is(err, domain.ErrCodeUnknown) {
			t.Fatalf("wrong guess %d must be code-unknown, got %v", i, err)
		}
	}
	if _, err := svc.ConsumeCode(ctx, "case-04@example.invalid", "000000"); !errors.Is(err, domain.ErrCodeAttemptsExhausted) {
		t.Errorf("5th wrong guess must lock, got %v", err)
	}
	if _, err := svc.ConsumeCode(ctx, "case-04@example.invalid", "111111"); !errors.Is(err, domain.ErrCodeAttemptsExhausted) {
		t.Errorf("right code after lock must still fail, got %v", err)
	}
}

func TestEnumerationSafeResponses(t *testing.T) {
	svc, _, _ := testService()
	ctx := context.Background()
	request(t, svc, "known@example.invalid")

	wrong := func() error {
		_, err := svc.ConsumeCode(ctx, "known@example.invalid", "000000")
		return err
	}
	unknown := func() error {
		_, err := svc.ConsumeCode(ctx, "nobody@example.invalid", "000000")
		return err
	}
	if !errors.Is(wrong(), domain.ErrCodeUnknown) || !errors.Is(unknown(), domain.ErrCodeUnknown) {
		t.Errorf("wrong code (%v) and unknown address (%v) must share code-unknown", wrong(), unknown())
	}
	if err := svc.RequestCode(ctx, "nobody@example.invalid"); err != nil {
		t.Errorf("request for unknown address must look successful, got %v", err)
	}
}

func TestResendCooldownAndHourlyQuota(t *testing.T) {
	svc, clock, mail := testService()
	ctx := context.Background()
	addr := "case-06@example.invalid"

	if err := svc.RequestCode(ctx, addr); err != nil {
		t.Fatal(err)
	}
	if err := svc.RequestCode(ctx, addr); err != nil {
		t.Fatal(err)
	}
	if got := mail.count(); got != 1 {
		t.Fatalf("cooldown resend must not issue, mails = %d", got)
	}
	clock.advance(61 * time.Second)
	for i := 0; i < 4; i++ {
		if err := svc.RequestCode(ctx, addr); err != nil {
			t.Fatal(err)
		}
		clock.advance(61 * time.Second)
	}
	if got := mail.count(); got != 5 {
		t.Fatalf("hourly quota is 5, mails = %d", got)
	}
	if err := svc.RequestCode(ctx, addr); err != nil {
		t.Fatalf("over-quota request must still look successful, got %v", err)
	}
	if got := mail.count(); got != 5 {
		t.Errorf("over-quota request must not issue, mails = %d", got)
	}
}

func TestRefreshRotationAndReuseRevokesFamily(t *testing.T) {
	svc, clock, _ := testService()
	ctx := context.Background()
	request(t, svc, "case-07@example.invalid")
	auth, err := svc.ConsumeCode(ctx, "case-07@example.invalid", "482916")
	if err != nil {
		t.Fatal(err)
	}

	rotated, err := svc.Refresh(ctx, auth.Session.FamilyID, auth.Session.RefreshToken)
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if rotated.RefreshToken == auth.Session.RefreshToken || rotated.AccessToken == auth.Session.AccessToken {
		t.Error("rotation must mint fresh tokens")
	}
	if _, err := svc.Refresh(ctx, auth.Session.FamilyID, auth.Session.RefreshToken); !errors.Is(err, domain.ErrSessionReuse) {
		t.Errorf("superseded refresh must be reuse, got %v", err)
	}
	if _, err := svc.Refresh(ctx, auth.Session.FamilyID, rotated.RefreshToken); !errors.Is(err, domain.ErrSessionRevoked) {
		t.Errorf("reuse must revoke the family, got %v", err)
	}
	if _, err := svc.ValidateAccess(ctx, auth.Session.FamilyID, rotated.AccessToken); !errors.Is(err, domain.ErrSessionRevoked) {
		t.Errorf("revoked family must fail access, got %v", err)
	}

	request(t, svc, "case-08@example.invalid")
	auth2, err := svc.ConsumeCode(ctx, "case-08@example.invalid", "111111")
	if err != nil {
		t.Fatal(err)
	}
	clock.advance(31 * 24 * time.Hour)
	if _, err := svc.Refresh(ctx, auth2.Session.FamilyID, auth2.Session.RefreshToken); !errors.Is(err, domain.ErrSessionExpired) {
		t.Errorf("31-day-old live family must be absolutely expired, got %v", err)
	}
}

func TestAccessExpiryAndRevokeAll(t *testing.T) {
	svc, clock, _ := testService()
	ctx := context.Background()
	request(t, svc, "case-09@example.invalid")
	auth, err := svc.ConsumeCode(ctx, "case-09@example.invalid", "482916")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ValidateAccess(ctx, auth.Session.FamilyID, auth.Session.AccessToken); err != nil {
		t.Errorf("live access must validate: %v", err)
	}
	if _, err := svc.ValidateAccess(ctx, auth.Session.FamilyID, "tok-999"); !errors.Is(err, domain.ErrCodeUnknown) {
		t.Errorf("forged access must be unknown, got %v", err)
	}
	clock.advance(901 * time.Second)
	if _, err := svc.ValidateAccess(ctx, auth.Session.FamilyID, auth.Session.AccessToken); !errors.Is(err, domain.ErrSessionExpired) {
		t.Errorf("901s-old access must be expired, got %v", err)
	}
	if err := svc.RevokeAll(ctx, auth.Account.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Refresh(ctx, auth.Session.FamilyID, auth.Session.RefreshToken); !errors.Is(err, domain.ErrSessionRevoked) {
		t.Errorf("revoked family must fail refresh, got %v", err)
	}
}

func TestNoPlaintextSecretsAtRest(t *testing.T) {
	svc, _, _ := testService()
	ctx := context.Background()
	request(t, svc, "case-10@example.invalid")
	auth, err := svc.ConsumeCode(ctx, "case-10@example.invalid", "482916")
	if err != nil {
		t.Fatal(err)
	}
	store := svc.Store.(*MemStore)
	store.mu.Lock()
	defer store.mu.Unlock()
	for _, rec := range store.codes {
		if rec.Hash == "482916" || strings.Contains(rec.Hash, "482916") {
			t.Error("code plaintext must never persist")
		}
	}
	fam := store.families[auth.Session.FamilyID]
	if fam.RefreshHash == auth.Session.RefreshToken || fam.AccessHash == auth.Session.AccessToken {
		t.Error("session token plaintext must never persist")
	}
	if fam.RefreshHash == "" || fam.AccessHash == "" {
		t.Error("token verifiers must persist as hashes")
	}
}

func TestConcurrentConsumeAdmitsExactlyOne(t *testing.T) {
	svc, _, _ := testService()
	ctx := context.Background()
	request(t, svc, "race@example.invalid")

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
	ok, consumed, other := 0, 0, 0
	for _, err := range errs {
		switch {
		case err == nil:
			ok++
		case errors.Is(err, domain.ErrCodeConsumed):
			consumed++
		default:
			other++
		}
	}
	if ok != 1 || consumed != racers-1 || other != 0 {
		t.Errorf("concurrent consume: %d ok, %d consumed, %d other (want 1/%d/0)", ok, consumed, other, racers-1)
	}
}

func TestReplaySupersededCodeIsConsumed(t *testing.T) {
	svc, clock, _ := testService()
	ctx := context.Background()
	request(t, svc, "sup@example.invalid")
	if _, err := svc.ConsumeCode(ctx, "sup@example.invalid", "482916"); err != nil {
		t.Fatalf("first consume: %v", err)
	}
	clock.advance(61 * time.Second)
	request(t, svc, "sup@example.invalid")
	if _, err := svc.ConsumeCode(ctx, "sup@example.invalid", "482916"); !errors.Is(err, domain.ErrCodeConsumed) {
		t.Errorf("replayed first code must be code-consumed, got %v", err)
	}
	if _, err := svc.ConsumeCode(ctx, "sup@example.invalid", "111111"); err != nil {
		t.Errorf("live second code must consume, got %v", err)
	}
}
