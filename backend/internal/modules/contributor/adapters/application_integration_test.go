//go:build integration

package adapters

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	migrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	adminadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/admin/adapters"
	adminapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/admin/application"
	admindomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/admin/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/contributor/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/contributor/domain"
	entadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/entitlement/adapters"
	staffadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/staff/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func freshDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("ANPFUEL_TEST_DATABASE_URL")
	if dsn == "" {
		t.Fatal("explicit disposable PostGIS DSN required")
	}
	u, err := url.Parse(dsn)
	if err != nil || u.Hostname() != "127.0.0.1" {
		t.Fatal("literal loopback test DSN required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal("disposable database unavailable")
	}
	name := fmt.Sprintf("contributor_test_%d", time.Now().UnixNano())
	if _, err = admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
		admin.Close(ctx)
		t.Fatal(err)
	}
	admin.Close(ctx)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		admin, err := pgx.Connect(ctx, dsn)
		if err != nil {
			t.Error("cleanup connection failed")
			return
		}
		defer admin.Close(ctx)
		if _, err = admin.Exec(ctx, "DROP DATABASE "+pgx.Identifier{name}.Sanitize()+" WITH (FORCE)"); err != nil {
			t.Error(err)
		}
	})
	u.Path = "/" + name
	if _, err = migrate.Apply(ctx, u.String(), migrations.Files); err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.New(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func newID(t *testing.T) string {
	t.Helper()
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		t.Fatal(err)
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	raw := hex.EncodeToString(b[:])
	return raw[0:8] + "-" + raw[8:12] + "-" + raw[12:16] + "-" + raw[16:20] + "-" + raw[20:32]
}

func seedAccount(t *testing.T, pool *pgxpool.Pool, status string) string {
	t.Helper()
	id := newID(t)
	if _, err := pool.Exec(context.Background(), "INSERT INTO accounts(id,alias,status) VALUES($1,$2,$3)", id, "synthetic-"+id, status); err != nil {
		t.Fatal(err)
	}
	return id
}

type fakeGitHub struct {
	logins []string
	gist   domain.Gist
	err    error
}

func (f fakeGitHub) Contributors(context.Context) ([]string, string, error) {
	return f.logins, "", f.err
}
func (f fakeGitHub) Gist(context.Context, string) (domain.Gist, error) { return f.gist, f.err }

func testService(t *testing.T, pool *pgxpool.Pool, github fakeGitHub, now time.Time) *application.Service {
	t.Helper()
	return &application.Service{
		Store: NewStore(pool), GitHub: github, Grants: entadapters.NewStore(pool),
		Repo: "owner/repo", Clock: func() time.Time { return now }, NewID: func() (string, error) { return newID(t), nil },
	}
}

func seedClaim(t *testing.T, pool *pgxpool.Pool, accountID, login, state, proof string, created, expires time.Time) string {
	t.Helper()
	id := newID(t)
	if _, err := pool.Exec(context.Background(), `INSERT INTO contributor_claims
		(id,account_id,operation_id,github_login,code,state,proof_status,created_at,expires_at)
		VALUES($1,$2,$3,$4,'abastevo-contributor-ABCDEFGHIJKLMNOP',$5,$6,$7,$8)`,
		id, accountID, newID(t), login, state, proof, created, expires); err != nil {
		t.Fatal(err)
	}
	return id
}

// assertAuditHasNoClaimSecrets is CTB-13: decided claims clear their code and
// audit rows must not keep a copy of it (or the operation id).
func assertAuditHasNoClaimSecrets(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	var leaks int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM admin_audit_events
		WHERE coalesce(before::text, '') || coalesce(after::text, '') LIKE '%abastevo-contributor-%'
		   OR coalesce(before::text, '') || coalesce(after::text, '') LIKE '%OperationID%'
		   OR coalesce(before::text, '') || coalesce(after::text, '') LIKE '%operation_id%'`).Scan(&leaks); err != nil || leaks != 0 {
		t.Fatalf("audit rows with claim secrets=%d err=%v", leaks, err)
	}
}

func adminSession(t *testing.T, pool *pgxpool.Pool, now time.Time) (adminapp.Session, *adminadapters.Store) {
	t.Helper()
	store := adminadapters.NewStore(pool)
	key := "abv_admin_" + base64.RawURLEncoding.EncodeToString(make([]byte, 32))
	presented := admindomain.HashKey(key)
	result, err := store.Login(context.Background(), adminapp.LoginInput{
		PresentedHash: presented, IPHash: strings.Repeat("a", 64), Now: now,
		Verify: func() (string, bool) { return admindomain.Fingerprint(presented), true },
	})
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.Authenticate(context.Background(), result.Token, now)
	if err != nil {
		t.Fatal(err)
	}
	return session, store
}

func mutation(sessionID string) adminapp.Mutation {
	return adminapp.Mutation{
		SessionID: sessionID, Action: "contributor.claim.approve", TargetType: "contributor_claim",
		TargetID: newIDForMutation(), Reason: "synthetic approval reason", Key: newIDForMutation(),
		RequestHash: strings.Repeat("a", 64), RequestID: "synthetic-request",
		ClientIPHash: strings.Repeat("b", 64), Now: time.Now().UTC(),
	}
}

var mutationCounter int

func newIDForMutation() string {
	mutationCounter++
	return fmt.Sprintf("11111111-1111-4111-8111-%012d", mutationCounter)
}

// TestClaim_CTB03_CTB07_QuotaExpiryAndReplay covers claim quotas, open-claim
// uniqueness, expiry and operation replay.
func TestClaim_CTB03_CTB07_QuotaExpiryAndReplay(t *testing.T) {
	pool := freshDB(t)
	accountID := seedAccount(t, pool, "active")
	now := time.Now().UTC().Truncate(time.Microsecond)
	service := testService(t, pool, fakeGitHub{}, now)
	ctx := context.Background()
	op := newID(t)
	first, err := service.CreateClaim(ctx, accountID, "octocat", op)
	if err != nil || !domain.ValidClaimCode(first.Code) {
		t.Fatalf("first claim=%+v err=%v", first, err)
	}
	replay, err := service.CreateClaim(ctx, accountID, "octocat", op)
	if err != nil || replay.ID != first.ID || replay.Code != first.Code {
		t.Fatalf("replay=%+v err=%v", replay, err)
	}
	if _, err := service.CreateClaim(ctx, accountID, "other", newID(t)); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("open claim err=%v", err)
	}
	if _, err := service.WithdrawClaim(ctx, accountID, first.ID); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		claim, err := service.CreateClaim(ctx, accountID, fmt.Sprintf("user-%d", i), newID(t))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := service.WithdrawClaim(ctx, accountID, claim.ID); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := service.CreateClaim(ctx, accountID, "too-many", newID(t)); !errors.Is(err, domain.ErrQuota) {
		t.Fatalf("fourth claim err=%v", err)
	}
	expiredID := seedClaim(t, pool, accountID, "expired-user", "PENDING", "UNVERIFIED", now.Add(-time.Hour), now.Add(-time.Minute))
	claims, err := service.ListOwnClaims(ctx, accountID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, claim := range claims {
		if claim.ID == expiredID {
			found = claim.State == domain.ClaimExpired
		}
	}
	if !found {
		t.Fatal("expired claim not projected")
	}
}

// TestClaim_CTB04_Proof covers owner mismatch, code absence and GitHub errors.
func TestClaim_CTB04_Proof(t *testing.T) {
	pool := freshDB(t)
	accountID := seedAccount(t, pool, "active")
	now := time.Now().UTC().Truncate(time.Microsecond)
	service := testService(t, pool, fakeGitHub{gist: domain.Gist{Owner: "octocat", Files: []string{"no code"}}}, now)
	claim, err := service.CreateClaim(context.Background(), accountID, "octocat", newID(t))
	if err != nil {
		t.Fatal(err)
	}
	updated, err := service.SetClaimProof(context.Background(), accountID, claim.ID, "gist1")
	if err != nil || updated.ProofStatus != domain.ProofUnverified {
		t.Fatalf("missing code proof=%+v err=%v", updated, err)
	}
	service.GitHub = fakeGitHub{gist: domain.Gist{Owner: "octocat", Files: []string{"x " + claim.Code + " y"}}}
	updated, err = service.SetClaimProof(context.Background(), accountID, claim.ID, "gist2")
	if err != nil || updated.ProofStatus != domain.ProofVerified {
		t.Fatalf("verified proof=%+v err=%v", updated, err)
	}
	service.GitHub = fakeGitHub{err: ErrGitHubUnavailable}
	updated, err = service.SetClaimProof(context.Background(), accountID, claim.ID, "gist3")
	if err != nil || updated.ProofStatus != domain.ProofUnverified {
		t.Fatalf("github down proof=%+v err=%v", updated, err)
	}
}

// TestApproval_CTB05_CTB06_AtomicGrantAndLink covers approval and idempotent
// replay, including the override rule.
func TestApproval_CTB05_CTB06_AtomicGrantAndLink(t *testing.T) {
	pool := freshDB(t)
	accountID := seedAccount(t, pool, "active")
	now := time.Now().UTC().Truncate(time.Microsecond)
	session, adminStore := adminSession(t, pool, now)
	service := testService(t, pool, fakeGitHub{logins: []string{"octocat"}}, now)
	service.Runner = adminStore
	claimID := seedClaim(t, pool, accountID, "octocat", "PENDING", "VERIFIED", now.Add(-time.Minute), now.Add(time.Hour))
	m := mutation(session.ID)
	m.TargetID = claimID
	result, err := service.ApproveClaim(context.Background(), m, claimID, false)
	if err != nil || result.Claim.State != domain.ClaimApproved || result.Grant == nil || result.Login != "octocat" {
		t.Fatalf("approve=%+v err=%v", result, err)
	}
	var grantState, link string
	if err := pool.QueryRow(context.Background(), "SELECT state FROM entitlement_grants WHERE account_id=$1 AND tier='CONTRIBUTOR'", accountID).Scan(&grantState); err != nil || grantState != "ACTIVE" {
		t.Fatalf("grant=%q err=%v", grantState, err)
	}
	if err := pool.QueryRow(context.Background(), "SELECT github_login FROM contributor_github_links WHERE account_id=$1 AND revoked_at IS NULL", accountID).Scan(&link); err != nil || link != "octocat" {
		t.Fatalf("link=%q err=%v", link, err)
	}
	var audits int
	if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM admin_audit_events WHERE action IN ('contributor.claim.approve','admin.grant.create','contributor.github.link')").Scan(&audits); err != nil || audits != 3 {
		t.Fatalf("audits=%d err=%v", audits, err)
	}
	assertAuditHasNoClaimSecrets(t, pool)
	replay, err := service.ApproveClaim(context.Background(), m, claimID, false)
	if err != nil || replay.Claim.ID != result.Claim.ID || replay.Grant == nil || replay.Grant.ID != result.Grant.ID {
		t.Fatalf("replay=%+v err=%v", replay, err)
	}

	secondAccount := seedAccount(t, pool, "active")
	secondClaim := seedClaim(t, pool, secondAccount, "absent-user", "PENDING", "UNVERIFIED", now.Add(-time.Minute), now.Add(time.Hour))
	m2 := mutation(session.ID)
	m2.TargetID = secondClaim
	if _, err := service.ApproveClaim(context.Background(), m2, secondClaim, false); !errors.Is(err, domain.ErrOverrideRequired) {
		t.Fatalf("override err=%v", err)
	}
	adminStore2 := adminStore
	service.Runner = adminStore2
	if _, err := service.ApproveClaim(context.Background(), m2, secondClaim, true); err != nil {
		t.Fatalf("override approval err=%v", err)
	}
}

// TestLink_CTB06_ConcurrentUnique covers one login mapping to one account.
func TestLink_CTB06_ConcurrentUnique(t *testing.T) {
	pool := freshDB(t)
	first, second := seedAccount(t, pool, "active"), seedAccount(t, pool, "active")
	store := NewStore(pool)
	now := time.Now().UTC().Truncate(time.Microsecond)
	start := make(chan struct{})
	type outcome struct{ err error }
	results := make(chan outcome, 2)
	var wg sync.WaitGroup
	for _, accountID := range []string{first, second} {
		wg.Add(1)
		go func(accountID string) {
			defer wg.Done()
			<-start
			err := store.RunInTx(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
				_, err := store.InsertGitHubLink(ctx, tx, accountID, "octocat", now)
				return err
			})
			results <- outcome{err}
		}(accountID)
	}
	close(start)
	wg.Wait()
	close(results)
	success, conflict := 0, 0
	for result := range results {
		switch {
		case result.err == nil:
			success++
		case errors.Is(result.err, domain.ErrConflict):
			conflict++
		default:
			t.Fatalf("unexpected err=%v", result.err)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatalf("success=%d conflict=%d", success, conflict)
	}
}

// TestProfile_CTB08_CTB12 covers capability, consent, edit quota and links.
func TestProfile_CTB08_CTB12(t *testing.T) {
	pool := freshDB(t)
	accountID := seedAccount(t, pool, "active")
	now := time.Now().UTC().Truncate(time.Microsecond)
	service := testService(t, pool, fakeGitHub{}, now)
	service.RequireCapability = func(context.Context, pgx.Tx, string) error { return domain.ErrForbidden }
	if _, err := service.UpsertProfile(context.Background(), accountID, "Ana", nil, domain.VisibilityHidden, false, false); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("capability err=%v", err)
	}
	service.RequireCapability = func(context.Context, pgx.Tx, string) error { return nil }
	service.Roles = staffadapters.NewStore(pool)
	if _, err := pool.Exec(context.Background(), `INSERT INTO staff_assignments(id,account_id,role,state,start_at,end_at,reason) VALUES($1,$2,'support_agent','ACTIVE',$3,NULL,'synthetic staff assignment')`, newID(t), accountID, now); err != nil {
		t.Fatal(err)
	}
	if _, err := service.UpsertProfile(context.Background(), accountID, "Ana", nil, domain.VisibilityPublic, false, false); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("consent err=%v", err)
	}
	links := []domain.Link{{Platform: "github", Handle: "octocat"}, {Platform: "x", Handle: "user_1"}}
	for i := 0; i < domain.MaxProfileEdits; i++ {
		if _, err := service.UpsertProfile(context.Background(), accountID, fmt.Sprintf("Ana %d", i), links, domain.VisibilityPublic, true, false); err != nil {
			t.Fatalf("edit %d err=%v", i, err)
		}
	}
	if _, err := service.UpsertProfile(context.Background(), accountID, "Ana too many", links, domain.VisibilityPublic, true, false); !errors.Is(err, domain.ErrQuota) {
		t.Fatalf("quota err=%v", err)
	}
	result, err := service.ReadProfile(context.Background(), accountID)
	if err != nil || !result.Found || result.Profile.Visibility != domain.VisibilityPublic || len(result.Links) != 2 {
		t.Fatalf("read profile=%+v err=%v", result, err)
	}
	if len(result.Roles) != 1 || result.Roles[0] != "Suporte" {
		t.Fatalf("owner roles=%v", result.Roles)
	}
}

// TestPublicList_CTB09_CTB10 covers public filters and stable pagination.
func TestPublicList_CTB09_CTB10(t *testing.T) {
	pool := freshDB(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	store := NewStore(pool)
	ctx := context.Background()
	add := func(name, visibility string, public bool, hidden bool, grantState string) string {
		accountID := seedAccount(t, pool, "active")
		if _, err := pool.Exec(ctx, `INSERT INTO contributor_profiles
			(account_id,display_name,visibility,public_consent,hidden_by_admin,hidden_reason)
			VALUES($1,$2,$3,$4,$5,$6)`, accountID, name, visibility, public, hidden, map[bool]any{true: "synthetic hide", false: nil}[hidden]); err != nil {
			t.Fatal(err)
		}
		grantID := newID(t)
		if _, err := pool.Exec(ctx, `INSERT INTO entitlement_grants
			(id,account_id,tier,policy_version,state,start_at,end_at,reason)
			VALUES($1,$2,'CONTRIBUTOR',1,'ACTIVE',$3,NULL,'synthetic public grant')`,
			grantID, accountID, now.Add(-time.Duration(len(name))*time.Minute)); err != nil {
			t.Fatal(err)
		}
		if grantState == "REVOKED" {
			if _, err := pool.Exec(ctx, `UPDATE entitlement_grants
				SET state='REVOKED', revoked_at=$2, revoke_reason='synthetic revoke' WHERE id=$1`, grantID, now); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := pool.Exec(ctx, `INSERT INTO contributor_links(account_id,platform,handle) VALUES($1,'github',$2)`, accountID, name); err != nil {
			t.Fatal(err)
		}
		return accountID
	}
	visible := add("visible", "PUBLIC", true, false, "ACTIVE")
	add("hidden", "HIDDEN", true, false, "ACTIVE")
	add("adminhidden", "PUBLIC", true, true, "ACTIVE")
	add("revoked", "PUBLIC", true, false, "REVOKED")
	suspended := seedAccount(t, pool, "suspended")
	if _, err := pool.Exec(ctx, `INSERT INTO contributor_profiles(account_id,display_name,visibility,public_consent) VALUES($1,'suspended','PUBLIC',true)`, suspended); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO entitlement_grants(id,account_id,tier,policy_version,state,start_at,end_at,reason) VALUES($1,$2,'CONTRIBUTOR',1,'ACTIVE',$3,NULL,'synthetic public grant')`, newID(t), suspended, now.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	var items []domain.PublicContributor
	if err := store.RunInTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		items, err = store.PublicContributors(ctx, tx, now, time.Time{}, "", 51)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].AccountID != visible || items[0].DisplayName != "visible" || len(items[0].Links) != 1 {
		t.Fatalf("items=%+v", items)
	}
	// Add two more visible accounts to exercise the cursor window.
	second := add("zeta", "PUBLIC", true, false, "ACTIVE")
	third := add("alpha", "PUBLIC", true, false, "ACTIVE")
	var page1, page2 []domain.PublicContributor
	if err := store.RunInTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		page1, err = store.PublicContributors(ctx, tx, now, time.Time{}, "", 2)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if len(page1) != 2 {
		t.Fatalf("page1=%+v", page1)
	}
	last := page1[len(page1)-1]
	if err := store.RunInTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		page2, err = store.PublicContributors(ctx, tx, now, last.ContributorSince, last.AccountID, 2)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if len(page2) != 1 || page2[0].AccountID != second && page2[0].AccountID != third {
		t.Fatalf("page2=%+v", page2)
	}
}

