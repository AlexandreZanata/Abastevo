// Package apple talks to the Sign in with Apple REST API for grant
// revocation (P13-T05, App Store guideline 5.1.1(v)): it redeems a
// native authorization code at /auth/token for a refresh token, seals it
// immediately, and later revokes it at /auth/revoke when the account is
// deleted. Each request authenticates with a short-lived ES256 client
// secret (iss team ID, sub client ID, aud https://appleid.apple.com, kid
// key ID) signed with the team's Sign in with Apple private key.
//
// Nothing here logs or returns a code, token or secret: errors are
// static sentinels. Responses are size-capped, redirects are refused and
// only HTTPS endpoints are contacted.
package apple

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// ProductionBaseURL is Apple's identity service origin.
const ProductionBaseURL = "https://appleid.apple.com"

const (
	issuer           = "https://appleid.apple.com"
	clientSecretTTL  = 5 * time.Minute
	maxResponseBytes = 64 << 10
	maxTokenBytes    = 2048
	requestTimeout   = 10 * time.Second
	grantKeyBytes    = 32
	sealVersion      = 1
)

var (
	// ErrExchange: Apple refused the code or returned a token for a
	// different client or subject.
	ErrExchange = errors.New("apple: authorization code exchange refused")
	// ErrRevoke: Apple refused the revocation request.
	ErrRevoke = errors.New("apple: token revocation refused")
	// ErrUnavailable: network failure, timeout, redirect or 5xx/429.
	ErrUnavailable = errors.New("apple: identity service unavailable")
	// ErrCredentials: missing or malformed team/key/client credentials.
	ErrCredentials = errors.New("apple: credentials unavailable")
	// ErrSealed: a sealed grant does not open under its binding.
	ErrSealed = errors.New("apple: sealed grant unreadable")
)

// Credentials identify the client to Apple. Key is the team's Sign in
// with Apple private key (P-256).
type Credentials struct {
	TeamID   string
	KeyID    string
	ClientID string
	Key      *ecdsa.PrivateKey
}

