//go:build integration

package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	alertsadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/alerts/adapters"
	alertshttp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/alerts/adapters/http"
	alertsapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/alerts/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/cloudsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/cloudsync/domain"
	ledgeradapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/ledger/adapters"
	ledgerhttp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/ledger/adapters/http"
	ledgerapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/ledger/application"
	metricsadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/metrics/adapters"
	metricshttp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/metrics/adapters/http"
	metricsapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/metrics/application"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

type journeyRouter interface{ RegisterRoutes(chi.Router) }

func journeyCall(t *testing.T, h journeyRouter, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	router := chi.NewRouter()
	h.RegisterRoutes(router)
	r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	return w
}

func journeySessions(owners map[string]string) func(context.Context, string, string) (string, error) {
	return func(_ context.Context, family, token string) (string, error) {
		owner, ok := owners[family]
		if !ok || token != "synthetic" {
			return "", errUnauthorized()
		}
		return owner, nil
	}
}

func journeySigned(_ *http.Request, _ string) (string, string, error) {
	return "33333333-3333-4333-8333-333333333333", "44444444-4444-4444-8444-444444444444", nil
}

func journeyBatchJSON(family, device string, epoch, consent int64, cmds ...application.Command) string {
	rawCmds, _ := json.Marshal(cmds)
	return `{"family_id":"` + family + `","access_token":"synthetic","device_id":"` + device + `","epoch":` + itoa(epoch) + `,"consent_revision":` + itoa(consent) + `,"commands":` + string(rawCmds) + `}`
}

func journeyRevision(t *testing.T, body string) int64 {
	t.Helper()
	var decoded map[string]any
	if err := json.Unmarshal([]byte(body), &decoded); err != nil {
		t.Fatalf("batch decode %v", err)
	}
	results, _ := decoded["results"].([]any)
	if len(results) == 0 {
		t.Fatal("empty results")
	}
	first, _ := results[0].(map[string]any)
	revision, _ := first["revision"].(float64)
	return int64(revision)
}

type journeyLedgerReader struct{ store *ledgeradapters.Store }

func (r journeyLedgerReader) ListRecords(ctx context.Context, accountID, dataset, vehicleID, from, to string, limit int) ([]metricsapp.LedgerRecord, error) {
	rows, err := r.store.ConsumptionRows(ctx, accountID, vehicleID, from, to, limit)
	if err != nil {
		return nil, err
	}
	out := make([]metricsapp.LedgerRecord, 0, len(rows))
	for _, row := range rows {
		out = append(out, metricsapp.LedgerRecord{Dataset: dataset, ID: row.ID, VehicleID: row.VehicleID, Payload: row.Payload, SegmentNo: row.SegmentNo, HasGap: row.HasGap})
	}
	return out, nil
}

type journeySender struct {
	mu    sync.Mutex
	calls int
}

func (f *journeySender) Send(_ context.Context, _ string, _ alertsapp.PushPayload) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	return "provider-msg-1", nil
}

func journeyRefuel(t *testing.T, id, vehicle string, odo int64) ledgerapp.Command {
	t.Helper()
	return journeyRefuelAt(t, id, vehicle, odo, "2026-03-15T10:30:00-03:00")
}

func journeyRefuelAt(t *testing.T, id, vehicle string, odo int64, occurred string) ledgerapp.Command {
	t.Helper()
	raw, _ := json.Marshal(map[string]any{"id": id, "vehicle_id": vehicle, "occurred_at": occurred, "timezone": "America/Sao_Paulo", "fuel": "ETHANOL", "unit": "L", "quantity_milli": 20000, "total_centavos": 12400, "odometer_metres": odo, "fill": "full"})
	return ledgerapp.Command{OperationID: newid(t), Dataset: "refuel", ID: id, Payload: raw}
}

