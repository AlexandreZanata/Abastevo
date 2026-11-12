package application_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	account "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/domain"
	"log/slog"
	"strings"
	"testing"
	"time"
)

const id = "7b0e2a54-0000-4000-8000-000000000001"

type store struct {
	row                       app.Row
	created, touched, revoked int
	failure                   error
	revokeFailure             error
}

func (s *store) Create(_ context.Context, row app.Row, _ app.Proof, _ string, _ time.Time) error {
	s.created++
	s.row = row
	return s.failure
}
func (s *store) Authenticate(ctx context.Context, lookup string, now time.Time, touch bool, verify func(app.Row) error) (app.Row, error) {
	if s.failure != nil {
		return app.Row{}, s.failure
	}
	if lookup != s.row.Lookup {
		return app.Row{}, domain.ErrUnauthorized
	}
	if err := verify(s.row); err != nil {
		return app.Row{}, err
	}
	if touch {
		s.touched++
		s.row.LastSeenAt = now
	}
	return s.row, nil
}
func TestPersistedExpiredOrFutureSessionFailsBeforeIdleTouch(t *testing.T) {
	for _, kind := range []string{"idle", "absolute", "rollback"} {
		t.Run(kind, func(t *testing.T) {
			service, store, now := fixture(t)
			result, err := service.Login(t.Context(), id, "proof", "")
			if err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "idle":
				*now = now.Add(domain.IdleTimeout)
			case "absolute":
				*now = store.row.ExpiresAt
				store.row.LastSeenAt = now.Add(-time.Minute)
			case "rollback":
				*now = now.Add(-time.Second)
			}
			if _, err := service.Authenticate(t.Context(), result.Token, result.CSRF, false); !errors.Is(err, domain.ErrUnauthorized) || store.touched != 0 {
				t.Fatal("non-live persisted session renewed", err, store.touched)
			}
		})
	}
}
func (s *store) Revoke(context.Context, string, string, time.Time) error {
	s.revoked++
	return s.revokeFailure
}
func fixture(t *testing.T) (*app.Service, *store, *time.Time) {
	t.Helper()
	now := time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)
	s := &store{}
	service := &app.Service{Enabled: true, Store: s, Hasher: account.SHA256Hasher{}, Now: func() time.Time { return now }, NewID: func() (string, error) { return id, nil },
		NewSecret: func() (string, error) { return strings.Repeat("a", 64), nil },
		Prove: func(context.Context, string, string) (app.Proof, error) {
			return app.Proof{AccountID: id, FamilyID: id, AuthenticatedAt: now, AccessExpiresAt: now.Add(domain.FreshWindow), FamilyExpiresAt: now.Add(24 * time.Hour), AccessHash: strings.Repeat("b", 64)}, nil
		}}
	return service, s, &now
}
func TestCompanyIssuanceStoresNoRawCredentialAndFreshnessDoesNotSlide(t *testing.T) {
	service, store, now := fixture(t)
	result, err := service.Login(t.Context(), id, "synthetic-account-proof", "")
	if err != nil {
		t.Fatal(err)
	}
	if result.Token == result.CSRF || store.row.TokenHash == result.Token || store.row.CSRFHash == result.CSRF || store.created != 1 {
		t.Fatal("secret separation")
	}
	if _, err := service.Authenticate(t.Context(), result.Token, result.CSRF, false); err != nil {
		t.Fatal(err)
	}
	*now = now.Add(domain.FreshWindow)
	if _, err := service.Authenticate(t.Context(), result.Token, result.CSRF, true); !errors.Is(err, domain.ErrFresh) {
		t.Fatal("idle activity extended freshness", err)
	}
	if store.touched != 1 {
		t.Fatal("failed fresh check touched idle")
	}
	if _, err := service.Authenticate(t.Context(), result.Token, domain.CSRFPrefix+strings.Repeat("c", 64), false); !errors.Is(err, domain.ErrCSRF) {
		t.Fatal(err)
	}
	if store.touched != 1 {
		t.Fatal("invalid csrf touched idle")
	}
	if _, err := service.Authenticate(t.Context(), result.Token, result.CSRF, false); err != nil {
		t.Fatal(err)
	}
	if err := service.Logout(t.Context(), result.Token, result.CSRF); err != nil || store.revoked != 1 {
		t.Fatal(err)
	}
	var log bytes.Buffer
	slog.New(slog.NewJSONHandler(&log, nil)).Info("session", "result", result, "record", store.row)
	if strings.Contains(log.String(), result.Token) || strings.Contains(log.String(), result.CSRF) || strings.Contains(log.String(), store.row.TokenHash) {
		t.Fatal("credential formatting leak")
	}
}
func TestMissingStaleProofAndStoreFailureFailClosed(t *testing.T) {
	service, store, now := fixture(t)
	service.Prove = nil
	if _, err := service.Login(t.Context(), id, "proof", ""); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatal(err)
	}
	service.Prove = func(context.Context, string, string) (app.Proof, error) {
		return app.Proof{AccountID: id, FamilyID: id, AuthenticatedAt: now.Add(-domain.FreshWindow), AccessExpiresAt: *now, FamilyExpiresAt: now.Add(time.Hour), AccessHash: strings.Repeat("b", 64)}, nil
	}
	if _, err := service.Login(t.Context(), id, "proof", ""); !errors.Is(err, domain.ErrFresh) {
		t.Fatal(err)
	}
	if store.created != 0 {
		t.Fatal("stale proof reached persistence")
	}
	service, store, _ = fixture(t)
	store.failure = errors.New("PRIVATE-DATABASE-OUTPUT")
	if _, err := service.Login(t.Context(), id, "proof", ""); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatal("unsafe persistence error")
	}
	service.Enabled = false
	if _, err := service.Authenticate(t.Context(), "cookie", "csrf", false); !errors.Is(err, domain.ErrDisabled) {
		t.Fatal(err)
	}
}

