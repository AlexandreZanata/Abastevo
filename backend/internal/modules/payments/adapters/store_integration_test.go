//go:build integration

package adapters

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	migrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func freshDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := createDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if _, err := migrate.Apply(ctx, dsn, migrations.Files); err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func createDB(t *testing.T) string {
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
	name := fmt.Sprintf("payments_test_%d", time.Now().UnixNano())
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
	return u.String()
}

func seedOwner(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	id, err := jobs.NewUUIDv4()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(context.Background(),
		"INSERT INTO accounts(id,alias,status) VALUES($1,$2,'active')", id, "synthetic-"+id); err != nil {
		t.Fatal(err)
	}
	return id
}

func testOffer() domain.Offer {
	return domain.Offer{
		ID: "premium.monthly", Family: "premium", Term: domain.TermMonthly,
		Price: domain.BRL(1234), Capabilities: domain.PremiumCapabilityList(),
		Channels: map[domain.Channel]bool{
			domain.ChannelPixManual: true, domain.ChannelCard: true, domain.ChannelPixAutomatic: true,
		},
		SaleEnabled: true,
	}
}

func newOrder(t *testing.T, owner string) domain.Order {
	t.Helper()
	catalog, err := domain.NewCatalog("pay-catalog-v1", testOffer())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	quote, err := catalog.Quote(domain.QuoteRequest{
		OfferID: testOffer().ID, Channel: domain.ChannelPixManual,
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	order, err := domain.NewOrder(testID(t), owner, quote, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := order.MarkPending(now); err != nil {
		t.Fatal(err)
	}
	return order
}

func testID(t *testing.T) string {
	t.Helper()
	id, err := jobs.NewUUIDv4()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func payloadHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func claimInput(order domain.Order, key string) ClaimAttemptInput {
	return ClaimAttemptInput{
		AccountID: order.AccountID, Provider: domain.ProviderAsaas, Environment: domain.EnvironmentTest,
		Merchant: "merchant_1", OperationKey: key, Kind: "create_attempt",
		PayloadHash: payloadHash(key), OrderID: order.ID, LeaseUntil: time.Now().UTC().Add(time.Minute),
	}
}

func settleInput(order domain.Order, claim Claim, settlementID, remoteID string) SettlementInput {
	return SettlementInput{
		Offer: testOffer(),
		Settlement: domain.Settlement{
			ID: settlementID, OrderID: order.ID, AttemptID: claim.AttemptID,
			Provider: domain.ProviderAsaas, Environment: domain.EnvironmentTest,
			Channel: domain.ChannelPixManual, MerchantID: "merchant_1",
			RemotePaymentID: remoteID, ProviderStatus: "RECEIVED",
			SettledAt: time.Now().UTC().Truncate(time.Microsecond),
		},
	}
}

func count(t *testing.T, pool *pgxpool.Pool, query string, args ...any) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestOrderAndCustomerIdempotency(t *testing.T) {
	pool := freshDB(t)
	store := NewStore(pool)
	owner := seedOwner(t, pool)
	order := newOrder(t, owner)
	ctx := context.Background()

	if err := store.CreateOrder(ctx, order); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateOrder(ctx, order); err != nil {
		t.Fatal(err)
	}
	changed := order
	changed.Amount = domain.BRL(999)
	if err := store.CreateOrder(ctx, changed); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("changed order err=%v", err)
	}

	mapping := CustomerMapping{
		AccountID: owner, Provider: domain.ProviderAsaas, Environment: domain.EnvironmentTest,
		Merchant: "merchant_1", RemoteCustomerID: "cus_1",
	}
	if err := store.MapCustomer(ctx, mapping); err != nil {
		t.Fatal(err)
	}
	if err := store.MapCustomer(ctx, mapping); err != nil {
		t.Fatal(err)
	}
	mapping.RemoteCustomerID = "cus_2"
	if err := store.MapCustomer(ctx, mapping); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("changed customer err=%v", err)
	}

	cycle := CycleInput{
		ID: testID(t), AccountID: owner, OfferID: order.OfferID, Source: domain.ChannelPixManual,
		PeriodStart: order.QuotedAt, PeriodEnd: order.QuotedAt.Add(30 * 24 * time.Hour), State: "PENDING",
	}
	if err := store.CreateCycle(ctx, cycle); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateCycle(ctx, cycle); err != nil {
		t.Fatal(err)
	}
	cycle.PeriodEnd = cycle.PeriodEnd.Add(time.Hour)
	if err := store.CreateCycle(ctx, cycle); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("changed cycle err=%v", err)
	}
}

func TestClaimAttemptLeaseConcurrencyAndChangedPayload(t *testing.T) {
	pool := freshDB(t)
	store := NewStore(pool)
	owner := seedOwner(t, pool)
	order := newOrder(t, owner)
	if err := store.CreateOrder(context.Background(), order); err != nil {
		t.Fatal(err)
	}
	input := claimInput(order, "op-1")
	const n = 12
	claims := make([]Claim, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			claims[i], errs[i] = store.ClaimAttempt(context.Background(), input)
		}(i)
	}
	wg.Wait()
	var winner Claim
	wins := 0
	for i, err := range errs {
		if err == nil {
			wins++
			winner = claims[i]
			continue
		}
		if !errors.Is(err, domain.ErrBusy) {
			t.Fatalf("unexpected claim error: %v", err)
		}
	}
	if wins != 1 {
		t.Fatalf("wins = %d", wins)
	}
	if winner.Generation != 1 || winner.AttemptNo != 1 {
		t.Fatalf("winner = %+v", winner)
	}
	if count(t, pool, "SELECT count(*) FROM payment_operations") != 1 ||
		count(t, pool, "SELECT count(*) FROM payment_attempts") != 1 {
		t.Fatal("concurrent claim duplicated local operation")
	}

	changed := input
	changed.PayloadHash = payloadHash("different")
	if _, err := store.ClaimAttempt(context.Background(), changed); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("changed payload err=%v", err)
	}

	if _, err := pool.Exec(context.Background(),
		"UPDATE payment_operations SET lease_until=clock_timestamp()-interval '1 second' WHERE id=$1",
		winner.OperationID); err != nil {
		t.Fatal(err)
	}
	newClaim, err := store.ClaimAttempt(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if newClaim.Generation != 2 || newClaim.AttemptNo != 2 {
		t.Fatalf("takeover = %+v", newClaim)
	}
	if err := store.MarkDispatched(context.Background(), winner); !errors.Is(err, domain.ErrStale) {
		t.Fatalf("stale dispatch err=%v", err)
	}
	if err := store.MarkDispatched(context.Background(), newClaim); err != nil {
		t.Fatal(err)
	}
}

