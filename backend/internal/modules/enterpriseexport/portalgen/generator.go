package portalgen

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html/template"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/enterpriseexport"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/enterpriseexport/analytics"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
)

//go:embed assets/portal.css
var embeddedAssets embed.FS

const (
	SchemaVersion = "enterprise-portal-v2"
	// defaultMaxPages leaves ~1.9x headroom over the real national portal
	// measured on staging 2026-10-26 (52,578 pages, 48,750 stations).
	defaultMaxPages   = 100_000
	defaultMaxBytes   = int64(2 << 30)
	maxRankingPerPage = 20
	minMunicipality   = 5
)

// BudgetError reports a page, byte or size budget breach. Generation and
// verification are deterministic, so the same snapshot under the same
// budgets always fails the same way; callers may coalesce retries.
type BudgetError struct {
	Msg string
}

func (e *BudgetError) Error() string { return e.Msg }

// BudgetKey identifies the effective generation and verification budgets
// for the given overrides (zero selects the defaults).
func BudgetKey(maxPages int, maxBytes int64) string {
	if maxPages <= 0 {
		maxPages = defaultMaxPages
	}
	if maxBytes <= 0 {
		maxBytes = defaultMaxBytes
	}
	return fmt.Sprintf("pages=%d;bytes=%d;html-gzip=%d;script-gzip=%d;shard-gzip=%d",
		maxPages, maxBytes, DefaultMaxHTMLGzip, DefaultMaxScriptGzip, DefaultMaxShardGzip)
}

type Options struct {
	SnapshotDir      string
	OutputDir        string
	MaxPages         int
	MaxBytes         int64
	Indexable        bool
	SiteURL          string
	SearchScriptPath string
	// OfficialOnly emits only the admitted official-records slice
	// (fiscalizacao pages, metodologia and 404): no price, station,
	// search, insight or download routes. It fails closed when the
	// snapshot carries no official dataset.
	OfficialOnly bool
	// LandingURL is the optional landing origin (scheme and host only). When
	// set, the shared header and footer link back to landing pages with
	// absolute URLs; when empty the artifact stays fully self-contained.
	LandingURL string
}

type Result struct {
	Pages         int
	Files         int
	Bytes         int64
	ContentSHA256 string
}

type FileDigest struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Bytes  int64  `json:"bytes"`
}

type Manifest struct {
	SchemaVersion      string       `json:"schema_version"`
	InputContentSHA256 string       `json:"input_content_sha256"`
	MetricsVersion     string       `json:"metrics_version"`
	PublishedAt        string       `json:"published_at"`
	Pages              int          `json:"pages"`
	Files              []FileDigest `json:"files"`
	OfficialRecords    *officialPin `json:"official_records,omitempty"`
	OfficialOnly       bool         `json:"official_only"`
	ContentSHA256      string       `json:"content_sha256"`
}

type pageBase struct {
	// Title is the short page title (the H1 of some templates); HeadTitle is
	// the <title> and og:title.
	Title       string
	HeadTitle   string
	Description string
	ActiveNav   string
	// Indexable is the per-page decision (global switch and pageIndexable);
	// Robots renders it and only indexable pages enter a sitemap.
	Indexable    bool
	Robots       string
	Canonical    string
	OGImage      string
	OfficialOnly bool
	// HasLanding enables landing links; Landing is their prefix, empty for a
	// landing on the portal's own origin (root-relative links).
	HasLanding  bool
	Landing     string
	Period      string
	PublishedAt string
	InputHash   string
	CSSURL      string
	ScriptURL   string
	Breadcrumbs []crumb
	// Modules are contextual link lists rendered in the page body.
	Modules []linkModule
	Route   string
	Family  pageFamily
	LastMod string
	// JSONLD is the inline structured data graph (B-BR-SEO-06).
	JSONLD template.JS
}

func (b pageBase) page() pageBase { return b }

// routedPage is template data that carries its page base.
type routedPage interface{ page() pageBase }

type crumb struct {
	Label   string
	Href    string
	Current bool
}

type summaryRow struct {
	Code         string
	Fuel         string
	FuelName     string
	FuelHref     string
	Unit         string
	Stations     int
	Observations int
	Min          int64
	Max          int64
	Spread       int64
	Insufficient bool
}

