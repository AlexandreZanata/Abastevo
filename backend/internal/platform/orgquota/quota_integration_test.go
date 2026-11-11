//go:build integration

package orgquota

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	identitypg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/adapters"
	identity "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func fixture(t *testing.T, org, owner, device int32) (*orgtest.Fixture, *Guard, []Definition) {
	t.Helper()
	f := orgtest.New(t)
	if _, err := f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT,UPDATE ON identity_rate_windows TO "+pgx.Identifier{f.RuntimeRole}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	defs := definitions()
	defs[0].Organization = identity.OperationQuota{Limit: org, Window: 24 * time.Hour}
	defs[0].Account = identity.OperationQuota{Limit: owner, Window: 24 * time.Hour}
	defs[0].Device = identity.OperationQuota{Limit: device, Window: 24 * time.Hour}
	g, err := New(f.RuntimePool(t, 16), bytes.Repeat([]byte{7}, 32), defs)
	if err != nil {
		t.Fatal(err)
	}
	return f, g, defs
}

func total(t *testing.T, pool *pgxpool.Pool, want int) {
	t.Helper()
	var n int
	if err := pool.QueryRow(t.Context(), "SELECT COALESCE(sum(count),0)::integer FROM identity_rate_windows").Scan(&n); err != nil || n != want {
		t.Fatal("quota count", n, want, err)
	}
}
func check(t *testing.T, g *Guard, p orgapi.Principal, limited bool) {
	t.Helper()
	err := g.Check(t.Context(), p, writeRule)
	if !limited {
		if err != nil {
			t.Fatal(err)
		}
		return
	}
	var limit *orgapi.RateLimitError
	if !errors.As(err, &limit) || limit.RetryAfter <= 0 || limit.RetryAfter > 24*time.Hour {
		t.Fatal("missing bounded rate failure", err)
	}
}

func TestAtomicAxesDeviceRotationAndOrganizationIsolation(t *testing.T) {
	f, g, _ := fixture(t, 3, 2, 1)
	p := principal(t, orgA)
	check(t, g, p, false)
	total(t, f.Admin, 3)
	check(t, g, p, true)
	total(t, f.Admin, 3) // Device denial rolls back org/account.
	p.DeviceKeyID = orgB
	check(t, g, p, false)
	total(t, f.Admin, 6)
	p.DeviceKeyID = account
	check(t, g, p, true)
	total(t, f.Admin, 6) // Account cap survives key rotation.
	p.AccountID = orgB
	check(t, g, p, false)
	total(t, f.Admin, 9)
	p.AccountID = device
	p.DeviceKeyID = device
	check(t, g, p, true)
	total(t, f.Admin, 9) // Org cap survives account/device rotation.
	check(t, g, principal(t, orgB), false)
	total(t, f.Admin, 12)
	var visible int
	if err := f.Admin.QueryRow(t.Context(), "SELECT count(*) FROM identity_rate_windows WHERE subject_digest LIKE '%7b0e2a54%' OR operation<>'write'").Scan(&visible); err != nil || visible != 0 {
		t.Fatal("private IDs/new operations persisted", err)
	}
}

func TestConcurrentAdmissionNeverExceedsTheOrganizationBudget(t *testing.T) {
	f, g, _ := fixture(t, 3, 30, 30)
	var group sync.WaitGroup
	var admitted, denied atomic.Int32
	start := make(chan struct{})
	for i := range 30 {
		group.Go(func() {
			<-start
			p := principal(t, orgA)
			p.AccountID = fmt.Sprintf("7b0e2a54-0000-4000-8000-%012d", i+100)
			p.DeviceKeyID = p.AccountID
			err := g.Check(t.Context(), p, writeRule)
			var limit *orgapi.RateLimitError
			switch {
			case err == nil:
				admitted.Add(1)
			case errors.As(err, &limit):
				denied.Add(1)
			default:
				t.Error(err)
			}
		})
	}
	close(start)
	group.Wait()
	if admitted.Load() != 3 || denied.Load() != 27 {
		t.Fatal(admitted.Load(), denied.Load())
	}
	total(t, f.Admin, 9)
}

func TestClassesAndWebDeviceFallbackAreIndependent(t *testing.T) {
	f, g, defs := fixture(t, 5, 5, 1)
	other := defs[0]
	other.Class = "synthetic.other"
	other.Permissions = []string{"org.other.manage"}
	defs = append(defs, other)
	var err error
	g, err = New(g.pool, g.key, defs)
	if err != nil {
		t.Fatal(err)
	}
	p := principal(t, orgA)
	p.Channel = "web"
	p.ContributorID = ""
	p.DeviceKeyID = ""
	check(t, g, p, false)
	check(t, g, p, true)
	if err := g.Check(t.Context(), p, orgapi.Rule{Permission: "org.other.manage", Write: true}); err != nil {
		t.Fatal("class shared another class's counters", err)
	}
	total(t, f.Admin, 6)
}

