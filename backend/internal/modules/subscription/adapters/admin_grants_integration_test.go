//go:build integration

package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"testing"
	"time"

	subapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/subscription/application"
	subdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/subscription/domain"
)

// TestAdminGrant_ENT09_ENT10_PaymentsUnconfiguredAndTiers proves every PRO capability and
// the status projection work from an admin grant with no payment ports.
func TestAdminGrant_ENT09_ENT10_PaymentsUnconfiguredAndTiers(t *testing.T) {
	pool := freshDB(t)
	store := NewStore(pool)
	owner := seedOwner(t, pool)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `INSERT INTO entitlement_grants
		(id,account_id,tier,policy_version,state,start_at,end_at,reason)
		VALUES($1,$2,'PRO',1,'ACTIVE',clock_timestamp()-interval '1 minute',NULL,'synthetic admin grant')`,
		newid(t), owner.AccountID); err != nil {
		t.Fatal(err)
	}
	for _, capability := range subdomain.PremiumCapabilities() {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if err := store.RequireCapability(ctx, tx, owner.AccountID, capability); err != nil {
			tx.Rollback(ctx)
			t.Fatalf("capability %s denied: %v", capability, err)
		}
		tx.Rollback(ctx)
	}
	service := &subapp.Service{Store: store}
	if service.Configured() {
		t.Fatal("payment ports unexpectedly configured")
	}
	status, err := service.Status(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	if status.State != subdomain.Active || status.Reason != "verified-admin" || status.PaymentsConfigured {
		t.Fatalf("status=%+v", status)
	}
	for _, capability := range subdomain.PremiumCapabilities() {
		if !slices.Contains(status.Capabilities, capability) {
			t.Fatalf("status misses %s: %v", capability, status.Capabilities)
		}
	}
	found := false
	for _, tier := range status.Tiers {
		if tier.Tier == "PRO" && tier.Source == "admin" && tier.ValidUntil == nil {
			found = true
		}
	}
	if !found {
		t.Fatalf("admin tier missing: %+v", status.Tiers)
	}
	raw, err := json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]any
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatal(err)
	}
	if _, ok := wire["tiers"]; !ok {
		t.Fatal("status JSON has no tiers")
	}
}

// TestAdminGrant_ENT06_ExpiryBoundary uses the injected read clock: the exact
// end instant is inactive and one nanosecond before it is active.
func TestAdminGrant_ENT06_ExpiryBoundary(t *testing.T) {
	pool := freshDB(t)
	owner := seedOwner(t, pool)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)
	end := now.Add(time.Hour)
	if _, err := pool.Exec(ctx, `INSERT INTO entitlement_grants
		(id,account_id,tier,policy_version,state,start_at,end_at,reason)
		VALUES($1,$2,'PRO',1,'ACTIVE',$3,$4,'synthetic boundary grant')`,
		newid(t), owner.AccountID, now.Add(-time.Minute), end); err != nil {
		t.Fatal(err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	caps, _, _, err := adminGrants(ctx, tx, owner.AccountID, end.Add(-time.Nanosecond))
	if err != nil || !slices.Contains(caps, "cloud.sync") {
		t.Fatalf("before boundary caps=%v err=%v", caps, err)
	}
	caps, _, _, err = adminGrants(ctx, tx, owner.AccountID, end)
	if err != nil || len(caps) != 0 {
		t.Fatalf("at boundary caps=%v err=%v", caps, err)
	}
}

// TestAdminGrant_ENT07_RevokeThenCheckDenies proves a committed revoke is
// immediately denied on the next capability check.
func TestAdminGrant_ENT07_RevokeThenCheckDenies(t *testing.T) {
	pool := freshDB(t)
	store := NewStore(pool)
	owner := seedOwner(t, pool)
	ctx := context.Background()
	grantID := newid(t)
	if _, err := pool.Exec(ctx, `INSERT INTO entitlement_grants
		(id,account_id,tier,policy_version,state,start_at,end_at,reason)
		VALUES($1,$2,'PRO',1,'ACTIVE',clock_timestamp()-interval '1 minute',NULL,'synthetic revoke grant')`,
		grantID, owner.AccountID); err != nil {
		t.Fatal(err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.RequireCapability(ctx, tx, owner.AccountID, "cloud.sync"); err != nil {
		t.Fatalf("active grant denied: %v", err)
	}
	tx.Rollback(ctx)
	if _, err := pool.Exec(ctx, `UPDATE entitlement_grants SET state='REVOKED',revoked_at=clock_timestamp(),revoke_reason='synthetic revoke' WHERE id=$1`, grantID); err != nil {
		t.Fatal(err)
	}
	tx, err = pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if err := store.RequireCapability(ctx, tx, owner.AccountID, "cloud.sync"); !errors.Is(err, subdomain.ErrDenied) {
		t.Fatalf("revoked grant err=%v", err)
	}
}

// TestAdminGrant_ENT10_UnconfiguredPaymentsOnlyAdminUnlocks keeps the pre-A2
// payment gate: with payment ports unconfigured a verified Play row unlocks
// nothing, while an admin grant still does (ENT-10).
func TestAdminGrant_ENT10_UnconfiguredPaymentsOnlyAdminUnlocks(t *testing.T) {
	pool := freshDB(t)
	store := NewStore(pool)
	ctx := context.Background()
	paid := seedOwner(t, pool)
	if _, err := pool.Exec(ctx, `INSERT INTO subscription_purchases(token_digest,account_id,environment,package_name,product_id,base_plan_id,state,valid_until,verified_at)
		VALUES (md5('ent10')||md5('ent10-synthetic'),$1,'test','com.anpfuel','synthetic.premium','monthly','ACTIVE',clock_timestamp()+interval '1 hour',clock_timestamp())`, paid.AccountID); err != nil {
		t.Fatal(err)
	}
	granted := seedOwner(t, pool)
	if _, err := pool.Exec(ctx, `INSERT INTO entitlement_grants
		(id,account_id,tier,policy_version,state,start_at,end_at,reason)
		VALUES($1,$2,'PRO',1,'ACTIVE',clock_timestamp()-interval '1 minute',NULL,'synthetic admin grant')`,
		newid(t), granted.AccountID); err != nil {
		t.Fatal(err)
	}
	check := func(owner string, adminOnly bool) error {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		if adminOnly {
			return store.RequireAdminCapability(ctx, tx, owner, "cloud.sync")
		}
		return store.RequireCapability(ctx, tx, owner, "cloud.sync")
	}
	if err := check(paid.AccountID, false); err != nil {
		t.Fatalf("configured payments deny the paid row: %v", err)
	}
	if err := check(paid.AccountID, true); !errors.Is(err, subdomain.ErrDenied) {
		t.Fatalf("unconfigured payments unlock the paid row: %v", err)
	}
	if err := check(granted.AccountID, true); err != nil {
		t.Fatalf("unconfigured payments deny the admin grant: %v", err)
	}
}
