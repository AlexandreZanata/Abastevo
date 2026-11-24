package http

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/adapters/oidc"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
)

const httpGoogleAudience = "web-test.apps.example.invalid"

func (f *providerFixture) googleNativeToken(t *testing.T, subject, email, nonce string) string {
	t.Helper()
	now := time.Unix(1_800_000_000, 0)
	token, err := f.google.TokenWithGoogleClaims(subject, email, httpGoogleAudience, domain.NativeNonceClaim(nonce), now,
		now.Add(time.Hour), "accounts.google.com", oidc.GoogleClaims{EmailVerified: true, HostedDomain: "example.invalid"})
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func TestGoogleNativeHTTPProfileAndConflictContract(t *testing.T) {
	f := newProviderHandler(t)
	const (
		path  = "/v1/accounts/providers/login"
		nonce = "google-native-raw-http-001"
		email = "native-first@example.invalid"
	)
	svc := svcOf(t, f)
	token := f.googleNativeToken(t, "native-http-subject-first", email, nonce)
	body := loginBody("google", token, nonce)
	res := f.call(t, "POST", path, body)
	if res.Code != http.StatusServiceUnavailable || errorCode(t, res.Body.Bytes()) != "account.provider-unavailable" {
		t.Fatalf("unset audience did not give a redacted 503: %d", res.Code)
	}
	svc.NativeAudiences = map[string]string{"google": httpGoogleAudience}
	res = f.call(t, "POST", path, body)
	var got struct {
		Account struct {
			ID         string `json:"account_id"`
			PublicName string `json:"public_name"`
		} `json:"account"`
		Created         bool  `json:"created"`
		ProfileComplete *bool `json:"profile_complete"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &got); err != nil || res.Code != http.StatusOK ||
		!got.Created || got.Account.ID == "" || got.Account.PublicName == "" || got.ProfileComplete == nil || *got.ProfileComplete {
		t.Fatalf("new Google sign-in contract: HTTP %d, decode=%v", res.Code, err)
	}
	for _, private := range []string{token, nonce, email, "native-http-subject-first"} {
		if strings.Contains(res.Body.String(), private) {
			t.Fatal("native sign-in response leaked a private claim")
		}
	}
	const conflictNonce = "google-native-raw-http-002"
	conflict := f.googleNativeToken(t, "native-http-subject-conflict", email, conflictNonce)
	res = f.call(t, "POST", path, loginBody("google", conflict, conflictNonce))
	if res.Code != http.StatusConflict || errorCode(t, res.Body.Bytes()) != "account.provider-email-in-use" || strings.Contains(res.Body.String(), email) {
		t.Fatalf("duplicate email must give a redacted 409: %d", res.Code)
	}
	verifier := svc.Verifier.(*oidc.Verifier)
	verifier.Keys = oidc.FailingKeys{}
	res = f.call(t, "POST", path, body)
	if res.Code != http.StatusServiceUnavailable || errorCode(t, res.Body.Bytes()) != "account.oidc-unavailable" {
		t.Fatalf("JWKS outage must give 503: %d", res.Code)
	}
}

func TestGoogleNativeLinkHTTPRequiresFreshLoginAndConflictIs409(t *testing.T) {
	f := newProviderHandler(t)
	svc := svcOf(t, f)
	svc.NativeAudiences = map[string]string{"google": httpGoogleAudience}
	first := f.signup(t, "link-first@example.invalid", "482916")
	second := f.signup(t, "link-second@example.invalid", "111111")
	const path = "/v1/accounts/providers/link-native"
	link := func(owner sessionOut, raw string) (int, []byte) {
		t.Helper()
		token := f.googleNativeToken(t, "native-http-linked-subject", "link-subject@example.invalid", raw)
		body, err := json.Marshal(map[string]string{
			"provider": "google", "id_token": token, "nonce": raw,
			"family_id": owner.FamilyID, "access_token": owner.AccessToken,
		})
		if err != nil {
			t.Fatal(err)
		}
		res := f.call(t, "POST", path, string(body))
		for _, private := range []string{token, raw, owner.AccessToken, "link-subject@example.invalid"} {
			if strings.Contains(res.Body.String(), private) {
				t.Fatal("Google link response leaked a private claim")
			}
		}
		return res.Code, res.Body.Bytes()
	}
	if status, body := link(first, "google-native-link-http-0001"); status != http.StatusOK || !strings.Contains(string(body), `"provider_link"`) {
		t.Fatalf("native link contract: %d", status)
	}
	if status, body := link(second, "google-native-link-http-0002"); status != http.StatusConflict || errorCode(t, body) != "account.link-cross-account-refused" {
		t.Fatalf("cross-account link contract: %d", status)
	}
	clock := svc.Clock.(*testClock)
	clock.mu.Lock()
	clock.now += int64(domain.FreshLoginSeconds + 1)
	clock.mu.Unlock()
	if status, body := link(first, "google-native-link-http-0003"); status != http.StatusForbidden || errorCode(t, body) != "account.reauthentication-required" {
		t.Fatalf("stale link contract: %d", status)
	}
}
