//go:build integration

package adapters

import (
	"context"
	"encoding/json"
	"errors"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/cloudsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/cloudsync/domain"
	"github.com/jackc/pgx/v5"
	"testing"
)

func TestMEX07CloudOnlyCannotCreateAdditionalVehicle(t *testing.T) {
	pool := freshDB(t)
	owner := seedOwner(t, pool)
	paid := paidStore(t, pool, owner)
	// Configure only cloud authority; no additional-registration proof is supplied.
	store := NewStore(pool)
	store.Paid = paid.Paid
	ctx := context.Background()
	first := vehicle(t)
	if _, err := store.PutVehicle(ctx, owner, first, 0); err != nil {
		t.Fatal(err)
	}
	before := counts(t, pool)
	if _, err := store.PutVehicle(ctx, owner, vehicle(t), 0); !errors.Is(err, domain.ErrDenied) {
		t.Fatalf("additional registration without capability: %v", err)
	}
	if after := counts(t, pool); after != before {
		t.Fatal("denied admission changed records")
	}
	if _, err := store.PutVehicle(ctx, owner, first, 0); err != nil {
		t.Fatal("retained same-ID replay", err)
	}
	first.Name = "Retained correction"
	if _, err := store.PutVehicle(ctx, owner, first, 1); err != nil {
		t.Fatal("retained correction", err)
	}
}

func TestMEX07ImportIsAtomicWithoutMultipleCapability(t *testing.T) {
	pool := freshDB(t)
	owner := seedOwner(t, pool)
	paid := paidStore(t, pool, owner)
	store := NewStore(pool)
	store.Paid = paid.Paid
	owner = readyDevice(t, store, owner)
	before := counts(t, pool)
	_, err := store.Apply(context.Background(), owner, app.Batch{Epoch: 1, ConsentRevision: 1, Commands: []app.Command{vehicleCommand(t, vehicle(t), 0), vehicleCommand(t, vehicle(t), 0)}})
	if !errors.Is(err, domain.ErrDenied) {
		t.Fatalf("two-ID import without capability: %v", err)
	}
	if after := counts(t, pool); after != before {
		t.Fatal("refused batch left records, receipts or changes")
	}
}

func TestMEX07CloudOnlyConcurrentOneSlotAndReplay(t *testing.T) {
	pool := freshDB(t)
	owner := seedOwner(t, pool)
	paid := paidStore(t, pool, owner)
	store := NewStore(pool)
	store.Paid = paid.Paid
	owner = readyDevice(t, store, owner)
	commands := make([]app.Command, 12)
	for i := range commands {
		commands[i] = vehicleCommand(t, vehicle(t), 0)
	}
	start := make(chan struct{})
	results := make(chan struct {
		index int
		err   error
	}, len(commands))
	for i, c := range commands {
		go func(index int, command app.Command) {
			<-start
			_, err := store.Apply(context.Background(), owner, app.Batch{Epoch: 1, ConsentRevision: 1, Commands: []app.Command{command}})
			results <- struct {
				index int
				err   error
			}{index, err}
		}(i, c)
	}
	close(start)
	winner := -1
	wins := 0
	for range commands {
		r := <-results
		if r.err == nil {
			wins++
			winner = r.index
		} else if !errors.Is(r.err, domain.ErrDenied) {
			t.Fatal(r.err)
		}
	}
	if wins != 1 || counts(t, pool) != "1/1/1" {
		t.Fatalf("wins %d counts %s", wins, counts(t, pool))
	}
	batch := app.Batch{Epoch: 1, ConsentRevision: 1, Commands: []app.Command{commands[winner]}}
	got, err := store.Apply(context.Background(), owner, batch)
	if err != nil || !got.Results[0].Replayed {
		t.Fatal("same operation replay", err)
	}
	batch.Commands[0].Payload = restorePayload(t, vehicle(t))
	// Keep the payload ID valid while changing operation identity to a different record.
	var v domain.Vehicle
	if err = json.Unmarshal(batch.Commands[0].Payload, &v); err != nil {
		t.Fatal(err)
	}
	batch.Commands[0].ID = v.ID
	if _, err = store.Apply(context.Background(), owner, batch); !errors.Is(err, domain.ErrConflict) {
		t.Fatal("divergent operation replay", err)
	}
	foreign := seedOwner(t, pool)
	wrong := owner
	wrong.FamilyID = foreign.FamilyID
	if _, err = store.Apply(context.Background(), wrong, app.Batch{Epoch: 1, ConsentRevision: 1, Commands: []app.Command{vehicleCommand(t, vehicle(t), 0)}}); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatal("foreign session", err)
	}
}

func TestMEX07LegacyOverLimitEditsRemainAvailable(t *testing.T) {
	pool := freshDB(t)
	owner := seedOwner(t, pool)
	paid := paidStore(t, pool, owner)
	store := NewStore(pool)
	store.Paid = paid.Paid
	var first domain.Vehicle
	for i := 0; i < 4; i++ {
		v := vehicle(t)
		v.Archived = true
		if i == 0 {
			first = v
		}
		if _, err := pool.Exec(context.Background(), "INSERT INTO cloud_records(account_id,dataset,id,payload) VALUES($1,'vehicles',$2,$3)", owner.AccountID, v.ID, restorePayload(t, v)); err != nil {
			t.Fatal(err)
		}
	}
	first.Name = "Retained legacy correction"
	if _, err := store.PutVehicle(context.Background(), owner, first, 1); err != nil {
		t.Fatal("legacy correction", err)
	}
	if _, err := store.PutVehicle(context.Background(), owner, vehicle(t), 0); !errors.Is(err, domain.ErrQuota) {
		t.Fatal("legacy quota", err)
	}
	rows, err := store.List(context.Background(), owner, "vehicles", 100, "")
	if err != nil || len(rows) != 4 {
		t.Fatal("legacy records hidden/deleted", err)
	}
}

