package analytics

import (
	"encoding/json"
	"math/rand"
	"reflect"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/enterpriseexport"
)

const (
	stationA = "00000000-0000-4000-8000-000000000001"
	stationB = "00000000-0000-4000-8000-000000000002"
	stationC = "00000000-0000-4000-8000-000000000003"
	stationD = "00000000-0000-4000-8000-000000000004"
	rev1     = "00000000-0000-4000-8000-000000000011"
	rev2     = "00000000-0000-4000-8000-000000000012"
	rev3     = "00000000-0000-4000-8000-000000000013"
)

func fixtureInput() Input {
	stations := []enterpriseexport.Station{
		{StationID: stationA, DisplayName: "A", State: "SP", MunicipalityCode: "3550308", OperationStatus: "active", LocationQuality: "unknown"},
		{StationID: stationB, DisplayName: "B", State: "SP", MunicipalityCode: "3550308", OperationStatus: "suspended", LocationQuality: "unknown"},
		{StationID: stationC, DisplayName: "C", State: "MG", MunicipalityCode: "3106200", OperationStatus: "active", LocationQuality: "unknown"},
		{StationID: stationD, DisplayName: "D", OperationStatus: "active", LocationQuality: "unknown"},
	}
	revisions := []enterpriseexport.Revision{
		{RevisionID: rev1, SurveyStart: "2026-10-04", SurveyEnd: "2026-10-10", PublishedAt: "2026-10-11T12:00:00Z", SourceURL: "https://example.test/r1.xlsx", SourceChecksum: "sha256:r1", ParserVersion: "anp-v1"},
		{RevisionID: rev2, SurveyStart: "2026-10-11", SurveyEnd: "2026-10-17", PublishedAt: "2026-10-18T12:00:00Z", SourceURL: "https://example.test/r2.xlsx", SourceChecksum: "sha256:r2", ParserVersion: "anp-v1"},
		{RevisionID: rev3, SurveyStart: "2026-10-18", SurveyEnd: "2026-10-24", PublishedAt: "2026-10-25T12:00:00Z", SourceURL: "https://example.test/r3.xlsx", SourceChecksum: "sha256:r3", ParserVersion: "anp-v1"},
	}
	price := func(id, rev, station, fuel, unit, collected string, amount int64) enterpriseexport.Price {
		return enterpriseexport.Price{
			PriceID: id, RevisionID: rev, StationID: station,
			FuelProduct: fuel, Unit: unit, AmountMilliBRL: amount,
			Currency: "BRL", CollectedOn: collected,
		}
	}
	prices := []enterpriseexport.Price{
		price("p101", rev1, stationA, "GASOLINE_REGULAR", "L", "2026-10-08", 5999),
		price("p102", rev1, stationB, "GASOLINE_REGULAR", "L", "2026-10-08", 5899),
		price("p103", rev1, stationC, "GASOLINE_REGULAR", "L", "2026-10-08", 6099),
		price("p104", rev1, stationD, "GASOLINE_REGULAR", "L", "2026-10-08", 5799),
		price("p105", rev1, stationA, "GASOLINE_REGULAR", "L", "2026-10-09", 5899),
		price("p106", rev1, stationA, "ETHANOL", "L", "2026-10-08", 3799),
		price("p107", rev1, stationC, "CNG", "M3", "2026-10-08", 4499),
		price("p108", rev1, stationD, "LPG_P13", "KG_13", "2026-10-08", 109990),
		price("p109", rev1, "00000000-0000-4000-8000-000000000099", "GASOLINE_REGULAR", "L", "2026-10-08", 5499),
		price("p110", rev1, stationA, "GASOLINE_REGULAR", "M3", "2026-10-08", 5000),
		price("p111", rev1, stationA, "DIESEL_S10", "L", "2026-10-20", 6000),
		{PriceID: "p112", RevisionID: rev1, StationID: stationA, FuelProduct: "GASOLINE_REGULAR", Unit: "L", AmountMilliBRL: 7000, Currency: "USD", CollectedOn: "2026-10-08"},
		{PriceID: "p113", RevisionID: rev1, StationID: stationA, FuelProduct: "GASOLINE_REGULAR", Unit: "L", AmountMilliBRL: 8000, Currency: "BRL", CollectedOn: "not-a-date"},
		price("p114", rev1, stationA, "JET", "L", "2026-10-08", 5000),
		price("p201", rev2, stationA, "GASOLINE_REGULAR", "L", "2026-10-15", 6199),
		price("p202", rev2, stationB, "GASOLINE_REGULAR", "L", "2026-10-15", 6199),
		price("p203", rev2, stationC, "GASOLINE_REGULAR", "L", "2026-10-15", 6299),
		price("p204", rev2, stationD, "GASOLINE_REGULAR", "L", "2026-10-15", 6099),
	}
	return Input{ContentSHA256: "sha256:snapshot", Stations: stations, Revisions: revisions, Prices: prices}
}

