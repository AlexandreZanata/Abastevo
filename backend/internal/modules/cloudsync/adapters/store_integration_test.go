//go:build integration

package adapters

import (
	"bytes"
	"context"
	"encoding/json"
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
	"strings"
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
	store.MultipleVehicles = func(ctx context.Context, tx pgx.Tx, owner string) error {
		err := subStore.RequireCapability(ctx, tx, owner, "vehicles.multiple")
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

func TestDeviceRegistrationRevocationAndOriginIsolation(t *testing.T) {
	pool := freshDB(t)
	p := seedOwner(t, pool)
	store := paidStore(t, pool, p)
	store.Origin = "one.example.test"
	ctx := context.Background()
	d, err := store.RegisterDevice(ctx, p)
	if err != nil || !domain.ValidID(d.ID) {
		t.Fatal("random device registration", err)
	}
	p.DeviceID = d.ID
	if err = store.CheckDevice(ctx, p); err != nil {
		t.Fatal("bound device", err)
	}
	again, err := store.RegisterDevice(ctx, p)
	if err != nil || again.ID != d.ID {
		t.Fatal("repeat register", err)
	}
	store.Origin = "two.example.test"
	if err = store.CheckDevice(ctx, p); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatal("origin crossover", err)
	}
	store.Origin = "one.example.test"
	if err = store.RevokeDevice(ctx, p, d.ID); err != nil {
		t.Fatal(err)
	}
	if err = store.CheckDevice(ctx, p); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatal("revoked device reused", err)
	}
	if _, err = pool.Exec(ctx, "UPDATE cloud_devices SET revoked_at=NULL WHERE id=$1", d.ID); err != nil {
		t.Fatal(err)
	}
	if err = store.CheckDevice(ctx, p); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatal("restored revoked row accepted", err)
	}
	fresh, err := store.RegisterDevice(ctx, p)
	if err != nil || fresh.ID == d.ID {
		t.Fatal("revoked UUID resurrected", err)
	}
}

func TestDeviceCapConcurrentSessionsAndFreeRevoke(t *testing.T) {
	pool := freshDB(t)
	owner := seedOwner(t, pool)
	foreign := seedOwner(t, pool)
	store := paidStore(t, pool, owner)
	store.Origin = "one.example.test"
	ctx := context.Background()
	principals := make([]app.Principal, 12)
	for i := range principals {
		p := owner
		p.FamilyID = newid(t)
		principals[i] = p
		if _, err := pool.Exec(ctx, "INSERT INTO account_session_families(id,account_id,refresh_salt,refresh_hash,access_salt,access_hash,access_expires,issued_at) VALUES($1,$2,'s','h','s','h',clock_timestamp()+interval '1 hour',clock_timestamp())", p.FamilyID, p.AccountID); err != nil {
			t.Fatal(err)
		}
	}
	var wg sync.WaitGroup
	errs := make(chan error, 12)
	devices := make(chan app.Device, 12)
	for _, p := range principals {
		wg.Add(1)
		go func(p app.Principal) {
			defer wg.Done()
			d, e := store.RegisterDevice(ctx, p)
			errs <- e
			if e == nil {
				devices <- d
			}
		}(p)
	}
	wg.Wait()
	close(errs)
	close(devices)
	wins := 0
	for e := range errs {
		if e == nil {
			wins++
		} else if !errors.Is(e, domain.ErrQuota) {
			t.Fatal(e)
		}
	}
	if wins != 3 {
		t.Fatalf("device cap %d want 3", wins)
	}
	d := <-devices
	if err := store.RevokeDevice(ctx, foreign, d.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal("foreign revoke", err)
	}
	store.Paid = nil
	if err := store.RevokeDevice(ctx, owner, d.ID); err != nil {
		t.Fatal("revoke requires paid capability", err)
	}
	if _, err := pool.Exec(ctx, "UPDATE accounts SET status='deleted' WHERE id=$1", owner.AccountID); err != nil {
		t.Fatal(err)
	}
	var live int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM cloud_devices WHERE account_id=$1 AND revoked_at IS NULL", owner.AccountID).Scan(&live); err != nil || live != 0 {
		t.Fatal("deletion device fence", err)
	}
}

func TestCloudAtomicCommandsReplayAndEpoch(t *testing.T) {
	pool := freshDB(t)
	p := seedOwner(t, pool)
	store := paidStore(t, pool, p)
	store.Origin = "one.example.test"
	ctx := context.Background()
	d, err := store.RegisterDevice(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	p.DeviceID = d.ID
	if err = store.EnsureState(ctx, p); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, "UPDATE cloud_state SET consent_mask=7,consent_revision=1 WHERE account_id=$1", p.AccountID); err != nil {
		t.Fatal(err)
	}
	v := vehicle(t)
	raw, _ := json.Marshal(v)
	batch := app.Batch{Epoch: 1, ConsentRevision: 1, Commands: []app.Command{{OperationID: newid(t), Dataset: "vehicles", ID: v.ID, Payload: raw}}}
	result, err := store.Apply(ctx, p, batch)
	if err != nil || len(result.Results) != 1 || result.Results[0].Revision != 1 {
		t.Fatal("atomic create", err)
	}
	replay, err := store.Apply(ctx, p, batch)
	if err != nil || !replay.Results[0].Replayed || replay.Results[0].Revision != 1 {
		t.Fatal("receipt replay", err)
	}
	changed := batch
	changed.Commands = append([]app.Command{}, batch.Commands...)
	v.Name = "Changed payload"
	changed.Commands[0].Payload, _ = json.Marshal(v)
	if _, err = store.Apply(ctx, p, changed); !errors.Is(err, domain.ErrConflict) {
		t.Fatal("changed operation reuse", err)
	}
	batch.Epoch = 2
	if _, err = store.Apply(ctx, p, batch); !errors.Is(err, domain.ErrResnapshot) {
		t.Fatal("stale epoch", err)
	}
}

