//go:build integration && nativefixture

package adapters_test

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	accountpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/adapters"
	accountweb "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/adapters/http"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/adapters/keyprover"
	identitypg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/adapters"
	identityweb "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/adapters/http"
	identity "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/enterprisesuite"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

type nativeContactClock struct{}

func (nativeContactClock) NowUnix() int64 { return time.Now().Unix() }

// TestDeviceContactNativeSourceFixture is an opt-in real source server, not a
// test-control API. Normal suites do not build the nativefixture tag. A caller
// provides a new private /tmp directory, consumes fixture.json without logging
// its credentials, and creates stop only after native acceptance completes.
func TestDeviceContactNativeSourceFixture(t *testing.T) {
	dir := os.Getenv("EMP_PRIV003_NATIVE_FIXTURE_DIR")
	if !filepath.IsAbs(dir) || !strings.HasPrefix(filepath.Clean(dir), "/tmp/") {
		t.Fatal("native fixture requires a private absolute /tmp directory")
	}
	if e := os.MkdirAll(dir, 0700); e != nil {
		t.Fatal(e)
	}
	info, e := os.Stat(dir)
	if e != nil || info.Mode().Perm() != 0700 {
		t.Fatal("native fixture directory must have mode0700")
	}
	fx := devicesFixture(t)
	role := pgx.Identifier{fx.f.RuntimeRole}.Sanitize()
	if _, e := fx.f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT,UPDATE ON identity_rate_windows,identity_contributors,identity_keys,identity_challenges,identity_idempotency,account_contributor_bindings,account_binding_audit,account_session_refresh_history TO "+role); e != nil {
		t.Fatal(e)
	}
	var super, bypass bool
	if e := fx.f.Runtime.QueryRow(t.Context(), `SELECT rolsuper,rolbypassrls FROM pg_roles WHERE rolname=current_user`).Scan(&super, &bypass); e != nil || super || bypass {
		t.Fatal("native fixture runtime must be ordinary", e)
	}
	const host = "api.anpfuel.example.invalid"
	const origin = "https://" + host
	registrar := identitypg.NewRegistrar(fx.f.Runtime)
	accounts := *fx.accounts
	accounts.Store = accountpg.NewPGStore(fx.f.Runtime)
	accounts.Clock = nativeContactClock{}
	accounts.KeyProver = keyprover.Prover{Now: time.Now, Keys: func(ctx context.Context, fp string) (string, string, string, bool, error) {
		k, e := registrar.FindDeviceKey(ctx, fp)
		return k.ContributorID, k.JWKX, k.JWKY, k.Revoked, e
	}}
	router := chi.NewRouter()
	secret := bytes.Repeat([]byte{89}, 32)
	identityweb.Handler{Registrar: registrar, Authority: host, QuotaSecret: secret, CheckQuota: identitypg.NewLimiter(fx.f.Runtime, identity.DefaultQuotaPolicy()).Check}.RegisterRoutes(router)
	accountweb.Handler{Service: &accounts}.RegisterRoutes(router)
	if e := enterprisesuite.Register(router, enterprisesuite.Options{Enabled: true, Origin: origin, CursorSecret: secret, Pool: fx.f.Runtime, Accounts: &accounts}); e != nil {
		t.Fatal(e)
	}
	key, e := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	serial, e := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if e != nil {
		t.Fatal(e)
	}
	template := &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: host}, DNSNames: []string{host}, NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(3 * time.Hour), KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, IsCA: true, BasicConstraintsValid: true}
	der, e := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if e != nil {
		t.Fatal(e)
	}
	private, e := x509.MarshalECPrivateKey(key)
	if e != nil {
		t.Fatal(e)
	}
	publicPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	pair, e := tls.X509KeyPair(publicPEM, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: private}))
	if e != nil {
		t.Fatal(e)
	}
	server := httptest.NewUnstartedServer(router)
	server.TLS = &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{pair}}
	server.StartTLS()
	defer server.Close()
	port := server.Listener.Addr().(*net.TCPAddr).Port
	write := func(name string, raw []byte) {
		t.Helper()
		path := filepath.Join(dir, name)
		f, e := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = f.Write(raw); e != nil {
			_ = f.Close()
			t.Fatal(e)
		}
		if e = f.Close(); e != nil {
			t.Fatal(e)
		}
		t.Cleanup(func() { _ = os.Remove(path) })
	}
	// No private TLS key is written; the exact generated public certificate is
	// the sole test trust anchor and the ordinary preview hostname stays signed.
	write("ca.pem", publicPEM)
	raw, e := json.Marshal(map[string]any{"origin": origin, "port": port, "certificate_pem": string(publicPEM), "account_id": fx.person.auth.Account.ID, "family_id": fx.person.auth.Session.FamilyID, "access_token": fx.person.auth.Session.AccessToken, "refresh_token": fx.person.auth.Session.RefreshToken, "access_expires_at": fx.person.auth.Session.AccessExpires, "refresh_expires_at": fx.person.auth.Session.AbsoluteExpires, "organization_id": fx.scope.OrgID().String(), "membership_id": fx.person.member})
	if e != nil {
		t.Fatal(e)
	}
	write("fixture.json", raw)
	t.Log("ordinary signed native source fixture ready; credentials remain in private /tmp file")
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-t.Context().Done():
			t.Fatal("native fixture context ended before stop")
		case <-ticker.C:
			if _, e := os.Stat(filepath.Join(dir, "stop")); e == nil {
				return
			} else if !os.IsNotExist(e) {
				t.Fatal(e)
			}
		}
	}
}
