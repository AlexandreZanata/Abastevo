//go:build integration

package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	migrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	cloud "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/cloudsync/adapters"
	driverhttp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/driver/adapters/http"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/driver/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/driver/domain"
	"github.com/go-chi/chi/v5"
)

func TestDriverHTTPCommandChainAndErasure(t *testing.T) {
	ctx := context.Background()
	p, _ := freshDriverDB(t, migrations.Files)
	owner := seedDriverOwner(t, p)
	car := seedDriverVehicle(t, p, owner)
	foreignOwner := seedDriverOwner(t, p)
	store := New(p, cloud.RequireDriverOwnedVehicle)
	reports := &app.ReportService{Store: store, Snapshots: NewReportStore(p), Now: func() time.Time { return time.Date(2026, 10, 26, 18, 0, 0, 0, time.UTC) }, Paid: func(context.Context, string) error { return nil }}
	h := driverhttp.Handler{
		Earnings: store, Commands: store, Reports: reports,
		Sessions: func(_ context.Context, family, _ string) (string, error) {
			if family == "44444444-4444-4444-8444-444444444444" {
				return owner, nil
			}
			return foreignOwner, nil
		},
		SignedOwner: func(r *http.Request, _ string) (string, string, error) {
			if r.Header.Get("X-Signed") != "yes" {
				return "", "", errors.New("unsigned")
			}
			return "66666666-6666-4666-8666-666666666666", "77777777-7777-4777-8777-777777777777", nil
		},
		CursorSecret: []byte(strings.Repeat("s", 32)), Origin: "one.example.test",
	}
	router := chi.NewRouter()
	h.RegisterRoutes(router)
	call := func(family, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "/v1/driver/commands", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-Signed", "yes")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		return w
	}
	payload := func(kind string, v any) string {
		raw, _ := json.Marshal(v)
		body, _ := json.Marshal(map[string]any{"family_id": "44444444-4444-4444-8444-444444444444", "access_token": "x", "kind": kind, "payload": json.RawMessage(raw)})
		return string(body)
	}
	in := earning(t, car)
	w := call("44444444-4444-4444-8444-444444444444", payload("EARNING_SAVE", in))
	if w.Code != 200 {
		t.Fatalf("earning save %d %s", w.Code, w.Body.String())
	}
	w = call("44444444-4444-4444-8444-444444444444", payload("EARNING_SAVE", in))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"replayed":true`) {
		t.Fatalf("earning replay %d %s", w.Code, w.Body.String())
	}
	if driverCount(t, p, "driver_earnings") != 1 {
		t.Fatal("duplicate earning")
	}
	erase := map[string]any{"id": in.Record.ID, "revision": 1, "epoch": 0}
	w = call("44444444-4444-4444-8444-444444444444", payload("EARNING_ERASE", erase))
	if w.Code != 200 {
		t.Fatalf("earning erase %d %s", w.Code, w.Body.String())
	}
	if _, e := store.Get(ctx, owner, in.Record.ID); !errors.Is(e, app.ErrNotFound) {
		t.Fatal("erased earning readable", e)
	}
	shiftID, opID := newDriverID(t), newDriverID(t)
	shift := app.ApplyShift{ShiftID: shiftID, Create: &app.ShiftStart{ID: shiftID, Origin: "android", Vehicle: car, Timezone: "America/Cuiaba"}, Command: domain.ShiftCommand{ID: opID, Action: domain.ActionStart, ExpectedVersion: 0, Clock: domain.Clock{BootID: "boot-a", ElapsedMillis: 1000, UTCMillis: 1_000_000}}, Policy: domain.ClockPolicy{MaximumCheckpointGapMillis: 60_000, WallToleranceMillis: 1000}}
	w = call("44444444-4444-4444-8444-444444444444", payload("SHIFT_APPLY", shift))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"version":1`) {
		t.Fatalf("shift %d %s", w.Code, w.Body.String())
	}
	modeled := int64(10_000)
	profile := domain.VehicleCostProfile{Vehicle: car, Ownership: "OWNED", Timezone: "America/Cuiaba", From: "2026-10-26", Until: "2026-10-27", Version: 1, Costs: []domain.ProfileCost{{ID: "reserve-1", Kind: "RESERVE", Provenance: "MODELED", From: "2026-10-26", Until: "2026-10-27", Amount: &modeled}}}
	cost := app.SaveCostProfile{OperationID: newDriverID(t), ExpectedVersion: 0, ExpectedEpoch: 0, Profile: profile}
	w = call("44444444-4444-4444-8444-444444444444", payload("COST_SAVE", cost))
	if w.Code != 200 {
		t.Fatalf("cost %d %s", w.Code, w.Body.String())
	}
	if _, e := store.GetCostProfile(ctx, owner, car, 1); e != nil {
		t.Fatal("cost read", e)
	}
	second := earning(t, car)
	w = call("44444444-4444-4444-8444-444444444444", payload("EARNING_SAVE", second))
	if w.Code != 200 {
		t.Fatalf("second earning %d %s", w.Code, w.Body.String())
	}
	reportBody := `{"family_id":"44444444-4444-4444-8444-444444444444","access_token":"x","vehicle_id":"` + car + `","timezone":"UTC","period":"DAY","anchor":"2026-10-26","basis":"ODOMETER"}`
	reportCall := func(path, family, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-Signed", "yes")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		return w
	}
	w = reportCall("/v1/driver/reports/build", "44444444-4444-4444-8444-444444444444", reportBody)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"earned_centavos":45000`) || !strings.Contains(w.Body.String(), `"modeled_result_centavos":35000`) {
		t.Fatalf("report build %d %s", w.Code, w.Body.String())
	}
	if w = reportCall("/v1/driver/reports/get", "44444444-4444-4444-8444-444444444444", reportBody); w.Code != 200 {
		t.Fatalf("report get %d %s", w.Code, w.Body.String())
	}
	corrected := second
	corrected.OperationID = newDriverID(t)
	corrected.ExpectedRevision = 1
	corrected.Record.Money.Net = new(int64)
	*corrected.Record.Money.Net = 46000
	if w = call("44444444-4444-4444-8444-444444444444", payload("EARNING_SAVE", corrected)); w.Code != 200 {
		t.Fatalf("correction %d %s", w.Code, w.Body.String())
	}
	if w = reportCall("/v1/driver/reports/get", "44444444-4444-4444-8444-444444444444", reportBody); w.Code != 409 {
		t.Fatalf("stale report get %d %s", w.Code, w.Body.String())
	}

	foreignFamily := "55555555-5555-4555-8555-555555555555"
	body, _ := json.Marshal(map[string]any{"family_id": foreignFamily, "access_token": "x", "kind": "COST_SAVE", "payload": json.RawMessage(mustJSON(cost))})
	r := httptest.NewRequest(http.MethodPost, "/v1/driver/commands", strings.NewReader(string(body)))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-Signed", "yes")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatalf("foreign owner %d %s", w.Code, w.Body.String())
	}
}

func mustJSON(v any) []byte { raw, _ := json.Marshal(v); return raw }
