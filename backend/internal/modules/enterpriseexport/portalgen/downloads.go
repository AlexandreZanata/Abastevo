package portalgen

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/enterpriseexport"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/enterpriseexport/analytics"
)

const DownloadsSchema = "abastevo-downloads-v1"

type downloadFile struct {
	Label string
	Href  string
	Rows  int
	Bytes int64
}

type downloadEntry struct {
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
	Bytes  int64  `json:"bytes"`
	Rows   int    `json:"rows"`
}

type dadosPage struct {
	pageBase
	Release  string
	Period   string
	Files    []downloadFile
	Manifest downloadFile
	Notes    []string
}

type downloadManifest struct {
	Schema             string          `json:"schema"`
	InputContentSHA256 string          `json:"input_content_sha256"`
	Release            string          `json:"release"`
	Period             string          `json:"period"`
	PublishedAt        string          `json:"published_at"`
	Attribution        string          `json:"attribution"`
	Notes              []string        `json:"notes"`
	Files              []downloadEntry `json:"files"`
}

var downloadNotes = []string{
	"Amostra datada da pesquisa ANP; não é preço ao vivo.",
	"Sem CPF, fotos de usuários, GPS de colaborador ou documentos internos.",
	"CSV em UTF-8; campos iniciados por =, +, - ou @ são prefixados com apóstrofo.",
	"Direitos de redistribuição de preços/cadastro pendentes; não use comercialmente sem licença.",
}

func summaryLevelOrder(level string) int {
	switch level {
	case analytics.LevelNational:
		return 0
	case analytics.LevelState:
		return 1
	case analytics.LevelMunicipality:
		return 2
	default:
		return 3
	}
}

func csvSafe(value string) string {
	value = strings.ReplaceAll(value, "\r", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	if value == "" {
		return value
	}
	switch value[0] {
	case '=', '+', '-', '@', '\t':
		return "'" + value
	}
	return value
}

func writeCSV(header []string, rows [][]string) ([]byte, int, error) {
	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)
	if err := writer.Write(header); err != nil {
		return nil, 0, err
	}
	for _, row := range rows {
		for i := range row {
			row[i] = csvSafe(row[i])
		}
		if err := writer.Write(row); err != nil {
			return nil, 0, err
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, 0, err
	}
	return buf.Bytes(), len(rows), nil
}

func sortedStations(stations map[string]enterpriseexport.Station) []enterpriseexport.Station {
	out := make([]enterpriseexport.Station, 0, len(stations))
	for _, station := range stations {
		out = append(out, station)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].DisplayName != out[j].DisplayName {
			return out[i].DisplayName < out[j].DisplayName
		}
		return out[i].StationID < out[j].StationID
	})
	return out
}

