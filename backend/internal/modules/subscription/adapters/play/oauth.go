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
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/subscription/domain"
)

const (
	googleTokenURI   = "https://oauth2.googleapis.com/token"
	publisherScope   = "https://www.googleapis.com/auth/androidpublisher"
	assertionTimeout = time.Hour
)

// ServiceAccount mints Google OAuth access tokens for the Android Publisher
// API with the JWT bearer grant (RFC 7523). The key file is the service
// account JSON key; only its email, key ID and RSA private key are used, and
// the token endpoint is pinned rather than read from the file.
type ServiceAccount struct {
	email, keyID string
	key          *rsa.PrivateKey
	http         *http.Client
	Now          func() time.Time
	// TokenURI overrides the pinned endpoint in tests.
	TokenURI string

	mu      sync.Mutex
	token   string
	expires time.Time
}

// NewServiceAccount parses a service account JSON key.
func NewServiceAccount(keyJSON []byte, client *http.Client) (*ServiceAccount, error) {
	var file struct {
		Type         string `json:"type"`
		ClientEmail  string `json:"client_email"`
		PrivateKeyID string `json:"private_key_id"`
		PrivateKey   string `json:"private_key"`
	}
	if json.Unmarshal(keyJSON, &file) != nil || file.Type != "service_account" || !strings.HasSuffix(file.ClientEmail, ".gserviceaccount.com") || len(file.PrivateKeyID) > 128 {
		return nil, domain.ErrInvalid
	}
	block, _ := pem.Decode([]byte(file.PrivateKey))
	if block == nil {
		return nil, domain.ErrInvalid
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, domain.ErrInvalid
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok || key.N.BitLen() < 2048 {
		return nil, domain.ErrInvalid
	}
	local := http.Client{Timeout: domain.ProviderTimeout}
	if client != nil {
		local = *client
		if local.Timeout == 0 || local.Timeout > domain.ProviderTimeout {
			local.Timeout = domain.ProviderTimeout
		}
	}
	local.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return errors.New("play: redirects refused") }
	return &ServiceAccount{email: file.ClientEmail, keyID: file.PrivateKeyID, key: key, http: &local, Now: time.Now, TokenURI: googleTokenURI}, nil
}

// AccessToken returns a cached bearer token, refreshing it a minute early.
func (s *ServiceAccount) AccessToken(ctx context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.Now()
	if s.token != "" && now.Add(time.Minute).Before(s.expires) {
		return s.token, nil
	}
	header := map[string]string{"alg": "RS256", "typ": "JWT"}
	if s.keyID != "" {
		header["kid"] = s.keyID
	}
	rawHeader, _ := json.Marshal(header)
	rawClaims, _ := json.Marshal(map[string]any{
		"iss": s.email, "scope": publisherScope, "aud": googleTokenURI,
		"iat": now.Unix(), "exp": now.Add(assertionTimeout).Unix(),
	})
	input := base64.RawURLEncoding.EncodeToString(rawHeader) + "." + base64.RawURLEncoding.EncodeToString(rawClaims)
	digest := sha256.Sum256([]byte(input))
	signature, err := rsa.SignPKCS1v15(rand.Reader, s.key, crypto.SHA256, digest[:])
	if err != nil {
		return "", domain.ErrUnavailable
	}
	form := url.Values{
		"grant_type": {"urn:ietf:params:oauth:grant-type:jwt-bearer"},
		"assertion":  {input + "." + base64.RawURLEncoding.EncodeToString(signature)},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.TokenURI, strings.NewReader(form.Encode()))
	if err != nil {
		return "", domain.ErrUnavailable
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	res, err := s.http.Do(req)
	if err != nil {
		return "", domain.ErrUnavailable
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 16<<10))
	if err != nil || res.StatusCode != http.StatusOK {
		return "", domain.ErrUnavailable
	}
	var body struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		ExpiresIn   int64  `json:"expires_in"`
	}
	if json.Unmarshal(raw, &body) != nil || !domain.ValidToken(body.AccessToken) || !strings.EqualFold(body.TokenType, "Bearer") || body.ExpiresIn < 120 || body.ExpiresIn > 7200 {
		return "", domain.ErrUnavailable
	}
	s.token = body.AccessToken
	s.expires = now.Add(time.Duration(body.ExpiresIn) * time.Second)
	return s.token, nil
}
