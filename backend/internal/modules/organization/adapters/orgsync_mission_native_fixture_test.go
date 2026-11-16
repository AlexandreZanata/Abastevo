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
	fa "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	identitypg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/adapters"
	identityweb "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/adapters/http"
	identity "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/domain"
	tripapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/enterprisesuite"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

// TestMissionNativeSourceFixture is an opt-in actual default enterprise
// runtime (real account, identity, device contact, company cookie session,
// signed reference feed, the EMP-TRIP-008 own mission list and the signed
// intake with the trip, lifecycle and mission-response owners) for the Android
// API 36 acceptance of EMP-TRIP-021. It is never a product endpoint or a
// test-control API. Its only extra behaviour is the owner's own reassignment
// of the second mission to another member through the real mission store,
// made when the device driver asks for it while the device is offline.
func TestMissionNativeSourceFixture(t *testing.T) {
	const host = "api.anpfuel.example.invalid"
	const origin = "https://" + host
	dir := os.Getenv("EMP_TRIP021_NATIVE_FIXTURE_DIR")
	if !filepath.IsAbs(dir) || !strings.HasPrefix(filepath.Clean(dir), "/data/dev/") {
		t.Fatal("native mission source requires a private absolute /data/dev directory")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(dir); err != nil || info.Mode().Perm() != 0700 {
		t.Fatal("native mission source directory must have mode0700")
	}
	x := tripFixtureOn(t, selfIntakeFixture(t))
	x.f.GrantReferenceFeed(t)
	tripReviewGrants(t, x.f)
	tripMissionLinkGrant(t, x.f)
	m := missionFixtureOn(t, x)
	m.missions.Reservations = m.assets
	role := pgx.Identifier{x.f.RuntimeRole}.Sanitize()
	if _, err := x.f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT ON org_trip_events,org_trip_held_events TO "+role); err != nil {
		t.Fatal(err)
	}
	person := x.person
	// The second member, the one the owner reassigns the second mission to while the device is offline.
	second := x.driver(t)
	x.assign(t, second.person.employee, x.vehicle.AssetID, fleet.AdditionalAssignment, nil, nil)
	// Organization B: the same account as an ordinary DRIVER/SELF member with its own asset and no mission.
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
	// Two planned missions on the vehicle, assigned to the device member (CREATE 1, PLAN 2, ASSIGN 3).
	text := func(v string) *string { return &v }
	planned := func(destination, reason string) string {
		created := m.create(t, m.unit, x.vehicle.AssetID)
		if _, err := m.plan(t, created.MissionID, 1, &tripapp.MissionPlanFields{DestinationText: text(destination), Reason: reason, Load: "CARGO",
			CargoDescription: text("Caixas de peças sintéticas")}); err != nil {
			t.Fatal("mission plan", err)
		}
		if _, err := m.assignTo(t, created.MissionID, 2, person.employee); err != nil {
			t.Fatal("mission assignment", err)
		}
		return created.MissionID
	}
	first := planned("Cliente Norte, doca 3", "Entrega programada")
	reassigned := planned("Cliente Sul, portão 2", "Coleta programada")
	if _, err := x.f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT,UPDATE ON identity_rate_windows,identity_contributors,identity_keys,identity_challenges,identity_idempotency,account_contributor_bindings,account_binding_audit,account_session_refresh_history TO "+role); err != nil {
		t.Fatal(err)
	}
	var unsafe bool
	if err := x.f.Runtime.QueryRow(t.Context(), `SELECT rolsuper OR rolbypassrls FROM pg_roles WHERE rolname=current_user`).Scan(&unsafe); err != nil || unsafe {
		t.Fatal("native mission source requires an ordinary runtime", err)
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
	// No injected handler or store: the real default enterprise runtime with its trip and mission owners.
	if err := enterprisesuite.Register(router, enterprisesuite.Options{Enabled: true, Origin: origin, CursorSecret: secret, Pool: x.f.Runtime, Accounts: &accounts}); err != nil {
		t.Fatal(err)
	}
	orgA, orgB := x.scope.OrgID().String(), other.scope.OrgID().String()
	type delivery struct {
		code, operations int
		answers          int
	}
	var mu sync.Mutex
	deliveries := map[string][]delivery{}
	mine := map[string][]int{}
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		intake, list := strings.HasSuffix(r.URL.Path, "/sync/operations"), strings.HasSuffix(r.URL.Path, "/missions/mine/list")
		if !intake && !list {
			router.ServeHTTP(w, r)
			return
		}
		var shape struct {
			Operations []struct {
				Type string `json:"type"`
			} `json:"operations"`
		}
		if intake {
			raw, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatal(err)
			}
			// Only the operation types are counted; the request is never logged.
			_ = json.Unmarshal(raw, &shape)
			r.Body = io.NopCloser(bytes.NewReader(raw))
		}
		answer := httptest.NewRecorder()
		router.ServeHTTP(answer, r)
		mu.Lock()
		if intake {
			d := delivery{code: answer.Code, operations: len(shape.Operations)}
			for _, op := range shape.Operations {
				if op.Type == tripapp.MissionRespondOperationType {
					d.answers++
				}
			}
			deliveries[r.URL.Path] = append(deliveries[r.URL.Path], d)
		} else {
			mine[r.URL.Path] = append(mine[r.URL.Path], answer.Code)
		}
		mu.Unlock()
		if intake {
			// Synthetic acknowledgements only (operation identifiers, digests, results, codes); the request is never logged.
			body := answer.Body.String()
			if len(body) > 4096 {
				body = body[:4096]
			}
			t.Logf("intake answer %d (%d operations) %s", answer.Code, len(shape.Operations), body)
		} else {
			t.Logf("company own mission list answer %d", answer.Code)
		}
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
	fixture, err := json.Marshal(map[string]any{"origin": origin, "port": port, "certificate_pem": string(public),
		"account_id": person.auth.Account.ID, "family_id": person.auth.Session.FamilyID,
		"access_token": person.auth.Session.AccessToken, "refresh_token": person.auth.Session.RefreshToken,
		"access_expires_at": person.auth.Session.AccessExpires, "refresh_expires_at": person.auth.Session.AbsoluteExpires,
		"missions": []string{first, reassigned},
		"organizations": []map[string]any{
			{"org_id": orgA, "membership_id": person.member, "employee_id": person.employee, "asset_id": x.vehicle.AssetID,
				"meter_id": x.vehicle.MeterID, "segment_id": x.vehicle.SegmentID},
			{"org_id": orgB, "membership_id": memberB, "employee_id": employeeB, "asset_id": assetB}}})
	if err != nil {
		t.Fatal(err)
	}
	write("ca.pem", public)
	write("fixture.json", fixture)
	t.Log("ordinary two-organization signed mission source ready; proof remains in private fixture")
	moved := false
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-t.Context().Done():
			t.Fatal("native mission fixture ended before acceptance stop")
		case <-ticker.C:
			if _, err := os.Stat(filepath.Join(dir, "reassign")); err == nil && !moved {
				// While the device is offline with its saved acceptance, the owner reassigns the second mission.
				if _, err := m.assignTo(t, reassigned, 3, second.person.employee); err != nil {
					t.Fatal("reassignment", err)
				}
				moved = true
				write("reassigned", []byte("1"))
				t.Log("the second mission is now another member's")
			}
			if _, err := os.Stat(filepath.Join(dir, "stop")); os.IsNotExist(err) {
				continue
			} else if err != nil {
				t.Fatal(err)
			}
			mu.Lock()
			pathA := "/v1/orgs/" + orgA + "/sync/operations"
			intakeA, intakeB := append([]delivery(nil), deliveries[pathA]...), append([]delivery(nil), deliveries["/v1/orgs/"+orgB+"/sync/operations"]...)
			listA, listB := append([]int(nil), mine["/v1/orgs/"+orgA+"/missions/mine/list"]...), append([]int(nil), mine["/v1/orgs/"+orgB+"/missions/mine/list"]...)
			mu.Unlock()
			t.Logf("native deliveries A=%+v B=%+v own lists A=%v B=%v", intakeA, intakeB, listA, listB)
			if !moved {
				t.Fatal("native acceptance stopped before the reassignment")
			}
			denied := 0
			for _, d := range intakeA {
				switch {
				case d.code == http.StatusOK:
				case d.code == http.StatusForbidden && d.operations == 1 && d.answers == 1:
					// EDR-343: the denied answer travelled alone and held nothing else.
					denied++
				default:
					t.Fatalf("a device delivery was refused with other records in it: %+v", d)
				}
				if d.answers > 0 && d.operations != 1 {
					t.Fatalf("an answer travelled with other records: %+v", d)
				}
			}
			if denied == 0 {
				t.Fatal("the reassigned mission's answer was never denied")
			}
			if len(intakeB) != 0 {
				t.Fatal("organization B received work of A")
			}
			for _, code := range listA {
				if code != http.StatusOK {
					t.Fatal("an own-list read was refused", code)
				}
			}
			if len(listA) < 3 {
				t.Fatal("the device did not refresh its missions online three times", listA)
			}
			type counts struct{ answers, linked, trips, completed, transitions, operations, acks int }
			var c counts
			if err := x.f.Admin.QueryRow(t.Context(), `SELECT
				(SELECT count(*) FROM org_trip_mission_transitions WHERE org_id=$1 AND source='DEVICE'),
				(SELECT count(*) FROM org_trip_mission_links WHERE org_id=$1 AND mission_id=$3),
				(SELECT count(*) FROM org_trips WHERE org_id=$1 AND author_membership_id=$2),
				(SELECT count(*) FROM org_trips WHERE org_id=$1 AND author_membership_id=$2 AND state='COMPLETED'),
				(SELECT count(*) FROM org_trip_transitions WHERE org_id=$1),
				(SELECT count(*) FROM org_sync_operations WHERE org_id=$1 AND membership_id=$2),
				(SELECT count(*) FROM org_sync_acknowledgements a JOIN org_sync_operations o ON o.org_id=a.org_id AND o.operation_id=a.operation_id WHERE a.org_id=$1 AND o.membership_id=$2 AND a.result='ACCEPTED')`,
				orgA, person.member, first).Scan(&c.answers, &c.linked, &c.trips, &c.completed, &c.transitions, &c.operations, &c.acks); err != nil {
				t.Fatal(err)
			}
			t.Logf("native mission counts %+v", c)
			// One accepted answer (the reassigned one stored nothing), one linked trip completed with a pause and a resume,
			// and its five operations (answer, start, pause, resume, finish) each accepted once.
			if c != (counts{answers: 1, linked: 1, trips: 1, completed: 1, transitions: 2, operations: 5, acks: 5}) {
				t.Fatalf("native mission effects differ: %+v", c)
			}
			var states string
			if err := x.f.Admin.QueryRow(t.Context(), `SELECT string_agg(state||':'||COALESCE(assignee_membership_id::text,''),',' ORDER BY id=$2 DESC) FROM org_trip_missions WHERE org_id=$1`,
				orgA, first).Scan(&states); err != nil {
				t.Fatal(err)
			}
			if states != "COMPLETED:"+person.member+",ASSIGNED:"+second.source.MembershipID {
				t.Fatal("mission heads differ", states)
			}
			return
		}
	}
}
