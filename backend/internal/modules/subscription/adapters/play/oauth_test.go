package play

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/subscription/domain"
)

func serviceAccountJSON(t *testing.T, key *rsa.PrivateKey) []byte {
	t.Helper()
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(map[string]string{
		"type": "service_account", "client_email": "billing@synthetic-project.iam.gserviceaccount.com",
		"private_key_id": "synthetic-key-id", "token_uri": "https://attacker.invalid/token",
		"private_key": string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})),
	})
	return raw
}

func TestServiceAccountMintsAndCachesPublisherToken(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != http.MethodPost || r.ParseForm() != nil || r.Form.Get("grant_type") != "urn:ietf:params:oauth:grant-type:jwt-bearer" {
			t.Errorf("grant %v", r.Form)
		}
		parts := strings.Split(r.Form.Get("assertion"), ".")
		if len(parts) != 3 {
			t.Fatalf("assertion %q", r.Form.Get("assertion"))
		}
		var claims map[string]any
		raw, _ := base64.RawURLEncoding.DecodeString(parts[1])
		_ = json.Unmarshal(raw, &claims)
		if claims["iss"] != "billing@synthetic-project.iam.gserviceaccount.com" || claims["scope"] != publisherScope || claims["aud"] != googleTokenURI {
			t.Errorf("claims %v", claims)
		}
		sig, _ := base64.RawURLEncoding.DecodeString(parts[2])
		digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
		if rsa.VerifyPKCS1v15(&key.PublicKey, crypto.SHA256, digest[:], sig) != nil {
			t.Error("assertion signature invalid")
		}
		_, _ = w.Write([]byte(`{"access_token":"ya29.synthetic","token_type":"Bearer","expires_in":3599}`))
	}))
	defer server.Close()
	account, err := NewServiceAccount(serviceAccountJSON(t, key), server.Client())
	if err != nil {
		t.Fatal(err)
	}
	if account.TokenURI != googleTokenURI {
		t.Fatal("token endpoint must be pinned, not read from the key file")
	}
	account.TokenURI = server.URL
	now := time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)
	account.Now = func() time.Time { return now }
	for i := 0; i < 2; i++ {
		token, err := account.AccessToken(context.Background())
		if err != nil || token != "ya29.synthetic" {
			t.Fatalf("token %q %v", token, err)
		}
	}
	if calls != 1 {
		t.Fatalf("token not cached: %d calls", calls)
	}
	now = now.Add(59 * time.Minute)
	if _, err := account.AccessToken(context.Background()); err != nil || calls != 2 {
		t.Fatalf("token not refreshed before expiry: %d %v", calls, err)
	}
}

func TestServiceAccountFailuresAreUnavailable(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	for _, reply := range []string{`{"error":"invalid_grant"}`, `{"access_token":"x y","token_type":"Bearer","expires_in":3600}`, `{"access_token":"ok","token_type":"mac","expires_in":3600}`} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.Contains(reply, "error") {
				w.WriteHeader(400)
			}
			_, _ = w.Write([]byte(reply))
		}))
		account, _ := NewServiceAccount(serviceAccountJSON(t, key), server.Client())
		account.TokenURI = server.URL
		if _, err := account.AccessToken(context.Background()); !errors.Is(err, domain.ErrUnavailable) {
			t.Fatalf("%s: %v", reply, err)
		}
		server.Close()
	}
	for _, bad := range []string{`{}`, `{"type":"authorized_user"}`, `{"type":"service_account","client_email":"a@b.com","private_key":"x"}`} {
		if _, err := NewServiceAccount([]byte(bad), nil); !errors.Is(err, domain.ErrInvalid) {
			t.Fatalf("%s accepted", bad)
		}
	}
}