func readyDevice(t *testing.T, store *Store, p app.Principal) app.Principal {
	t.Helper()
	store.Origin = "one.example.test"
	ctx := context.Background()
	d, e := store.RegisterDevice(ctx, p)
	if e != nil {
		t.Fatal(e)
	}
	p.DeviceID = d.ID
	if e = store.EnsureState(ctx, p); e != nil {
		t.Fatal(e)
	}
	if _, e = store.Pool.Exec(ctx, "UPDATE cloud_state SET consent_mask=7,consent_revision=1 WHERE account_id=$1", p.AccountID); e != nil {
		t.Fatal(e)
	}
	return p
}
func vehicleCommand(t *testing.T, v domain.Vehicle, expected int64) app.Command {
	raw, _ := json.Marshal(v)
	return app.Command{OperationID: newid(t), Dataset: "vehicles", ID: v.ID, ExpectedRevision: expected, Payload: raw}
}
func TestCloudBatchRollbackAndTwelveWayReplay(t *testing.T) {
	pool := freshDB(t)
	p := seedOwner(t, pool)
	store := paidStore(t, pool, p)
	p = readyDevice(t, store, p)
	ctx := context.Background()
	first := vehicleCommand(t, vehicle(t), 0)
	second := vehicleCommand(t, vehicle(t), 99)
	batch := app.Batch{Epoch: 1, ConsentRevision: 1, Commands: []app.Command{first, second}}
	if _, err := store.Apply(ctx, p, batch); !errors.Is(err, domain.ErrConflict) {
		t.Fatal("partial batch accepted", err)
	}
	var records, changes, receipts int
	var success *time.Time
	if err := pool.QueryRow(ctx, "SELECT (SELECT count(*) FROM cloud_records),(SELECT count(*) FROM cloud_changes),(SELECT count(*) FROM cloud_operations)").Scan(&records, &changes, &receipts); err != nil || records+changes+receipts != 0 {
		t.Fatal("partial SQL effects", err)
	}
	if err := pool.QueryRow(ctx, "SELECT last_success_at FROM cloud_devices WHERE id=$1", p.DeviceID).Scan(&success); err != nil || success != nil {
		t.Fatal("failed batch marked success", err)
	}
	if _, err := pool.Exec(ctx, `CREATE FUNCTION reject_cloud_receipt() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'synthetic'; END $$; CREATE TRIGGER reject_cloud_receipt BEFORE INSERT ON cloud_operations FOR EACH ROW EXECUTE FUNCTION reject_cloud_receipt()`); err != nil {
		t.Fatal(err)
	}
	batch.Commands = []app.Command{first}
	if _, err := store.Apply(ctx, p, batch); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatal("receipt failure hidden", err)
	}
	if _, err := pool.Exec(ctx, "DROP TRIGGER reject_cloud_receipt ON cloud_operations"); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 12)
	fresh := make(chan bool, 12)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, e := store.Apply(ctx, p, batch)
			errs <- e
			if e == nil {
				fresh <- !r.Results[0].Replayed
			}
		}()
	}
	wg.Wait()
	close(errs)
	close(fresh)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	wins := 0
	for f := range fresh {
		if f {
			wins++
		}
	}
	if wins != 1 {
		t.Fatalf("replay effects %d want 1", wins)
	}
	if err := pool.QueryRow(ctx, "SELECT (SELECT count(*) FROM cloud_records),(SELECT count(*) FROM cloud_changes),(SELECT count(*) FROM cloud_operations)").Scan(&records, &changes, &receipts); err != nil || records != 1 || changes != 1 || receipts != 1 {
		t.Fatal("duplicate effects", err)
	}
	if err := store.RevokeDevice(ctx, p, p.DeviceID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Apply(ctx, p, batch); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatal("receipt recovered revoked device", err)
	}
}
func TestCloudConsentLimitsAndDeletedEpoch(t *testing.T) {
	pool := freshDB(t)
	p := seedOwner(t, pool)
	store := paidStore(t, pool, p)
	p = readyDevice(t, store, p)
	ctx := context.Background()
	batch := app.Batch{Epoch: 1, ConsentRevision: 1, Commands: []app.Command{vehicleCommand(t, vehicle(t), 0)}}
	if _, err := pool.Exec(ctx, "UPDATE cloud_state SET consent_mask=0 WHERE account_id=$1", p.AccountID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Apply(ctx, p, batch); !errors.Is(err, domain.ErrDenied) {
		t.Fatal("no-consent upload", err)
	}
	if _, err := pool.Exec(ctx, "UPDATE accounts SET status='deleted' WHERE id=$1", p.AccountID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Apply(ctx, p, batch); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatal("deleted owner batch", err)
	}
	var epoch int64
	if err := pool.QueryRow(ctx, "SELECT epoch FROM cloud_state WHERE account_id=$1", p.AccountID).Scan(&epoch); err != nil || epoch != 2 {
		t.Fatal("deletion epoch not fenced", err)
	}
	batch.Commands = make([]app.Command, 51)
	if _, err := normalizeBatch(batch); !errors.Is(err, domain.ErrInvalid) {
		t.Fatal("unbounded batch", err)
	}
}

func TestCloudFreeReadExportErasureInventoryAndForeignSession(t *testing.T) {
	pool := freshDB(t)
	owner := seedOwner(t, pool)
	foreign := seedOwner(t, pool)
	store := paidStore(t, pool, owner)
	p := readyDevice(t, store, owner)
	ctx := context.Background()
	v := vehicle(t)
	batch := app.Batch{Epoch: 1, ConsentRevision: 1, Commands: []app.Command{vehicleCommand(t, v, 0)}}
	if _, err := store.Apply(ctx, p, batch); err != nil {
		t.Fatal(err)
	}
	store.Paid = nil
	rows, err := store.List(ctx, owner, "vehicles", 100, "")
	if err != nil || len(rows) != 1 || rows[0].ID != v.ID {
		t.Fatal("free export", err)
	}
	rows, err = store.List(ctx, foreign, "vehicles", 100, "")
	if err != nil || len(rows) != 0 {
		t.Fatal("IDOR export", err)
	}
	wrong := owner
	wrong.FamilyID = foreign.FamilyID
	if _, err = store.List(ctx, wrong, "vehicles", 100, ""); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatal("foreign session export", err)
	}
	if _, err = store.List(ctx, owner, "professional", 100, ""); !errors.Is(err, domain.ErrInvalid) {
		t.Fatal("unknown inventory reported complete", err)
	}
	devices, err := store.Devices(ctx, owner, 100, "")
	if err != nil || len(devices) != 1 {
		t.Fatal("owner device inventory", err)
	}
	epoch, err := store.Erase(ctx, owner, "all", "")
	if err != nil || epoch != 2 {
		t.Fatal("free erasure", err)
	}
	if again, err := store.Erase(ctx, owner, "all", ""); err != nil || again != epoch {
		t.Fatal("erasure retry bumped epoch", err)
	}
	var content int
	if err = pool.QueryRow(ctx, "SELECT (SELECT count(*) FROM cloud_records WHERE payload IS NOT NULL)+(SELECT count(*) FROM cloud_changes WHERE payload IS NOT NULL)").Scan(&content); err != nil || content != 0 {
		t.Fatal("erased copies retained", err)
	}
	if err = store.CheckDevice(ctx, p); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatal("erased device access", err)
	}
	state, err := store.Status(ctx, owner)
	if err != nil || state.Epoch != 2 || len(state.EnabledDatasets) != 0 {
		t.Fatal("erasure consent/status", err)
	}
}

