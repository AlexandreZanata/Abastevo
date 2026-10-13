//go:build integration

package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/cloudsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/cloudsync/domain"
	dirread "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/adapters/read"
)

func TestBackupManifestConsistentRead(t *testing.T) {
	pool := freshDB(t)
	p := seedOwner(t, pool)
	store := paidStore(t, pool, p)
	p = readyDevice(t, store, p)
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
	prefs := domain.Preferences{Version: 1, State: "MT", Municipality: "5103403", Locale: "pt-BR", RadiusKm: 3, ShowPriceHistory: true}
	rawPrefs, _ := json.Marshal(prefs)
	if _, err := store.Apply(ctx, p, app.Batch{Epoch: 1, ConsentRevision: 1, Commands: []app.Command{{OperationID: newid(t), Dataset: "preferences", ID: domain.PreferencesID, ExpectedRevision: 0, Payload: rawPrefs}}}); err != nil {
		t.Fatal(err)
	}
	first, err := store.Manifest(ctx, p)
	if err != nil || first.Version != 1 || first.Revision != 3 || first.VehicleCount != 1 || first.FavoriteCount != 1 || first.PreferenceCount != 1 {
		t.Fatalf("manifest %+v %v", first, err)
	}
	if len(first.Checksum) != 64 || len(first.Datasets) != 3 || len(first.Excluded) == 0 {
		t.Fatalf("manifest envelope %+v", first)
	}
	for _, forbidden := range []string{"credentials", "photos", "professional"} {
		found := false
		for _, excluded := range first.Excluded {
			if excluded == forbidden {
				found = true
			}
		}
		if !found {
			t.Fatalf("exclusion missing %s", forbidden)
		}
	}
	preview, err := store.Preview(ctx, p, app.Batch{Epoch: 1, ConsentRevision: 1, Commands: []app.Command{vehicleCommand(t, vehicle(t), 0)}})
	if err != nil || len(preview.Items) != 1 {
		t.Fatal("preview", err)
	}
	again, err := store.Manifest(ctx, p)
	if err != nil || again.Checksum != first.Checksum || again.Revision != first.Revision {
		t.Fatal("pending changes reported protected")
	}
	updated := v
	updated.Name = "Write during snapshot"
	if _, err = store.Apply(ctx, p, app.Batch{Epoch: 1, ConsentRevision: 1, Commands: []app.Command{vehicleCommand(t, updated, 1)}}); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Snapshot(ctx, p, app.SnapshotQuery{Dataset: "vehicles", Through: first.Revision, Epoch: first.Epoch, ConsentRevision: first.ConsentRevision}, 10); !errors.Is(err, domain.ErrResnapshot) {
		t.Fatal("inconsistent dataset revisions advanced")
	}
	second, err := store.Manifest(ctx, p)
	if err != nil || second.Revision != 4 || second.Checksum == first.Checksum {
		t.Fatalf("manifest after write %+v %v", second, err)
	}
	if _, err = pool.Exec(ctx, "INSERT INTO cloud_records(account_id,dataset,id,payload) VALUES($1,'vehicles',$2,'{}')", p.AccountID, newid(t)); err != nil {
		t.Fatal(err)
	}
	third, err := store.Manifest(ctx, p)
	if err != nil || third.VehicleCount != 2 {
		t.Fatalf("oversize unknown schema %+v %v", third, err)
	}
	if _, err = pool.Exec(ctx, "INSERT INTO cloud_records(account_id,dataset,id,payload) VALUES($1,'professional',$2,'{}')", p.AccountID, newid(t)); err == nil {
		t.Fatal("unknown dataset accepted")
	}
	var payloads string
	if err = pool.QueryRow(ctx, "SELECT coalesce(string_agg(payload::text,''), '') FROM cloud_records WHERE account_id=$1 AND NOT deleted", p.AccountID).Scan(&payloads); err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"password", "credential", "gps", "permission"} {
		if strings.Contains(strings.ToLower(payloads), forbidden) {
			t.Fatalf("forbidden field persisted %s", forbidden)
		}
	}
}

func TestBackupManifestLapseAndIsolation(t *testing.T) {
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
	isolated, err := store.Manifest(ctx, foreign)
	if err != nil || isolated.VehicleCount != 0 || isolated.Revision != 0 {
		t.Fatalf("cross-owner manifest leaked %+v %v", isolated, err)
	}
	forged := p
	forged.DeviceID = newid(t)
	if _, err = store.Manifest(ctx, forged); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatal("foreign device manifest admitted")
	}
	store.Origin = "two.example.test"
	if _, err = store.Manifest(ctx, p); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatal("origin crossover manifest admitted")
	}
	store.Origin = "one.example.test"
	store.Paid = nil
	if _, err = store.Manifest(ctx, p); !errors.Is(err, domain.ErrDenied) {
		t.Fatal("lapsed manifest admitted")
	}
}
