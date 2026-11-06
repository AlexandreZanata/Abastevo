package application

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"strings"
)

// Frozen FOR-25 methodology. The first bulletin is a cross-sectional mean of
// one latest observation per station in two accepted complete survey windows.
const (
	BulletinMethodologyVersion     = "forum-official-mean-v1"
	BulletinMinimumSampledStations = 30
)

var (
	ErrBulletinInvalid          = errors.New("official: invalid bulletin query")
	ErrBulletinUnavailable      = errors.New("official: bulletin revision pair unavailable")
	ErrBulletinOverflow         = errors.New("official: bulletin sum overflow")
	ErrBulletinInvalidAggregate = errors.New("official: invalid bulletin aggregate")
)

// BulletinQuery is the validated typed intent for one fuel/unit comparison.
type BulletinQuery struct {
	MethodologyVersion string
	Fuel               string
	Unit               string
}

// BulletinRevision is the immutable input manifest for one compared revision.
type BulletinRevision struct {
	RevisionID     string
	SurveyStart    string
	SurveyEnd      string
	PublishedAt    string
	SourceURL      string
	SourceChecksum string
	ParserVersion  string
}

// BulletinCoverage records the sampled and source denominators. A bulletin is
// always sampled and never claims national completeness.
type BulletinCoverage struct {
	SourceObservations          int64
	SampledStations             int64
	SourceStations              int64
	Sampled                     bool
	NationalCompletenessClaimed bool
}

// BulletinSample is one exact UF/fuel/unit aggregate for one revision.
type BulletinSample struct {
	UF               string
	SumMilliBrl      int64
	ObservationCount int64
	MeanNumerator    int64
	MeanDenominator  int64
	Mean             string
	Coverage         BulletinCoverage
}

// BulletinPair is one eligible current/previous UF comparison.
type BulletinPair struct {
	UF       string
	Fuel     string
	Unit     string
	Current  BulletinSample
	Previous BulletinSample
}

// BulletinSkipReason is the typed operational reason a UF was not compared.
type BulletinSkipReason string

const (
	BulletinSkipNoCurrentSample   BulletinSkipReason = "NO_CURRENT_SAMPLE"
	BulletinSkipNoPreviousSample  BulletinSkipReason = "NO_PREVIOUS_SAMPLE"
	BulletinSkipBelowMinStations  BulletinSkipReason = "BELOW_MIN_STATIONS"
	BulletinSkipNonPositiveSample BulletinSkipReason = "NON_POSITIVE_SAMPLE"
)

// BulletinSkip records one absent or ineligible region without inventing data.
type BulletinSkip struct {
	UF     string
	Reason BulletinSkipReason
}

// BulletinSet is the reproducible output of one aggregate read.
type BulletinSet struct {
	MethodologyVersion string
	Query              BulletinQuery
	CurrentRevision    BulletinRevision
	PreviousRevision   BulletinRevision
	Pairs              []BulletinPair
	Skipped            []BulletinSkip
}

// BulletinReader is the Official-owned bounded read port.
type BulletinReader interface {
	LatestTwoCurrentPublishedRevisions(ctx context.Context) (BulletinRevision, BulletinRevision, error)
	ReadBulletinSamples(ctx context.Context, revisionID, fuel, unit string) ([]BulletinSample, error)
}

// ValidateBulletinQuery accepts only the frozen methodology and Official
// ranking fuel/unit vocabulary.
func ValidateBulletinQuery(q BulletinQuery) (BulletinQuery, error) {
	q.MethodologyVersion = strings.TrimSpace(q.MethodologyVersion)
	q.Fuel = strings.TrimSpace(q.Fuel)
	q.Unit = strings.TrimSpace(q.Unit)
	if q.MethodologyVersion != BulletinMethodologyVersion {
		return BulletinQuery{}, fmt.Errorf("%w: methodology", ErrBulletinInvalid)
	}
	if unit, ok := rankingUnits[q.Fuel]; !ok || unit != q.Unit {
		return BulletinQuery{}, fmt.Errorf("%w: fuel/unit", ErrBulletinInvalid)
	}
	return q, nil
}

// ReadWeeklyBulletins reads the two current complete revisions and their
// bounded UF samples, then applies the frozen eligibility and exact mean rules.
func ReadWeeklyBulletins(ctx context.Context, reader BulletinReader, q BulletinQuery) (BulletinSet, error) {
	validated, err := ValidateBulletinQuery(q)
	if err != nil {
		return BulletinSet{}, err
	}
	current, previous, err := reader.LatestTwoCurrentPublishedRevisions(ctx)
	if err != nil {
		return BulletinSet{}, err
	}
	currentSamples, err := reader.ReadBulletinSamples(ctx, current.RevisionID, validated.Fuel, validated.Unit)
	if err != nil {
		return BulletinSet{}, err
	}
	previousSamples, err := reader.ReadBulletinSamples(ctx, previous.RevisionID, validated.Fuel, validated.Unit)
	if err != nil {
		return BulletinSet{}, err
	}
	return BuildBulletinSet(validated, current, previous, currentSamples, previousSamples)
}

