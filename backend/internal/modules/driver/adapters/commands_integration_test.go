//go:build integration

package adapters

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	migrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	cloud "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/cloudsync/adapters"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/driver/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/driver/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type commandAuthority struct {
	principal app.Principal
	err       error
}

func (c commandAuthority) Verify(context.Context, app.Proof) (app.Principal, error) {
	return c.principal, c.err
}

func commandProof() app.Proof {
	return app.Proof{Signature: []byte("synthetic-signature"), RequestHash: "synthetic-request"}
}

func commandPrincipal(owner, origin string) app.Principal {
	return app.Principal{Owner: owner, Origin: origin, Contributor: "22222222-2222-4222-8222-222222222222", KeyID: "33333333-3333-4333-8333-333333333333"}
}

func commandService(store *Store, owner string) *app.CommandService {
	return &app.CommandService{Store: store, Authority: commandAuthority{principal: commandPrincipal(owner, "one.example.test")}, Origin: "one.example.test"}
}

func insertEarning(t *testing.T, p *pgxpool.Pool, owner, vehicle, coverage string, from, to time.Time, net int64) string {
	t.Helper()
	id := newDriverID(t)
	hash := sourceHash("source-" + id)
	content := sourceHash("content-" + id)
	if _, e := p.Exec(context.Background(), `INSERT INTO driver_earnings(account_id,id,vehicle_id,platform,source_hash,coverage_kind,currency,from_at,to_at,timezone,basis,net_centavos,content_sha256)
VALUES($1,$2,$3,'UBER',$4,$5,'BRL',$6,$7,'America/Cuiaba','INCLUSIVE_NET',$8,$9)`, owner, id, vehicle, hash, coverage, from, to, net, content); e != nil {
		t.Fatal(e)
	}
	return id
}

func TestDriverSignedCommandsCapabilityAndRetainedReads(t *testing.T) {
	ctx := context.Background()
	p, _ := freshDriverDB(t, migrations.Files)
	owner := seedDriverOwner(t, p)
	car := seedDriverVehicle(t, p, owner)
	store := New(p, cloud.RequireDriverOwnedVehicle)
	allow := true
	store.SetPaid(func(context.Context, pgx.Tx, string) error {
		if !allow {
			return app.ErrDenied
		}
		return nil
	})
	service := commandService(store, owner)
	first := earning(t, car)
	receipt, e := service.SaveEarning(ctx, commandProof(), owner, first)
	if e != nil || receipt.Replayed {
		t.Fatal("signed save", receipt, e)
	}
	allow = false
	before := driverCount(t, p, "driver_earnings")
	lapsed := earning(t, car)
	lapsed.Record.From = first.Record.To
	lapsed.Record.To = first.Record.To.Add(time.Hour)
	if _, e = service.SaveEarning(ctx, commandProof(), owner, lapsed); !errors.Is(e, app.ErrDenied) {
		t.Fatal("lapsed capability", e)
	}
	if driverCount(t, p, "driver_earnings") != before {
		t.Fatal("lapsed write left a row")
	}
	if got, e := service.GetEarning(ctx, commandProof(), owner, first.Record.ID); e != nil || got.Record.ID != first.Record.ID {
		t.Fatal("retained read after lapse", got, e)
	}
	invalid := &app.CommandService{Store: store, Authority: commandAuthority{err: errors.New("bad signature")}, Origin: "one.example.test"}
	if _, e = invalid.GetEarning(ctx, commandProof(), owner, first.Record.ID); !errors.Is(e, app.ErrSignedProof) {
		t.Fatal("invalid signature", e)
	}
	wrongOwner := &app.CommandService{Store: store, Authority: commandAuthority{principal: commandPrincipal(seedDriverOwner(t, p), "one.example.test")}, Origin: "one.example.test"}
	if _, e = wrongOwner.GetEarning(ctx, commandProof(), owner, first.Record.ID); !errors.Is(e, app.ErrSignedProof) {
		t.Fatal("owner mismatch", e)
	}
	wrongOrigin := &app.CommandService{Store: store, Authority: commandAuthority{principal: commandPrincipal(owner, "one.example.test")}, Origin: "other.example.test"}
	if _, e = wrongOrigin.GetEarning(ctx, commandProof(), owner, first.Record.ID); !errors.Is(e, app.ErrSignedProof) {
		t.Fatal("origin mismatch", e)
	}
	foreignOwner := seedDriverOwner(t, p)
	foreignCar := seedDriverVehicle(t, p, foreignOwner)
	if _, e = service.GetEarning(ctx, commandProof(), owner, newDriverID(t)); !errors.Is(e, app.ErrNotFound) {
		t.Fatal("foreign/unknown record", e)
	}
	allow = true
	if _, e = service.SaveEarning(ctx, commandProof(), owner, earning(t, foreignCar)); !errors.Is(e, app.ErrDenied) {
		t.Fatal("foreign vehicle", e)
	}
	if _, e = p.Exec(ctx, "UPDATE accounts SET status='suspended' WHERE id=$1", owner); e != nil {
		t.Fatal(e)
	}
	if _, e = service.SaveEarning(ctx, commandProof(), owner, earning(t, car)); !errors.Is(e, app.ErrDenied) {
		t.Fatal("suspended account write", e)
	}
}

