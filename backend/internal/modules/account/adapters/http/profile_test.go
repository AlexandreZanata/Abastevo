package http

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpserver"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/telemetry"
)

func TestEmailConsumeRetired(t *testing.T) {
	h, _ := testHandler()
	call(t, h, "POST", "/v1/accounts/email/codes", `{"email":"retired@example.invalid"}`)
	got := call(t, h, "POST", "/v1/accounts/email/consume", `{"email":"retired@example.invalid","code":"482916"}`)
	if got.Code != 410 || !strings.Contains(got.Body.String(), "account.email-login-retired") {
		t.Fatalf("retired route: %d %s", got.Code, got.Body.String())
	}
}

func TestVerifiedAccountHTTPContractAndRedactedLogs(t *testing.T) {
	h, _ := testHandler()
	var logs bytes.Buffer
	logger, err := telemetry.New(&logs, "debug", telemetry.RoleAPI)
	if err != nil {
		t.Fatal(err)
	}
	router := httpserver.NewRouter(logger)
	h.RegisterRoutes(router)
	post := func(route string, body string, status int) []byte {
		t.Helper()
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", route, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, r)
		if w.Code != status {
			t.Fatalf("%s: status %d, want %d, body %s", route, w.Code, status, w.Body.String())
		}
		if w.Header().Get("Cache-Control") != "no-store" {
			t.Errorf("%s: missing no-store", route)
		}
		return w.Body.Bytes()
	}

	const email = "test-verified-account@example.invalid"
	const publicName = "Pessoa Sintética"
	post("/v1/accounts/email/codes", `{"email":"`+email+`","purpose":"signup","locale":"en"}`, 202)
	var signed struct {
		AccountKey string     `json:"account_key"`
		Session    sessionOut `json:"session"`
	}
	out := post("/v1/accounts/keys/signup", `{"email":"`+email+`","code":"482916","name":"`+publicName+`"}`, 201)
	if err := json.Unmarshal(out, &signed); err != nil || signed.AccountKey == "" || signed.Session.AccessToken == "" {
		t.Fatal("signup wire response missing credentials")
	}
	proof := `"family_id":"` + signed.Session.FamilyID + `","access_token":"` + signed.Session.AccessToken + `"`
	profile := post("/v1/accounts/profile", "{"+proof+"}", 200)
	if bytes.Contains(profile, []byte(email)) || !bytes.Contains(profile, []byte(`"recovery_email_hint":"t***@example.invalid"`)) ||
		!bytes.Contains(profile, []byte(`"providers":[]`)) {
		t.Fatal("owner profile does not match privacy or provider-array contract")
	}
	post("/v1/accounts/profile", "{"+proof+`,"family_ID":"duplicated"}`, 400)
	post("/v1/accounts/profile", "{"+proof+`,"account_id":"forged"}`, 400)
	out = post("/v1/accounts/profile/name", "{"+proof+`,"name":"Oficial"}`, 400)
	if !bytes.Contains(out, []byte("account.name-invalid")) {
		t.Fatal("reserved-name verdict missing")
	}
	post("/v1/accounts/profile/name", "{"+proof+`,"name":"Pessoa de Teste"}`, 200)
	post("/v1/accounts/email/codes", `{"email":"`+email+`","purpose":"recovery"}`, 202)
	out = post("/v1/accounts/recovery", `{"email":"`+email+`","code":"111111"}`, 200)
	var recovered struct {
		AccountKey      string     `json:"account_key"`
		Session         sessionOut `json:"session"`
		SessionsRevoked bool       `json:"sessions_revoked"`
	}
	if err := json.Unmarshal(out, &recovered); err != nil || !recovered.SessionsRevoked ||
		recovered.AccountKey == "" || recovered.AccountKey == signed.AccountKey {
		t.Fatal("recovery wire response missing rotated credentials")
	}
	post("/v1/accounts/profile", "{"+proof+"}", 401)
	if !strings.Contains(logs.String(), "/v1/accounts/recovery") {
		t.Fatal("request logging did not run")
	}
	for _, private := range []string{email, publicName, "Pessoa de Teste", "482916", "111111",
		signed.AccountKey, signed.Session.AccessToken, signed.Session.RefreshToken,
		recovered.AccountKey, recovered.Session.AccessToken, recovered.Session.RefreshToken} {
		if private == "" || strings.Contains(logs.String(), private) {
			t.Fatal("account log contains private data or test credential missing")
		}
	}
}