func TestCloudFreeErasureRollbackAndIndexedReads(t *testing.T) {
	pool := freshDB(t)
	p := seedOwner(t, pool)
	foreign := seedOwner(t, pool)
	store := paidStore(t, pool, p)
	p = readyDevice(t, store, p)
	ctx := context.Background()
	v := vehicle(t)
	if _, err := store.Apply(ctx, p, app.Batch{Epoch: 1, ConsentRevision: 1, Commands: []app.Command{vehicleCommand(t, v, 0)}}); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `CREATE FUNCTION reject_cloud_epoch() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'synthetic'; END $$; CREATE TRIGGER reject_cloud_epoch BEFORE UPDATE ON cloud_state FOR EACH ROW EXECUTE FUNCTION reject_cloud_epoch()`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Erase(ctx, p, "all", ""); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatal("privacy failure reported success", err)
	}
	rows, err := store.List(ctx, p, "vehicles", 100, "")
	if err != nil || len(rows) != 1 {
		t.Fatal("failed erasure lost data", err)
	}
	if err = store.CheckDevice(ctx, p); err != nil {
		t.Fatal("failed erasure revoked authority", err)
	}
	if _, err = pool.Exec(ctx, "DROP TRIGGER reject_cloud_epoch ON cloud_state"); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO cloud_records(account_id,dataset,id,payload) SELECT $1,'vehicles',md5(i::text)::uuid,'{}' FROM generate_series(1,10000)i`, foreign.AccountID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, "ANALYZE cloud_records"); err != nil {
		t.Fatal(err)
	}
	plan, err := pool.Query(ctx, "EXPLAIN SELECT dataset,id,revision,deleted,payload FROM cloud_records WHERE account_id=$1 AND dataset='vehicles' AND NOT deleted AND id>$2 ORDER BY id LIMIT 100", p.AccountID, zeroID)
	if err != nil {
		t.Fatal(err)
	}
	text := ""
	for plan.Next() {
		var line string
		if err = plan.Scan(&line); err != nil {
			t.Fatal(err)
		}
		text += line
	}
	plan.Close()
	if !strings.Contains(text, "Index") || (!strings.Contains(text, "cloud_records_live_idx") && !strings.Contains(text, "cloud_records_pkey")) {
		t.Fatal("owner page did not use index", text)
	}
	before, _ := store.Status(ctx, p)
	if _, err = store.Erase(ctx, p, "vehicles", newid(t)); err != nil {
		t.Fatal(err)
	}
	after, _ := store.Status(ctx, p)
	if before.Epoch != after.Epoch || len(before.EnabledDatasets) != len(after.EnabledDatasets) {
		t.Fatal("unknown resource changed consent")
	}
	if _, err = store.Erase(ctx, p, "all", ""); err != nil {
		t.Fatal("retry erasure", err)
	}
}

func TestIncrementalFeedFixedWatermarkAndEpoch(t *testing.T) {
	pool := freshDB(t)
	p := seedOwner(t, pool)
	store := paidStore(t, pool, p)
	p = readyDevice(t, store, p)
	ctx := context.Background()
	v := vehicle(t)
	if _, err := store.Apply(ctx, p, app.Batch{Epoch: 1, ConsentRevision: 1, Commands: []app.Command{vehicleCommand(t, v, 0)}}); err != nil {
		t.Fatal(err)
	}
	first, err := store.Pull(ctx, p, app.Position{Version: 1}, 1)
	if err != nil || len(first.Changes) != 1 || first.Through != 1 {
		t.Fatal("initial feed", err)
	}
	v.Name = "Second revision"
	if _, err = store.Apply(ctx, p, app.Batch{Epoch: 1, ConsentRevision: 1, Commands: []app.Command{vehicleCommand(t, v, 1)}}); err != nil {
		t.Fatal(err)
	}
	bounded, err := store.Pull(ctx, p, app.Position{Version: 1, Epoch: 1, ConsentRevision: 1, After: 1, Through: 1}, 100)
	if err != nil || len(bounded.Changes) != 0 || bounded.Through != 1 {
		t.Fatal("interleaving changed watermark", err)
	}
	next, err := store.Pull(ctx, p, app.Position{Version: 1, Epoch: 1, ConsentRevision: 1, After: 1}, 100)
	if err != nil || len(next.Changes) != 1 || next.Changes[0].Sequence != 2 {
		t.Fatal("new cycle lost change", err)
	}
	if _, err = pool.Exec(ctx, "UPDATE cloud_state SET compacted_through=2 WHERE account_id=$1", p.AccountID); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Pull(ctx, p, app.Position{Version: 1, Epoch: 1, ConsentRevision: 1, After: 1}, 100); !errors.Is(err, domain.ErrResnapshot) {
		t.Fatal("compacted cursor silently skipped", err)
	}
}

func TestPullPaginatedInterleavedAndSameInstant(t *testing.T) {
	pool := freshDB(t)
	p := seedOwner(t, pool)
	store := paidStore(t, pool, p)
	p = readyDevice(t, store, p)
	ctx := context.Background()
	v := vehicle(t)
	if _, err := store.Apply(ctx, p, app.Batch{Epoch: 1, ConsentRevision: 1, Commands: []app.Command{vehicleCommand(t, v, 0)}}); err != nil {
		t.Fatal(err)
	}
	for i, name := range []string{"Second revision", "Third revision"} {
		v.Name = name
		if _, err := store.Apply(ctx, p, app.Batch{Epoch: 1, ConsentRevision: 1, Commands: []app.Command{vehicleCommand(t, v, int64(i+1))}}); err != nil {
			t.Fatal(err)
		}
	}
	first, err := store.Pull(ctx, p, app.Position{Version: 1}, 1)
	if err != nil || len(first.Changes) != 1 || first.Changes[0].Sequence != 1 || first.Through != 3 {
		t.Fatalf("first page watermark %v %v", first, err)
	}
	v.Name = "Fourth revision"
	if _, err = store.Apply(ctx, p, app.Batch{Epoch: 1, ConsentRevision: 1, Commands: []app.Command{vehicleCommand(t, v, 3)}}); err != nil {
		t.Fatal(err)
	}
	pinned, err := store.Pull(ctx, p, app.Position{Version: 1, Epoch: 1, ConsentRevision: 1, After: 1, Through: 3}, 10)
	if err != nil || len(pinned.Changes) != 2 || pinned.Changes[0].Sequence != 2 || pinned.Changes[1].Sequence != 3 || pinned.Through != 3 {
		t.Fatalf("pinned page leaked interleaving %v %v", pinned, err)
	}
	fresh, err := store.Pull(ctx, p, app.Position{Version: 1, Epoch: 1, ConsentRevision: 1, After: 1}, 10)
	if err != nil || len(fresh.Changes) != 3 || fresh.Changes[2].Sequence != 4 {
		t.Fatalf("new cycle lost interleaving %v %v", fresh, err)
	}
	stamp := time.Now().UTC().Truncate(time.Microsecond)
	if _, err = pool.Exec(ctx, "UPDATE cloud_changes SET created_at=$1 WHERE account_id=$2", stamp, p.AccountID); err != nil {
		t.Fatal(err)
	}
	ordered, err := store.Pull(ctx, p, app.Position{Version: 1, Epoch: 1, ConsentRevision: 1, After: 1, Through: 3}, 10)
	if err != nil || len(ordered.Changes) != 2 || ordered.Changes[0].Sequence != 2 || ordered.Changes[1].Sequence != 3 {
		t.Fatalf("same-instant order lost %v %v", ordered, err)
	}
	if _, err = store.Pull(ctx, p, app.Position{Version: 2, Epoch: 1, ConsentRevision: 1, After: 1, Through: 3}, 10); !errors.Is(err, domain.ErrInvalid) {
		t.Fatal("foreign version accepted")
	}
	if _, err = store.Pull(ctx, p, app.Position{Version: 1, Epoch: 1, ConsentRevision: 1, After: 1, Through: 99}, 10); !errors.Is(err, domain.ErrInvalid) {
		t.Fatal("future watermark accepted")
	}
	if _, err = store.Pull(ctx, p, app.Position{Version: 1, Epoch: 1, ConsentRevision: 1, After: 3, Through: 1}, 10); !errors.Is(err, domain.ErrInvalid) {
		t.Fatal("reversed cursor accepted")
	}
	if _, err = store.Pull(ctx, p, app.Position{Version: 1, Epoch: 1, ConsentRevision: 1, After: 0}, 0); !errors.Is(err, domain.ErrInvalid) {
		t.Fatal("unbounded page accepted")
	}
	if _, err = store.Pull(ctx, p, app.Position{Version: 1, Epoch: 1, ConsentRevision: 1, After: 0}, 101); !errors.Is(err, domain.ErrInvalid) {
		t.Fatal("oversize page accepted")
	}
}

func TestPullIsolationRevocationConsentAndPlans(t *testing.T) {
	pool := freshDB(t)
	p := seedOwner(t, pool)
	foreign := seedOwner(t, pool)
	store := paidStore(t, pool, p)
	paidStore(t, pool, foreign)
	p = readyDevice(t, store, p)
	foreign = readyDevice(t, store, foreign)
	ctx := context.Background()
	v := vehicle(t)
	if _, err := store.Apply(ctx, p, app.Batch{Epoch: 1, ConsentRevision: 1, Commands: []app.Command{vehicleCommand(t, v, 0)}}); err != nil {
		t.Fatal(err)
	}
	isolated, err := store.Pull(ctx, foreign, app.Position{Version: 1}, 100)
	if err != nil || len(isolated.Changes) != 0 {
		t.Fatalf("cross-owner feed leaked %v %v", isolated, err)
	}
	forged := p
	forged.DeviceID = newid(t)
	if _, err = store.Pull(ctx, forged, app.Position{Version: 1, Epoch: 1, ConsentRevision: 1, After: 0}, 10); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatal("foreign device pull admitted")
	}
	store.Origin = "two.example.test"
	if _, err = store.Pull(ctx, p, app.Position{Version: 1, Epoch: 1, ConsentRevision: 1, After: 0}, 10); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatal("origin crossover pull admitted")
	}
	store.Origin = "one.example.test"
	if err = store.RevokeDevice(ctx, p, p.DeviceID); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Pull(ctx, p, app.Position{Version: 1, Epoch: 1, ConsentRevision: 1, After: 0}, 10); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatal("revoked device pull admitted")
	}
	p = readyDevice(t, store, p)
	if _, err = pool.Exec(ctx, "UPDATE cloud_state SET epoch=2, revision=revision+1 WHERE account_id=$1", p.AccountID); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Pull(ctx, p, app.Position{Version: 1, Epoch: 1, ConsentRevision: 1, After: 0}, 10); !errors.Is(err, domain.ErrResnapshot) {
		t.Fatal("stale epoch silently advanced")
	}
	if _, err = pool.Exec(ctx, "UPDATE cloud_state SET epoch=1, consent_mask=0, consent_revision=2 WHERE account_id=$1", p.AccountID); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Pull(ctx, p, app.Position{Version: 1, Epoch: 1, ConsentRevision: 1, After: 0}, 10); !errors.Is(err, domain.ErrResnapshot) {
		t.Fatal("stale consent silently advanced")
	}
	filtered, err := store.Pull(ctx, p, app.Position{Version: 1, Epoch: 1, ConsentRevision: 2, After: 0}, 100)
	if err != nil || len(filtered.Changes) != 0 || filtered.Through != 2 {
		t.Fatalf("revoked consent leaked %v %v", filtered, err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO cloud_changes(account_id,sequence,epoch,dataset,id,record_revision,deleted,payload) SELECT $1, generate_series(100,10100),1,'vehicles',md5(g::text)::uuid,1,false,'{}' FROM generate_series(1,1)g`, foreign.AccountID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, "ANALYZE cloud_changes"); err != nil {
		t.Fatal(err)
	}
	plan, err := pool.Query(ctx, "EXPLAIN SELECT sequence,epoch,dataset,id,record_revision,deleted,payload FROM cloud_changes WHERE account_id=$1 AND sequence>$2 AND sequence<=$3 ORDER BY sequence LIMIT 100", p.AccountID, 0, 2)
	if err != nil {
		t.Fatal(err)
	}
	text := ""
	for plan.Next() {
		var line string
		if err = plan.Scan(&line); err != nil {
			t.Fatal(err)
		}
		text += line
	}
	plan.Close()
	if !strings.Contains(text, "Index") || (!strings.Contains(text, "cloud_changes_pkey") && !strings.Contains(text, "cloud_changes")) {
		t.Fatal("delta page did not use owner sequence index", text)
	}
	store.Paid = nil
	if _, err = store.Pull(ctx, p, app.Position{Version: 1, Epoch: 1, ConsentRevision: 2, After: 0}, 10); !errors.Is(err, domain.ErrDenied) {
		t.Fatal("unpaid pull admitted")
	}
}

