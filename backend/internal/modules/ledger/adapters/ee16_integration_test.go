//go:build integration

package adapters

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/ledger/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/metrics"
)

func ee16Consent(t *testing.T, store *Store, accountID string) {
	t.Helper()
	if _, err := store.SetConsent(context.Background(), accountID, []string{"refuel", "expense", "charge"}, true); err != nil {
		t.Fatal(err)
	}
}

func ee16Charge(t *testing.T, id, vehicle, occurred string, energy, total, fees *int64, socStart, socEnd *int, odo *int64) application.Command {
	t.Helper()
	payload := map[string]any{"id": id, "vehicle_id": vehicle, "occurred_at": occurred, "timezone": "America/Sao_Paulo"}
	if energy != nil {
		payload["energy_wh"] = *energy
	}
	if total != nil {
		payload["total_centavos"] = *total
	}
	if fees != nil {
		payload["fees_centavos"] = *fees
	}
	if socStart != nil {
		payload["soc_start_percent"] = *socStart
	}
	if socEnd != nil {
		payload["soc_end_percent"] = *socEnd
	}
	if odo != nil {
		payload["odometer_metres"] = *odo
	}
	raw, _ := json.Marshal(payload)
	return application.Command{OperationID: newid(t), Dataset: "charge", ID: id, Payload: raw}
}

func int64ptr(v int64) *int64 { return &v }

func intptr(v int) *int { return &v }

func TestEE16ChargeTotalsGoldenParity(t *testing.T) {
	pool := freshDB(t)
	owner := newAccount(t, pool)
	vehicle := newVehicle(t, pool, owner)
	store := NewStore(pool)
	ee16Consent(t, store, owner)
	ctx := context.Background()
	a, b := int64(100000), int64(250000)
	if _, err := store.Apply(ctx, owner, application.Batch{Commands: []application.Command{
		ee16Charge(t, newid(t), vehicle, "2026-03-05T10:00:00-03:00", int64ptr(10000), int64ptr(1000), nil, intptr(20), intptr(60), &a),
		ee16Charge(t, newid(t), vehicle, "2026-03-20T10:00:00-03:00", int64ptr(30000), int64ptr(3000), int64ptr(150), intptr(60), intptr(90), &b),
		reportExpense(t, newid(t), vehicle, "2026-03-10T10:00:00-03:00", "tolls"),
	}}); err != nil {
		t.Fatal(err)
	}
	got, err := store.BuildReport(ctx, owner, 2026, 3, "America/Sao_Paulo")
	if err != nil {
		t.Fatal(err)
	}
	// Hand-derived golden values: unequal sessions summed first.
	if got.Totals.ChargeEnergyWh != 40000 || got.Totals.ChargeCostCentavos != 4000 || got.Totals.ChargeFeesCentavos != 150 {
		t.Fatalf("charge totals %+v", got.Totals)
	}
	if got.Totals.ExpenseCentavos != 1000 || got.Totals.ByCategory["tolls"] != 1000 {
		t.Fatalf("expense coexistence %+v", got.Totals)
	}
	if got.Totals.ByVehicle[vehicle] != 5000 {
		t.Fatalf("vehicle money double-counted or lost: %+v", got.Totals.ByVehicle)
	}
	if got.Totals.PurchaseCentavos != 0 {
		t.Fatalf("fuel purchase invented %+v", got.Totals)
	}
	pure := metrics.EnergyInterval{PurchasedWh: 40000, CostCentavos: 4000, DistanceMetres: 150000, HasEnergy: true, Complete: true}.Compute()
	if pure.Unknown != "" {
		t.Fatal(pure.Unknown)
	}
	if got.Totals.EnergyCoverage[vehicle] != pure.WhPerKm || pure.WhPerKm != "800/3" {
		t.Fatalf("energy parity: report %q pure %q", got.Totals.EnergyCoverage[vehicle], pure.WhPerKm)
	}
	if got.Totals.CombinedCostPerKm[vehicle] != pure.CentavosPerKm || pure.CentavosPerKm != "80/3" {
		t.Fatalf("combined parity: report %q pure %q", got.Totals.CombinedCostPerKm[vehicle], pure.CentavosPerKm)
	}
	if _, ok := got.Totals.Coverage[vehicle]; ok {
		t.Fatalf("fuel coverage invented for charge-only vehicle: %+v", got.Totals.Coverage)
	}
}