type rankingRow struct {
	Position  int
	Name      string
	LegalName string
	StationID string
	CNPJ      string
	Place     string
	Status    string
	Amount    int64
	Collected string
	TieCount  int
}

type rankingView struct {
	Title        string
	Fuel         string
	Unit         string
	Population   int
	Rows         []rankingRow
	Truncated    bool
	TiesAtCutoff int
}

type stateLink struct {
	Code     string
	Name     string
	Href     string
	Stations int
	Fuels    int
}

type municipalityLink struct {
	Code         string
	Name         string
	UF           string
	Href         string
	Stations     int
	Insufficient bool
}

type consultarPage struct {
	pageBase
	States        []stateLink
	National      []summaryRow
	SearchStates  []searchState
	SearchRelease string
}

type precosPage struct {
	pageBase
	FuelHubs      []fuelHubLink
	National      []summaryRow
	Rankings      []rankingView
	States        []stateLink
	SearchRelease string
	SearchFuels   []searchFuel
}

type statePage struct {
	pageBase
	NoDataCities   []municipalityLink
	UF             string
	Name           string
	Summaries      []summaryRow
	Rankings       []rankingView
	Municipalities []municipalityLink
}

type municipalityPage struct {
	pageBase
	NoData        bool
	Stations      []moduleLink
	StationsTotal int
	Code          string
	Name          string
	UF            string
	Summaries     []summaryRow
	Rankings      []rankingView
	Rows          []rankingRow
	Insufficient  bool
	Population    int
}

type stationView struct {
	ID               string
	Name             string
	Registry         string
	LegalName        string
	CNPJ             string
	Address          string
	Status           string
	Location         string
	Place            string
	StateHref        string
	StateName        string
	Municipality     string
	MunicipalityHref string
}

type observationView struct {
	Fuel      string
	Unit      string
	Condition string
	Amount    int64
	Collected string
	Period    string
}

type historyPointView struct {
	Period    string
	Collected string
	Amount    *int64
	Missing   bool
}

type historyView struct {
	Fuel   string
	Unit   string
	Points []historyPointView
}

type sourceView struct {
	URL           string
	Checksum      string
	ParserVersion string
	PublishedAt   string
	Period        string
}

type stationPage struct {
	pageBase
	Station         stationView
	Observations    []observationView
	Histories       []historyView
	Source          sourceView
	OfficialRecords []officialRecordView
	OfficialCount   int
	OfficialMore    bool
}

type observation struct {
	station enterpriseexport.Station
	price   enterpriseexport.Price
}

type generator struct {
	opts             Options
	outDir           string
	tmpl             *template.Template
	files            map[string]FileDigest
	pages            int
	bytes            int64
	indexable        bool
	siteURL          string
	cssURL           string
	scriptURL        string
	searchPartsReady bool
	// searchRelease/searchFuels/searchFiscal describe the v2 search index
	// of a full build; empty in official-only builds.
	searchRelease string
	searchFuels   []searchFuel
	searchFiscal  bool
	sitemap       []sitemapEntry
	hasSitemap    bool
	// cityRoutes maps an IBGE code to its readable /precos/{uf}/{slug}/ route.
	cityRoutes map[string]string
	statePages map[string]bool
	cityPages  map[string]bool
	cityState  map[string]string
	fuelHubs   map[string]fuelHubLink
	latest     *enterpriseexport.Revision
	// Station index and city pages, prepared before any page is written.
	entries         []stationEntry
	pricesByStation map[string][]enterpriseexport.Price
	cities          map[string]*cityInfo
	observed        map[string][]observation
	// Page-wide publication facts.
	period        string
	periodShort   string
	surveyDate    string
	published     string
	inputHash     string
	buildDate     string
	dataDate      string
	landingPrefix string
}

