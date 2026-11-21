//go:build integration

package adapters_test

import (
	"context"
	"errors"
	"fmt"
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

func TestDataKeysBindCurrentCompanyPlanAndReleaseOneBearer(t *testing.T) {
	fx, _, _ := lifecycleFixture(t)
	ctx := t.Context()
	pool := fx.f.RuntimePool(t, 4)
	role := pgx.Identifier{fx.f.RuntimeRole}.Sanitize()
	if _, err := fx.f.Admin.Exec(ctx, "GRANT SELECT,INSERT,UPDATE ON org_data_api_keys TO "+role+
		"; GRANT SELECT ON enterprise_plan_catalog,org_billing_trial_grants TO "+role+
		"; GRANT SELECT,INSERT ON org_billing_subscription_events TO "+role); err != nil {
		t.Fatal(err)
	}
	known, err := capability.NewCatalog([]string{"organizations", "data"})
	if err != nil {
		t.Fatal(err)
	}
	defs := append(datapg.KeyAuditDefinitions(), billing.TrialAuditDefinitions()...)
	defs = append(defs, cappg.AuditDefinitions()...)
	catalog, err := audit.NewCatalog(defs)
	if err != nil {
		t.Fatal(err)
	}
	audits := auditpg.NewStore(pool, catalog)
	flags := cappg.NewStore(pool, known, audits)
	flags.PlanWrites = billing.EntitlementStore{Pool: pool, Capabilities: known, Now: time.Now}
	verifier, err := domain.NewKeyVerifier([]byte(strings.Repeat("s", 32)))
	if err != nil {
		t.Fatal(err)
	}
	store := datapg.KeyStore{Pool: pool, Audit: audits, Verifier: verifier,
		Authority: adapters.DataKeyAuthority{Memberships: fx.store.Memberships, Roles: fx.store.Roles, Capabilities: flags}}
	access := datapg.KeyAccess{Pool: pool, Entitlements: flags, Verifier: verifier}
	now := lifecycleDBNow(t, fx)
	issue := data.Command{Action: data.IssueKey, OperationID: onboardingID(t), Label: "Synthetic API client",
		Scopes: []domain.Scope{domain.ReadRegional}, ExpiresAt: now.Add(time.Hour)}
	write := func(input data.Command) (data.KeyResult, error) {
		return store.WriteKey(ctx, fx.scope, fx.actor, fx.member, input)
	}
	if _, err := write(issue); !errors.Is(err, capability.ErrEntitlementRequired) {
		t.Fatalf("issued a Data key before approval: %v", err)
	}
	org := fx.scope.OrgID().String()
	grant := onboardingID(t)
	if _, err := fx.f.Admin.Exec(ctx, `INSERT INTO enterprise_plan_catalog
(product_code,plan_code,revision,display_name,capabilities,api_calls)
VALUES('data','synthetic',1,'Synthetic Data',ARRAY['organizations','data'],10)`); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.f.Admin.Exec(ctx, `INSERT INTO org_billing_trial_grants
(org_id,id,product_code,plan_code,plan_revision,eligible_from,eligible_until,duration_seconds,approval_ref)
VALUES($1,$2,'data','synthetic',1,$3,$4,86400,'synthetic-reviewed-eligibility')`,
		org, grant, now.Add(-time.Hour), now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	trial := billing.TrialStore{Pool: pool, Ledger: billing.SubscriptionLedger{Capabilities: known}, Audit: audits}
	if _, err := trial.Start(ctx, fx.scope, grant, fx.actor.AccountID, onboardingID(t)); err != nil {
		t.Fatal(err)
	}

	type result struct {
		out data.KeyResult
		err error
	}
	outcomes := make(chan result, 2)
	for range 2 {
		go func() {
			out, err := write(issue)
			outcomes <- result{out, err}
		}()
	}
	first, second := <-outcomes, <-outcomes
	if first.out.Replayed {
		first, second = second, first
	}
	if first.err != nil || second.err != nil || first.out.Replayed || !second.out.Replayed ||
		first.out.Bearer.Empty() || !second.out.Bearer.Empty() || first.out.Key.ID != second.out.Key.ID {
		t.Fatalf("matching requests did not release exactly one bearer: %+v %+v", first, second)
	}
	old := first.out.Bearer
	if strings.Contains(fmt.Sprintf("%+v %#v", old, old), old.Reveal()) {
		t.Fatal("a formatted Data key revealed its bearer")
	}
	altered := issue
	altered.Label = "Different synthetic purpose"
	if _, err := write(altered); !errors.Is(err, audit.ErrReused) {
		t.Fatalf("changed replay unexpectedly succeeded: %v", err)
	}
	callback := func(_ context.Context, _ pgx.Tx, key domain.Key) error {
		if key.ID != old.ID() {
			t.Fatal("unexpected key context")
		}
		return nil
	}
	if err := access.WithAuthorizedKey(ctx, fx.scope, old, domain.ReadRegional, callback); err != nil {
		t.Fatal("current key was denied", err)
	}
	other := fx.f.Organizations[1].Scope(t)
	if err := access.WithAuthorizedKey(ctx, other, old, domain.ReadRegional, callback); !errors.Is(err, domain.ErrKeyDenied) {
		t.Fatal("foreign organization admitted the key", err)
	}
	wrong, err := domain.NewBearer(old.ID(), strings.Repeat("a", 64))
	if err != nil {
		t.Fatal(err)
	}
	if err := access.WithAuthorizedKey(ctx, fx.scope, wrong, domain.ReadRegional, callback); !errors.Is(err, domain.ErrKeyDenied) {
		t.Fatal("wrong bearer admitted", err)
	}
	rotate := issue
	rotate.Action, rotate.OperationID, rotate.TargetID = data.RotateKey, onboardingID(t), old.ID()
	rotate.ExpectedRevision, rotate.OverlapUntil = 1, now.Add(30*time.Minute)
	broader := rotate
	broader.Scopes = []domain.Scope{domain.ReadRegional, "admin.write"}
	if _, err := write(broader); !errors.Is(err, domain.ErrInvalidKey) {
		t.Fatal("rotation allowed a broader scope", err)
	}
	rotated, err := write(rotate)
	if err != nil || rotated.Bearer.Empty() || rotated.Key.ID == old.ID() {
		t.Fatalf("replacement not issued: %+v %v", rotated, err)
	}
	if err := access.WithAuthorizedKey(ctx, fx.scope, old, domain.ReadRegional, callback); err != nil {
		t.Fatal("recorded overlap not honored", err)
	}
	if _, err := flags.Set(ctx, fx.scope, capability.Actor{AccountID: fx.actor.AccountID}, capability.Change{
		OperationID: onboardingID(t), Capability: "data", Enabled: false,
	}); err != nil {
		t.Fatal(err)
	}
	live := func(_ context.Context, _ pgx.Tx, _ domain.Key) error { return nil }
	if err := access.WithAuthorizedKey(ctx, fx.scope, rotated.Bearer, domain.ReadRegional, live); !errors.Is(err, domain.ErrKeyDenied) {
		t.Fatal("plan hold did not immediately stop bearer access", err)
	}
	_, err = write(data.Command{Action: data.RevokeKey, OperationID: onboardingID(t), TargetID: old.ID(), ExpectedRevision: 2})
	if err != nil {
		t.Fatal("manager could not reduce access after plan hold", err)
	}
	if _, err := flags.Set(ctx, fx.scope, capability.Actor{AccountID: fx.actor.AccountID}, capability.Change{
		OperationID: onboardingID(t), Capability: "data", ExpectedRevision: 1, Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := access.WithAuthorizedKey(ctx, fx.scope, old, domain.ReadRegional, live); !errors.Is(err, domain.ErrKeyDenied) {
		t.Fatal("revoked old bearer was admitted", err)
	}
	if err := access.WithAuthorizedKey(ctx, fx.scope, rotated.Bearer, domain.ReadRegional, live); err != nil {
		t.Fatal("revocation affected a distinct key", err)
	}
	firstPage, err := store.ListKeys(ctx, fx.scope, fx.actor, fx.member, "", 1)
	if err != nil || len(firstPage.Keys) != 1 || firstPage.AfterID == "" {
		t.Fatalf("unexpected first key page: %+v %v", firstPage, err)
	}
	lastPage, err := store.ListKeys(ctx, fx.scope, fx.actor, fx.member, firstPage.AfterID, 1)
	if err != nil || len(lastPage.Keys) != 1 || lastPage.AfterID != "" ||
		lastPage.Keys[0].ID == firstPage.Keys[0].ID {
		t.Fatalf("unexpected last key page: %+v %v", lastPage, err)
	}
	var retained, events int
	var fingerprint string
	if err := fx.f.Admin.QueryRow(ctx, `SELECT verifier,
(SELECT count(*) FROM org_data_api_keys WHERE org_id=$1),
(SELECT count(*) FROM org_audit_events WHERE org_id=$1 AND action LIKE 'data.key.%')
FROM org_data_api_keys WHERE org_id=$1 AND id=$2`, org, old.ID()).Scan(&fingerprint, &retained, &events); err != nil ||
		len(fingerprint) != 64 || strings.Contains(old.Reveal(), fingerprint) || retained != 2 || events != 3 {
		t.Fatalf("key lifecycle did not retain two verifier rows and three events: retained=%d events=%d err=%v", retained, events, err)
	}
}
