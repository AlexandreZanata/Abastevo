package http

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
)

type unavailableMail struct {
	purpose domain.CodePurpose
	locale  string
}

func (*unavailableMail) SendCode(context.Context, string, string) error {
	return errors.New("unexpected legacy delivery path")
}

func (m *unavailableMail) SendCodeWithPurpose(_ context.Context, _, _ string, purpose domain.CodePurpose, locale string) error {
	m.purpose, m.locale = purpose, locale
	return errors.New("simulated provider refusal: member@example.invalid 482916")
}

func TestCodeDeliveryPurposeAndLocalePreserveGenericResponseOnMailFailure(t *testing.T) {
	fixture := newProviderHandler(t)
	h := fixture.handler
	service := svcOf(t, fixture)
	before := call(t, h, "POST", "/v1/accounts/email/codes", `{"email":"healthy@example.invalid"}`)
	sender := &unavailableMail{}
	service.Mail = sender
	failed := call(t, h, "POST", "/v1/accounts/email/codes", `{"email":"member@example.invalid","purpose":"recovery","locale":"en-US"}`)
	if failed.Code != http.StatusAccepted || failed.Body.String() != before.Body.String() || failed.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("mail refusal changed the generic acceptance response")
	}
	if sender.purpose != domain.CodePurposeRecovery || sender.locale != "en-US" {
		t.Fatal("delivery did not receive the requested purpose and locale")
	}
	if rec := call(t, h, "POST", "/v1/accounts/email/codes", `{"email":"member@example.invalid","purpose":"login"}`); rec.Code != http.StatusBadRequest {
		t.Fatal("unsupported email purpose must fail strict request decoding")
	}
}
