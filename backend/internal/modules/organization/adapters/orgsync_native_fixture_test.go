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
	"sync/atomic"
	"testing"
	"time"

	accountpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/adapters"
	accountweb "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/adapters/http"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/adapters/keyprover"
	identitypg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/adapters"
	identityweb "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/adapters/http"
	identity "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	eventpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgevent/adapters"
	event "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgevent/domain"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/enterprisesuite"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgquota"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

// TestSyncQueueNativeSourceFixture is an opt-in actual source, never a product
// endpoint or test-control API. Its only extra behavior discards one real answer
// after normal intake commits, proving lost-answer recovery on Android.
func TestSyncQueueNativeSourceFixture(t *testing.T) {
	const nativeQueueHost = "api.anpfuel.example.invalid"
	const nativeQueueOrigin = "https://" + nativeQueueHost
	dir := os.Getenv("EMP_OFFL003_NATIVE_FIXTURE_DIR")
	if !filepath.IsAbs(dir) || !strings.HasPrefix(filepath.Clean(dir), "/tmp/") {
		t.Fatal("native queue source requires a private absolute /tmp directory")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(dir); err != nil || info.Mode().Perm() != 0700 {
		t.Fatal("native queue source directory must have mode0700")
	}
	fx := syncFixture(t)
	other := legalFixtureFor(t, fx.f, fx.onboarding, fx.actor, fx.flags)
	_, member := teamDriver(t, other, fx.person.auth.Account.ID)
	if _, _, err := other.onboarding.Repository.ActivateOrganization(t.Context(), other.scope, other.actor, other.member, onboardingID(t), 1); err != nil {
		t.Fatal(err)
	}
	b := fx
	b.deviceFixtureState.legalFixtureState = other
	matrixAssign(t, b.deviceFixtureState, matrixPerson{member: member}, "ADMIN", "ORG", "ACTIVE")
	target := onboardingID(t)
	if err := unitSchemaExec(t, other, `INSERT INTO org_sync_test_targets VALUES($1,$2,$3,1)`, other.scope.OrgID().String(), target, member); err != nil {
		t.Fatal(err)
	}
	role := pgx.Identifier{fx.f.RuntimeRole}.Sanitize()
	if _, err := fx.f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT,UPDATE ON identity_rate_windows,identity_contributors,identity_keys,identity_challenges,identity_idempotency,account_contributor_bindings,account_binding_audit,account_session_refresh_history TO "+role); err != nil {
		t.Fatal(err)
	}
	var unsafe bool
	if err := fx.f.Runtime.QueryRow(t.Context(), `SELECT rolsuper OR rolbypassrls FROM pg_roles WHERE rolname=current_user`).Scan(&unsafe); err != nil || unsafe {
		t.Fatal("native source requires an ordinary runtime", err)
	}
	registrar := identitypg.NewRegistrar(fx.f.Runtime)
	accounts := *fx.accounts
	accounts.Store, accounts.Clock = accountpg.NewPGStore(fx.f.Runtime), nativeContactClock{}
	accounts.KeyProver = keyprover.Prover{Now: time.Now, Keys: func(ctx context.Context, fp string) (string, string, string, bool, error) {
		key, err := registrar.FindDeviceKey(ctx, fp)
		return key.ContributorID, key.JWKX, key.JWKY, key.Revoked, err
	}}
	consumers := map[string]eventpg.Effect{syncTestConsumer: func(ctx context.Context, scope kernel.OrgScope, tx pgx.Tx, e event.Event) error {
		_, err := tx.Exec(ctx, `INSERT INTO org_sync_test_followups VALUES($1,$2,$3)`, scope.OrgID().String(), e.ID, e.Name)
		return err
	}}
	_, consumer, err := enterprisesuite.IntakeEvents(fx.f.Runtime, consumers)
	if err != nil || consumer == nil {
		t.Fatal("native follow-up registration", err)
	}
	quota := identity.OperationQuota{Limit: 2000, Window: time.Hour}
	budget := orgquota.TransportDefinition{Class: "validation.native", Organization: quota, Account: quota, Device: quota,
		Rationale: "Explicit synthetic API36 queue acceptance: bounded native attempts below2000/hour on each axis; no production default."}
	router := chi.NewRouter()
	secret := bytes.Repeat([]byte{95}, 32)
	identityweb.Handler{Registrar: registrar, Authority: nativeQueueHost, QuotaSecret: secret,
		CheckQuota: identitypg.NewLimiter(fx.f.Runtime, identity.DefaultQuotaPolicy()).Check}.RegisterRoutes(router)
	accountweb.Handler{Service: &accounts}.RegisterRoutes(router)
	if err := enterprisesuite.Register(router, enterprisesuite.Options{Enabled: true, Origin: nativeQueueOrigin, CursorSecret: secret,
		Pool: fx.f.Runtime, Accounts: &accounts, IntakeHandlers: []syncapp.Handler{fx.handler}, IntakeBudget: &budget,
		IntakePlans: fx.config.Plans, IntakeConsumers: consumers}); err != nil {
		t.Fatal(err)
	}
	var dropped atomic.Bool
	var replayMu sync.Mutex
	var lostAnswer []byte
	var recoveredAnswer bool
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/sync/operations") {
			router.ServeHTTP(w, r)
			return
		}
		answer := httptest.NewRecorder()
		router.ServeHTTP(answer, r)
		var receipt struct {
			Acknowledgements []struct{ Result string } `json:"acknowledgements"`
		}
		committed := false
		if answer.Code == http.StatusOK && json.Unmarshal(answer.Body.Bytes(), &receipt) == nil {
			for _, ack := range receipt.Acknowledgements {
				committed = committed || ack.Result == "ACCEPTED"
			}
		}
		replayMu.Lock()
		discard := committed && dropped.CompareAndSwap(false, true)
		if discard {
			lostAnswer = bytes.Clone(answer.Body.Bytes())
		} else if committed && len(lostAnswer) != 0 && bytes.Equal(lostAnswer, answer.Body.Bytes()) {
			// Equal immutable ACK bytes prove replay of the lost operation ID,
			// full intent digest, original timestamp and business revision.
			recoveredAnswer = true
		}
		replayMu.Unlock()
		if discard {
			hijacker, ok := w.(http.Hijacker)
			if !ok {
				t.Error("native fixture requires HTTP1 lost-answer transport")
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			connection, _, err := hijacker.Hijack()
			if err != nil {
				t.Error("native source could not discard committed answer")
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
	template := &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: nativeQueueHost}, DNSNames: []string{nativeQueueHost},
		NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true,
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
	fixture, err := json.Marshal(map[string]any{"origin": nativeQueueOrigin, "port": port, "certificate_pem": string(public),
		"account_id": fx.person.auth.Account.ID, "family_id": fx.person.auth.Session.FamilyID,
		"access_token": fx.person.auth.Session.AccessToken, "refresh_token": fx.person.auth.Session.RefreshToken,
		"access_expires_at": fx.person.auth.Session.AccessExpires, "refresh_expires_at": fx.person.auth.Session.AbsoluteExpires,
		"operation_type": syncTestType, "organizations": []map[string]string{
			{"org_id": fx.scope.OrgID().String(), "membership_id": fx.person.member, "target_id": fx.target},
			{"org_id": other.scope.OrgID().String(), "membership_id": member, "target_id": target}}})
	if err != nil {
		t.Fatal(err)
	}
	write("ca.pem", public)
	write("fixture.json", fixture)
	t.Log("ordinary two-organization signed queue source ready; proof remains in private fixture")
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-t.Context().Done():
			t.Fatal("native queue fixture ended before acceptance stop")
		case <-ticker.C:
			if _, err := os.Stat(filepath.Join(dir, "stop")); os.IsNotExist(err) {
				continue
			} else if err != nil {
				t.Fatal(err)
			}
			if !dropped.Load() {
				t.Fatal("actual lost-answer intake was not reached")
			}
			replayMu.Lock()
			replayed := recoveredAnswer
			replayMu.Unlock()
			if !replayed {
				t.Fatal("the exact immutable lost acknowledgement was not replayed")
			}
			dispatcher := jobs.Dispatcher{Queue: jobs.NewQueue(fx.f.Runtime), Handlers: map[string]jobs.Handler{consumer.Kind(): consumer}, Kind: consumer.Kind(), WorkerID: "synthetic-native-queue"}
			for range 100 {
				found, err := dispatcher.RunOnce(t.Context())
				if err != nil {
					t.Fatal("native follow-up dispatch", err)
				}
				if !found {
					var complete bool
					if err := fx.f.Admin.QueryRow(t.Context(), `
SELECT (SELECT count(*)=2 AND count(DISTINCT org_id)=2 FROM org_sync_test_records)
 AND (SELECT count(*)=2 FROM org_sync_operations)
 AND (SELECT count(*)=2 FROM org_sync_effects)
 AND (SELECT count(*)=2 FROM org_sync_acknowledgements)
 AND (SELECT count(*)=2 FROM org_sync_test_followups)
 AND NOT EXISTS(
  SELECT 1 FROM org_sync_test_records r
  LEFT JOIN org_sync_operations o USING(org_id,operation_id)
  LEFT JOIN org_sync_effects e USING(org_id,operation_id)
  LEFT JOIN org_sync_acknowledgements a USING(org_id,operation_id)
  LEFT JOIN org_sync_test_followups f USING(org_id,operation_id)
  WHERE o.operation_id IS NULL OR e.operation_id IS NULL OR a.operation_id IS NULL OR f.operation_id IS NULL
   OR o.owner_account_id<>$1::uuid OR o.membership_id<>r.author_member
   OR o.entity_id<>r.target_id OR o.operation_type<>$2 OR o.entity_type<>'setting'
   OR a.result<>'ACCEPTED' OR a.entity_revision<>2 OR e.entity_revision<>2
   OR r.held OR r.review_id IS NOT NULL OR cardinality(r.links)<>0 OR cardinality(r.proposed_refs)<>0
   OR NOT ((r.org_id,r.target_id,r.author_member)=($3::uuid,$4::uuid,$5::uuid)
        OR (r.org_id,r.target_id,r.author_member)=($6::uuid,$7::uuid,$8::uuid)))
 AND (SELECT count(*)=2 AND bool_and(revision=2) FROM org_sync_test_targets)`,
						fx.person.auth.Account.ID, syncTestType,
						fx.scope.OrgID().String(), fx.target, fx.person.member,
						other.scope.OrgID().String(), target, member).Scan(&complete); err != nil || !complete {
						t.Fatal("native source did not converge on independent effects/ACKs/follow-ups", err)
					}
					return
				}
			}
			t.Fatal("native acceptance exceeded bounded follow-up count")
		}
	}
}
