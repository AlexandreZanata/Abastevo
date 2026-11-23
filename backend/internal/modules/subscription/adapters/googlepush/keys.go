package googlepush

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"net/http"
	"sync"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/subscription/domain"
)

// GoogleCertsURL publishes the RSA keys that sign Google-issued OIDC tokens,
// including Pub/Sub push tokens.
const GoogleCertsURL = "https://www.googleapis.com/oauth2/v3/certs"

// HTTPKeys resolves a key ID from the pinned Google JWKS with a bounded
// cache. An unknown key ID refreshes the set at most once per minute.
type HTTPKeys struct {
	URL    string
	Client *http.Client
	TTL    time.Duration
	Now    func() time.Time

	mu      sync.Mutex
	keys    map[string]*rsa.PublicKey
	fetched time.Time
}

func (k *HTTPKeys) now() time.Time {
	if k.Now != nil {
		return k.Now()
	}
	return time.Now()
}

// Key returns the RSA public key for kid.
func (k *HTTPKeys) Key(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	now := k.now()
	ttl := k.TTL
	if ttl <= 0 {
		ttl = time.Hour
	}
	fresh := !k.fetched.IsZero() && now.Sub(k.fetched) < ttl
	if key, ok := k.keys[kid]; ok && fresh {
		return key, nil
	}
	if !k.fetched.IsZero() && now.Sub(k.fetched) < time.Minute {
		return nil, domain.ErrUnauthorized
	}
	keys, err := k.fetch(ctx)
	if err != nil {
		return nil, err
	}
	k.keys, k.fetched = keys, now
	if key, ok := keys[kid]; ok {
		return key, nil
	}
	return nil, domain.ErrUnauthorized
}

func (k *HTTPKeys) fetch(ctx context.Context) (map[string]*rsa.PublicKey, error) {
	url := k.URL
	if url == "" {
		url = GoogleCertsURL
	}
	client := k.Client
	if client == nil {
		client = &http.Client{Timeout: domain.ProviderTimeout}
	}
	local := *client
	local.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return errors.New("googlepush: redirects refused") }
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, domain.ErrUnavailable
	}
	res, err := local.Do(req)
	if err != nil {
		return nil, domain.ErrUnavailable
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 64<<10))
	if err != nil || res.StatusCode != http.StatusOK {
		return nil, domain.ErrUnavailable
	}
	var set struct {
		Keys []struct {
			Kty string `json:"kty"`
			Alg string `json:"alg"`
			Use string `json:"use"`
			Kid string `json:"kid"`
			N   string `json:"n"`
			E   string `json:"e"`
		} `json:"keys"`
	}
	if json.Unmarshal(raw, &set) != nil {
		return nil, domain.ErrUnavailable
	}
	keys := map[string]*rsa.PublicKey{}
	for _, jwk := range set.Keys {
		if jwk.Kty != "RSA" || jwk.Kid == "" || (jwk.Alg != "" && jwk.Alg != "RS256") || (jwk.Use != "" && jwk.Use != "sig") {
			continue
		}
		n, errN := base64.RawURLEncoding.DecodeString(jwk.N)
		e, errE := base64.RawURLEncoding.DecodeString(jwk.E)
		if errN != nil || errE != nil || len(e) == 0 || len(e) > 4 {
			continue
		}
		exponent := 0
		for _, b := range e {
			exponent = exponent<<8 | int(b)
		}
		key := &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: exponent}
		if key.N.BitLen() < 2048 || exponent < 3 {
			continue
		}
		keys[jwk.Kid] = key
	}
	if len(keys) == 0 {
		return nil, domain.ErrUnavailable
	}
	return keys, nil
}