// BuildBulletinSet is pure: deterministic order, exact rational means and
// typed skips for absent/ineligible regions.
func BuildBulletinSet(q BulletinQuery, current, previous BulletinRevision, currentSamples, previousSamples []BulletinSample) (BulletinSet, error) {
	validated, err := ValidateBulletinQuery(q)
	if err != nil {
		return BulletinSet{}, err
	}
	if current.RevisionID == "" || previous.RevisionID == "" {
		return BulletinSet{}, ErrBulletinUnavailable
	}
	currentByUF, err := samplesByUF(currentSamples)
	if err != nil {
		return BulletinSet{}, err
	}
	previousByUF, err := samplesByUF(previousSamples)
	if err != nil {
		return BulletinSet{}, err
	}
	ufs := make([]string, 0, len(currentByUF)+len(previousByUF))
	seen := map[string]bool{}
	for uf := range currentByUF {
		ufs = append(ufs, uf)
		seen[uf] = true
	}
	for uf := range previousByUF {
		if !seen[uf] {
			ufs = append(ufs, uf)
		}
	}
	sort.Strings(ufs)

	set := BulletinSet{
		MethodologyVersion: validated.MethodologyVersion,
		Query:              validated,
		CurrentRevision:    current,
		PreviousRevision:   previous,
		Pairs:              make([]BulletinPair, 0, len(ufs)),
		Skipped:            make([]BulletinSkip, 0),
	}
	for _, uf := range ufs {
		cur, hasCurrent := currentByUF[uf]
		prev, hasPrevious := previousByUF[uf]
		switch {
		case !hasCurrent:
			set.Skipped = append(set.Skipped, BulletinSkip{UF: uf, Reason: BulletinSkipNoCurrentSample})
			continue
		case !hasPrevious:
			set.Skipped = append(set.Skipped, BulletinSkip{UF: uf, Reason: BulletinSkipNoPreviousSample})
			continue
		case cur.Coverage.SampledStations < BulletinMinimumSampledStations ||
			prev.Coverage.SampledStations < BulletinMinimumSampledStations:
			set.Skipped = append(set.Skipped, BulletinSkip{UF: uf, Reason: BulletinSkipBelowMinStations})
			continue
		case cur.SumMilliBrl <= 0 || prev.SumMilliBrl <= 0:
			set.Skipped = append(set.Skipped, BulletinSkip{UF: uf, Reason: BulletinSkipNonPositiveSample})
			continue
		}

		curFinal, err := finalizeBulletinSample(cur)
		if err != nil {
			return BulletinSet{}, err
		}
		prevFinal, err := finalizeBulletinSample(prev)
		if err != nil {
			return BulletinSet{}, err
		}
		set.Pairs = append(set.Pairs, BulletinPair{
			UF:       uf,
			Fuel:     validated.Fuel,
			Unit:     validated.Unit,
			Current:  curFinal,
			Previous: prevFinal,
		})
	}
	return set, nil
}

// ParseMilliSum converts an exact PostgreSQL numeric sum text into a positive
// int64. Out-of-range, zero, negative and malformed values are refusals.
func ParseMilliSum(raw string) (int64, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return 0, ErrBulletinOverflow
	}
	value, ok := new(big.Int).SetString(trimmed, 10)
	if !ok || value.Sign() <= 0 || !value.IsInt64() {
		return 0, ErrBulletinOverflow
	}
	return value.Int64(), nil
}

func samplesByUF(samples []BulletinSample) (map[string]BulletinSample, error) {
	out := make(map[string]BulletinSample, len(samples))
	for _, sample := range samples {
		uf := strings.TrimSpace(sample.UF)
		if uf == "" {
			return nil, ErrBulletinInvalidAggregate
		}
		if _, exists := out[uf]; exists {
			return nil, ErrBulletinInvalidAggregate
		}
		sample.UF = uf
		out[uf] = sample
	}
	return out, nil
}

func finalizeBulletinSample(sample BulletinSample) (BulletinSample, error) {
	if sample.ObservationCount <= 0 || sample.SumMilliBrl <= 0 {
		return BulletinSample{}, ErrBulletinInvalidAggregate
	}
	if sample.Coverage.SampledStations != sample.ObservationCount ||
		sample.Coverage.SourceStations < sample.Coverage.SampledStations ||
		sample.Coverage.SourceObservations < sample.ObservationCount {
		return BulletinSample{}, ErrBulletinInvalidAggregate
	}
	divisor := gcdInt64(sample.SumMilliBrl, sample.ObservationCount)
	sample.MeanNumerator = sample.SumMilliBrl / divisor
	sample.MeanDenominator = sample.ObservationCount / divisor
	if sample.MeanDenominator == 1 {
		sample.Mean = fmt.Sprintf("%d", sample.MeanNumerator)
	} else {
		sample.Mean = fmt.Sprintf("%d/%d", sample.MeanNumerator, sample.MeanDenominator)
	}
	sample.Coverage.Sampled = true
	sample.Coverage.NationalCompletenessClaimed = false
	return sample, nil
}
