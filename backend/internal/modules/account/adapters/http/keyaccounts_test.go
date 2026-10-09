package http

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
)

func testKeyHandler() Handler {
	var mu sync.Mutex
	n := 0
	return Handler{Service: &application.Service{
		Clock:    &testClock{now: 1_700_000_000},
		Hasher:   domain.SHA256Hasher{},
		Mail:     &testMail{},
		Store:    application.NewMemStore(),
		CodeGen:  func() (string, error) { return "482916", nil },
		TokenGen: func() (string, error) { return strings.Repeat("t", 64), nil },
		AliasGen: func() (string, error) { return "alias-test", nil },
		KeyGen:   domain.GenerateAccountKey,
		IDGen: func() (string, error) {
			mu.Lock()
			defer mu.Unlock()
			n++
			return fmt.Sprintf("aaaaaaaa-1111-4111-8111-1111111111%02d", n), nil
		},
	}}
}

func TestCreateKeyAccountRoundTrip(t *testing.T) {
	h := testKeyHandler()
	rec := call(t, h, "POST", "/v1/accounts/keys", `{"username":"Ana123"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var created struct {
		Username   string `json:"username"`
		AccountKey string `json:"account_key"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.Username != "ana123" || len(created.AccountKey) != domain.KeyLength {
		t.Fatalf("unexpected creation reply: %+v", created)
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Error("key reply must be no-store")
	}
	// Key-only login opens a session for the same account.
	rec = call(t, h, "POST", "/v1/accounts/keys/login",
		`{"account_key":"`+created.AccountKey+`"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("login status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var logged struct {
		Account struct {
			Alias string `json:"public_alias"`
		} `json:"account"`
		Session struct {
			AccessToken string `json:"access_token"`
		} `json:"session"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &logged); err != nil {
		t.Fatal(err)
	}
	if logged.Account.Alias != "ana123" || logged.Session.AccessToken == "" {
		t.Fatalf("unexpected login reply: %+v", logged)
	}
}

func TestCreateKeyAccountConflicts(t *testing.T) {
	h := testKeyHandler()
	if rec := call(t, h, "POST", "/v1/accounts/keys", `{"username":"ana"}`); rec.Code != http.StatusCreated {
		t.Fatalf("first signup status = %d", rec.Code)
	}
	rec := call(t, h, "POST", "/v1/accounts/keys", `{"username":"ana"}`)
	if rec.Code != http.StatusConflict {
		t.Errorf("taken username status = %d, want 409", rec.Code)
	}
	for _, body := range []string{`{"username":"ab"}`, `{"username":""}`, `{}`} {
		if rec := call(t, h, "POST", "/v1/accounts/keys", body); rec.Code != http.StatusBadRequest {
			t.Errorf("body %s status = %d, want 400", body, rec.Code)
		}
	}
}

// wrongKey flips the last glyph to a different alphabet glyph so the
// mutated key is deterministically wrong even when the minted key ends in
// the replacement glyph (a 1-in-31 flake otherwise).
func wrongKey(key string) string {
	if key[:31]+"x" == key {
		return key[:31] + "y"
	}
	return key[:31] + "x"
}

func TestLoginWithKeyNoOracle(t *testing.T) {
	h := testKeyHandler()
	rec := call(t, h, "POST", "/v1/accounts/keys", `{"username":"zezinho"}`)
	var created struct {
		AccountKey string `json:"account_key"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	bodies := []string{
		`{"account_key":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`,
		`{"account_key":"` + wrongKey(created.AccountKey) + `"}`,
		`{"account_key":"short"}`,
		`{}`,
	}
	for _, body := range bodies {
		rec := call(t, h, "POST", "/v1/accounts/keys/login", body)
		if rec.Code != http.StatusUnauthorized && rec.Code != http.StatusBadRequest {
			t.Errorf("body %s status = %d, want 401 (or 400 for empty)", body, rec.Code)
		}
		if rec.Code == http.StatusUnauthorized {
			var doc struct {
				Error struct {
					Code string `json:"code"`
				} `json:"error"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
				t.Fatal(err)
			}
			if doc.Error.Code != "account.key-invalid" {
				t.Errorf("verdict = %s, want account.key-invalid", doc.Error.Code)
			}
		}
	}
	_ = context.Background
}
