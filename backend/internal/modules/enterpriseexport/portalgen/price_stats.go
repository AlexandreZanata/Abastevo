package portalgen

import (
	"sort"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/enterpriseexport/analytics"
)

// sampleMean is the B-BR-SEO-10 mean of one summary: the sum of the latest
// price of each station divided by the station count, in milli-BRL, rounded
// half up. It is not shown for a sample below minMunicipality stations.
func sampleMean(sumMilliBRL int64, stations int) (int64, bool) {
	if stations < minMunicipality || sumMilliBRL <= 0 {
		return 0, false
	}
	n := int64(stations)
	return (2*sumMilliBRL + n) / (2 * n), true
}

func summaryMean(summary analytics.Summary) (int64, bool) {
	return sampleMean(summary.SumMilliBRL, summary.StationCount)
}

// summaryKey identifies one summary inside a revision.
type summaryKey struct {
	level, code, fuel, unit string
}

func keyOf(summary analytics.Summary) summaryKey {
	return summaryKey{summary.Geography.Level, summary.Geography.Code, summary.Fuel, summary.Unit}
}

func indexSummaries(summaries []analytics.Summary) map[summaryKey]analytics.Summary {
	index := make(map[summaryKey]analytics.Summary, len(summaries))
	for _, summary := range summaries {
		index[keyOf(summary)] = summary
	}
	return index
}

// canonicalSummary reports whether a summary is in its product's canonical
// unit, the only unit a price page compares.
func canonicalSummary(summary analytics.Summary) bool {
	unit, err := productUnit(summary.Fuel)
	return err == nil && unit == summary.Unit
}

// comparisonRow is one fuel of the "previous survey" block. A comparable
// row shows movements; a row whose station sample changed shows the previous
// figures and no variation (B-BR-SEO-10).
type comparisonRow struct {
	Code             string
	Fuel             string
	PreviousStations int
	CurrentStations  int
	Comparable       bool
	Min, Max, Mean   string
	Situation        string
	// MeanDelta is the difference of the two rounded means of a comparable
	// row; HasMeanDelta is false when either sample shows no mean.
	MeanDelta    int64
	HasMeanDelta bool
}

type comparisonView struct {
	PreviousPeriod string
	Rows           []comparisonRow
}

// comparison builds the block for one geography against the previous survey;
// fuel narrows it to one product. It is nil without a previous survey or
// when no fuel of the geography was in it.
func (g *generator) comparison(current []analytics.Summary, level, code, fuel string) *comparisonView {
	if g.previous == nil {
		return nil
	}
	rows := []comparisonRow{}
	for _, now := range current {
		if now.Geography.Level != level || now.Geography.Code != code || !canonicalSummary(now) || (fuel != "" && now.Fuel != fuel) {
			continue
		}
		before, ok := g.previousSummaries[keyOf(now)]
		if !ok {
			continue
		}
		change, err := analytics.CompareSummaries(before, now)
		if err != nil || (!change.Comparable && !change.SampleChanged) {
			continue
		}
		row := comparisonRow{
			Code: now.Fuel, Fuel: fuelLabel(now.Fuel),
			PreviousStations: before.StationCount, CurrentStations: now.StationCount, Comparable: change.Comparable,
		}
		meanNow, hasNow := summaryMean(now)
		meanBefore, hasBefore := summaryMean(before)
		if change.Comparable {
			row.Min, row.Max, row.Mean = movementValue(change.ChangeMinMilliBRL), movementValue(change.ChangeMaxMilliBRL), "—"
			if hasNow && hasBefore {
				row.MeanDelta, row.HasMeanDelta = meanNow-meanBefore, true
				row.Mean = movement(row.MeanDelta)
			}
			row.Situation = "mesma amostra de postos"
		} else {
			row.Min, row.Max, row.Mean = "antes "+brl(before.MinMilliBRL), "antes "+brl(before.MaxMilliBRL), "—"
			if hasBefore {
				row.Mean = "antes " + brl(meanBefore)
			}
			row.Situation = "amostra diferente; sem variação calculada"
		}
		rows = append(rows, row)
	}
	if len(rows) == 0 {
		return nil
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if fuelRank(rows[i].Code) != fuelRank(rows[j].Code) {
			return fuelRank(rows[i].Code) < fuelRank(rows[j].Code)
		}
		return rows[i].Code < rows[j].Code
	})
	return &comparisonView{PreviousPeriod: periodLabel(g.previous.SurveyStart, g.previous.SurveyEnd), Rows: rows}
}
