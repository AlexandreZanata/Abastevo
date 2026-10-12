//go:build integration

package http

import (
	"context"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	identityadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/adapters"
	identitydomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/domain"
	privacyadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/privacy/adapters"
	privacyapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/privacy/application"
	legal "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/legalterms"
)

type publishedConsentTerms struct{}

func (publishedConsentTerms) CurrentVersion() string { return legal.Version }
func (publishedConsentTerms) CurrentDigest() string  { return legal.Digest }

// Opt-in cross-runtime proof: the production Kotlin transport signs real TLS
// requests verified by the production Go verifier and disposable PostgreSQL.
// Generated synthetic private keys exist only in an owner-only temporary file.
func TestAndroidConsentInteroperability(t *testing.T) {
	if os.Getenv("ANPFUEL_ANDROID_CONSENT_INTEROP") != "1" {
		t.Skip("explicit Android cross-runtime harness required")
	}
	server := httptest.NewUnstartedServer(http.NotFoundHandler())
	authority := server.Listener.Addr().String()
	router, pool := signedConsentFixtureFor(t, authority, publishedConsentTerms{})
	owner := enrollSignedCaller(t, pool)
	other := enrollSignedCaller(t, pool)
	registrar := identityadapters.NewRegistrar(pool)
	ports := privacyapp.ConsentPorts{Clock: time.Now, Store: privacyadapters.NewConsentStore(pool), Terms: publishedConsentTerms{}, Attribution: registrar.AttributionToken}
	if _, err := privacyapp.GrantConsent(context.Background(), ports, privacyapp.Caller{ContributorID: owner.id}, privacyapp.GrantDTO{TermsVersion: legal.Version, TermsDigest: legal.Digest, Revision: 100}); err != nil {
		t.Fatal(err)
	}
	router.Get("/fixture/challenge/{owner}", func(w http.ResponseWriter, r *http.Request) {
		fp := owner.fp
		if strings.HasSuffix(r.URL.Path, "/other") {
			fp = other.fp
		}
		challenge, err := registrar.IssueChallenge(r.Context(), fp, identitydomain.PurposeSign)
		if err != nil {
			http.Error(w, "challenge unavailable", 503)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"nonce": challenge.Nonce})
	})
	server.Config.Handler = router
	server.StartTLS()
	defer server.Close()
	dir := t.TempDir()
	key := func(c *signedCaller) map[string]string {
		raw, err := x509.MarshalPKCS8PrivateKey(c.priv)
		if err != nil {
			t.Fatal(err)
		}
		pub, err := c.priv.PublicKey.Bytes()
		if err != nil {
			t.Fatal(err)
		}
		enc := base64.RawURLEncoding.EncodeToString
		return map[string]string{"key": base64.StdEncoding.EncodeToString(raw), "x": enc(pub[1:33]), "y": enc(pub[33:65]), "fingerprint": c.fp}
	}
	fixture := map[string]any{"origin": server.URL, "owner": key(owner), "other": key(other), "certificate": string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})), "digest": legal.Digest}
	raw, err := json.Marshal(fixture)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "synthetic-consent.json")
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	root, err := filepath.Abs("../../../../../..")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, filepath.Join(root, "gradlew"), "--offline", "--max-workers=2", ":data:testDebugUnitTest", "--tests", "*TrainingConsentBackendInteropTest", "--console=plain")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "ANPFUEL_ANDROID_CONSENT_FIXTURE="+path)
	log, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Android interoperability failed: %v\n%s", err, log)
	}
	t.Log("Kotlin transport: real TLS grant/read/revoke/replay/stale/other-owner/unsigned proof passed")
	view, err := privacyapp.ReadConsent(context.Background(), ports, privacyapp.Caller{ContributorID: owner.id})
	if err != nil {
		t.Fatal(err)
	}
	if view.State != "REVOKED" || view.Revision != 102 || view.Granted {
		t.Fatalf("unexpected final consent %s/%d", view.State, view.Revision)
	}
}
