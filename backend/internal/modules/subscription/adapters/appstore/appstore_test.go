package appstore

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/subscription/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/subscription/domain"
)

const (
	bundle   = "com.anpfuel.app"
	product  = "com.anpfuel.app.premium.monthly"
	original = "2000000123456789"
	binding  = "6f1c2b8e-6d2a-8c3e-9a41-2b7d5e0c1f34"
)

var testNow = time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)

type pki struct {
	roots    *x509.CertPool
	leafKey  *ecdsa.PrivateKey
	x5c      []string
	rootCert *x509.Certificate
}

func certificate(t *testing.T, template, parent *x509.Certificate, pub, signer any) *x509.Certificate {
	t.Helper()
	der, err := x509.CreateCertificate(rand.Reader, template, parent, pub, signer)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return cert
}

// newPKI builds a synthetic Apple-shaped chain. Extension OIDs are optional
// so tests can prove each marker is required.
func newPKI(t *testing.T, leafOID, intermediateOID bool) pki {
	t.Helper()
	key := func() *ecdsa.PrivateKey {
		k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		return k
	}
	rootKey, interKey, leafKey := key(), key(), key()
	window := func(serial int64, name string, ca bool) *x509.Certificate {
		return &x509.Certificate{SerialNumber: big.NewInt(serial), Subject: pkix.Name{CommonName: name},
			NotBefore: testNow.Add(-24 * time.Hour), NotAfter: testNow.Add(365 * 24 * time.Hour),
			IsCA: ca, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature}
	}
	root := certificate(t, window(1, "Synthetic Root", true), window(1, "Synthetic Root", true), &rootKey.PublicKey, rootKey)
	interTemplate := window(2, "Synthetic WWDR", true)
	if intermediateOID {
		interTemplate.ExtraExtensions = []pkix.Extension{{Id: asn1.ObjectIdentifier{1, 2, 840, 113635, 100, 6, 2, 1}, Value: []byte{5, 0}}}
	}
	inter := certificate(t, interTemplate, root, &interKey.PublicKey, rootKey)
	leafTemplate := window(3, "Synthetic Store Signing", false)
	if leafOID {
		leafTemplate.ExtraExtensions = []pkix.Extension{{Id: asn1.ObjectIdentifier{1, 2, 840, 113635, 100, 6, 11, 1}, Value: []byte{5, 0}}}
	}
	leaf := certificate(t, leafTemplate, inter, &leafKey.PublicKey, interKey)
	roots := x509.NewCertPool()
	roots.AddCert(root)
	enc := base64.StdEncoding.EncodeToString
	return pki{roots: roots, leafKey: leafKey, rootCert: root, x5c: []string{enc(leaf.Raw), enc(inter.Raw), enc(root.Raw)}}
}

func (p pki) sign(t *testing.T, payload any) string {
	t.Helper()
	header, _ := json.Marshal(map[string]any{"alg": "ES256", "x5c": p.x5c})
	body, _ := json.Marshal(payload)
	input := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(body)
	digest := sha256.Sum256([]byte(input))
	r, s, err := ecdsa.Sign(rand.Reader, p.leafKey, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	sig := make([]byte, 64)
	r.FillBytes(sig[:32])
	s.FillBytes(sig[32:])
	return input + "." + base64.RawURLEncoding.EncodeToString(sig)
}

func transaction(environment string) map[string]any {
	return map[string]any{
		"transactionId": "2000000999", "originalTransactionId": original, "bundleId": bundle,
		"productId": product, "expiresDate": testNow.Add(20 * 24 * time.Hour).UnixMilli(),
		"type": "Auto-Renewable Subscription", "appAccountToken": binding,
		"inAppOwnershipType": "PURCHASED", "environment": environment, "signedDate": testNow.UnixMilli(),
	}
}

func renewal(environment string) map[string]any {
	return map[string]any{"originalTransactionId": original, "autoRenewStatus": 1, "environment": environment}
}

func statusBody(t *testing.T, p pki, environment string, status int, tx, rn map[string]any) []byte {
	t.Helper()
	body, _ := json.Marshal(map[string]any{
		"environment": environment, "bundleId": bundle, "appAppleId": 1,
		"data": []any{map[string]any{"subscriptionGroupIdentifier": "21000000", "lastTransactions": []any{map[string]any{
			"originalTransactionId": original, "status": status,
			"signedTransactionInfo": p.sign(t, tx), "signedRenewalInfo": p.sign(t, rn),
		}}}},
	})
	return body
}

func p8(t *testing.T) ([]byte, *ecdsa.PrivateKey) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), key
}

type route struct {
	status int
	body   []byte
}