type failingSalt struct {
	account.CodeHasher
	after int
}

func (h *failingSalt) NewSalt() (string, error) {
	h.after--
	if h.after == 0 {
		return "", errors.New("PRIVATE-ENTROPY-ERROR")
	}
	return h.CodeHasher.NewSalt()
}

func TestCredentialGenerationFailureReturnsNoCredentialsOrWrite(t *testing.T) {
	for _, stage := range []string{"id", "cookie", "csrf", "cookie salt", "csrf salt"} {
		t.Run(stage, func(t *testing.T) {
			service, store, _ := fixture(t)
			switch stage {
			case "id":
				service.NewID = func() (string, error) { return "", errors.New("PRIVATE-ENTROPY-ERROR") }
			case "cookie", "csrf":
				calls := 0
				service.NewSecret = func() (string, error) {
					calls++
					if stage == "cookie" || calls == 2 {
						return "", errors.New("PRIVATE-ENTROPY-ERROR")
					}
					return strings.Repeat("a", 64), nil
				}
			case "cookie salt":
				service.Hasher = &failingSalt{CodeHasher: account.SHA256Hasher{}, after: 1}
			case "csrf salt":
				service.Hasher = &failingSalt{CodeHasher: account.SHA256Hasher{}, after: 2}
			}
			result, err := service.Login(t.Context(), id, "proof", "")
			if !errors.Is(err, domain.ErrUnavailable) || result != (app.Result{}) || store.created != 0 {
				t.Fatal("generation failure released credentials or wrote session", err)
			}
		})
	}
}

func TestPrivateFormattingAndFailedLogoutDoNotClaimSuccess(t *testing.T) {
	service, store, _ := fixture(t)
	result, err := service.Login(t.Context(), id, "proof", "")
	if err != nil {
		t.Fatal(err)
	}
	proof, err := service.Prove(t.Context(), id, "proof")
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []any{result, result.Row, proof} {
		var log bytes.Buffer
		slog.New(slog.NewJSONHandler(&log, nil)).Info("session", "value", value)
		out := fmt.Sprintf("%v %+v %#v", value, value, value) + log.String()
		for _, secret := range []string{result.Token, result.CSRF, result.Row.TokenHash, result.Row.CSRFHash, proof.AccessHash} {
			if strings.Contains(out, secret) {
				t.Fatal("private formatting exposed verifier")
			}
		}
	}
	store.revokeFailure = errors.New("PRIVATE-DATABASE-ERROR")
	if err := service.Logout(t.Context(), result.Token, result.CSRF); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatal("failed logout claimed success", err)
	}
	before := store.revoked
	if err := service.Logout(t.Context(), result.Token, "invalid"); !errors.Is(err, domain.ErrCSRF) || store.revoked != before {
		t.Fatal("invalid csrf reached logout mutation", err)
	}
}
