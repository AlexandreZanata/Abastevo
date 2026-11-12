//go:build integration

package adapters_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	accountpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/adapters"
	account "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/application"
	accountdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
	companypg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/adapters"
	company "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	memberpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/adapters"
	memberapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/application"
	member "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/domain"
	accesspg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/adapters"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
	auditpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/adapters"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	cappg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/adapters"
	cap "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/enterprisesuite"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

type onboardingClock struct{ now time.Time }

func (c onboardingClock) NowUnix() int64 { return c.now.Unix() }
func onboardingFixture(t *testing.T) (*orgtest.Fixture, *adapters.OnboardingStore, app.Onboarding, app.CreationActor, *cappg.Store) {
	t.Helper()
	return onboardingOnFixture(t, orgtest.New(t))
}
func onboardingOnFixture(t *testing.T, f *orgtest.Fixture) (*orgtest.Fixture, *adapters.OnboardingStore, app.Onboarding, app.CreationActor, *cappg.Store) {
	t.Helper()
	f.Runtime = f.RuntimePool(t, 12)
	r := pgx.Identifier{f.RuntimeRole}.Sanitize()
	if _, err := f.Admin.Exec(t.Context(), "GRANT USAGE ON SCHEMA public TO "+r+"; GRANT SELECT,INSERT,UPDATE ON organizations,org_units,org_employee_identities,org_memberships,org_role_assignments,org_capabilities TO "+r+"; GRANT SELECT,INSERT ON org_onboarding_commands,org_audit_events TO "+r+"; GRANT SELECT,UPDATE ON accounts,account_session_families TO "+r+"; GRANT SELECT ON payment_billing_exits TO "+r+"; GRANT SELECT,INSERT,UPDATE,DELETE ON company_web_sessions TO "+r); err != nil {
		t.Fatal(err)
	}
	var profilesInstalled bool
	if err := f.Admin.QueryRow(t.Context(), `SELECT to_regclass('public.org_employee_profiles') IS NOT NULL`).Scan(&profilesInstalled); err != nil {
		t.Fatal(err)
	}
	if profilesInstalled {
		if _, err := f.Admin.Exec(t.Context(), "GRANT SELECT ON org_employee_profiles TO "+r); err != nil {
			t.Fatal(err)
		}
	}

	now := time.Now().UTC().Truncate(time.Second)
	accounts := &account.Service{Clock: onboardingClock{now}, Store: accountpg.NewPGStore(f.Admin), Hasher: accountdomain.SHA256Hasher{}, IDGen: jobs.NewUUIDv4, TokenGen: accountdomain.GenerateToken, KeyGen: accountdomain.GenerateAccountKey}
	key, err := accounts.CreateKeyAccount(t.Context(), "syntheticcreator")
	if err != nil {
		t.Fatal(err)
	}
	auth, err := accounts.LoginWithKey(t.Context(), key.Key)
	if err != nil {
		t.Fatal(err)
	}
	companyService := &company.Service{Enabled: true, Store: &companypg.Store{Pool: f.Runtime}, Hasher: accountdomain.SHA256Hasher{}, Prove: companypg.AccountProof(accounts), Now: func() time.Time { return now }, NewID: jobs.NewUUIDv4, NewSecret: accountdomain.GenerateToken}
	cookie, err := companyService.Login(t.Context(), auth.Session.FamilyID, auth.Session.AccessToken, "")
	if err != nil {
		t.Fatal(err)
	}
	defs := append(adapters.OnboardingAuditDefinitions(), cappg.AuditDefinitions()...)
	auditCatalog, err := audit.NewCatalog(defs)
	if err != nil {
		t.Fatal(err)
	}
	auditStore := auditpg.NewStore(f.Runtime, auditCatalog)
	catalog, err := cap.NewCatalog([]string{"organizations"})
	if err != nil {
		t.Fatal(err)
	}
	flags := cappg.NewStore(f.Runtime, catalog, auditStore)
	store := &adapters.OnboardingStore{Pool: f.Runtime, Roots: adapters.NewStore(f.Runtime), Memberships: memberpg.NewStore(f.Runtime), Roles: accesspg.NewStore(f.Runtime), Audit: auditStore, Capabilities: flags}
	return f, store, app.Onboarding{Enabled: true, Repository: store, NewID: jobs.NewUUIDv4}, app.CreationActor{AccountID: cookie.Row.AccountID, SessionID: cookie.Row.ID}, flags
}
func onboardingCommand(t *testing.T, operation string) d.Creation {
	t.Helper()
	v, err := d.NewCreation(d.CreationInput{OperationID: operation, Profile: d.Profile{Name: "Synthetic organization", TimeZone: "America/Cuiaba", Kinds: []d.Kind{d.Other}}, UnitName: "Main synthetic unit", Terms: d.TermsAcceptance{Version: d.SyntheticTermsVersion, Digest: d.SyntheticTermsDigest, Accepted: true}})
	if err != nil {
		t.Fatal(err)
	}
	return v
}
func TestAtomicOrganizationCreationReplayAndSetup(t *testing.T) {
	f, store, service, actor, flags := onboardingFixture(t)
	operation, err := jobs.NewUUIDv4()
	if err != nil {
		t.Fatal(err)
	}
	command := onboardingCommand(t, operation)
	result, replay, err := service.Create(t.Context(), actor, command)
	if err != nil || replay || result.State != d.PendingSetup || result.Revision != 1 {
		t.Fatal("creation failed", replay, err)
	}
	repeated, replay, err := service.Create(t.Context(), actor, command)
	if err != nil || !replay || repeated != result {
		t.Fatal("creation retry changed acknowledgement", replay, err)
	}
	var complete bool
	if err := f.Admin.QueryRow(t.Context(), `SELECT (SELECT count(*) FROM organizations WHERE org_id=$1 AND state='PENDING_SETUP')=1 AND (SELECT count(*) FROM org_units WHERE org_id=$1 AND state='ACTIVE')=1 AND (SELECT count(*) FROM org_memberships WHERE org_id=$1 AND state='ACTIVE' AND account_id=$2)=1 AND (SELECT count(*) FROM org_role_assignments WHERE org_id=$1 AND role='OWNER')=1 AND (SELECT count(*) FROM org_onboarding_commands WHERE org_id=$1 AND kind='CREATED')=1 AND (SELECT count(*) FROM org_audit_events WHERE org_id=$1)=1`, result.OrgID.String(), actor.AccountID).Scan(&complete); err != nil || !complete {
		t.Fatal("incomplete composition", err)
	}
	scope, err := kernel.NewOrgScope(result.OrgID)
	if err != nil {
		t.Fatal(err)
	}
	activation, err := jobs.NewUUIDv4()
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.ActivateOrganization(t.Context(), scope, actor, result.MembershipID, activation, 1); !errors.Is(err, cap.ErrEntitlementRequired) {
		t.Fatal("unprovisioned capability activated setup", err)
	}
	change, err := jobs.NewUUIDv4()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := flags.Set(t.Context(), scope, cap.Actor{AccountID: actor.AccountID}, cap.Change{OperationID: change, Capability: "organizations", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	active, replay, err := store.ActivateOrganization(t.Context(), scope, actor, result.MembershipID, activation, 1)
	if err != nil || replay || active.State != d.Active || active.Revision != 2 {
		t.Fatal("activation failed", replay, err)
	}
	again, replay, err := store.ActivateOrganization(t.Context(), scope, actor, result.MembershipID, activation, 1)
	if err != nil || !replay || again != active {
		t.Fatal("activation replay failed", replay, err)
	}
}

func onboardingID(t *testing.T) string {
	t.Helper()
	id, err := jobs.NewUUIDv4()
	if err != nil {
		t.Fatal(err)
	}
	return id
}
func assertOnboardingCount(t *testing.T, f *orgtest.Fixture, want int) {
	t.Helper()
	for _, table := range []string{"organizations", "org_units", "org_employee_identities", "org_memberships", "org_role_assignments", "org_onboarding_commands", "org_audit_events"} {
		predicate := ""
		if table == "organizations" {
			predicate = " WHERE has_snapshot"
		}
		var got int
		if err := f.Admin.QueryRow(t.Context(), "SELECT count(*) FROM "+pgx.Identifier{table}.Sanitize()+predicate).Scan(&got); err != nil || got != want {
			t.Fatalf("%s count: got %d want %d (%v)", table, got, want, err)
		}
	}
}
func TestOrganizationCreationSerializesPendingCeilingAndRetries(t *testing.T) {
	for _, same := range []bool{false, true} {
		name := "distinct operations"
		if same {
			name = "same operation"
		}
		t.Run(name, func(t *testing.T) {
			f, _, service, actor, _ := onboardingFixture(t)
			const n = 12
			commands := make([]d.Creation, n)
			operation := onboardingID(t)
			for i := range commands {
				if !same {
					operation = onboardingID(t)
				}
				commands[i] = onboardingCommand(t, operation)
			}
			type outcome struct {
				result app.CreationResult
				replay bool
				err    error
			}
			outcomes := make(chan outcome, n)
			start := make(chan struct{})
			var wg sync.WaitGroup
			for _, command := range commands {
				wg.Go(func() {
					<-start
					r, replay, err := service.Create(t.Context(), actor, command)
					outcomes <- outcome{r, replay, err}
				})
			}
			close(start)
			wg.Wait()
			close(outcomes)
			created, repeated, refused := 0, 0, 0
			var first app.CreationResult
			for got := range outcomes {
				if got.err != nil {
					if !errors.Is(got.err, kernel.ErrPendingOrgLimit) || got.replay || got.result != (app.CreationResult{}) {
						t.Fatal("unsafe failed creation", got.err)
					}
					refused++
					continue
				}
				if got.replay {
					repeated++
				} else {
					created++
				}
				if same {
					if first == (app.CreationResult{}) {
						first = got.result
					} else if first != got.result {
						t.Fatal("concurrent retry changed receipt")
					}
				}
			}
			if same {
				if created != 1 || repeated != 11 || refused != 0 {
					t.Fatal(created, repeated, refused)
				}
				assertOnboardingCount(t, f, 1)
			} else {
				if created != 3 || refused != 9 || repeated != 0 {
					t.Fatal(created, repeated, refused)
				}
				assertOnboardingCount(t, f, 3)
			}
		})
	}
}

type failingOnboardingMemberships struct {
	memberapp.TransactionRepository
}

func (f failingOnboardingMemberships) SaveTx(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, expected int64, value member.Membership) (bool, error) {
	if expected == 2 {
		return false, errors.New("synthetic private persistence detail")
	}
	return f.TransactionRepository.SaveTx(ctx, tx, scope, expected, value)
}
func TestOrganizationCreationRollsBackCompositionAndCommitFailures(t *testing.T) {
	for _, commit := range []bool{false, true} {
		name := "membership failure"
		if commit {
			name = "deferred commit failure"
		}
		t.Run(name, func(t *testing.T) {
			f, store, service, actor, _ := onboardingFixture(t)
			if commit {
				if _, err := f.Admin.Exec(t.Context(), `CREATE FUNCTION synthetic_onboarding_commit_failure() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'synthetic private commit detail'; END $$;
 CREATE CONSTRAINT TRIGGER synthetic_onboarding_commit_failure AFTER INSERT ON org_onboarding_commands DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION synthetic_onboarding_commit_failure()`); err != nil {
					t.Fatal(err)
				}
			} else {
				store.Memberships = failingOnboardingMemberships{store.Memberships}
			}
			got, replay, err := service.Create(t.Context(), actor, onboardingCommand(t, onboardingID(t)))
			if !errors.Is(err, app.ErrUnavailable) || replay || got != (app.CreationResult{}) {
				t.Fatal("failed transaction returned result or private detail", replay, err)
			}
			assertOnboardingCount(t, f, 0)
		})
	}
}
func TestOrganizationCreationRejectsChangedIntent(t *testing.T) {
	f, _, service, actor, _ := onboardingFixture(t)
	command := onboardingCommand(t, onboardingID(t))
	if _, _, err := service.Create(t.Context(), actor, command); err != nil {
		t.Fatal(err)
	}
	profile := command.Profile()
	profile.Name = "Changed synthetic intent"
	changed, err := d.NewCreation(d.CreationInput{OperationID: command.OperationID(), Profile: profile, UnitName: command.UnitName(), Terms: command.Terms()})
	if err != nil {
		t.Fatal(err)
	}
	got, replay, err := service.Create(t.Context(), actor, changed)
	if !errors.Is(err, app.ErrOperationReused) || replay || got != (app.CreationResult{}) {
		t.Fatal("changed intent adopted previous receipt", replay, err)
	}
	assertOnboardingCount(t, f, 1)
}
func TestOrganizationCreationRechecksCurrentPersonalProof(t *testing.T) {
	cases := []struct {
		name, sql string
		want      error
	}{
		{"revoked cookie", `UPDATE company_web_sessions SET revoked_at=clock_timestamp() WHERE account_id=$1`, app.ErrUnauthenticated},
		{"revoked origin family", `UPDATE account_session_families SET revoked_at=clock_timestamp() WHERE account_id=$1`, app.ErrUnauthenticated},
		{"stale authentication", `UPDATE company_web_sessions SET authenticated_at=clock_timestamp()-interval '17 minutes',created_at=clock_timestamp()-interval '16 minutes',expires_at=clock_timestamp()+interval '7 hours' WHERE account_id=$1`, app.ErrUnauthenticated},
		{"account suspended", `UPDATE accounts SET status='suspended' WHERE id=$1`, app.ErrUnauthenticated},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			f, _, service, actor, _ := onboardingFixture(t)
			if _, err := f.Admin.Exec(t.Context(), test.sql, actor.AccountID); err != nil {
				t.Fatal(err)
			}
			got, replay, err := service.Create(t.Context(), actor, onboardingCommand(t, onboardingID(t)))
			if !errors.Is(err, test.want) || replay || got != (app.CreationResult{}) {
				t.Fatal("invalid current proof admitted", replay, err)
			}
			assertOnboardingCount(t, f, 0)
		})
	}
}

