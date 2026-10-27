package application

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/admin/domain"
	"github.com/jackc/pgx/v5"
)

type fakeStore struct {
	session Session
	login   LoginInput
	result  LoginResult
	mutated Mutation
}

func (f *fakeStore) Login(_ context.Context, in LoginInput) (LoginResult, error) {
	f.login = in
	if in.Verify == nil {
		return LoginResult{}, domain.ErrDenied
	}
	fingerprint, ok := in.Verify()
	if !ok {
		return LoginResult{}, domain.ErrDenied
	}
	f.result.Fingerprint = fingerprint
	return f.result, nil
}

func (f *fakeStore) Authenticate(context.Context, string, time.Time) (Session, error) {
	return f.session, nil
}

func (f *fakeStore) Logout(_ context.Context, _ Session, m Mutation) (MutationResult, error) {
	f.mutated = m
	return MutationResult{Status: 204}, nil
}

func (f *fakeStore) RevokeAll(_ context.Context, _ Session, m Mutation) (MutationResult, error) {
	f.mutated = m
	return MutationResult{Status: 204}, nil
}

func syntheticKey(fill byte) string {
	return domain.KeyPrefix + base64.RawURLEncoding.EncodeToString([]byte(strings.Repeat(string(fill), domain.KeyRandom)))
}

func serviceFor(t *testing.T, store Store, keys ...string) *Service {
	t.Helper()
	configured := make([]domain.ConfiguredKey, 0, len(keys))
	for _, key := range keys {
		hash := domain.HashKey(key)
		configured = append(configured, domain.ConfiguredKey{Hash: hash, Fingerprint: domain.Fingerprint(hash)})
	}
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	return &Service{Store: store, Keys: configured, Clock: func() time.Time { return now }}
}

// TestLogin_ADM03HashesBeforeStore proves only a digest crosses the port.
func TestLogin_ADM03HashesBeforeStore(t *testing.T) {
	key := syntheticKey('1')
	store := &fakeStore{}
	service := serviceFor(t, store, key)
	if _, err := service.Login(context.Background(), key, strings.Repeat("a", 64), "req-1"); err != nil {
		t.Fatal(err)
	}
	if store.login.PresentedHash != domain.HashKey(key) || strings.Contains(store.login.PresentedHash, key) {
		t.Fatal("raw key crossed the store port")
	}
	if !strings.EqualFold(store.result.Fingerprint, domain.Fingerprint(domain.HashKey(key))) {
		t.Fatal("fingerprint missing")
	}
}

// TestAuthenticate_ADM07RemovedHashInvalidatesSession covers rotation.
func TestAuthenticate_ADM07RemovedHashInvalidatesSession(t *testing.T) {
	oldKey := syntheticKey('2')
	newKey := syntheticKey('3')
	oldFingerprint := domain.Fingerprint(domain.HashKey(oldKey))
	token := base64.RawURLEncoding.EncodeToString([]byte(strings.Repeat("t", domain.SessionTokenBytes)))
	store := &fakeStore{session: Session{ID: "synthetic", KeyFingerprint: oldFingerprint}}
	service := serviceFor(t, store, newKey)
	if _, err := service.Authenticate(context.Background(), token); err == nil {
		t.Fatal("session from a removed key hash accepted")
	}
}

func (f *fakeStore) Mutate(context.Context, Mutation, func(context.Context, pgx.Tx) (MutationResult, []AuditEvent, error)) (MutationResult, error) {
	return MutationResult{}, domain.ErrUnavailable
}

func (f *fakeStore) RunInTx(ctx context.Context, fn func(context.Context, pgx.Tx) error) error {
	return fn(ctx, nil)
}

func (f *fakeStore) SearchAccounts(context.Context, string) ([]Account, error) {
	return nil, domain.ErrUnavailable
}

func (f *fakeStore) GetAccountTx(context.Context, pgx.Tx, string) (Account, error) {
	return Account{}, domain.ErrUnavailable
}

func (f *fakeStore) PaidSummaryTx(context.Context, pgx.Tx, string) (PaidSummary, error) {
	return PaidSummary{}, domain.ErrUnavailable
}

func (f *fakeStore) ListAudit(context.Context, AuditFilter) ([]AuditRecord, error) {
	return nil, domain.ErrUnavailable
}