func secondDevice(t *testing.T, pool *pgxpool.Pool, store *Store, p app.Principal) app.Principal {
	t.Helper()
	other := p
	other.FamilyID = newid(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, "INSERT INTO account_session_families(id,account_id,refresh_salt,refresh_hash,access_salt,access_hash,access_expires,issued_at) VALUES($1,$2,'s','h','s','h',clock_timestamp()+interval '1 hour',clock_timestamp())", other.FamilyID, other.AccountID); err != nil {
		t.Fatal(err)
	}
	d, err := store.RegisterDevice(ctx, other)
	if err != nil {
		t.Fatal(err)
	}
	other.DeviceID = d.ID
	return other
}

func TestPushConflictsRecoverable(t *testing.T) {
	pool := freshDB(t)
	p := seedOwner(t, pool)
	store := paidStore(t, pool, p)
	p = readyDevice(t, store, p)
	other := secondDevice(t, pool, store, p)
	ctx := context.Background()
	v := vehicle(t)
	if _, err := store.Apply(ctx, p, app.Batch{Epoch: 1, ConsentRevision: 1, Commands: []app.Command{vehicleCommand(t, v, 0)}}); err != nil {
		t.Fatal(err)
	}
	stale := v
	stale.Name = "Device B stale edit"
	staleCmd := vehicleCommand(t, stale, 1)
	updated := v
	updated.Name = "Device A current edit"
	if _, err := store.Apply(ctx, p, app.Batch{Epoch: 1, ConsentRevision: 1, Commands: []app.Command{vehicleCommand(t, updated, 1)}}); err != nil {
		t.Fatal(err)
	}
	_, err := store.Apply(ctx, other, app.Batch{Epoch: 1, ConsentRevision: 1, Commands: []app.Command{staleCmd}})
	var conflict *app.ConflictError
	if !errors.As(err, &conflict) || !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("stale arrival overwrote current %v", err)
	}
	if conflict.Conflict.ExpectedRevision != 1 || conflict.Conflict.CurrentRevision != 2 || conflict.Conflict.Erased {
		t.Fatalf("conflict base/current wrong %+v", conflict.Conflict)
	}
	if len(conflict.Conflict.ProposedPayload) == 0 || len(conflict.Conflict.CurrentPayload) == 0 {
		t.Fatal("conflict missing base/current/proposed")
	}
	rows, err := store.List(ctx, p, "vehicles", 100, "")
	if err != nil || len(rows) != 1 {
		t.Fatal("list after conflict", err)
	}
	var kept domain.Vehicle
	if err = json.Unmarshal(rows[0].Payload, &kept); err != nil || kept.Name != "Device A current edit" {
		t.Fatal("last arrival overwrote", kept)
	}
	fresh := stale
	fresh.Name = "Device B resolved edit"
	resolved, err := store.Apply(ctx, other, app.Batch{Epoch: 1, ConsentRevision: 1, Commands: []app.Command{vehicleCommand(t, fresh, 2)}})
	if err != nil || resolved.Results[0].Revision != 3 {
		t.Fatal("fresh revision resolution", err)
	}
	independent := app.Batch{Epoch: 1, ConsentRevision: 1, Commands: []app.Command{vehicleCommand(t, vehicle(t), 0), vehicleCommand(t, vehicle(t), 0)}}
	merged, err := store.Apply(ctx, p, independent)
	if err != nil || len(merged.Results) != 2 {
		t.Fatal("independent merge refused", err)
	}
	prefs := domain.Preferences{Version: 1, State: "MT", Municipality: "5103403", Locale: "pt-BR", RadiusKm: 3, ShowPriceHistory: true}
	rawPrefs, _ := json.Marshal(prefs)
	replayCmd := app.Command{OperationID: newid(t), Dataset: "preferences", ID: domain.PreferencesID, ExpectedRevision: 0, Payload: rawPrefs}
	first, err := store.Apply(ctx, p, app.Batch{Epoch: 1, ConsentRevision: 1, Commands: []app.Command{replayCmd}})
	if err != nil {
		t.Fatal(err)
	}
	again, err := store.Apply(ctx, p, app.Batch{Epoch: 1, ConsentRevision: 1, Commands: []app.Command{replayCmd}})
	if err != nil || !again.Results[0].Replayed || again.Results[0].Revision != first.Results[0].Revision {
		t.Fatal("lost response replay", err)
	}
	divergent := replayCmd
	changedPrefs := prefs
	changedPrefs.RadiusKm = 5
	divergent.Payload, _ = json.Marshal(changedPrefs)
	if _, err = store.Apply(ctx, p, app.Batch{Epoch: 1, ConsentRevision: 1, Commands: []app.Command{divergent}}); !errors.Is(err, domain.ErrConflict) {
		t.Fatal("changed operation reuse accepted")
	}
}

