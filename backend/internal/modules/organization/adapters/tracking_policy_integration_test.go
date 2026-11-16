//go:build integration

package adapters_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	migrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const trackingPolicyMigration = "000247_organization_tracking_policy_commands.sql"

// policyRequest is a goroutine-safe company request returning the status
// and the error code (empty on success).
func policyRequest(ctx context.Context, router chi.Router, actor matrixPerson, path string, body any) (int, string) {
	raw, _ := json.Marshal(body)
	r := httptest.NewRequestWithContext(ctx, http.MethodPost, deviceTestOrigin+path, bytes.NewReader(raw))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Origin", deviceTestOrigin)
	r.AddCookie(actor.cookie)
	r.Header.Set("X-CSRF", actor.csrf)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	var result struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &result)
	return w.Code, result.Error.Code
}

// BUC-EMP-238: two administrators queued on one revision behind a held
// organization lock; the first queued wins and the other is CONFLICT with
// nothing written, in both orderings.
func TestTrackingPolicyConcurrentChangesConvergeInBothOrders(t *testing.T) {
	x := trackingFixture(t)
	router := trackingPolicyRouter(t, x)
	fx := x.deviceFixtureState
	org := x.scope.OrgID().String()
	change := "/v1/orgs/" + org + "/tracking/policy/change"
	people := [2]matrixPerson{matrixNewPerson(t, fx, router), matrixNewPerson(t, fx, router)}
	matrixAssign(t, fx, people[0], "OWNER", "ORG", "ACTIVE")
	matrixAssign(t, fx, people[1], "ADMIN", "ORG", "ACTIVE")
	notice := lifecycleDBNow(t, x.legalFixtureState).Add(-time.Hour).Format(time.RFC3339Nano)
	race := func(order [2]int, revision any) [2]int {
		t.Helper()
		ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
		defer cancel()
		holder, err := x.f.Runtime.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer holder.Rollback(ctx)
		if _, err := holder.Exec(ctx, `SELECT set_config('anpfuel.org_id',$1,true)`, org); err != nil {
			t.Fatal(err)
		}
		if _, err := holder.Exec(ctx, `SELECT 1 FROM organizations WHERE org_id=$1 FOR UPDATE`, org); err != nil {
			t.Fatal(err)
		}
		waitBlocked := func(n int) {
			ticker := time.NewTicker(5 * time.Millisecond)
			defer ticker.Stop()
			for {
				var blocked int
				if err := x.f.Admin.QueryRow(ctx, `SELECT count(*) FROM pg_stat_activity WHERE datname=current_database() AND usename=$1 AND wait_event_type='Lock' AND pg_blocking_pids(pid)<>'{}'`, x.f.RuntimeRole).Scan(&blocked); err != nil {
					t.Fatal(err)
				}
				if blocked >= n {
					return
				}
				select {
				case <-ticker.C:
				case <-ctx.Done():
					t.Fatal("no lock wait observed")
				}
			}
		}
		var statuses [2]chan int
		for i := range statuses {
			statuses[i] = make(chan int, 1)
		}
		policies := [2]string{"EXPECTED", "OPTIONAL"}
		send := func(i int) {
			status, _ := policyRequest(ctx, router, people[i], change, policyBody(onboardingID(t), revision, policies[i], "notice-2026.11", notice))
			statuses[i] <- status
		}
		go send(order[0])
		waitBlocked(1)
		go send(order[1])
		waitBlocked(2)
		if err := holder.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		return [2]int{<-statuses[order[0]], <-statuses[order[1]]}
	}
	for round, order := range [][2]int{{0, 1}, {1, 0}} {
		var revision any
		if round > 0 {
			revision = int64(round)
		}
		got := race(order, revision)
		if got != [2]int{200, 409} {
			t.Fatal("first queued change must win", order, got)
		}
		var policy string
		var revisions, receipts, events int
		if err := x.f.Admin.QueryRow(t.Context(), `SELECT (SELECT policy FROM org_tracking_policies WHERE org_id=$1 ORDER BY revision DESC LIMIT 1),
 (SELECT count(*) FROM org_tracking_policies WHERE org_id=$1),(SELECT count(*) FROM org_tracking_policy_commands WHERE org_id=$1),
 (SELECT count(*) FROM org_audit_events WHERE org_id=$1 AND action='gps.policy.changed')`, org).Scan(&policy, &revisions, &receipts, &events); err != nil {
			t.Fatal(err)
		}
		want := []string{"EXPECTED", "OPTIONAL"}[order[0]]
		if policy != want || revisions != round+1 || receipts != round+1 || events != round+1 {
			t.Fatal("the losing change left state", policy, revisions, receipts, events)
		}
	}
}