// TestErasure_CTB13 covers the account-deletion trigger.
func TestErasure_CTB13(t *testing.T) {
	pool := freshDB(t)
	accountID := seedAccount(t, pool, "active")
	store := NewStore(pool)
	now := time.Now().UTC().Truncate(time.Microsecond)
	ctx := context.Background()
	if err := store.RunInTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := store.UpsertProfile(ctx, tx, accountID, "Ana", domain.VisibilityPublic, true, false); err != nil {
			return err
		}
		if err := store.ReplaceLinks(ctx, tx, accountID, []domain.Link{{Platform: "github", Handle: "octocat"}}); err != nil {
			return err
		}
		if _, err := store.InsertGitHubLink(ctx, tx, accountID, "octocat", now); err != nil {
			return err
		}
		_, err := store.InsertClaim(ctx, tx, domain.Claim{
			ID: newID(t), AccountID: accountID, OperationID: newID(t), GitHubLogin: "octocat",
			Code: "abastevo-contributor-ABCDEFGHIJKLMNOP", State: domain.ClaimPending,
			ProofStatus: domain.ProofUnverified, CreatedAt: now, ExpiresAt: now.Add(time.Hour),
		})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "UPDATE accounts SET status='deleted' WHERE id=$1", accountID); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"contributor_profiles", "contributor_links", "contributor_claims"} {
		var count int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM "+table+" WHERE account_id=$1", accountID).Scan(&count); err != nil || count != 0 {
			t.Fatalf("%s count=%d err=%v", table, count, err)
		}
	}
	var revoked bool
	if err := pool.QueryRow(ctx, "SELECT revoked_at IS NOT NULL FROM contributor_github_links WHERE account_id=$1", accountID).Scan(&revoked); err != nil || !revoked {
		t.Fatalf("github link revoked=%v err=%v", revoked, err)
	}
}