// Generate reads one frozen export candidate and writes a complete private
// static portal candidate. It never calls the database, the API or a source.
func Generate(opts Options) (Result, error) {
	if opts.SnapshotDir == "" || opts.OutputDir == "" {
		return Result{}, fmt.Errorf("portalgen: snapshot and output directories are required")
	}
	if opts.MaxPages <= 0 {
		opts.MaxPages = defaultMaxPages
	}
	if opts.MaxBytes <= 0 {
		opts.MaxBytes = defaultMaxBytes
	}
	siteURL := strings.TrimRight(opts.SiteURL, "/")
	if siteURL != "" && !validOrigin(siteURL) {
		return Result{}, fmt.Errorf("portalgen: invalid site URL")
	}
	opts.LandingURL = strings.TrimRight(opts.LandingURL, "/")
	if opts.LandingURL != "" && !validOrigin(opts.LandingURL) {
		return Result{}, fmt.Errorf("portalgen: invalid landing URL")
	}
	if opts.Indexable && siteURL == "" {
		return Result{}, fmt.Errorf("portalgen: indexable output requires a site URL")
	}
	if _, err := os.Stat(opts.OutputDir); err == nil {
		return Result{}, fmt.Errorf("portalgen: output directory already exists")
	} else if !os.IsNotExist(err) {
		return Result{}, err
	}
	parent := filepath.Dir(opts.OutputDir)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return Result{}, err
	}
	tmp, err := os.MkdirTemp(parent, "."+filepath.Base(opts.OutputDir)+".tmp-")
	if err != nil {
		return Result{}, err
	}
	success := false
	defer func() {
		if !success {
			_ = os.RemoveAll(tmp)
		}
	}()

	manifest, snapshot, projection, err := readInput(opts.SnapshotDir)
	if err != nil {
		return Result{}, err
	}
	g := &generator{
		opts: opts, outDir: tmp, files: map[string]FileDigest{}, indexable: opts.Indexable,
		siteURL: siteURL, buildDate: isoDate(manifest.BuiltAt),
		statePages: map[string]bool{}, cityPages: map[string]bool{},
	}
	if opts.LandingURL != "" && opts.LandingURL != siteURL {
		g.landingPrefix = opts.LandingURL
	}
	g.dataDate = g.buildDate
	g.tmpl, err = template.New("portal").Funcs(template.FuncMap{
		"brl": brl, "brlPtr": brlPtr, "dateBR": dateBR,
		"periodLabel": periodLabel, "fuelLabel": fuelLabel, "plural": plural,
	}).Parse(templates)
	if err != nil {
		return Result{}, err
	}
	if err := g.copyAssets(); err != nil {
		return Result{}, err
	}

	stations := map[string]enterpriseexport.Station{}
	for _, station := range snapshot.Stations {
		if !isUUID(station.StationID) {
			return Result{}, fmt.Errorf("portalgen: invalid station id")
		}
		if _, exists := stations[station.StationID]; exists {
			return Result{}, fmt.Errorf("portalgen: duplicate station %s", station.StationID)
		}
		stations[station.StationID] = station
	}
	revisions := make(map[string]enterpriseexport.Revision, len(snapshot.Revisions))
	for _, revision := range snapshot.Revisions {
		if !isUUID(revision.RevisionID) {
			return Result{}, fmt.Errorf("portalgen: invalid revision id")
		}
		if _, exists := revisions[revision.RevisionID]; exists {
			return Result{}, fmt.Errorf("portalgen: duplicate revision %s", revision.RevisionID)
		}
		revisions[revision.RevisionID] = revision
	}
	for _, price := range snapshot.Prices {
		if _, ok := stations[price.StationID]; !ok {
			return Result{}, fmt.Errorf("portalgen: price has unknown station")
		}
		if _, ok := revisions[price.RevisionID]; !ok {
			return Result{}, fmt.Errorf("portalgen: price has unknown revision")
		}
	}
	official, err := loadOfficialDataset(opts.SnapshotDir, stations, !opts.OfficialOnly)
	if err != nil {
		return Result{}, err
	}
	combinedHash := combinedInputHash(manifest.ContentSHA256, official)
	if opts.OfficialOnly {
		if official == nil {
			return Result{}, fmt.Errorf("portalgen: official-only output requires an official dataset")
		}
		g.published, g.inputHash = publicationLabel(manifest.BuiltAt), combinedHash
		if err := g.generateOfficial(official, stations); err != nil {
			return Result{}, err
		}
		if err := g.generateStatic(); err != nil {
			return Result{}, err
		}
		if err := g.writeSitemap(); err != nil {
			return Result{}, err
		}
		if err := g.writeRobots(); err != nil {
			return Result{}, err
		}
		officialManifest, err := g.writeManifest(manifest, projection, official, combinedHash)
		if err != nil {
			return Result{}, err
		}
		if err := os.Rename(tmp, opts.OutputDir); err != nil {
			return Result{}, err
		}
		success = true
		return Result{
			Pages: g.pages, Files: len(g.files) + 1, Bytes: g.bytes,
			ContentSHA256: officialManifest.ContentSHA256,
		}, nil
	}
	latest := latestRevision(snapshot.Revisions)
	observations, err := latestObservations(snapshot, latest, stations)
	if err != nil {
		return Result{}, err
	}
	var latestCoverage analytics.Coverage
	if latest != nil {
		if coverage, ok := coverageFor(projection, latest.RevisionID); ok {
			latestCoverage = coverage
			if coverage.IncludedObservations != len(observations) {
				return Result{}, fmt.Errorf("portalgen: analytics coverage and snapshot observations disagree")
			}
		}
	}
	obsByStation := map[string][]observation{}
	for _, item := range observations {
		obsByStation[item.station.StationID] = append(obsByStation[item.station.StationID], item)
	}
	// Search files have immutable cache headers, so format changes need a
	// fresh URL even when the input snapshot has not changed.
	searchDigest := sha256.Sum256([]byte(manifest.ContentSHA256 + "\x00" + GeneratorRevision))
	searchRelease := hex.EncodeToString(searchDigest[:])
	shards, err := buildSearchShards(snapshot, observations, searchRelease)
	if err != nil {
		return Result{}, err
	}
	searchStates := make([]searchState, 0, len(shards))
	for _, shard := range shards {
		searchStates = append(searchStates, searchState{Code: shard.UF, Name: searchStateName(shard.UF)})
	}
	sort.Slice(searchStates, func(i, j int) bool { return searchStates[i].Code < searchStates[j].Code })
	if err := g.copySearchScript(opts.SearchScriptPath); err != nil {
		return Result{}, err
	}
	for _, shard := range shards {
		if err := validateSearchShard(shard); err != nil {
			return Result{}, err
		}
		if err := g.writeAsset("dados/busca/"+searchRelease+"/"+shard.UF+".json", shard.Data); err != nil {
			return Result{}, err
		}
	}
	indexes, searchIdx, err := buildSearchIndexes(snapshot, observations, fiscalSearchRows(official, stations, true), searchRelease)
	if err != nil {
		return Result{}, err
	}
	g.searchRelease, g.searchFuels, g.searchFiscal = searchRelease, searchIdx.Fuels, searchIdx.Fiscal
	for _, file := range indexes {
		if err := g.writeAsset("dados/busca/"+searchRelease+"/"+file.Path, file.Data); err != nil {
			return Result{}, err
		}
	}
	stateStations, municipalityStations, municipalityState, municipalityName, err := geography(observations)
	if err != nil {
		return Result{}, err
	}
	g.cityState = municipalityState
	latestSummaries := summariesFor(projection, latest)
	g.fuelHubs = planFuelHubs(latestSummaries)
	g.latest = latest
	latestRankings := rankingsFor(projection, latest)
	previous := previousRevision(snapshot.Revisions, latest)
	previousSummaries := summariesForRevision(projection, previous)
	published := publicationLabel(manifest.BuiltAt)
	period := ""
	if latest != nil {
		period = periodLabel(latest.SurveyStart, latest.SurveyEnd)
		g.periodShort = periodShort(latest.SurveyStart, latest.SurveyEnd)
		g.surveyDate = dateBR(latest.SurveyEnd)
		g.dataDate = revisionDataDate(*latest, g.buildDate)
	}
	g.period, g.published, g.inputHash = period, published, combinedHash
	g.observed = obsByStation
	g.prepareStationIndex(snapshot, stations, revisions, obsByStation)
	g.prepareGeography(latestSummaries, municipalityState, municipalityName)
	release := releaseSlug(manifest.ContentSHA256)
	downloadFiles, manifestLink, err := g.writeDownloads(snapshot, latest, stations, observations, latestSummaries, release, period, published, combinedHash)
	if err != nil {
		return Result{}, err
	}
	cards := buildInsights(latest, previous, latestSummaries, previousSummaries, latestCoverage, observations, municipalityStations, published)
	if official != nil {
		cards = append(cards, buildOfficialInsights(official)...)
	}
	if err := g.generateCuriosidades(cards); err != nil {
		return Result{}, err
	}
	if err := g.generateDownloads(downloadFiles, manifestLink, release); err != nil {
		return Result{}, err
	}
	if err := g.generateConsultar(latestSummaries, stateStations, searchStates, searchRelease); err != nil {
		return Result{}, err
	}
	if err := g.generatePrecos(latestSummaries, latestRankings, stations, stateStations); err != nil {
		return Result{}, err
	}
	if err := g.generateStates(latestSummaries, latestRankings, stations, municipalityStations); err != nil {
		return Result{}, err
	}
	if err := g.generateMunicipalities(latestSummaries, observations, municipalityStations); err != nil {
		return Result{}, err
	}
	if err := g.generateFuelHubs(latestSummaries, municipalityName); err != nil {
		return Result{}, err
	}
	if err := g.generateStations(snapshot, stations, revisions, official); err != nil {
		return Result{}, err
	}
	if err := g.generateOfficial(official, stations); err != nil {
		return Result{}, err
	}
	if err := g.generateStatic(); err != nil {
		return Result{}, err
	}
	if err := g.writeSitemap(); err != nil {
		return Result{}, err
	}
	if err := g.writeRobots(); err != nil {
		return Result{}, err
	}
	portalManifest, err := g.writeManifest(manifest, projection, official, combinedHash)
	if err != nil {
		return Result{}, err
	}
	if err := os.Rename(tmp, opts.OutputDir); err != nil {
		return Result{}, err
	}
	success = true
	return Result{
		Pages: g.pages, Files: len(g.files) + 1, Bytes: g.bytes,
		ContentSHA256: portalManifest.ContentSHA256,
	}, nil
}

