//go:build integration

package adapters_test

import (
	"errors"
	"slices"
	"testing"
	"time"

	permissions "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	organization "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	auditpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/adapters"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	billing "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgbilling/adapters"
	capability "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestReviewedCompanyTrialIsScopedAuditedAndConsumedOnce(t *testing.T) {
	fx, _, _ := lifecycleFixture(t)
	ctx := t.Context()
	pool := fx.f.RuntimePool(t, 3)
	role := pgx.Identifier{fx.f.RuntimeRole}.Sanitize()
	if _, err := fx.f.Admin.Exec(ctx, "GRANT SELECT ON enterprise_plan_catalog,org_billing_trial_grants TO "+role+
		"; GRANT SELECT,INSERT ON org_billing_subscription_events TO "+role); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.f.Admin.Exec(ctx, `INSERT INTO enterprise_plan_catalog
(product_code,plan_code,revision,display_name,capabilities,active_vehicles)
VALUES('fleet','synthetic',1,'Synthetic company',ARRAY['organizations'],2)`); err != nil {
		t.Fatal(err)
	}
	org := fx.scope.OrgID().String()
	a, b, expired := onboardingID(t), onboardingID(t), onboardingID(t)
	at := lifecycleDBNow(t, fx)
	for _, grant := range []struct {
		id   string
		ends time.Time
	}{{a, at.Add(time.Hour)}, {b, at.Add(time.Hour)}, {expired, at.Add(-time.Hour)}} {
		if _, err := fx.f.Admin.Exec(ctx, `INSERT INTO org_billing_trial_grants
(org_id,id,product_code,plan_code,plan_revision,eligible_from,eligible_until,duration_seconds,approval_ref)
VALUES($1,$2,'fleet','synthetic',1,$3,$4,86400,'synthetic-reviewed-eligibility')`,
			org, grant.id, at.Add(-2*time.Hour), grant.ends); err != nil {
			t.Fatal(err)
		}
	}
	known, err := capability.NewCatalog([]string{"organizations"})
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := audit.NewCatalog(billing.TrialAuditDefinitions())
	if err != nil {
		t.Fatal(err)
	}
	store := billing.TrialStore{Pool: pool, Ledger: billing.SubscriptionLedger{Capabilities: known},
		Audit: auditpg.NewStore(pool, catalog)}
	operator := fx.actor.AccountID
	operation := onboardingID(t)
	if _, err := store.Start(ctx, fx.scope, expired, operator, onboardingID(t)); !errors.Is(err, billing.ErrTrialIneligible) {
		t.Fatalf("expired grant activated: %v", err)
	}
	if _, err := store.Start(ctx, fx.f.Organizations[1].Scope(t), a, operator, operation); !errors.Is(err, billing.ErrTrialIneligible) {
		t.Fatalf("another organization consumed the grant: %v", err)
	}
	disallowed := store
	disallowed.Pool = fx.f.Admin
	if _, err := disallowed.Start(ctx, fx.scope, a, operator, operation); !errors.Is(err, database.ErrUnsafeOrgRole) {
		t.Fatalf("database owner activated a trial through the runtime entry point: %v", err)
	}

	err = database.WithOrgTx(ctx, pool, fx.scope, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO org_billing_subscription_events
(org_id,subscription_id,revision,product_code,plan_code,plan_revision,from_state,state,prior_state,event_kind,event_source,
 evidence_reference,occurred_at,term_start,term_end)
VALUES($1,$2,1,'fleet','synthetic',1,'','TRIALING','','START_TRIAL','TRIAL_POLICY',
 'synthetic-reviewed-eligibility',$3,$3,$3::timestamptz + interval '24 hours')`, org, a, at)
		return err
	})
	var openingError *pgconn.PgError
	if !errors.As(err, &openingError) || openingError.Code != "23514" ||
		openingError.Message != "subscription opening proof unavailable" {
		t.Fatalf("direct trial without its operator audit committed: %v", err)
	}

	type outcome struct {
		value billing.TrialResult
		err   error
	}
	replies := make(chan outcome, 2)
	for range 2 {
		go func() {
			value, err := store.Start(ctx, fx.scope, a, operator, operation)
			replies <- outcome{value, err}
		}()
	}
	first, second := <-replies, <-replies
	if first.err != nil || second.err != nil || first.value.Replayed == second.value.Replayed ||
		first.value.SubscriptionID != a || second.value.SubscriptionID != a ||
		first.value.Period != second.value.Period || first.value.Period.End.Sub(first.value.Period.Start) != 24*time.Hour {
		t.Fatalf("matching trial commands did not converge on one reviewed period: %+v %+v", first, second)
	}
	// A paid term for another plan must begin exactly at the trial end, and
	// provider billing periods are whole seconds.
	var fractional bool
	if err := fx.f.Admin.QueryRow(ctx, `SELECT bool_or(term_start<>date_trunc('second',term_start) OR term_end<>date_trunc('second',term_end))
FROM org_billing_subscription_events WHERE org_id=$1 AND event_kind='START_TRIAL'`, org).Scan(&fractional); err != nil || fractional ||
		!first.value.Period.Start.Equal(first.value.Period.Start.Truncate(time.Second)) {
		t.Fatalf("trial term is not a whole-second provider boundary: %+v %v", first.value.Period, err)
	}
	if _, err := store.Start(ctx, fx.scope, a, operator, onboardingID(t)); !errors.Is(err, billing.ErrTrialConflict) {
		t.Fatalf("consumed grant started a second term: %v", err)
	}
	if _, err := store.Start(ctx, fx.scope, b, operator, operation); !errors.Is(err, audit.ErrReused) {
		t.Fatalf("original operation was rebound to a different grant: %v", err)
	}
	if _, err := store.Start(ctx, fx.scope, b, operator, onboardingID(t)); !errors.Is(err, billing.ErrTrialConflict) {
		t.Fatalf("another grant replaced the existing subscription: %v", err)
	}

	var trials, audited, paid, roles int
	if err := fx.f.Admin.QueryRow(ctx, `SELECT
 (SELECT count(*) FROM org_billing_subscription_events WHERE org_id=$1 AND event_kind='START_TRIAL'),
 (SELECT count(*) FROM org_audit_events WHERE org_id=$1 AND action='billing.trial.started' AND actor_id=$2),
 (SELECT count(*) FROM org_billing_settlement_receipts WHERE org_id=$1),
 (SELECT count(*) FROM org_role_assignments WHERE org_id=$1)`, org, operator).Scan(&trials, &audited, &paid, &roles); err != nil ||
		trials != 1 || audited != 1 || paid != 0 || roles != 1 {
		t.Fatalf("trial changed roles or financial history: trials=%d audit=%d paid=%d roles=%d err=%v", trials, audited, paid, roles, err)
	}

	// An ended subscription remains discoverable to the current member.
	// Simulate the retained read after the term and the pilot write switch end.
	flags, err := fx.flags.State(ctx, fx.scope, "organizations")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fx.flags.Set(ctx, fx.scope, capability.Actor{AccountID: operator}, capability.Change{
		OperationID: onboardingID(t), Capability: "organizations", ExpectedRevision: flags.Revision, Enabled: false,
	}); err != nil {
		t.Fatal(err)
	}
	contexts := contextStore(fx)
	contexts.Plans = billing.EntitlementStore{Pool: pool, Capabilities: known,
		Now: func() time.Time { return first.value.Period.End.Add(time.Second) }}
	actor := organization.CompanyActor{AccountID: operator, SessionID: fx.actor.SessionID}
	page, err := contexts.ListAccountContexts(ctx, actor, 100, "")
	if err != nil || len(page.Organizations) != 1 || page.Organizations[0].ID != org {
		t.Fatalf("company plan history disappeared after writes stopped: %+v %v", page, err)
	}
	view, err := contexts.ReadAccountContext(ctx, actor, fx.scope, fx.member)
	if err != nil || !slices.Contains(view.Permissions, permissions.Permission("subs.plan.read")) ||
		slices.Contains(view.Permissions, permissions.Permission("idn.employee.manage")) {
		t.Fatalf("ended subscription offered the wrong company controls: %+v %v", view, err)
	}
}