func TestComputeGolden(t *testing.T) {
	projection, err := Compute(fixtureInput(), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if projection.MetricsVersion != MetricsVersion || projection.InputContentSHA256 != "sha256:snapshot" {
		t.Errorf("projection identity = %+v", projection)
	}
	if len(projection.Definitions) != 5 {
		t.Errorf("definitions = %d", len(projection.Definitions))
	}

	coverage := findCoverage(t, projection, rev1)
	if coverage.SourceObservations != 14 || coverage.IncludedObservations != 7 ||
		coverage.StationCount != 4 || coverage.UnlocatedObservations != 2 ||
		coverage.DirectoryStations != 4 || coverage.UnpricedStations != 0 {
		t.Errorf("revision 1 coverage = %+v", coverage)
	}
	for _, reason := range []string{
		"older_observation", "unknown_station", "unit_mismatch", "period_mismatch",
		"unsupported_currency", "invalid_date", "unknown_fuel",
	} {
		if coverage.Excluded[reason] != 1 {
			t.Errorf("exclusion %s = %d, want 1", reason, coverage.Excluded[reason])
		}
	}
	empty := findCoverage(t, projection, rev3)
	if empty.SourceObservations != 0 || empty.IncludedObservations != 0 || empty.StationCount != 0 {
		t.Errorf("empty revision coverage = %+v", empty)
	}

	nationalGas1 := findSummary(t, projection, rev1, "GASOLINE_REGULAR", "L", LevelNational, "BR")
	if nationalGas1.SourceObservations != 5 || nationalGas1.StationCount != 4 ||
		nationalGas1.SumMilliBRL != 23696 || nationalGas1.MinMilliBRL != 5799 ||
		nationalGas1.MaxMilliBRL != 6099 || nationalGas1.SpreadMilliBRL != 300 ||
		nationalGas1.StationSampleSHA256 == "" {
		t.Errorf("national gasoline summary = %+v", nationalGas1)
	}
	city := findSummary(t, projection, rev1, "GASOLINE_REGULAR", "L", LevelMunicipality, "3550308")
	if city.StationCount != 2 || city.SumMilliBRL != 11798 || city.SpreadMilliBRL != 0 {
		t.Errorf("municipal summary = %+v", city)
	}
	stateSP := findSummary(t, projection, rev1, "GASOLINE_REGULAR", "L", LevelState, "SP")
	if stateSP.StationCount != 2 || stateSP.SumMilliBRL != 11798 {
		t.Errorf("state summary = %+v", stateSP)
	}
	stateMG := findSummary(t, projection, rev1, "GASOLINE_REGULAR", "L", LevelState, "MG")
	if stateMG.StationCount != 1 || stateMG.MinMilliBRL != 6099 {
		t.Errorf("state MG summary = %+v", stateMG)
	}
	for _, fuel := range []struct{ fuel, unit string }{
		{"ETHANOL", "L"}, {"CNG", "M3"}, {"LPG_P13", "KG_13"},
	} {
		got := findSummary(t, projection, rev1, fuel.fuel, fuel.unit, LevelNational, "BR")
		if got.StationCount != 1 {
			t.Errorf("%s summary = %+v", fuel.fuel, got)
		}
	}
	// The national metric comes from station observations, not an equal-weight
	// average of the three municipal means (5924 vs 5932.33 milli-BRL).
	if nationalGas1.SumMilliBRL != city.SumMilliBRL+stateMG.SumMilliBRL+5799 {
		t.Errorf("national sum did not reconcile with station facts: %+v", nationalGas1)
	}
	if nationalGas1.SumMilliBRL*3 == (city.SumMilliBRL/int64(city.StationCount)+stateMG.SumMilliBRL/int64(stateMG.StationCount)+5799)*int64(nationalGas1.StationCount) {
		t.Error("national mean was fabricated as an average of municipal means")
	}

	rank := findRanking(t, projection, rev1, "GASOLINE_REGULAR", "L", LevelNational, "BR")
	if rank.PopulationStations != 4 || len(rank.Entries) != 4 {
		t.Fatalf("national ranking = %+v", rank)
	}
	wantStations := []string{stationD, stationA, stationB, stationC}
	wantAmounts := []int64{5799, 5899, 5899, 6099}
	for i, entry := range rank.Entries {
		if entry.StationID != wantStations[i] || entry.AmountMilliBRL != wantAmounts[i] ||
			entry.Position != i+1 {
			t.Errorf("entry %d = %+v", i, entry)
		}
	}
	if rank.Entries[1].TieCount != 2 || rank.Entries[1].TiedWithPrevious ||
		rank.Entries[2].TieCount != 2 || !rank.Entries[2].TiedWithPrevious {
		t.Errorf("tie semantics = %+v", rank.Entries)
	}
	stateRank := findRanking(t, projection, rev1, "GASOLINE_REGULAR", "L", LevelState, "SP")
	if stateRank.PopulationStations != 2 || len(stateRank.Entries) != 2 ||
		stateRank.Entries[0].AmountMilliBRL != 5899 || stateRank.Entries[1].AmountMilliBRL != 5899 {
		t.Errorf("state ranking = %+v", stateRank)
	}

	gas2 := findSummary(t, projection, rev2, "GASOLINE_REGULAR", "L", LevelNational, "BR")
	if gas2.StationCount != 4 || gas2.SumMilliBRL != 24796 || gas2.MinMilliBRL != 6099 ||
		gas2.MaxMilliBRL != 6299 || gas2.SpreadMilliBRL != 200 {
		t.Errorf("revision 2 gasoline summary = %+v", gas2)
	}
	change, err := CompareSummaries(nationalGas1, gas2)
	if err != nil {
		t.Fatal(err)
	}
	if !change.Comparable || change.SampleChanged ||
		change.ChangeSumMilliBRL == nil || *change.ChangeSumMilliBRL != 1100 ||
		change.ChangeMinMilliBRL == nil || *change.ChangeMinMilliBRL != 300 ||
		change.ChangeMaxMilliBRL == nil || *change.ChangeMaxMilliBRL != 200 ||
		change.ChangeSpreadMilliBRL == nil || *change.ChangeSpreadMilliBRL != -100 {
		t.Errorf("summary change = %+v", change)
	}

	history, err := StationHistory(fixtureInput().Prices, fixtureInput().Revisions, stationA, "GASOLINE_REGULAR", "L")
	if err != nil {
		t.Fatal(err)
	}
	if len(history.Points) != 3 {
		t.Fatalf("history points = %d", len(history.Points))
	}
	if history.Points[0].AmountMilliBRL == nil || *history.Points[0].AmountMilliBRL != 5899 ||
		history.Points[0].CollectedOn != "2026-10-09" || history.Points[0].Missing {
		t.Errorf("first point = %+v", history.Points[0])
	}
	if history.Points[1].AmountMilliBRL == nil || *history.Points[1].AmountMilliBRL != 6199 {
		t.Errorf("second point = %+v", history.Points[1])
	}
	if !history.Points[2].Missing || history.Points[2].AmountMilliBRL != nil {
		t.Errorf("missing week became a price: %+v", history.Points[2])
	}
	if _, err := StationHistory(fixtureInput().Prices, fixtureInput().Revisions, stationA, "GASOLINE_REGULAR", "M3"); err == nil {
		t.Error("history accepted a unit mismatch")
	}

	sampleChanged := gas2
	sampleChanged.StationSampleSHA256 = "sha256:different"
	change, err = CompareSummaries(nationalGas1, sampleChanged)
	if err != nil {
		t.Fatal(err)
	}
	if !change.SampleChanged || change.Comparable {
		t.Errorf("same-count different-sample change = %+v", change)
	}
}

func TestRankingTruncationKeepsCutoffTies(t *testing.T) {
	projection, err := Compute(fixtureInput(), Options{MaxRankingRows: 2})
	if err != nil {
		t.Fatal(err)
	}
	rank := findRanking(t, projection, rev1, "GASOLINE_REGULAR", "L", LevelNational, "BR")
	if !rank.Truncated || rank.TiesAtCutoff != 2 || len(rank.Entries) != 3 {
		t.Errorf("truncated ranking = %+v", rank)
	}
	if rank.Entries[2].AmountMilliBRL != 5899 {
		t.Errorf("cutoff tie was dropped: %+v", rank.Entries[2])
	}
}

func TestComputeReconcilesCensus(t *testing.T) {
	projection, err := Compute(fixtureInput(), Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, coverage := range projection.Coverage {
		excluded := 0
		for _, count := range coverage.Excluded {
			excluded += count
		}
		if coverage.IncludedObservations+excluded != coverage.SourceObservations {
			t.Errorf("revision %s census: included %d + excluded %d != source %d",
				coverage.RevisionID, coverage.IncludedObservations, excluded, coverage.SourceObservations)
		}
	}
	for _, ranking := range projection.Rankings {
		if ranking.Geography.Level != LevelNational {
			continue
		}
		coverage := findCoverage(t, projection, ranking.RevisionID)
		for _, fuelUnit := range coverage.FuelUnits {
			if fuelUnit.Fuel == ranking.Fuel && fuelUnit.Unit == ranking.Unit &&
				ranking.PopulationStations != fuelUnit.Stations {
				t.Errorf("ranking population %d != coverage stations %d for %s/%s",
					ranking.PopulationStations, fuelUnit.Stations, ranking.Fuel, ranking.Unit)
			}
		}
	}
}

func TestComputeIsOrderIndependent(t *testing.T) {
	first, err := Compute(fixtureInput(), Options{})
	if err != nil {
		t.Fatal(err)
	}
	input := fixtureInput()
	rng := rand.New(rand.NewSource(7))
	rng.Shuffle(len(input.Stations), func(i, j int) { input.Stations[i], input.Stations[j] = input.Stations[j], input.Stations[i] })
	rng.Shuffle(len(input.Revisions), func(i, j int) { input.Revisions[i], input.Revisions[j] = input.Revisions[j], input.Revisions[i] })
	rng.Shuffle(len(input.Prices), func(i, j int) { input.Prices[i], input.Prices[j] = input.Prices[j], input.Prices[i] })
	second, err := Compute(input, Options{})
	if err != nil {
		t.Fatal(err)
	}
	a, _ := json.Marshal(first)
	b, _ := json.Marshal(second)
	if string(a) != string(b) {
		t.Error("projection depends on input order")
	}
	if !reflect.DeepEqual(first.Coverage, second.Coverage) {
		t.Error("coverage is not order independent")
	}
}

func TestComputeRejectsInvalidInput(t *testing.T) {
	cases := []struct {
		name  string
		input Input
	}{
		{"no hash", Input{}},
		{"duplicate station", func() Input {
			in := fixtureInput()
			in.Stations = append(in.Stations, in.Stations[0])
			return in
		}()},
		{"duplicate revision", func() Input {
			in := fixtureInput()
			in.Revisions = append(in.Revisions, in.Revisions[0])
			return in
		}()},
		{"orphan price", func() Input {
			in := fixtureInput()
			in.Prices = append(in.Prices, enterpriseexport.Price{
				PriceID: "orphan", RevisionID: "missing", StationID: stationA,
				FuelProduct: "GASOLINE_REGULAR", Unit: "L", AmountMilliBRL: 1, Currency: "BRL", CollectedOn: "2026-10-08",
			})
			return in
		}()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Compute(tc.input, Options{}); err == nil {
				t.Error("invalid input accepted")
			}
		})
	}
}

func findCoverage(t *testing.T, projection Projection, revision string) Coverage {
	t.Helper()
	for _, coverage := range projection.Coverage {
		if coverage.RevisionID == revision {
			return coverage
		}
	}
	t.Fatalf("coverage for %s not found", revision)
	return Coverage{}
}

func findSummary(t *testing.T, projection Projection, revision, fuel, unit, level, code string) Summary {
	t.Helper()
	for _, summary := range projection.Summaries {
		if summary.RevisionID == revision && summary.Fuel == fuel && summary.Unit == unit &&
			summary.Geography.Level == level && summary.Geography.Code == code {
			return summary
		}
	}
	t.Fatalf("summary %s/%s/%s/%s/%s not found", revision, fuel, unit, level, code)
	return Summary{}
}

func findRanking(t *testing.T, projection Projection, revision, fuel, unit, level, code string) Ranking {
	t.Helper()
	for _, ranking := range projection.Rankings {
		if ranking.RevisionID == revision && ranking.Fuel == fuel && ranking.Unit == unit &&
			ranking.Geography.Level == level && ranking.Geography.Code == code {
			return ranking
		}
	}
	t.Fatalf("ranking %s/%s/%s/%s/%s not found", revision, fuel, unit, level, code)
	return Ranking{}
}
