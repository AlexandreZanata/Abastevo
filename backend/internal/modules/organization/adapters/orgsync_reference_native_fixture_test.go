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
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	accountpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/adapters"
	accountweb "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/adapters/http"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/adapters/keyprover"
	fa "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	identitypg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/adapters"
	identityweb "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/adapters/http"
	identity "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/enterprisesuite"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

// TestReferenceFeedNativeSourceFixture is an opt-in actual default enterprise
// runtime (real account, identity, device contact and the signed reference
// feed of EMP-OFFL-006) for the Android API 36 acceptance of EMP-FLEET-013.
// It is never a product endpoint or a test-control API. Its only extra
// behaviour is an actual company command: after the device reports that it is
// offline, it ends the device member's primary assignment through the real
// FLEET-006 store, so the device must keep listing the asset until it
// reconnects and receives the tombstone.
func TestReferenceFeedNativeSourceFixture(t *testing.T) {
	const host = "api.anpfuel.example.invalid"
	const origin = "https://" + host
	dir := os.Getenv("EMP_FLEET013_NATIVE_FIXTURE_DIR")
	if !filepath.IsAbs(dir) || !strings.HasPrefix(filepath.Clean(dir), "/data/dev/") {
		t.Fatal("native reference source requires a private absolute /data/dev directory")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(dir); err != nil || info.Mode().Perm() != 0700 {
		t.Fatal("native reference source directory must have mode0700")
	}
	endIn, err := time.ParseDuration(os.Getenv("EMP_FLEET013_TEMPORARY_END_IN"))
	if err != nil || endIn < time.Minute || endIn > 30*time.Minute {
		t.Fatal("EMP_FLEET013_TEMPORARY_END_IN must be a duration between 1m and 30m")
	}
	x := selfIntakeFixture(t)
	x.f.GrantReferenceFeed(t)
	person := x.person
	// Organization A: the asset with an actual meter as PRIMARY and a second asset
	// as TEMPORARY whose server-side end passes while the device is offline.
	primaryA, _ := x.assign(t, person.employee, x.installed.AssetID, fleet.PrimaryAssignment, nil, nil)
	temporaryAsset, _ := lifecycleAssetIn(t, x.legalFixtureState, x.assets, x.unit, fleet.Active)
	end := time.Now().UTC().Add(endIn).Truncate(time.Microsecond)
	x.assign(t, person.employee, temporaryAsset, fleet.TemporaryAssignment, nil, &end)
	foreignDriver := x.driver(t)
	foreignAsset, _ := lifecycleAssetIn(t, x.legalFixtureState, x.assets, x.unit, fleet.Active)
	x.assign(t, foreignDriver.person.employee, foreignAsset, fleet.PrimaryAssignment, nil, nil)
	// Organization B: the same account as an ordinary DRIVER/SELF member with its own asset.
	other := legalFixtureFor(t, x.f, x.onboarding, x.actor, x.flags)
	if _, _, err := other.onboarding.Repository.ActivateOrganization(t.Context(), other.scope, other.actor, other.member, onboardingID(t), 1); err != nil {
		t.Fatal(err)
	}
	employeeB, memberB := teamDriver(t, other, person.auth.Account.ID)
	assetsB := assignmentStore(t, other)
	var unitB string
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT id::text FROM org_units WHERE org_id=$1`, other.scope.OrgID().String()).Scan(&unitB); err != nil {
		t.Fatal(err)
	}
	assetB, _ := lifecycleAssetIn(t, other, assetsB, unitB, fleet.Active)
	if result, _, err := assignmentWrite(t, other, assetsB, assignmentCreate(t, fa.AssignmentCreateInput{AssetID: assetB, Kind: fleet.PrimaryAssignment, EmployeeIDs: []string{employeeB}})); err != nil || len(result.AssignmentIDs) != 1 {
		t.Fatal("second organization assignment", err)
	}
	role := pgx.Identifier{x.f.RuntimeRole}.Sanitize()
	if _, err := x.f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT,UPDATE ON identity_rate_windows,identity_contributors,identity_keys,identity_challenges,identity_idempotency,account_contributor_bindings,account_binding_audit,account_session_refresh_history TO "+role); err != nil {
		t.Fatal(err)
	}
	var unsafe bool
	if err := x.f.Runtime.QueryRow(t.Context(), `SELECT rolsuper OR rolbypassrls FROM pg_roles WHERE rolname=current_user`).Scan(&unsafe); err != nil || unsafe {
		t.Fatal("native reference source requires an ordinary runtime", err)
	}
	registrar := identitypg.NewRegistrar(x.f.Runtime)
	accounts := *x.accounts
	accounts.Store, accounts.Clock = accountpg.NewPGStore(x.f.Runtime), nativeContactClock{}
	accounts.KeyProver = keyprover.Prover{Now: time.Now, Keys: func(ctx context.Context, fp string) (string, string, string, bool, error) {
		key, err := registrar.FindDeviceKey(ctx, fp)
		return key.ContributorID, key.JWKX, key.JWKY, key.Revoked, err
	}}
	router := chi.NewRouter()
	secret := bytes.Repeat([]byte{98}, 32)
	identityweb.Handler{Registrar: registrar, Authority: host, QuotaSecret: secret,
		CheckQuota: identitypg.NewLimiter(x.f.Runtime, identity.DefaultQuotaPolicy()).Check}.RegisterRoutes(router)
	accountweb.Handler{Service: &accounts}.RegisterRoutes(router)
	// No injected handler or store: the real default enterprise runtime, reference feed included.
	if err := enterprisesuite.Register(router, enterprisesuite.Options{Enabled: true, Origin: origin, CursorSecret: secret, Pool: x.f.Runtime, Accounts: &accounts}); err != nil {
		t.Fatal(err)
	}
	orgA, orgB := x.scope.OrgID().String(), other.scope.OrgID().String()
	var mu sync.Mutex
	pulls := map[string]int{}
	statuses := map[string][]int{}
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/sync/reference") {
			router.ServeHTTP(w, r)
			return
		}
		org := ""
		for _, candidate := range []string{orgA, orgB} {
			if strings.HasPrefix(r.URL.Path, "/v1/orgs/"+candidate+"/") {
				org = candidate
			}
		}
		answer := httptest.NewRecorder()
		router.ServeHTTP(answer, r)
		mu.Lock()
		pulls[org]++
		statuses[org] = append(statuses[org], answer.Code)
		mu.Unlock()
		for name, values := range answer.Header() {
			w.Header()[name] = values
		}
		w.WriteHeader(answer.Code)
		_, _ = w.Write(answer.Body.Bytes())
	}))
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: host}, DNSNames: []string{host},
		NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(2 * time.Hour), IsCA: true, BasicConstraintsValid: true,
		KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	if err != nil {
		t.Fatal(err)
	}
	public := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	server.TLS = &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: privateKey}}}
	server.StartTLS()
	defer server.Close()
	port := server.Listener.Addr().(*net.TCPAddr).Port
	write := func(name string, value []byte) {
		t.Helper()
		path := filepath.Join(dir, name)
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.Write(value); err != nil {
			_ = file.Close()
			t.Fatal(err)
		}
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Remove(path) })
	}
	fixture, err := json.Marshal(map[string]any{"origin": origin, "port": port, "certificate_pem": string(public),
		"account_id": person.auth.Account.ID, "family_id": person.auth.Session.FamilyID,
		"access_token": person.auth.Session.AccessToken, "refresh_token": person.auth.Session.RefreshToken,
		"access_expires_at": person.auth.Session.AccessExpires, "refresh_expires_at": person.auth.Session.AbsoluteExpires,
		"temporary_end": end.Format(time.RFC3339Nano), "organizations": []map[string]any{
			{"org_id": orgA, "membership_id": person.member, "primary_asset_id": x.installed.AssetID, "meter_id": x.installed.MeterID,
				"segment_id": x.installed.SegmentID, "temporary_asset_id": temporaryAsset, "foreign_asset_id": foreignAsset},
			{"org_id": orgB, "membership_id": memberB, "primary_asset_id": assetB}}})
	if err != nil {
		t.Fatal(err)
	}
	write("ca.pem", public)
	write("fixture.json", fixture)
	t.Log("ordinary two-organization signed reference source ready; proof remains in private fixture")
	ended := false
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-t.Context().Done():
			t.Fatal("native reference fixture ended before acceptance stop")
		case <-ticker.C:
			if _, err := os.Stat(filepath.Join(dir, "offline")); err == nil && !ended {
				mu.Lock()
				seeded := pulls[orgA] > 0 && pulls[orgB] > 0
				mu.Unlock()
				if !seeded {
					t.Fatal("the device went offline before pulling both organizations")
				}
				// An actual company command while the device has no route to this source.
				x.end(t, x.installed.AssetID, primaryA)
				ended = true
				write("ended", []byte("1"))
				t.Log("primary assignment ended on the server while the device is offline")
			}
			if _, err := os.Stat(filepath.Join(dir, "stop")); os.IsNotExist(err) {
				continue
			} else if err != nil {
				t.Fatal(err)
			}
			mu.Lock()
			counts := map[string]any{"pullsA": pulls[orgA], "pullsB": pulls[orgB], "statusesA": statuses[orgA], "statusesB": statuses[orgB]}
			mu.Unlock()
			t.Logf("native reference request counts %v", counts)
			if !ended {
				t.Fatal("native acceptance stopped before the offline server-side end")
			}
			type logged struct{ upserts, deletes int }
			entries := func(org, member string) logged {
				var v logged
				if err := x.f.Admin.QueryRow(t.Context(), `SELECT count(*) FILTER(WHERE op='UPSERT'),count(*) FILTER(WHERE op='DELETE') FROM org_reference_feed_entries WHERE org_id=$1 AND membership_id=$2`, org, member).Scan(&v.upserts, &v.deletes); err != nil {
					t.Fatal(err)
				}
				return v
			}
			// A: two upserts at the first pull, then two tombstones (ended and expired) delivered at reconnection.
			if a, b := entries(orgA, person.member), entries(orgB, memberB); a != (logged{2, 2}) || b != (logged{1, 0}) {
				t.Fatalf("native feed logs differ: A %+v B %+v", a, b)
			}
			// One pull per organization online (stage 1) and one after reconnection (stage 3): nothing reached
			// this source while the device had no route to it.
			if len(statuses[orgA]) != 2 || len(statuses[orgB]) != 2 {
				t.Fatal("native reference pull counts differ", len(statuses[orgA]), len(statuses[orgB]))
			}
			for _, org := range []string{orgA, orgB} {
				for _, code := range statuses[org] {
					if code != http.StatusOK {
						t.Fatal("a device pull was refused", org, code)
					}
				}
			}
			return
		}
	}
}
