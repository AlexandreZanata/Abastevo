//go:build integration

package jobs

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"

	migrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	payadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/adapters"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func pay19FreshStore(t *testing.T) *payadapters.Store {
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
		t.Fatal(err)
	}
	name := fmt.Sprintf("payjobs_test_%d", time.Now().UnixNano())
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
			t.Error(err)
			return
		}
		defer admin.Close(ctx)
		if _, err = admin.Exec(ctx, "DROP DATABASE "+pgx.Identifier{name}.Sanitize()+" WITH (FORCE)"); err != nil {
			t.Error(err)
		}
	})
	u.Path = "/" + name
	if _, err := migrate.Apply(ctx, u.String(), migrations.Files); err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.New(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return payadapters.NewStore(pool)
}

func pay19RecordInbox(t *testing.T, store *payadapters.Store, provider domain.Provider, eventID, eventType string) {
	t.Helper()
	record := pay19Event(provider, eventID, eventType)
	result, err := store.RecordInbox(context.Background(), record.InboxEvent)
	if err != nil || !result.Created {
		t.Fatalf("record %s err=%v result=%+v", eventID, err, result)
	}
}

func pay19InboxState(t *testing.T, store *payadapters.Store, provider domain.Provider, eventID string) string {
	t.Helper()
	record, err := store.Inbox(context.Background(), provider, domain.EnvironmentTest, "merchant-test", eventID)
	if err != nil {
		t.Fatal(err)
	}
	return record.State
}

func TestPAY19DrainReconcilesRealInboxRows(t *testing.T) {
	store := pay19FreshStore(t)
	pay19RecordInbox(t, store, domain.ProviderAsaas, "evt_p19_unknown", "SYNTHETIC_UNKNOWN")
	pay19RecordInbox(t, store, domain.ProviderAsaas, "evt_p19_received", "PAYMENT_RECEIVED")

	settlement, lifecycle, dispute, cycles := &fakeProcessor{}, &fakeProcessor{}, &fakeProcessor{}, &fakeProcessor{}
	// The fake terminal-marks like a real service: the restart must converge.
	settlement.after = func(event app.InboxRecord) error {
		return store.MarkInbox(context.Background(), event.InboxEvent, "PROCESSED", "")
	}
	router := &Router{
		Store: store, Settlement: settlement, Lifecycle: lifecycle,
		Dispute: dispute, Cycles: cycles,
		Channels: []Channel{{Provider: domain.ProviderAsaas, Environment: domain.EnvironmentTest, Merchant: "merchant-test"}},
	}
	count, err := router.Drain(context.Background())
	if err != nil || count != 2 {
		t.Fatalf("count=%d err=%v", count, err)
	}
	if len(settlement.calls) != 1 || settlement.calls[0] != "evt_p19_received" {
		t.Fatalf("settlement calls=%v", settlement.calls)
	}
	if pay19InboxState(t, store, domain.ProviderAsaas, "evt_p19_unknown") != "PROCESSED" {
		t.Fatal("unknown event not retained as processed")
	}
	// Restart converges: nothing pending, no second effect.
	again, err := router.Drain(context.Background())
	if err != nil || again != 0 {
		t.Fatalf("restart count=%d err=%v", again, err)
	}
	if len(settlement.calls) != 1 {
		t.Fatalf("restart reprocessed: %v", settlement.calls)
	}
}

func TestPAY19DrainPreservesPendingThroughOutage(t *testing.T) {
	store := pay19FreshStore(t)
	pay19RecordInbox(t, store, domain.ProviderAsaas, "evt_p19_outage", "PAYMENT_RECEIVED")

	settlement := &fakeProcessor{err: domain.ErrUnavailable}
	router := &Router{
		Store: store, Settlement: settlement,
		Lifecycle: &fakeProcessor{}, Dispute: &fakeProcessor{}, Cycles: &fakeProcessor{},
		Channels: []Channel{{Provider: domain.ProviderAsaas, Environment: domain.EnvironmentTest, Merchant: "merchant-test"}},
	}
	count, err := router.Drain(context.Background())
	if !errors.Is(err, domain.ErrUnavailable) || count != 0 {
		t.Fatalf("count=%d err=%v", count, err)
	}
	if pay19InboxState(t, store, domain.ProviderAsaas, "evt_p19_outage") != "PENDING" {
		t.Fatal("outage consumed the event")
	}
	// Recovery processes the same durable row exactly once.
	settlement.err = nil
	settlement.calls = nil
	recovered, err := router.Drain(context.Background())
	if err != nil || recovered != 1 || len(settlement.calls) != 1 {
		t.Fatalf("recovered=%d calls=%v err=%v", recovered, settlement.calls, err)
	}
}

func TestPAY19DrainMarksConflictsWithoutLosingThem(t *testing.T) {
	store := pay19FreshStore(t)
	pay19RecordInbox(t, store, domain.ProviderStripe, "evt_p19_bad", "invoice.paid")

	conflict := &fakeProcessor{}
	// Like a real service: mark the conflict terminally, then report it.
	conflict.after = func(event app.InboxRecord) error {
		if err := store.MarkInbox(context.Background(), event.InboxEvent, "CONFLICT", "test-binding"); err != nil {
			return err
		}
		return domain.ErrConflict
	}
	router := &Router{
		Store: store, Settlement: conflict,
		Lifecycle: &fakeProcessor{}, Dispute: &fakeProcessor{}, Cycles: &fakeProcessor{},
		Channels: []Channel{{Provider: domain.ProviderStripe, Environment: domain.EnvironmentTest, Merchant: "merchant-test"}},
	}
	count, err := router.Drain(context.Background())
	if err == nil || count != 0 {
		t.Fatalf("count=%d err=%v", count, err)
	}
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("err=%v", err)
	}
	if pay19InboxState(t, store, domain.ProviderStripe, "evt_p19_bad") != "CONFLICT" {
		t.Fatal("conflict not retained")
	}
	// The conflicted row never re-enters the drain.
	again, err := router.Drain(context.Background())
	if err != nil || again != 0 {
		t.Fatalf("restart count=%d err=%v", again, err)
	}
}
