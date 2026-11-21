//go:build integration

package adapters_test

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters"
	auditpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/adapters"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	billing "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgbilling/adapters"
	plans "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgbilling/domain"
	cappg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/adapters"
	capability "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/domain"
	datapg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgdata/adapters"
	data "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgdata/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgdata/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type dataUsageFixtureState struct {
	fx       legalFixtureState
	pool     *pgxpool.Pool
	keys     datapg.KeyStore
	meter    datapg.UsageMeter
	flags    *cappg.Store
	auth     adapters.DataKeyAuthority
	original data.KeyResult
	issue    data.Command
	term     billing.TrialResult
	now      time.Time
}

func dataUsageFixture(t *testing.T, calls int64) dataUsageFixtureState {
	return dataUsageFixtureFor(t, calls, 24*time.Hour)
}

func dataUsageFixtureFor(t *testing.T, calls int64, duration time.Duration) dataUsageFixtureState {
	t.Helper()
	fx, _, _ := lifecycleFixture(t)
	return dataUsageFixtureOn(t, fx, calls, duration)
}

func dataUsageFixtureOn(t *testing.T, fx legalFixtureState, calls int64, duration time.Duration) dataUsageFixtureState {
	t.Helper()
	ctx := t.Context()
	pool := fx.f.RuntimePool(t, 5)
	role := pgx.Identifier{fx.f.RuntimeRole}.Sanitize()
	for _, grant := range []string{
		"GRANT SELECT,INSERT,UPDATE ON org_data_api_keys,org_data_usage_terms,org_data_rate_windows TO ",
		"GRANT SELECT,INSERT ON org_data_request_receipts,org_billing_subscription_events TO ",
		"GRANT SELECT ON enterprise_plan_catalog,org_billing_trial_grants,enterprise_data_rate_policies TO ",
	} {
		if _, err := fx.f.Admin.Exec(ctx, grant+role); err != nil {
			t.Fatal(err)
		}
	}
	known, err := capability.NewCatalog([]string{"organizations", "data"})
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := audit.NewCatalog(append(datapg.KeyAuditDefinitions(), billing.TrialAuditDefinitions()...))
	if err != nil {
		t.Fatal(err)
	}
	audits := auditpg.NewStore(pool, catalog)
	plans := billing.EntitlementStore{Pool: pool, Capabilities: known, Now: time.Now}
	flags := cappg.NewStore(pool, known, audits)
	flags.PlanWrites = plans
	verifier, err := domain.NewKeyVerifier([]byte(strings.Repeat("u", 32)))
	if err != nil {
		t.Fatal(err)
	}
	auth := adapters.DataKeyAuthority{Memberships: fx.store.Memberships, Roles: fx.store.Roles, Capabilities: flags}
	keys := datapg.KeyStore{Pool: pool, Audit: audits, Verifier: verifier, Authority: auth}
	meter := datapg.UsageMeter{Plans: plans, Keys: datapg.KeyAccess{Pool: pool, Entitlements: flags, Verifier: verifier}}
	grantID := onboardingID(t)
	org := fx.scope.OrgID().String()
	now := lifecycleDBNow(t, fx)
	if _, err := fx.f.Admin.Exec(ctx, `INSERT INTO enterprise_plan_catalog
(product_code,plan_code,revision,display_name,capabilities,api_calls)
VALUES('data','synthetic',1,'Synthetic Data',ARRAY['organizations','data'],$1)`, calls); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.f.Admin.Exec(ctx, `INSERT INTO org_billing_trial_grants
(org_id,id,product_code,plan_code,plan_revision,eligible_from,eligible_until,duration_seconds,approval_ref)
VALUES($1,$2,'data','synthetic',1,$3,$4,$5,'synthetic-reviewed-eligibility')`,
		org, grantID, now.Add(-time.Hour), now.Add(time.Hour), int64(duration/time.Second)); err != nil {
		t.Fatal(err)
	}
	trial := billing.TrialStore{Pool: pool, Ledger: billing.SubscriptionLedger{Capabilities: known}, Audit: audits}
	term, err := trial.Start(ctx, fx.scope, grantID, fx.actor.AccountID, onboardingID(t))
	if err != nil {
		t.Fatal(err)
	}
	issue := data.Command{Action: data.IssueKey, OperationID: onboardingID(t), Label: "Synthetic usage test",
		Scopes: []domain.Scope{domain.ReadRegional}, ExpiresAt: now.Add(time.Hour)}
	original, err := keys.WriteKey(ctx, fx.scope, fx.actor, fx.member, issue)
	if err != nil {
		t.Fatal(err)
	}
	return dataUsageFixtureState{fx: fx, pool: pool, keys: keys, meter: meter, flags: flags, auth: auth, original: original, issue: issue, term: term, now: now}
}

