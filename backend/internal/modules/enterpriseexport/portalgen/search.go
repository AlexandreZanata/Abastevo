package portalgen

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/enterpriseexport"
)

const SearchSchema = "abastevo-search-v1"

type searchState struct {
	Code string
	Name string
}

type searchShard struct {
	Schema       string          `json:"schema"`
	Release      string          `json:"release"`
	UF           string          `json:"uf"`
	Municipality string          `json:"municipio,omitempty"`
	Count        int             `json:"count"`
	Stations     []searchStation `json:"stations"`
}

type searchStation struct {
	ID               string           `json:"i"`
	Name             string           `json:"n"`
	SearchName       string           `json:"s"`
	CNPJ             string           `json:"c,omitempty"`
	Municipality     string           `json:"m,omitempty"`
	MunicipalityName string           `json:"mn,omitempty"`
	UF               string           `json:"u,omitempty"`
	Status           string           `json:"st,omitempty"`
	StatusLabel      string           `json:"sl,omitempty"`
	Prices           map[string]int64 `json:"p,omitempty"`
	Fuels            int              `json:"f,omitempty"`
}

type shardFile struct {
	UF   string
	Data []byte
}

var searchFold = strings.NewReplacer(
	"á", "a", "à", "a", "â", "a", "ã", "a", "ä", "a",
	"é", "e", "è", "e", "ê", "e", "ë", "e",
	"í", "i", "ì", "i", "î", "i", "ï", "i",
	"ó", "o", "ò", "o", "ô", "o", "õ", "o", "ö", "o",
	"ú", "u", "ù", "u", "û", "u", "ü", "u",
	"ç", "c", "ñ", "n",
)

func normalizeSearchName(value string) string {
	return strings.Join(strings.Fields(searchFold.Replace(strings.ToLower(value))), " ")
}

func normalizeCNPJValue(value string) string {
	var out strings.Builder
	for _, r := range strings.ToUpper(value) {
		if r >= '0' && r <= '9' || r >= 'A' && r <= 'Z' {
			out.WriteRune(r)
		}
	}
	return out.String()
}

// buildSearchShards creates one bounded locator per state plus one for stations
// without a state. It never creates a national all-stations shard.
func buildSearchShards(snapshot enterpriseexport.Snapshot, observations []observation, release string) ([]shardFile, error) {
	prices := map[string]map[string]int64{}
	for _, item := range observations {
		if prices[item.station.StationID] == nil {
			prices[item.station.StationID] = map[string]int64{}
		}
		prices[item.station.StationID][item.price.FuelProduct] = item.price.AmountMilliBRL
	}
	groups := map[string][]enterpriseexport.Station{}
	for _, station := range snapshot.Stations {
		uf := station.State
		if !isUF(uf) {
			uf = "sem-uf"
		}
		groups[uf] = append(groups[uf], station)
	}
	codes := make([]string, 0, len(groups))
	for code := range groups {
		codes = append(codes, code)
	}
	sort.Strings(codes)
	out := make([]shardFile, 0, len(codes))
	for _, uf := range codes {
		stations := groups[uf]
		sort.Slice(stations, func(i, j int) bool {
			left := normalizeSearchName(stations[i].DisplayName)
			right := normalizeSearchName(stations[j].DisplayName)
			if left != right {
				return left < right
			}
			return stations[i].StationID < stations[j].StationID
		})
		rows := make([]searchStation, 0, len(stations))
		for _, station := range stations {
			row := searchStation{
				ID: station.StationID, Name: station.DisplayName,
				SearchName:       normalizeSearchName(station.DisplayName),
				CNPJ:             normalizeCNPJValue(station.CNPJ),
				Municipality:     station.MunicipalityCode,
				MunicipalityName: municipalityLabel(station.MunicipalityCode, station.Address["municipio"]),
				UF:               station.State,
				Status:           station.OperationStatus,
				StatusLabel:      statusLabel(station.OperationStatus),
				Prices:           prices[station.StationID],
			}
			if row.Prices != nil && len(row.Prices) == 0 {
				row.Prices = nil
			}
			rows = append(rows, row)
		}
		shard := searchShard{
			Schema: SearchSchema, Release: release, UF: uf,
			Count: len(rows), Stations: rows,
		}
		data, err := json.Marshal(shard)
		if err != nil {
			return nil, err
		}
		data = append(data, '\n')
		out = append(out, shardFile{UF: uf, Data: data})
	}
	return out, nil
}

func releaseSlug(contentHash string) string {
	raw := strings.TrimPrefix(contentHash, "sha256:")
	var out strings.Builder
	for _, r := range raw {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-' {
			if out.Len() >= 64 {
				break
			}
			out.WriteRune(r)
		}
	}
	if out.Len() == 0 {
		return "release"
	}
	return out.String()
}

func searchStateName(code string) string {
	if code == "sem-uf" {
		return "Sem UF"
	}
	return ufName(code)
}

func validateSearchShard(shard shardFile) error {
	var decoded searchShard
	if err := json.Unmarshal(shard.Data, &decoded); err != nil {
		return err
	}
	if decoded.Schema != SearchSchema || decoded.UF != shard.UF || decoded.Count != len(decoded.Stations) {
		return fmt.Errorf("portalgen: invalid search shard")
	}
	return nil
}
