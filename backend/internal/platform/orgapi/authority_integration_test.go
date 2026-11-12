//go:build integration

package orgapi_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	accountpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/adapters"
	accountapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/application"
	accountdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
	companypg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/adapters"
	companyweb "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/adapters/http"
	companyapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/application"
	identitypg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/adapters/auth"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	accesspg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/adapters"
	accessweb "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/adapters/http"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	accessdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	orgpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters"
	org "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type currentResource struct {
	pool  *pgxpool.Pool
	calls int
}

func (r *currentResource) Resolve(ctx context.Context, s kernel.OrgScope, _ access.Subject) (accessdomain.ResourceFacts, error) {
	r.calls++
	var id, author string
	err := database.WithOrgTx(ctx, r.pool, s, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT id::text,COALESCE(authored_by::text,'') FROM auth_test_resources WHERE org_id=$1 AND id=$2`, s.OrgID().String(), chi.URLParamFromCtx(ctx, "record_id")).Scan(&id, &author)
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return accessdomain.ResourceFacts{}, access.ErrNotFound
		}
		return accessdomain.ResourceFacts{}, err
	}
	return accessdomain.ResourceFacts{OrgID: s.OrgID(), ID: id, AuthoredBy: author}, nil
}

type currentCapabilities struct {
	denied bool
	calls  int
}

func (c *currentCapabilities) Check(context.Context, kernel.OrgScope, access.Subject, access.Policy) error {
	c.calls++
	if c.denied {
		return access.ErrEntitlementRequired
	}
	return nil
}

type currentRecords struct {
	denied bool
	calls  int
}

func (r *currentRecords) Check(context.Context, kernel.OrgScope, access.Subject, access.Policy, accessdomain.ResourceFacts, []accessdomain.Rule) error {
	r.calls++
	if r.denied {
		return access.ErrSelfApproval
	}
	return nil
}

func TestCurrentAuthorizationThroughRealAndroidAndCompanyProof(t *testing.T) {
	f := orgtest.New(t)
	f.GrantTeamReductions(t)
	f.GrantSettingsLifecycle(t)
	now := time.Now().UTC().Truncate(time.Second)
	role := pgx.Identifier{f.RuntimeRole}.Sanitize()
	if _, err := f.Admin.Exec(t.Context(), "GRANT USAGE ON SCHEMA public TO "+role+"; GRANT SELECT,INSERT,UPDATE ON organizations,org_employee_identities,org_memberships,org_role_assignments TO "+role+"; GRANT SELECT,UPDATE ON accounts,account_session_families TO "+role+"; GRANT SELECT ON payment_billing_exits TO "+role+"; GRANT SELECT,INSERT,UPDATE,DELETE ON company_web_sessions TO "+role); err != nil {
		t.Fatal(err)
	}
	scopeOf := func(raw string) kernel.OrgScope {
		t.Helper()
		id, err := kernel.ParseOrgID(raw)
		if err != nil {
			t.Fatal(err)
		}
		s, err := kernel.NewOrgScope(id)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	a, b := scopeOf("7b0e2a54-0000-4000-8000-000000000301"), scopeOf("7b0e2a54-0000-4000-8000-000000000302")
	for _, s := range []kernel.OrgScope{a, b} {
		root, err := org.NewPending(s, org.Profile{})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := orgpg.NewStore(f.Runtime).Create(t.Context(), s, root); err != nil {
			t.Fatal(err)
		}
	}
	accounts := accountpg.NewPGStore(f.Admin)
	hasher := accountdomain.SHA256Hasher{}
	for _, id := range []string{accountID, "7b0e2a54-0000-4000-8000-000000000099"} {
		keyHash := strings.Repeat("a", 64)
		if id != accountID {
			keyHash = strings.Repeat("b", 64)
		}
		if err := accounts.CreateAccount(t.Context(), accountdomain.Account{ID: id, Alias: "synthetic-" + id, Status: accountdomain.StatusActive, CreatedAt: now.Unix()}, keyHash); err != nil {
			t.Fatal(err)
		}
	}
	salt, err := hasher.NewSalt()
	if err != nil {
		t.Fatal(err)
	}
	if err := accounts.CreateFamily(t.Context(), accountdomain.SessionFamily{ID: familyID, AccountID: accountID, RefreshSalt: salt, RefreshHash: hasher.Hash(salt, "synthetic-refresh"), AccessSalt: salt, AccessHash: hasher.Hash(salt, "synthetic-access"), AccessExpires: now.Add(15 * time.Minute).Unix(), IssuedAt: now.Unix()}); err != nil {
		t.Fatal(err)
	}
	accountService := &accountapp.Service{Store: accounts, Hasher: hasher, Clock: liveClock{now: now.Unix()}}
	registrar := identitypg.NewRegistrar(f.Admin)
	signer := enrollSigner(t, registrar)
	if err := accounts.BindContributor(t.Context(), accountdomain.ContributorBinding{AccountID: accountID, ContributorID: signer.contributor, KeyFingerprint: signer.fingerprint, BoundAt: now.Unix()}, "7b0e2a54-0000-4000-8000-000000000098"); err != nil {
		t.Fatal(err)
	}
	seedOwner := func(member, employee, account, assignment string) {
		t.Helper()
		if err := database.WithOrgTx(t.Context(), f.Runtime, a, func(tx pgx.Tx) error {
			if _, err := tx.Exec(t.Context(), `INSERT INTO org_employee_identities(org_id,id,state) VALUES($1,$2,'ACTIVE')`, a.OrgID().String(), employee); err != nil {
				return err
			}
			if _, err := tx.Exec(t.Context(), `INSERT INTO org_memberships(org_id,id,employee_id,account_id,access_model,state) VALUES($1,$2,$3,$4,'EXISTING_ACCOUNT','INVITED')`, a.OrgID().String(), member, employee, account); err != nil {
				return err
			}
			if _, err := tx.Exec(t.Context(), `INSERT INTO org_role_assignments(org_id,id,membership_id,role,scope_type,scope_target,valid_from,assigned_by,reason) VALUES($1,$2,$3,'OWNER','ORG',$1,$4,$3,'synthetic authority')`, a.OrgID().String(), assignment, member, now.Add(-time.Hour)); err != nil {
				return err
			}
			_, err := tx.Exec(t.Context(), `UPDATE org_memberships SET state='ACTIVE',revision=revision+1 WHERE org_id=$1 AND id=$2`, a.OrgID().String(), member)
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	const ownerAssignment = "7b0e2a54-0000-4000-8000-000000000701"
	seedOwner(membershipID, "7b0e2a54-0000-4000-8000-000000000801", accountID, ownerAssignment)
	seedOwner("7b0e2a54-0000-4000-8000-000000000602", "7b0e2a54-0000-4000-8000-000000000802", "7b0e2a54-0000-4000-8000-000000000099", "7b0e2a54-0000-4000-8000-000000000702")
	const ownRecord = "7b0e2a54-0000-4000-8000-000000000901"
	const foreignRecord = "7b0e2a54-0000-4000-8000-000000000902"
	if _, err := f.Admin.Exec(t.Context(), `CREATE TABLE auth_test_resources(org_id UUID NOT NULL REFERENCES organizations(org_id),id UUID NOT NULL,authored_by UUID,PRIMARY KEY(org_id,id));ALTER TABLE auth_test_resources ENABLE ROW LEVEL SECURITY;ALTER TABLE auth_test_resources FORCE ROW LEVEL SECURITY;CREATE POLICY resource_scope ON auth_test_resources USING(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid)`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Admin.Exec(t.Context(), "GRANT SELECT ON auth_test_resources TO "+role); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Admin.Exec(t.Context(), `INSERT INTO auth_test_resources(org_id,id,authored_by) VALUES($1,$2,$5),($3,$4,NULL)`, a.OrgID().String(), ownRecord, b.OrgID().String(), foreignRecord, membershipID); err != nil {
		t.Fatal(err)
	}
	companyNow := now
	companyService := &companyapp.Service{Enabled: true, Store: &companypg.Store{Pool: f.Runtime}, Hasher: hasher, Prove: companypg.AccountProof(accountService), Now: func() time.Time { return companyNow }, NewID: jobs.NewUUIDv4, NewSecret: accountdomain.GenerateToken}
	cookie, err := companyService.Login(t.Context(), familyID, "synthetic-access", "")
	if err != nil {
		t.Fatal(err)
	}
	web, err := companyweb.New(companyService, origin)
	if err != nil {
		t.Fatal(err)
	}
	verifier := auth.Verifier{Pool: f.Admin, Authority: "api.example.invalid", Now: func() time.Time { return now }}
	sessions := accessweb.AccountSessions{Service: accountService}
	gateway := orgapi.Gateway{Enabled: true, Origin: origin, CursorSecret: []byte(strings.Repeat("s", 32)), Sessions: sessions.Validate, SessionConfirm: sessions.Confirm, WebSession: web.WebSession, WebSessionConfirm: web.WebSessionConfirm, SignedOwner: func(r *http.Request, account string) (string, string, error) {
		proof, err := verifier.Verify(r.Context(), r)
		if err != nil {
			return "", "", orgapi.Unauthenticated
		}
		owner, found, err := accounts.FindBindingOwner(r.Context(), proof.ContributorID)
		if err != nil || !found || owner != account {
			return "", "", orgapi.Unauthenticated
		}
		return proof.ContributorID, proof.KeyID, nil
	}}
	resources := &currentResource{pool: f.Runtime}
	capabilities := &currentCapabilities{}
	records := &currentRecords{}
	store := accesspg.NewStore(f.Runtime)
	service := access.Authorization{Source: store, Capabilities: capabilities, Records: records, Now: func() time.Time { return now }}
	router := chi.NewRouter()
	calls := 0
	var retained access.Decision
	var old context.Context
	if err := accessweb.Register(router, "/v1/orgs/{org_id}/synthetic/{record_id}/read", gateway, orgapi.Rule{Permission: "idn.employee.read", Fresh: true}, service, access.Policy{Permission: "idn.employee.read", Capability: "identity", Effect: access.Read, Fresh: true}, resources, func(ctx context.Context, p orgapi.Principal, decision access.Decision, _ orgapi.Session) (any, error) {
		calls++
		retained = decision
		old = ctx
		if decision.Subject().MembershipID() != membershipID || p.AccountID != accountID {
			t.Fatal("wrong subject")
		}
		if decision.Subject().ContributorID() != p.ContributorID || decision.Subject().DeviceKeyID() != p.DeviceKeyID {
			t.Fatal("verified device attribution lost")
		}
		return map[string]string{"record_id": decision.TargetID()}, nil
	}); err != nil {
		t.Fatal(err)
	}
	path := func(org, record string) string { return "/v1/orgs/" + org + "/synthetic/" + record + "/read" }
	dispatch := func(r *http.Request) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		return w
	}
	android := func(org, record string) *httptest.ResponseRecorder {
		return dispatch(signedRequest(t, registrar, signer, path(org, record), `{"family_id":"`+familyID+`","access_token":"synthetic-access"}`, "api.example.invalid", now))
	}
	browser := func(org, record string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", origin+path(org, record), strings.NewReader("{}"))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", origin)
		r.Header.Set(companyweb.CSRFHeader, cookie.CSRF)
		r.AddCookie(&http.Cookie{Name: companyweb.CookieName, Value: cookie.Token})
		return dispatch(r)
	}
	if w := android(a.OrgID().String(), ownRecord); w.Code != 200 || calls != 1 {
		t.Fatal("real signed request refused", w.Code, w.Body.String())
	}
	badProof := signedRequest(t, registrar, signer, path(a.OrgID().String(), ownRecord), `{"family_id":"`+familyID+`","access_token":"synthetic-access"}`, "api.example.invalid", now)
	badProof.Header.Set(auth.HeaderSignature, "invalid")
	beforeProof := resources.calls
	if w := dispatch(badProof); w.Code != 401 || resources.calls != beforeProof || calls != 1 {
		t.Fatal("invalid proof reached authority/resource dispatch", w.Code)
	}
	if err := retained.Check(old, a, accountID, "idn.employee.read", ownRecord); err != access.ErrInvalidContext {
		t.Fatal("request context remained usable", err)
	}
	if w := browser(a.OrgID().String(), ownRecord); w.Code != 200 || calls != 2 {
		t.Fatal("real company request refused", w.Code, w.Body.String())
	}
	before := resources.calls
	if w := android(b.OrgID().String(), foreignRecord); w.Code != 404 || resources.calls != before {
		t.Fatal("foreign organization inspected resource", w.Code)
	}
	foreign, unknown := android(a.OrgID().String(), foreignRecord), android(a.OrgID().String(), "7b0e2a54-0000-4000-8000-000000000999")
	if foreign.Code != 404 || unknown.Code != 404 || foreign.Body.String() != unknown.Body.String() {
		t.Fatal("foreign existence hint", foreign.Body.String(), unknown.Body.String())
	}
	capabilities.denied = true
	before = calls
	if w := android(a.OrgID().String(), ownRecord); w.Code != 403 || calls != before || !strings.Contains(w.Body.String(), "ENTITLEMENT_REQUIRED") {
		t.Fatal("capability bypass", w.Code)
	}
	capabilities.denied = false
	records.denied = true
	if w := browser(a.OrgID().String(), ownRecord); w.Code != 409 || calls != before {
		t.Fatal("record rule bypass", w.Code)
	}
	records.denied = false
	companyNow = now.Add(15 * time.Minute)
	before = capabilities.calls
	if w := browser(a.OrgID().String(), ownRecord); w.Code != 401 || capabilities.calls != before {
		t.Fatal("freshness gate order bypass", w.Code)
	}
	companyNow = now
	if _, changed, err := store.End(t.Context(), a, ownerAssignment, 1); err != nil || !changed {
		t.Fatal(changed, err)
	}
	before = calls
	if w := android(a.OrgID().String(), ownRecord); w.Code != 403 || calls != before || !strings.Contains(w.Body.String(), `"code":"idn.employee.read"`) {
		t.Fatal("revoked role authorized", w.Code, w.Body.String())
	}
	if w := browser(a.OrgID().String(), ownRecord); w.Code != 403 || calls != before {
		t.Fatal("cookie cached role", w.Code)
	}
	// A new SELF period is resolved immediately, without restoring OWNER. The
	// scope uses the resource author's stored membership, never a client flag.
	if err := database.WithOrgTx(t.Context(), f.Runtime, a, func(tx pgx.Tx) error {
		_, err := tx.Exec(t.Context(), `INSERT INTO org_role_assignments(org_id,id,membership_id,role,scope_type,scope_target,valid_from,assigned_by,reason) VALUES($1,$2,$3,'DRIVER','SELF',$3,$4,$3,'synthetic self role')`, a.OrgID().String(), "7b0e2a54-0000-4000-8000-000000000703", membershipID, now.Add(-time.Minute))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	tripCalls := 0
	if err := accessweb.Register(router, "/v1/orgs/{org_id}/synthetic/{record_id}/trip", gateway, orgapi.Rule{Permission: "trip.trip.read"}, service, access.Policy{Permission: "trip.trip.read", Capability: "trips", Effect: access.Read}, resources, func(context.Context, orgapi.Principal, access.Decision, orgapi.Session) (any, error) {
		tripCalls++
		return struct{}{}, nil
	}); err != nil {
		t.Fatal(err)
	}
	tripPath := "/v1/orgs/" + a.OrgID().String() + "/synthetic/" + ownRecord + "/trip"
	tripRequest := func() *http.Request {
		return signedRequest(t, registrar, signer, tripPath, `{"family_id":"`+familyID+`","access_token":"synthetic-access"}`, "api.example.invalid", now)
	}
	if w := dispatch(tripRequest()); w.Code != 200 || tripCalls != 1 {
		t.Fatal("current SELF role refused own record", w.Code, w.Body.String())
	}
	if _, err := f.Admin.Exec(t.Context(), `UPDATE auth_test_resources SET authored_by=$1 WHERE org_id=$2 AND id=$3`, "7b0e2a54-0000-4000-8000-000000000602", a.OrgID().String(), ownRecord); err != nil {
		t.Fatal(err)
	}
	if w := dispatch(tripRequest()); w.Code != 403 || tripCalls != 1 {
		t.Fatal("SELF widened to another member's record", w.Code)
	}
	if err := database.WithOrgTx(t.Context(), f.Runtime, a, func(tx pgx.Tx) error {
		_, err := tx.Exec(t.Context(), `UPDATE org_memberships SET state='ENDED',end_reason='OFFBOARDED',revision=revision+1 WHERE org_id=$1 AND id=$2`, a.OrgID().String(), membershipID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if w := android(a.OrgID().String(), ownRecord); w.Code != 404 {
		t.Fatal("offboarded access remained", w.Code)
	}
	if w := browser(a.OrgID().String(), ownRecord); w.Code != 404 {
		t.Fatal("offboarded cookie authorized", w.Code)
	}
}
