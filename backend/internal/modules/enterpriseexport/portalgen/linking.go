package portalgen

import (
	"math"
	"sort"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/enterpriseexport"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/enterpriseexport/analytics"
)

const (
	nearbyCities        = 8
	otherCitiesInState  = 24
	siblingStations     = 10
	cityStationListCap  = 400
	noDataStationsCap   = 200
	brazilLatMin        = -34.0
	brazilLatMax        = 6.0
	brazilLonMin        = -74.5
	brazilLonMax        = -28.5
	earthRadiusKm       = 6371.0
	maxStateCityListing = 1000
)

// linkModule is one titled list of contextual body links (B-BR-SEO-07).
type linkModule struct {
	Title string
	Links []moduleLink
}

type moduleLink struct {
	Label string
	Href  string
	Note  string
}

// cityInfo is one city page: a city with current-survey data or a city
// with registry stations only (honest no-data page).
type cityInfo struct {
	Code        string
	UF          string
	Name        string
	Route       string
	HasData     bool
	Stations    []*stationEntry
	Lat, Lon    float64
	HasCentroid bool
}

// prepareStationIndex computes every station's name, gate and head once,
// before any page is written, so city pages can list stations and station
// pages can link siblings.
func (g *generator) prepareStationIndex(snapshot enterpriseexport.Snapshot, stations map[string]enterpriseexport.Station, revisions map[string]enterpriseexport.Revision, observations map[string][]observation) {
	ordered := make([]enterpriseexport.Station, 0, len(stations))
	for _, station := range stations {
		ordered = append(ordered, station)
	}
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].DisplayName != ordered[j].DisplayName {
			return ordered[i].DisplayName < ordered[j].DisplayName
		}
		return ordered[i].StationID < ordered[j].StationID
	})
	g.pricesByStation = map[string][]enterpriseexport.Price{}
	for _, price := range snapshot.Prices {
		g.pricesByStation[price.StationID] = append(g.pricesByStation[price.StationID], price)
	}
	recent := recentSurveyRevisions(revisions, stationRecentSurveyWeeks)
	g.entries = g.prepareStations(ordered, g.pricesByStation, revisions, recent, observations)
}

// prepareGeography builds the city pages (data cities from the current
// survey plus registry cities), their readable routes, station lists and
// coordinate centroids.
func (g *generator) prepareGeography(summaries []analytics.Summary, municipalityState, municipalityName map[string]string) {
	g.cities = map[string]*cityInfo{}
	for _, summary := range summaries {
		code := summary.Geography.Code
		if summary.Geography.Level != analytics.LevelMunicipality || !isIBGE(code) || !isUF(municipalityState[code]) {
			continue
		}
		name := municipalityName[code]
		if name == "" {
			name = "Município " + code
		}
		g.cities[code] = &cityInfo{Code: code, UF: municipalityState[code], Name: name, HasData: true}
	}
	conflicts := map[string]bool{}
	for i := range g.entries {
		entry := &g.entries[i]
		code, uf := entry.station.MunicipalityCode, entry.station.State
		if !isIBGE(code) || !knownUF(uf) {
			continue
		}
		city, ok := g.cities[code]
		if !ok {
			name := entry.city
			if name == "" {
				name = "Município " + code
			}
			city = &cityInfo{Code: code, UF: uf, Name: name}
			g.cities[code] = city
		}
		if city.UF != uf {
			if !city.HasData {
				conflicts[code] = true
			}
			continue
		}
		city.Stations = append(city.Stations, entry)
	}
	for code := range conflicts {
		delete(g.cities, code)
	}
	keys := make([]cityKey, 0, len(g.cities))
	for _, code := range sortedKeys(g.cities) {
		city := g.cities[code]
		keys = append(keys, cityKey{Code: code, UF: city.UF, Name: city.Name})
		lat, lon, n := 0.0, 0.0, 0
		for _, entry := range city.Stations {
			if point := entry.station.Coordinates; point != nil && point.Lat >= brazilLatMin && point.Lat <= brazilLatMax && point.Lon >= brazilLonMin && point.Lon <= brazilLonMax {
				lat, lon, n = lat+point.Lat, lon+point.Lon, n+1
			}
		}
		if n > 0 {
			city.Lat, city.Lon, city.HasCentroid = lat/float64(n), lon/float64(n), true
		}
	}
	g.cityRoutes = assignCityRoutes(keys)
	for code, city := range g.cities {
		city.Route = g.cityRoutes[code]
	}
}

// dataCities lists cities with current-survey data, optionally in one UF,
// ordered by name then code.
func (g *generator) dataCities(uf string) []*cityInfo {
	out := []*cityInfo{}
	for _, city := range g.cities {
		if city.HasData && (uf == "" || city.UF == uf) {
			out = append(out, city)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].Code < out[j].Code
	})
	return out
}

