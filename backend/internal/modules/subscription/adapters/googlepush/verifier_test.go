package googlepush

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestAuthenticatedPushRSAClaims(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	base := map[string]any{"iss": "https://accounts.google.com", "aud": "https://backend.example.invalid/rtdn", "email": "sender@example.invalid", "email_verified": true, "iat": now.Unix(), "exp": now.Add(time.Hour).Unix()}
	for _, name := range []string{"valid", "wrong-audience", "wrong-issuer", "wrong-account", "unverified", "expired", "future", "wrong-kid", "tampered"} {
		t.Run(name, func(t *testing.T) {
			c := map[string]any{}
			for k, v := range base {
				c[k] = v
			}
			kid := "trusted"
			switch name {
			case "wrong-audience":
				c["aud"] = "foreign"
			case "wrong-issuer":
				c["iss"] = "https://evil.invalid"
			case "wrong-account":
				c["email"] = "foreign@example.invalid"
			case "unverified":
				c["email_verified"] = false
			case "expired":
				c["exp"] = now.Add(-time.Second).Unix()
			case "future":
				c["iat"] = now.Add(2 * time.Minute).Unix()
			case "wrong-kid":
				kid = "foreign"
			}
			head, _ := json.Marshal(map[string]string{"alg": "RS256", "kid": kid})
			raw, _ := json.Marshal(c)
			signed := base64.RawURLEncoding.EncodeToString(head) + "." + base64.RawURLEncoding.EncodeToString(raw)
			sum := sha256.Sum256([]byte(signed))
			sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, sum[:])
			if err != nil {
				t.Fatal(err)
			}
			if name == "tampered" {
				sig[0] ^= 1
			}
			token := signed + "." + base64.RawURLEncoding.EncodeToString(sig)
			v := Verifier{Audience: base["aud"].(string), ServiceAccount: base["email"].(string), Now: func() time.Time { return now }, Keys: func(_ context.Context, kid string) (*rsa.PublicKey, error) {
				if kid != "trusted" {
					return nil, errors.New("unknown key")
				}
				return &key.PublicKey, nil
			}}
			err = v.Verify(context.Background(), token)
			if (err == nil) != (name == "valid") {
				t.Fatal("incorrect trusted push verdict", err)
			}
		})
	}
}
