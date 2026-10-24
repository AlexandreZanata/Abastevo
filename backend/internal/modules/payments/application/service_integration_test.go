//go:build integration

package application_test

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"

	migrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/adapters"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func freshCheckoutDB(t *testing.T) *pgxpool.Pool {
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
	name := fmt.Sprintf("checkout_test_%d", time.Now().UnixNano())
	if _, err = admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
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
	return pool
}

func seedCheckoutOwner(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	id, err := jobs.NewUUIDv4()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(context.Background(), "INSERT INTO accounts(id,alias,status) VALUES($1,$2,'active')", id, "synthetic-"+id); err != nil {
		t.Fatal(err)
	}
	return id
}

func checkoutCatalog(t *testing.T) domain.Catalog {
	t.Helper()
	catalog, err := domain.NewCatalog("pay-catalog-v1", domain.Offer{
		ID: testOffer, Family: "premium", Term: domain.TermMonthly, Price: domain.BRL(1234),
		Capabilities: domain.PremiumCapabilityList(),
		Channels:     map[domain.Channel]bool{domain.ChannelCard: true},
		SaleEnabled:  true,
	})
	if err != nil {
		t.Fatal(err)
	}
	return catalog
}

func TestCheckoutServiceRealStore(t *testing.T) {
	pool := freshCheckoutDB(t)
	owner := seedCheckoutOwner(t, pool)
	provider := newFakeProvider()
	service := &app.Service{
		Store: adapters.NewStore(pool), Provider: provider, Catalog: checkoutCatalog(t),
		Config: app.Config{
			Environment: domain.EnvironmentTest, MerchantID: "acct_synthetic", PriceID: "price_synthetic",
			CatalogVersion: "pay-catalog-v1", ReturnOrigin: "https://app.example.invalid",
			NewSalesEnabled: true, LeaseTTL: time.Minute,
		},
	}
	request := app.StartRequest{OperationID: testOperation, OfferID: testOffer}
	first, err := service.Start(context.Background(), app.Principal{AccountID: owner}, request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Start(context.Background(), app.Principal{AccountID: owner}, request)
	if err != nil || !second.Replayed || second.SessionID != first.SessionID {
		t.Fatalf("replay=%+v err=%v", second, err)
	}
	if provider.createCount != 1 {
		t.Fatalf("create count = %d", provider.createCount)
	}
	var orders, operations, attempts, customers, settlements, grants int
	if err := pool.QueryRow(context.Background(), `SELECT
		(SELECT count(*) FROM payment_orders),
		(SELECT count(*) FROM payment_operations),
		(SELECT count(*) FROM payment_attempts),
		(SELECT count(*) FROM payment_customers),
		(SELECT count(*) FROM payment_settlements),
		(SELECT count(*) FROM payment_grants)`).Scan(&orders, &operations, &attempts, &customers, &settlements, &grants); err != nil {
		t.Fatal(err)
	}
	if orders != 1 || operations != 1 || attempts != 1 || customers != 1 || settlements != 0 || grants != 0 {
		t.Fatalf("orders=%d operations=%d attempts=%d customers=%d settlements=%d grants=%d", orders, operations, attempts, customers, settlements, grants)
	}
}

func TestCheckoutServiceLostResponseRealStore(t *testing.T) {
	pool := freshCheckoutDB(t)
	owner := seedCheckoutOwner(t, pool)
	provider := newFakeProvider()
	provider.ambiguousOnce = true
	service := &app.Service{
		Store: adapters.NewStore(pool), Provider: provider, Catalog: checkoutCatalog(t),
		Config: app.Config{
			Environment: domain.EnvironmentTest, MerchantID: "acct_synthetic", PriceID: "price_synthetic",
			CatalogVersion: "pay-catalog-v1", ReturnOrigin: "https://app.example.invalid",
			NewSalesEnabled: true, LeaseTTL: time.Minute,
		},
	}
	request := app.StartRequest{OperationID: testOperation, OfferID: testOffer}
	if _, err := service.Start(context.Background(), app.Principal{AccountID: owner}, request); !errors.Is(err, app.ErrAmbiguous) {
		t.Fatalf("first err = %v", err)
	}
	if _, err := pool.Exec(context.Background(),
		"UPDATE payment_operations SET lease_until=clock_timestamp()-interval '1 second'"); err != nil {
		t.Fatal(err)
	}
	recovered, err := service.Start(context.Background(), app.Principal{AccountID: owner}, request)
	if err != nil || !recovered.Replayed {
		t.Fatalf("recovered=%+v err=%v", recovered, err)
	}
	if provider.createCount != 1 || provider.findCount != 1 {
		t.Fatalf("create=%d find=%d", provider.createCount, provider.findCount)
	}
}
