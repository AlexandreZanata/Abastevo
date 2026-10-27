//go:build integration

package adapters

import (
	"context"
	"errors"
	"testing"
	"time"

	migrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	cloud "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/cloudsync/adapters"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/driver/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/driver/domain"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestDriverReportRevisionSnapshotAndStale(t *testing.T) {
	ctx := context.Background()
	p, _ := freshDriverDB(t, migrations.Files)
	owner := seedDriverOwner(t, p)
	car := seedDriverVehicle(t, p, owner)
	store := New(p, cloud.RequireDriverOwnedVehicle)
	in := earning(t, car)
	if _, e := store.Save(ctx, owner, in); e != nil {
		t.Fatal(e)
	}
	from := time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC)
	to := from.Add(24 * time.Hour)
	distance := distanceRecord(t, car, "ODOMETER", "MEASURED", from, to)
	metres, start, end := int64(500_000), int64(0), int64(500_000)
	distance.Metres, distance.StartOdometerMetres, distance.EndOdometerMetres = &metres, &start, &end
	if _, e := store.SaveDistance(ctx, owner, app.SaveDistance{OperationID: newDriverID(t), Record: distance}); e != nil {
		t.Fatal(e)
	}
	modeled := int64(10_000)
	profile := domain.VehicleCostProfile{Vehicle: car, Ownership: "OWNED", Timezone: "America/Cuiaba", From: "2026-10-26", Until: "2026-10-27", Version: 1, Costs: []domain.ProfileCost{{ID: "reserve-1", Kind: "RESERVE", Provenance: "MODELED", From: "2026-10-26", Until: "2026-10-27", Amount: &modeled}}}
	if _, e := store.SaveCostProfile(ctx, owner, app.SaveCostProfile{OperationID: newDriverID(t), ExpectedVersion: 0, ExpectedEpoch: 0, Profile: profile}); e != nil {
		t.Fatal(e)
	}
	now := time.Date(2026, 10, 26, 18, 0, 0, 0, time.UTC)
	allow := true
	service := &app.ReportService{Store: store, Snapshots: NewReportStore(p), Now: func() time.Time { return now }, Paid: func(context.Context, string) error {
		if !allow {
			return app.ErrDenied
		}
		return nil
	}}
	req := app.ReportRequest{VehicleID: car, Timezone: "UTC", Period: "DAY", Anchor: "2026-10-26", Basis: "ODOMETER"}
	report, e := service.Build(ctx, owner, req)
	if e != nil {
		t.Fatal(e)
	}
	if report.EarnedCentavos == nil || *report.EarnedCentavos != 45_000 || report.ModeledCostsCentavos == nil || *report.ModeledCostsCentavos != 10_000 || report.RecordedResultCentavos == nil || *report.RecordedResultCentavos != 45_000 || report.ModeledResultCentavos == nil || *report.ModeledResultCentavos != 35_000 || report.DistanceMetres == nil || *report.DistanceMetres != 500_000 || report.RecordedPerKm != "90" || report.ModeledPerKm != "70" || report.SnapshotVersion != 1 {
		t.Fatalf("report %+v", report)
	}
	if report.Coverage["result"] != "complete" || report.Coverage["distance"] != "complete" || report.Coverage["time"] != "time-not-projected" {
		t.Fatalf("coverage %+v", report.Coverage)
	}
	got, e := service.Get(ctx, owner, req)
	if e != nil || got.Revision != report.Revision || got.SnapshotVersion != 1 {
		t.Fatal("get", got, e)
	}
	allow = false
	if _, e := service.Build(ctx, owner, req); !errors.Is(e, app.ErrDenied) {
		t.Fatal("lapsed report generation", e)
	}
	if _, e := service.Get(ctx, owner, req); e != nil {
		t.Fatal("retained report read", e)
	}
	allow = true
	updated := in
	updated.OperationID = newDriverID(t)
	updated.ExpectedRevision = 1
	updated.Record.Money.Net = new(int64)
	*updated.Record.Money.Net = 46_000
	if _, e := store.Save(ctx, owner, updated); e != nil {
		t.Fatal(e)
	}
	if _, e := service.Get(ctx, owner, req); !errors.Is(e, app.ErrStale) {
		t.Fatal("stale snapshot admitted", e)
	}
	staleReq := req
	staleReq.ExpectedRevision = report.Revision
	if _, e := service.Build(ctx, owner, staleReq); !errors.Is(e, app.ErrStale) {
		t.Fatal("stale build revision admitted", e)
	}
	fresh, e := service.Build(ctx, owner, req)
	if e != nil || fresh.Revision == report.Revision || *fresh.EarnedCentavos != 46_000 {
		t.Fatal("fresh build", fresh, e)
	}
	if got, e := service.Get(ctx, owner, req); e != nil || got.Revision != fresh.Revision {
		t.Fatal("fresh snapshot", got, e)
	}
}

func TestDriverReportSnapshotRestartExpiryAndAccountErasure(t *testing.T) {
	ctx := context.Background()
	p, dsn := freshDriverDB(t, migrations.Files)
	owner := seedDriverOwner(t, p)
	car := seedDriverVehicle(t, p, owner)
	store := New(p, cloud.RequireDriverOwnedVehicle)
	if _, e := store.Save(ctx, owner, earning(t, car)); e != nil {
		t.Fatal(e)
	}
	req := app.ReportRequest{VehicleID: car, Timezone: "UTC", Period: "DAY", Anchor: "2026-10-26", Basis: "ODOMETER"}
	service := &app.ReportService{Store: store, Snapshots: NewReportStore(p), Paid: func(context.Context, string) error { return nil }}
	if _, e := service.Build(ctx, owner, req); e != nil {
		t.Fatal(e)
	}
	p.Close()
	restarted, e := pgxpool.New(ctx, dsn)
	if e != nil {
		t.Fatal(e)
	}
	defer restarted.Close()
	restartedStore := New(restarted, cloud.RequireDriverOwnedVehicle)
	restartedReports := &app.ReportService{Store: restartedStore, Snapshots: NewReportStore(restarted), Paid: func(context.Context, string) error { return nil }}
	if _, e := restartedReports.Get(ctx, owner, req); e != nil {
		t.Fatal("restart", e)
	}
	if _, e := restarted.Exec(ctx, "UPDATE accounts SET status='deleted' WHERE id=$1", owner); e != nil {
		t.Fatal(e)
	}
	if driverCount(t, restarted, "driver_report_snapshots") != 0 {
		t.Fatal("soft deletion retained snapshots")
	}
	owner2 := seedDriverOwner(t, restarted)
	car2 := seedDriverVehicle(t, restarted, owner2)
	store2 := New(restarted, cloud.RequireDriverOwnedVehicle)
	if _, e := store2.Save(ctx, owner2, earning(t, car2)); e != nil {
		t.Fatal(e)
	}
	req2 := req
	req2.VehicleID = car2
	reports2 := &app.ReportService{Store: store2, Snapshots: NewReportStore(restarted), Paid: func(context.Context, string) error { return nil }}
	if _, e := reports2.Build(ctx, owner2, req2); e != nil {
		t.Fatal(e)
	}
	if _, e := NewReportStore(restarted).DeleteExpiredReportSnapshots(ctx, time.Now().Add(time.Hour), 100); e != nil {
		t.Fatal(e)
	}
	if _, e := reports2.Get(ctx, owner2, req2); !errors.Is(e, app.ErrNotFound) {
		t.Fatal("expired snapshot readable", e)
	}
}