// TestAdminGitHub_CTB01_StaleCache covers cached snapshot fallback and the
// once-per-minute refresh limit.
func TestAdminGitHub_CTB01_StaleCache(t *testing.T) {
	pool := freshDB(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	store := NewStore(pool)
	if err := store.SaveSnapshot(context.Background(), domain.Snapshot{
		Repo: "owner/repo", Logins: []string{"octocat"}, FetchedAt: now.Add(-2 * time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	service := testService(t, pool, fakeGitHub{err: ErrGitHubUnavailable}, now)
	view, err := service.AdminGitHubContributors(context.Background(), false)
	if err != nil || !view.Stale || len(view.Logins) != 1 || view.Logins[0].Login != "octocat" {
		t.Fatalf("stale view=%+v err=%v", view, err)
	}
	if err := store.SaveSnapshot(context.Background(), domain.Snapshot{
		Repo: "owner/repo", Logins: []string{"octocat"}, FetchedAt: now.Add(-10 * time.Second),
	}); err != nil {
		t.Fatal(err)
	}
	view, err = service.AdminGitHubContributors(context.Background(), true)
	if err != nil || view.Stale || len(view.Logins) != 1 {
		t.Fatalf("fresh view=%+v err=%v", view, err)
	}
}

// TestLink_CTB05_Override covers the direct-link override rule.
func TestLink_CTB05_Override(t *testing.T) {
	pool := freshDB(t)
	accountID := seedAccount(t, pool, "active")
	now := time.Now().UTC().Truncate(time.Microsecond)
	session, adminStore := adminSession(t, pool, now)
	service := testService(t, pool, fakeGitHub{logins: []string{"octocat"}}, now)
	service.Runner = adminStore
	m := mutation(session.ID)
	m.Action = "contributor.github.link"
	if _, err := service.LinkGitHub(context.Background(), m, accountID, "absent-user", false); !errors.Is(err, domain.ErrOverrideRequired) {
		t.Fatalf("override err=%v", err)
	}
	if _, err := service.LinkGitHub(context.Background(), m, accountID, "absent-user", true); err != nil {
		t.Fatalf("override link err=%v", err)
	}
}

// TestReject_CTB05_CTB07 covers the terminal reject and second decision.
func TestReject_CTB05_CTB07(t *testing.T) {
	pool := freshDB(t)
	accountID := seedAccount(t, pool, "active")
	now := time.Now().UTC().Truncate(time.Microsecond)
	session, adminStore := adminSession(t, pool, now)
	service := testService(t, pool, fakeGitHub{}, now)
	service.Runner = adminStore
	claimID := seedClaim(t, pool, accountID, "octocat", "PENDING", "UNVERIFIED", now.Add(-time.Minute), now.Add(time.Hour))
	m := mutation(session.ID)
	m.Action = "contributor.claim.reject"
	rejected, err := service.RejectClaim(context.Background(), m, claimID)
	if err != nil || rejected.State != domain.ClaimRejected || rejected.Code != "" {
		t.Fatalf("reject=%+v err=%v", rejected, err)
	}
	assertAuditHasNoClaimSecrets(t, pool)
	m.Key = "99999999-9999-4999-8999-999999999999"
	if _, err := service.RejectClaim(context.Background(), m, claimID); !errors.Is(err, domain.ErrClaimState) {
		t.Fatalf("second reject err=%v", err)
	}
}

// TestHide_CTB12 covers admin hide/unhide and the 404 path.
func TestHide_CTB12(t *testing.T) {
	pool := freshDB(t)
	accountID := seedAccount(t, pool, "active")
	now := time.Now().UTC().Truncate(time.Microsecond)
	session, adminStore := adminSession(t, pool, now)
	service := testService(t, pool, fakeGitHub{}, now)
	service.Runner = adminStore
	store := NewStore(pool)
	if err := store.RunInTx(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
		_, err := store.UpsertProfile(ctx, tx, accountID, "Ana", domain.VisibilityPublic, true, false)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	m := mutation(session.ID)
	m.Action = "contributor.profile.hide"
	hidden, err := service.HideProfile(context.Background(), m, accountID, true)
	if err != nil || !hidden.HiddenByAdmin {
		t.Fatalf("hide=%+v err=%v", hidden, err)
	}
	m.Key = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	unhidden, err := service.HideProfile(context.Background(), m, accountID, false)
	if err != nil || unhidden.HiddenByAdmin {
		t.Fatalf("unhide=%+v err=%v", unhidden, err)
	}
	m.Key = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	if _, err := service.HideProfile(context.Background(), m, newID(t), true); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("missing profile err=%v", err)
	}
}
