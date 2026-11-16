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
	"io"
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
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/adapters/storage"
	fa "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	identitypg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/adapters"
	identityweb "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/adapters/http"
	identity "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/domain"
	mediapg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgmedia/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/enterprisesuite"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

// TestReceiptNativeSourceFixture is an opt-in actual default enterprise
// runtime (real account, identity, device contact, signed reference feed,
// signed intake with every default owner, the signed attachment session and
// the capability-bound document ingress over a disposable private bucket) for
// the Android API 36 acceptance of EMP-DOC-004 (and of the producers that
// reuse its capture path). It is never a product endpoint or a test-control
// API. Like the EMP-OFFL-005 fixture its only extra behaviour is a transport
// fault on the ingress armed by the device driver through files in the
// private fixture directory ("truncate", "flip"); at the stop the driver's
// expect.json names the exact effects the device stages must have produced.
func TestReceiptNativeSourceFixture(t *testing.T) {
	const host = "api.anpfuel.example.invalid"
	const origin = "https://" + host
	dir := os.Getenv("EMP_DOC004_NATIVE_FIXTURE_DIR")
	if !filepath.IsAbs(dir) || !strings.HasPrefix(filepath.Clean(dir), "/data/dev/") {
		t.Fatal("native receipt source requires a private absolute /data/dev directory")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(dir); err != nil || info.Mode().Perm() != 0700 {
		t.Fatal("native receipt source directory must have mode0700")
	}
	required := func(name string) string {
		t.Helper()
		value := os.Getenv(name)
		if value == "" {
			t.Fatalf("explicit disposable %s required", name)
		}
		return value
	}
	documents := mediapg.StorageConfig{Endpoint: required("ANPFUEL_TEST_STORAGE_ENDPOINT"), Bucket: required("ANPFUEL_TEST_STORAGE_BUCKET"), Region: required("ANPFUEL_TEST_STORAGE_REGION"),
		Credentials: storage.Credentials{AccessKeyID: required("ANPFUEL_TEST_STORAGE_ACCESS_KEY_ID"), SecretAccessKey: required("ANPFUEL_TEST_STORAGE_SECRET_ACCESS_KEY")}, AllowLoopback: true}
	x := tripFixtureOn(t, selfIntakeFixture(t))
	x.f.GrantReferenceFeed(t)
	tripReviewGrants(t, x.f)
	grantEmployeeTables(t, x.legalFixtureState)
	documentGrants(t, x.deviceFixtureState)
	// EMP-FUEL-011: the default runtime's fueling owner (and its EXP-002 expense) and the FUELING link resolver.
	fuelingGrants(t, x.legalFixtureState)
	role := pgx.Identifier{x.f.RuntimeRole}.Sanitize()
	if _, err := x.f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT ON org_trip_events,org_trip_held_events,org_document_links TO "+role); err != nil {
		t.Fatal(err)
	}
	person := x.person
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
	if _, err := x.f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT,UPDATE ON identity_rate_windows,identity_contributors,identity_keys,identity_challenges,identity_idempotency,account_contributor_bindings,account_binding_audit,account_session_refresh_history TO "+role); err != nil {
		t.Fatal(err)
	}
	var unsafe bool
	if err := x.f.Runtime.QueryRow(t.Context(), `SELECT rolsuper OR rolbypassrls FROM pg_roles WHERE rolname=current_user`).Scan(&unsafe); err != nil || unsafe {
		t.Fatal("native receipt source requires an ordinary runtime", err)
	}
	registrar := identitypg.NewRegistrar(x.f.Runtime)
	accounts := *x.accounts
	accounts.Store, accounts.Clock = accountpg.NewPGStore(x.f.Runtime), nativeContactClock{}
	accounts.KeyProver = keyprover.Prover{Now: time.Now, Keys: func(ctx context.Context, fp string) (string, string, string, bool, error) {
		key, err := registrar.FindDeviceKey(ctx, fp)
		return key.ContributorID, key.JWKX, key.JWKY, key.Revoked, err
	}}
	router := chi.NewRouter()
	secret := bytes.Repeat([]byte{99}, 32)
	identityweb.Handler{Registrar: registrar, Authority: host, QuotaSecret: secret,
		CheckQuota: identitypg.NewLimiter(x.f.Runtime, identity.DefaultQuotaPolicy()).Check}.RegisterRoutes(router)
	accountweb.Handler{Service: &accounts}.RegisterRoutes(router)
	// No injected handler or store: the real default enterprise runtime with its trip and document owners.
	if err := enterprisesuite.Register(router, enterprisesuite.Options{Enabled: true, Origin: origin, CursorSecret: secret, Pool: x.f.Runtime, Accounts: &accounts, DocumentStorage: &documents}); err != nil {
		t.Fatal(err)
	}
	orgA, orgB := x.scope.OrgID().String(), other.scope.OrgID().String()
	var mu sync.Mutex
	statuses := map[string][]int{}
	record := func(kind string, code int) {
		mu.Lock()
		statuses[kind] = append(statuses[kind], code)
		mu.Unlock()
	}
	// Pops the next armed ingress fault, if any; "plan" holds comma-separated faults.
	nextFault := func() string {
		mu.Lock()
		defer mu.Unlock()
		path := filepath.Join(dir, "plan")
		raw, err := os.ReadFile(path)
		if err != nil {
			return ""
		}
		faults := strings.Split(strings.TrimSpace(string(raw)), ",")
		if len(faults) == 0 || faults[0] == "" {
			return ""
		}
		if err := os.WriteFile(path, []byte(strings.Join(faults[1:], ",")), 0600); err != nil {
			t.Error(err)
		}
		return faults[0]
	}
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/content/upload"):
			fault := nextFault()
			if fault == "truncate" || fault == "flip" {
				raw, err := io.ReadAll(io.LimitReader(r.Body, 6*1024*1024))
				if err != nil || len(raw) < 4 {
					t.Error("unreadable device upload")
					return
				}
				length := int64(len(raw))
				if fault == "truncate" {
					raw = raw[:len(raw)/2]
				} else {
					raw[len(raw)/2] ^= 0xff
				}
				r.Body, r.ContentLength = io.NopCloser(bytes.NewReader(raw)), length
			}
			answer := httptest.NewRecorder()
			router.ServeHTTP(answer, r)
			record("upload-"+orDefault(fault, "clean"), answer.Code)
			t.Logf("ingress answer %d (%s)", answer.Code, orDefault(fault, "clean"))
			for name, values := range answer.Header() {
				w.Header()[name] = values
			}
			w.WriteHeader(answer.Code)
			_, _ = w.Write(answer.Body.Bytes())
		case strings.HasSuffix(r.URL.Path, "/sync/attachments"), strings.HasSuffix(r.URL.Path, "/sync/operations"):
			answer := httptest.NewRecorder()
			router.ServeHTTP(answer, r)
			kind := "session"
			if strings.HasSuffix(r.URL.Path, "/sync/operations") {
				kind = "intake"
			}
			record(kind+":"+r.URL.Path, answer.Code)
			// Status codes only; neither request nor capability is logged.
			t.Logf("%s answer %d", kind, answer.Code)
			for name, values := range answer.Header() {
				w.Header()[name] = values
			}
			w.WriteHeader(answer.Code)
			_, _ = w.Write(answer.Body.Bytes())
		default:
			router.ServeHTTP(w, r)
		}
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
		NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(3 * time.Hour), IsCA: true, BasicConstraintsValid: true,
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
		"organizations": []map[string]any{
			{"org_id": orgA, "membership_id": person.member, "employee_id": person.employee, "asset_id": x.vehicle.AssetID,
				"meter_id": x.vehicle.MeterID, "segment_id": x.vehicle.SegmentID},
			{"org_id": orgB, "membership_id": memberB, "employee_id": employeeB, "asset_id": assetB}}})
	if err != nil {
		t.Fatal(err)
	}
	write("ca.pem", public)
	write("fixture.json", fixture)
	t.Cleanup(func() {
		for _, name := range []string{"plan", "expect.json", "stop"} {
			_ = os.Remove(filepath.Join(dir, name))
		}
	})
	t.Log("ordinary two-organization signed attachment source ready; proof remains in private fixture")
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-t.Context().Done():
			t.Fatal("native receipt fixture ended before acceptance stop")
		case <-ticker.C:
			if _, err := os.Stat(filepath.Join(dir, "stop")); os.IsNotExist(err) {
				continue
			} else if err != nil {
				t.Fatal(err)
			}
			mu.Lock()
			observed := map[string][]int{}
			for key, values := range statuses {
				observed[key] = append([]int(nil), values...)
			}
			mu.Unlock()
			t.Logf("native receipt answers %v", observed)
			raw, err := os.ReadFile(filepath.Join(dir, "expect.json"))
			if err != nil {
				t.Fatal("the driver must name the expected effects", err)
			}
			var expect struct {
				Documents int            `json:"documents"`
				Stored    int            `json:"stored"`
				Links     map[string]int `json:"links"`
				Uploads   map[string]int `json:"uploads"`
				Fuelings  int            `json:"fuelings"`
				Trips     int            `json:"trips"`
			}
			if err := json.Unmarshal(raw, &expect); err != nil {
				t.Fatal(err)
			}
			for key, values := range observed {
				if strings.Contains(key, orgB) {
					t.Fatal("organization B received receipt or record work of A", key)
				}
				for _, code := range values {
					if strings.HasPrefix(key, "session:") && code != http.StatusOK || key == "upload-clean" && code != http.StatusOK ||
						(key == "upload-truncate" || key == "upload-flip") && code != http.StatusUnprocessableEntity {
						t.Fatal("unexpected native answer", key, code)
					}
				}
			}
			for kind, count := range expect.Uploads {
				if len(observed["upload-"+kind]) != count {
					t.Fatalf("native ingress attempts differ: %v, expected %v", observed, expect.Uploads)
				}
			}
			var documents, stored, received, storedEvents, fuelings, trips, foreign int
			if err := x.f.Admin.QueryRow(t.Context(), `SELECT
				(SELECT count(*) FROM org_documents WHERE org_id=$1 AND created_by=$2),
				(SELECT count(*) FROM org_documents WHERE org_id=$1 AND created_by=$2 AND storage_state='STORED'),
				(SELECT count(*) FROM org_audit_events WHERE org_id=$1 AND action='document.upload.received'),
				(SELECT count(*) FROM org_audit_events WHERE org_id=$1 AND action='document.upload.stored'),
				(SELECT count(*) FROM org_fuelings WHERE org_id=$1 AND author_membership_id=$2),
				(SELECT count(*) FROM org_trips WHERE org_id=$1 AND author_membership_id=$2),
				(SELECT count(*) FROM org_documents WHERE org_id=$3)`,
				orgA, person.member, orgB).Scan(&documents, &stored, &received, &storedEvents, &fuelings, &trips, &foreign); err != nil {
				t.Fatal(err)
			}
			links := map[string]int{}
			rows, err := x.f.Admin.Query(t.Context(), `SELECT l.target_kind,count(*) FROM org_document_links l JOIN org_documents d ON d.org_id=l.org_id AND d.id=l.document_id
				WHERE l.org_id=$1 AND d.created_by=$2 GROUP BY l.target_kind`, orgA, person.member)
			if err != nil {
				t.Fatal(err)
			}
			for rows.Next() {
				var kind string
				var count int
				if err := rows.Scan(&kind, &count); err != nil {
					t.Fatal(err)
				}
				links[kind] = count
			}
			rows.Close()
			t.Logf("native receipt effects documents=%d stored=%d received=%d stored_events=%d links=%v fuelings=%d trips=%d foreign=%d",
				documents, stored, received, storedEvents, links, fuelings, trips, foreign)
			// Every capture declared, linked, received and stored exactly once; B holds nothing.
			if documents != expect.Documents || stored != expect.Stored || received != expect.Stored || storedEvents != expect.Stored ||
				fuelings != expect.Fuelings || trips != expect.Trips || foreign != 0 || len(links) != len(expect.Links) {
				t.Fatalf("native receipt effects differ from %+v", expect)
			}
			for kind, count := range expect.Links {
				if links[kind] != count {
					t.Fatalf("native receipt links differ: %v, expected %v", links, expect.Links)
				}
			}
			return
		}
	}
}