func haversineKm(lat1, lon1, lat2, lon2 float64) float64 {
	rad := math.Pi / 180
	dLat, dLon := (lat2-lat1)*rad, (lon2-lon1)*rad
	a := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(lat1*rad)*math.Cos(lat2*rad)*math.Sin(dLon/2)*math.Sin(dLon/2)
	return 2 * earthRadiusKm * math.Asin(math.Min(1, math.Sqrt(a)))
}

// nearestDataCities returns up to k data cities closest to city's centroid,
// ties broken by code; none when the city has no station coordinates.
func (g *generator) nearestDataCities(city *cityInfo, k int) []*cityInfo {
	if !city.HasCentroid {
		return nil
	}
	type candidate struct {
		city     *cityInfo
		distance float64
	}
	candidates := []candidate{}
	for _, other := range g.cities {
		if other.Code == city.Code || !other.HasData || !other.HasCentroid {
			continue
		}
		candidates = append(candidates, candidate{other, haversineKm(city.Lat, city.Lon, other.Lat, other.Lon)})
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].distance != candidates[j].distance {
			return candidates[i].distance < candidates[j].distance
		}
		return candidates[i].city.Code < candidates[j].city.Code
	})
	out := make([]*cityInfo, 0, k)
	for _, item := range candidates {
		if len(out) == k {
			break
		}
		out = append(out, item.city)
	}
	return out
}

// ringAfter returns up to n items following position i in a cyclic order,
// never i itself; it spreads inbound links evenly across a list.
func ringAfter[T any](items []T, i, n int) []T {
	out := []T{}
	for step := 1; step < len(items) && len(out) < n; step++ {
		out = append(out, items[(i+step)%len(items)])
	}
	return out
}

func cityLinks(cities []*cityInfo) []moduleLink {
	links := make([]moduleLink, 0, len(cities))
	for _, city := range cities {
		links = append(links, moduleLink{Label: "Preços em " + placeLabel(city.Name, city.UF), Href: city.Route})
	}
	return links
}

// cityModules links nearby cities, other cities of the UF and the state.
func (g *generator) cityModules(city *cityInfo) []linkModule {
	modules := []linkModule{}
	if nearby := g.nearestDataCities(city, nearbyCities); len(nearby) > 0 {
		modules = append(modules, linkModule{Title: "Cidades próximas", Links: cityLinks(nearby)})
	}
	peers := g.dataCities(city.UF)
	position := -1
	for i, peer := range peers {
		if peer.Code == city.Code {
			position = i
		}
	}
	var others []*cityInfo
	if position >= 0 {
		others = ringAfter(peers, position, otherCitiesInState)
	} else {
		others = peers[:min(len(peers), otherCitiesInState)]
	}
	links := cityLinks(others)
	if g.statePages[city.UF] {
		links = append([]moduleLink{{Label: "Preço dos combustíveis em " + ufName(city.UF), Href: stateRoute(city.UF)}}, links...)
	}
	modules = append(modules, linkModule{Title: "Outras cidades de " + ufName(city.UF), Links: links})
	return modules
}

// fuelModule links the hubs of the given products.
func (g *generator) fuelModule(title string, codes []string) linkModule {
	seen := map[string]bool{}
	links := []moduleLink{}
	for _, hub := range g.orderedFuelHubs() {
		for _, code := range codes {
			if code == hub.Code && !seen[code] {
				seen[code] = true
				words := fuelVocabulary[code]
				links = append(links, moduleLink{Label: "Preço " + words.Article + " " + words.Name + " por estado", Href: hub.Href})
			}
		}
	}
	return linkModule{Title: title, Links: links}
}

// landingModule links related landing guides and tools; empty when the
// build has no landing.
func (g *generator) landingModule(links ...moduleLink) linkModule {
	if g.opts.LandingURL == "" {
		return linkModule{}
	}
	for i := range links {
		links[i].Href = g.landingPrefix + links[i].Href
	}
	return linkModule{Title: "Guias e ferramentas", Links: links}
}

func hasFuelFamilies(codes []string, families ...string) bool {
	present := map[string]bool{}
	for _, family := range fuelFamilies(codes) {
		present[family] = true
	}
	for _, family := range families {
		if !present[family] {
			return false
		}
	}
	return true
}