func newClient(t *testing.T, p pki, environment string, fallback bool, production, sandbox route) (*Client, *ecdsa.PrivateKey, *[]string) {
	t.Helper()
	seen := []string{}
	serve := func(r route, name string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			seen = append(seen, name+" "+req.URL.Path+" "+req.Header.Get("Authorization"))
			w.WriteHeader(r.status)
			_, _ = w.Write(r.body)
		}))
	}
	prod, sand := serve(production, "production"), serve(sandbox, "sandbox")
	t.Cleanup(prod.Close)
	t.Cleanup(sand.Close)
	pemKey, key := p8(t)
	c, err := New(Config{BundleID: bundle, Environment: environment, IssuerID: "57246542-96fe-1a63-e053-0824d011072a",
		KeyID: "2X9R4HXF34", PrivateKeyPEM: pemKey, Products: []string{product}, SandboxFallback: fallback}, nil, p.roots)
	if err != nil {
		t.Fatal(err)
	}
	c.Now = func() time.Time { return testNow }
	c.Authorities = [2]string{prod.URL, sand.URL}
	return c, key, &seen
}

func request(environment string) app.ProviderRequest {
	return app.ProviderRequest{Token: original, ExpectedAccount: binding, Environment: environment, PackageName: bundle}
}

func TestPinnedAppleRootFingerprint(t *testing.T) {
	sum := sha256.Sum256(appleRootG3)
	if hex.EncodeToString(sum[:]) != "63343abfb89a6a03ebb57e9b3f5fa7be7c4f5c756f3017b3a8c488c3653e9179" {
		t.Fatal("embedded Apple Root CA - G3 changed")
	}
	if _, err := AppleRoots(); err != nil {
		t.Fatal(err)
	}
}

func TestVerifyActiveSubscriptionBindsAccountAndProduct(t *testing.T) {
	p := newPKI(t, true, true)
	c, apiKey, seen := newClient(t, p, "production", false,
		route{200, statusBody(t, p, "Production", 1, transaction("Production"), renewal("Production"))}, route{404, nil})
	got, err := c.Verify(context.Background(), request("production"))
	if err != nil {
		t.Fatal(err)
	}
	s := got.Snapshot
	if s.Provider != domain.ProviderAppStore || s.State != domain.Active || s.ProductID != product || s.BasePlanID != product || !s.Acknowledged {
		t.Fatalf("snapshot %+v", s)
	}
	if !s.ValidUntil.Equal(testNow.Add(20 * 24 * time.Hour).Truncate(time.Millisecond)) {
		t.Fatalf("valid until %s", s.ValidUntil)
	}
	if len(*seen) != 1 || !strings.HasPrefix((*seen)[0], "production /inApps/v1/subscriptions/"+original+" Bearer ") {
		t.Fatalf("requests %v", *seen)
	}
	assertAPIToken(t, strings.TrimPrefix(strings.SplitN((*seen)[0], " ", 3)[2], "Bearer "), apiKey)
}

func assertAPIToken(t *testing.T, token string, key *ecdsa.PrivateKey) {
	t.Helper()
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("token %q", token)
	}
	var header, claims map[string]any
	rawHeader, _ := base64.RawURLEncoding.DecodeString(parts[0])
	rawClaims, _ := base64.RawURLEncoding.DecodeString(parts[1])
	_ = json.Unmarshal(rawHeader, &header)
	_ = json.Unmarshal(rawClaims, &claims)
	if header["alg"] != "ES256" || header["kid"] != "2X9R4HXF34" || header["typ"] != "JWT" {
		t.Fatalf("header %v", header)
	}
	if claims["aud"] != "appstoreconnect-v1" || claims["bid"] != bundle || claims["iss"] != "57246542-96fe-1a63-e053-0824d011072a" {
		t.Fatalf("claims %v", claims)
	}
	if exp, iat := claims["exp"].(float64), claims["iat"].(float64); exp-iat <= 0 || exp-iat > 3600 {
		t.Fatalf("lifetime %v", exp-iat)
	}
	sig, _ := base64.RawURLEncoding.DecodeString(parts[2])
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if len(sig) != 64 || !ecdsa.Verify(&key.PublicKey, digest[:], new(big.Int).SetBytes(sig[:32]), new(big.Int).SetBytes(sig[32:])) {
		t.Fatal("API token signature invalid")
	}
}