func TestWebhookBeforeReplySettlesOnce(t *testing.T) {
	pool := freshDB(t)
	store := NewStore(pool)
	owner := seedOwner(t, pool)
	order := newOrder(t, owner)
	ctx := context.Background()
	if err := store.CreateOrder(ctx, order); err != nil {
		t.Fatal(err)
	}
	claim, err := store.ClaimAttempt(ctx, claimInput(order, "op-settle"))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.MarkDispatched(ctx, claim); err != nil {
		t.Fatal(err)
	}
	input := settleInput(order, claim, testID(t), "remote-settle-1")
	outcome, err := store.RecordSettlement(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Replay || outcome.Excess || outcome.GrantID == "" {
		t.Fatalf("outcome = %+v", outcome)
	}
	replay, err := store.RecordSettlement(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if !replay.Replay || replay.GrantID != outcome.GrantID {
		t.Fatalf("replay = %+v", replay)
	}
	if count(t, pool, "SELECT count(*) FROM payment_settlements") != 1 ||
		count(t, pool, "SELECT count(*) FROM payment_grants") != 1 {
		t.Fatal("settlement duplicated local economic effect")
	}
	if count(t, pool, "SELECT count(*) FROM job_queue WHERE kind='payment-settlement'") != 1 {
		t.Fatal("outbox not converged")
	}
	var orderState, attemptState, operationState string
	if err := pool.QueryRow(ctx, "SELECT state FROM payment_orders WHERE id=$1", order.ID).Scan(&orderState); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, "SELECT state FROM payment_attempts WHERE id=$1", claim.AttemptID).Scan(&attemptState); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, "SELECT state FROM payment_operations WHERE id=$1", claim.OperationID).Scan(&operationState); err != nil {
		t.Fatal(err)
	}
	if orderState != "PAID" || attemptState != "SUCCEEDED" || operationState != "SUCCEEDED" {
		t.Fatalf("states order=%s attempt=%s operation=%s", orderState, attemptState, operationState)
	}
}

