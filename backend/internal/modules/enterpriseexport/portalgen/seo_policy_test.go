package portalgen

import (
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/enterpriseexport"
)

func TestPageIndexablePolicy(t *testing.T) {
	cases := []struct {
		name  string
		facts indexFacts
		want  bool
	}{
		{"hub", indexFacts{Family: familyHub}, true},
		{"fiscal hub with data", indexFacts{Family: familyFiscalHub, HasCurrentData: true}, true},
		{"fiscal hub gated", indexFacts{Family: familyFiscalHub}, false},
		{"fiscal pagination", indexFacts{Family: familyFiscalPage, HasCurrentData: true}, false},
		{"fiscal record", indexFacts{Family: familyFiscalRecord, HasCurrentData: true}, false},
		{"state with survey", indexFacts{Family: familyState, HasCurrentData: true}, true},
		{"state without survey", indexFacts{Family: familyState}, false},
		{"city with survey", indexFacts{Family: familyCity, HasCurrentData: true}, true},
		{"city without survey", indexFacts{Family: familyCity}, false},
		{"fuel hub with survey", indexFacts{Family: familyFuel, HasCurrentData: true}, true},
		{"fuel hub without survey", indexFacts{Family: familyFuel}, false},
		{"station gate passed", indexFacts{Family: familyStation, RecentOfficialPrice: true, CompleteAddress: true}, true},
		{"station stale", indexFacts{Family: familyStation, CompleteAddress: true}, false},
		{"station incomplete address", indexFacts{Family: familyStation, RecentOfficialPrice: true}, false},
		{"not found", indexFacts{Family: familyNotFound, HasCurrentData: true}, false},
	}
	for _, tc := range cases {
		if got := pageIndexable(tc.facts); got != tc.want {
			t.Errorf("%s: pageIndexable = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestRobotsValueHonoursGlobalSwitch(t *testing.T) {
	if got := robotsValue(false, true); got != "noindex,nofollow" {
		t.Errorf("global off = %q", got)
	}
	if got := robotsValue(true, true); got != "index,follow" {
		t.Errorf("indexable = %q", got)
	}
	if got := robotsValue(true, false); got != "noindex,follow" {
		t.Errorf("gated = %q", got)
	}
}

func TestRecentSurveyWeeksAndStationGate(t *testing.T) {
	revisions := map[string]enterpriseexport.Revision{}
	ids := []string{}
	// Ten weekly revisions plus a correction of the newest week.
	for i := 0; i < 10; i++ {
		id := "rev-" + string(rune('a'+i))
		start := addDays("2026-08-09", 7*i)
		revisions[id] = enterpriseexport.Revision{RevisionID: id, SurveyStart: start, SurveyEnd: addDays(start, 6)}
		ids = append(ids, id)
	}
	revisions["rev-j2"] = enterpriseexport.Revision{RevisionID: "rev-j2", SurveyStart: revisions["rev-j"].SurveyStart, SurveyEnd: revisions["rev-j"].SurveyEnd}
	recent := recentSurveyRevisions(revisions, stationRecentSurveyWeeks)
	if len(recent) != 9 || recent["rev-a"] || recent["rev-b"] || !recent["rev-c"] || !recent["rev-j2"] {
		t.Fatalf("recent revisions = %v", recent)
	}
	price := func(rev, day string) enterpriseexport.Price {
		return enterpriseexport.Price{RevisionID: rev, FuelProduct: "GASOLINE_REGULAR", Unit: "L", Currency: "BRL", AmountMilliBRL: 5999, CollectedOn: day}
	}
	full := enterpriseexport.Station{State: "MT", MunicipalityCode: "5103403", Address: map[string]string{"street": "AVENIDA CUIABÁ,", "number": "10"}}
	cases := []struct {
		name     string
		station  enterpriseexport.Station
		prices   []enterpriseexport.Price
		recent   bool
		complete bool
		last     string
	}{
		{"oldest recent week", full, []enterpriseexport.Price{price("rev-c", addDays(revisions["rev-c"].SurveyStart, 1))}, true, true, addDays(revisions["rev-c"].SurveyStart, 1)},
		{"just outside the window", full, []enterpriseexport.Price{price("rev-b", addDays(revisions["rev-b"].SurveyStart, 1))}, false, true, addDays(revisions["rev-b"].SurveyStart, 1)},
		{"collected outside its survey week", full, []enterpriseexport.Price{price("rev-j", "2026-01-01")}, false, true, ""},
		{"wrong unit", full, []enterpriseexport.Price{{RevisionID: "rev-j", FuelProduct: "GASOLINE_REGULAR", Unit: "M3", Currency: "BRL", CollectedOn: revisions["rev-j"].SurveyStart}}, false, true, ""},
		{"no street", enterpriseexport.Station{State: "MT", MunicipalityCode: "5103403", Address: map[string]string{"street": "5103403"}}, []enterpriseexport.Price{price("rev-j", revisions["rev-j"].SurveyStart)}, true, false, revisions["rev-j"].SurveyStart},
		{"no municipality", enterpriseexport.Station{State: "MT", Address: map[string]string{"street": "Rua A"}}, nil, false, false, ""},
		{"address municipality fallback", enterpriseexport.Station{State: "MT", Address: map[string]string{"street": "Rua A", "municipio": "Cuiabá"}}, nil, false, true, ""},
		{"unknown UF", enterpriseexport.Station{State: "XX", MunicipalityCode: "5103403", Address: map[string]string{"street": "Rua A"}}, nil, false, false, ""},
	}
	for _, tc := range cases {
		facts := stationIndexFacts(tc.station, tc.prices, revisions, recent)
		if facts.RecentOfficialPrice != tc.recent || facts.CompleteAddress != tc.complete || facts.LastObserved != tc.last {
			t.Errorf("%s: facts = %+v, want recent=%v complete=%v last=%q", tc.name, facts, tc.recent, tc.complete, tc.last)
		}
	}
}

func addDays(day string, n int) string {
	parsed, err := parseDay(day)
	if err != nil {
		panic(err)
	}
	return parsed.AddDate(0, 0, n).Format("2006-01-02")
}