func TestDataUsageSharesTermAndRateAcrossRotationAndRetries(t *testing.T) {
	env := dataUsageFixture(t, 2)
	ctx, fx := t.Context(), env.fx
	org := fx.scope.OrgID().String()
	keys, meter, original, issue, term, now := env.keys, env.meter, env.original, env.issue, env.term, env.now
	next := func() datapg.UsageCall {
		return datapg.UsageCall{ID: onboardingID(t), Hash: sha256.Sum256([]byte("synthetic regional query"))}
	}
	render := func(context.Context, pgx.Tx, datapg.UsageAuthority) error { return nil }
	read := func(key domain.Bearer, call datapg.UsageCall, reader func(context.Context, pgx.Tx, datapg.UsageAuthority) error) error {
		return meter.Read(ctx, fx.scope, key, domain.ReadRegional, call, reader)
	}
	a := next()
	if err := read(original.Bearer, a, render); !errors.Is(err, datapg.ErrNoUsagePolicy) {
		t.Fatalf("Data request without reviewed rate policy was admitted: %v", err)
	}
	seconds := int32(86400)
	if now.Unix()%int64(seconds) > int64(seconds)-60 {
		seconds = 86513
	}
	if _, err := fx.f.Admin.Exec(ctx, `INSERT INTO enterprise_data_rate_policies
(product_code,plan_code,plan_revision,window_seconds,window_calls,approval_ref)
VALUES('data','synthetic',1,$1,5,'synthetic-reviewed-rate')`, seconds); err != nil {
		t.Fatal(err)
	}
	if err := read(original.Bearer, a, func(context.Context, pgx.Tx, datapg.UsageAuthority) error { return domain.ErrRightsPending }); !errors.Is(err, domain.ErrRightsPending) {
		t.Fatalf("unavailable source unexpectedly admitted: %v", err)
	}
	if err := read(original.Bearer, a, render); err != nil {
		t.Fatal("rolled-back read consumed the caller's entitlement", err)
	}
	if err := read(original.Bearer, a, render); err != nil {
		t.Fatal("retry failed to converge", err)
	}
	changed := a
	changed.Hash = sha256.Sum256([]byte("different synthetic query"))
	if err := read(original.Bearer, changed, render); !errors.Is(err, datapg.ErrUsageConflict) {
		t.Fatal("changed retry was accepted", err)
	}
	races := make(chan error, 2)
	for range 2 {
		call := next()
		go func() { races <- read(original.Bearer, call, render) }()
	}
	first, second := <-races, <-races
	if first != nil {
		first, second = second, first
	}
	if first != nil || !errors.Is(second, datapg.ErrQuotaExceeded) {
		t.Fatalf("last unit was overspent or not admitted: %v %v", first, second)
	}
	rotate := issue
	rotate.Action, rotate.OperationID, rotate.TargetID, rotate.ExpectedRevision = data.RotateKey, onboardingID(t), original.Key.ID, 1
	rotate.OverlapUntil = now.Add(10 * time.Minute)
	replacement, err := keys.WriteKey(ctx, fx.scope, fx.actor, fx.member, rotate)
	if err != nil {
		t.Fatal(err)
	}
	if err := read(replacement.Bearer, next(), render); !errors.Is(err, datapg.ErrQuotaExceeded) {
		t.Fatalf("rotating the key reset the term quota: %v", err)
	}
	if err := read(replacement.Bearer, a, render); err != nil {
		t.Fatal("matching retry could not resume through the replacement", err)
	}
	if err := read(original.Bearer, a, render); err != nil {
		t.Fatal("last rate unit could not be used during overlap", err)
	}
	var limited datapg.RateExceeded
	if err := read(replacement.Bearer, a, render); !errors.As(err, &limited) || limited.RetryAfter <= 0 {
		t.Fatalf("new key bypassed the inherited rate window: %v", err)
	}
	foreign := fx.f.Organizations[1].Scope(t)
	if err := meter.Read(ctx, foreign, replacement.Bearer, domain.ReadRegional, a, render); !errors.Is(err, domain.ErrKeyDenied) {
		t.Fatalf("foreign organization consumed the key: %v", err)
	}
	var calls, receipts, familyCalls int64
	err = fx.f.Admin.QueryRow(ctx, `SELECT
 (SELECT calls FROM org_data_usage_terms WHERE org_id=$1 AND term_start=$2),
 (SELECT count(*) FROM org_data_request_receipts WHERE org_id=$1 AND term_start=$2),
 (SELECT COALESCE(sum(calls),0)::bigint FROM org_data_rate_windows WHERE org_id=$1 AND family_id=$3)`,
		org, term.Period.Start, original.Key.ID).Scan(&calls, &receipts, &familyCalls)
	if err != nil || calls != 2 || receipts != 2 || familyCalls != 5 {
		t.Fatalf("usage ledger drifted: term=%d receipts=%d rate=%d error=%v", calls, receipts, familyCalls, err)
	}
}