func reconciliationDecision(id string, kind, primary string, counterparts []domain.ReconciledRecord, payout string, from, to time.Time) domain.ReconciliationDecision {
	return domain.ReconciliationDecision{ID: id, Version: 1, Kind: kind, Platform: "UBER", From: from, To: to, PrimaryID: primary, PrimaryRevision: 1, Counterparts: counterparts, PayoutSourceHash: payout}
}

func TestDriverReconciliationDecisionsVersioningAndErasure(t *testing.T) {
	ctx := context.Background()
	p, _ := freshDriverDB(t, migrations.Files)
	owner := seedDriverOwner(t, p)
	car := seedDriverVehicle(t, p, owner)
	from := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	to := from.Add(time.Hour)
	summary := insertEarning(t, p, owner, car, "SUMMARY", from, to, 45_000)
	detail := insertEarning(t, p, owner, car, "DETAIL", from.Add(15*time.Minute), from.Add(45*time.Minute), 45_000)
	summary2 := insertEarning(t, p, owner, car, "SUMMARY", from.Add(20*time.Minute), from.Add(50*time.Minute), 45_000)
	foreignOwner := seedDriverOwner(t, p)
	foreignCar := seedDriverVehicle(t, p, foreignOwner)
	foreign := insertEarning(t, p, foreignOwner, foreignCar, "DETAIL", from, to, 45_000)
	store := New(p, cloud.RequireDriverOwnedVehicle)
	store.SetPaid(func(context.Context, pgx.Tx, string) error { return nil })
	service := commandService(store, owner)
	id := newDriverID(t)
	decision := reconciliationDecision(id, "SUMMARY_DETAIL", summary, []domain.ReconciledRecord{{ID: detail, Revision: 1}}, "", from, to)
	in := app.SaveReconciliation{OperationID: newDriverID(t), ExpectedVersion: 0, ExpectedEpoch: 0, Decision: decision}
	receipt, e := service.SaveReconciliation(ctx, commandProof(), owner, in)
	if e != nil || receipt.Version != 1 {
		t.Fatal("decision", receipt, e)
	}
	if replay, e := service.SaveReconciliation(ctx, commandProof(), owner, in); e != nil || !replay.Replayed {
		t.Fatal("decision retry", replay, e)
	}
	got, e := service.GetReconciliation(ctx, commandProof(), owner, id, 1)
	if e != nil || got.PrimaryID != summary || len(got.Counterparts) != 1 || got.Counterparts[0].ID != detail {
		t.Fatalf("decision read %+v %v", got, e)
	}
	if list, e := service.ListReconciliations(ctx, commandProof(), owner, "", 10); e != nil || len(list) != 1 {
		t.Fatal("decision list", list, e)
	}
	changed := in
	changed.Decision.To = to.Add(time.Minute)
	if _, e := service.SaveReconciliation(ctx, commandProof(), owner, changed); !errors.Is(e, domain.ErrConflict) {
		t.Fatal("changed body reused operation", e)
	}
	stale := in
	stale.OperationID = newDriverID(t)
	if _, e := service.SaveReconciliation(ctx, commandProof(), owner, stale); !errors.Is(e, domain.ErrConflict) {
		t.Fatal("stale decision version", e)
	}
	foreignDecision := reconciliationDecision(newDriverID(t), "SUMMARY_DETAIL", foreign, []domain.ReconciledRecord{{ID: detail, Revision: 1}}, "", from, to)
	foreignIn := app.SaveReconciliation{OperationID: newDriverID(t), ExpectedVersion: 0, ExpectedEpoch: 0, Decision: foreignDecision}
	if _, e := service.SaveReconciliation(ctx, commandProof(), owner, foreignIn); !errors.Is(e, app.ErrNotFound) {
		t.Fatal("foreign decision record", e)
	}
	wrongRevision := reconciliationDecision(newDriverID(t), "SUMMARY_DETAIL", summary, []domain.ReconciledRecord{{ID: detail, Revision: 2}}, "", from, to)
	wrongIn := app.SaveReconciliation{OperationID: newDriverID(t), ExpectedVersion: 0, ExpectedEpoch: 0, Decision: wrongRevision}
	if _, e := service.SaveReconciliation(ctx, commandProof(), owner, wrongIn); !errors.Is(e, domain.ErrConflict) {
		t.Fatal("stale decision revision", e)
	}
	notMixed := reconciliationDecision(newDriverID(t), "SUMMARY_DETAIL", summary, []domain.ReconciledRecord{{ID: summary2, Revision: 1}}, "", from, to)
	notMixedIn := app.SaveReconciliation{OperationID: newDriverID(t), ExpectedVersion: 0, ExpectedEpoch: 0, Decision: notMixed}
	if _, e := service.SaveReconciliation(ctx, commandProof(), owner, notMixedIn); !errors.Is(e, domain.ErrConflict) {
		t.Fatal("summary-only detail decision", e)
	}
	payout := reconciliationDecision(newDriverID(t), "PAYOUT_EARNINGS", summary, nil, sourceHash("payout-source"), from, to)
	payoutIn := app.SaveReconciliation{OperationID: newDriverID(t), ExpectedVersion: 0, ExpectedEpoch: 0, Decision: payout}
	if _, e := service.SaveReconciliation(ctx, commandProof(), owner, payoutIn); e != nil {
		t.Fatal("payout decision", e)
	}
	missingPayout := reconciliationDecision(newDriverID(t), "PAYOUT_EARNINGS", summary, nil, "", from, to)
	if _, e := service.SaveReconciliation(ctx, commandProof(), owner, app.SaveReconciliation{OperationID: newDriverID(t), ExpectedVersion: 0, ExpectedEpoch: 0, Decision: missingPayout}); !errors.Is(e, domain.ErrInvalid) {
		t.Fatal("payout without source", e)
	}
	if e := service.EraseEarningOwner(ctx, commandProof(), owner, 0); e != nil {
		t.Fatal(e)
	}
	if _, e := service.GetReconciliation(ctx, commandProof(), owner, id, 1); !errors.Is(e, app.ErrNotFound) {
		t.Fatal("owner erase left a decision", e)
	}
	if _, e := service.SaveReconciliation(ctx, commandProof(), owner, in); !errors.Is(e, app.ErrErased) {
		t.Fatal("stale decision epoch", e)
	}
}