func TestPushDeleteConflictAndRollback(t *testing.T) {
	pool := freshDB(t)
	p := seedOwner(t, pool)
	store := paidStore(t, pool, p)
	p = readyDevice(t, store, p)
	other := secondDevice(t, pool, store, p)
	ctx := context.Background()
	store.Station = dirread.NewReader(pool).ActiveReference
	station := newid(t)
	if _, err := pool.Exec(ctx, "INSERT INTO directory_stations(id,display_name) VALUES($1,'Synthetic station')", station); err != nil {
		t.Fatal(err)
	}
	rawFav, _ := json.Marshal(domain.Favorite{StationID: station})
	createFav := app.Command{OperationID: newid(t), Dataset: "favorites", ID: station, ExpectedRevision: 0, Payload: rawFav}
	if _, err := store.Apply(ctx, p, app.Batch{Epoch: 1, ConsentRevision: 1, Commands: []app.Command{createFav}}); err != nil {
		t.Fatal(err)
	}
	deleteFav := app.Command{OperationID: newid(t), Dataset: "favorites", ID: station, ExpectedRevision: 1, Delete: true}
	if _, err := store.Apply(ctx, p, app.Batch{Epoch: 1, ConsentRevision: 1, Commands: []app.Command{deleteFav}}); err != nil {
		t.Fatal(err)
	}
	staleUpdate := app.Command{OperationID: newid(t), Dataset: "favorites", ID: station, ExpectedRevision: 1, Payload: rawFav}
	_, err := store.Apply(ctx, other, app.Batch{Epoch: 1, ConsentRevision: 1, Commands: []app.Command{staleUpdate}})
	var conflict *app.ConflictError
	if !errors.As(err, &conflict) || !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("delete versus update accepted %v", err)
	}
	if !conflict.Conflict.Erased || len(conflict.Conflict.CurrentPayload) != 0 || len(conflict.Conflict.ProposedPayload) == 0 {
		t.Fatalf("delete conflict missing erased fence %+v", conflict.Conflict)
	}
	var receiptBody string
	if err = pool.QueryRow(ctx, "SELECT coalesce(string_agg(receipt::text,''), '') FROM cloud_operations WHERE account_id=$1", p.AccountID).Scan(&receiptBody); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(receiptBody, "Synthetic station") || strings.Contains(receiptBody, "display_name") {
		t.Fatal("conflict receipt retained private content")
	}
	good := vehicleCommand(t, vehicle(t), 0)
	bad := vehicleCommand(t, vForRollback(t, pool, p, store), 99)
	before := counts(t, pool)
	if _, err = store.Apply(ctx, p, app.Batch{Epoch: 1, ConsentRevision: 1, Commands: []app.Command{good, bad}}); !errors.Is(err, domain.ErrConflict) {
		t.Fatal("partial batch accepted")
	}
	if after := counts(t, pool); after != before {
		t.Fatalf("conflict batch left effects %v want %v", after, before)
	}
	var success *time.Time
	if err = pool.QueryRow(ctx, "SELECT last_success_at FROM cloud_devices WHERE id=$1", p.DeviceID).Scan(&success); err != nil {
		t.Fatal(err)
	}
	store.Paid = nil
	if _, err = store.Apply(ctx, p, app.Batch{Epoch: 1, ConsentRevision: 1, Commands: []app.Command{vehicleCommand(t, vehicle(t), 0)}}); !errors.Is(err, domain.ErrDenied) {
		t.Fatal("expired entitlement push admitted")
	}
}