func TestAmbiguousOutcomePersistsAndRecovers(t *testing.T) {
	pool := freshDB(t)
	store := NewStore(pool)
	owner := seedOwner(t, pool)
	order := newOrder(t, owner)
	ctx := context.Background()
	if err := store.CreateOrder(ctx, order); err != nil {
		t.Fatal(err)
	}
	claim, err := store.ClaimAttempt(ctx, claimInput(order, "op-unknown"))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.MarkDispatched(ctx, claim); err != nil {
		t.Fatal(err)
	}
	if err := store.MarkOutcome(ctx, claim, domain.AttemptUnknown, ""); err != nil {
		t.Fatal(err)
	}
	var attemptState, operationState string
	if err := pool.QueryRow(ctx, "SELECT state FROM payment_attempts WHERE id=$1", claim.AttemptID).Scan(&attemptState); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, "SELECT state FROM payment_operations WHERE id=$1", claim.OperationID).Scan(&operationState); err != nil {
		t.Fatal(err)
	}
	if attemptState != "UNKNOWN" || operationState != "OUTCOME_UNKNOWN" {
		t.Fatalf("states attempt=%s operation=%s", attemptState, operationState)
	}
	if _, err := store.ClaimAttempt(ctx, claimInput(order, "op-unknown")); !errors.Is(err, domain.ErrBusy) {
		t.Fatalf("live unknown lease claim err=%v", err)
	}
	if _, err := pool.Exec(ctx,
		"UPDATE payment_operations SET lease_until=clock_timestamp()-interval '1 second' WHERE id=$1",
		claim.OperationID); err != nil {
		t.Fatal(err)
	}
	recovered, err := store.ClaimAttempt(ctx, claimInput(order, "op-unknown"))
	if err != nil {
		t.Fatal(err)
	}
	if recovered.Generation != 2 || recovered.AttemptNo != 2 {
		t.Fatalf("recovered = %+v", recovered)
	}
	if err := store.MarkDispatched(ctx, recovered); err != nil {
		t.Fatal(err)
	}
	if err := store.MarkOutcome(ctx, recovered, domain.AttemptFailed, ""); err != nil {
		t.Fatal(err)
	}
	if err := store.MarkOutcome(ctx, claim, domain.AttemptSucceeded, "late"); !errors.Is(err, domain.ErrStale) {
		t.Fatalf("stale outcome err=%v", err)
	}
}

func TestInboxAndOutboxIdempotency(t *testing.T) {
	pool := freshDB(t)
	store := NewStore(pool)
	ctx := context.Background()
	input := InboxInput{
		Provider: domain.ProviderAsaas, Environment: domain.EnvironmentTest, Merchant: "merchant_1",
		EventID: "evt_1", PayloadHash: payloadHash("event-1"), EventType: "PAYMENT_RECEIVED",
		ResourceType: "payment", ResourceID: "pay_1",
	}
	first, err := store.RecordInbox(ctx, input)
	if err != nil || !first.Created {
		t.Fatalf("first inbox = %+v err=%v", first, err)
	}
	second, err := store.RecordInbox(ctx, input)
	if err != nil || second.Created {
		t.Fatalf("second inbox = %+v err=%v", second, err)
	}
	if count(t, pool, "SELECT count(*) FROM job_queue WHERE kind='payment-inbox'") != 1 {
		t.Fatal("inbox outbox not converged")
	}
	changed := input
	changed.PayloadHash = payloadHash("event-2")
	if _, err := store.RecordInbox(ctx, changed); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("changed event err=%v", err)
	}
	if err := store.MarkInbox(ctx, input, "PROCESSED", ""); err != nil {
		t.Fatal(err)
	}
	if err := store.MarkInbox(ctx, input, "PROCESSED", ""); !errors.Is(err, domain.ErrStale) {
		t.Fatalf("reprocessed inbox err=%v", err)
	}
}