func TestEE16CrossMonthTimezone(t *testing.T) {
	pool := freshDB(t)
	owner := newAccount(t, pool)
	vehicle := newVehicle(t, pool, owner)
	store := NewStore(pool)
	ee16Consent(t, store, owner)
	ctx := context.Background()
	// 2026-03-01T02:00:00Z is still February in Sao Paulo.
	if _, err := store.Apply(ctx, owner, application.Batch{Commands: []application.Command{
		ee16Charge(t, newid(t), vehicle, "2026-03-01T02:00:00Z", int64ptr(12000), int64ptr(900), nil, nil, nil, nil),
	}}); err != nil {
		t.Fatal(err)
	}
	feb, err := store.BuildReport(ctx, owner, 2026, 2, "America/Sao_Paulo")
	if err != nil || feb.Totals.ChargeCostCentavos != 900 || feb.Totals.ChargeEnergyWh != 12000 {
		t.Fatalf("feb placement %+v %v", feb, err)
	}
	marUTC, err := store.BuildReport(ctx, owner, 2026, 3, "UTC")
	if err != nil || marUTC.Totals.ChargeCostCentavos != 900 {
		t.Fatalf("utc placement %+v %v", marUTC, err)
	}
	marSP, err := store.BuildReport(ctx, owner, 2026, 3, "America/Sao_Paulo")
	if err != nil || marSP.Totals.ChargeCostCentavos != 0 || marSP.Totals.RecordCount != 0 {
		t.Fatalf("leak across months %+v %v", marSP, err)
	}
}

func TestEE16LateCorrectionInvalidates(t *testing.T) {
	pool := freshDB(t)
	owner := newAccount(t, pool)
	vehicle := newVehicle(t, pool, owner)
	store := NewStore(pool)
	ee16Consent(t, store, owner)
	ctx := context.Background()
	id := newid(t)
	if _, err := store.Apply(ctx, owner, application.Batch{Commands: []application.Command{
		ee16Charge(t, id, vehicle, "2026-03-05T10:00:00-03:00", int64ptr(15000), int64ptr(1275), nil, nil, nil, nil),
	}}); err != nil {
		t.Fatal(err)
	}
	first, err := store.BuildReport(ctx, owner, 2026, 3, "UTC")
	if err != nil || first.Totals.ChargeCostCentavos != 1275 {
		t.Fatalf("first %+v %v", first, err)
	}
	corrected := ee16Charge(t, id, vehicle, "2026-03-05T10:00:00-03:00", int64ptr(15000), int64ptr(2000), nil, nil, nil, nil)
	corrected.OperationID = newid(t)
	corrected.ExpectedRevision = 1
	if _, err = store.Apply(ctx, owner, application.Batch{Commands: []application.Command{corrected}}); err != nil {
		t.Fatal(err)
	}
	if _, err = store.GetReport(ctx, owner, 2026, 3, "UTC"); err == nil {
		t.Fatal("stale charge projection readable after correction")
	}
	second, err := store.BuildReport(ctx, owner, 2026, 3, "UTC")
	if err != nil || second.Totals.ChargeCostCentavos != 2000 || second.Revision == first.Revision {
		t.Fatalf("correction not converged %+v %v", second, err)
	}
}

func TestEE16PHEVSharedDistance(t *testing.T) {
	pool := freshDB(t)
	owner := newAccount(t, pool)
	vehicle := newVehicle(t, pool, owner)
	store := NewStore(pool)
	ee16Consent(t, store, owner)
	ctx := context.Background()
	a, b := int64(100000), int64(200000)
	first := mexFill(t, newid(t), vehicle, "2026-03-01T10:00:00Z", &a)
	last := mexFill(t, newid(t), vehicle, "2026-03-10T10:00:00Z", &b)
	charge := ee16Charge(t, newid(t), vehicle, "2026-03-05T10:00:00-03:00", int64ptr(15000), int64ptr(1275), nil, intptr(20), intptr(80), nil)
	charge.Payload = mustReodo(t, charge.Payload, int64(150000))
	if _, err := store.Apply(ctx, owner, application.Batch{Commands: []application.Command{first, charge, last}}); err != nil {
		t.Fatal(err)
	}
	got, err := store.BuildReport(ctx, owner, 2026, 3, "UTC")
	if err != nil {
		t.Fatal(err)
	}
	// Money exact and counted once: 12400+12400 fuel plus 1275 charge.
	if got.Totals.PurchaseCentavos != 24800 || got.Totals.ChargeEnergyWh != 15000 || got.Totals.ChargeCostCentavos != 1275 {
		t.Fatalf("money %+v", got.Totals)
	}
	if got.Totals.ByVehicle[vehicle] != 24800+1275 {
		t.Fatalf("vehicle money double-counted: %+v", got.Totals.ByVehicle)
	}
	// Shared distance (100000m) attributed to neither carrier alone.
	if got.Totals.Coverage[vehicle] != "phev-shared-distance" {
		t.Fatalf("fuel ratio double-attributes: %+v", got.Totals.Coverage)
	}
	if got.Totals.EnergyCoverage[vehicle] != "phev-shared-distance" {
		t.Fatalf("energy ratio double-attributes: %+v", got.Totals.EnergyCoverage)
	}
	pure := metrics.CombinedCostPerKm(24800, 1275, 100000)
	if got.Totals.CombinedCostPerKm[vehicle] != pure.CentavosPerKm || pure.CentavosPerKm != "1043/4" {
		t.Fatalf("combined %+v pure %+v", got.Totals.CombinedCostPerKm, pure)
	}
}

