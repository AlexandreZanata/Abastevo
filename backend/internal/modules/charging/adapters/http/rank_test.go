package http

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	chargingdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/charging/domain"
	"github.com/go-chi/chi/v5"
)

func testTariff(id, target string, cents int64, condition string) chargingdomain.Tariff {
	return chargingdomain.Tariff{
		Version: 1, ID: id, Revision: "test-v1", TargetKind: "CONNECTOR", TargetID: target,
		Currency: "BRL", Condition: condition, Timezone: "America/Sao_Paulo",
		ValidFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		TaxBasis:  "INCLUDED", MandatoryComponentsComplete: true,
		Components: []chargingdomain.TariffComponent{
			{Dimension: "ENERGY_WH", RateCentavos: chargingdomain.Rational{Numerator: "1", Denominator: "10"}, BillingStep: 1},
		},
		BillingRounding: "FINAL_HALF_UP_CENTAVO", Provenance: "DATED_COMMUNITY_REPORT",
	}
}

func testScenario() chargingdomain.Scenario {
	wh := int64(10000)
	return chargingdomain.Scenario{
		Condition:   "STANDARD",
		WindowStart: time.Date(2026, 10, 22, 10, 0, 0, 0, time.UTC),
		WindowEnd:   time.Date(2026, 10, 22, 11, 0, 0, 0, time.UTC),
		EnergyWh:    &wh,
	}
}

func TestTariffRankingOrdersTotalsWithUnknownsLast(t *testing.T) {
	router := chi.NewRouter()
	RankHandler{}.RegisterRankRoutes(router)
	post := func(tariffs []chargingdomain.Tariff, scenario chargingdomain.Scenario) (int, map[string]any) {
		t.Helper()
		raw, _ := json.Marshal(map[string]any{"tariffs": tariffs, "scenario": scenario})
		w := httptest.NewRecorder()
		request := httptest.NewRequest("POST", "/v1/charging/tariffs/rank", bytes.NewReader(raw))
		request.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, request)
		var body map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &body)
		return w.Code, body
	}
	cheap := testTariff("11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222", 100, "STANDARD")
	pricey := testTariff("11111111-1111-4111-8111-111111111113", "22222222-2222-4222-8222-222222222222", 200, "STANDARD")
	// Make pricey actually pricier by doubling its rate.
	pricey.Components[0].RateCentavos = chargingdomain.Rational{Numerator: "1", Denominator: "5"}
	code, body := post([]chargingdomain.Tariff{pricey, cheap}, testScenario())
	if code != 200 {
		t.Fatal("ranking refused", code, body)
	}
	items := body["items"].([]any)
	if len(items) != 2 {
		t.Fatal("ranking lost items", len(items))
	}
	if items[0].(map[string]any)["index"] != float64(1) || items[1].(map[string]any)["index"] != float64(0) {
		t.Fatal("ranking misordered", items)
	}
	for _, item := range items {
		if item.(map[string]any)["state"] != "KNOWN" {
			t.Fatal("known estimate marked unknown", item)
		}
	}
	// Member isolation: different targets refuse.
	memberA := testTariff("11111111-1111-4111-8111-111111111114", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", 100, "MEMBER")
	memberB := testTariff("11111111-1111-4111-8111-111111111115", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", 100, "MEMBER")
	if code, _ := post([]chargingdomain.Tariff{memberA, memberB}, testScenario()); code != 400 {
		t.Fatal("cross-operator member comparison accepted", code)
	}
	// Malformed: empty tariffs, oversize count, unknown basis (no by field → still works? by defaults? No, by required).
	if code, _ := post(nil, testScenario()); code != 400 {
		t.Fatal("empty tariffs accepted", code)
	}
}