func readInput(dir string) (enterpriseexport.Manifest, enterpriseexport.Snapshot, analytics.Projection, error) {
	var manifest enterpriseexport.Manifest
	raw, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return manifest, enterpriseexport.Snapshot{}, analytics.Projection{}, err
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return manifest, enterpriseexport.Snapshot{}, analytics.Projection{}, err
	}
	if manifest.SchemaVersion != enterpriseexport.SchemaVersion || manifest.ContentSHA256 == "" {
		return manifest, enterpriseexport.Snapshot{}, analytics.Projection{}, fmt.Errorf("portalgen: invalid export manifest")
	}
	var projection analytics.Projection
	raw, err = os.ReadFile(filepath.Join(dir, "analytics-v1.json"))
	if err != nil {
		return manifest, enterpriseexport.Snapshot{}, analytics.Projection{}, err
	}
	if err := json.Unmarshal(raw, &projection); err != nil {
		return manifest, enterpriseexport.Snapshot{}, analytics.Projection{}, err
	}
	if projection.MetricsVersion != analytics.MetricsVersion || projection.InputContentSHA256 != manifest.ContentSHA256 {
		return manifest, enterpriseexport.Snapshot{}, analytics.Projection{}, fmt.Errorf("portalgen: analytics do not match the export manifest")
	}
	snapshot, err := enterpriseexport.ReadSnapshot(dir)
	if err != nil {
		return manifest, enterpriseexport.Snapshot{}, analytics.Projection{}, err
	}
	return manifest, snapshot, projection, nil
}