func TestSettlementRollsBackOnGrantFailure(t *testing.T) {
	pool := freshDB(t)
	store := NewStore(pool)
	owner := seedOwner(t, pool)
	order := newOrder(t, owner)
	ctx := context.Background()
	if err := store.CreateOrder(ctx, order); err != nil {
		t.Fatal(err)
	}
	claim, err := store.ClaimAttempt(ctx, claimInput(order, "op-rollback"))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.MarkDispatched(ctx, claim); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `CREATE FUNCTION payment_test_fail_grant() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'forced grant failure'; END $$`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `CREATE TRIGGER payment_test_fail_grant BEFORE INSERT ON payment_grants
FOR EACH ROW EXECUTE FUNCTION payment_test_fail_grant()`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DROP TRIGGER IF EXISTS payment_test_fail_grant ON payment_grants")
		_, _ = pool.Exec(context.Background(), "DROP FUNCTION IF EXISTS payment_test_fail_grant()")
	})
	input := settleInput(order, claim, testID(t), "remote-rollback")
	if _, err := store.RecordSettlement(ctx, input); err == nil {
		t.Fatal("forced grant failure was not returned")
	}
	if count(t, pool, "SELECT count(*) FROM payment_settlements") != 0 ||
		count(t, pool, "SELECT count(*) FROM payment_grants") != 0 ||
		count(t, pool, "SELECT count(*) FROM job_queue WHERE kind='payment-settlement'") != 0 {
		t.Fatal("failed settlement left partial rows")
	}
	var orderState, attemptState string
	if err := pool.QueryRow(ctx, "SELECT state FROM payment_orders WHERE id=$1", order.ID).Scan(&orderState); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, "SELECT state FROM payment_attempts WHERE id=$1", claim.AttemptID).Scan(&attemptState); err != nil {
		t.Fatal(err)
	}
	if orderState != "PENDING" || attemptState != "DISPATCHED" {
		t.Fatalf("rollback states order=%s attempt=%s", orderState, attemptState)
	}
	if _, err := pool.Exec(ctx, "DROP TRIGGER payment_test_fail_grant ON payment_grants"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "DROP FUNCTION payment_test_fail_grant()"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.RecordSettlement(ctx, input); err != nil {
		t.Fatal(err)
	}
}

