package application

import (
	"errors"
	"testing"
)

func rankingQuery() RankingQuery {
	return RankingQuery{
		Version: 1, Kind: "PRICE_RANKING",
		Source:        RankingSourceQuery{Kind: "ANP_WEEKLY", SurveyWeekStart: "2025-01-06"},
		Product:       RankingProduct{Fuel: "GASOLINE_REGULAR", Unit: "L"},
		Condition:     RankingCondition{Kind: "STANDARD"},
		Geography:     RankingGeography{Level: "COUNTRY", Country: "BR"},
		Metric:        RankingMetric{Kind: "AREA_MEAN", GroupBy: "MUNICIPALITY", Sort: "ASC", Weighting: "ELIGIBLE_SOURCE_OBSERVATIONS"},
		Limit:         10,
		PolicyVersion: "mex-exploration-v1",
	}
}

func rankingData() RankingData {
	return RankingData{
		Revision: RankingRevision{
			RevisionID: "revision-1", SurveyWeekStart: "2025-01-06", SurveyWeekEnd: "2025-01-12",
			PublishedAt: "2025-01-13T12:00:00Z", SourceURL: "https://www.gov.br/anp/w1.xlsx", SourceChecksum: "sha256:synthetic",
		},
		Municipalities: []MunicipalityAggregate{
			{State: "SP", MunicipalityCode: "3550308", SumMilliBrl: 11999, ObservationCount: 2, MinMilliBrl: 5998, MaxMilliBrl: 6001},
			{State: "SP", MunicipalityCode: "3509502", SumMilliBrl: 6000, ObservationCount: 1, MinMilliBrl: 6000, MaxMilliBrl: 6000},
			{State: "RJ", MunicipalityCode: "3304557", SumMilliBrl: 6100, ObservationCount: 1, MinMilliBrl: 6100, MaxMilliBrl: 6100},
		},
		SourceStations: 6, SourceObservations: 4,
	}
}

func TestValidateRankingAcceptsOnlyFrozenCombinations(t *testing.T) {
	if _, err := ValidateRanking(rankingQuery()); err != nil {
		t.Fatal(err)
	}
	observation := rankingQuery()
	observation.Metric = RankingMetric{Kind: "OBSERVATION_PRICE", GroupBy: "STATION", Sort: "DESC"}
	observation.Geography = RankingGeography{Level: "MUNICIPALITY", Country: "BR", UF: "SP", IBGECode: "3550308"}
	if _, err := ValidateRanking(observation); err != nil {
		t.Fatal(err)
	}
	state := rankingQuery()
	state.Metric.GroupBy = "STATE"
	state.Geography = RankingGeography{Level: "STATE", Country: "BR", UF: "RJ"}
	if _, err := ValidateRanking(state); err != nil {
		t.Fatal(err)
	}
}

func TestValidateRankingRefusesUnsupportedInputs(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*RankingQuery)
		want   error
	}{
		{"community source", func(q *RankingQuery) { q.Source.Kind = "COMMUNITY" }, ErrUnsupportedSource},
		{"member condition", func(q *RankingQuery) { q.Condition.Kind = "MEMBER" }, ErrUnsupportedCondition},
		{"country area group", func(q *RankingQuery) { q.Metric.GroupBy = "COUNTRY" }, ErrUnsupportedMetric},
		{"observation weighting", func(q *RankingQuery) {
			q.Metric.Kind = "OBSERVATION_PRICE"
			q.Metric.GroupBy = "STATION"
			q.Metric.Weighting = "ELIGIBLE_SOURCE_OBSERVATIONS"
		}, ErrUnsupportedMetric},
		{"wrong unit", func(q *RankingQuery) { q.Product.Fuel = "CNG"; q.Product.Unit = "L" }, ErrInvalidFilter},
		{"bad uf", func(q *RankingQuery) { q.Geography = RankingGeography{Level: "STATE", Country: "BR", UF: "XX"} }, ErrInvalidFilter},
		{"bad ibge", func(q *RankingQuery) {
			q.Geography = RankingGeography{Level: "MUNICIPALITY", Country: "BR", UF: "SP", IBGECode: "123"}
		}, ErrInvalidFilter},
		{"bad limit", func(q *RankingQuery) { q.Limit = 101 }, ErrInvalidFilter},
		{"bad week", func(q *RankingQuery) { q.Source.SurveyWeekStart = "2025-13-01" }, ErrInvalidFilter},
		{"bad policy", func(q *RankingQuery) { q.PolicyVersion = "v2" }, ErrInvalidFilter},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			q := rankingQuery()
			tc.mutate(&q)
			if _, err := ValidateRanking(q); !errors.Is(err, tc.want) {
				t.Fatalf("got %v, want %v", err, tc.want)
			}
		})
	}
}

func TestBuildAreaMeanIsExactWeightedAndStable(t *testing.T) {
	q := rankingQuery()
	result, err := BuildRanking(q, rankingData())
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Areas) != 3 {
		t.Fatalf("areas %+v", result.Areas)
	}
	want := []AreaAggregate{
		{Level: "MUNICIPALITY", State: "SP", MunicipalityCode: "3550308", MeanNumerator: 11999, MeanDenominator: 2, Mean: "11999/2", MinMilliBrl: 5998, MaxMilliBrl: 6001, VariationMilliBrl: 3, Observations: 2},
		{Level: "MUNICIPALITY", State: "SP", MunicipalityCode: "3509502", MeanNumerator: 6000, MeanDenominator: 1, Mean: "6000", MinMilliBrl: 6000, MaxMilliBrl: 6000, VariationMilliBrl: 0, Observations: 1},
		{Level: "MUNICIPALITY", State: "RJ", MunicipalityCode: "3304557", MeanNumerator: 6100, MeanDenominator: 1, Mean: "6100", MinMilliBrl: 6100, MaxMilliBrl: 6100, VariationMilliBrl: 0, Observations: 1},
	}
	for i := range want {
		if result.Areas[i] != want[i] {
			t.Fatalf("area %d = %+v, want %+v", i, result.Areas[i], want[i])
		}
	}
	if !result.Coverage.Sampled || result.Coverage.NationalCompletenessClaimed ||
		result.Coverage.SourceObservations != 4 || result.Coverage.StationsWithPrice != 4 ||
		result.Coverage.SourceStations != 6 || result.Coverage.Municipalities != 3 ||
		result.Coverage.States != 2 || result.Coverage.UnlocatedObservations != 0 {
		t.Fatalf("coverage %+v", result.Coverage)
	}
	if result.Source.RevisionID != "revision-1" || result.Source.SourceChecksum != "sha256:synthetic" {
		t.Fatalf("source %+v", result.Source)
	}
}

