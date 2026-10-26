package portalgen

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/enterpriseexport"
)

// Search index v2: static, release-addressed files that let the browser
// search every station nationally by name/CNPJ, by city or state, list a
// city's stations, rank one fuel's prices and search official records,
// without a national all-stations download and without calling an API.
const (
	SearchIndexSchema  = "abastevo-search-index-v1"
	SearchPrefixSchema = "abastevo-search-prefix-v1"
	SearchPricesSchema = "abastevo-search-prices-v1"
	SearchFiscalSchema = "abastevo-search-fiscal-v1"
	searchPrefixLength = 3
)

// searchStopwords never select a prefix shard: they occur in most names
// ("POSTO", "LTDA"), so a query must also carry a distinctive term. The list
// travels in the index file so the browser uses exactly the same set.
var searchStopwords = []string{
	"abastecimento", "auto", "autoposto", "cia", "combustivel", "combustiveis",
	"comercial", "comercio", "da", "das", "de", "derivados", "do", "dos",
	"eireli", "epp", "lubrificantes", "ltda", "me", "petroleo", "posto",
	"postos", "rede", "sa", "servico", "servicos",
}

type searchIndex struct {
	Schema         string               `json:"schema"`
	Release        string               `json:"release"`
	PrefixLength   int                  `json:"prefix_length"`
	Stopwords      []string             `json:"stopwords"`
	Fiscal         bool                 `json:"fiscal"`
	Fuels          []searchFuel         `json:"fuels"`
	Municipalities []searchMunicipality `json:"municipalities"`
}

type searchFuel struct {
	Code  string `json:"code"`
	Label string `json:"label"`
	Unit  string `json:"unit"`
	Count int    `json:"count"`
}

type searchMunicipality struct {
	Code       string `json:"m"`
	Name       string `json:"n"`
	SearchName string `json:"s"`
	UF         string `json:"u"`
	Stations   int    `json:"k"`
}

type searchPrefixShard struct {
	Schema   string          `json:"schema"`
	Release  string          `json:"release"`
	Key      string          `json:"key"`
	Count    int             `json:"count"`
	Stations []searchStation `json:"stations"`
}

type searchPriceShard struct {
	Schema  string           `json:"schema"`
	Release string           `json:"release"`
	Fuel    string           `json:"fuel"`
	Label   string           `json:"label"`
	Unit    string           `json:"unit"`
	Count   int              `json:"count"`
	Rows    []searchPriceRow `json:"rows"`
}

type searchPriceRow struct {
	ID               string `json:"i"`
	Name             string `json:"n"`
	CNPJ             string `json:"c,omitempty"`
	Municipality     string `json:"m,omitempty"`
	MunicipalityName string `json:"mn,omitempty"`
	UF               string `json:"u,omitempty"`
	Milli            int64  `json:"v"`
	CollectedOn      string `json:"d,omitempty"`
}

type searchFiscalShard struct {
	Schema  string            `json:"schema"`
	Release string            `json:"release"`
	Count   int               `json:"count"`
	Records []searchFiscalRow `json:"records"`
}

type searchFiscalRow struct {
	RecordID         string `json:"r"`
	CNPJ             string `json:"c"`
	Status           string `json:"st"`
	Amount           string `json:"a,omitempty"`
	Date             string `json:"d,omitempty"`
	StationID        string `json:"i,omitempty"`
	StationName      string `json:"n,omitempty"`
	Municipality     string `json:"m,omitempty"`
	MunicipalityName string `json:"mn,omitempty"`
	UF               string `json:"u,omitempty"`
}

// indexFile is one release-relative search file under dados/busca/<release>/.
type indexFile struct {
	Path string
	Data []byte
}

//go:embed assets/ibge-municipalities.json
var ibgeMunicipalitiesRaw []byte

var (
	ibgeOnce  sync.Once
	ibgeNames map[string]struct{ N, U string }
)

// municipalityLabel names an IBGE municipality code from the embedded IBGE
// table (derived from data/src/main/assets/ibge_municipalities.json),
// falling back to the source address text; unknown codes stay empty.
func municipalityLabel(code, addressName string) string {
	ibgeOnce.Do(func() {
		raw := map[string]struct {
			N string `json:"n"`
			U string `json:"u"`
		}{}
		if err := json.Unmarshal(ibgeMunicipalitiesRaw, &raw); err == nil {
			ibgeNames = make(map[string]struct{ N, U string }, len(raw))
			for k, v := range raw {
				ibgeNames[k] = struct{ N, U string }{v.N, v.U}
			}
		}
	})
	if entry, ok := ibgeNames[code]; ok && entry.N != "" {
		return titlePlace(entry.N)
	}
	return titlePlace(strings.TrimSpace(addressName))
}

var placeParticles = map[string]bool{"de": true, "da": true, "do": true, "das": true, "dos": true, "e": true, "d'": true}