func TestMigrationUpgradeReplayAndPostGIS(t *testing.T) {
	dsn := createDB(t)
	before, target := fstest.MapFS{}, fstest.MapFS{}
	names, err := fs.Glob(migrations.Files, "*.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		raw, err := fs.ReadFile(migrations.Files, name)
		if err != nil {
			t.Fatal(err)
		}
		target[name] = &fstest.MapFile{Data: raw}
		if name < "000123_payment_intents.sql" {
			before[name] = &fstest.MapFile{Data: raw}
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if _, err := migrate.Apply(ctx, dsn, before); err != nil {
		t.Fatal(err)
	}
	applied, err := migrate.Apply(ctx, dsn, target)
	if err != nil || len(applied) != 5 || applied[0] != "000123" || applied[1] != "000124" ||
		applied[2] != "000125" || applied[3] != "000126" || applied[4] != "000127" {
		t.Fatalf("upgrade applied=%v err=%v", applied, err)
	}
	again, err := migrate.Apply(ctx, dsn, target)
	if err != nil || len(again) != 0 {
		t.Fatalf("replay applied=%v err=%v", again, err)
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var postgis string
	if err := pool.QueryRow(ctx, "SELECT PostGIS_version()").Scan(&postgis); err != nil || postgis == "" {
		t.Fatalf("postgis=%q err=%v", postgis, err)
	}
	var exists bool
	if err := pool.QueryRow(ctx, "SELECT to_regclass('payment_grants') IS NOT NULL").Scan(&exists); err != nil || !exists {
		t.Fatalf("payment_grants exists=%v err=%v", exists, err)
	}
	if err := pool.QueryRow(ctx, "SELECT to_regclass('payment_pix_auto_instructions') IS NOT NULL").Scan(&exists); err != nil || !exists {
		t.Fatalf("instruction table exists=%v err=%v", exists, err)
	}
	if err := pool.QueryRow(ctx, "SELECT to_regclass('payment_pix_auto_eligibility') IS NOT NULL").Scan(&exists); err != nil || !exists {
		t.Fatalf("eligibility table exists=%v err=%v", exists, err)
	}
	if err := pool.QueryRow(ctx, "SELECT to_regclass('payment_source_switches') IS NOT NULL").Scan(&exists); err != nil || !exists {
		t.Fatalf("source switch table exists=%v err=%v", exists, err)
	}
}

func TestConcurrentSettlementSingleGrant(t *testing.T) {
	pool := freshDB(t)
	store := NewStore(pool)
	owner := seedOwner(t, pool)
	order := newOrder(t, owner)
	ctx := context.Background()
	if err := store.CreateOrder(ctx, order); err != nil {
		t.Fatal(err)
	}
	claim, err := store.ClaimAttempt(ctx, claimInput(order, "op-concurrent-settle"))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.MarkDispatched(ctx, claim); err != nil {
		t.Fatal(err)
	}
	input := settleInput(order, claim, testID(t), "remote-concurrent-settle")
	const n = 8
	outcomes := make([]SettlementOutcome, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			outcomes[i], errs[i] = store.RecordSettlement(context.Background(), input)
		}(i)
	}
	wg.Wait()
	granted := 0
	for i, err := range errs {
		if err != nil {
			t.Fatalf("settlement %d err=%v", i, err)
		}
		if !outcomes[i].Replay && outcomes[i].GrantID != "" && !outcomes[i].Excess {
			granted++
		}
	}
	if granted != 1 {
		t.Fatalf("granted outcomes = %d", granted)
	}
	if count(t, pool, "SELECT count(*) FROM payment_settlements") != 1 ||
		count(t, pool, "SELECT count(*) FROM payment_grants") != 1 ||
		count(t, pool, "SELECT count(*) FROM job_queue WHERE kind='payment-settlement'") != 1 {
		t.Fatal("concurrent settlement duplicated local effect")
	}
}

func TestSameOrderExcessIsRecordedWithoutSecondGrant(t *testing.T) {
	pool := freshDB(t)
	store := NewStore(pool)
	owner := seedOwner(t, pool)
	order := newOrder(t, owner)
	ctx := context.Background()
	if err := store.CreateOrder(ctx, order); err != nil {
		t.Fatal(err)
	}
	claim, err := store.ClaimAttempt(ctx, claimInput(order, "op-same-order-excess"))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.MarkDispatched(ctx, claim); err != nil {
		t.Fatal(err)
	}
	first := settleInput(order, claim, testID(t), "remote-same-order-1")
	if _, err := store.RecordSettlement(ctx, first); err != nil {
		t.Fatal(err)
	}
	second := settleInput(order, claim, testID(t), "remote-same-order-2")
	outcome, err := store.RecordSettlement(ctx, second)
	if err != nil {
		t.Fatal(err)
	}
	if !outcome.Excess || outcome.GrantID != "" {
		t.Fatalf("outcome = %+v", outcome)
	}
	if count(t, pool, "SELECT count(*) FROM payment_settlements") != 2 ||
		count(t, pool, "SELECT count(*) FROM payment_grants") != 1 ||
		count(t, pool, "SELECT count(*) FROM payment_settlements WHERE outcome='EXCESS'") != 1 {
		t.Fatal("same-order excess duplicated grant")
	}
}

func TestCapacityExcessIsRecordedWithoutGrant(t *testing.T) {
	pool := freshDB(t)
	store := NewStore(pool)
	owner := seedOwner(t, pool)
	ctx := context.Background()
	orders := make([]domain.Order, 3)
	claims := make([]Claim, 3)
	for i := range orders {
		orders[i] = newOrder(t, owner)
		if err := store.CreateOrder(ctx, orders[i]); err != nil {
			t.Fatal(err)
		}
		claim, err := store.ClaimAttempt(ctx, claimInput(orders[i], fmt.Sprintf("op-excess-%d", i)))
		if err != nil {
			t.Fatal(err)
		}
		if err := store.MarkDispatched(ctx, claim); err != nil {
			t.Fatal(err)
		}
		claims[i] = claim
	}
	for i := range orders {
		input := settleInput(orders[i], claims[i], testID(t), fmt.Sprintf("remote-excess-%d", i))
		outcome, err := store.RecordSettlement(ctx, input)
		if err != nil {
			t.Fatal(err)
		}
		if i < 2 {
			if outcome.Excess || outcome.GrantID == "" {
				t.Fatalf("grant %d outcome=%+v", i, outcome)
			}
			continue
		}
		if !outcome.Excess || outcome.GrantID != "" {
			t.Fatalf("excess outcome=%+v", outcome)
		}
	}
	if count(t, pool, "SELECT count(*) FROM payment_settlements") != 3 ||
		count(t, pool, "SELECT count(*) FROM payment_grants") != 2 ||
		count(t, pool, "SELECT count(*) FROM payment_settlements WHERE outcome='EXCESS'") != 1 {
		t.Fatal("excess settlement not recorded distinctly")
	}
}