// ClientSecret mints the ES256 client secret JWT Apple requires on
// /auth/token and /auth/revoke, valid for five minutes from now.
func (c Credentials) ClientSecret(now time.Time) (string, error) {
	if c.Key == nil || c.Key.Curve != elliptic.P256() || c.TeamID == "" || c.KeyID == "" || c.ClientID == "" {
		return "", ErrCredentials
	}
	header, err := json.Marshal(map[string]string{"alg": "ES256", "kid": c.KeyID})
	if err != nil {
		return "", ErrCredentials
	}
	claims, err := json.Marshal(map[string]any{
		"iss": c.TeamID,
		"iat": now.Unix(),
		"exp": now.Add(clientSecretTTL).Unix(),
		"aud": issuer,
		"sub": c.ClientID,
	})
	if err != nil {
		return "", ErrCredentials
	}
	signing := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(claims)
	digest := sha256.Sum256([]byte(signing))
	r, s, err := ecdsa.Sign(rand.Reader, c.Key, digest[:])
	if err != nil {
		return "", ErrCredentials
	}
	sig := append(pad32(r), pad32(s)...)
	return signing + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

func pad32(n *big.Int) []byte {
	out := make([]byte, 32)
	n.FillBytes(out)
	return out
}

// ParsePrivateKey reads Apple's downloaded .p8 key: a PEM "PRIVATE KEY"
// block holding a PKCS#8 P-256 ECDSA key. Anything else is refused.
func ParsePrivateKey(raw []byte) (*ecdsa.PrivateKey, error) {
	block, _ := pem.Decode(raw)
	if block == nil || block.Type != "PRIVATE KEY" {
		return nil, ErrCredentials
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, ErrCredentials
	}
	key, ok := parsed.(*ecdsa.PrivateKey)
	if !ok || key.Curve != elliptic.P256() {
		return nil, ErrCredentials
	}
	return key, nil
}

// LoadCredentials reads the private key and the 32-byte grant sealing
// key from their configured files. Errors never echo paths or bytes.
func LoadCredentials(teamID, keyID, clientID, privateKeyFile, grantKeyFile string) (Credentials, *Sealer, error) {
	rawKey, err := os.ReadFile(privateKeyFile)
	if err != nil {
		return Credentials{}, nil, ErrCredentials
	}
	key, err := ParsePrivateKey(rawKey)
	if err != nil {
		return Credentials{}, nil, err
	}
	info, err := os.Stat(grantKeyFile)
	if err != nil || !info.Mode().IsRegular() || info.Size() != grantKeyBytes {
		return Credentials{}, nil, ErrCredentials
	}
	grantKey, err := os.ReadFile(grantKeyFile)
	if err != nil {
		return Credentials{}, nil, ErrCredentials
	}
	sealer, err := NewSealer(grantKey)
	clear(grantKey)
	if err != nil {
		return Credentials{}, nil, err
	}
	creds := Credentials{TeamID: teamID, KeyID: keyID, ClientID: clientID, Key: key}
	if _, err := creds.ClientSecret(time.Now()); err != nil {
		return Credentials{}, nil, err
	}
	return creds, sealer, nil
}

// NewClient loads the configured credentials and returns a production
// client. Errors are ErrCredentials without paths or key bytes.
func NewClient(teamID, keyID, clientID, privateKeyFile, grantKeyFile string) (*Client, error) {
	creds, sealer, err := LoadCredentials(teamID, keyID, clientID, privateKeyFile, grantKeyFile)
	if err != nil {
		return nil, err
	}
	return &Client{BaseURL: ProductionBaseURL, Credentials: creds, Sealer: sealer, Now: time.Now}, nil
}

// Reason classifies a capture or revocation failure for logs: a fixed
// vocabulary, never error text that could carry provider detail.
func Reason(err error) string {
	switch {
	case err == nil:
		return "ok"
	case errors.Is(err, ErrExchange):
		return "exchange-refused"
	case errors.Is(err, ErrRevoke):
		return "revoke-refused"
	case errors.Is(err, ErrUnavailable):
		return "provider-unavailable"
	case errors.Is(err, ErrCredentials):
		return "credentials-unavailable"
	case errors.Is(err, ErrSealed):
		return "sealed-grant-unreadable"
	default:
		return "store-failure"
	}
}

// Sealer encrypts refresh tokens at rest with AES-256-GCM: a version
// byte, a random 12-byte nonce, then ciphertext and tag, authenticated
// with the caller's binding (the grant id) as additional data.
type Sealer struct{ aead cipher.AEAD }

// NewSealer builds a sealer from exactly 32 key bytes.
func NewSealer(key []byte) (*Sealer, error) {
	if len(key) != grantKeyBytes {
		return nil, ErrCredentials
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, ErrCredentials
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, ErrCredentials
	}
	return &Sealer{aead: aead}, nil
}

// Seal encrypts token bound to aad.
func (s *Sealer) Seal(token, aad string) ([]byte, error) {
	if s == nil || s.aead == nil || !validToken(token) || aad == "" {
		return nil, ErrSealed
	}
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, ErrSealed
	}
	out := append([]byte{sealVersion}, nonce...)
	return s.aead.Seal(out, nonce, []byte(token), []byte(aad)), nil
}

// Open decrypts a sealed token under aad.
func (s *Sealer) Open(raw []byte, aad string) (string, error) {
	if s == nil || s.aead == nil || aad == "" || len(raw) < 1+s.aead.NonceSize()+s.aead.Overhead()+1 || raw[0] != sealVersion {
		return "", ErrSealed
	}
	n := s.aead.NonceSize()
	clearText, err := s.aead.Open(nil, raw[1:1+n], raw[1+n:], []byte(aad))
	if err != nil || !validToken(string(clearText)) {
		return "", ErrSealed
	}
	return string(clearText), nil
}

func validToken(token string) bool {
	if len(token) == 0 || len(token) > maxTokenBytes {
		return false
	}
	for i := 0; i < len(token); i++ {
		if token[i] < 0x21 || token[i] > 0x7e {
			return false
		}
	}
	return true
}

// Client implements the account GrantExchanger and GrantRevoker ports.
type Client struct {
	BaseURL     string
	Credentials Credentials
	Sealer      *Sealer
	HTTP        *http.Client
	Now         func() time.Time
}

func (c *Client) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

