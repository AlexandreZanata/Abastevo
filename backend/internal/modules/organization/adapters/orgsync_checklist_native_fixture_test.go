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
	maintpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/maintenance/adapters"
	maintapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/maintenance/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/maintenance/domain/checklist"
	orgpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters"
	auditpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/adapters"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/enterprisesuite"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

// TestChecklistNativeSourceFixture is an opt-in actual default enterprise
// runtime (real account, identity, device contact, company cookie session,
// the EMP-CHECK-001 applicable read, the EMP-CHECK-002 signed intake and run
// read) for the Android API 36 acceptance of EMP-CHECK-002. It is never a
// product endpoint or a test-control API. Its only extra behaviour is the
// owner's own policy changes of the DEPARTURE template through the real
// template store, made when the device driver asks for them, so a run the
// device computed on a cached WARN revision is rechecked under BLOCK.
func TestChecklistNativeSourceFixture(t *testing.T) {
	const host = "api.anpfuel.example.invalid"
	const origin = "https://" + host
	dir := os.Getenv("EMP_CHECK002_NATIVE_FIXTURE_DIR")
	if !filepath.IsAbs(dir) || !strings.HasPrefix(filepath.Clean(dir), "/data/dev/") {
		t.Fatal("native checklist source requires a private absolute /data/dev directory")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(dir); err != nil || info.Mode().Perm() != 0700 {
		t.Fatal("native checklist source directory must have mode0700")
	}
	x := tripFixtureOn(t, selfIntakeFixture(t))
	x.f.GrantReferenceFeed(t)
	tripReviewGrants(t, x.f)
	grantChecklistTables(t, x.legalFixtureState)
	runGrants(t, x.f)
	role := pgx.Identifier{x.f.RuntimeRole}.Sanitize()
	if _, err := x.f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT ON org_trip_events,org_trip_held_events TO "+role); err != nil {
		t.Fatal(err)
	}
	person := x.person
	// The owner's DEPARTURE template for the organization: BLOCK, Luzes optional, Freios critical, Pneus.
	catalog, err := audit.NewCatalog(append(maintpg.ChecklistAuditDefinitions(), maintpg.RunAuditDefinitions()...))
	if err != nil {
		t.Fatal(err)
	}
	templates := &maintpg.ChecklistStore{Pool: x.f.Runtime, Authority: &orgpg.ChecklistAuthority{Memberships: x.legalFixtureState.store.Memberships, Roles: x.legalFixtureState.store.Roles,
		Capabilities: x.flags, Structure: &orgpg.ResourceStructure{Kind: orgpg.AssetStructure, Pool: x.f.Runtime, Allocations: x.assets},
		Assignee: &orgpg.ResourceAssignee{Pool: x.f.Runtime, Assignments: x.assets}}, Audit: auditpg.NewStore(x.f.Runtime, catalog)}
	template, _, err := templateRun(t, x.legalFixtureState, templates, templateCommand(t, maintapp.TemplateCommandInput{Kind: maintapp.CommandCreate,
		Context: checklist.Departure, Name: "Checklist de saída sintético", Target: organizationTarget(), Items: runItems(), Policy: checklist.DefaultPolicy()}))
	if err != nil || template.PolicyRevision != 1 {
		t.Fatal("departure template fixture", err)
	}
	policy := func(mode checklist.Mode) {
		t.Helper()
		next, _, err := templateRun(t, x.legalFixtureState, templates, templateCommand(t, maintapp.TemplateCommandInput{Kind: maintapp.CommandPolicy,
			TemplateID: template.TemplateID, ExpectedRevision: template.Revision, Policy: checklist.Policy{Mode: mode, ValiditySeconds: 43200}}))
		if err != nil {
			t.Fatal("policy change", mode, err)
		}
		template = next
	}
	if _, err := x.f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT,UPDATE ON identity_rate_windows,identity_contributors,identity_keys,identity_challenges,identity_idempotency,account_contributor_bindings,account_binding_audit,account_session_refresh_history TO "+role); err != nil {
		t.Fatal(err)
	}
	var unsafe bool
	if err := x.f.Runtime.QueryRow(t.Context(), `SELECT rolsuper OR rolbypassrls FROM pg_roles WHERE rolname=current_user`).Scan(&unsafe); err != nil || unsafe {
		t.Fatal("native checklist source requires an ordinary runtime", err)
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
	// No injected handler or store: the real default enterprise runtime with its checklist owners.
	if err := enterprisesuite.Register(router, enterprisesuite.Options{Enabled: true, Origin: origin, CursorSecret: secret, Pool: x.f.Runtime, Accounts: &accounts}); err != nil {
		t.Fatal(err)
	}
	org := x.scope.OrgID().String()
	var mu sync.Mutex
	statuses := map[string][]int{}
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		kind := ""
		switch {
		case strings.HasSuffix(r.URL.Path, "/sync/operations"):
			kind = "intake"
		case strings.HasSuffix(r.URL.Path, "/applicable"):
			kind = "applicable"
		case strings.Contains(r.URL.Path, "/checklists/") && strings.HasSuffix(r.URL.Path, "/read"):
			kind = "run-read"
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
		if kind == "intake" {
			// Synthetic acknowledgements only (operation identifiers, digests, results, codes); the request is never logged.
			body := answer.Body.String()
			if len(body) > 4096 {
				body = body[:4096]
			}
			t.Logf("intake answer %d %s", answer.Code, body)
		} else {
			t.Logf("company %s answer %d", kind, answer.Code)
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
		"org_id": org, "membership_id": person.member, "asset_id": x.vehicle.AssetID, "template_id": template.TemplateID})
	if err != nil {
		t.Fatal(err)
	}
	write("ca.pem", public)
	write("fixture.json", fixture)
	t.Log("ordinary signed checklist source ready; proof remains in private fixture")
	exists := func(name string) bool {
		_, err := os.Stat(filepath.Join(dir, name))
		if err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
		return err == nil
	}
	warned, blocked := false, false
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-t.Context().Done():
			t.Fatal("native checklist fixture ended before acceptance stop")
		case <-ticker.C:
			if exists("warn") && !warned {
				// The owner relaxes the policy; the device caches this WARN revision online.
				policy(checklist.Warn)
				warned = true
				write("warned", []byte("1"))
				t.Log("departure policy is WARN at revision", template.PolicyRevision)
			}
			if exists("block") && !blocked {
				// The owner restores BLOCK while the device is offline with the cached WARN revision.
				policy(checklist.Block)
				blocked = true
				write("blocked", []byte("1"))
				t.Log("departure policy is BLOCK at revision", template.PolicyRevision)
			}
			if !exists("stop") {
				continue
			}
			mu.Lock()
			intake, applicable, reads := append([]int(nil), statuses["intake"]...), append([]int(nil), statuses["applicable"]...), append([]int(nil), statuses["run-read"]...)
			mu.Unlock()
			t.Logf("native checklist requests intake=%v applicable=%v run-read=%v", intake, applicable, reads)
			if !warned || !blocked {
				t.Fatal("native acceptance stopped before both policy changes")
			}
			for _, codes := range [][]int{intake, applicable, reads} {
				for _, code := range codes {
					if code != http.StatusOK {
						t.Fatal("a device request was refused", code)
					}
				}
			}
			if len(applicable) != 2 || len(reads) < 2 {
				t.Fatal("the device did not read the template twice and both runs", len(applicable), len(reads))
			}
			type counts struct{ runs, answers, operations, acks, accepted int }
			var c counts
			if err := x.f.Admin.QueryRow(t.Context(), `SELECT
				(SELECT count(*) FROM org_checklist_runs WHERE org_id=$1 AND author_membership_id=$2),
				(SELECT count(*) FROM org_checklist_run_answers a JOIN org_checklist_runs r ON (r.org_id,r.id)=(a.org_id,a.run_id) WHERE r.org_id=$1 AND r.author_membership_id=$2),
				(SELECT count(*) FROM org_sync_operations WHERE org_id=$1 AND membership_id=$2),
				(SELECT count(*) FROM org_sync_acknowledgements a JOIN org_sync_operations o ON o.org_id=a.org_id AND o.operation_id=a.operation_id WHERE a.org_id=$1 AND o.membership_id=$2),
				(SELECT count(*) FROM org_sync_acknowledgements a JOIN org_sync_operations o ON o.org_id=a.org_id AND o.operation_id=a.operation_id WHERE a.org_id=$1 AND o.membership_id=$2 AND a.result='ACCEPTED')`,
				org, person.member).Scan(&c.runs, &c.answers, &c.operations, &c.acks, &c.accepted); err != nil {
				t.Fatal(err)
			}
			t.Logf("native checklist counts %+v", c)
			// Two runs of this member, three answers each, two operations each answered once ACCEPTED; nothing refused.
			if c != (counts{runs: 2, answers: 6, operations: 2, acks: 2, accepted: 2}) {
				t.Fatalf("native checklist effects differ: %+v", c)
			}
			rows, err := x.f.Admin.Query(t.Context(), `SELECT device_outcome||':'||recheck_outcome||':'||outcome||':'||outcome_differs||':'||template_superseded||':'||
				policy_revision||':'||server_policy_revision||':'||revision||':'||acknowledged||':'||(SELECT string_agg(position||answer||COALESCE(':'||length(note)::text,''),',' ORDER BY position)
				FROM org_checklist_run_answers a WHERE a.org_id=r.org_id AND a.run_id=r.id)
				FROM org_checklist_runs r WHERE r.org_id=$1 AND r.author_membership_id=$2 ORDER BY r.occurred_at`, org, person.member)
			if err != nil {
				t.Fatal(err)
			}
			var stored []string
			for rows.Next() {
				var v string
				if err := rows.Scan(&v); err != nil {
					t.Fatal(err)
				}
				stored = append(stored, v)
			}
			rows.Close()
			t.Logf("native checklist runs %v", stored)
			// BUC-EMP-222/223: the first run agrees (BLOCKING); the second was computed on the cached WARN revision 2
			// while BLOCK (revision 3) was in force, so it is stored BLOCKING with the device's claim, held for review.
			want := []string{"BLOCKING:BLOCKING:BLOCKING:false:false:1:1:1:false:1OK,2NOT_OK:15,3OK",
				"COMPLETED_WITH_ISSUES:BLOCKING:BLOCKING:true:false:2:3:1:true:1OK,2NOT_OK,3OK"}
			if strings.Join(stored, "|") != strings.Join(want, "|") {
				t.Fatalf("stored runs differ:\n got %v\nwant %v", stored, want)
			}
			return
		}
	}
}
