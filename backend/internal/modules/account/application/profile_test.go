package application

import (
	"errors"
	"strings"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
)

func TestInvalidSignupNameAndEmailRefuseBeforeStoreIO(t *testing.T) {
	svc := Service{Clock: &fakeClock{now: 1_800_000_000}}
	for _, name := range []string{
		"", "A", strings.Repeat("Á", 41), "A-1234", "Nome 12345", "admin",
		"Nome@example.invalid", "Nome/Conta", "Nome:Conta", "Nome\nConta", "Nome\u200eConta",
	} {
		if _, err := svc.SignUp(t.Context(), name, "owner@example.invalid", "123456"); !errors.Is(err, domain.ErrNameInvalid) {
			t.Errorf("name validation failed before IO for %q: %v", name, err)
		}
	}
	for _, email := range []string{"", "user", "@example.invalid", "user@ex\rample.invalid"} {
		if _, err := svc.SignUp(t.Context(), "Maria Silva", email, "123456"); !errors.Is(err, domain.ErrEmailInvalid) {
			t.Errorf("email validation failed before IO for %q: %v", email, err)
		}
	}
}
