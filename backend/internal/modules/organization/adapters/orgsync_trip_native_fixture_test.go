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
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
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
func TestTripNativeSourceFixture(t *testing.T) {
	const host = "api.anpfuel.example.invalid"
	const origin = "https://" + host
	dir := os.Getenv("EMP_TRIP005_NATIVE_FIXTURE_DIR")
	if !filepath.IsAbs(dir) || !strings.HasPrefix(filepath.Clean(dir), "/data/dev/") {
		t.Fatal("native trip source requires a private absolute /data/dev directory")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(dir); err != nil || info.Mode().Perm() != 0700 {
		t.Fatal("native trip source directory must have mode0700")
	}
	x := tripFixtureOn(t, selfIntakeFixture(t))
	x.f.GrantReferenceFeed(t)
	tripReviewGrants(t, x.f)
	role := pgx.Identifier{x.f.RuntimeRole}.Sanitize()
	if _, err := x.f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT ON org_trip_events,org_trip_held_events TO "+role); err != nil {
		t.Fatal(err)
	}
	person := x.person
	// The second member of the same organization, assigned to the same vehicle, with an actual enrolled device.
	second := x.driver(t)
	x.assign(t, second.person.employee, x.vehicle.AssetID, fleet.AdditionalAssignment, nil, nil)
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
		t.Fatal("native trip source requires an ordinary runtime", err)
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
	// No injected handler or store: the real default enterprise runtime with its trip owners.
	if err := enterprisesuite.Register(router, enterprisesuite.Options{Enabled: true, Origin: origin, CursorSecret: secret, Pool: x.f.Runtime, Accounts: &accounts}); err != nil {
		t.Fatal(err)
	}
	orgA, orgB := x.scope.OrgID().String(), other.scope.OrgID().String()
	var mu sync.Mutex
	deliveries := map[string][]int{}
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/sync/operations") {
			router.ServeHTTP(w, r)
			return
		}
		answer := httptest.NewRecorder()
		router.ServeHTTP(answer, r)
		mu.Lock()
		deliveries[r.URL.Path] = append(deliveries[r.URL.Path], answer.Code)
		mu.Unlock()
		// Synthetic acknowledgements only (operation identifiers, digests, results, codes); the request is never logged.
		body := answer.Body.String()
		if len(body) > 4096 {
			body = body[:4096]
		}
		t.Logf("intake answer %d %s", answer.Code, body)
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
	// The device driver identity is the membership's own employee record; the app reads it from the
	// context read (EDR-244), which the organization HTTP tests prove separately.
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
	t.Log("ordinary two-organization signed trip source ready; proof remains in private fixture")
	secondStarted := false
	var secondTrip string
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-t.Context().Done():
			t.Fatal("native trip fixture ended before acceptance stop")
		case <-ticker.C:
			if _, err := os.Stat(filepath.Join(dir, "second-start")); err == nil && !secondStarted {
				// Another member's device starts a trip on the same vehicle through the same intake first.
				op := x.startOp(t, second, x.declaredNow(t), nil)
				ack, err := x.process(t, second, op)
				if err != nil || ack.Result() != protocol.Accepted {
					t.Fatal("the second member's start was not accepted", err, ack.Result(), ack.Code())
				}
				secondTrip, secondStarted = tripOf(op), true
				write("second-started", []byte("1"))
				t.Log("a second member's trip is in progress on the same vehicle")
			}
			if _, err := os.Stat(filepath.Join(dir, "stop")); os.IsNotExist(err) {
				continue
			} else if err != nil {
				t.Fatal(err)
			}
			mu.Lock()
			pathA := "/v1/orgs/" + orgA + "/sync/operations"
			statusesA, statusesB := append([]int(nil), deliveries[pathA]...), append([]int(nil), deliveries["/v1/orgs/"+orgB+"/sync/operations"]...)
			mu.Unlock()
			t.Logf("native intake deliveries A=%v B=%v", statusesA, statusesB)
			if !secondStarted {
				t.Fatal("native acceptance stopped before the second member's start")
			}
			for _, code := range statusesA {
				if code != http.StatusOK {
					t.Fatal("a device delivery was refused", code)
				}
			}
			if len(statusesB) != 0 {
				t.Fatal("organization B received trip work of A")
			}
			type counts struct{ trips, completed, finishes, events, submits, heldStarts, operations, acks int }
			var c counts
			if err := x.f.Admin.QueryRow(t.Context(), `SELECT
				(SELECT count(*) FROM org_trips WHERE org_id=$1 AND author_membership_id=$2),
				(SELECT count(*) FROM org_trips WHERE org_id=$1 AND author_membership_id=$2 AND state='COMPLETED'),
				(SELECT count(*) FROM org_trip_finishes WHERE org_id=$1),
				(SELECT count(*) FROM org_trip_events WHERE org_id=$1),
				(SELECT count(*) FROM org_trip_review_entries WHERE org_id=$1 AND kind='SUBMIT'),
				(SELECT count(*) FROM org_trip_held_starts WHERE org_id=$1),
				(SELECT count(*) FROM org_sync_operations WHERE org_id=$1 AND membership_id=$2),
				(SELECT count(*) FROM org_sync_acknowledgements a JOIN org_sync_operations o ON o.org_id=a.org_id AND o.operation_id=a.operation_id WHERE a.org_id=$1 AND o.membership_id=$2)`,
				orgA, person.member).Scan(&c.trips, &c.completed, &c.finishes, &c.events, &c.submits, &c.heldStarts, &c.operations, &c.acks); err != nil {
				t.Fatal(err)
			}
			t.Logf("native trip counts %+v (second member trip present: %t)", c, secondTrip != "")
			// Trip 1: start, STOP, NOTE, finish and submission each stored once; trip 2: the held start only (its
			// STOP waits for the start's resolution). Nothing else reached A from this device.
			if c != (counts{trips: 1, completed: 1, finishes: 1, events: 2, submits: 1, heldStarts: 1, operations: 6, acks: 6}) {
				t.Fatalf("native trip effects differ: %+v", c)
			}
			var busy bool
			if err := x.f.Admin.QueryRow(t.Context(), `SELECT EXISTS(SELECT 1 FROM org_trip_held_starts h JOIN org_sync_review_items i ON i.org_id=h.org_id AND i.operation_id=h.operation_id WHERE h.org_id=$1 AND i.code='ASSET_BUSY')`, orgA).Scan(&busy); err != nil || !busy {
				t.Fatal("the device's second start was not held ASSET_BUSY for review", err)
			}
			return
		}
	}
}