func TestVerifyMapsRenewalGraceAndRevocation(t *testing.T) {
	p := newPKI(t, true, true)
	grace := testNow.Add(30 * 24 * time.Hour)
	cases := []struct {
		name   string
		status int
		tx     func(map[string]any)
		rn     func(map[string]any)
		state  domain.State
		until  time.Time
	}{
		{"renewal off", 1, nil, func(r map[string]any) { r["autoRenewStatus"] = 0 }, domain.Canceled, testNow.Add(20 * 24 * time.Hour)},
		{"grace", 4, nil, func(r map[string]any) { r["gracePeriodExpiresDate"] = grace.UnixMilli() }, domain.Grace, grace},
		{"billing retry", 3, nil, nil, domain.Hold, testNow.Add(20 * 24 * time.Hour)},
		{"expired", 2, nil, nil, domain.Expired, testNow.Add(20 * 24 * time.Hour)},
		{"refunded", 1, func(tx map[string]any) { tx["revocationDate"] = testNow.UnixMilli() }, nil, domain.Revoked, testNow.Add(20 * 24 * time.Hour)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tx, rn := transaction("Sandbox"), renewal("Sandbox")
			if tc.tx != nil {
				tc.tx(tx)
			}
			if tc.rn != nil {
				tc.rn(rn)
			}
			c, _, _ := newClient(t, p, "test", false, route{500, nil}, route{200, statusBody(t, p, "Sandbox", tc.status, tx, rn)})
			got, err := c.Verify(context.Background(), request("test"))
			if err != nil {
				t.Fatal(err)
			}
			if got.Snapshot.State != tc.state || !got.Snapshot.ValidUntil.Equal(tc.until.Truncate(time.Millisecond)) {
				t.Fatalf("got %s until %s", got.Snapshot.State, got.Snapshot.ValidUntil)
			}
		})
	}
}

func TestVerifyDeniesForeignOrUnsellablePurchases(t *testing.T) {
	p := newPKI(t, true, true)
	cases := map[string]func(tx map[string]any){
		"other account":   func(tx map[string]any) { tx["appAccountToken"] = "11111111-2222-8333-8444-555555555555" },
		"no binding":      func(tx map[string]any) { delete(tx, "appAccountToken") },
		"unknown product": func(tx map[string]any) { tx["productId"] = "com.anpfuel.app.other" },
		"other app":       func(tx map[string]any) { tx["bundleId"] = "com.example.other" },
		"other env":       func(tx map[string]any) { tx["environment"] = "Production" },
		"not a sub":       func(tx map[string]any) { tx["type"] = "Non-Consumable" },
		"other chain":     func(tx map[string]any) { tx["originalTransactionId"] = "2000000000000001" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			tx := transaction("Sandbox")
			mutate(tx)
			c, _, _ := newClient(t, p, "test", false, route{500, nil}, route{200, statusBody(t, p, "Sandbox", 1, tx, renewal("Sandbox"))})
			if _, err := c.Verify(context.Background(), request("test")); !errors.Is(err, domain.ErrDenied) {
				t.Fatalf("got %v", err)
			}
		})
	}
	c, _, _ := newClient(t, p, "test", false, route{500, nil}, route{200, nil})
	if _, err := c.Verify(context.Background(), app.ProviderRequest{Token: "abc", ExpectedAccount: binding, Environment: "test", PackageName: bundle}); !errors.Is(err, domain.ErrDenied) {
		t.Fatalf("non-numeric transaction id: %v", err)
	}
}

func TestSignedDataMustChainToPinnedRootWithAppleMarkers(t *testing.T) {
	trusted := newPKI(t, true, true)
	foreign := newPKI(t, true, true)
	noLeafMarker := newPKI(t, false, true)
	noIntermediateMarker := newPKI(t, true, false)
	verifier := SignedDataVerifier{Roots: trusted.roots, Now: func() time.Time { return testNow }}
	if _, err := verifier.Verify(trusted.sign(t, transaction("Sandbox"))); err != nil {
		t.Fatal(err)
	}
	for name, signer := range map[string]pki{"foreign root": foreign, "leaf marker": noLeafMarker, "intermediate marker": noIntermediateMarker} {
		v := verifier
		if name != "foreign root" {
			v.Roots = signer.roots
		}
		if _, err := v.Verify(signer.sign(t, transaction("Sandbox"))); err == nil {
			t.Fatalf("%s accepted", name)
		}
	}
	signed := trusted.sign(t, transaction("Sandbox"))
	parts := strings.Split(signed, ".")
	tampered, _ := json.Marshal(transaction("Production"))
	if _, err := verifier.Verify(parts[0] + "." + base64.RawURLEncoding.EncodeToString(tampered) + "." + parts[2]); err == nil {
		t.Fatal("tampered payload accepted")
	}
	expired := verifier
	expired.Now = func() time.Time { return testNow.Add(2 * 365 * 24 * time.Hour) }
	if _, err := expired.Verify(signed); err == nil {
		t.Fatal("expired chain accepted")
	}
}

