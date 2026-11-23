package googlepush

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/subscription/domain"
)

func TestHTTPKeysCachesAndBoundsRefetch(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []any{
			map[string]string{"kty": "RSA", "alg": "RS256", "use": "sig", "kid": "k1",
				"n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
				"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes())},
			map[string]string{"kty": "EC", "kid": "ec"},
		}})
	}))
	defer server.Close()
	now := time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)
	keys := &HTTPKeys{URL: server.URL, Client: server.Client(), Now: func() time.Time { return now }}
	got, err := keys.Key(context.Background(), "k1")
	if err != nil || got.N.Cmp(key.N) != 0 || got.E != key.E {
		t.Fatalf("key %v", err)
	}
	if _, err = keys.Key(context.Background(), "k1"); err != nil || calls != 1 {
		t.Fatalf("cache miss: %d %v", calls, err)
	}
	if _, err = keys.Key(context.Background(), "unknown"); !errors.Is(err, domain.ErrUnauthorized) || calls != 1 {
		t.Fatalf("unknown kid must not refetch within a minute: %d %v", calls, err)
	}
	now = now.Add(2 * time.Minute)
	if _, err = keys.Key(context.Background(), "ec"); !errors.Is(err, domain.ErrUnauthorized) || calls != 2 {
		t.Fatalf("non-RSA key accepted or not refreshed: %d %v", calls, err)
	}
	now = now.Add(2 * time.Hour)
	if _, err = keys.Key(context.Background(), "k1"); err != nil || calls != 3 {
		t.Fatalf("expired cache not refreshed: %d %v", calls, err)
	}
}

func TestHTTPKeysOutageIsUnavailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(503) }))
	defer server.Close()
	keys := &HTTPKeys{URL: server.URL, Client: server.Client()}
	if _, err := keys.Key(context.Background(), "k1"); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatalf("got %v", err)
	}
}
