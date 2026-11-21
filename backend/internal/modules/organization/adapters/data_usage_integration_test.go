//go:build integration

package adapters_test

import (
	"context"
	"crypto/sha256"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters"
	auditpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/adapters"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	billing "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgbilling/adapters"
	cappg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/adapters"
	capability "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/domain"
	datapg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgdata/adapters"
	data "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgdata/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgdata/domain"
	"github.com/jackc/pgx/v5"
)

func TestDataUsageSharesTermAndRateAcrossRotationAndRetries(t *testing.T) {
	fx, _, _ := lifecycleFixture(t)
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
	keys := datapg.KeyStore{Pool: pool, Audit: audits, Verifier: verifier,
		Authority: adapters.DataKeyAuthority{Memberships: fx.store.Memberships, Roles: fx.store.Roles, Capabilities: flags}}
	meter := datapg.UsageMeter{Plans: plans, Keys: datapg.KeyAccess{Pool: pool, Entitlements: flags, Verifier: verifier}}
	grantID := onboardingID(t)
	org := fx.scope.OrgID().String()
	now := lifecycleDBNow(t, fx)
	if _, err := fx.f.Admin.Exec(ctx, `INSERT INTO enterprise_plan_catalog
(product_code,plan_code,revision,display_name,capabilities,api_calls)
VALUES('data','synthetic',1,'Synthetic Data',ARRAY['organizations','data'],2)`); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.f.Admin.Exec(ctx, `INSERT INTO org_billing_trial_grants
(org_id,id,product_code,plan_code,plan_revision,eligible_from,eligible_until,duration_seconds,approval_ref)
VALUES($1,$2,'data','synthetic',1,$3,$4,86400,'synthetic-reviewed-eligibility')`,
		org, grantID, now.Add(-time.Hour), now.Add(time.Hour)); err != nil {
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
	next := func() datapg.UsageCall {
		return datapg.UsageCall{ID: onboardingID(t), Hash: sha256.Sum256([]byte("synthetic regional query"))}
	}
	render := func(context.Context, pgx.Tx) error { return nil }
	read := func(key domain.Bearer, call datapg.UsageCall, reader func(context.Context, pgx.Tx) error) error {
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
	if err := read(original.Bearer, a, func(context.Context, pgx.Tx) error { return domain.ErrRightsPending }); !errors.Is(err, domain.ErrRightsPending) {
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