func mustReodo(t *testing.T, raw json.RawMessage, odo int64) json.RawMessage {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	payload["odometer_metres"] = odo
	out, _ := json.Marshal(payload)
	return out
}

func TestEE16SocGapSegmentMissing(t *testing.T) {
	for _, kind := range []string{"soc", "gap", "segment", "missing"} {
		t.Run(kind, func(t *testing.T) {
			pool := freshDB(t)
			owner := newAccount(t, pool)
			vehicle := newVehicle(t, pool, owner)
			s := NewStore(pool)
			ee16Consent(t, s, owner)
			ctx := context.Background()
			a, b := int64(100000), int64(200000)
			first := ee16Charge(t, newid(t), vehicle, "2026-03-01T10:00:00-03:00", int64ptr(10000), int64ptr(1000), nil, intptr(80), intptr(20), &a)
			last := ee16Charge(t, newid(t), vehicle, "2026-03-02T10:00:00-03:00", int64ptr(10000), int64ptr(1000), nil, intptr(20), intptr(80), &b)
			if kind == "missing" {
				last = ee16Charge(t, last.ID, vehicle, "2026-03-02T10:00:00-03:00", int64ptr(10000), int64ptr(1000), nil, intptr(20), intptr(80), nil)
			}
			if _, err := s.Apply(ctx, owner, application.Batch{Commands: []application.Command{first}}); err != nil {
				t.Fatal(err)
			}
			if kind == "segment" {
				if _, err := s.OpenSegment(ctx, owner, vehicle, "reset"); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := s.Apply(ctx, owner, application.Batch{Commands: []application.Command{last}}); err != nil {
				t.Fatal(err)
			}
			if kind == "gap" {
				if _, err := s.DeclareGap(ctx, owner, vehicle, newid(t), first.ID, last.ID, ""); err != nil {
					t.Fatal(err)
				}
			}
			got, err := s.BuildReport(ctx, owner, 2026, 3, "America/Sao_Paulo")
			if err != nil {
				t.Fatal(err)
			}
			want := map[string]string{"soc": "soc-inconsistent", "gap": "declared-fill-gap", "segment": "odometer-segment-boundary", "missing": "odometer-missing"}[kind]
			if got.Totals.EnergyCoverage[vehicle] != want {
				t.Fatalf("coverage=%q want %q", got.Totals.EnergyCoverage[vehicle], want)
			}
			if got.Totals.ChargeCostCentavos != 2000 || got.Totals.ChargeEnergyWh != 20000 {
				t.Fatalf("money hidden by evidence flag: %+v", got.Totals)
			}
			if v := got.Totals.CombinedCostPerKm[vehicle]; v == "" {
				t.Fatal("combined cost suppressed")
			} else if kind == "soc" && v != metrics.CombinedCostPerKm(0, 2000, 100000).CentavosPerKm {
				t.Fatalf("combined mismatch %q", v)
			}
		})
	}
}

func TestEE16ErasureCoversCharge(t *testing.T) {
	pool := freshDB(t)
	owner := newAccount(t, pool)
	vehicle := newVehicle(t, pool, owner)
	s := NewStore(pool)
	ee16Consent(t, s, owner)
	ctx := context.Background()
	if _, err := s.Apply(ctx, owner, application.Batch{Commands: []application.Command{
		ee16Charge(t, newid(t), vehicle, "2026-03-05T10:00:00-03:00", int64ptr(15000), int64ptr(1275), nil, nil, nil, nil),
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.BuildReport(ctx, owner, 2026, 3, "UTC"); err != nil {
		t.Fatal(err)
	}
	if err := s.EraseAll(ctx, owner); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetReport(ctx, owner, 2026, 3, "UTC"); err == nil {
		t.Fatal("erased charge projection readable")
	}
	if _, err := s.Apply(ctx, owner, application.Batch{Commands: []application.Command{
		ee16Charge(t, newid(t), vehicle, "2026-03-06T10:00:00-03:00", int64ptr(15000), int64ptr(1275), nil, nil, nil, nil),
	}}); err == nil {
		t.Fatal("charge admitted after erasure revoked consent")
	}
	empty, err := s.BuildReport(ctx, owner, 2026, 3, "UTC")
	if err != nil || empty.Totals.ChargeEnergyWh != 0 || empty.Totals.RecordCount != 0 {
		t.Fatalf("post-erasure rebuild %+v %v", empty, err)
	}
}
