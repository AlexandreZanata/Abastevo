package application

import (
	"context"
	"errors"
	"testing"
)

func bulletinQuery() BulletinQuery {
	return BulletinQuery{
		MethodologyVersion: BulletinMethodologyVersion,
		Fuel:               "GASOLINE_REGULAR",
		Unit:               "L",
	}
}

func bulletinRevision(id, start, end string) BulletinRevision {
	return BulletinRevision{
		RevisionID:     id,
		SurveyStart:    start,
		SurveyEnd:      end,
		PublishedAt:    "2026-11-06T12:00:00Z",
		SourceURL:      "https://www.gov.br/anp/synthetic.xlsx",
		SourceChecksum: "sha256:synthetic-" + id,
		ParserVersion:  "synthetic-parser-v1",
	}
}

func bulletinSample(uf string, sum, count, sourceObservations, sourceStations int64) BulletinSample {
	return BulletinSample{
		UF:               uf,
		SumMilliBrl:      sum,
		ObservationCount: count,
		Coverage: BulletinCoverage{
			SourceObservations:          sourceObservations,
			SampledStations:             count,
			SourceStations:              sourceStations,
			Sampled:                     true,
			NationalCompletenessClaimed: false,
		},
	}
}

func TestValidateBulletinQuery(t *testing.T) {
	valid, err := ValidateBulletinQuery(bulletinQuery())
	if err != nil {
		t.Fatalf("valid query refused: %v", err)
	}
	if valid.MethodologyVersion != BulletinMethodologyVersion || valid.Fuel != "GASOLINE_REGULAR" || valid.Unit != "L" {
		t.Fatalf("unexpected normalized query %+v", valid)
	}
	gas, err := ValidateBulletinQuery(BulletinQuery{
		MethodologyVersion: BulletinMethodologyVersion,
		Fuel:               "CNG",
		Unit:               "M3",
	})
	if err != nil || gas.Unit != "M3" {
		t.Fatalf("CNG/M3 must be accepted: %+v %v", gas, err)
	}

	cases := []BulletinQuery{
		{MethodologyVersion: "v2", Fuel: "GASOLINE_REGULAR", Unit: "L"},
		{MethodologyVersion: BulletinMethodologyVersion, Fuel: "DIESEL", Unit: "L"},
		{MethodologyVersion: BulletinMethodologyVersion, Fuel: "CNG", Unit: "L"},
		{MethodologyVersion: BulletinMethodologyVersion, Fuel: "GASOLINE_REGULAR", Unit: "KG_13"},
	}
	for _, q := range cases {
		if _, err := ValidateBulletinQuery(q); !errors.Is(err, ErrBulletinInvalid) {
			t.Errorf("query %+v: expected ErrBulletinInvalid, got %v", q, err)
		}
	}
}

func TestBuildBulletinSetComputesExactMeanAndCoverage(t *testing.T) {
	current := bulletinRevision("00000000-0000-4000-8000-000000000001", "2026-10-26", "2026-11-01")
	previous := bulletinRevision("00000000-0000-4000-8000-000000000002", "2026-10-19", "2026-10-25")

	set, err := BuildBulletinSet(
		bulletinQuery(),
		current,
		previous,
		[]BulletinSample{bulletinSample("SP", 300003, 30, 33, 40)},
		[]BulletinSample{bulletinSample("SP", 270000, 30, 30, 38)},
	)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if set.MethodologyVersion != BulletinMethodologyVersion || len(set.Pairs) != 1 || len(set.Skipped) != 0 {
		t.Fatalf("unexpected set %+v", set)
	}
	pair := set.Pairs[0]
	if pair.UF != "SP" || pair.Fuel != "GASOLINE_REGULAR" || pair.Unit != "L" {
		t.Fatalf("unexpected pair identity %+v", pair)
	}
	if pair.Current.SumMilliBrl != 300003 || pair.Current.ObservationCount != 30 ||
		pair.Current.MeanNumerator != 100001 || pair.Current.MeanDenominator != 10 ||
		pair.Current.Mean != "100001/10" {
		t.Fatalf("current exact mean %+v", pair.Current)
	}
	if pair.Previous.MeanNumerator != 9000 || pair.Previous.MeanDenominator != 1 || pair.Previous.Mean != "9000" {
		t.Fatalf("previous exact mean %+v", pair.Previous)
	}
	if pair.Current.Coverage.SourceObservations != 33 ||
		pair.Current.Coverage.SampledStations != 30 ||
		pair.Current.Coverage.SourceStations != 40 ||
		!pair.Current.Coverage.Sampled ||
		pair.Current.Coverage.NationalCompletenessClaimed {
		t.Fatalf("coverage %+v", pair.Current.Coverage)
	}
	if set.CurrentRevision.RevisionID != current.RevisionID ||
		set.PreviousRevision.RevisionID != previous.RevisionID ||
		set.CurrentRevision.SourceChecksum != current.SourceChecksum {
		t.Fatalf("manifest not preserved: %+v", set)
	}
}