func TestMEX07CapabilityExpiryRollsBackEveryAdmissionPath(t *testing.T) {
	for _, path := range []string{"foundation", "batch", "restore"} {
		t.Run(path, func(t *testing.T) {
			pool := freshDB(t)
			owner := seedOwner(t, pool)
			store := paidStore(t, pool, owner)
			owner = readyDevice(t, store, owner)
			ctx := context.Background()
			base := vehicle(t)
			if _, err := store.Apply(ctx, owner, app.Batch{Epoch: 1, ConsentRevision: 1, Commands: []app.Command{vehicleCommand(t, base, 0)}}); err != nil {
				t.Fatal(err)
			}
			next := vehicle(t)
			command := vehicleCommand(t, next, 0)
			var snapshot string
			var preview app.RestorePreview
			restoreID := newid(t)
			if path == "restore" {
				store.Objects = newFakeObjects()
				snapshot, preview = commitSnapshot(t, store, owner, 1, []restoreRecord{{Dataset: "vehicles", ID: next.ID, Revision: 1, Payload: restorePayload(t, next)}})
			}
			gate := store.MultipleVehicles
			calls := 0
			store.MultipleVehicles = func(ctx context.Context, tx pgx.Tx, id string) error {
				calls++
				if calls == 2 {
					if _, err := tx.Exec(ctx, "UPDATE subscription_purchases SET valid_until=clock_timestamp() WHERE account_id=$1", id); err != nil {
						return err
					}
				}
				return gate(ctx, tx, id)
			}
			attempt := func() error {
				switch path {
				case "foundation":
					_, err := store.PutVehicle(ctx, owner, next, 0)
					return err
				case "batch":
					_, err := store.Apply(ctx, owner, app.Batch{Epoch: 1, ConsentRevision: 1, Commands: []app.Command{command}})
					return err
				default:
					_, err := store.RestoreCommit(ctx, owner, snapshot, restoreID, preview.CurrentRevision, preview.ExpiresAt)
					return err
				}
			}
			before := counts(t, pool)
			if err := attempt(); !errors.Is(err, domain.ErrDenied) {
				t.Fatalf("expired proof accepted: %v", err)
			}
			if calls != 2 || before != counts(t, pool) {
				t.Fatalf("no final recheck/partial effects: calls=%d counts=%s", calls, counts(t, pool))
			}
			var receipts int
			if err := pool.QueryRow(ctx, "SELECT count(*) FROM cloud_restores WHERE account_id=$1", owner.AccountID).Scan(&receipts); err != nil || receipts != 0 {
				t.Fatal("failed restore receipt", err)
			}
			store.MultipleVehicles = gate
			if err := attempt(); err != nil {
				t.Fatal("rollback/retry failed", err)
			}
		})
	}
}

func TestMEX07PreviewAndRestoreRecheckSlots(t *testing.T) {
	pool := freshDB(t)
	owner := seedOwner(t, pool)
	store := paidStore(t, pool, owner)
	owner = readyDevice(t, store, owner)
	ctx := context.Background()
	initial := vehicle(t)
	if _, err := store.Apply(ctx, owner, app.Batch{Epoch: 1, ConsentRevision: 1, Commands: []app.Command{vehicleCommand(t, initial, 0)}}); err != nil {
		t.Fatal(err)
	}
	store.Objects = newFakeObjects()
	v := vehicle(t)
	snapshot, preview := commitSnapshot(t, store, owner, 1, []restoreRecord{{Dataset: "vehicles", ID: v.ID, Revision: 1, Payload: restorePayload(t, v)}})
	store.MultipleVehicles = nil
	before := counts(t, pool)
	fresh, err := store.PreviewRestore(ctx, owner, snapshot)
	if err != nil || fresh.Compatible || len(fresh.Issues) != 1 || fresh.Issues[0] != "vehicle-capability-required" {
		t.Fatalf("restore preview %+v %v", fresh, err)
	}
	if _, err = store.RestoreCommit(ctx, owner, snapshot, newid(t), preview.CurrentRevision, preview.ExpiresAt); !errors.Is(err, domain.ErrDenied) {
		t.Fatal("old preview authorized restore", err)
	}
	cmd := vehicleCommand(t, v, 0)
	view, err := store.Preview(ctx, owner, app.Batch{Epoch: 1, ConsentRevision: 1, Commands: []app.Command{cmd}})
	if err != nil || view.Items[0].Action != "quota" {
		t.Fatalf("import preview %+v %v", view, err)
	}
	if before != counts(t, pool) {
		t.Fatal("preview or failed restore changed data")
	}
}

func TestMEX07PreviewDoesNotCountDuplicateOperationTwice(t *testing.T) {
	pool := freshDB(t)
	owner := seedOwner(t, pool)
	paid := paidStore(t, pool, owner)
	store := NewStore(pool)
	store.Paid = paid.Paid
	owner = readyDevice(t, store, owner)
	cmd := vehicleCommand(t, vehicle(t), 0)
	batch := app.Batch{Epoch: 1, ConsentRevision: 1, Commands: []app.Command{cmd, cmd}}
	view, err := store.Preview(context.Background(), owner, batch)
	if err != nil || view.Items[0].Action != "create" || view.Items[1].Action != "converge" {
		t.Fatalf("duplicate ID preview %+v %v", view, err)
	}
	if _, err = store.Apply(context.Background(), owner, batch); err != nil {
		t.Fatal(err)
	}
	if counts(t, pool) != "1/1/1" {
		t.Fatal("duplicate ID consumed slots")
	}
}