func policyGuardCode(err error) string {
	var pe *pgconn.PgError
	if errors.As(err, &pe) {
		return pe.Code
	}
	return ""
}

// EDR-303: the receipt table is forced-RLS and insert-only, and its guard
// refuses a receipt that names an older version, a version whose notice was
// not in force, another organization's scope or an inactive author.
func TestTrackingPolicyCommandSchemaGuards(t *testing.T) {
	x := trackingFixture(t)
	trackingPolicyGrants(t, x)
	org := x.scope.OrgID().String()
	member, account := x.source.MembershipID, x.source.Actor.AccountID
	role := pgx.Identifier{x.f.RuntimeRole}.Sanitize()
	if _, err := x.f.Admin.Exec(t.Context(), "GRANT UPDATE,DELETE ON org_tracking_policy_commands TO "+role); err != nil {
		t.Fatal(err)
	}
	var forced bool
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT relrowsecurity AND relforcerowsecurity FROM pg_class WHERE oid='org_tracking_policy_commands'::regclass`).Scan(&forced); err != nil || !forced {
		t.Fatal("receipts are not forced-RLS", err)
	}
	exec := func(scope, sql string, args ...any) error {
		tx, err := x.f.Runtime.Begin(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = tx.Rollback(context.Background()) }()
		if _, err := tx.Exec(t.Context(), `SELECT set_config('anpfuel.org_id',$1,true)`, scope); err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(t.Context(), sql, args...); err != nil {
			return err
		}
		return tx.Commit(t.Context())
	}
	receipt := `INSERT INTO org_tracking_policy_commands(org_id,operation_id,actor_account_id,actor_membership_id,request_hash,result_revision)VALUES($1,$2,$3,$4,$5,$6)`
	hash := strings.Repeat("ab", 32)
	notice := lifecycleDBNow(t, x.legalFixtureState).Add(-time.Hour)
	x.policy(t, "EXPECTED", &notice)
	x.policy(t, "OFF", nil)
	if code := policyGuardCode(exec(org, receipt, org, onboardingID(t), account, member, hash, 1)); code != "23514" {
		t.Fatal("a receipt of an older version was admitted", code)
	}
	if code := policyGuardCode(exec(onboardingID(t), receipt, org, onboardingID(t), account, member, hash, 2)); code != "42501" {
		t.Fatal("another scope wrote a receipt", code)
	}
	if code := policyGuardCode(exec(org, receipt, org, onboardingID(t), onboardingID(t), member, hash, 2)); code != "23514" {
		t.Fatal("a receipt of another account was admitted", code)
	}
	if code := policyGuardCode(exec(org, receipt, org, onboardingID(t), account, member, "x", 2)); code != "23514" {
		t.Fatal("a malformed digest was admitted", code)
	}
	op := onboardingID(t)
	if err := exec(org, receipt, org, op, account, member, hash, 2); err != nil {
		t.Fatal("a receipt of the newest version", err)
	}
	if code := policyGuardCode(exec(org, receipt, org, onboardingID(t), account, member, hash, 2)); code != "23505" {
		t.Fatal("a second receipt of one version", code)
	}
	// A version whose notice is not in force at its effective instant.
	future := lifecycleDBNow(t, x.legalFixtureState).Add(time.Hour)
	x.policy(t, "EXPECTED", &future)
	if code := policyGuardCode(exec(org, receipt, org, onboardingID(t), account, member, hash, 3)); code != "23514" {
		t.Fatal("a receipt for a notice not in force", code)
	}
	for _, sql := range []string{`UPDATE org_tracking_policy_commands SET request_hash=request_hash WHERE org_id=$1`, `DELETE FROM org_tracking_policy_commands WHERE org_id=$1`} {
		if code := policyGuardCode(exec(org, sql, org)); code != "55000" {
			t.Fatal("receipts are not insert-only", sql, code)
		}
	}
	if _, err := x.f.Admin.Exec(t.Context(), `TRUNCATE org_tracking_policy_commands`); policyGuardCode(err) != "55000" {
		t.Fatal("receipts can be truncated", err)
	}
	var visible int
	if err := exec(onboardingID(t), `SELECT 1`); err != nil {
		t.Fatal(err)
	}
	tx, err := x.f.Runtime.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if _, err := tx.Exec(t.Context(), `SELECT set_config('anpfuel.org_id',$1,true)`, onboardingID(t)); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(t.Context(), `SELECT count(*) FROM org_tracking_policy_commands`).Scan(&visible); err != nil || visible != 0 {
		t.Fatal("another scope sees receipts", visible, err)
	}
}

// A populated head before 000247 (a policy version written the EMP-GPS-004
// way and a recording session) upgrades by an ordinary non-superuser
// migrator: an injected failure leaves schema and data untouched, recovery
// applies once, replay is a no-op, the previous image stays compatible, and
// the composed route then writes the next version over the existing one.
func TestTrackingPolicyPopulatedUpgradeFailureRecoveryAndPreviousImage(t *testing.T) {
	previous := fstest.MapFS{}
	entries, err := fs.ReadDir(migrations.Files, ".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() != "schema_min_supported" && (entry.Name() >= trackingPolicyMigration || !strings.HasSuffix(entry.Name(), ".sql")) {
			continue
		}
		raw, err := fs.ReadFile(migrations.Files, entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		previous[entry.Name()] = &fstest.MapFile{Data: raw}
	}
	if _, ok := previous[trackingMigration]; !ok {
		t.Fatal("previous image does not contain", trackingMigration)
	}
	f := orgtest.NewWithMigrations(t, previous)
	_, _, onboarding, actor, flags := onboardingOnFixture(t, f)
	legal := legalFixtureFor(t, f, onboarding, actor, flags)
	x := trackingFixtureOn(t, tripFixtureOn(t, selfIntakeFixtureOn(t, syncFixtureOn(t, devicesFixtureOn(t, legal)))))
	d := x.syncFixtureState
	x.inForce(t)
	session := onboardingID(t)
	x.startTracking(t, d, session, 1, "PRECISE", 2)
	snapshot := func() string {
		t.Helper()
		var value string
		if err := f.Admin.QueryRow(t.Context(), `SELECT jsonb_build_array((SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,revision)FROM org_tracking_policies x),
(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_tracking_sessions x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,session_id,seq)FROM org_tracking_transitions x),
(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,operation_id)FROM org_audit_events x))::text`).Scan(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	before := snapshot()
	migrator := f.RuntimeRole + "_policy_migrator"
	role := pgx.Identifier{migrator}.Sanitize()
	if _, err := f.Admin.Exec(t.Context(), "CREATE ROLE "+role+" NOLOGIN NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE;GRANT USAGE,CREATE ON SCHEMA public TO "+role+";GRANT SELECT,INSERT ON schema_migrations TO "+role+
		";GRANT REFERENCES ON organizations,org_tracking_policies,org_memberships TO "+role); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := f.Admin.Exec(ctx, "REASSIGN OWNED BY "+role+" TO CURRENT_USER;DROP OWNED BY "+role+";DROP ROLE "+role); err != nil {
			t.Error(err)
		}
	})
	location, err := url.Parse(f.AdminDSN)
	if err != nil {
		t.Fatal(err)
	}
	query := location.Query()
	query.Set("options", "-c role="+migrator)
	location.RawQuery = strings.ReplaceAll(query.Encode(), "+", "%20")
	raw, err := fs.ReadFile(migrations.Files, trackingPolicyMigration)
	if err != nil {
		t.Fatal(err)
	}
	version := strings.SplitN(trackingPolicyMigration, "_", 2)[0]
	broken := fstest.MapFS{trackingPolicyMigration: &fstest.MapFile{Data: append(append([]byte{}, raw...), []byte("\nSELECT 1/0;\n")...)}}
	if _, err := migrate.Apply(t.Context(), location.String(), broken); err == nil || !strings.Contains(err.Error(), "SQLSTATE 22012") {
		t.Fatal("injected policy migration failure not reached", err)
	}
	var clean bool
	if err := f.Admin.QueryRow(t.Context(), `SELECT to_regclass('org_tracking_policy_commands') IS NULL AND to_regprocedure('org_tracking_policy_command_guard()') IS NULL
 AND NOT EXISTS(SELECT 1 FROM schema_migrations WHERE version=$1)`, version).Scan(&clean); err != nil || !clean || before != snapshot() {
		t.Fatal("failed policy migration changed the original schema or data", err)
	}
	current := fstest.MapFS{}
	for path, data := range previous {
		current[path] = data
	}
	current[trackingPolicyMigration] = &fstest.MapFile{Data: raw}
	if applied, err := migrate.Apply(t.Context(), location.String(), current); err != nil || len(applied) != 1 || applied[0] != version {
		t.Fatal("ordinary policy migration recovery", applied, err)
	}
	if before != snapshot() {
		t.Fatal("policy upgrade rewrote existing state")
	}
	if applied, err := migrate.Apply(t.Context(), location.String(), current); err != nil || len(applied) != 0 {
		t.Fatal("policy migration no-op replay", err)
	}
	if err := migrate.Check(t.Context(), f.Admin, previous); err != nil {
		t.Fatal("previous embedded image incompatible", err)
	}
	if err := f.Admin.QueryRow(t.Context(), `SELECT NOT rolsuper AND NOT rolbypassrls AND NOT rolcanlogin AND(SELECT relrowsecurity AND relforcerowsecurity AND relowner=(SELECT oid FROM pg_roles WHERE rolname=$1)
 FROM pg_class WHERE oid='org_tracking_policy_commands'::regclass)FROM pg_roles WHERE rolname=$1`, migrator).Scan(&clean); err != nil || !clean {
		t.Fatal("the receipt table is not owned by the ordinary forced-RLS migrator", err)
	}
	// The composed route writes revision 2 over the pre-upgrade version.
	router := trackingPolicyRouter(t, x)
	owner := matrixNewPerson(t, x.deviceFixtureState, router)
	matrixAssign(t, x.deviceFixtureState, owner, "OWNER", "ORG", "ACTIVE")
	org := x.scope.OrgID().String()
	if status, code := policyRequest(t.Context(), router, owner, "/v1/orgs/"+org+"/tracking/policy/change", policyBody(onboardingID(t), nil, "OFF", nil, nil)); status != 409 || code != "CONFLICT" {
		t.Fatal("a change ignoring the existing version", status, code)
	}
	if status, code := policyRequest(t.Context(), router, owner, "/v1/orgs/"+org+"/tracking/policy/change", policyBody(onboardingID(t), 1, "OFF", nil, nil)); status != 200 || code != "" {
		t.Fatal("a change after the upgrade", status, code)
	}
	batch := x.batchOp(t, d, session, 1, []int{2, 12}, nil)
	ack, err := x.process(t, d, batch)
	x.accepted(t, batch, ack, err, 1, trackingCounts{segments: 1})
	if _, err := migrate.Apply(t.Context(), f.AdminDSN, migrations.Files); err != nil {
		t.Fatal("remaining embedded chain after the 247 rehearsal", err)
	}
}