func TestProductionFallsBackToSandboxOnlyWhenEnabled(t *testing.T) {
	p := newPKI(t, true, true)
	sandbox := route{200, statusBody(t, p, "Sandbox", 1, transaction("Sandbox"), renewal("Sandbox"))}
	strict, _, seen := newClient(t, p, "production", false, route{404, []byte(`{"errorCode":4040010}`)}, sandbox)
	if _, err := strict.Verify(context.Background(), request("production")); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("strict production: %v", err)
	}
	if len(*seen) != 1 {
		t.Fatalf("sandbox must not be called: %v", *seen)
	}
	review, _, seen := newClient(t, p, "production", true, route{404, nil}, sandbox)
	got, err := review.Verify(context.Background(), request("production"))
	if err != nil || got.Snapshot.State != domain.Active {
		t.Fatalf("fallback: %+v %v", got, err)
	}
	if len(*seen) != 2 {
		t.Fatalf("requests %v", *seen)
	}
	mismatch, _, _ := newClient(t, p, "production", false,
		route{200, statusBody(t, p, "Production", 1, transaction("Sandbox"), renewal("Sandbox"))}, route{404, nil})
	if _, err := mismatch.Verify(context.Background(), request("production")); !errors.Is(err, domain.ErrDenied) {
		t.Fatalf("sandbox transaction in production response: %v", err)
	}
}

func TestProviderFailuresStayRetryable(t *testing.T) {
	p := newPKI(t, true, true)
	for _, status := range []int{401, 429, 500, 503} {
		c, _, _ := newClient(t, p, "test", false, route{500, nil}, route{status, nil})
		if _, err := c.Verify(context.Background(), request("test")); !errors.Is(err, domain.ErrUnavailable) {
			t.Fatalf("%d: %v", status, err)
		}
	}
	c, _, _ := newClient(t, p, "test", false, route{500, nil}, route{400, nil})
	if _, err := c.Verify(context.Background(), request("test")); !errors.Is(err, domain.ErrDenied) {
		t.Fatalf("400: %v", err)
	}
	if _, err := c.Verify(context.Background(), app.ProviderRequest{Token: original, ExpectedAccount: binding, Environment: "production", PackageName: bundle}); !errors.Is(err, domain.ErrDenied) {
		t.Fatalf("environment mismatch: %v", err)
	}
}

func TestNotificationTarget(t *testing.T) {
	p := newPKI(t, true, true)
	c, _, _ := newClient(t, p, "production", false, route{404, nil}, route{404, nil})
	note := func(env string) Notification {
		n := Notification{NotificationType: "DID_RENEW", NotificationUUID: "002e14d5-51f5-4503-b5a8-c3a1af68eb20", Version: "2.0"}
		n.Data = &struct {
			AppAppleID            int64  `json:"appAppleId"`
			BundleID              string `json:"bundleId"`
			Environment           string `json:"environment"`
			SignedTransactionInfo string `json:"signedTransactionInfo"`
		}{1, bundle, env, p.sign(t, transaction(env))}
		return n
	}
	if got, err := c.NotificationTarget(note("Production")); err != nil || got != original {
		t.Fatalf("got %q %v", got, err)
	}
	if _, err := c.NotificationTarget(note("Sandbox")); !errors.Is(err, domain.ErrDenied) {
		t.Fatalf("sandbox notice in strict production: %v", err)
	}
	test := Notification{NotificationType: "TEST", NotificationUUID: "002e14d5-51f5-4503-b5a8-c3a1af68eb21", Version: "2.0"}
	if got, err := c.NotificationTarget(test); err != nil || got != "" {
		t.Fatalf("test notice %q %v", got, err)
	}
	wrong := note("Production")
	wrong.Data.BundleID = "com.example.other"
	if _, err := c.NotificationTarget(wrong); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("other app: %v", err)
	}
	old := note("Production")
	old.Version = "1.0"
	if _, err := c.NotificationTarget(old); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("v1 notice: %v", err)
	}
}

func TestNewRejectsIncompleteConfiguration(t *testing.T) {
	pemKey, _ := p8(t)
	good := Config{BundleID: bundle, Environment: "test", IssuerID: "issuer", KeyID: "KEY", PrivateKeyPEM: pemKey, Products: []string{product}}
	if _, err := New(good, nil, nil); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*Config){
		"bundle":      func(c *Config) { c.BundleID = "" },
		"environment": func(c *Config) { c.Environment = "staging" },
		"issuer":      func(c *Config) { c.IssuerID = "" },
		"key id":      func(c *Config) { c.KeyID = "has space" },
		"products":    func(c *Config) { c.Products = nil },
		"key":         func(c *Config) { c.PrivateKeyPEM = []byte("not a key") },
	} {
		c := good
		mutate(&c)
		if _, err := New(c, nil, nil); !errors.Is(err, domain.ErrInvalid) {
			t.Fatalf("%s: %v", name, err)
		}
	}
}