func latestRevision(revisions []enterpriseexport.Revision) *enterpriseexport.Revision {
	if len(revisions) == 0 {
		return nil
	}
	sorted := append([]enterpriseexport.Revision(nil), revisions...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].SurveyStart != sorted[j].SurveyStart {
			return sorted[i].SurveyStart < sorted[j].SurveyStart
		}
		return sorted[i].RevisionID < sorted[j].RevisionID
	})
	latest := sorted[len(sorted)-1]
	return &latest
}

func latestObservations(snapshot enterpriseexport.Snapshot, latest *enterpriseexport.Revision, stations map[string]enterpriseexport.Station) ([]observation, error) {
	if latest == nil {
		return nil, nil
	}
	start, err := time.Parse("2006-01-02", latest.SurveyStart)
	if err != nil {
		return nil, fmt.Errorf("portalgen: invalid survey start")
	}
	end, err := time.Parse("2006-01-02", latest.SurveyEnd)
	if err != nil {
		return nil, fmt.Errorf("portalgen: invalid survey end")
	}
	type key struct{ station, fuel, unit string }
	latestByKey := map[key]enterpriseexport.Price{}
	for _, price := range snapshot.Prices {
		if price.RevisionID != latest.RevisionID {
			continue
		}
		if price.Currency != "BRL" {
			continue
		}
		unit, err := kernel.Product(price.FuelProduct).Unit()
		if err != nil || price.Unit != string(unit) {
			continue
		}
		collected, err := time.Parse("2006-01-02", price.CollectedOn)
		if err != nil || collected.Before(start) || collected.After(end) {
			continue
		}
		k := key{station: price.StationID, fuel: price.FuelProduct, unit: price.Unit}
		if previous, ok := latestByKey[k]; !ok || newerPrice(price, previous) {
			latestByKey[k] = price
		}
	}
	out := make([]observation, 0, len(latestByKey))
	for k, price := range latestByKey {
		out = append(out, observation{station: stations[k.station], price: price})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].station.DisplayName != out[j].station.DisplayName {
			return out[i].station.DisplayName < out[j].station.DisplayName
		}
		if out[i].station.StationID != out[j].station.StationID {
			return out[i].station.StationID < out[j].station.StationID
		}
		if out[i].price.FuelProduct != out[j].price.FuelProduct {
			return out[i].price.FuelProduct < out[j].price.FuelProduct
		}
		return out[i].price.Unit < out[j].price.Unit
	})
	return out, nil
}