func vForRollback(t *testing.T, pool *pgxpool.Pool, p app.Principal, store *Store) domain.Vehicle {
	t.Helper()
	v := vehicle(t)
	ctx := context.Background()
	if _, err := store.Apply(ctx, p, app.Batch{Epoch: 1, ConsentRevision: 1, Commands: []app.Command{vehicleCommand(t, v, 0)}}); err != nil {
		t.Fatal(err)
	}
	return v
}

func counts(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	var records, changes, receipts int
	if err := pool.QueryRow(context.Background(), "SELECT (SELECT count(*) FROM cloud_records),(SELECT count(*) FROM cloud_changes),(SELECT count(*) FROM cloud_operations)").Scan(&records, &changes, &receipts); err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("%d/%d/%d", records, changes, receipts)
}

func TestCompactionOfflineEditAndAntiResurrection(t *testing.T) {
	pool := freshDB(t)
	p := seedOwner(t, pool)
	store := paidStore(t, pool, p)
	p = readyDevice(t, store, p)
	other := secondDevice(t, pool, store, p)
	ctx := context.Background()
	store.Station = dirread.NewReader(pool).ActiveReference
	v := vehicle(t)
	if _, err := store.Apply(ctx, p, app.Batch{Epoch: 1, ConsentRevision: 1, Commands: []app.Command{vehicleCommand(t, v, 0)}}); err != nil {
		t.Fatal(err)
	}
	station := newid(t)
	if _, err := pool.Exec(ctx, "INSERT INTO directory_stations(id,display_name) VALUES($1,'Synthetic station')", station); err != nil {
		t.Fatal(err)
	}
	rawFav, _ := json.Marshal(domain.Favorite{StationID: station})
	if _, err := store.Apply(ctx, p, app.Batch{Epoch: 1, ConsentRevision: 1, Commands: []app.Command{{OperationID: newid(t), Dataset: "favorites", ID: station, ExpectedRevision: 0, Payload: rawFav}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Apply(ctx, p, app.Batch{Epoch: 1, ConsentRevision: 1, Commands: []app.Command{{OperationID: newid(t), Dataset: "favorites", ID: station, ExpectedRevision: 1, Delete: true}}}); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-31 * 24 * time.Hour)
	if _, err := pool.Exec(ctx, "UPDATE cloud_changes SET created_at=$1 WHERE account_id=$2", old, p.AccountID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "UPDATE cloud_operations SET created_at=$1 WHERE account_id=$2", old, p.AccountID); err != nil {
		t.Fatal(err)
	}
	floor, err := store.Compact(ctx, p.AccountID, time.Now())
	if err != nil || floor == 0 {
		t.Fatalf("compact floor %d %v", floor, err)
	}
	var changes, receipts, fences int
	if err = pool.QueryRow(ctx, "SELECT (SELECT count(*) FROM cloud_changes WHERE account_id=$1),(SELECT count(*) FROM cloud_operations WHERE account_id=$1),(SELECT count(*) FROM cloud_erased_ids WHERE account_id=$1)", p.AccountID).Scan(&changes, &receipts, &fences); err != nil {
		t.Fatal(err)
	}
	if changes != 0 || receipts != 0 || fences == 0 {
		t.Fatalf("retention cleanup changes=%d receipts=%d fences=%d", changes, receipts, fences)
	}
	if _, err = store.Pull(ctx, p, app.Position{Version: 1, Epoch: 1, ConsentRevision: 1, After: 0}, 10); !errors.Is(err, domain.ErrResnapshot) {
		t.Fatal("compacted cursor silently advanced")
	}
	recreate := app.Command{OperationID: newid(t), Dataset: "favorites", ID: station, ExpectedRevision: 0, Payload: rawFav}
	if _, err = store.Apply(ctx, other, app.Batch{Epoch: 1, ConsentRevision: 1, Commands: []app.Command{recreate}}); !errors.Is(err, domain.ErrConflict) {
		t.Fatal("erased UUID resurrected via push")
	}
	if _, err = store.PutFavorite(ctx, other, station, 0); !errors.Is(err, domain.ErrConflict) {
		t.Fatal("erased UUID resurrected via foundation")
	}
	rows, err := store.List(ctx, p, "vehicles", 100, "")
	if err != nil || len(rows) != 1 || rows[0].ID != v.ID {
		t.Fatal("compaction cleared live vehicle", err)
	}
}

func TestCompactionRecreateParallelCrashAndPlans(t *testing.T) {
	pool := freshDB(t)
	p := seedOwner(t, pool)
	store := paidStore(t, pool, p)
	p = readyDevice(t, store, p)
	ctx := context.Background()
	v := vehicle(t)
	if _, err := store.Apply(ctx, p, app.Batch{Epoch: 1, ConsentRevision: 1, Commands: []app.Command{vehicleCommand(t, v, 0)}}); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-31 * 24 * time.Hour)
	if _, err := pool.Exec(ctx, "UPDATE cloud_changes SET created_at=$1 WHERE account_id=$2", old, p.AccountID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "UPDATE cloud_operations SET created_at=$1 WHERE account_id=$2", old, p.AccountID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `CREATE FUNCTION reject_compact_delete() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'synthetic'; END $$; CREATE TRIGGER reject_compact_delete BEFORE DELETE ON cloud_changes FOR EACH ROW EXECUTE FUNCTION reject_compact_delete()`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Compact(ctx, p.AccountID, time.Now()); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatal("crash reported success", err)
	}
	var floor int64
	if err := pool.QueryRow(ctx, "SELECT compacted_through FROM cloud_state WHERE account_id=$1", p.AccountID).Scan(&floor); err != nil || floor != 0 {
		t.Fatalf("crash advanced floor %d %v", floor, err)
	}
	if _, err := pool.Exec(ctx, "DROP TRIGGER reject_compact_delete ON cloud_changes"); err != nil {
		t.Fatal(err)
	}
	results := make(chan int64, 2)
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			f, e := store.Compact(ctx, p.AccountID, time.Now())
			results <- f
			errs <- e
		}()
	}
	wg.Wait()
	close(results)
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	floors := []int64{}
	for f := range results {
		floors = append(floors, f)
	}
	if len(floors) != 2 || floors[0] != floors[1] || floors[0] == 0 {
		t.Fatalf("parallel workers diverged %v", floors)
	}
	if _, err := store.Erase(ctx, p, "all", ""); err != nil {
		t.Fatal(err)
	}
	fresh := secondDevice(t, pool, store, p)
	if _, err := store.Pull(ctx, fresh, app.Position{Version: 1, Epoch: 1, ConsentRevision: 1, After: 0}, 10); !errors.Is(err, domain.ErrResnapshot) {
		t.Fatalf("recreated account cursor advanced %v", err)
	}
	if _, err := store.PutVehicle(ctx, fresh, v, 1); !errors.Is(err, domain.ErrConflict) {
		t.Fatal("erased vehicle resurrected via foundation")
	}
	granted, err := store.SetConsent(ctx, fresh, []string{"vehicles", "favorites", "preferences"}, true)
	if err != nil {
		t.Fatal(err)
	}
	archived := v
	archived.Archived = true
	if _, err := store.Apply(ctx, fresh, app.Batch{Epoch: granted.Epoch, ConsentRevision: granted.ConsentRevision, Commands: []app.Command{vehicleCommand(t, archived, 1)}}); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("erased vehicle updated %v", err)
	}
	plan, err := pool.Query(ctx, "EXPLAIN SELECT COALESCE(MAX(sequence),0) FROM cloud_changes WHERE account_id=$1 AND created_at<$2", p.AccountID, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	text := ""
	for plan.Next() {
		var line string
		if err = plan.Scan(&line); err != nil {
			t.Fatal(err)
		}
		text += line
	}
	plan.Close()
	if !strings.Contains(text, "Index") {
		t.Fatal("compaction floor did not use index", text)
	}
}