func TestRequiredAxisAndCommitFailuresRollbackThenRecover(t *testing.T) {
	for _, kind := range []string{"account", "device", "commit", "permission", "unsafe role", "cancelled"} {
		t.Run(kind, func(t *testing.T) {
			f, g, _ := fixture(t, 5, 5, 5)
			p := principal(t, orgA)
			role := pgx.Identifier{f.RuntimeRole}.Sanitize()
			cleanup := ""
			switch kind {
			case "account", "device":
				i := 1
				if kind == "device" {
					i = 2
				}
				subjects := g.subjects("synthetic.command", p)
				if _, err := f.Admin.Exec(t.Context(), "CREATE TABLE synthetic_quota_failure(subject text)"); err != nil {
					t.Fatal(err)
				}
				if _, err := f.Admin.Exec(t.Context(), "INSERT INTO synthetic_quota_failure VALUES($1)", subjects[i]); err != nil {
					t.Fatal(err)
				}
				if _, err := f.Admin.Exec(t.Context(), "GRANT SELECT ON synthetic_quota_failure TO "+role+`;
					CREATE FUNCTION synthetic_quota_failure() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
					IF NEW.subject_digest IN (SELECT subject FROM synthetic_quota_failure) THEN RAISE EXCEPTION 'PRIVATE-QUOTA-FAILURE'; END IF;
					RETURN NEW; END $$;
					CREATE TRIGGER synthetic_failure BEFORE INSERT OR UPDATE ON identity_rate_windows FOR EACH ROW EXECUTE FUNCTION synthetic_quota_failure();`); err != nil {
					t.Fatal(err)
				}
				cleanup = "DROP TRIGGER synthetic_failure ON identity_rate_windows"
			case "commit":
				if _, err := f.Admin.Exec(t.Context(), `CREATE FUNCTION synthetic_commit_failure() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'PRIVATE-COMMIT-FAILURE'; END $$;
					CREATE CONSTRAINT TRIGGER synthetic_failure AFTER INSERT ON identity_rate_windows DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION synthetic_commit_failure();`); err != nil {
					t.Fatal(err)
				}
				cleanup = "DROP TRIGGER synthetic_failure ON identity_rate_windows"
			case "permission":
				if _, err := f.Admin.Exec(t.Context(), "REVOKE INSERT ON identity_rate_windows FROM "+role); err != nil {
					t.Fatal(err)
				}
				cleanup = "GRANT INSERT ON identity_rate_windows TO " + role
			case "unsafe role":
				bad, err := New(f.Admin, g.key, definitions())
				if err != nil {
					t.Fatal(err)
				}
				if err := bad.Check(t.Context(), p, writeRule); !errors.Is(err, orgapi.Unavailable) {
					t.Fatal("privileged runtime admitted", err)
				}
				total(t, f.Admin, 0)
			}
			if kind != "unsafe role" {
				ctx := t.Context()
				if kind == "cancelled" {
					var cancel context.CancelFunc
					ctx, cancel = context.WithCancel(ctx)
					cancel()
				}
				if err := g.Check(ctx, p, writeRule); !errors.Is(err, orgapi.Unavailable) {
					t.Fatal("failure did not fail closed", err)
				}
				total(t, f.Admin, 0)
			}
			if cleanup != "" {
				if _, err := f.Admin.Exec(t.Context(), cleanup); err != nil {
					t.Fatal(err)
				}
			}
			check(t, g, p, false)
			total(t, f.Admin, 3)
		})
	}
}

func TestFixedWindowRolloverAndExistingCleanup(t *testing.T) {
	f, g, _ := fixture(t, 1, 1, 1)
	p := principal(t, orgA)
	check(t, g, p, false)
	check(t, g, p, true)
	if _, err := f.Admin.Exec(t.Context(), "UPDATE identity_rate_windows SET window_start=window_start-interval '1 day',expires_at=expires_at-interval '1 day'"); err != nil {
		t.Fatal(err)
	}
	check(t, g, p, false)
	total(t, f.Admin, 6)
	legacy := identitypg.NewLimiter(f.Admin, identity.DefaultQuotaPolicy())
	removed, err := legacy.Cleanup(t.Context())
	if err != nil || removed != 3 {
		t.Fatal("existing cleanup did not preserve live window", removed, err)
	}
	check(t, g, p, true)
	total(t, f.Admin, 3)
}

func TestCancellationDuringObservedDatabaseLockDoesNotChargeQuota(t *testing.T) {
	f, g, _ := fixture(t, 5, 5, 5)
	p := principal(t, orgA)
	check(t, g, p, false)
	tx, err := f.Admin.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	subjects := g.subjects("synthetic.command", p)
	if _, err := tx.Exec(t.Context(), "SELECT count FROM identity_rate_windows WHERE subject_digest=$1 FOR UPDATE", subjects[0]); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- g.Check(ctx, p, writeRule) }()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	for {
		var blocked bool
		if err := f.Admin.QueryRow(t.Context(), "SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE usename=$1 AND wait_event_type='Lock')", f.RuntimeRole).Scan(&blocked); err != nil {
			t.Fatal(err)
		}
		if blocked {
			break
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatal("admission never reached held counter lock")
		}
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, orgapi.Unavailable) {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cancelled admission did not stop")
	}
	_ = tx.Rollback(context.Background())
	total(t, f.Admin, 3)
	check(t, g, p, false)
	total(t, f.Admin, 6)
}