func TestBuildAreaMeanStateRollupAndTies(t *testing.T) {
	q := rankingQuery()
	q.Metric.GroupBy = "STATE"
	data := rankingData()
	result, err := BuildRanking(q, data)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Areas) != 2 ||
		result.Areas[0].State != "SP" || result.Areas[0].Mean != "17999/3" ||
		result.Areas[0].Observations != 3 || result.Areas[0].MinMilliBrl != 5998 ||
		result.Areas[0].MaxMilliBrl != 6001 || result.Areas[0].VariationMilliBrl != 3 ||
		result.Areas[1].State != "RJ" || result.Areas[1].Mean != "6100" {
		t.Fatalf("state rollup %+v", result.Areas)
	}

	tied := rankingQuery()
	tied.Metric.GroupBy = "MUNICIPALITY"
	data.Municipalities = []MunicipalityAggregate{
		{State: "SP", MunicipalityCode: "3550308", SumMilliBrl: 12000, ObservationCount: 2, MinMilliBrl: 6000, MaxMilliBrl: 6000},
		{State: "SP", MunicipalityCode: "3509502", SumMilliBrl: 6000, ObservationCount: 1, MinMilliBrl: 6000, MaxMilliBrl: 6000},
		{State: "RJ", MunicipalityCode: "3304557", SumMilliBrl: 6100, ObservationCount: 1, MinMilliBrl: 6100, MaxMilliBrl: 6100},
	}
	ascending, err := BuildRanking(tied, data)
	if err != nil {
		t.Fatal(err)
	}
	if ascending.Areas[0].MunicipalityCode != "3509502" || ascending.Areas[1].MunicipalityCode != "3550308" ||
		ascending.Areas[2].MunicipalityCode != "3304557" {
		t.Fatalf("ascending ties %+v", ascending.Areas)
	}
	tied.Metric.Sort = "DESC"
	descending, err := BuildRanking(tied, data)
	if err != nil {
		t.Fatal(err)
	}
	if descending.Areas[0].MunicipalityCode != "3304557" ||
		descending.Areas[1].MunicipalityCode != "3509502" || descending.Areas[2].MunicipalityCode != "3550308" {
		t.Fatalf("descending ties %+v", descending.Areas)
	}
}

func TestBuildStationRankingStableAndBounded(t *testing.T) {
	q := rankingQuery()
	q.Metric = RankingMetric{Kind: "OBSERVATION_PRICE", GroupBy: "STATION", Sort: "ASC"}
	q.Limit = 2
	data := rankingData()
	data.Municipalities = []MunicipalityAggregate{
		{State: "SP", MunicipalityCode: "3550308", SumMilliBrl: 18098, ObservationCount: 3, MinMilliBrl: 5999, MaxMilliBrl: 6100},
	}
	data.Stations = []StationObservationRow{
		{StationID: "bbbbbbbb-1111-4111-8111-000000000002", AmountMilliBrl: 5999, CollectedOn: "2025-01-08"},
		{StationID: "aaaaaaaa-1111-4111-8111-000000000001", AmountMilliBrl: 5999, CollectedOn: "2025-01-08"},
		{StationID: "cccccccc-1111-4111-8111-000000000003", AmountMilliBrl: 6100, CollectedOn: "2025-01-08"},
	}
	result, err := BuildRanking(q, data)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Stations) != 2 ||
		result.Stations[0].StationID != "aaaaaaaa-1111-4111-8111-000000000001" ||
		result.Stations[1].StationID != "bbbbbbbb-1111-4111-8111-000000000002" {
		t.Fatalf("ascending stations %+v", result.Stations)
	}
	// Coverage describes the full selected population, not the page.
	if result.Coverage.StationsWithPrice != 3 || result.Coverage.SourceObservations != 4 {
		t.Fatalf("page coverage %+v", result.Coverage)
	}
	q.Metric.Sort = "DESC"
	result, err = BuildRanking(q, data)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Stations) != 2 || result.Stations[0].StationID != "cccccccc-1111-4111-8111-000000000003" ||
		result.Stations[1].StationID != "aaaaaaaa-1111-4111-8111-000000000001" {
		t.Fatalf("descending stations %+v", result.Stations)
	}
}

func TestBuildRankingUnknownPricesStayCoverageNotZero(t *testing.T) {
	q := rankingQuery()
	data := rankingData()
	data.Municipalities = nil
	data.SourceObservations = 0
	data.SourceStations = 5
	result, err := BuildRanking(q, data)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Areas) != 0 {
		t.Fatalf("invented area %+v", result.Areas)
	}
	if result.Coverage.StationsWithPrice != 0 || result.Coverage.SourceStations != 5 ||
		result.Coverage.SourceObservations != 0 || result.Coverage.NationalCompletenessClaimed {
		t.Fatalf("coverage %+v", result.Coverage)
	}
}