func TestCompanyPlanAndDataReadsFollowCommittedSuspension(t *testing.T) {
	env := dataUsageFixture(t, 2)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	fx := env.fx
	role, password := fx.f.ProvisionScriptRole(t)
	pool := fx.f.ScriptRuntimePool(t, role, password, 4)
	usage := datapg.UsageStore{Pool: pool, Authority: env.auth, Flags: env.flags, Plans: env.meter.Plans, Enabled: true}
	planReader := adapters.SubscriptionAuthority{Memberships: fx.store.Memberships, Roles: fx.store.Roles}
	known, err := capability.NewCatalog([]string{"organizations", "data"})
	if err != nil {
		t.Fatal(err)
	}
	overview := billing.OverviewStore{Pool: pool, Authority: planReader, Capabilities: known, BillingEnabled: true}
	meter := env.meter
	meter.Keys.Pool = pool
	if current, err := usage.ReadUsage(ctx, fx.scope, fx.actor, fx.member, data.DatasetAfter{}, 20); err != nil ||
		!current.Enabled || current.Term == nil || current.Term.Ceiling != 2 {
		t.Fatalf("active fixture Data allowance unavailable: %+v / %v", current, err)
	}

	holder, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		t.Fatal(err)
	}
	defer holder.Rollback(context.Background())
	if _, err := holder.Exec(ctx, `SELECT pg_catalog.set_config('anpfuel.org_id',$1,true)`, fx.scope.OrgID().String()); err != nil {
		t.Fatal(err)
	}
	ledger := billing.SubscriptionLedger{Capabilities: known}
	event := plans.LifecycleEvent{Scope: fx.scope, SubscriptionID: env.term.SubscriptionID, ExpectedRevision: 1,
		Kind: plans.SuspendAccess, Source: plans.ReviewedOperator, At: lifecycleDBNow(t, fx), Reference: "synthetic-reviewed-suspension"}
	if _, err := ledger.ApplyTx(ctx, holder, plans.PlanKey{Product: "data", Plan: "synthetic", Revision: 1}, event); err != nil {
		t.Fatal(err)
	}
	checks := map[string]func() (time.Time, error){
		"usage panel": func() (time.Time, error) {
			view, err := usage.ReadUsage(ctx, fx.scope, fx.actor, fx.member, data.DatasetAfter{}, 20)
			if err != nil || view.Enabled || view.Term != nil || view.Rate != nil || len(view.Datasets) != 0 {
				return time.Time{}, fmt.Errorf("suspended plan exposed current Data allowance: %+v / %v", view, err)
			}
			return view.ObservedAt, nil
		},
		"billing panel": func() (time.Time, error) {
			view, err := overview.Read(ctx, fx.scope, fx.actor, fx.member)
			if err != nil || len(view.Plans) != 1 || view.Plans[0].State != plans.Suspended {
				return time.Time{}, fmt.Errorf("billing panel did not observe committed suspension: %+v / %v", view, err)
			}
			return view.ObservedAt, nil
		},
		"regional API": func() (time.Time, error) {
			call := datapg.UsageCall{ID: env.issue.OperationID, Hash: sha256.Sum256([]byte("synthetic request after suspension"))}
			err := meter.Read(ctx, fx.scope, env.original.Bearer, domain.ReadRegional, call,
				func(context.Context, pgx.Tx, datapg.UsageAuthority) error { return errors.New("unexpected admission") })
			if !errors.Is(err, domain.ErrKeyDenied) {
				return time.Time{}, fmt.Errorf("regional API failed to refuse the suspension: %v", err)
			}
			return time.Time{}, nil
		},
	}
	type result struct {
		name       string
		observedAt time.Time
		err        error
	}
	results := make(chan result, len(checks))
	for name, check := range checks {
		go func() {
			at, err := check()
			results <- result{name: name, observedAt: at, err: err}
		}()
	}
	pid := int(holder.Conn().PgConn().PID())
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		var waiting int
		if err := fx.f.Admin.QueryRow(ctx, `SELECT count(*) FROM pg_stat_activity
 WHERE datname=current_database() AND usename=$1 AND wait_event_type='Lock'
   AND $2=ANY(pg_blocking_pids(pid)) AND query LIKE '%name: LockEntitlementRead%'`, role, pid).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting == len(checks) {
			break
		}
		select {
		case early := <-results:
			t.Fatalf("%s completed before the in-flight suspension committed: %v", early.name, early.err)
		case <-ctx.Done():
			t.Fatal("reads never reached the shared entitlement lock")
		case <-ticker.C:
		}
	}
	var beforeCommit time.Time
	if err := holder.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&beforeCommit); err != nil {
		t.Fatal(err)
	}
	if err := holder.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	for range checks {
		read := <-results
		if read.err != nil {
			t.Errorf("%s: %v", read.name, read.err)
		} else if read.name != "regional API" && read.observedAt.Before(beforeCommit) {
			t.Errorf("%s used the clock from before its entitlement lock wait", read.name)
		}
	}
	var used int
	if err := fx.f.Admin.QueryRow(ctx, `SELECT count(*) FROM org_data_request_receipts WHERE org_id=$1`, fx.scope.OrgID().String()).Scan(&used); err != nil || used != 0 {
		t.Fatalf("refused Data call changed its subscription usage: %d / %v", used, err)
	}
}
