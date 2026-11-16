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
	identitypg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/adapters"
	identityweb "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/adapters/http"
	identity "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/enterprisesuite"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

// TestTrackingNativeSourceFixture is an opt-in actual default enterprise
// runtime (real account, identity, device contact, company cookie session,
// the EMP-GPS-002 policy read, the signed intake with the trip owners and the
// EMP-GPS-004 tracking owners) for the Android API 36 acceptance of
// EMP-GPS-002/003 on the owned emulator. It is never a product endpoint or a
// test-control API. Its only extra behaviour is the organization's tracking
// policy, recorded as EXPECTED with an employee notice already in force, the
// way the settings owner records it.
func TestTrackingNativeSourceFixture(t *testing.T) {
	const host = "api.anpfuel.example.invalid"
	const origin = "https://" + host
	dir := os.Getenv("EMP_GPS002_NATIVE_FIXTURE_DIR")
	if !filepath.IsAbs(dir) || !strings.HasPrefix(filepath.Clean(dir), "/data/dev/") {
		t.Fatal("native tracking source requires a private absolute /data/dev directory")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(dir); err != nil || info.Mode().Perm() != 0700 {
		t.Fatal("native tracking source directory must have mode0700")
	}
	x := tripFixtureOn(t, selfIntakeFixture(t))
	x.f.GrantReferenceFeed(t)
	tripReviewGrants(t, x.f)
	role := pgx.Identifier{x.f.RuntimeRole}.Sanitize()
	for _, grant := range []string{
		"GRANT SELECT,INSERT ON org_trip_events,org_trip_held_events TO " + role,
		"GRANT SELECT ON org_tracking_policies TO " + role,
		"GRANT SELECT,INSERT ON org_tracking_sessions,org_tracking_transitions,org_tracking_segments,org_tracking_summaries TO " + role,
		"GRANT SELECT,INSERT,UPDATE ON job_queue TO " + role,
		"GRANT SELECT,INSERT,UPDATE ON identity_rate_windows,identity_contributors,identity_keys,identity_challenges,identity_idempotency,account_contributor_bindings,account_binding_audit,account_session_refresh_history TO " + role,
	} {
		if _, err := x.f.Admin.Exec(t.Context(), grant); err != nil {
			t.Fatal(err)
		}
	}
	org := x.scope.OrgID().String()
	person := x.person
	// The organization's policy: EXPECTED with its employee notice in force since yesterday (EDR-271, B-BR-EMP-468).
	func() {
		tx, err := x.f.Admin.Begin(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = tx.Rollback(context.Background()) }()
		if _, err := tx.Exec(t.Context(), `SELECT set_config('anpfuel.org_id',$1,true)`, org); err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(t.Context(), `INSERT INTO org_tracking_policies(org_id,revision,policy,notice_version,notice_in_force_at)
 VALUES($1,1,'EXPECTED','aviso-2026.11',clock_timestamp()-interval '1 day')`, org); err != nil {
			t.Fatal("tracking policy fixture", err)
		}
		if err := tx.Commit(t.Context()); err != nil {
			t.Fatal(err)
		}
	}()
	var unsafe bool
	if err := x.f.Runtime.QueryRow(t.Context(), `SELECT rolsuper OR rolbypassrls FROM pg_roles WHERE rolname=current_user`).Scan(&unsafe); err != nil || unsafe {
		t.Fatal("native tracking source requires an ordinary runtime", err)
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
	// No injected handler or store: the real default enterprise runtime with its trip and tracking owners.
	if err := enterprisesuite.Register(router, enterprisesuite.Options{Enabled: true, Origin: origin, CursorSecret: secret, Pool: x.f.Runtime, Accounts: &accounts}); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	statuses := map[string][]int{}
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		kind := ""
		switch {
		case strings.HasSuffix(r.URL.Path, "/sync/operations"):
			kind = "intake"
		case strings.HasSuffix(r.URL.Path, "/tracking/policy/mine"):
			kind = "policy"
		}
		if kind == "" {
			router.ServeHTTP(w, r)
			return
		}
		answer := httptest.NewRecorder()
		router.ServeHTTP(answer, r)
		mu.Lock()
		statuses[kind] = append(statuses[kind], answer.Code)
		mu.Unlock()
		// Synthetic acknowledgements and policy answers only (identifiers, digests, results, codes); requests are never logged.
		body := answer.Body.String()
		if len(body) > 4096 {
			body = body[:4096]
		}
		t.Logf("%s answer %d %s", kind, answer.Code, body)
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
	certificate := &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: host}, DNSNames: []string{host},
		NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(3 * time.Hour), IsCA: true, BasicConstraintsValid: true,
		KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, certificate, certificate, &privateKey.PublicKey, privateKey)
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
		"org_id": org, "membership_id": person.member, "employee_id": person.employee, "asset_id": x.vehicle.AssetID,
		"meter_id": x.vehicle.MeterID, "segment_id": x.vehicle.SegmentID})
	if err != nil {
		t.Fatal(err)
	}
	write("ca.pem", public)
	write("fixture.json", fixture)
	t.Log("ordinary signed tracking source ready (policy EXPECTED, notice in force); proof remains in private fixture")
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-t.Context().Done():
			t.Fatal("native tracking fixture ended before acceptance stop")
		case <-ticker.C:
			if _, err := os.Stat(filepath.Join(dir, "stop")); os.IsNotExist(err) {
				continue
			} else if err != nil {
				t.Fatal(err)
			}
			mu.Lock()
			intake, policy := append([]int(nil), statuses["intake"]...), append([]int(nil), statuses["policy"]...)
			mu.Unlock()
			t.Logf("native tracking requests intake=%v policy=%v", intake, policy)
			for _, codes := range [][]int{intake, policy} {
				for _, code := range codes {
					if code != http.StatusOK {
						t.Fatal("a device request was refused", code)
					}
				}
			}
			if len(policy) < 1 || len(intake) < 1 {
				t.Fatal("the device never read the policy or never delivered")
			}
			var transitions string
			if err := x.f.Admin.QueryRow(t.Context(), `SELECT COALESCE(string_agg(t.state||':'||COALESCE(t.cause,'-'),',' ORDER BY t.seq),'')
				FROM org_tracking_transitions t JOIN org_tracking_sessions s ON (s.org_id,s.id)=(t.org_id,t.session_id)
				WHERE s.org_id=$1 AND s.owner_membership_id=$2`, org, person.member).Scan(&transitions); err != nil {
				t.Fatal(err)
			}
			t.Logf("native tracking transitions %s", transitions)
			want := "NOT_STARTED:NOT_STARTED,RECORDING:-,INTERRUPTED:STOPPED_BY_DRIVER,RECORDING:-,INTERRUPTED:PROCESS_STOPPED,ENDED:TRIP_COMPLETED"
			if transitions != want {
				t.Fatalf("stored transitions differ:\n got %s\nwant %s", transitions, want)
			}
			type counts struct{ sessions, policyRevision, segments, stored, received, duplicates, outside, mock, sequenceGaps, completed, operations, accepted int }
			var c counts
			if err := x.f.Admin.QueryRow(t.Context(), `SELECT
				(SELECT count(*) FROM org_tracking_sessions WHERE org_id=$1 AND owner_membership_id=$2),
				(SELECT COALESCE(max(policy_revision),0) FROM org_tracking_sessions WHERE org_id=$1 AND owner_membership_id=$2 AND policy='EXPECTED'),
				(SELECT count(*) FROM org_tracking_segments WHERE org_id=$1 AND author_membership_id=$2),
				(SELECT COALESCE(sum(stored_count),0) FROM org_tracking_segments WHERE org_id=$1 AND author_membership_id=$2),
				(SELECT COALESCE(sum(received_count),0) FROM org_tracking_segments WHERE org_id=$1 AND author_membership_id=$2),
				(SELECT COALESCE(sum(duplicate_count),0) FROM org_tracking_segments WHERE org_id=$1 AND author_membership_id=$2),
				(SELECT COALESCE(sum(outside_session_count),0) FROM org_tracking_segments WHERE org_id=$1 AND author_membership_id=$2),
				(SELECT COALESCE(sum(mock_count),0) FROM org_tracking_segments WHERE org_id=$1 AND author_membership_id=$2),
				(SELECT count(*) FROM (SELECT seq-row_number() OVER (ORDER BY seq) AS d FROM org_tracking_segments WHERE org_id=$1 AND author_membership_id=$2) q WHERE d<>0),
				(SELECT count(*) FROM org_trips WHERE org_id=$1 AND author_membership_id=$2 AND state='COMPLETED'),
				(SELECT count(*) FROM org_sync_operations WHERE org_id=$1 AND membership_id=$2),
				(SELECT count(*) FROM org_sync_acknowledgements a JOIN org_sync_operations o ON o.org_id=a.org_id AND o.operation_id=a.operation_id
				  WHERE a.org_id=$1 AND o.membership_id=$2 AND a.result='ACCEPTED')`, org, person.member).Scan(&c.sessions, &c.policyRevision,
				&c.segments, &c.stored, &c.received, &c.duplicates, &c.outside, &c.mock, &c.sequenceGaps, &c.completed, &c.operations, &c.accepted); err != nil {
				t.Fatal(err)
			}
			t.Logf("native tracking counts %+v", c)
			// One session under policy revision 1; at least the batch sealed at the stop and the one sealed at recovery; every
			// received fix stored once inside the session; consecutive sequences; every operation of this member accepted.
			if c.sessions != 1 || c.policyRevision != 1 || c.segments < 2 || c.stored < 2 || c.stored != c.received || c.duplicates != 0 ||
				c.outside != 0 || c.mock != 0 || c.sequenceGaps != 0 || c.completed != 1 || c.operations < 9 || c.accepted != c.operations {
				t.Fatalf("native tracking effects differ: %+v", c)
			}
			var queued int
			if err := x.f.Admin.QueryRow(t.Context(), `SELECT count(*) FROM job_queue WHERE kind='org-tracking-process'`).Scan(&queued); err != nil {
				t.Log("processing job not inspected", err)
			} else {
				t.Logf("track processing jobs queued: %d", queued)
			}
			if _, err := os.Stat(filepath.Join(dir, "expect.json")); err == nil {
				t.Log("device expectations present")
			}
			return
		}
	}
}