func TestImportPreviewPreservesUUIDsAndCounts(t *testing.T) {
	pool := freshDB(t)
	p := seedOwner(t, pool)
	store := paidStore(t, pool, p)
	p = readyDevice(t, store, p)
	ctx := context.Background()
	cloud := vehicle(t)
	if _, err := store.Apply(ctx, p, app.Batch{Epoch: 1, ConsentRevision: 1, Commands: []app.Command{vehicleCommand(t, cloud, 0)}}); err != nil {
		t.Fatal(err)
	}
	same := cloud
	guest := []app.Command{vehicleCommand(t, same, 0), vehicleCommand(t, vehicle(t), 0), vehicleCommand(t, vehicle(t), 0)}
	preview, err := store.Preview(ctx, p, app.Batch{Epoch: 1, ConsentRevision: 1, Commands: guest})
	if err != nil || len(preview.Items) != 3 {
		t.Fatalf("preview %v", err)
	}
	if preview.Items[0].Action != "converge" || preview.Items[1].Action != "create" || preview.Items[2].Action != "create" {
		t.Fatalf("preview merge %+v", preview.Items)
	}
	if after := counts(t, pool); after != "1/1/1" {
		t.Fatalf("preview uploaded guest %s", after)
	}
	divergent := cloud
	divergent.Name = "Guest divergent name"
	divPreview, err := store.Preview(ctx, p, app.Batch{Epoch: 1, ConsentRevision: 1, Commands: []app.Command{vehicleCommand(t, divergent, 0)}})
	if err != nil || divPreview.Items[0].Action != "conflict" {
		t.Fatalf("UUID conflict not reported %+v %v", divPreview, err)
	}
	fourth := app.Batch{Epoch: 1, ConsentRevision: 1, Commands: []app.Command{vehicleCommand(t, vehicle(t), 0), vehicleCommand(t, vehicle(t), 0), vehicleCommand(t, vehicle(t), 0)}}
	quotaPreview, err := store.Preview(ctx, p, app.Batch{Epoch: 1, ConsentRevision: 1, Commands: fourth.Commands})
	if err != nil {
		t.Fatal(err)
	}
	quota := 0
	for _, item := range quotaPreview.Items {
		if item.Action == "quota" {
			quota++
		}
	}
	if quota == 0 {
		t.Fatalf("fourth slot not reported %+v", quotaPreview.Items)
	}
	if _, err = store.Apply(ctx, p, app.Batch{Epoch: 1, ConsentRevision: 1, Commands: fourth.Commands}); !errors.Is(err, domain.ErrQuota) {
		t.Fatal("fourth vehicle admitted")
	}
	rows, err := store.List(ctx, p, "vehicles", 100, "")
	if err != nil || len(rows) != 1 || rows[0].ID != cloud.ID {
		t.Fatal("UUID rewritten", err)
	}
}