func TestDriverCommandDuplicateCancellationAndStaleEpoch(t *testing.T) {
	ctx := context.Background()
	p, _ := freshDriverDB(t, migrations.Files)
	owner := seedDriverOwner(t, p)
	car := seedDriverVehicle(t, p, owner)
	store := New(p, cloud.RequireDriverOwnedVehicle)
	store.SetPaid(func(context.Context, pgx.Tx, string) error { return nil })
	service := commandService(store, owner)
	in := earning(t, car)
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, e := service.SaveEarning(ctx, commandProof(), owner, in)
			results <- e
		}()
	}
	wg.Wait()
	close(results)
	for e := range results {
		if e != nil {
			t.Fatal("concurrent duplicate", e)
		}
	}
	if driverCount(t, p, "driver_earnings") != 1 || driverCount(t, p, "driver_earning_commands") != 1 {
		t.Fatal("duplicate save persisted twice")
	}
	updated := in
	updated.OperationID = newDriverID(t)
	updated.ExpectedRevision = 1
	updated.Record.Money.Net = new(int64)
	*updated.Record.Money.Net = 44_000
	if _, e := service.SaveEarning(ctx, commandProof(), owner, updated); e != nil {
		t.Fatal("revision update", e)
	}
	beforeCancelEarnings, beforeCancelCommands := driverCount(t, p, "driver_earnings"), driverCount(t, p, "driver_earning_commands")
	adj := app.SaveAdjustment{OperationID: newDriverID(t), SourceHash: sourceHash("adjustment"), ExpectedEpoch: 0, Adjustment: domain.Adjustment{ID: newDriverID(t), TargetID: in.Record.ID, Kind: "CORRECTION", TargetRevision: 1, Delta: 100}}
	if _, e := service.AdjustEarning(ctx, commandProof(), owner, adj); !errors.Is(e, domain.ErrConflict) {
		t.Fatal("stale correction accepted", e)
	}
	if driverCount(t, p, "driver_earning_adjustments") != 0 {
		t.Fatal("stale correction left a row")
	}
	cancelOwner := seedDriverOwner(t, p)
	cancelCar := seedDriverVehicle(t, p, cancelOwner)
	cancelStore := New(p, cloud.RequireDriverOwnedVehicle)
	cancelStore.SetPaid(func(context.Context, pgx.Tx, string) error { return context.Canceled })
	cancelService := commandService(cancelStore, cancelOwner)
	if _, e := cancelService.SaveEarning(ctx, commandProof(), cancelOwner, earning(t, cancelCar)); !errors.Is(e, context.Canceled) {
		t.Fatal("cancellation", e)
	}
	if driverCount(t, p, "driver_earnings") != beforeCancelEarnings || driverCount(t, p, "driver_earning_commands") != beforeCancelCommands {
		t.Fatal("cancellation left partial state")
	}
	staleOwner := seedDriverOwner(t, p)
	staleCar := seedDriverVehicle(t, p, staleOwner)
	staleService := commandService(store, staleOwner)
	staleIn := earning(t, staleCar)
	if _, e := staleService.SaveEarning(ctx, commandProof(), staleOwner, staleIn); e != nil {
		t.Fatal(e)
	}
	if e := staleService.EraseEarningOwner(ctx, commandProof(), staleOwner, 0); e != nil {
		t.Fatal(e)
	}
	staleIn.OperationID = newDriverID(t)
	if _, e := staleService.SaveEarning(ctx, commandProof(), staleOwner, staleIn); !errors.Is(e, app.ErrErased) {
		t.Fatal("old epoch restored", e)
	}
}