// titlePlace renders an upper-case IBGE name as "São José dos Campos",
// keeping Portuguese particles lower-case after the first word.
func titlePlace(value string) string {
	words := strings.Fields(strings.ToLower(value))
	for i, word := range words {
		if i > 0 && placeParticles[word] {
			continue
		}
		words[i] = titleWord(word)
	}
	return strings.Join(words, " ")
}

func titleWord(word string) string {
	if strings.HasPrefix(word, "d'") && len(word) > 2 {
		return "d'" + titleWord(word[2:])
	}
	parts := strings.Split(word, "-")
	for i, part := range parts {
		if part == "" || (i > 0 && placeParticles[part]) {
			continue
		}
		r, size := utf8.DecodeRuneInString(part)
		parts[i] = string(unicode.ToUpper(r)) + part[size:]
	}
	return strings.Join(parts, "-")
}

// searchTokens splits a normalized name into ASCII-alphanumeric terms.
func searchTokens(normalized string) []string {
	return strings.FieldsFunc(normalized, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9')
	})
}

func prefixKeys(station enterpriseexport.Station) []string {
	stop := map[string]bool{}
	for _, word := range searchStopwords {
		stop[word] = true
	}
	keys := map[string]bool{}
	for _, token := range searchTokens(normalizeSearchName(station.DisplayName)) {
		if len(token) < searchPrefixLength || stop[token] || strings.Trim(token, "0123456789") == "" {
			continue
		}
		keys["n-"+token[:searchPrefixLength]] = true
	}
	if cnpj := normalizeCNPJValue(station.CNPJ); len(cnpj) >= searchPrefixLength {
		keys["c-"+strings.ToLower(cnpj[:searchPrefixLength])] = true
	}
	out := make([]string, 0, len(keys))
	for key := range keys {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

func searchRow(station enterpriseexport.Station, prices map[string]int64, withPrices bool) searchStation {
	row := searchStation{
		ID: station.StationID, Name: station.DisplayName,
		SearchName:       normalizeSearchName(station.DisplayName),
		CNPJ:             normalizeCNPJValue(station.CNPJ),
		Municipality:     station.MunicipalityCode,
		MunicipalityName: municipalityLabel(station.MunicipalityCode, station.Address["municipio"]),
		UF:               station.State,
		Status:           station.OperationStatus,
		Fuels:            len(prices),
	}
	if withPrices && len(prices) > 0 {
		row.Prices = prices
	}
	return row
}

func sortStations(stations []enterpriseexport.Station) {
	sort.Slice(stations, func(i, j int) bool {
		left, right := normalizeSearchName(stations[i].DisplayName), normalizeSearchName(stations[j].DisplayName)
		if left != right {
			return left < right
		}
		return stations[i].StationID < stations[j].StationID
	})
}

func marshalIndexFile(path string, value any) (indexFile, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return indexFile{}, err
	}
	return indexFile{Path: path, Data: append(data, '\n')}, nil
}

// buildSearchIndexes emits the v2 search files for one release. Fiscal rows
// are included only when official records exist and stations may be linked.
func buildSearchIndexes(snapshot enterpriseexport.Snapshot, observations []observation, fiscal []searchFiscalRow, release string) ([]indexFile, searchIndex, error) {
	prices := map[string]map[string]int64{}
	for _, item := range observations {
		if prices[item.station.StationID] == nil {
			prices[item.station.StationID] = map[string]int64{}
		}
		prices[item.station.StationID][item.price.FuelProduct] = item.price.AmountMilliBRL
	}
	stations := append([]enterpriseexport.Station(nil), snapshot.Stations...)
	sortStations(stations)

	prefixes := map[string][]searchStation{}
	cities := map[string][]searchStation{}
	cityUF := map[string]string{}
	cityName := map[string]string{}
	for _, station := range stations {
		row := searchRow(station, prices[station.StationID], false)
		for _, key := range prefixKeys(station) {
			prefixes[key] = append(prefixes[key], row)
		}
		if station.MunicipalityCode != "" && isUF(station.State) {
			code := station.MunicipalityCode
			if existing := cityUF[code]; existing != "" && existing != station.State {
				return nil, searchIndex{}, fmt.Errorf("portalgen: municipality code spans states")
			}
			cityUF[code] = station.State
			cityName[code] = row.MunicipalityName
			cities[code] = append(cities[code], searchRow(station, prices[station.StationID], true))
		}
	}

	files := make([]indexFile, 0, len(prefixes)+len(cities)+16)
	keys := make([]string, 0, len(prefixes))
	for key := range prefixes {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		file, err := marshalIndexFile("p/"+key+".json", searchPrefixShard{
			Schema: SearchPrefixSchema, Release: release, Key: key,
			Count: len(prefixes[key]), Stations: prefixes[key],
		})
		if err != nil {
			return nil, searchIndex{}, err
		}
		files = append(files, file)
	}

	municipalities := make([]searchMunicipality, 0, len(cities))
	codes := make([]string, 0, len(cities))
	for code := range cities {
		codes = append(codes, code)
	}
	sort.Strings(codes)
	for _, code := range codes {
		name := cityName[code]
		if name == "" {
			name = "Município " + code
		}
		municipalities = append(municipalities, searchMunicipality{
			Code: code, Name: name, SearchName: normalizeSearchName(name),
			UF: cityUF[code], Stations: len(cities[code]),
		})
		file, err := marshalIndexFile("m/"+code+".json", searchShard{
			Schema: SearchSchema, Release: release, UF: cityUF[code], Municipality: code,
			Count: len(cities[code]), Stations: cities[code],
		})
		if err != nil {
			return nil, searchIndex{}, err
		}
		files = append(files, file)
	}

	byFuel := map[string][]searchPriceRow{}
	units := map[string]string{}
	for _, item := range observations {
		station := item.station
		byFuel[item.price.FuelProduct] = append(byFuel[item.price.FuelProduct], searchPriceRow{
			ID: station.StationID, Name: station.DisplayName, CNPJ: normalizeCNPJValue(station.CNPJ),
			Municipality: station.MunicipalityCode, UF: station.State,
			MunicipalityName: municipalityLabel(station.MunicipalityCode, station.Address["municipio"]),
			Milli:            item.price.AmountMilliBRL, CollectedOn: item.price.CollectedOn,
		})
		units[item.price.FuelProduct] = item.price.Unit
	}
	fuels := make([]searchFuel, 0, len(byFuel))
	fuelCodes := make([]string, 0, len(byFuel))
	for code := range byFuel {
		fuelCodes = append(fuelCodes, code)
	}
	sort.Slice(fuelCodes, func(i, j int) bool {
		left, right := fuelRank(fuelCodes[i]), fuelRank(fuelCodes[j])
		if left != right {
			return left < right
		}
		return fuelCodes[i] < fuelCodes[j]
	})
	for _, code := range fuelCodes {
		rows := byFuel[code]
		sort.Slice(rows, func(i, j int) bool {
			if rows[i].Milli != rows[j].Milli {
				return rows[i].Milli < rows[j].Milli
			}
			return rows[i].ID < rows[j].ID
		})
		fuels = append(fuels, searchFuel{Code: code, Label: fuelLabel(code), Unit: units[code], Count: len(rows)})
		file, err := marshalIndexFile("precos/"+code+".json", searchPriceShard{
			Schema: SearchPricesSchema, Release: release, Fuel: code, Label: fuelLabel(code),
			Unit: units[code], Count: len(rows), Rows: rows,
		})
		if err != nil {
			return nil, searchIndex{}, err
		}
		files = append(files, file)
	}

	if len(fiscal) > 0 {
		file, err := marshalIndexFile("fiscalizacao.json", searchFiscalShard{
			Schema: SearchFiscalSchema, Release: release, Count: len(fiscal), Records: fiscal,
		})
		if err != nil {
			return nil, searchIndex{}, err
		}
		files = append(files, file)
	}

	index := searchIndex{
		Schema: SearchIndexSchema, Release: release, PrefixLength: searchPrefixLength,
		Stopwords: searchStopwords, Fiscal: len(fiscal) > 0, Fuels: fuels,
		Municipalities: municipalities,
	}
	file, err := marshalIndexFile("indice.json", index)
	if err != nil {
		return nil, searchIndex{}, err
	}
	return append(files, file), index, nil
}

// fuelOrder lists fuels the way drivers look for them; the first one is the
// default of the price explorer. Unknown codes sort after, alphabetically.
var fuelOrder = []string{"GASOLINE_REGULAR", "ETHANOL", "GASOLINE_ADDITIVED", "DIESEL_S10", "DIESEL_S500", "CNG", "LPG_P13"}

func fuelRank(code string) int {
	for i, known := range fuelOrder {
		if known == code {
			return i
		}
	}
	return len(fuelOrder)
}

// fiscalSearchRows projects official record views for the fiscal search,
// carrying station name and place only for safely linked records.
func fiscalSearchRows(official *officialDataset, stations map[string]enterpriseexport.Station, linkStations bool) []searchFiscalRow {
	if official == nil {
		return nil
	}
	rows := make([]searchFiscalRow, 0, len(official.Records))
	for _, record := range official.Records {
		view := officialView(record, stations, linkStations)
		row := searchFiscalRow{
			RecordID: view.RecordID, CNPJ: normalizeCNPJValue(record.SubjectCNPJ),
			Status: view.Status, Amount: view.Amount, Date: valueOr(view.DecidedOn, view.AsOf),
		}
		if view.StationHref != "" {
			if station, ok := stations[record.StationID]; ok {
				row.StationID = station.StationID
				row.StationName = station.DisplayName
				row.Municipality = station.MunicipalityCode
				row.MunicipalityName = municipalityLabel(station.MunicipalityCode, station.Address["municipio"])
				row.UF = station.State
			}
		}
		rows = append(rows, row)
	}
	return rows
}
