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
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	identitypg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/adapters"
	identityweb "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/adapters/http"
	identity "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/enterprisesuite"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

// TestSyncReviewNativeSourceFixture is an opt-in actual default enterprise runtime
// (real Meter intake, real review store and the real signed own-result route)
// for the Android API36 review acceptance. It is never a product endpoint or a
// test-control API. Its only extra behavior discards one real resolution answer
// after the server produced it, proving lost-answer recovery on Android, and it
// records company decisions through the actual reviewer store after the device
// has observed each held review unresolved.
func TestSyncReviewNativeSourceFixture(t *testing.T) {
	const host = "api.anpfuel.example.invalid"
	const origin = "https://" + host
	dir := os.Getenv("EMP_OFFL008_NATIVE_FIXTURE_DIR")
	if !filepath.IsAbs(dir) || !strings.HasPrefix(filepath.Clean(dir), "/tmp/") {
		t.Fatal("native review source requires a private absolute /tmp directory")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(dir); err != nil || info.Mode().Perm() != 0700 {
		t.Fatal("native review source directory must have mode0700")
	}
	fx, _, installedA := meterEvidenceFixture(t)
	reviews := meterReviewStore(t, fx)
	// Second organization: the same account as an ordinary member with its own meter.
	other := legalFixtureFor(t, fx.f, fx.onboarding, fx.actor, fx.flags)
	_, memberB := teamDriver(t, other, fx.person.auth.Account.ID)
	if _, _, err := other.onboarding.Repository.ActivateOrganization(t.Context(), other.scope, other.actor, other.member, onboardingID(t), 1); err != nil {
		t.Fatal(err)
	}
	assetsB := fleetStore(t, other)
	assetB, _, err := fleetWrite(t, other, assetsB, fleetCommand(t, fleetCreateInput()))
	if err != nil {
		t.Fatal("second organization asset", err)
	}
	installedB, _, err := meterInstall(t, other, meterStore(t, other), meterInstallCommand(t, meterInstallInput(t, assetB.AssetID, time.Now().UTC().Add(-4*time.Hour))))
	if err != nil {
		t.Fatal("second organization meter", err)
	}
	b := fx
	b.deviceFixtureState.legalFixtureState = other
	// A formerly effective ORG grant that later ended: the real disposition holds
	// PERMISSION_CHANGED for a later declaration instead of applying or forbidding.
	matrixAssign(t, fx.deviceFixtureState, matrixPerson{member: fx.person.member}, "FINANCE", "ORG", "ACTIVE")
	matrixAssign(t, b.deviceFixtureState, matrixPerson{member: memberB}, "ADMIN", "ORG", "ACTIVE")
	matrixAssign(t, b.deviceFixtureState, matrixPerson{member: memberB}, "FINANCE", "ORG", "ACTIVE")
	role := pgx.Identifier{fx.f.RuntimeRole}.Sanitize()
	if _, err := fx.f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT,UPDATE ON identity_rate_windows,identity_contributors,identity_keys,identity_challenges,identity_idempotency,account_contributor_bindings,account_binding_audit,account_session_refresh_history TO "+role); err != nil {
		t.Fatal(err)
	}
	var unsafe bool
	if err := fx.f.Runtime.QueryRow(t.Context(), `SELECT rolsuper OR rolbypassrls FROM pg_roles WHERE rolname=current_user`).Scan(&unsafe); err != nil || unsafe {
		t.Fatal("native review source requires an ordinary runtime", err)
	}
	registrar := identitypg.NewRegistrar(fx.f.Runtime)
	accounts := *fx.accounts
	accounts.Store, accounts.Clock = accountpg.NewPGStore(fx.f.Runtime), nativeContactClock{}
	accounts.KeyProver = keyprover.Prover{Now: time.Now, Keys: func(ctx context.Context, fp string) (string, string, string, bool, error) {
		key, err := registrar.FindDeviceKey(ctx, fp)
		return key.ContributorID, key.JWKX, key.JWKY, key.Revoked, err
	}}
	router := chi.NewRouter()
	secret := bytes.Repeat([]byte{97}, 32)
	identityweb.Handler{Registrar: registrar, Authority: host, QuotaSecret: secret,
		CheckQuota: identitypg.NewLimiter(fx.f.Runtime, identity.DefaultQuotaPolicy()).Check}.RegisterRoutes(router)
	accountweb.Handler{Service: &accounts}.RegisterRoutes(router)
	// No injected handler, consumer, budget or plan: the real default Meter runtime.
	if err := enterprisesuite.Register(router, enterprisesuite.Options{Enabled: true, Origin: origin, CursorSecret: secret, Pool: fx.f.Runtime, Accounts: &accounts}); err != nil {
		t.Fatal(err)
	}
	orgA, orgB := fx.scope.OrgID().String(), other.scope.OrgID().String()
	var mu sync.Mutex
	unresolvedPolls := map[string]int{}
	reviewRequests := map[string]int{}
	operationRequests := map[string]int{}
	var lostResolution []byte
	recovered := false
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		org := ""
		for _, candidate := range []string{orgA, orgB} {
			if strings.HasPrefix(r.URL.Path, "/v1/orgs/"+candidate+"/") {
				org = candidate
			}
		}
		if strings.HasSuffix(r.URL.Path, "/sync/operations") {
			mu.Lock()
			operationRequests[org]++
			mu.Unlock()
		}
		if !strings.HasSuffix(r.URL.Path, "/sync/review-results") {
			router.ServeHTTP(w, r)
			return
		}
		answer := httptest.NewRecorder()
		router.ServeHTTP(answer, r)
		var wire struct {
			Results []struct {
				Resolution json.RawMessage `json:"resolution"`
			} `json:"results"`
		}
		decided := []byte(nil)
		ok := answer.Code == http.StatusOK && json.Unmarshal(answer.Body.Bytes(), &wire) == nil && len(wire.Results) > 0
		if ok {
			for _, result := range wire.Results {
				if len(result.Resolution) != 0 && string(result.Resolution) != "null" {
					decided = bytes.Clone(result.Resolution)
				}
			}
		}
		mu.Lock()
		reviewRequests[org]++
		discard := false
		if ok && decided == nil {
			unresolvedPolls[org]++
		}
		if ok && decided != nil && org == orgA {
			if lostResolution == nil {
				lostResolution, discard = decided, true
			} else if bytes.Equal(lostResolution, decided) {
				// The immutable decision is replayed byte-for-byte to a fresh request.
				recovered = true
			}
		}
		mu.Unlock()
		if discard {
			hijacker, ok := w.(http.Hijacker)
			if !ok {
				t.Error("native review fixture requires HTTP1 lost-answer transport")
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			connection, _, err := hijacker.Hijack()
			if err != nil {
				t.Error("native review source could not discard the resolution answer")
				return
			}
			_ = connection.Close()
			return
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
		"account_id": fx.person.auth.Account.ID, "family_id": fx.person.auth.Session.FamilyID,
		"access_token": fx.person.auth.Session.AccessToken, "refresh_token": fx.person.auth.Session.RefreshToken,
		"access_expires_at": fx.person.auth.Session.AccessExpires, "refresh_expires_at": fx.person.auth.Session.AbsoluteExpires,
		"operation_type": "meter.reading.record", "organizations": []map[string]string{
			{"org_id": orgA, "membership_id": fx.person.member, "asset_id": installedA.AssetID, "meter_id": installedA.MeterID, "segment_id": installedA.SegmentID},
			{"org_id": orgB, "membership_id": memberB, "asset_id": installedB.AssetID, "meter_id": installedB.MeterID, "segment_id": installedB.SegmentID}}})
	if err != nil {
		t.Fatal(err)
	}
	write("ca.pem", public)
	write("fixture.json", fixture)
	t.Log("ordinary two-organization signed review source ready; proof remains in private fixture")
	type held struct {
		org, operation, review string
		ack                    []byte
	}
	heldFor := func(org string) (held, bool) {
		var h held
		h.org = org
		err := fx.f.Admin.QueryRow(t.Context(), `SELECT i.operation_id::text,i.id::text,a.canonical_ack FROM org_sync_review_items i JOIN org_sync_acknowledgements a ON(a.org_id,a.operation_id,a.review_item_id)=(i.org_id,i.operation_id,i.id) JOIN org_sync_operations o ON(o.org_id,o.operation_id)=(i.org_id,i.operation_id) WHERE i.org_id=$1 AND o.owner_account_id=$2 AND i.proof_version=1 AND a.result='ACCEPTED_FOR_REVIEW'`, org, fx.person.auth.Account.ID).Scan(&h.operation, &h.review, &h.ack)
		return h, err == nil
	}
	var decided []held
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-t.Context().Done():
			t.Fatal("native review fixture ended before acceptance stop")
		case <-ticker.C:
			mu.Lock()
			ready := unresolvedPolls[orgA] > 0 && unresolvedPolls[orgB] > 0
			mu.Unlock()
			if ready && decided == nil {
				a, okA := heldFor(orgA)
				bHeld, okB := heldFor(orgB)
				if !okA || !okB {
					t.Fatal("device observed unresolved reviews without actual held source rows")
				}
				for i, h := range []held{a, bHeld} {
					kind, reason := app.MeterReviewAccept, ""
					scope, legal, reviewer := fx.scope, fx.legalFixtureState, fx.member
					if i == 1 {
						kind, reason = app.MeterReviewRefuse, "Synthetic native reviewer refusal 😀"
						scope, legal, reviewer = other.scope, other, other.member
					}
					id, err := jobs.NewUUIDv4()
					if err != nil {
						t.Fatal(err)
					}
					command, err := app.NewMeterReviewDecision(app.MeterReviewDecisionInput{OperationID: id, ReviewItemID: h.review, ExpectedRevision: 1, Decision: kind, Reason: reason})
					if err != nil {
						t.Fatal(err)
					}
					if _, replay, err := reviews.ResolveMeterReview(t.Context(), scope, fleetActor(legal), reviewer, command); err != nil || replay {
						t.Fatal("actual company reviewer decision", err)
					}
				}
				decided = []held{a, bHeld}
				write("resolved", []byte("1"))
				t.Log("actual reviewer decisions recorded after unresolved device polls")
			}
			if _, err := os.Stat(filepath.Join(dir, "stop")); os.IsNotExist(err) {
				continue
			} else if err != nil {
				t.Fatal(err)
			}
			mu.Lock()
			lost, replayed := lostResolution != nil, recovered
			counts := map[string]int{"opsA": operationRequests[orgA], "opsB": operationRequests[orgB], "reviewA": reviewRequests[orgA], "reviewB": reviewRequests[orgB]}
			mu.Unlock()
			t.Logf("native review request counts %v", counts)
			if decided == nil || !lost || !replayed {
				t.Fatal("native acceptance missed the decision, lost answer or exact replay", decided == nil, lost, replayed)
			}
			for i, h := range decided {
				var ack []byte
				var outcome string
				var readings int
				if err := fx.f.Admin.QueryRow(t.Context(), `SELECT a.canonical_ack,r.outcome,(SELECT count(*) FROM org_meter_readings m WHERE m.org_id=a.org_id AND m.operation_id=a.operation_id) FROM org_sync_acknowledgements a JOIN org_sync_review_resolutions r ON(r.org_id,r.review_item_id)=(a.org_id,a.review_item_id) WHERE a.org_id=$1 AND a.operation_id=$2`, h.org, h.operation).Scan(&ack, &outcome, &readings); err != nil {
					t.Fatal("native decision rows", err)
				}
				want, wantReadings := "ACCEPTED", 1
				if i == 1 {
					want, wantReadings = "REFUSED", 0
				}
				if !bytes.Equal(ack, h.ack) || outcome != want || readings != wantReadings {
					t.Fatal("native review rewrote an original ACK or applied the wrong effect", i, outcome, readings)
				}
			}
			var exact bool
			if err := fx.f.Admin.QueryRow(t.Context(), `SELECT (SELECT count(*) FROM org_sync_review_resolutions WHERE org_id IN($1,$2))=2 AND (SELECT count(*) FROM org_sync_review_items WHERE org_id IN($1,$2))=2 AND (SELECT count(*) FROM org_meter_held_readings WHERE org_id IN($1,$2))=2`, orgA, orgB).Scan(&exact); err != nil || !exact {
				t.Fatal("native source did not converge on exactly two held items and decisions", err)
			}
			return
		}
	}
}
