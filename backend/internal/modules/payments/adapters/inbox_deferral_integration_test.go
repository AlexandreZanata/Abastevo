//go:build integration

package adapters

import (
	"errors"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/domain"
)

func TestPaymentInboxDeferralOwnsOneWakeupAndRollsBackFailedScheduling(t *testing.T) {
	pool := freshDB(t)
	ctx := t.Context()
	store := NewStore(pool)
	event := InboxInput{Provider: domain.ProviderStripe, Environment: domain.EnvironmentTest, Merchant: "merchant_test",
		EventID: "synthetic-advance-term", PayloadHash: payloadHash("synthetic paid invoice"), EventType: "invoice.paid",
		ResourceType: "invoice", ResourceID: "in_synthetic"}
	if _, err := store.RecordInbox(ctx, event); err != nil {
		t.Fatal(err)
	}
	ordinary := event
	ordinary.EventID, ordinary.ResourceID, ordinary.PayloadHash = "synthetic-ordinary", "in_ordinary", payloadHash("ordinary test invoice")
	if _, err := store.RecordInbox(ctx, ordinary); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `CREATE FUNCTION synthetic_deferred_enqueue_failure() RETURNS trigger
LANGUAGE plpgsql AS $$BEGIN
IF NEW.dedupe_key LIKE 'payment-inbox-deferred:%' THEN RAISE EXCEPTION 'synthetic scheduling failure'; END IF;
RETURN NEW;
END $$;
CREATE TRIGGER synthetic_deferred_enqueue_failure BEFORE INSERT ON job_queue
FOR EACH ROW EXECUTE FUNCTION synthetic_deferred_enqueue_failure();`); err != nil {
		t.Fatal(err)
	}
	until := time.Now().UTC().Truncate(time.Microsecond).Add(5 * time.Second)
	if err := store.DeferInbox(ctx, event, until); err == nil {
		t.Fatal("a failed wake-up silently deferred the original event")
	}
	if count(t, pool, "SELECT count(*) FROM payment_inbox WHERE deferred_until IS NOT NULL") != 0 {
		t.Fatal("deferral survived the failed enqueue transaction")
	}
	if _, err := pool.Exec(ctx, "DROP TRIGGER synthetic_deferred_enqueue_failure ON job_queue"); err != nil {
		t.Fatal(err)
	}
	mismatched := event
	mismatched.PayloadHash = payloadHash("different canonical proof")
	if err := store.DeferInbox(ctx, mismatched, until); !errors.Is(err, domain.ErrStale) {
		t.Fatalf("divergent event changed the schedule: %v", err)
	}
	results := make(chan error, 2)
	for range 2 {
		go func() { results <- store.DeferInbox(ctx, event, until) }()
	}
	for range 2 {
		if err := <-results; err != nil {
			t.Fatal("matching schedule did not converge", err)
		}
	}
	if err := store.DeferInbox(ctx, event, until.Add(time.Hour)); !errors.Is(err, domain.ErrStale) {
		t.Fatalf("a retry moved the approved boundary: %v", err)
	}
	if count(t, pool, "SELECT count(*) FROM job_queue WHERE dedupe_key LIKE 'payment-inbox-deferred:%'") != 1 {
		t.Fatal("one event must schedule exactly one future wake-up")
	}
	var scheduled time.Time
	if err := pool.QueryRow(ctx, "SELECT not_before FROM job_queue WHERE dedupe_key LIKE 'payment-inbox-deferred:%'").Scan(&scheduled); err != nil || !scheduled.Equal(until) {
		t.Fatalf("job and inbox do not share the approved boundary: %v %v", scheduled, err)
	}
	pending, err := store.PendingInbox(ctx, event.Provider, event.Environment, event.Merchant, 10)
	if err != nil || len(pending) != 1 || pending[0].EventID != ordinary.EventID {
		t.Fatalf("future charge blocked ordinary inbox processing or ran early: %+v %v", pending, err)
	}
	if err := store.MarkInbox(ctx, event, "PROCESSED", ""); !errors.Is(err, domain.ErrStale) {
		t.Fatal("advance event became terminal before its term", err)
	}
	timer := time.NewTimer(time.Until(until.Add(50 * time.Millisecond)))
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	pending, err = store.PendingInbox(ctx, event.Provider, event.Environment, event.Merchant, 10)
	if err != nil || len(pending) != 2 {
		t.Fatalf("term boundary did not release the original event: %+v %v", pending, err)
	}
	if err := store.MarkInbox(ctx, event, "PROCESSED", ""); err != nil {
		t.Fatal("eligible event cannot finish", err)
	}
	if count(t, pool, "SELECT count(*) FROM payment_inbox WHERE state='PROCESSED' AND deferred_until IS NULL") != 1 {
		t.Fatal("terminal processing did not clear the schedule")
	}
}
