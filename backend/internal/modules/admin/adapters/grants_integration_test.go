//go:build integration

package adapters

import (
	"context"
	"errors"
	"testing"
	"time"

	accountdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/admin/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/admin/domain"
	entadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/entitlement/adapters"
	entdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/entitlement/domain"
)

func grantService(t *testing.T) (*Store, *app.Service, app.Session, string) {
	t.Helper()
	pool := freshDB(t)
	store := NewStore(pool)
	now := time.Now().UTC().Truncate(time.Microsecond)
	key := syntheticKey('u')
	result, err := store.Login(context.Background(), acceptedStoreInput(key, now, "198.51.100.80"))
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.Authenticate(context.Background(), result.Token, now)
	if err != nil {
		t.Fatal(err)
	}
	service := &app.Service{
		Store: store, Grants: entadapters.NewStore(pool),
		Clock: func() time.Time { return now }, NewID: newUUID,
	}
	accountID := newUUIDForTest(t)
	if _, err := pool.Exec(context.Background(), "INSERT INTO accounts(id,alias,status) VALUES($1,$2,'active')", accountID, "synthetic-"+accountID); err != nil {
		t.Fatal(err)
	}
	return store, service, session, accountID
}

func newUUIDForTest(t *testing.T) string {
	t.Helper()
	id, err := newUUID()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func grantInput(accountID, tier, reason, key string) app.CreateGrantInput {
	return app.CreateGrantInput{
		AccountID: accountID, Tier: tier, Reason: reason, Key: key,
		RequestHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		RequestID:   "synthetic-request", ClientIPHash: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
	}
}

// TestGrant_ENT02_ENT03_ReplaceAndAudit covers duplicate refusal, replace
// atomicity and the two audit rows.
func TestGrant_ENT02_ENT03_ReplaceAndAudit(t *testing.T) {
	store, service, session, accountID := grantService(t)
	ctx := context.Background()
	first, err := service.CreateGrant(ctx, session, grantInput(accountID, "PRO", "synthetic first grant", "11111111-1111-4111-8111-111111111111"))
	if err != nil || first.Grant.Tier != "PRO" || first.Replaced != nil {
		t.Fatalf("first=%+v err=%v", first, err)
	}
	if _, err := service.CreateGrant(ctx, session, grantInput(accountID, "PRO", "synthetic duplicate grant", "22222222-2222-4222-8222-222222222222")); !errors.Is(err, entdomain.ErrExists) {
		t.Fatalf("duplicate err=%v", err)
	}
	replace := grantInput(accountID, "PRO", "synthetic replaced grant", "33333333-3333-4333-8333-333333333333")
	replace.Replace = true
	second, err := service.CreateGrant(ctx, session, replace)
	if err != nil || second.Replaced == nil || second.Replaced.State != "REVOKED" || second.Grant.ID == first.Grant.ID {
		t.Fatalf("replace=%+v err=%v", second, err)
	}
	var audits int
	if err := store.Pool.QueryRow(ctx, "SELECT count(*) FROM admin_audit_events WHERE action IN ('admin.grant.create','admin.grant.revoke')").Scan(&audits); err != nil || audits != 3 {
		t.Fatalf("audits=%d err=%v", audits, err)
	}
	var active int
	if err := store.Pool.QueryRow(ctx, "SELECT count(*) FROM entitlement_grants WHERE account_id=$1 AND state='ACTIVE'", accountID).Scan(&active); err != nil || active != 1 {
		t.Fatalf("active=%d err=%v", active, err)
	}
}

// TestGrant_ENT02_AdminIneligible covers suspended/deleted targets through
// the admin application.
func TestGrant_ENT02_AdminIneligible(t *testing.T) {
	store, service, session, accountID := grantService(t)
	ctx := context.Background()
	for _, status := range []string{"suspended", "deleted"} {
		if _, err := store.Pool.Exec(ctx, "UPDATE accounts SET status=$1 WHERE id=$2", status, accountID); err != nil {
			t.Fatal(err)
		}
		_, err := service.CreateGrant(ctx, session, grantInput(accountID, "PRO", "synthetic ineligible grant", "44444444-4444-4444-8444-444444444444"))
		if !errors.Is(err, entdomain.ErrIneligible) {
			t.Fatalf("%s err=%v", status, err)
		}
	}
}

// TestGrant_ENT07_RevokeAgainConflicts covers the terminal revoke state.
func TestGrant_ENT07_RevokeAgainConflicts(t *testing.T) {
	_, service, session, accountID := grantService(t)
	ctx := context.Background()
	created, err := service.CreateGrant(ctx, session, grantInput(accountID, "PRO", "synthetic revoke grant", "55555555-5555-4555-8555-555555555555"))
	if err != nil {
		t.Fatal(err)
	}
	input := app.RevokeGrantInput{
		GrantID: created.Grant.ID, Reason: "synthetic revoke reason", Key: "66666666-6666-4666-8666-666666666666",
		RequestHash: "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
	}
	if _, _, err := service.RevokeGrant(ctx, session, input); err != nil {
		t.Fatal(err)
	}
	input.Key = "77777777-7777-4777-8777-777777777777"
	if _, _, err := service.RevokeGrant(ctx, session, input); !errors.Is(err, entdomain.ErrAlreadyRevoked) {
		t.Fatalf("second revoke err=%v", err)
	}
}

// TestGrant_ENT04_EndAtBounds covers the application validation.
func TestGrant_ENT04_EndAtBounds(t *testing.T) {
	_, service, session, accountID := grantService(t)
	tooSoon := time.Now().Add(30 * time.Minute)
	input := grantInput(accountID, "PRO", "synthetic short grant", "88888888-8888-4888-8888-888888888888")
	input.EndAt = &tooSoon
	if _, err := service.CreateGrant(context.Background(), session, input); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("short end_at err=%v", err)
	}
}

// TestSearchAccounts_BUCADM02 covers exact/prefix/email lookups and
// injection-like input.
func TestSearchAccounts_BUCADM02(t *testing.T) {
	pool := freshDB(t)
	store := NewStore(pool)
	ctx := context.Background()
	first, second := newUUIDForTest(t), newUUIDForTest(t)
	if _, err := pool.Exec(ctx, "INSERT INTO accounts(id,alias,status) VALUES($1,'syntheticalpha','active'),($2,'syntheticbeta','active')", first, second); err != nil {
		t.Fatal(err)
	}
	email := "synthetic" + "@example.invalid"
	hash := accountdomain.AddressHash(email)
	if _, err := pool.Exec(ctx, "INSERT INTO account_addresses(address_hash,account_id) VALUES($1,$2)", hash, first); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		query string
		want  int
	}{
		{first, 1}, {"syntheticalpha", 1}, {"syn", 2}, {email, 1},
		{"' OR 1=1 --", 0}, {"%", 0}, {"_", 0},
	}
	for _, tc := range cases {
		got, err := store.SearchAccounts(ctx, tc.query)
		if err != nil {
			t.Fatalf("%q err=%v", tc.query, err)
		}
		if len(got) != tc.want {
			t.Fatalf("%q returned %d want %d: %+v", tc.query, len(got), tc.want, got)
		}
	}
}