func TestJourneyFullBackend(t *testing.T) {
	pool := freshDB(t)
	owner := seedOwner(t, pool)
	cloudStore := paidStore(t, pool, owner)
	deviceA := readyDevice(t, cloudStore, owner)
	deviceB := secondDevice(t, pool, cloudStore, owner)
	cloudStore.Objects = newCampaignObjects()
	owners := map[string]string{deviceA.FamilyID: owner.AccountID, deviceB.FamilyID: owner.AccountID}
	keys := map[string][2]string{owner.AccountID: {owner.ContributorID, owner.KeyID}}
	cloudHandler := protocolHandler(t, cloudStore, owners, keys)
	ledgerStore := ledgeradapters.NewStore(pool)
	alertsStore := alertsadapters.NewStore(pool)
	sender := &journeySender{}
	alertsStore.Sender = sender
	ledgerHandler := ledgerhttp.Handler{Service: ledgerStore, CursorSecret: []byte(strings.Repeat("s", 32)), Origin: "one.example.test", Sessions: journeySessions(owners), SignedOwner: journeySigned}
	alertsHandler := alertshttp.Handler{Service: alertsStore, Sessions: journeySessions(owners), SignedOwner: journeySigned}
	metricsHandler := metricshttp.Handler{Service: metricsadapters.NewStore(journeyLedgerReader{store: ledgerStore}), Sessions: journeySessions(owners), SignedOwner: journeySigned}
	allowCloud := cloudStore.Paid
	cloudStore.Paid = func(context.Context, pgx.Tx, string) error { return domain.ErrDenied }
	deniedBody := journeyBatchJSON(deviceA.FamilyID, deviceA.DeviceID, 1, 1, vehicleCommand(t, vehicle(t), 0))
	if w := journeyCall(t, cloudHandler, "/v1/cloud/sync/push", deniedBody); w.Code != 403 {
		t.Fatalf("capability gate %d", w.Code)
	}
	cloudStore.Paid = allowCloud
	v := vehicle(t)
	w := journeyCall(t, cloudHandler, "/v1/cloud/sync/push", journeyBatchJSON(deviceA.FamilyID, deviceA.DeviceID, 1, 1, vehicleCommand(t, v, 0)))
	if w.Code != 200 || journeyRevision(t, w.Body.String()) != 1 {
		t.Fatalf("device A create %d %s", w.Code, w.Body.String())
	}
	updated := v
	updated.Name = "Device B edit"
	w = journeyCall(t, cloudHandler, "/v1/cloud/sync/push", journeyBatchJSON(deviceB.FamilyID, deviceB.DeviceID, 1, 1, vehicleCommand(t, updated, 1)))
	if w.Code != 200 || journeyRevision(t, w.Body.String()) != 2 {
		t.Fatalf("device B edit %d %s", w.Code, w.Body.String())
	}
	manifestBody := `{"family_id":"` + deviceA.FamilyID + `","access_token":"synthetic","device_id":"` + deviceA.DeviceID + `"}`
	w = journeyCall(t, cloudHandler, "/v1/cloud/snapshots/manifest", manifestBody)
	if w.Code != 200 {
		t.Fatalf("manifest %d", w.Code)
	}
	var manifest map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &manifest); err != nil || int64(manifest["revision"].(float64)) != 2 {
		t.Fatalf("manifest revision %+v %v", manifest, err)
	}
	ctx := context.Background()
	snapshotID := newid(t)
	snapshotData := []byte(`{"version":1,"revision":2,"epoch":1,"consent_revision":1,"records":[]}`)
	if _, err := cloudStore.StoreSnapshot(ctx, deviceA, snapshotID, 2, 1, 1, snapshotData); err != nil {
		t.Fatal(err)
	}
	previewBody := `{"family_id":"` + deviceA.FamilyID + `","access_token":"synthetic","device_id":"` + deviceA.DeviceID + `","snapshot_id":"` + snapshotID + `"}`
	w = journeyCall(t, cloudHandler, "/v1/cloud/restore/preview", previewBody)
	if w.Code != 200 {
		t.Fatalf("preview %d", w.Code)
	}
	var preview map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	commitBody := `{"family_id":"` + deviceA.FamilyID + `","access_token":"synthetic","device_id":"` + deviceA.DeviceID + `","snapshot_id":"` + snapshotID + `","restore_id":"` + newid(t) + `","preview_revision":` + itoa(int64(preview["current_revision"].(float64))) + `,"preview_expires_at":` + itoa(int64(preview["expires_at"].(float64))) + `}`
	w = journeyCall(t, cloudHandler, "/v1/cloud/restore/commit", commitBody)
	if w.Code != 200 {
		t.Fatalf("commit %d %s", w.Code, w.Body.String())
	}
	consentBody := `{"family_id":"` + deviceA.FamilyID + `","access_token":"synthetic","datasets":["refuel","expense"],"enabled":true}`
	if w := journeyCall(t, ledgerHandler, "/v1/ledger/consent", consentBody); w.Code != 200 {
		t.Fatalf("consent %d", w.Code)
	}
	vehicleB := vehicle(t)
	vehicleB.ID = newid(t)
	if _, err := cloudStore.PutVehicle(ctx, deviceA, vehicleB, 0); err != nil {
		t.Fatal(err)
	}
	refuelA1, refuelA2 := newid(t), newid(t)
	ledgerBody := func(cmds ...ledgerapp.Command) string {
		rawCmds, _ := json.Marshal(cmds)
		return `{"family_id":"` + deviceA.FamilyID + `","access_token":"synthetic","commands":` + string(rawCmds) + `}`
	}
	w = journeyCall(t, ledgerHandler, "/v1/ledger/commands", ledgerBody(journeyRefuelAt(t, refuelA1, v.ID, 100000, "2026-03-15T10:30:00-03:00"), journeyRefuelAt(t, refuelA2, v.ID, 140000, "2026-03-15T11:30:00-03:00")))
	if w.Code != 200 {
		t.Fatalf("ledger refuels %d %s", w.Code, w.Body.String())
	}
	refuelB1, refuelB2 := newid(t), newid(t)
	w = journeyCall(t, ledgerHandler, "/v1/ledger/commands", ledgerBody(journeyRefuelAt(t, refuelB1, vehicleB.ID, 100000, "2026-03-15T10:30:00-03:00"), journeyRefuelAt(t, refuelB2, vehicleB.ID, 140000, "2026-03-15T11:30:00-03:00")))
	if w.Code != 200 {
		t.Fatalf("ledger refuels B %d", w.Code)
	}
	reportBody := `{"family_id":"` + deviceA.FamilyID + `","access_token":"synthetic","year":2026,"month":3,"timezone":"America/Sao_Paulo"}`
	w = journeyCall(t, ledgerHandler, "/v1/ledger/reports/build", reportBody)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "49600") {
		t.Fatalf("report %d %s", w.Code, w.Body.String())
	}
	compareBody := `{"family_id":"` + deviceA.FamilyID + `","access_token":"synthetic","baseline":{"vehicle_id":"` + v.ID + `","from":"2026-03-01T00:00:00Z","to":"2026-04-01T00:00:00Z","timezone":"UTC","method":"fuel-purchase-interval"},"current":{"vehicle_id":"` + vehicleB.ID + `","from":"2026-03-01T00:00:00Z","to":"2026-04-01T00:00:00Z","timezone":"UTC","method":"fuel-purchase-interval"}}`
	w = journeyCall(t, metricsHandler, "/v1/metrics/compare", compareBody)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"compatible":true`) {
		t.Fatalf("compare %d %s", w.Code, w.Body.String())
	}
	station := newid(t)
	if _, err := pool.Exec(ctx, "INSERT INTO directory_stations(id,display_name) VALUES($1,'Synthetic station')", station); err != nil {
		t.Fatal(err)
	}
	ruleRaw, _ := json.Marshal(map[string]any{
		"predicates": []any{map[string]any{"kind": "threshold", "fuel": "ETHANOL", "unit": "L", "condition": "STANDARD", "source": "community", "threshold_milli": 6200}},
		"operator":   "AND", "target": map[string]any{"station_ids": []string{station}},
		"schedule": map[string]any{"days": []int{0, 1, 2, 3, 4, 5, 6}, "window_start": "00:00", "window_end": "00:00", "timezone": "UTC", "quiet_start": "00:00", "quiet_end": "00:00", "cooldown_hours": 0},
	})
	ruleID := newid(t)
	ruleCmd, _ := json.Marshal([]map[string]any{{"operation_id": newid(t), "rule_id": ruleID, "payload": json.RawMessage(ruleRaw)}})
	w = journeyCall(t, alertsHandler, "/v1/alerts/commands", `{"family_id":"`+deviceA.FamilyID+`","access_token":"synthetic","commands":`+string(ruleCmd)+`}`)
	if w.Code != 200 || journeyRevision(t, w.Body.String()) != 1 {
		t.Fatalf("rule create %d %s", w.Code, w.Body.String())
	}
	now := time.Now().UTC().Truncate(time.Second)
	if _, err := pool.Exec(ctx, "INSERT INTO community_observations(id,contributor_ref,client_submission_id,station_id,fuel_product,unit,amount_milli_brl,condition_kind,qualifier_key,received_at,policy_version) VALUES($1,'synthetic','synthetic-journey',$2,'ETHANOL','L',6000,'STANDARD','',$3,'v1')", newid(t), station, now.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO community_observation_decisions(id,observation_id,sequence,to_state,policy_version) SELECT $1,id,1,'VALIDATED','v1' FROM community_observations WHERE client_submission_id='synthetic-journey'", newid(t)); err != nil {
		t.Fatal(err)
	}
	evaluated, err := alertsStore.Evaluate(ctx, owner.AccountID, ruleID, "", 100, now)
	if err != nil || evaluated.Candidates != 1 {
		t.Fatalf("evaluate %+v %v", evaluated, err)
	}
	dispatched, err := alertsStore.Dispatch(ctx, now, 25)
	if err != nil || dispatched.Dispatched != 1 {
		t.Fatalf("dispatch %+v %v", dispatched, err)
	}
	tokenBody := `{"family_id":"` + deviceA.FamilyID + `","access_token":"synthetic","token":"journey-token-1"}`
	if w := journeyCall(t, alertsHandler, "/v1/alerts/push/enroll", tokenBody); w.Code != 200 {
		t.Fatalf("enroll %d", w.Code)
	}
	alertsStore.Sender = sender
	var journeyInbox string
	if err := pool.QueryRow(ctx, "SELECT inbox_id FROM alert_inbox WHERE account_id=$1 AND status='dispatched' LIMIT 1", owner.AccountID).Scan(&journeyInbox); err != nil {
		t.Fatal(err)
	}
	if _, err := alertsStore.SendInbox(ctx, owner.AccountID, journeyInbox, "journey-token-1"); err != nil {
		t.Fatalf("send %v", err)
	}
	w = journeyCall(t, alertsHandler, "/v1/alerts/inbox", `{"family_id":"`+deviceA.FamilyID+`","access_token":"synthetic","limit":20}`)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "sent") {
		t.Fatalf("inbox sent %d %s", w.Code, w.Body.String())
	}
	cloudStore.Paid = func(context.Context, pgx.Tx, string) error { return domain.ErrDenied }
	ledgerStore.Paid = func(context.Context, pgx.Tx, string) error { return errors.New("ledger: capability required") }
	alertsStore.Paid = func(context.Context, pgx.Tx, string) error { return errors.New("alerts: capability required") }
	if w := journeyCall(t, cloudHandler, "/v1/cloud/sync/push", journeyBatchJSON(deviceA.FamilyID, deviceA.DeviceID, 1, 1, vehicleCommand(t, vehicle(t), 0))); w.Code != 403 {
		t.Fatalf("lapsed cloud write %d", w.Code)
	}
	if w := journeyCall(t, ledgerHandler, "/v1/ledger/commands", ledgerBody(journeyRefuel(t, newid(t), v.ID, 150000))); w.Code != 403 {
		t.Fatalf("lapsed ledger write %d", w.Code)
	}
	if w := journeyCall(t, ledgerHandler, "/v1/ledger/reports/build", reportBody); w.Code != 403 {
		t.Fatalf("lapsed build %d", w.Code)
	}
	if w := journeyCall(t, ledgerHandler, "/v1/ledger/records", `{"family_id":"`+deviceA.FamilyID+`","access_token":"synthetic","dataset":"refuel","limit":20}`); w.Code != 200 {
		t.Fatalf("lapsed free read %d", w.Code)
	}
	if w := journeyCall(t, alertsHandler, "/v1/alerts/rules", `{"family_id":"`+deviceA.FamilyID+`","access_token":"synthetic"}`); w.Code != 200 {
		t.Fatalf("lapsed free rules %d", w.Code)
	}
	cloudStore.Paid = allowCloud
	ledgerStore.Paid = nil
	alertsStore.Paid = nil
	delBody := `{"family_id":"` + deviceA.FamilyID + `","access_token":"synthetic","dataset":"refuel","id":"` + refuelA1 + `"}`
	if w := journeyCall(t, ledgerHandler, "/v1/ledger/delete", delBody); w.Code != 200 {
		t.Fatalf("free delete %d", w.Code)
	}
	if w := journeyCall(t, alertsHandler, "/v1/alerts/erase", `{"family_id":"`+deviceA.FamilyID+`","access_token":"synthetic"}`); w.Code != 200 {
		t.Fatalf("alerts erase %d", w.Code)
	}
}

func TestJourneyNegatives(t *testing.T) {
	pool := freshDB(t)
	owner := seedOwner(t, pool)
	foreignOwner := seedOwner(t, pool)
	cloudStore := paidStore(t, pool, owner)
	deviceA := readyDevice(t, cloudStore, owner)
	owners := map[string]string{deviceA.FamilyID: owner.AccountID}
	foreignFam := newid(t)
	owners[foreignFam] = foreignOwner.AccountID
	keys := map[string][2]string{owner.AccountID: {owner.ContributorID, owner.KeyID}}
	cloudHandler := protocolHandler(t, cloudStore, owners, keys)
	ledgerStore := ledgeradapters.NewStore(pool)
	if _, err := ledgerStore.SetConsent(context.Background(), owner.AccountID, []string{"refuel", "expense"}, true); err != nil {
		t.Fatal(err)
	}
	alertsStore := alertsadapters.NewStore(pool)
	alertsHandler := alertshttp.Handler{Service: alertsStore, Sessions: journeySessions(owners), SignedOwner: journeySigned}
	ledgerHandler := ledgerhttp.Handler{Service: ledgerStore, CursorSecret: []byte(strings.Repeat("s", 32)), Origin: "one.example.test", Sessions: journeySessions(owners), SignedOwner: journeySigned}
	metricsHandler := metricshttp.Handler{Service: metricsadapters.NewStore(journeyLedgerReader{store: ledgerStore}), Sessions: journeySessions(owners), SignedOwner: journeySigned}
	v := vehicle(t)
	createBody := journeyBatchJSON(deviceA.FamilyID, deviceA.DeviceID, 1, 1, vehicleCommand(t, v, 0))
	w := journeyCall(t, cloudHandler, "/v1/cloud/sync/push", createBody)
	if w.Code != 200 {
		t.Fatalf("create %d", w.Code)
	}
	stale := v
	stale.Name = "Stale overwrite"
	w = journeyCall(t, cloudHandler, "/v1/cloud/sync/push", journeyBatchJSON(deviceA.FamilyID, deviceA.DeviceID, 1, 1, vehicleCommand(t, stale, 0)))
	if w.Code != 409 {
		t.Fatalf("order violation %d", w.Code)
	}
	w = journeyCall(t, cloudHandler, "/v1/cloud/sync/push", createBody)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"replayed":true`) {
		t.Fatalf("response loss %d %s", w.Code, w.Body.String())
	}
	if _, err := pool.Exec(context.Background(), `CREATE FUNCTION reject_journey_insert() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'synthetic'; END $$; CREATE TRIGGER reject_journey_insert BEFORE INSERT ON ledger_records FOR EACH ROW EXECUTE FUNCTION reject_journey_insert()`); err != nil {
		t.Fatal(err)
	}
	refuelJSON := func() string {
		raw, _ := json.Marshal(map[string]any{"id": newid(t), "vehicle_id": v.ID, "occurred_at": "2026-03-15T10:30:00-03:00", "timezone": "America/Sao_Paulo", "fuel": "ETHANOL", "unit": "L", "quantity_milli": 20000, "total_centavos": 12400, "fill": "full"})
		cmds, _ := json.Marshal([]map[string]any{{"operation_id": newid(t), "dataset": "refuel", "id": newid(t), "payload": json.RawMessage(raw)}})
		return `{"family_id":"` + deviceA.FamilyID + `","access_token":"synthetic","commands":` + string(cmds) + `}`
	}
	if w := journeyCall(t, ledgerHandler, "/v1/ledger/commands", refuelJSON()); w.Code/100 != 5 {
		t.Fatalf("crash %d", w.Code)
	}
	if _, err := pool.Exec(context.Background(), "DROP TRIGGER reject_journey_insert ON ledger_records"); err != nil {
		t.Fatal(err)
	}
	restartedLedger := ledgeradapters.NewStore(pool)
	restartedHandler := ledgerhttp.Handler{Service: restartedLedger, CursorSecret: []byte(strings.Repeat("s", 32)), Origin: "one.example.test", Sessions: journeySessions(owners), SignedOwner: journeySigned}
	if w := journeyCall(t, restartedHandler, "/v1/ledger/commands", refuelJSON()); w.Code != 200 {
		t.Fatalf("restart %d", w.Code)
	}
	foreignList := `{"family_id":"` + foreignFam + `","access_token":"synthetic","dataset":"refuel","limit":20}`
	w = journeyCall(t, restartedHandler, "/v1/ledger/records", foreignList)
	if w.Code != 200 || strings.Contains(w.Body.String(), v.ID) {
		t.Fatalf("idor leak %d %s", w.Code, w.Body.String())
	}
	crossover := `{"family_id":"` + foreignFam + `","access_token":"synthetic","device_id":"` + deviceA.DeviceID + `","limit":20}`
	if w := journeyCall(t, cloudHandler, "/v1/cloud/sync/pull", crossover); w.Code != 401 {
		t.Fatalf("account switch admitted %d", w.Code)
	}
	update := v
	update.Name = "Corrected name"
	w = journeyCall(t, cloudHandler, "/v1/cloud/sync/push", journeyBatchJSON(deviceA.FamilyID, deviceA.DeviceID, 1, 1, vehicleCommand(t, update, 1)))
	if w.Code != 200 {
		t.Fatalf("lifecycle correction %d", w.Code)
	}
	for path, body := range map[string]string{
		"/v1/cloud/sync/push": `{"family_id":"` + deviceA.FamilyID + `","access_token":"synthetic","device_id":"` + deviceA.DeviceID + `","epoch":1,"consent_revision":1,"commands":[]}`,
		"/v1/ledger/commands": `{"family_id":"` + deviceA.FamilyID + `","access_token":"synthetic","commands":[]}`,
		"/v1/cloud/sync/pull": `{"family_id":"` + deviceA.FamilyID + `","access_token":"synthetic","device_id":"` + deviceA.DeviceID + `","limit":101}`,
		"/v1/metrics/compare": `{"family_id":"` + deviceA.FamilyID + `","access_token":"synthetic","baseline":{"vehicle_id":"not-a-uuid","from":"2026-01-01T00:00:00Z","to":"2026-02-01T00:00:00Z","timezone":"UTC","method":"fuel-purchase-interval"},"current":{"vehicle_id":"` + v.ID + `","from":"2026-01-01T00:00:00Z","to":"2026-02-01T00:00:00Z","timezone":"UTC","method":"fuel-purchase-interval"}}`,
		"/v1/ledger/records":  `{"family_id":"` + deviceA.FamilyID + `","access_token":"synthetic","dataset":"refuel","limit":20,"cursor":"forged"}`,
		"/v1/alerts/commands": `{"family_id":"` + deviceA.FamilyID + `","access_token":"synthetic","commands":[]}`,
	} {
		var handler journeyRouter = cloudHandler
		switch {
		case strings.HasPrefix(path, "/v1/ledger/"):
			handler = restartedHandler
		case strings.HasPrefix(path, "/v1/metrics/"):
			handler = metricsHandler
		case strings.HasPrefix(path, "/v1/alerts/"):
			handler = alertsHandler
		}
		if w := journeyCall(t, handler, path, body); w.Code != 400 {
			t.Fatalf("negative %s admitted %d", path, w.Code)
		}
	}
}
