package apple

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	testTeam    = "TEAM123456"
	testKeyID   = "KEY1234567"
	testClient  = "com.anpfuel.app"
	testSubject = "001234.synthetic.subject"
	testCode    = "c0ffee.0.synthetic-code"
	testRefresh = "r0synthetic.refresh-token_value"
)

var testNow = time.Unix(1_800_000_000, 0)

func testKey(t *testing.T) *ecdsa.PrivateKey {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func testSealer(t *testing.T) *Sealer {
	t.Helper()
	s, err := NewSealer(bytes.Repeat([]byte{7}, 32))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// segment encodes one JWT segment; the inputs are plain maps that always
// marshal, so the fake server never needs a test-goroutine failure.
func segment(v any) string {
	raw, _ := json.Marshal(v)
	return base64.RawURLEncoding.EncodeToString(raw)
}

// secretError checks the client secret exactly as Apple documents it.
func secretError(secret string, pub *ecdsa.PublicKey) error {
	parts := strings.Split(secret, ".")
	if len(parts) != 3 {
		return errors.New("client secret is not a compact JWS")
	}
	var header map[string]string
	raw, _ := base64.RawURLEncoding.DecodeString(parts[0])
	if err := json.Unmarshal(raw, &header); err != nil || header["alg"] != "ES256" || header["kid"] != testKeyID {
		return fmt.Errorf("header = %s", raw)
	}
	var claims map[string]any
	raw, _ = base64.RawURLEncoding.DecodeString(parts[1])
	if err := json.Unmarshal(raw, &claims); err != nil {
		return err
	}
	if claims["iss"] != testTeam || claims["sub"] != testClient || claims["aud"] != "https://appleid.apple.com" ||
		claims["iat"] != float64(testNow.Unix()) || claims["exp"] != float64(testNow.Add(5*time.Minute).Unix()) {
		return fmt.Errorf("claims = %s", raw)
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || len(sig) != 64 {
		return errors.New("signature must be raw R||S")
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if !ecdsa.Verify(pub, digest[:], new(big.Int).SetBytes(sig[:32]), new(big.Int).SetBytes(sig[32:])) {
		return errors.New("client secret signature does not verify")
	}
	return nil
}

type fakeApple struct {
	t          *testing.T
	pub        *ecdsa.PublicKey
	mu         sync.Mutex
	tokenCalls int
	revoked    []string
	status     int
	idSubject  string
	idAudience any
	idIssuer   string
	refresh    string
	redirect   bool
	huge       bool
}

func (f *fakeApple) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if f.redirect {
		http.Redirect(w, r, "https://attacker.example.invalid/auth/token", http.StatusFound)
		return
	}
	if r.PostForm.Get("client_id") != testClient {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_client"}`))
		return
	}
	if err := secretError(r.PostForm.Get("client_secret"), f.pub); err != nil {
		f.t.Error(err)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_client"}`))
		return
	}
	if f.status != 0 {
		w.WriteHeader(f.status)
		_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
		return
	}
	switch r.URL.Path {
	case "/auth/token":
		f.tokenCalls++
		if r.PostForm.Get("grant_type") != "authorization_code" || r.PostForm.Get("code") != testCode || r.PostForm.Has("redirect_uri") {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
			return
		}
		if f.huge {
			_, _ = w.Write(bytes.Repeat([]byte("x"), maxResponseBytes+10))
			return
		}
		idToken := segment(map[string]string{"alg": "RS256", "kid": "apple"}) + "." +
			segment(map[string]any{"iss": f.idIssuer, "aud": f.idAudience, "sub": f.idSubject, "exp": testNow.Add(time.Hour).Unix()}) + ".c2ln"
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "a0synthetic", "token_type": "Bearer", "expires_in": 3600,
			"refresh_token": f.refresh, "id_token": idToken,
		})
	case "/auth/revoke":
		if r.PostForm.Get("token_type_hint") != "refresh_token" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		f.revoked = append(f.revoked, r.PostForm.Get("token"))
		w.WriteHeader(http.StatusOK)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func newFakeClient(t *testing.T) (*Client, *fakeApple) {
	t.Helper()
	key := testKey(t)
	fake := &fakeApple{t: t, pub: &key.PublicKey, idSubject: testSubject, idAudience: testClient, idIssuer: "https://appleid.apple.com", refresh: testRefresh}
	srv := httptest.NewTLSServer(fake)
	t.Cleanup(srv.Close)
	return &Client{
		BaseURL:     srv.URL,
		Credentials: Credentials{TeamID: testTeam, KeyID: testKeyID, ClientID: testClient, Key: key},
		Sealer:      testSealer(t),
		HTTP:        srv.Client(),
		Now:         func() time.Time { return testNow },
	}, fake
}

func TestClientSecretMatchesAppleContract(t *testing.T) {
	key := testKey(t)
	secret, err := Credentials{TeamID: testTeam, KeyID: testKeyID, ClientID: testClient, Key: key}.ClientSecret(testNow)
	if err != nil {
		t.Fatal(err)
	}
	if err := secretError(secret, &key.PublicKey); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []Credentials{
		{KeyID: testKeyID, ClientID: testClient, Key: key},
		{TeamID: testTeam, ClientID: testClient, Key: key},
		{TeamID: testTeam, KeyID: testKeyID, Key: key},
		{TeamID: testTeam, KeyID: testKeyID, ClientID: testClient},
	} {
		if _, err := bad.ClientSecret(testNow); !errors.Is(err, ErrCredentials) {
			t.Fatalf("incomplete credentials minted a secret: %v", err)
		}
	}
}

func TestExchangeSealsRefreshTokenBoundToGrant(t *testing.T) {
	c, fake := newFakeClient(t)
	sealed, err := c.Exchange(context.Background(), "apple", testCode, testSubject, "grant-a")
	if err != nil {
		t.Fatalf("exchange: %v", err)
	}
	if bytes.Contains(sealed, []byte(testRefresh)) {
		t.Fatal("sealed grant contains the plaintext token")
	}
	if got, err := c.Sealer.Open(sealed, "grant-a"); err != nil || got != testRefresh {
		t.Fatalf("open: %q %v", got, err)
	}
	if _, err := c.Sealer.Open(sealed, "grant-b"); !errors.Is(err, ErrSealed) {
		t.Fatal("sealed grant opened under another binding")
	}
	tampered := append([]byte(nil), sealed...)
	tampered[len(tampered)-1] ^= 1
	if _, err := c.Sealer.Open(tampered, "grant-a"); !errors.Is(err, ErrSealed) {
		t.Fatal("tampered grant opened")
	}
	if fake.tokenCalls != 1 {
		t.Fatalf("token calls = %d", fake.tokenCalls)
	}
}

func TestExchangeRefusesForeignIdentityOrRefusal(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*fakeApple)
		want   error
	}{
		{"other subject", func(f *fakeApple) { f.idSubject = "someone-else" }, ErrExchange},
		{"other audience", func(f *fakeApple) { f.idAudience = "com.example.other" }, ErrExchange},
		{"audience list without client", func(f *fakeApple) { f.idAudience = []string{"com.example.other"} }, ErrExchange},
		{"other issuer", func(f *fakeApple) { f.idIssuer = "https://appleid.example.invalid" }, ErrExchange},
		{"missing refresh token", func(f *fakeApple) { f.refresh = "" }, ErrExchange},
		{"invalid grant", func(f *fakeApple) { f.status = http.StatusBadRequest }, ErrExchange},
		{"apple outage", func(f *fakeApple) { f.status = http.StatusServiceUnavailable }, ErrUnavailable},
		{"rate limited", func(f *fakeApple) { f.status = http.StatusTooManyRequests }, ErrUnavailable},
		{"redirect never followed", func(f *fakeApple) { f.redirect = true }, ErrUnavailable},
		{"oversized response", func(f *fakeApple) { f.huge = true }, ErrUnavailable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, fake := newFakeClient(t)
			tc.mutate(fake)
			sealed, err := c.Exchange(context.Background(), "apple", testCode, testSubject, "grant-a")
			if !errors.Is(err, tc.want) || sealed != nil {
				t.Fatalf("want %v, got %v", tc.want, err)
			}
			if strings.Contains(err.Error(), testCode) || strings.Contains(err.Error(), testRefresh) {
				t.Fatal("error leaked a code or token")
			}
		})
	}
	c, _ := newFakeClient(t)
	if _, err := c.Exchange(context.Background(), "google", testCode, testSubject, "grant-a"); !errors.Is(err, ErrExchange) {
		t.Fatalf("non-apple provider: %v", err)
	}
	c.Credentials.ClientID = "com.example.wrong"
	if _, err := c.Exchange(context.Background(), "apple", testCode, testSubject, "grant-a"); !errors.Is(err, ErrExchange) {
		t.Fatalf("invalid client: %v", err)
	}
	c.BaseURL = "http://appleid.example.invalid"
	if _, err := c.Exchange(context.Background(), "apple", testCode, testSubject, "grant-a"); !errors.Is(err, ErrCredentials) {
		t.Fatalf("plain HTTP endpoint must be refused: %v", err)
	}
}