func newerPrice(candidate, existing enterpriseexport.Price) bool {
	if candidate.CollectedOn != existing.CollectedOn {
		return candidate.CollectedOn > existing.CollectedOn
	}
	return candidate.PriceID > existing.PriceID
}

func coverageFor(projection analytics.Projection, revisionID string) (analytics.Coverage, bool) {
	for _, coverage := range projection.Coverage {
		if coverage.RevisionID == revisionID {
			return coverage, true
		}
	}
	return analytics.Coverage{}, false
}

func summariesFor(projection analytics.Projection, latest *enterpriseexport.Revision) []analytics.Summary {
	if latest == nil {
		return nil
	}
	return summariesForRevision(projection, latest)
}

func summariesForRevision(projection analytics.Projection, revision *enterpriseexport.Revision) []analytics.Summary {
	if revision == nil {
		return nil
	}
	out := make([]analytics.Summary, 0)
	for _, summary := range projection.Summaries {
		if summary.RevisionID == revision.RevisionID {
			out = append(out, summary)
		}
	}
	return out
}

func rankingsFor(projection analytics.Projection, latest *enterpriseexport.Revision) []analytics.Ranking {
	if latest == nil {
		return nil
	}
	out := make([]analytics.Ranking, 0)
	for _, ranking := range projection.Rankings {
		if ranking.RevisionID == latest.RevisionID {
			out = append(out, ranking)
		}
	}
	return out
}

func geography(observations []observation) (map[string]int, map[string]int, map[string]string, map[string]string, error) {
	stateStations := map[string]int{}
	municipalityStations := map[string]int{}
	municipalityState := map[string]string{}
	municipalityName := map[string]string{}
	stateSets := map[string]map[string]bool{}
	municipalitySets := map[string]map[string]bool{}
	for _, item := range observations {
		station := item.station
		if station.State != "" {
			if stateSets[station.State] == nil {
				stateSets[station.State] = map[string]bool{}
			}
			stateSets[station.State][station.StationID] = true
		}
		if station.MunicipalityCode != "" && station.State != "" {
			code := station.MunicipalityCode
			if municipalitySets[code] == nil {
				municipalitySets[code] = map[string]bool{}
			}
			municipalitySets[code][station.StationID] = true
			if existing := municipalityState[code]; existing != "" && existing != station.State {
				return nil, nil, nil, nil, fmt.Errorf("portalgen: municipality code spans states")
			}
			municipalityState[code] = station.State
			if municipalityName[code] == "" {
				if name := municipalityLabel(code, station.Address["municipio"]); name != "" {
					municipalityName[code] = name
				}
			}
		}
	}
	for code, set := range stateSets {
		stateStations[code] = len(set)
	}
	for code, set := range municipalitySets {
		municipalityStations[code] = len(set)
	}
	return stateStations, municipalityStations, municipalityState, municipalityName, nil
}

