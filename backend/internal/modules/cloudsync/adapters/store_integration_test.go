//go:build integration

package adapters

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	migrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/cloudsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/cloudsync/domain"
	dirread "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/adapters/read"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	sub "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/subscription/adapters"
	subapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/subscription/application"
	subdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/subscription/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"io/fs"
	"net/url"
	"os"
	"sync"
	"testing"
	"time"
)

func freshDB(t *testing.T) *pgxpool.Pool { return freshFS(t, migrations.Files) }
func freshFS(t *testing.T, files fs.FS) *pgxpool.Pool {
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
	name := fmt.Sprintf("premium_test_%d", time.Now().UnixNano())
	if _, err = admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
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
	if _, err = migrate.Apply(ctx, u.String(), files); err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.New(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}
func newid(t *testing.T) string {
	t.Helper()
	id, err := jobs.NewUUIDv4()
	if err != nil {
		t.Fatal(err)
	}
	return id
}
func seedOwner(t *testing.T, pool *pgxpool.Pool) app.Principal {
	t.Helper()
	p := app.Principal{AccountID: newid(t), FamilyID: newid(t), ContributorID: newid(t), KeyID: newid(t)}
	ctx := context.Background()
	for _, v := range []struct {
		sql  string
		args []any
	}{
		{"INSERT INTO accounts(id,alias,status) VALUES($1,$2,'active')", []any{p.AccountID, "synthetic-" + p.AccountID}},
		{"INSERT INTO account_session_families(id,account_id,refresh_salt,refresh_hash,access_salt,access_hash,access_expires,issued_at) VALUES($1,$2,'salt','hash','salt','hash',clock_timestamp()+interval '1 hour',clock_timestamp())", []any{p.FamilyID, p.AccountID}},
		{"INSERT INTO identity_contributors(id) VALUES($1)", []any{p.ContributorID}},
		{"INSERT INTO identity_keys(id,contributor_id,algorithm,public_jwk,fingerprint) VALUES($1,$2,'ecdsa-p256-sha512','{}',$3)", []any{p.KeyID, p.ContributorID, "synthetic-" + p.KeyID}},
		{"INSERT INTO account_contributor_bindings(account_id,contributor_id,key_fingerprint) VALUES($1,$2,'synthetic')", []any{p.AccountID, p.ContributorID}},
	} {
		if _, err := pool.Exec(ctx, v.sql, v.args...); err != nil {
			t.Fatal(err)
		}
	}
	return p
}

type testProvider struct{}

func (testProvider) Verify(context.Context, subapp.ProviderRequest) (subapp.ProviderPurchase, error) {
	return subapp.ProviderPurchase{Snapshot: subdomain.Snapshot{State: subdomain.Active, ValidUntil: time.Now().Add(time.Hour), ProductID: "synthetic.premium", BasePlanID: "monthly", Acknowledged: true}}, nil
}
func paidStore(t *testing.T, pool *pgxpool.Pool, p app.Principal) *Store {
	t.Helper()
	subStore := sub.NewStore(pool)
	codec, _ := sub.NewTokenCodec(bytes.Repeat([]byte{4}, 32))
	binding, _ := sub.NewAccountBinding(bytes.Repeat([]byte{5}, 32), "test", "com.anpfuel")
	svc := &subapp.Service{Store: subStore, Tokens: codec, Provider: testProvider{}, AccountBinding: binding, Environment: "test", PackageName: "com.anpfuel"}
	_, err := svc.Admit(context.Background(), subapp.Principal{AccountID: p.AccountID, FamilyID: p.FamilyID, ContributorID: p.ContributorID, KeyID: p.KeyID}, subapp.Command{OperationID: newid(t), Token: "synthetic-cloud-" + p.AccountID, Kind: "verify"})
	if err != nil {
		t.Fatal(err)
	}
	store := NewStore(pool)
	store.Paid = func(ctx context.Context, tx pgx.Tx, owner string) error {
		err := subStore.RequireCapability(ctx, tx, owner, "cloud.sync")
		if errors.Is(err, subdomain.ErrDenied) {
			return domain.ErrDenied
		}
		if errors.Is(err, subdomain.ErrUnauthorized) {
			return domain.ErrUnauthorized
		}
		return err
	}
	return store
}
func vehicle(t *testing.T) domain.Vehicle {
	return domain.Vehicle{ID: newid(t), Name: "Synthetic vehicle", CapacityMilli: 50000, Fuel: kernel.Ethanol, Unit: kernel.Liter, PriceSource: "CHEAPEST_STATION"}
}
func TestVehicleOwnerMappingReplayAndThreeSlotRace(t *testing.T) {
	pool := freshDB(t)
	p := seedOwner(t, pool)
	foreign := seedOwner(t, pool)
	store := paidStore(t, pool, p)
	paidStore(t, pool, foreign)
	ctx := context.Background()
	v := vehicle(t)
	got, err := store.PutVehicle(ctx, p, v, 0)
	if err != nil || got.ID != v.ID || got.Revision != 1 {
		t.Fatal("mapping", err)
	}
	got, err = store.PutVehicle(ctx, p, v, 0)
	if err != nil || got.Revision != 1 {
		t.Fatal("repeat import", err)
	}
	changed := v
	changed.Name = "Changed"
	if _, err = store.PutVehicle(ctx, p, changed, 0); !errors.Is(err, domain.ErrConflict) {
		t.Fatal("divergent initial UUID overwrite", err)
	}
	if _, err = store.PutVehicle(ctx, foreign, v, 0); err != nil {
		t.Fatal("cross-owner UUID collision", err)
	}
	wrong := p
	wrong.FamilyID = foreign.FamilyID
	if _, err = store.PutVehicle(ctx, wrong, v, 1); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatal("foreign session", err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 12)
	values := make([]domain.Vehicle, 12)
	for i := range values {
		values[i] = vehicle(t)
		values[i].Archived = i == 0
	}
	for _, candidate := range values {
		wg.Add(1)
		go func(v domain.Vehicle) { defer wg.Done(); _, e := store.PutVehicle(ctx, p, v, 0); errs <- e }(candidate)
	}
	wg.Wait()
	close(errs)
	wins := 0
	for e := range errs {
		if e == nil {
			wins++
		} else if !errors.Is(e, domain.ErrQuota) {
			t.Fatal(e)
		}
	}
	if wins != 2 {
		t.Fatalf("quota race %d want 2", wins)
	}
	if _, err = store.PutVehicle(ctx, p, changed, 1); err != nil {
		t.Fatal("conditional update", err)
	}
	if _, err = store.PutVehicle(ctx, p, v, 1); !errors.Is(err, domain.ErrConflict) {
		t.Fatal("stale update", err)
	}
	if _, err = NewStore(pool).PutVehicle(ctx, p, v, 2); !errors.Is(err, domain.ErrDenied) {
		t.Fatal("nil paid port admitted", err)
	}
}
func TestVehicleDeletionAndStaleSubscriptionFailClosed(t *testing.T) {
	pool := freshDB(t)
	p := seedOwner(t, pool)
	store := paidStore(t, pool, p)
	ctx := context.Background()
	v := vehicle(t)
	if _, err := store.PutVehicle(ctx, p, v, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "UPDATE subscription_purchases SET refresh_required=true WHERE account_id=$1", p.AccountID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.PutVehicle(ctx, p, vehicle(t), 0); !errors.Is(err, domain.ErrDenied) {
		t.Fatal("dirty proof grant", err)
	}
	if _, err := pool.Exec(ctx, "UPDATE accounts SET status='deleted' WHERE id=$1", p.AccountID); err != nil {
		t.Fatal(err)
	}
	var raw []byte
	var deleted bool
	if err := pool.QueryRow(ctx, "SELECT payload,deleted FROM cloud_records WHERE account_id=$1 AND id=$2", p.AccountID, v.ID).Scan(&raw, &deleted); err != nil || raw != nil || !deleted {
		t.Fatal("owner erasure", err)
	}
	if _, err := store.PutVehicle(ctx, p, v, 1); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatal("deleted owner", err)
	}
}

func TestVehicleGateExpiryBeforeCommitRollsBack(t *testing.T) {
	pool := freshDB(t)
	p := seedOwner(t, pool)
	store := paidStore(t, pool, p)
	gate := store.Paid
	calls := 0
	store.Paid = func(ctx context.Context, tx pgx.Tx, owner string) error {
		calls++
		if calls == 2 {
			return domain.ErrDenied
		}
		return gate(ctx, tx, owner)
	}
	if _, err := store.PutVehicle(context.Background(), p, vehicle(t), 0); !errors.Is(err, domain.ErrDenied) {
		t.Fatal("expiry before commit", err)
	}
	var count int
	if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM cloud_records").Scan(&count); err != nil || count != 0 {
		t.Fatal("expired write persisted", err)
	}
	store.Paid = func(context.Context, pgx.Tx, string) error { return domain.ErrUnavailable }
	if _, err := store.PutVehicle(context.Background(), p, vehicle(t), 0); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatal("outage disguised as payment refusal", err)
	}
}