func TestBuildBulletinSetSkipsIneligibleRegions(t *testing.T) {
	current := bulletinRevision("00000000-0000-4000-8000-000000000001", "2026-10-26", "2026-11-01")
	previous := bulletinRevision("00000000-0000-4000-8000-000000000002", "2026-10-19", "2026-10-25")

	set, err := BuildBulletinSet(
		bulletinQuery(),
		current,
		previous,
		[]BulletinSample{
			bulletinSample("SP", 300000, 30, 30, 35),
			bulletinSample("MG", 300000, 30, 30, 35),
			bulletinSample("BA", 290000, 29, 29, 35),
			bulletinSample("PR", 300000, 30, 30, 35),
		},
		[]BulletinSample{
			bulletinSample("SP", 270000, 30, 30, 35),
			bulletinSample("RJ", 270000, 30, 30, 35),
			bulletinSample("BA", 270000, 30, 30, 35),
			bulletinSample("PR", 0, 30, 30, 35),
		},
	)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if len(set.Pairs) != 1 || set.Pairs[0].UF != "SP" {
		t.Fatalf("only SP must be eligible, got %+v", set.Pairs)
	}
	want := map[string]BulletinSkipReason{
		"BA": BulletinSkipBelowMinStations,
		"MG": BulletinSkipNoPreviousSample,
		"PR": BulletinSkipNonPositiveSample,
		"RJ": BulletinSkipNoCurrentSample,
	}
	if len(set.Skipped) != len(want) {
		t.Fatalf("expected %d skips, got %+v", len(want), set.Skipped)
	}
	for _, skip := range set.Skipped {
		if want[skip.UF] != skip.Reason {
			t.Errorf("skip %s: expected %s, got %s", skip.UF, want[skip.UF], skip.Reason)
		}
	}
}

