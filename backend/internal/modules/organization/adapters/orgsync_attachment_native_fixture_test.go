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

// TestTripNativeSourceFixture is an opt-in actual default enterprise runtime
// (real account, identity, device contact, signed reference feed and signed
// intake with the trip owners of EMP-TRIP-002/003/004) for the Android API 36
// acceptance of EMP-TRIP-005. It is never a product endpoint or a
// test-control API. Its only extra behaviour is a second assigned member's
// actual trip.start on the same vehicle through the real intake store, made
// when the device driver asks for it, so the device's own later start is held
// ASSET_BUSY by the server.
// TestAttachmentNativeSourceFixture is an opt-in actual default enterprise
// runtime (real account, identity, device contact, signed reference feed,
// signed intake with the trip owners, the signed attachment session and the
// capability-bound document ingress over a disposable private bucket) for the
// Android API 36 acceptance of EMP-OFFL-005. It is never a product endpoint or
// a test-control API. Its only extra behaviour is a transport fault on the
// ingress that the device driver arms through files in the private fixture
// directory: "truncate" forwards half of the next upload body with its full
// declared length, "flip" alters one byte of it, "stall" reads part of it and
// holds the connection until the client disappears, as a network or a killed
// process would.
func TestAttachmentNativeSourceFixture(t *testing.T) {
	const host = "api.anpfuel.example.invalid"
	const origin = "https://" + host
	dir := os.Getenv("EMP_OFFL005_NATIVE_FIXTURE_DIR")
	if !filepath.IsAbs(dir) || !strings.HasPrefix(filepath.Clean(dir), "/data/dev/") {
		t.Fatal("native attachment source requires a private absolute /data/dev directory")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(dir); err != nil || info.Mode().Perm() != 0700 {
		t.Fatal("native attachment source directory must have mode0700")
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
		t.Fatal("native attachment source requires an ordinary runtime", err)
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
			if fault == "stall" {
				// Part of the body arrives, then the connection is held until the device process is killed.
				_, _ = io.ReadFull(r.Body, make([]byte, 64))
				// Recorded when the transfer is held: nothing of it ever reaches the document service.
				record("upload-stalled", 0)
				if err := os.WriteFile(filepath.Join(dir, "upload-stalled"), []byte("1"), 0600); err != nil {
					t.Error(err)
				}
				select {
				case <-r.Context().Done():
					t.Log("stalled upload ended by the client")
				case <-time.After(20 * time.Second):
					t.Log("stalled upload released after the device process was killed")
				}
				return
			}
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
		for _, name := range []string{"plan", "upload-stalled"} {
			_ = os.Remove(filepath.Join(dir, name))
		}
	})
	t.Log("ordinary two-organization signed attachment source ready; proof remains in private fixture")
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-t.Context().Done():
			t.Fatal("native attachment fixture ended before acceptance stop")
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
			t.Logf("native attachment answers %v", observed)
			for key, values := range observed {
				if strings.Contains(key, orgB) {
					t.Fatal("organization B received attachment or trip work of A", key)
				}
				for _, code := range values {
					if strings.HasPrefix(key, "session:") && code != http.StatusOK || key == "upload-clean" && code != http.StatusOK ||
						(key == "upload-truncate" || key == "upload-flip") && code != http.StatusUnprocessableEntity {
						t.Fatal("unexpected native answer", key, code)
					}
				}
			}
			if len(observed["upload-truncate"]) != 1 || len(observed["upload-flip"]) != 1 || len(observed["upload-stalled"]) != 1 || len(observed["upload-clean"]) != 3 {
				t.Fatalf("native ingress attempts differ: %v", observed)
			}
			type counts struct{ documents, stored, links, received, storedEvents int }
			var c counts
			if err := x.f.Admin.QueryRow(t.Context(), `SELECT
				(SELECT count(*) FROM org_documents WHERE org_id=$1 AND created_by=$2),
				(SELECT count(*) FROM org_documents WHERE org_id=$1 AND created_by=$2 AND storage_state='STORED' AND storage_revision=3),
				(SELECT count(*) FROM org_document_links l JOIN org_meter_readings r ON r.org_id=l.org_id AND r.id=l.target_id
					WHERE l.org_id=$1 AND l.target_kind='METER_READING' AND r.origin='TRIP_START' AND r.author_membership_id=$2),
				(SELECT count(*) FROM org_audit_events WHERE org_id=$1 AND action='document.upload.received'),
				(SELECT count(*) FROM org_audit_events WHERE org_id=$1 AND action='document.upload.stored')`,
				orgA, person.member).Scan(&c.documents, &c.stored, &c.links, &c.received, &c.storedEvents); err != nil {
				t.Fatal(err)
			}
			t.Logf("native attachment counts %+v", c)
			// Three photos of the departure reading: each declared, linked, received and stored exactly once.
			if c != (counts{documents: 3, stored: 3, links: 3, received: 3, storedEvents: 3}) {
				t.Fatalf("native attachment effects differ: %+v", c)
			}
			var foreign int
			if err := x.f.Admin.QueryRow(t.Context(), `SELECT count(*) FROM org_documents WHERE org_id=$1`, orgB).Scan(&foreign); err != nil || foreign != 0 {
				t.Fatal("organization B holds a document", err, foreign)
			}
			return
		}
	}
}

func orDefault(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
