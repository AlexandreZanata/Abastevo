package http

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
)

const nativeAudience = "com.anpfuel.app"

func (f *providerFixture) enableNative(t *testing.T) {
	t.Helper()
	svc, ok := f.handler.Service.(*application.Service)
	if !ok {
		t.Fatal("fixture service is not the application service")
	}
	svc.NativeAudiences = map[string]string{"apple": nativeAudience}
}

func (f *providerFixture) nativeToken(t *testing.T, sub, aud, rawNonce string) string {
	t.Helper()
	now := time.Unix(1_800_000_000, 0)
	tok, err := f.apple.TokenWith(sub, "relay@privaterelay.appleid.com", aud, domain.NativeNonceClaim(rawNonce), now, now.Add(10*time.Minute), domain.IssuerApple)
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

func loginBody(provider, token, nonce string) string {
	raw, _ := json.Marshal(map[string]string{"provider": provider, "id_token": token, "nonce": nonce})
	return string(raw)
}

type loginOut struct {
	Account struct {
		AccountID string `json:"account_id"`
		Alias     string `json:"public_alias"`
		Status    string `json:"status"`
	} `json:"account"`
	Session struct {
		FamilyID        string `json:"family_id"`
		AccountID       string `json:"account_id"`
		AccessToken     string `json:"access_token"`
		RefreshToken    string `json:"refresh_token"`
		AccessExpires   string `json:"access_expires_at"`
		AbsoluteExpires string `json:"absolute_expires_at"`
	} `json:"session"`
	Created *bool `json:"created"`
}

func errorCode(t *testing.T, body []byte) string {
	t.Helper()
	var env struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("decode error envelope: %v (%s)", err, body)
	}
	return env.Error.Code
}

func TestProviderLoginUnconfiguredFailsClosed(t *testing.T) {
	f := newProviderHandler(t)
	nonce := "raw-native-nonce-http-0001"
	rec := f.call(t, "POST", "/v1/accounts/providers/login", loginBody("apple", f.nativeToken(t, "apple-http-0", nativeAudience, nonce), nonce))
	if rec.Code != 503 || errorCode(t, rec.Body.Bytes()) != "account.provider-unavailable" {
		t.Fatalf("want 503 provider-unavailable, got %d (%s)", rec.Code, rec.Body.String())
	}
}

func TestProviderLoginHTTPContract(t *testing.T) {
	f := newProviderHandler(t)
	f.enableNative(t)
	nonce := "raw-native-nonce-http-0002"
	token := f.nativeToken(t, "apple-http-subject-1", nativeAudience, nonce)
	rec := f.call(t, "POST", "/v1/accounts/providers/login", loginBody("apple", token, nonce))
	if rec.Code != 200 {
		t.Fatalf("login: %d (%s)", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("login response must be no-store")
	}
	body := rec.Body.String()
	for _, secret := range []string{token, nonce, domain.NativeNonceClaim(nonce), "apple-http-subject-1", "privaterelay"} {
		if strings.Contains(body, secret) {
			t.Fatalf("response leaked a provider value: %s", body)
		}
	}
	var out loginOut
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Created == nil || !*out.Created || out.Account.AccountID == "" || out.Account.Status != "active" ||
		out.Session.AccountID != out.Account.AccountID || out.Session.AccessToken == "" || out.Session.RefreshToken == "" ||
		out.Session.AccessExpires == "" || out.Session.AbsoluteExpires == "" {
		t.Fatalf("response shape must match the email consume: %s", body)
	}
	// Identical key set to the email-code consume response.
	var shape map[string]json.RawMessage
	_ = json.Unmarshal(rec.Body.Bytes(), &shape)
	if len(shape) != 3 || shape["account"] == nil || shape["session"] == nil || shape["created"] == nil {
		t.Fatalf("unexpected top-level keys: %s", body)
	}

	// Replay of the same proof refuses without a second session.
	rec = f.call(t, "POST", "/v1/accounts/providers/login", loginBody("apple", token, nonce))
	if rec.Code != 401 || errorCode(t, rec.Body.Bytes()) != "account.oidc-nonce-reused" {
		t.Fatalf("replay: %d (%s)", rec.Code, rec.Body.String())
	}

	// A fresh proof for the same subject signs in to the same account.
	nonce2 := "raw-native-nonce-http-0003"
	rec = f.call(t, "POST", "/v1/accounts/providers/login", loginBody("apple", f.nativeToken(t, "apple-http-subject-1", nativeAudience, nonce2), nonce2))
	var again loginOut
	if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &again) != nil || again.Created == nil || *again.Created || again.Account.AccountID != out.Account.AccountID {
		t.Fatalf("second login: %d (%s)", rec.Code, rec.Body.String())
	}
}

func TestProviderLoginHTTPRefusals(t *testing.T) {
	f := newProviderHandler(t)
	f.enableNative(t)
	nonce := "raw-native-nonce-http-0010"
	good := f.nativeToken(t, "apple-http-subject-2", nativeAudience, nonce)
	cases := []struct {
		name, body string
		status     int
		code       string
	}{
		{"unknown provider", loginBody("facebook", good, nonce), 400, "account.bad-body"},
		{"google not native", loginBody("google", good, nonce), 503, "account.provider-unavailable"},
		{"missing nonce", loginBody("apple", good, ""), 400, "account.bad-body"},
		{"missing token", loginBody("apple", "", nonce), 400, "account.bad-body"},
		{"short nonce", loginBody("apple", good, "short"), 400, "account.bad-body"},
		{"oversized token", loginBody("apple", strings.Repeat("A", domain.MaxProviderTokenBytes+1), nonce), 400, "account.bad-body"},
		{"unknown field", `{"provider":"apple","id_token":"` + good + `","nonce":"` + nonce + `","audience":"com.anpfuel.app"}`, 400, "account.bad-body"},
		{"link audience token", loginBody("apple", f.nativeToken(t, "apple-http-subject-2", providerAudience, nonce), nonce), 401, "account.oidc-wrong-audience"},
		{"raw nonce mismatch", loginBody("apple", good, nonce+"-other"), 401, "account.oidc-nonce-mismatch"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := f.call(t, "POST", "/v1/accounts/providers/login", tc.body)
			if rec.Code != tc.status || errorCode(t, rec.Body.Bytes()) != tc.code {
				t.Fatalf("want %d %s, got %d (%s)", tc.status, tc.code, rec.Code, rec.Body.String())
			}
		})
	}
	// None of the refusals consumed the honest proof.
	if rec := f.call(t, "POST", "/v1/accounts/providers/login", loginBody("apple", good, nonce)); rec.Code != 200 {
		t.Fatalf("honest login after refusals: %d (%s)", rec.Code, rec.Body.String())
	}
}
