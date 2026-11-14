package portalgen

import (
	"fmt"
	"sort"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/enterpriseexport"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/enterpriseexport/analytics"
)

func summaryRows(summaries []analytics.Summary, level, code string, markInsufficient bool) []summaryRow {
	rows := make([]summaryRow, 0)
	for _, summary := range summaries {
		if summary.Geography.Level != level || summary.Geography.Code != code {
			continue
		}
		rows = append(rows, summaryRow{
			Code: summary.Fuel, Fuel: fuelLabel(summary.Fuel), FuelName: fuelName(summary.Fuel), Unit: summary.Unit,
			Stations: summary.StationCount, Observations: summary.SourceObservations,
			Min: summary.MinMilliBRL, Max: summary.MaxMilliBRL, Spread: summary.SpreadMilliBRL,
			Insufficient: markInsufficient && summary.StationCount < minMunicipality,
		})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Fuel != rows[j].Fuel {
			return rows[i].Fuel < rows[j].Fuel
		}
		return rows[i].Unit < rows[j].Unit
	})
	return rows
}

func rankingViews(rankings []analytics.Ranking, level, code string, stations map[string]enterpriseexport.Station, pageCap bool) ([]rankingView, error) {
	views := make([]rankingView, 0)
	for _, ranking := range rankings {
		if ranking.Geography.Level != level || (code != "" && ranking.Geography.Code != code) {
			continue
		}
		rows := make([]rankingRow, 0, len(ranking.Entries))
		for _, entry := range ranking.Entries {
			station, ok := stations[entry.StationID]
			if !ok {
				return nil, fmt.Errorf("portalgen: ranking references unknown station")
			}
			rows = append(rows, rankingRow{
				Position: entry.Position, Name: stationDisplayTitle(station.DisplayName), LegalName: stationLegalName(station), StationID: entry.StationID,
				CNPJ: cnpjBR(station.CNPJ), Place: stationPlace(station),
				Status: statusLabel(entry.OperationStatus), Amount: entry.AmountMilliBRL,
				Collected: dateBR(entry.CollectedOn), TieCount: entry.TieCount,
			})
		}
		view := rankingView{
			Title: fuelLabel(ranking.Fuel) + " · " + ranking.Unit,
			Fuel:  fuelLabel(ranking.Fuel), Unit: ranking.Unit,
			Population: ranking.PopulationStations, Rows: rows,
			Truncated: ranking.Truncated, TiesAtCutoff: ranking.TiesAtCutoff,
		}
		if pageCap && len(view.Rows) > maxRankingPerPage {
			cutoff := view.Rows[maxRankingPerPage-1].Amount
			end := maxRankingPerPage
			for end < len(view.Rows) && view.Rows[end].Amount == cutoff {
				end++
			}
			view.Rows = view.Rows[:end]
			view.Truncated = true
			view.TiesAtCutoff = 0
			for _, row := range view.Rows {
				if row.Amount == cutoff {
					view.TiesAtCutoff++
				}
			}
		}
		views = append(views, view)
	}
	sort.Slice(views, func(i, j int) bool {
		if views[i].Fuel != views[j].Fuel {
			return views[i].Fuel < views[j].Fuel
		}
		return views[i].Unit < views[j].Unit
	})
	return views, nil
}

func observationRows(items []observation, limit int, ranked bool) []rankingRow {
	sort.Slice(items, func(i, j int) bool {
		if ranked && items[i].price.AmountMilliBRL != items[j].price.AmountMilliBRL {
			return items[i].price.AmountMilliBRL < items[j].price.AmountMilliBRL
		}
		if items[i].station.DisplayName != items[j].station.DisplayName {
			return items[i].station.DisplayName < items[j].station.DisplayName
		}
		return items[i].station.StationID < items[j].station.StationID
	})
	counts := map[int64]int{}
	for _, item := range items {
		counts[item.price.AmountMilliBRL]++
	}
	rows := make([]rankingRow, 0, len(items))
	for i, item := range items {
		position := 0
		if ranked {
			position = i + 1
		}
		rows = append(rows, rankingRow{
			Position: position, Name: stationDisplayTitle(item.station.DisplayName), LegalName: stationLegalName(item.station), StationID: item.station.StationID,
			CNPJ: cnpjBR(item.station.CNPJ), Place: stationPlace(item.station),
			Status: statusLabel(item.station.OperationStatus), Amount: item.price.AmountMilliBRL,
			Collected: dateBR(item.price.CollectedOn), TieCount: counts[item.price.AmountMilliBRL],
		})
		if limit > 0 && len(rows) >= limit {
			break
		}
	}
	return rows
}

func observationRankings(items []observation) []rankingView {
	groups := map[string][]observation{}
	for _, item := range items {
		key := item.price.FuelProduct + "\x00" + item.price.Unit
		groups[key] = append(groups[key], item)
	}
	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	views := make([]rankingView, 0, len(keys))
	for _, key := range keys {
		group := groups[key]
		rows := observationRows(group, 0, true)
		if len(rows) > maxRankingPerPage {
			cutoff := rows[maxRankingPerPage-1].Amount
			end := maxRankingPerPage
			for end < len(rows) && rows[end].Amount == cutoff {
				end++
			}
			rows = rows[:end]
			ties := 0
			for _, row := range rows {
				if row.Amount == cutoff {
					ties++
				}
			}
			views = append(views, rankingView{
				Title: fuelLabel(group[0].price.FuelProduct) + " · " + group[0].price.Unit,
				Fuel:  fuelLabel(group[0].price.FuelProduct), Unit: group[0].price.Unit,
				Population: len(group), Rows: rows, Truncated: true, TiesAtCutoff: ties,
			})
			continue
		}
		views = append(views, rankingView{
			Title: fuelLabel(group[0].price.FuelProduct) + " · " + group[0].price.Unit,
			Fuel:  fuelLabel(group[0].price.FuelProduct), Unit: group[0].price.Unit,
			Population: len(group), Rows: rows,
		})
	}
	return views
}

func isUF(value string) bool {
	if len(value) != 2 {
		return false
	}
	for _, r := range value {
		if r < 'A' || r > 'Z' {
			return false
		}
	}
	return true
}

func isIBGE(value string) bool {
	if len(value) != 7 {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func stationPlace(station enterpriseexport.Station) string {
	if station.MunicipalityCode != "" && station.State != "" {
		return station.MunicipalityCode + "/" + station.State
	}
	if station.State != "" {
		return station.State
	}
	return "localização não informada"
}