func TestBuildBulletinSetRejectsInconsistentAggregates(t *testing.T) {
	current := bulletinRevision("00000000-0000-4000-8000-000000000001", "2026-10-26", "2026-11-01")
	previous := bulletinRevision("00000000-0000-4000-8000-000000000002", "2026-10-19", "2026-10-25")

	t.Run("missing pair", func(t *testing.T) {
		_, err := BuildBulletinSet(bulletinQuery(), BulletinRevision{}, previous, nil, nil)
		if !errors.Is(err, ErrBulletinUnavailable) {
			t.Fatalf("expected ErrBulletinUnavailable, got %v", err)
		}
	})

	t.Run("duplicate current UF", func(t *testing.T) {
		_, err := BuildBulletinSet(
			bulletinQuery(), current, previous,
			[]BulletinSample{
				bulletinSample("SP", 300000, 30, 30, 35),
				bulletinSample("SP", 300001, 30, 30, 35),
			},
			[]BulletinSample{bulletinSample("SP", 270000, 30, 30, 35)},
		)
		if !errors.Is(err, ErrBulletinInvalidAggregate) {
			t.Fatalf("expected ErrBulletinInvalidAggregate, got %v", err)
		}
	})

	t.Run("count differs from sampled stations", func(t *testing.T) {
		bad := bulletinSample("SP", 300000, 30, 30, 35)
		bad.ObservationCount = 31
		_, err := BuildBulletinSet(
			bulletinQuery(), current, previous,
			[]BulletinSample{bad},
			[]BulletinSample{bulletinSample("SP", 270000, 30, 30, 35)},
		)
		if !errors.Is(err, ErrBulletinInvalidAggregate) {
			t.Fatalf("expected ErrBulletinInvalidAggregate, got %v", err)
		}
	})

	t.Run("source stations below sample", func(t *testing.T) {
		bad := bulletinSample("SP", 300000, 30, 30, 29)
		_, err := BuildBulletinSet(
			bulletinQuery(), current, previous,
			[]BulletinSample{bad},
			[]BulletinSample{bulletinSample("SP", 270000, 30, 30, 35)},
		)
		if !errors.Is(err, ErrBulletinInvalidAggregate) {
			t.Fatalf("expected ErrBulletinInvalidAggregate, got %v", err)
		}
	})
}

func TestParseMilliSumRejectsOverflowAndNonPositive(t *testing.T) {
	got, err := ParseMilliSum("9223372036854775807")
	if err != nil || got != 9223372036854775807 {
		t.Fatalf("max int64: got %d, %v", got, err)
	}
	for _, raw := range []string{"9223372036854775808", "18446744073709551614", "0", "-1", "", "abc"} {
		if _, err := ParseMilliSum(raw); !errors.Is(err, ErrBulletinOverflow) {
			t.Errorf("%q: expected ErrBulletinOverflow, got %v", raw, err)
		}
	}
}

type fakeBulletinReader struct {
	current, previous BulletinRevision
	currentSamples    []BulletinSample
	previousSamples   []BulletinSample
	err               error
	gotRevision       string
}

func (f *fakeBulletinReader) LatestTwoCurrentPublishedRevisions(ctx context.Context) (BulletinRevision, BulletinRevision, error) {
	if f.err != nil {
		return BulletinRevision{}, BulletinRevision{}, f.err
	}
	return f.current, f.previous, nil
}

func (f *fakeBulletinReader) ReadBulletinSamples(ctx context.Context, revisionID, fuel, unit string) ([]BulletinSample, error) {
	f.gotRevision = revisionID
	if fuel != "GASOLINE_REGULAR" || unit != "L" {
		return nil, ErrBulletinInvalid
	}
	if revisionID == f.current.RevisionID {
		return f.currentSamples, nil
	}
	return f.previousSamples, nil
}

func TestReadWeeklyBulletinsUsesOfficialPort(t *testing.T) {
	current := bulletinRevision("00000000-0000-4000-8000-000000000001", "2026-10-26", "2026-11-01")
	previous := bulletinRevision("00000000-0000-4000-8000-000000000002", "2026-10-19", "2026-10-25")
	reader := &fakeBulletinReader{
		current:         current,
		previous:        previous,
		currentSamples:  []BulletinSample{bulletinSample("SP", 300000, 30, 30, 35)},
		previousSamples: []BulletinSample{bulletinSample("SP", 270000, 30, 30, 35)},
	}
	set, err := ReadWeeklyBulletins(context.Background(), reader, bulletinQuery())
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(set.Pairs) != 1 || set.Pairs[0].UF != "SP" {
		t.Fatalf("unexpected set %+v", set)
	}
	if reader.gotRevision != previous.RevisionID {
		t.Fatalf("reader not called for both revisions: %q", reader.gotRevision)
	}

	reader.err = ErrBulletinUnavailable
	if _, err := ReadWeeklyBulletins(context.Background(), reader, bulletinQuery()); !errors.Is(err, ErrBulletinUnavailable) {
		t.Fatalf("expected unavailable, got %v", err)
	}
}
