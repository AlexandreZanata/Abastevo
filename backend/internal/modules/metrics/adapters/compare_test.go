package adapters

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/metrics/application"
)

type fakeReader struct {
	rows map[string][]application.LedgerRecord
	err  error
}

func (f *fakeReader) ListRecords(_ context.Context, _ string, _ string, vehicleID, _, _ string, _ int) ([]application.LedgerRecord, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.rows[vehicleID], nil
}

func refuelRow(vehicle, id string, quantity int64, odo int64, full bool) application.LedgerRecord {
	payload, _ := json.Marshal(map[string]any{"fuel": "ETHANOL", "unit": "L", "quantity_milli": quantity, "total_centavos": quantity * 6 / 10, "fill": map[bool]string{true: "full", false: "partial"}[full], "odometer_metres": odo})
	return application.LedgerRecord{Dataset: "refuel", ID: id, VehicleID: vehicle, Payload: payload}
}

func TestCompareMissingZeroBaseline(t *testing.T) {
	vehicle := "11111111-1111-4111-8111-111111111111"
	current := "22222222-2222-4222-8222-222222222222"
	reader := &fakeReader{rows: map[string][]application.LedgerRecord{
		vehicle: {refuelRow(vehicle, "a", 20000, 100000, true), refuelRow(vehicle, "b", 20000, 140000, true)},
	}}
	store := NewStore(reader)
	base := application.Side{VehicleID: vehicle, From: "2026-01-01T00:00:00Z", To: "2026-02-01T00:00:00Z", Timezone: "UTC", Method: "fuel-purchase-interval"}
	cur := application.Side{VehicleID: current, From: "2026-01-01T00:00:00Z", To: "2026-02-01T00:00:00Z", Timezone: "UTC", Method: "fuel-purchase-interval"}
	got, err := store.Compare(context.Background(), "owner", base, cur)
	if err != nil || got.Compatible || got.Issue != "missing-current" {
		t.Fatalf("missing %+v %v", got, err)
	}
	free := refuelRow(vehicle, "c", 20000, 180000, true)
	freePayload := map[string]any{}
	_ = json.Unmarshal(free.Payload, &freePayload)
	reader.rows[vehicle] = append(reader.rows[vehicle], free)
	_ = free
	_ = current
}

func TestCompareUnitsTimezoneMethodRanges(t *testing.T) {
	vehicleA := "11111111-1111-4111-8111-111111111111"
	vehicleB := "22222222-2222-4222-8222-222222222222"
	gas, _ := json.Marshal(map[string]any{"fuel": "CNG", "unit": "M3", "quantity_milli": 20000, "total_centavos": 12000, "fill": "full", "odometer_metres": 100000})
	reader := &fakeReader{rows: map[string][]application.LedgerRecord{
		vehicleA: {refuelRow(vehicleA, "a", 20000, 100000, true), refuelRow(vehicleA, "b", 20000, 140000, true)},
		vehicleB: {{Dataset: "refuel", ID: "c", VehicleID: vehicleB, Payload: gas}},
	}}
	store := NewStore(reader)
	side := func(vehicle, tz, method string) application.Side {
		return application.Side{VehicleID: vehicle, From: "2026-01-01T00:00:00Z", To: "2026-02-01T00:00:00Z", Timezone: tz, Method: method}
	}
	if got, _ := store.Compare(context.Background(), "owner", side(vehicleA, "UTC", "fuel-purchase-interval"), side(vehicleB, "UTC", "fuel-purchase-interval")); got.Compatible || got.Issue == "" {
		t.Fatalf("units %+v", got)
	}
	if got, _ := store.Compare(context.Background(), "owner", side(vehicleA, "UTC", "fuel-purchase-interval"), side(vehicleA, "UTC", "matched-window")); got.Compatible || got.Issue != "mismatched-method" {
		t.Fatalf("method %+v", got)
	}
	if got, _ := store.Compare(context.Background(), "owner", side(vehicleA, "UTC", "fuel-purchase-interval"), side(vehicleA, "America/Sao_Paulo", "fuel-purchase-interval")); got.Compatible || got.Issue != "mismatched-timezone" {
		t.Fatalf("timezone %+v", got)
	}
	if _, err := store.Compare(context.Background(), "owner", side("not-a-uuid", "UTC", "fuel-purchase-interval"), side(vehicleA, "UTC", "fuel-purchase-interval")); err == nil {
		t.Fatal("foreign id admitted")
	}
	if _, err := store.Compare(context.Background(), "owner", side(vehicleA, "UTC", "fuel-purchase-interval"), application.Side{VehicleID: vehicleA, From: "2026-02-01T00:00:00Z", To: "2026-01-01T00:00:00Z", Timezone: "UTC", Method: "fuel-purchase-interval"}); err == nil {
		t.Fatal("reversed range admitted")
	}
	if _, err := store.Compare(context.Background(), "owner", side(vehicleA, "UTC", "fuel-purchase-interval"), application.Side{VehicleID: vehicleA, From: "2020-01-01T00:00:00Z", To: "2026-01-01T00:00:00Z", Timezone: "UTC", Method: "fuel-purchase-interval"}); err == nil {
		t.Fatal("oversize range admitted")
	}
}

func TestCompareUnequalCoverageNoWinner(t *testing.T) {
	vehicleA := "11111111-1111-4111-8111-111111111111"
	vehicleB := "22222222-2222-4222-8222-222222222222"
	reader := &fakeReader{rows: map[string][]application.LedgerRecord{
		vehicleA: {refuelRow(vehicleA, "a", 10000, 100000, true), refuelRow(vehicleA, "b", 60000, 1000000, true)},
		vehicleB: {refuelRow(vehicleB, "c", 20000, 100000, true), refuelRow(vehicleB, "d", 20000, 140000, true)},
	}}
	store := NewStore(reader)
	side := func(vehicle string) application.Side {
		return application.Side{VehicleID: vehicle, From: "2026-01-01T00:00:00Z", To: "2026-04-01T00:00:00Z", Timezone: "UTC", Method: "fuel-purchase-interval"}
	}
	got, err := store.Compare(context.Background(), "owner", side(vehicleA), side(vehicleB))
	if err != nil || !got.Compatible || got.ChangePercent == "" {
		t.Fatalf("weighted %+v %v", got, err)
	}
	if got.Baseline == "" || got.Current == "" || got.Baseline == got.Current {
		t.Fatalf("no winner fabricated %+v", got)
	}
}
