package http

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

// After a valid session proof, an unexpected deletion failure is an outage:
// a 401 would tell the client its session is gone and sign the user out.
func TestSelfDeleteInternalFailuresAreUnavailableNotUnauthenticated(t *testing.T) {
	for name, failure := range map[string]error{
		"trigger":  &pgconn.PgError{Code: "P8104", Message: "settings reduction source unavailable"},
		"commit":   errors.New("commit unexpectedly resulted in rollback"),
		"payments": errors.New("payments: provider cancellation failed"),
	} {
		t.Run(name, func(t *testing.T) {
			f := newProviderHandler(t)
			sess := f.signup(t, name+"-down@example.invalid", "482916")
			svcOf(t, f).RequestBillingExit = func(context.Context, string) error { return failure }
			rec := f.call(t, "POST", "/v1/accounts/deletion", deleteBody(sess))
			if rec.Code != 503 || !strings.Contains(rec.Body.String(), "account.unavailable") || strings.Contains(rec.Body.String(), "P8104") {
				t.Fatalf("internal deletion failure: %d %s", rec.Code, rec.Body.String())
			}
			if rec := f.call(t, "POST", "/v1/accounts/providers/list", deleteBody(sess)); rec.Code != 200 {
				t.Fatal("failed deletion changed the account", rec.Code)
			}
		})
	}
	f := newProviderHandler(t)
	sess := f.signup(t, "wiring-down@example.invalid", "482916")
	svcOf(t, f).RequestBillingExit = nil
	if rec := f.call(t, "POST", "/v1/accounts/deletion", deleteBody(sess)); rec.Code != 503 {
		t.Fatal("missing billing wiring reported as authentication", rec.Code, rec.Body.String())
	}
	if rec := f.call(t, "POST", "/v1/accounts/deletion", `{"family_id":"missing","access_token":"bad"}`); rec.Code != 401 {
		t.Fatal("invalid proof no longer 401", rec.Code)
	}
}