func TestImportStaleSnapshotAccountSwitchAndRevokedConsent(t *testing.T) {
	pool := freshDB(t)
	p := seedOwner(t, pool)
	foreign := seedOwner(t, pool)
	store := paidStore(t, pool, p)
	paidStore(t, pool, foreign)
	p = readyDevice(t, store, p)
	foreign = readyDevice(t, store, foreign)
	ctx := context.Background()
	v := vehicle(t)
	if _, err := store.Apply(ctx, p, app.Batch{Epoch: 1, ConsentRevision: 1, Commands: []app.Command{vehicleCommand(t, v, 0)}}); err != nil {
		t.Fatal(err)
	}
	before := counts(t, pool)
	cancelPreview, err := store.Preview(ctx, p, app.Batch{Epoch: 1, ConsentRevision: 1, Commands: []app.Command{vehicleCommand(t, vehicle(t), 0)}})
	if err != nil || cancelPreview.Items[0].Action != "create" {
		t.Fatal("preview", err)
	}
	if after := counts(t, pool); after != before {
		t.Fatal("canceled preview wrote")
	}
	if _, err = store.Preview(ctx, p, app.Batch{Epoch: 2, ConsentRevision: 1, Commands: []app.Command{vehicleCommand(t, vehicle(t), 0)}}); !errors.Is(err, domain.ErrResnapshot) {
		t.Fatal("stale preview epoch accepted")
	}
	if _, err = store.Preview(ctx, p, app.Batch{Epoch: 1, ConsentRevision: 9, Commands: []app.Command{vehicleCommand(t, vehicle(t), 0)}}); !errors.Is(err, domain.ErrDenied) {
		t.Fatal("stale preview consent accepted")
	}
	wrong := p
	wrong.FamilyID = foreign.FamilyID
	if _, err = store.Preview(ctx, wrong, app.Batch{Epoch: 1, ConsentRevision: 1, Commands: []app.Command{vehicleCommand(t, vehicle(t), 0)}}); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatal("account switch preview admitted")
	}
	status, err := store.SetConsent(ctx, p, []string{"vehicles"}, false)
	if err != nil || len(status.EnabledDatasets) != 2 {
		t.Fatalf("revoke %v %v", status, err)
	}
	if _, err = store.Apply(ctx, p, app.Batch{Epoch: status.Epoch, ConsentRevision: status.ConsentRevision, Commands: []app.Command{vehicleCommand(t, vehicle(t), 0)}}); !errors.Is(err, domain.ErrDenied) {
		t.Fatal("revoked consent push admitted")
	}
	rows, err := store.List(ctx, p, "vehicles", 100, "")
	if err != nil || len(rows) != 1 {
		t.Fatal("revoke cleared local cloud copy", err)
	}
	restored, err := store.SetConsent(ctx, p, []string{"vehicles"}, true)
	if err != nil || len(restored.EnabledDatasets) != 3 {
		t.Fatal("regrant", err)
	}
	if _, err = store.Preview(ctx, p, app.Batch{Epoch: 1, ConsentRevision: 1, Commands: []app.Command{vehicleCommand(t, vehicle(t), 0)}}); !errors.Is(err, domain.ErrDenied) {
		t.Fatal("obsolete preview version accepted")
	}
}

func TestSnapshotConsistentPagesAndResume(t *testing.T) {
	pool := freshDB(t)
	p := seedOwner(t, pool)
	foreign := seedOwner(t, pool)
	store := paidStore(t, pool, p)
	paidStore(t, pool, foreign)
	p = readyDevice(t, store, p)
	foreign = readyDevice(t, store, foreign)
	ctx := context.Background()
	firstVehicle := vehicle(t)
	if _, err := store.Apply(ctx, p, app.Batch{Epoch: 1, ConsentRevision: 1, Commands: []app.Command{vehicleCommand(t, firstVehicle, 0)}}); err != nil {
		t.Fatal(err)
	}
	secondVehicle := vehicle(t)
	if _, err := store.Apply(ctx, p, app.Batch{Epoch: 1, ConsentRevision: 1, Commands: []app.Command{vehicleCommand(t, secondVehicle, 0)}}); err != nil {
		t.Fatal(err)
	}
	first, err := store.Snapshot(ctx, p, app.SnapshotQuery{Dataset: "vehicles"}, 1)
	if err != nil || len(first.Records) != 1 || first.Through == 0 {
		t.Fatalf("snapshot first page %v %v", first, err)
	}
	through := first.Through
	after := first.Records[0].ID
	interleaved := firstVehicle
	interleaved.Name = "Interleaved write"
	if _, err = store.Apply(ctx, p, app.Batch{Epoch: 1, ConsentRevision: 1, Commands: []app.Command{vehicleCommand(t, interleaved, 1)}}); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Snapshot(ctx, p, app.SnapshotQuery{Dataset: "vehicles", After: after, Through: through, Epoch: 1, ConsentRevision: 1}, 10); !errors.Is(err, domain.ErrResnapshot) {
		t.Fatal("inconsistent snapshot page advanced")
	}
	restarted, err := store.Snapshot(ctx, p, app.SnapshotQuery{Dataset: "vehicles"}, 10)
	if err != nil || len(restarted.Records) != 2 {
		t.Fatalf("explicit restart %v %v", restarted, err)
	}
	if _, err = store.Snapshot(ctx, foreign, app.SnapshotQuery{Dataset: "vehicles"}, 10); err != nil {
		t.Fatal("foreign snapshot", err)
	} else {
		rows, _ := store.Snapshot(ctx, foreign, app.SnapshotQuery{Dataset: "vehicles"}, 10)
		if len(rows.Records) != 0 {
			t.Fatal("cross-owner snapshot leaked")
		}
	}
	forged := p
	forged.DeviceID = newid(t)
	if _, err = store.Snapshot(ctx, forged, app.SnapshotQuery{Dataset: "vehicles"}, 10); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatal("foreign device snapshot admitted")
	}
	store.Origin = "two.example.test"
	if _, err = store.Snapshot(ctx, p, app.SnapshotQuery{Dataset: "vehicles"}, 10); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatal("origin crossover snapshot admitted")
	}
	store.Origin = "one.example.test"
	if _, err = store.SetConsent(ctx, p, []string{"nope"}, true); !errors.Is(err, domain.ErrInvalid) {
		t.Fatal("direct mask accepted")
	}
	if _, err = store.Snapshot(ctx, p, app.SnapshotQuery{Dataset: "professional"}, 10); !errors.Is(err, domain.ErrInvalid) {
		t.Fatal("unknown dataset snapshot accepted")
	}
}