func (g *generator) writeDownloads(snapshot enterpriseexport.Snapshot, latest *enterpriseexport.Revision, stations map[string]enterpriseexport.Station, observations []observation, summaries []analytics.Summary, release, period, published, inputHash string) ([]downloadFile, downloadFile, error) {
	fuelCount := map[string]int{}
	for _, item := range observations {
		fuelCount[item.station.StationID]++
	}
	stationRows := make([][]string, 0, len(stations))
	for _, station := range sortedStations(stations) {
		stationRows = append(stationRows, []string{
			station.StationID, station.DisplayName, station.CNPJ,
			station.MunicipalityCode, strings.TrimSpace(station.Address["municipio"]),
			station.State, station.OperationStatus, station.LocationQuality,
			strings.TrimSpace(station.Address["street"]), strings.TrimSpace(station.Address["number"]),
			strings.TrimSpace(station.Address["district"]), strings.TrimSpace(station.Address["postal_code"]),
			fmt.Sprint(fuelCount[station.StationID]),
		})
	}
	stationCSV, stationRowsCount, err := writeCSV([]string{
		"station_id", "display_name", "cnpj", "municipality_code", "municipality_name",
		"state", "operation_status", "location_quality", "address_street", "address_number",
		"address_district", "address_postal_code", "latest_fuel_count",
	}, stationRows)
	if err != nil {
		return nil, downloadFile{}, err
	}

	surveyStart, surveyEnd, sourceURL, sourceChecksum := "", "", "", ""
	if latest != nil {
		surveyStart, surveyEnd = latest.SurveyStart, latest.SurveyEnd
		sourceURL, sourceChecksum = latest.SourceURL, latest.SourceChecksum
	}
	priceRows := make([][]string, 0, len(observations))
	for _, item := range observations {
		priceRows = append(priceRows, []string{
			item.station.StationID, item.station.DisplayName, item.station.CNPJ,
			item.station.State, item.station.MunicipalityCode,
			item.price.FuelProduct, item.price.Unit, "STANDARD",
			fmt.Sprint(item.price.AmountMilliBRL), brl(item.price.AmountMilliBRL),
			item.price.CollectedOn, surveyStart, surveyEnd,
			item.price.RevisionID, sourceURL, sourceChecksum,
		})
	}
	priceCSV, priceRowsCount, err := writeCSV([]string{
		"station_id", "display_name", "cnpj", "state", "municipality_code",
		"fuel", "unit", "condition", "amount_milli_brl", "amount_brl",
		"collected_on", "survey_start", "survey_end", "revision_id", "source_url", "source_checksum",
	}, priceRows)
	if err != nil {
		return nil, downloadFile{}, err
	}

	sortedSummaries := append([]analytics.Summary(nil), summaries...)
	sort.Slice(sortedSummaries, func(i, j int) bool {
		if sortedSummaries[i].Geography.Level != sortedSummaries[j].Geography.Level {
			return summaryLevelOrder(sortedSummaries[i].Geography.Level) < summaryLevelOrder(sortedSummaries[j].Geography.Level)
		}
		if sortedSummaries[i].Geography.Code != sortedSummaries[j].Geography.Code {
			return sortedSummaries[i].Geography.Code < sortedSummaries[j].Geography.Code
		}
		if sortedSummaries[i].Fuel != sortedSummaries[j].Fuel {
			return sortedSummaries[i].Fuel < sortedSummaries[j].Fuel
		}
		return sortedSummaries[i].Unit < sortedSummaries[j].Unit
	})
	summaryRows := make([][]string, 0, len(sortedSummaries))
	for _, summary := range sortedSummaries {
		summaryRows = append(summaryRows, []string{
			summary.RevisionID, summary.SurveyStart, summary.SurveyEnd,
			summary.Geography.Level, summary.Geography.Code,
			summary.Fuel, summary.Unit, summary.Condition,
			fmt.Sprint(summary.StationCount), fmt.Sprint(summary.SourceObservations),
			fmt.Sprint(summary.MinMilliBRL), fmt.Sprint(summary.MaxMilliBRL),
			fmt.Sprint(summary.SpreadMilliBRL), summary.Method,
		})
	}
	summaryCSV, summaryRowsCount, err := writeCSV([]string{
		"revision_id", "survey_start", "survey_end", "geography_level", "geography_code",
		"fuel", "unit", "condition", "station_count", "source_observations",
		"min_milli_brl", "max_milli_brl", "spread_milli_brl", "method",
	}, summaryRows)
	if err != nil {
		return nil, downloadFile{}, err
	}

	if err := g.writeAsset("dados/"+release+"/estacoes.csv", stationCSV); err != nil {
		return nil, downloadFile{}, err
	}
	if err := g.writeAsset("dados/"+release+"/precos.csv", priceCSV); err != nil {
		return nil, downloadFile{}, err
	}
	if err := g.writeAsset("dados/"+release+"/resumos.csv", summaryCSV); err != nil {
		return nil, downloadFile{}, err
	}
	files := []downloadFile{
		{Label: "Postos (CSV)", Href: "/dados/" + release + "/estacoes.csv", Rows: stationRowsCount, Bytes: g.files["dados/"+release+"/estacoes.csv"].Bytes},
		{Label: "Preços da pesquisa mais recente (CSV)", Href: "/dados/" + release + "/precos.csv", Rows: priceRowsCount, Bytes: g.files["dados/"+release+"/precos.csv"].Bytes},
		{Label: "Resumos por região (CSV)", Href: "/dados/" + release + "/resumos.csv", Rows: summaryRowsCount, Bytes: g.files["dados/"+release+"/resumos.csv"].Bytes},
	}
	manifest := downloadManifest{
		Schema: DownloadsSchema, InputContentSHA256: inputHash, Release: release,
		Period: period, PublishedAt: published,
		Attribution: "Fonte: ANP — Levantamento de Preços de Combustíveis. Cálculo e exibição: Abastevo. Não afiliado à ANP.",
		Notes:       downloadNotes,
		Files: []downloadEntry{
			{Name: "estacoes.csv", SHA256: g.files["dados/"+release+"/estacoes.csv"].SHA256, Bytes: g.files["dados/"+release+"/estacoes.csv"].Bytes, Rows: stationRowsCount},
			{Name: "precos.csv", SHA256: g.files["dados/"+release+"/precos.csv"].SHA256, Bytes: g.files["dados/"+release+"/precos.csv"].Bytes, Rows: priceRowsCount},
			{Name: "resumos.csv", SHA256: g.files["dados/"+release+"/resumos.csv"].SHA256, Bytes: g.files["dados/"+release+"/resumos.csv"].Bytes, Rows: summaryRowsCount},
		},
	}
	raw, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, downloadFile{}, err
	}
	raw = append(raw, '\n')
	if err := g.writeAsset("dados/"+release+"/manifest.json", raw); err != nil {
		return nil, downloadFile{}, err
	}
	manifestLink := downloadFile{
		Label: "Manifesto dos downloads (JSON)",
		Href:  "/dados/" + release + "/manifest.json",
		Rows:  0,
		Bytes: g.files["dados/"+release+"/manifest.json"].Bytes,
	}
	return files, manifestLink, nil
}