func (g *generator) writePage(rel, templateName string, data any) error {
	if g.pages+1 > g.opts.MaxPages {
		return &BudgetError{Msg: "portalgen: page budget exceeded"}
	}
	clean := filepath.Clean("/" + rel)
	rel = strings.TrimPrefix(clean, "/")
	full := filepath.Join(g.outDir, rel)
	if full != g.outDir && !strings.HasPrefix(full, g.outDir+string(os.PathSeparator)) {
		return fmt.Errorf("portalgen: unsafe page path")
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
		return err
	}
	var buf bytes.Buffer
	if err := g.tmpl.ExecuteTemplate(&buf, templateName, data); err != nil {
		return err
	}
	if g.bytes+int64(buf.Len()) > g.opts.MaxBytes {
		return &BudgetError{Msg: "portalgen: byte budget exceeded"}
	}
	if err := os.WriteFile(full, buf.Bytes(), 0o600); err != nil {
		return err
	}
	g.pages++
	g.bytes += int64(buf.Len())
	g.files[rel] = FileDigest{Path: rel, SHA256: digestBytes(buf.Bytes()), Bytes: int64(buf.Len())}
	return nil
}

func (g *generator) writeAsset(rel string, data []byte) error {
	if strings.HasPrefix(rel, "dados/busca/") && strings.HasSuffix(rel, ".json") {
		files, err := partitionSearchFile(rel, data)
		if err != nil {
			return err
		}
		if len(files) > 1 && g.opts.SearchScriptPath != "" && !g.searchPartsReady {
			return fmt.Errorf("portalgen: multipart search requires matching search script")
		}
		for _, file := range files {
			if err := g.writeAssetRaw(file.Path, file.Data); err != nil {
				return err
			}
		}
		return nil
	}
	return g.writeAssetRaw(rel, data)
}

func (g *generator) writeAssetRaw(rel string, data []byte) error {
	if g.bytes+int64(len(data)) > g.opts.MaxBytes {
		return &BudgetError{Msg: "portalgen: byte budget exceeded"}
	}
	full := filepath.Join(g.outDir, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(full, data, 0o600); err != nil {
		return err
	}
	g.bytes += int64(len(data))
	g.files[rel] = FileDigest{Path: rel, SHA256: digestBytes(data), Bytes: int64(len(data))}
	return nil
}

func (g *generator) copySearchScript(path string) error {
	if path == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("portalgen: search script: %w", err)
	}
	if len(data) == 0 {
		return fmt.Errorf("portalgen: empty search script")
	}
	g.searchPartsReady = bytes.Contains(data, []byte(SearchPartsSchema))
	sum := sha256.Sum256(data)
	name := "portal." + hex.EncodeToString(sum[:4]) + ".js"
	if err := g.writeAsset("assets/"+name, data); err != nil {
		return err
	}
	g.scriptURL = "/assets/" + name
	return nil
}

func (g *generator) copyAssets() error {
	css, err := embeddedAssets.ReadFile("assets/portal.css")
	if err != nil {
		return err
	}
	sum := sha256.Sum256(css)
	name := "portal." + hex.EncodeToString(sum[:4]) + ".css"
	if err := g.writeAsset("assets/"+name, css); err != nil {
		return err
	}
	g.cssURL = "/assets/" + name
	return nil
}

// writeRoute renders one routed page and, when the page decision is
// indexable, registers it for the sitemap with its data date.
func (g *generator) writeRoute(templateName string, data routedPage) error {
	base := data.page()
	if !strings.HasPrefix(base.Route, "/") || !strings.HasSuffix(base.Route, "/") {
		return fmt.Errorf("portalgen: invalid page route")
	}
	if err := g.writePage(strings.TrimPrefix(base.Route, "/")+"index.html", templateName, data); err != nil {
		return err
	}
	if base.Indexable {
		g.sitemap = append(g.sitemap, sitemapEntry{Route: base.Route, Family: base.Family, LastMod: base.LastMod})
	}
	return nil
}

type sitemapEntry struct {
	Route   string
	Family  pageFamily
	LastMod string
}