type command struct{ orgapi.Command }

func TestHTTPAuthorityPrecedesQuotaAndAdmittedFailuresStillSpendRateBudget(t *testing.T) {
	f, g, _ := fixture(t, 2, 2, 2)
	allowed := true
	work := 0
	failWork := false
	gateway := orgapi.Gateway{
		Enabled: true, Origin: "https://api.example.invalid", CursorSecret: bytes.Repeat([]byte{1}, 32), WriteAdmission: g,
		Sessions:    func(context.Context, string, string) (string, error) { return account, nil },
		SignedOwner: func(*http.Request, string) (string, string, error) { return account, device, nil },
		Resolve: func(_ context.Context, _ orgapi.Actor, s kernel.OrgScope, _ orgapi.Rule) (orgapi.Membership, error) {
			if !allowed || s.OrgID().String() != orgA {
				return orgapi.Membership{}, orgapi.NotFound
			}
			return orgapi.Membership{ID: account, AccountID: account, OrgID: s.OrgID()}, nil
		},
	}
	router := chi.NewRouter()
	if err := orgapi.Register(router, "/v1/orgs/{org_id}/synthetic/write", gateway, writeRule, func(context.Context, orgapi.Principal, command) (any, error) {
		work++
		if failWork {
			return nil, orgapi.Conflict
		}
		return struct{}{}, nil
	}); err != nil {
		t.Fatal(err)
	}
	call := func(org string) *httptest.ResponseRecorder {
		body := `{"family_id":"` + account + `","access_token":"PRIVATE-TOKEN","schema_version":1,"operation_id":"` + account + `"}`
		r := httptest.NewRequest("POST", "/v1/orgs/"+org+"/synthetic/write", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		return w
	}
	if w := call(orgB); w.Code != 404 {
		t.Fatal(w.Code)
	}
	total(t, f.Admin, 0)
	allowed = false
	if w := call(orgA); w.Code != 404 {
		t.Fatal(w.Code)
	}
	total(t, f.Admin, 0)
	allowed = true
	if w := call(orgA); w.Code != 200 {
		t.Fatal(w.Code)
	}
	failWork = true
	if w := call(orgA); w.Code != 409 {
		t.Fatal(w.Code)
	}
	total(t, f.Admin, 6)
	w := call(orgA)
	if w.Code != 429 || work != 2 || w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("ETag") != "" {
		t.Fatal("unsafe quota response", w.Code, work)
	}
	retry, err := strconv.Atoi(w.Header().Get("Retry-After"))
	if err != nil || retry < 1 || retry > 86400 {
		t.Fatal("invalid retry", err, retry)
	}
	if strings.Contains(w.Body.String(), "PRIVATE-") || strings.Contains(w.Body.String(), orgA) {
		t.Fatal("private response leaked")
	}
	total(t, f.Admin, 6)
	allowed = false
	if w := call(orgA); w.Code != 404 {
		t.Fatal("quota exposed membership state")
	}
	total(t, f.Admin, 6)
}

// A denial reports when every exhausted axis allows another write, not only the
// first one checked: a retry after the organization minute would still meet the
// exhausted account hour.
func TestRetryAfterWaitsForEveryExhaustedAxis(t *testing.T) {
	f := orgtest.New(t)
	if _, err := f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT,UPDATE ON identity_rate_windows TO "+pgx.Identifier{f.RuntimeRole}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	defs := definitions()
	defs[0].Organization = identity.OperationQuota{Limit: 1, Window: time.Minute}
	defs[0].Account = identity.OperationQuota{Limit: 1, Window: time.Hour}
	defs[0].Device = identity.OperationQuota{Limit: 5, Window: time.Hour}
	g, err := New(f.RuntimePool(t, 4), bytes.Repeat([]byte{7}, 32), defs)
	if err != nil {
		t.Fatal(err)
	}
	p := principal(t, orgA)
	check(t, g, p, false)
	hourEnd := time.Now().UTC().Truncate(time.Hour).Add(time.Hour)
	var limit *orgapi.RateLimitError
	if err := g.Check(t.Context(), p, writeRule); !errors.As(err, &limit) || limit.RetryAfter < time.Until(hourEnd)-2*time.Second || limit.RetryAfter > time.Hour {
		t.Fatalf("retry %v before the account window ends in %v (%v)", limit, time.Until(hourEnd), err)
	}
	total(t, f.Admin, 3)
}