func TestRevokeSendsOpenedRefreshToken(t *testing.T) {
	c, fake := newFakeClient(t)
	sealed, err := c.Sealer.Seal(testRefresh, "grant-r")
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Revoke(context.Background(), "apple", sealed, "grant-r"); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if len(fake.revoked) != 1 || fake.revoked[0] != testRefresh {
		t.Fatalf("revoked = %v", fake.revoked)
	}
	if err := c.Revoke(context.Background(), "apple", sealed, "grant-other"); !errors.Is(err, ErrSealed) {
		t.Fatalf("foreign binding must not reach Apple: %v", err)
	}
	if len(fake.revoked) != 1 {
		t.Fatal("unreadable grant reached the revoke endpoint")
	}
	fake.status = http.StatusBadRequest
	if err := c.Revoke(context.Background(), "apple", sealed, "grant-r"); !errors.Is(err, ErrRevoke) {
		t.Fatalf("refusal: %v", err)
	}
	fake.status = http.StatusBadGateway
	if err := c.Revoke(context.Background(), "apple", sealed, "grant-r"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("outage: %v", err)
	}
}

func writePEM(t *testing.T, dir, name, typ string, der []byte) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: typ, Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadCredentialsValidatesKeyMaterial(t *testing.T) {
	dir := t.TempDir()
	der, err := x509.MarshalPKCS8PrivateKey(testKey(t))
	if err != nil {
		t.Fatal(err)
	}
	p8 := writePEM(t, dir, "AuthKey.p8", "PRIVATE KEY", der)
	grantKey := filepath.Join(dir, "grant.key")
	if err := os.WriteFile(grantKey, bytes.Repeat([]byte{3}, 32), 0o600); err != nil {
		t.Fatal(err)
	}
	creds, sealer, err := LoadCredentials(testTeam, testKeyID, testClient, p8, grantKey)
	if err != nil || sealer == nil || creds.Key == nil {
		t.Fatalf("load: %v", err)
	}

	p384, _ := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	der384, _ := x509.MarshalPKCS8PrivateKey(p384)
	rsaKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	derRSA, _ := x509.MarshalPKCS8PrivateKey(rsaKey)
	shortKey := filepath.Join(dir, "short.key")
	_ = os.WriteFile(shortKey, bytes.Repeat([]byte{3}, 31), 0o600)
	garbage := filepath.Join(dir, "garbage.p8")
	_ = os.WriteFile(garbage, []byte("not a key"), 0o600)
	for name, paths := range map[string][2]string{
		"p384 key":         {writePEM(t, dir, "p384.p8", "PRIVATE KEY", der384), grantKey},
		"rsa key":          {writePEM(t, dir, "rsa.p8", "PRIVATE KEY", derRSA), grantKey},
		"wrong pem type":   {writePEM(t, dir, "ec.pem", "EC PRIVATE KEY", der), grantKey},
		"garbage":          {garbage, grantKey},
		"missing key file": {filepath.Join(dir, "absent.p8"), grantKey},
		"short grant key":  {p8, shortKey},
		"grant key is dir": {p8, dir},
	} {
		if _, _, err := LoadCredentials(testTeam, testKeyID, testClient, paths[0], paths[1]); !errors.Is(err, ErrCredentials) {
			t.Fatalf("%s: want credentials error, got %v", name, err)
		} else if strings.Contains(err.Error(), dir) {
			t.Fatalf("%s: error echoed a path", name)
		}
	}
}

func TestSealerRefusesBadInput(t *testing.T) {
	s := testSealer(t)
	for _, token := range []string{"", "has space", strings.Repeat("t", maxTokenBytes+1)} {
		if _, err := s.Seal(token, "aad"); !errors.Is(err, ErrSealed) {
			t.Fatalf("sealed %q", token)
		}
	}
	if _, err := s.Seal(testRefresh, ""); !errors.Is(err, ErrSealed) {
		t.Fatal("sealed without a binding")
	}
	if _, err := s.Open([]byte{2, 1, 2, 3}, "aad"); !errors.Is(err, ErrSealed) {
		t.Fatal("opened a foreign version")
	}
	if _, err := NewSealer(make([]byte, 16)); !errors.Is(err, ErrCredentials) {
		t.Fatal("accepted a 16-byte key")
	}
}