func TestFavoriteCanonicalReferencesAndOwnedPreferences(t *testing.T) {
	pool := freshDB(t)
	p := seedOwner(t, pool)
	foreign := seedOwner(t, pool)
	store := paidStore(t, pool, p)
	paidStore(t, pool, foreign)
	ctx := context.Background()
	store.Station = dirread.NewReader(pool).ActiveReference
	station := newid(t)
	if _, err := store.PutFavorite(ctx, p, station, 0); !errors.Is(err, domain.ErrInvalid) {
		t.Fatal("unknown station recreated", err)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO directory_stations(id,display_name) VALUES($1,'Synthetic station')", station); err != nil {
		t.Fatal(err)
	}
	got, err := store.PutFavorite(ctx, p, station, 0)
	if err != nil || got.ID != station {
		t.Fatal("canonical favorite", err)
	}
	again, err := store.PutFavorite(ctx, p, station, 0)
	if err != nil || again.Revision != got.Revision {
		t.Fatal("repeat favorite", err)
	}
	if _, err = store.PutFavorite(ctx, foreign, station, 0); err != nil {
		t.Fatal("owned set isolation", err)
	}
	if _, err = pool.Exec(ctx, "UPDATE directory_stations SET status='retired' WHERE id=$1", station); err != nil {
		t.Fatal(err)
	}
	if _, err = store.PutFavorite(ctx, p, station, 1); !errors.Is(err, domain.ErrInvalid) {
		t.Fatal("retired station selected", err)
	}
	prefs := domain.Preferences{Version: 1, State: "MT", Municipality: "5103403", Locale: "pt-BR", RadiusKm: 3, ShowPriceHistory: true}
	got, err = store.PutPreferences(ctx, p, prefs, 0)
	if err != nil || got.ID != domain.PreferencesID {
		t.Fatal("preference singleton", err)
	}
	prefs.Locale = "en"
	if _, err = store.PutPreferences(ctx, p, prefs, 0); !errors.Is(err, domain.ErrConflict) {
		t.Fatal("preference overwrite", err)
	}
	if _, err = store.PutPreferences(ctx, p, prefs, got.Revision); err != nil {
		t.Fatal("preference conditional update", err)
	}
	wrong := p
	wrong.FamilyID = foreign.FamilyID
	if _, err = store.PutPreferences(ctx, wrong, prefs, 2); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatal("IDOR preference", err)
	}
	var directoryCount int
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM directory_stations").Scan(&directoryCount); err != nil || directoryCount != 1 {
		t.Fatal("private selection created public facts", err)
	}
}