// priceGuides picks contextual landing links for a price page.
func (g *generator) priceGuides(place string, codes []string) linkModule {
	links := []moduleLink{}
	if hasFuelFamilies(codes, "gasolina", "etanol") {
		links = append(links,
			moduleLink{Label: "Etanol ou gasolina" + place + ": calcule qual compensa", Href: "/ferramentas/etanol-ou-gasolina/"},
			moduleLink{Label: "Guia: quando o etanol compensa mais que a gasolina", Href: "/guias/etanol-ou-gasolina/"})
	}
	links = append(links,
		moduleLink{Label: "Como ler os preços da pesquisa ANP", Href: "/guias/como-ler-precos/"},
		moduleLink{Label: "Como funciona a pesquisa semanal da ANP", Href: "/guias/pesquisa-anp/"},
		moduleLink{Label: "Quanto custa encher o tanque", Href: "/ferramentas/encher-o-tanque/"},
		moduleLink{Label: "Calcule o custo de uma viagem de carro", Href: "/ferramentas/custo-de-viagem/"})
	return g.landingModule(links...)
}

// stationGuides picks landing links for a station page.
func stationGuides(fuels []string) []moduleLink {
	links := []moduleLink{{Label: "Como ler os preços da pesquisa ANP", Href: "/guias/como-ler-precos/"}}
	if hasFuelFamilies(fuels, "gasolina", "etanol") {
		links = append(links, moduleLink{Label: "Etanol ou gasolina: calcule qual compensa", Href: "/ferramentas/etanol-ou-gasolina/"})
	}
	return append(links, moduleLink{Label: "Calcule o custo por quilômetro rodado", Href: "/ferramentas/custo-por-km/"})
}

// hubModule links the other portal hubs from a hub page body.
func (g *generator) hubModule(self string) linkModule {
	hubs := []moduleLink{
		{Label: "Preços de combustíveis por estado e cidade", Href: "/precos/"},
		{Label: "Consultar preços por posto ou CNPJ", Href: "/consultar/"},
		{Label: "Perguntas aprovadas sobre os dados", Href: "/curiosidades/"},
		{Label: "Baixar dados públicos em CSV", Href: "/dados/"},
		{Label: "Fiscalização: multas aplicadas pela ANP", Href: "/fiscalizacao/"},
		{Label: "Metodologia e fontes", Href: "/metodologia/"},
	}
	links := []moduleLink{}
	for _, hub := range hubs {
		if hub.Href == self || (g.opts.OfficialOnly && hub.Href != "/fiscalizacao/" && hub.Href != "/metodologia/") {
			continue
		}
		links = append(links, hub)
	}
	return linkModule{Title: "Mais dados do abastevo", Links: links}
}

// stationRows lists a city's stations for its page: every indexable station
// plus every station observed in the current survey (data cities), or the
// registry stations (no-data cities), bounded.
func stationRows(city *cityInfo, observed map[string][]observation, limit int) ([]moduleLink, int) {
	rows := []moduleLink{}
	total := 0
	for _, entry := range city.Stations {
		if city.HasData && !entry.indexable && len(observed[entry.station.StationID]) == 0 {
			continue
		}
		total++
		if len(rows) == limit {
			continue
		}
		rows = append(rows, moduleLink{Label: entry.name, Href: "/postos/" + entry.station.StationID + "/", Note: stationDisplayTitle(stationStreet(entry.station))})
	}
	return rows, total
}

// siblingStations links up to siblingStations other stations of the city:
// a ring over its indexable stations (all registry stations when none is
// indexable), so each indexable station gets inbound links from its peers.
func siblingLinks(city *cityInfo, self *stationEntry) []moduleLink {
	pool := []*stationEntry{}
	for _, entry := range city.Stations {
		if entry.indexable {
			pool = append(pool, entry)
		}
	}
	if len(pool) == 0 || (len(pool) == 1 && pool[0] == self) {
		pool = city.Stations
	}
	position := sort.Search(len(pool), func(i int) bool { return !stationBefore(pool[i], self) })
	var picked []*stationEntry
	if position < len(pool) && pool[position] == self {
		picked = ringAfter(pool, position, siblingStations)
	} else {
		picked = ringAfter(append([]*stationEntry{self}, rotate(pool, position)...), 0, siblingStations)
	}
	links := make([]moduleLink, 0, len(picked))
	for _, entry := range picked {
		if entry == self {
			continue
		}
		links = append(links, moduleLink{Label: entry.name, Href: "/postos/" + entry.station.StationID + "/", Note: stationDisplayTitle(stationStreet(entry.station))})
	}
	return links
}

func stationBefore(a, b *stationEntry) bool {
	if a.station.DisplayName != b.station.DisplayName {
		return a.station.DisplayName < b.station.DisplayName
	}
	return a.station.StationID < b.station.StationID
}

func rotate[T any](items []T, i int) []T {
	if len(items) == 0 {
		return nil
	}
	i %= len(items)
	return append(append([]T(nil), items[i:]...), items[:i]...)
}