type sitemapURL struct {
	Loc     string `xml:"loc"`
	LastMod string `xml:"lastmod,omitempty"`
}

type sitemapSet struct {
	XMLName xml.Name     `xml:"urlset"`
	XMLNS   string       `xml:"xmlns,attr"`
	URLs    []sitemapURL `xml:"url"`
}

func (g *generator) writeRobots() error {
	body := "User-agent: *\nDisallow: /\n"
	if g.indexable {
		body = "User-agent: *\nAllow: /\n"
		if g.hasSitemap {
			body += "Sitemap: " + g.siteURL + "/sitemap.xml\n"
		}
	}
	return g.writeAsset("robots.txt", []byte(body))
}

func (g *generator) writeManifest(exportManifest enterpriseexport.Manifest, projection analytics.Projection, official *officialDataset, inputHash string) (Manifest, error) {
	files := make([]FileDigest, 0, len(g.files))
	for _, file := range g.files {
		files = append(files, file)
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	manifest := Manifest{
		SchemaVersion: SchemaVersion, InputContentSHA256: inputHash,
		MetricsVersion: projection.MetricsVersion,
		PublishedAt:    exportManifest.BuiltAt,
		Pages:          g.pages, Files: files,
		OfficialOnly: g.opts.OfficialOnly,
	}
	if official != nil {
		pin := official.Manifest.pin()
		manifest.OfficialRecords = &pin
	}
	contentSHA256, err := manifestContentDigest(manifest)
	if err != nil {
		return Manifest{}, err
	}
	manifest.ContentSHA256 = contentSHA256
	encoded, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return Manifest{}, err
	}
	encoded = append(encoded, '\n')
	if g.bytes+int64(len(encoded)) > g.opts.MaxBytes {
		return Manifest{}, &BudgetError{Msg: "portalgen: byte budget exceeded"}
	}
	if err := os.WriteFile(filepath.Join(g.outDir, "portal-manifest.json"), encoded, 0o600); err != nil {
		return Manifest{}, err
	}
	g.bytes += int64(len(encoded))
	return manifest, nil
}

// manifestContentDigest binds the complete manifest, including the official
// pin, so a verifier can reject a changed source/association descriptor even
// when the page files are untouched.
func manifestContentDigest(manifest Manifest) (string, error) {
	descriptor, err := json.Marshal(struct {
		SchemaVersion      string       `json:"schema_version"`
		InputContentSHA256 string       `json:"input_content_sha256"`
		MetricsVersion     string       `json:"metrics_version"`
		PublishedAt        string       `json:"published_at"`
		Pages              int          `json:"pages"`
		Files              []FileDigest `json:"files"`
		OfficialRecords    *officialPin `json:"official_records,omitempty"`
		OfficialOnly       bool         `json:"official_only"`
	}{
		SchemaVersion: manifest.SchemaVersion, InputContentSHA256: manifest.InputContentSHA256,
		MetricsVersion: manifest.MetricsVersion, PublishedAt: manifest.PublishedAt,
		Pages: manifest.Pages, Files: manifest.Files, OfficialRecords: manifest.OfficialRecords,
		OfficialOnly: manifest.OfficialOnly,
	})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(descriptor)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

// validOrigin accepts only an http(s) scheme and host, without credentials,
// path, query or fragment.
func validOrigin(raw string) bool {
	parsed, err := url.Parse(raw)
	return err == nil && (parsed.Scheme == "https" || parsed.Scheme == "http") && parsed.Host != "" &&
		parsed.User == nil && parsed.RawQuery == "" && parsed.Fragment == "" && (parsed.Path == "" || parsed.Path == "/")
}

func digestBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// isoDate is the UTC calendar day of an RFC 3339 instant, or "".
func isoDate(value string) string {
	at, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return ""
	}
	return at.UTC().Format("2006-01-02")
}

// revisionDataDate is the day the revision's data changed: its publication
// day, else its survey end, else fallback.
func revisionDataDate(revision enterpriseexport.Revision, fallback string) string {
	if day := isoDate(revision.PublishedAt); day != "" {
		return day
	}
	if _, err := parseDay(revision.SurveyEnd); err == nil {
		return revision.SurveyEnd
	}
	return fallback
}