// Exchange redeems a native authorization code (grant_type
// authorization_code, no redirect_uri for native apps) and returns the
// refresh token sealed under aad. The id_token of the response comes
// straight from Apple's token endpoint over TLS (OIDC Core 3.1.3.7), so
// its claims are checked without a second signature verification: the
// issuer, this client as audience and the already verified subject must
// all match, or the token is discarded.
func (c *Client) Exchange(ctx context.Context, provider, code, subject, aad string) ([]byte, error) {
	if provider != "apple" || code == "" || subject == "" || aad == "" {
		return nil, ErrExchange
	}
	if c.Sealer == nil {
		return nil, ErrCredentials
	}
	secret, err := c.Credentials.ClientSecret(c.now())
	if err != nil {
		return nil, err
	}
	form := url.Values{
		"client_id":     {c.Credentials.ClientID},
		"client_secret": {secret},
		"code":          {code},
		"grant_type":    {"authorization_code"},
	}
	status, body, err := c.post(ctx, "/auth/token", form)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, classify(status, ErrExchange)
	}
	var out struct {
		RefreshToken string `json:"refresh_token"`
		IDToken      string `json:"id_token"`
	}
	if err := json.Unmarshal(body, &out); err != nil || !validToken(out.RefreshToken) {
		return nil, ErrExchange
	}
	if !c.sameIdentity(out.IDToken, subject) {
		return nil, ErrExchange
	}
	return c.Sealer.Seal(out.RefreshToken, aad)
}

// Revoke opens the sealed refresh token and revokes it. Apple answers
// 200 both when it revokes and when the token was already invalid.
func (c *Client) Revoke(ctx context.Context, provider string, sealed []byte, aad string) error {
	if provider != "apple" {
		return ErrRevoke
	}
	if c.Sealer == nil {
		return ErrCredentials
	}
	token, err := c.Sealer.Open(sealed, aad)
	if err != nil {
		return err
	}
	secret, err := c.Credentials.ClientSecret(c.now())
	if err != nil {
		return err
	}
	form := url.Values{
		"client_id":       {c.Credentials.ClientID},
		"client_secret":   {secret},
		"token":           {token},
		"token_type_hint": {"refresh_token"},
	}
	status, _, err := c.post(ctx, "/auth/revoke", form)
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return classify(status, ErrRevoke)
	}
	return nil
}

func classify(status int, refused error) error {
	if status == http.StatusTooManyRequests || status >= 500 {
		return ErrUnavailable
	}
	return refused
}

func (c *Client) sameIdentity(idToken, subject string) bool {
	parts := strings.Split(idToken, ".")
	if len(parts) != 3 {
		return false
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return false
	}
	var claims struct {
		Issuer  string `json:"iss"`
		Subject string `json:"sub"`
		Aud     any    `json:"aud"`
	}
	if err := json.Unmarshal(raw, &claims); err != nil {
		return false
	}
	if claims.Issuer != issuer || claims.Subject != subject {
		return false
	}
	switch aud := claims.Aud.(type) {
	case string:
		return aud == c.Credentials.ClientID
	case []any:
		for _, a := range aud {
			if s, ok := a.(string); ok && s == c.Credentials.ClientID {
				return true
			}
		}
	}
	return false
}

func (c *Client) post(ctx context.Context, path string, form url.Values) (int, []byte, error) {
	base := c.BaseURL
	if base == "" {
		base = ProductionBaseURL
	}
	if !strings.HasPrefix(base, "https://") {
		return 0, nil, ErrCredentials
	}
	client := c.HTTP
	if client == nil {
		client = &http.Client{Timeout: requestTimeout}
	}
	// Never follow redirects, without mutating the caller's client.
	pinned := *client
	pinned.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	if pinned.Timeout == 0 {
		pinned.Timeout = requestTimeout
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(base, "/")+path, strings.NewReader(form.Encode()))
	if err != nil {
		return 0, nil, ErrUnavailable
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "abastevo-backend")
	resp, err := pinned.Do(req)
	if err != nil {
		return 0, nil, ErrUnavailable
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		return 0, nil, ErrUnavailable
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil || len(body) > maxResponseBytes {
		return 0, nil, ErrUnavailable
	}
	return resp.StatusCode, body, nil
}
