package application

import (
	"context"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
)

func testKeyService() *Service {
	svc, _, _ := testService()
	svc.KeyGen = domain.GenerateAccountKey
	return svc
}

func TestCreateKeyAccountMintsUniqueKey(t *testing.T) {
	svc := testKeyService()
	ctx := context.Background()
	got, err := svc.CreateKeyAccount(ctx, "Ana123")
	if err != nil {
		t.Fatalf("CreateKeyAccount: %v", err)
	}
	if got.Account.Alias != "ana123" || got.Account.Status != domain.StatusActive {
		t.Errorf("signup must create an active aliased account: %+v", got.Account)
	}
	if len(got.Key) != domain.KeyLength {
		t.Errorf("key length = %d, want %d", len(got.Key), domain.KeyLength)
	}
	// Key authenticates: key-only login opens a standard session.
	logged, err := svc.LoginWithKey(ctx, got.Key)
	if err != nil {
		t.Fatalf("LoginWithKey: %v", err)
	}
	if logged.Account.ID != got.Account.ID || logged.Session.AccessToken == "" {
		t.Errorf("login must return the account session: %+v", logged)
	}
	// Grouped transcription still logs in.
	grouped := got.Key[:8] + "-" + got.Key[8:16] + " " + got.Key[16:24] + got.Key[24:]
	if _, err := svc.LoginWithKey(ctx, grouped); err != nil {
		t.Errorf("grouped key must log in: %v", err)
	}
}

func TestCreateKeyAccountRefusesTakenAndInvalid(t *testing.T) {
	svc := testKeyService()
	ctx := context.Background()
	if _, err := svc.CreateKeyAccount(ctx, "ana"); err != nil {
		t.Fatalf("first signup: %v", err)
	}
	if _, err := svc.CreateKeyAccount(ctx, " ANA "); err != domain.ErrUsernameTaken {
		t.Errorf("taken username must refuse, got %v", err)
	}
	for _, name := range []string{"ab", "1abc", "ana!", "ana-sorriso"} {
		if _, err := svc.CreateKeyAccount(ctx, name); err != domain.ErrUsernameInvalid {
			t.Errorf("username %q must be invalid, got %v", name, err)
		}
	}
}

func TestLoginWithKeyNoOracle(t *testing.T) {
	svc := testKeyService()
	ctx := context.Background()
	created, err := svc.CreateKeyAccount(ctx, "zezinho")
	if err != nil {
		t.Fatal(err)
	}
	// Unknown key, wrong key and malformed key share one failure.
	wrong := created.Key[:31] + "x"
	if wrong == created.Key {
		wrong = created.Key[:31] + "y"
	}
	for _, candidate := range []string{
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		wrong,
		"short",
		"zzzz zzzz zzzz zzzz zzzz zzzz zzzz zzzz",
	} {
		if _, err := svc.LoginWithKey(ctx, candidate); err != domain.ErrKeyInvalid {
			t.Errorf("candidate %q must fail as key-invalid, got %v", candidate, err)
		}
	}
}

func TestLoginWithKeyRespectsSuspension(t *testing.T) {
	svc := testKeyService()
	ctx := context.Background()
	created, err := svc.CreateKeyAccount(ctx, "suspenso")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.SuspendAccount(ctx, created.Account.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.LoginWithKey(ctx, created.Key); err != domain.ErrAccountSuspended {
		t.Errorf("suspended account must refuse, got %v", err)
	}
}