func TestOrganizationOnboardingThroughComposedCompanyHTTP(t *testing.T) {
	f, _, _, _, flags := onboardingFixture(t)
	role := pgx.Identifier{f.RuntimeRole}.Sanitize()
	if _, err := f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT,UPDATE ON identity_rate_windows TO "+role); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	accounts := &account.Service{Clock: onboardingClock{now}, Store: accountpg.NewPGStore(f.Admin), Hasher: accountdomain.SHA256Hasher{}, IDGen: jobs.NewUUIDv4, TokenGen: accountdomain.GenerateToken, KeyGen: accountdomain.GenerateAccountKey}
	key, err := accounts.CreateKeyAccount(t.Context(), "synthetichttp")
	if err != nil {
		t.Fatal(err)
	}
	auth, err := accounts.LoginWithKey(t.Context(), key.Key)
	if err != nil {
		t.Fatal(err)
	}
	router := chi.NewRouter()
	const origin = "https://company.example.invalid"
	if err := enterprisesuite.Register(router, enterprisesuite.Options{Enabled: true, Origin: origin, CursorSecret: bytes.Repeat([]byte{9}, 32), Pool: f.Runtime, Accounts: accounts}); err != nil {
		t.Fatal(err)
	}
	var cookie *http.Cookie
	csrf := ""
	call := func(path string, body any, want int, edit func(*http.Request)) map[string]any {
		t.Helper()
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		request := httptest.NewRequest(http.MethodPost, origin+path, bytes.NewReader(raw))
		request.Header.Set("Origin", origin)
		request.Header.Set("Content-Type", "application/json")
		if cookie != nil {
			request.AddCookie(cookie)
			request.Header.Set("X-CSRF", csrf)
		}
		if edit != nil {
			edit(request)
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != want || response.Header().Get("Cache-Control") != "no-store" || response.Header().Get("ETag") != "" {
			t.Fatalf("%s got %d want %d: %s", path, response.Code, want, response.Body.String())
		}
		var result map[string]any
		if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if path == "/v1/org-access/session/login" && want == 200 {
			cookies := response.Result().Cookies()
			if len(cookies) != 1 {
				t.Fatal("missing dedicated cookie")
			}
			cookie = cookies[0]
			csrf = result["csrf_token"].(string)
		}
		return result
	}
	call("/v1/org-access/session/login", map[string]any{"family_id": auth.Session.FamilyID, "access_token": auth.Session.AccessToken}, 200, nil)
	operation := onboardingID(t)
	body := map[string]any{"schema_version": 1, "operation_id": operation, "name": "Synthetic HTTP company", "time_zone": "America/Cuiaba", "kinds": []string{"OTHER"}, "unit_name": "Main synthetic unit", "terms": map[string]any{"version": d.SyntheticTermsVersion, "digest": d.SyntheticTermsDigest, "accepted": true}}
	const create = "/v1/org-access/organizations/create"
	for _, test := range []struct {
		name   string
		edit   func(*http.Request)
		status int
	}{
		{"foreign origin", func(r *http.Request) { r.Header.Set("Origin", "https://foreign.example.invalid") }, 403},
		{"missing csrf", func(r *http.Request) { r.Header.Del("X-CSRF") }, 403},
		{"admin cookie", func(r *http.Request) { r.Header.Set("Cookie", "__Host-abv_admin=synthetic") }, 401},
		{"account bearer", func(r *http.Request) { r.Header.Set("Authorization", "Bearer synthetic") }, 401},
	} {
		t.Run(test.name, func(t *testing.T) { call(create, body, test.status, test.edit) })
	}
	created := call(create, body, 201, nil)
	replayed := call(create, body, 201, nil)
	if created["replayed"] != false || replayed["replayed"] != true {
		t.Fatal("HTTP replay marker")
	}
	for _, key := range []string{"organization_id", "unit_id", "employee_id", "membership_id", "assignment_id"} {
		if created[key] != replayed[key] {
			t.Fatal("HTTP replay changed identity", key)
		}
	}
	id, err := kernel.ParseOrgID(created["organization_id"].(string))
	if err != nil {
		t.Fatal(err)
	}
	scope, err := kernel.NewOrgScope(id)
	if err != nil {
		t.Fatal(err)
	}
	path := "/v1/orgs/" + id.String() + "/organization/activate"
	activate := map[string]any{"schema_version": 1, "operation_id": onboardingID(t), "revision": 1}
	call(path, activate, 403, nil)
	if _, err := flags.Set(t.Context(), scope, cap.Actor{AccountID: auth.Account.ID}, cap.Change{OperationID: onboardingID(t), Capability: "organizations", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	call(path, activate, 401, func(r *http.Request) { r.Header.Del("Cookie") })
	roots := adapters.NewStore(f.Runtime)
	pending, err := roots.Get(t.Context(), scope)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := pending.Snapshot(scope)
	if err != nil {
		t.Fatal(err)
	}
	profile := snapshot.Profile
	missingProfile := profile
	missingProfile.Name = ""
	incomplete, err := pending.ConfigurePending(scope, 1, missingProfile)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := roots.Save(t.Context(), scope, 1, incomplete); err != nil {
		t.Fatal(err)
	}
	activate["revision"] = 2
	checklist := call(path, activate, 409, nil)
	details := checklist["error"].(map[string]any)["details"].([]any)
	if len(details) != 1 || details[0].(map[string]any)["field"] != "name" || details[0].(map[string]any)["code"] != "REQUIRED" {
		t.Fatal("missing safe checklist details", details)
	}
	completed, err := incomplete.ConfigurePending(scope, 2, profile)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := roots.Save(t.Context(), scope, 2, completed); err != nil {
		t.Fatal(err)
	}
	activate["revision"] = 3
	active := call(path, activate, 200, nil)
	if active["state"] != "ACTIVE" || active["revision"] != float64(4) || active["data_mode"] != "SYNTHETIC" {
		t.Fatal("HTTP setup result incomplete")
	}
	if call(path, activate, 200, nil)["replayed"] != true {
		t.Fatal("HTTP activation replay")
	}
	// Changed intent is bounded, malformed body proof cannot select an account.
	body["name"] = "Changed HTTP intent"
	call(create, body, 409, nil)
	body["account_id"] = auth.Account.ID
	call(create, body, 400, nil)
	delete(body, "account_id")
	call("/v1/orgs/"+onboardingID(t)+"/organization/activate", activate, 404, nil)
	// Failure after normal cookie verification is caught again inside the write.
	if _, err := f.Admin.Exec(t.Context(), `UPDATE company_web_sessions SET revoked_at=clock_timestamp() WHERE account_id=$1`, auth.Account.ID); err != nil {
		t.Fatal(err)
	}
	body["operation_id"] = onboardingID(t)
	call(create, body, 401, nil)
	// Only source IDs, flags and bounded metadata occur in the private receipts.
	var receipts int
	if err := f.Admin.QueryRow(t.Context(), `SELECT count(*) FROM org_onboarding_commands WHERE org_id=$1 AND terms_kind='SYNTHETIC'`, id.String()).Scan(&receipts); err != nil || receipts != 1 {
		t.Fatal("missing synthetic receipt", err)
	}
	if strings.Contains(created["organization_id"].(string), auth.Account.ID) {
		t.Fatal("response confused account and organization")
	}
}

func TestManagedAccountWithRealFreshCookieCannotCreateOrOwn(t *testing.T) {
	f, _, service, _, _ := onboardingFixture(t)
	scope := scopeFor(t, onboardingID(t))
	root, err := d.NewPending(scope, d.Profile{Name: "Managed origin fixture", TimeZone: "UTC", Kinds: []d.Kind{d.Other}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adapters.NewStore(f.Runtime).Create(t.Context(), scope, root); err != nil {
		t.Fatal(err)
	}
	managed := onboardingID(t)

	tx, err := f.Admin.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(t.Context(), `SELECT set_config('anpfuel.org_id',$1,true)`, scope.OrgID().String()); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(t.Context(), `INSERT INTO accounts(id,alias,status,enterprise_managed_org_id) VALUES($1,'synthetic-managed-creator','active',$2)`, managed, scope.OrgID().String()); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	hasher := accountdomain.SHA256Hasher{}
	salt, err := hasher.NewSalt()
	if err != nil {
		t.Fatal(err)
	}
	family := onboardingID(t)
	token := "synthetic-managed-proof"
	accounts := &account.Service{Clock: onboardingClock{now}, Store: accountpg.NewPGStore(f.Admin), Hasher: hasher}
	if err := accounts.Store.CreateFamily(t.Context(), accountdomain.SessionFamily{ID: family, AccountID: managed, RefreshSalt: salt, RefreshHash: hasher.Hash(salt, "synthetic-refresh"), AccessSalt: salt, AccessHash: hasher.Hash(salt, token), AccessExpires: now.Add(15 * time.Minute).Unix(), IssuedAt: now.Unix()}); err != nil {
		t.Fatal(err)
	}
	cookies := &company.Service{Enabled: true, Store: &companypg.Store{Pool: f.Runtime}, Hasher: hasher, Prove: companypg.AccountProof(accounts), Now: func() time.Time { return now }, NewID: jobs.NewUUIDv4, NewSecret: accountdomain.GenerateToken}
	cookie, err := cookies.Login(t.Context(), family, token, "")
	if err != nil {
		t.Fatal(err)
	}
	// The normal service verifies a real account family and issues a live cookie;
	// the negative is the immutable managed provenance, not a stale fake actor.
	if _, err := cookies.Authenticate(t.Context(), cookie.Token, cookie.CSRF, true); err != nil {
		t.Fatal(err)
	}
	got, replay, err := service.Create(t.Context(), app.CreationActor{AccountID: managed, SessionID: cookie.Row.ID}, onboardingCommand(t, onboardingID(t)))
	if !errors.Is(err, kernel.ErrOwnerRequiresPersonal) || replay || got != (app.CreationResult{}) {
		t.Fatal("managed origin created company", err)
	}
	members := memberpg.NewStore(f.Runtime)
	employee, err := member.NewEmployee(scope, onboardingID(t))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := members.CreateEmployee(t.Context(), scope, employee); err != nil {
		t.Fatal(err)
	}
	invited, err := member.NewInvited(scope, onboardingID(t), employee, member.ManagedAccess)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := members.Create(t.Context(), scope, invited); err != nil {
		t.Fatal(err)
	}
	ref, err := member.ManagedAccount(scope, managed)
	if err != nil {
		t.Fatal(err)
	}
	bound, err := invited.BindAccount(scope, 1, ref)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := members.Save(t.Context(), scope, 1, bound); err != nil {
		t.Fatal(err)
	}
	snapshot, err := bound.Snapshot(scope)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := access.RehydrateAssignment(scope, access.AssignmentSnapshot{OrgID: scope.OrgID(), ID: onboardingID(t), MembershipID: snapshot.ID, AssignedBy: snapshot.ID, Role: access.Owner, Scope: access.OrgScope, TargetID: scope.OrgID().String(), ValidFrom: now, Reason: "Synthetic denied ownership", Revision: 1})
	if err != nil {
		t.Fatal(err)
	}
	if changed, err := accesspg.NewStore(f.Runtime).Create(t.Context(), scope, owner); !errors.Is(err, kernel.ErrOwnerRequiresPersonal) || changed {
		t.Fatal("managed account granted OWNER", changed, err)
	}
	var count int
	if err := f.Admin.QueryRow(t.Context(), `SELECT count(*) FROM org_onboarding_commands`).Scan(&count); err != nil || count != 0 {
		t.Fatal("managed refusal committed partial setup", err)
	}
}
func TestPendingOwnerGrantsReserveSlotsAcrossOrganizations(t *testing.T) {
	f, _, _, actor, _ := onboardingFixture(t)
	roles := accesspg.NewStore(f.Runtime)
	members := memberpg.NewStore(f.Runtime)
	const n = 4
	scopes := make([]kernel.OrgScope, n)
	assignments := make([]access.Assignment, n)
	for i := range n {
		scopes[i] = scopeFor(t, onboardingID(t))
		scope := scopes[i]
		root, err := d.NewPending(scope, d.Profile{})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := adapters.NewStore(f.Runtime).Create(t.Context(), scope, root); err != nil {
			t.Fatal(err)
		}
		employee, err := member.NewEmployee(scope, onboardingID(t))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := members.CreateEmployee(t.Context(), scope, employee); err != nil {
			t.Fatal(err)
		}
		membership, err := member.NewInvited(scope, onboardingID(t), employee, member.ExistingAccount)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := members.Create(t.Context(), scope, membership); err != nil {
			t.Fatal(err)
		}
		accountRef, err := member.PersonalAccount(actor.AccountID)
		if err != nil {
			t.Fatal(err)
		}
		bound, err := membership.BindAccount(scope, 1, accountRef)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := members.Save(t.Context(), scope, 1, bound); err != nil {
			t.Fatal(err)
		}
		snap, err := bound.Snapshot(scope)
		if err != nil {
			t.Fatal(err)
		}
		assignments[i], err = access.RehydrateAssignment(scope, access.AssignmentSnapshot{OrgID: scope.OrgID(), ID: onboardingID(t), MembershipID: snap.ID, AssignedBy: snap.ID, Role: access.Owner, Scope: access.OrgScope, TargetID: scope.OrgID().String(), ValidFrom: time.Now().Add(time.Hour), Reason: "Synthetic future ownership", Revision: 1})
		if err != nil {
			t.Fatal(err)
		}
	}
	type outcome struct {
		index   int
		changed bool
		err     error
	}
	results := make(chan outcome, n)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range n {
		wg.Go(func() {
			<-start
			changed, err := roles.Create(t.Context(), scopes[i], assignments[i])
			results <- outcome{i, changed, err}
		})
	}
	close(start)
	wg.Wait()
	close(results)
	accepted, refused := 0, 0
	winner, loser := -1, -1
	for got := range results {
		if got.err == nil && got.changed {
			accepted++
			winner = got.index
		} else if errors.Is(got.err, kernel.ErrPendingOrgLimit) && !got.changed {
			refused++
			loser = got.index
		} else {
			t.Fatal("owner grant result", got.changed, got.err)
		}
	}
	if accepted != 3 || refused != 1 {
		t.Fatal("owner grants bypassed pending cap", accepted, refused)
	}
	snapshot, err := assignments[winner].Snapshot(scopes[winner])
	if err != nil {
		t.Fatal(err)
	}
	if _, changed, err := roles.End(t.Context(), scopes[winner], snapshot.ID, 1); err != nil || !changed {
		t.Fatal("could not release future owner reservation", err)
	}
	if changed, err := roles.Create(t.Context(), scopes[loser], assignments[loser]); err != nil || !changed {
		t.Fatal("ended ownership still reserved slot", err)
	}
}

func TestSetupTransactionRechecksLiveFreshOriginProof(t *testing.T) {
	for _, test := range []struct{ name, sql string }{
		{"cookie revocation", `UPDATE company_web_sessions SET revoked_at=clock_timestamp() WHERE account_id=$1`},
		{"origin family revocation", `UPDATE account_session_families SET revoked_at=clock_timestamp() WHERE account_id=$1`},
		{"freshness elapsed", `UPDATE company_web_sessions SET authenticated_at=clock_timestamp()-interval '17 minutes',created_at=clock_timestamp()-interval '16 minutes',expires_at=clock_timestamp()+interval '7 hours' WHERE account_id=$1`},
	} {
		t.Run(test.name, func(t *testing.T) {
			f, store, service, actor, flags := onboardingFixture(t)
			created, _, err := service.Create(t.Context(), actor, onboardingCommand(t, onboardingID(t)))
			if err != nil {
				t.Fatal(err)
			}
			scope := scopeFor(t, created.OrgID.String())
			if _, err := flags.Set(t.Context(), scope, cap.Actor{AccountID: actor.AccountID}, cap.Change{OperationID: onboardingID(t), Capability: "organizations", Enabled: true}); err != nil {
				t.Fatal(err)
			}
			if _, err := f.Admin.Exec(t.Context(), test.sql, actor.AccountID); err != nil {
				t.Fatal(err)
			}
			result, replay, err := store.ActivateOrganization(t.Context(), scope, actor, created.MembershipID, onboardingID(t), 1)
			if !errors.Is(err, app.ErrUnauthenticated) || replay || result != (app.ActivationResult{}) {
				t.Fatal("revoked/stale proof reached setup mutation", err)
			}
			var untouched bool
			if err := f.Admin.QueryRow(t.Context(), `SELECT state='PENDING_SETUP' AND revision=1 AND NOT EXISTS(SELECT 1 FROM org_onboarding_commands WHERE org_id=$1 AND kind='ACTIVATED') FROM organizations WHERE org_id=$1`, scope.OrgID().String()).Scan(&untouched); err != nil || !untouched {
				t.Fatal("refused proof changed setup", err)
			}
		})
	}
}

func TestSetupConcurrentReplayAndFailedCommitReturnOnlyCommittedResult(t *testing.T) {
	for _, fail := range []bool{false, true} {
		name := "concurrent exact activation"
		if fail {
			name = "deferred activation failure"
		}
		t.Run(name, func(t *testing.T) {
			f, store, service, actor, flags := onboardingFixture(t)
			created, _, err := service.Create(t.Context(), actor, onboardingCommand(t, onboardingID(t)))
			if err != nil {
				t.Fatal(err)
			}
			scope := scopeFor(t, created.OrgID.String())
			if _, err := flags.Set(t.Context(), scope, cap.Actor{AccountID: actor.AccountID}, cap.Change{OperationID: onboardingID(t), Capability: "organizations", Enabled: true}); err != nil {
				t.Fatal(err)
			}
			if fail {
				if _, err := f.Admin.Exec(t.Context(), `CREATE FUNCTION synthetic_activation_failure() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'synthetic private activation failure'; END $$;
 CREATE CONSTRAINT TRIGGER synthetic_activation_failure AFTER INSERT ON org_onboarding_commands DEFERRABLE INITIALLY DEFERRED FOR EACH ROW WHEN(NEW.kind='ACTIVATED') EXECUTE FUNCTION synthetic_activation_failure()`); err != nil {
					t.Fatal(err)
				}
			}
			operation := onboardingID(t)
			type outcome struct {
				value  app.ActivationResult
				replay bool
				err    error
			}
			const n = 12
			results := make(chan outcome, n)
			start := make(chan struct{})
			var wg sync.WaitGroup
			for range n {
				wg.Go(func() {
					<-start
					value, replay, err := store.ActivateOrganization(t.Context(), scope, actor, created.MembershipID, operation, 1)
					results <- outcome{value, replay, err}
				})
			}
			close(start)
			wg.Wait()
			close(results)
			fresh, replayed, failed := 0, 0, 0
			var original app.ActivationResult
			for got := range results {
				if fail {
					if !errors.Is(got.err, app.ErrUnavailable) || got.replay || got.value != (app.ActivationResult{}) {
						t.Fatal("failed activation leaked provisional success", got.err)
					}
					failed++
					continue
				}
				if got.err != nil {
					t.Fatal("concurrent activation failed", got.err)
				}
				if got.replay {
					replayed++
				} else {
					fresh++
				}
				if original == (app.ActivationResult{}) {
					original = got.value
				} else if original != got.value {
					t.Fatal("activation retry changed acknowledgement")
				}
			}
			if fail {
				if failed != n {
					t.Fatal("missing failures", failed)
				}
			} else if fresh != 1 || replayed != n-1 {
				t.Fatal("multiple activation writes", fresh, replayed)
			}
			var revision int64
			var receipts int
			if err := f.Admin.QueryRow(t.Context(), `SELECT revision,(SELECT count(*) FROM org_onboarding_commands WHERE org_id=$1 AND kind='ACTIVATED') FROM organizations WHERE org_id=$1`, scope.OrgID().String()).Scan(&revision, &receipts); err != nil {
				t.Fatal(err)
			}
			if fail {
				if revision != 1 || receipts != 0 {
					t.Fatal("failed activation retained mutations", revision, receipts)
				}
			} else {
				if revision != 2 || receipts != 1 {
					t.Fatal("concurrent activation duplicated mutations", revision, receipts)
				}
				if _, _, err := store.ActivateOrganization(t.Context(), scope, actor, created.MembershipID, onboardingID(t), 1); !errors.Is(err, d.ErrConflict) {
					t.Fatal("stale distinct activation admitted", err)
				}
				if _, _, err := store.ActivateOrganization(t.Context(), scope, actor, created.MembershipID, operation, 2); !errors.Is(err, app.ErrOperationReused) {
					t.Fatal("changed activation intent adopted", err)
				}
			}
		})
	}
}
