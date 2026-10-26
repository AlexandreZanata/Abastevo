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
	SchemaVersion     = "enterprise-portal-v2"
	defaultMaxPages   = 50_000
	defaultMaxBytes   = int64(2 << 30)
	maxRankingPerPage = 20
	minMunicipality   = 5
)

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
	Title        string
	Description  string
	ActiveNav    string
	Indexable    bool
	OfficialOnly bool
	Landing      string
	Period       string
	PublishedAt  string
	InputHash    string
	CSSURL       string
	ScriptURL    string
	Breadcrumbs  []crumb
}

type crumb struct {
	Label   string
	Href    string
	Current bool
}

type summaryRow struct {
	Fuel         string
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
	National []summaryRow
	Rankings []rankingView
	States   []stateLink
}

type statePage struct {
	pageBase
	UF             string
	Name           string
	Summaries      []summaryRow
	Rankings       []rankingView
	Municipalities []municipalityLink
}

type municipalityPage struct {
	pageBase
	Code         string
	Name         string
	UF           string
	Summaries    []summaryRow
	Rankings     []rankingView
	Rows         []rankingRow
	Insufficient bool
	Population   int
}

type stationView struct {
	ID               string
	Name             string
	CNPJ             string
	Address          string
	Status           string
	Location         string
	Place            string
	StateHref        string
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
	opts          Options
	outDir        string
	tmpl          *template.Template
	files         map[string]FileDigest
	pages         int
	bytes         int64
	indexable     bool
	siteURL       string
	cssURL        string
	scriptURL     string
	sitemapRoutes []string
	hasSitemap    bool
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
		siteURL: siteURL,
	}
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
		officialPublished := publicationLabel(manifest.BuiltAt)
		if err := g.generateOfficial(official, stations, "", officialPublished, combinedHash); err != nil {
			return Result{}, err
		}
		if err := g.generateStatic("", officialPublished, combinedHash); err != nil {
			return Result{}, err
		}
		if err := g.writeSitemap(manifest.BuiltAt); err != nil {
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
	release := releaseSlug(manifest.ContentSHA256)
	shards, err := buildSearchShards(snapshot, observations, release)
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
		if err := g.writeAsset("dados/busca/"+release+"/"+shard.UF+".json", shard.Data); err != nil {
			return Result{}, err
		}
	}
	stateStations, municipalityStations, municipalityState, municipalityName, err := geography(observations)
	if err != nil {
		return Result{}, err
	}
	latestSummaries := summariesFor(projection, latest)
	latestRankings := rankingsFor(projection, latest)
	previous := previousRevision(snapshot.Revisions, latest)
	previousSummaries := summariesForRevision(projection, previous)
	published := publicationLabel(manifest.BuiltAt)
	period := ""
	if latest != nil {
		period = periodLabel(latest.SurveyStart, latest.SurveyEnd)
	}
	downloadFiles, manifestLink, err := g.writeDownloads(snapshot, latest, stations, observations, latestSummaries, release, period, published, combinedHash)
	if err != nil {
		return Result{}, err
	}
	cards := buildInsights(latest, previous, latestSummaries, previousSummaries, latestCoverage, observations, municipalityStations, published)
	if official != nil {
		cards = append(cards, buildOfficialInsights(official)...)
	}
	if err := g.generateCuriosidades(cards, period, published, combinedHash); err != nil {
		return Result{}, err
	}
	if err := g.generateDownloads(downloadFiles, manifestLink, period, release, published, combinedHash); err != nil {
		return Result{}, err
	}

	if err := g.generateConsultar(latestSummaries, stateStations, searchStates, release, period, published, combinedHash); err != nil {
		return Result{}, err
	}
	if err := g.generatePrecos(latestSummaries, latestRankings, stations, stateStations, period, published, combinedHash); err != nil {
		return Result{}, err
	}
	if err := g.generateStates(latestSummaries, latestRankings, stations, municipalityStations, municipalityState, municipalityName, period, published, combinedHash); err != nil {
		return Result{}, err
	}
	if err := g.generateMunicipalities(latestSummaries, observations, municipalityStations, municipalityState, municipalityName, period, published, combinedHash); err != nil {
		return Result{}, err
	}
	if err := g.generateStations(snapshot, stations, obsByStation, stateStations, municipalityStations, municipalityName, official, period, published, combinedHash); err != nil {
		return Result{}, err
	}
	if err := g.generateOfficial(official, stations, period, published, combinedHash); err != nil {
		return Result{}, err
	}
	if err := g.generateStatic(period, published, combinedHash); err != nil {
		return Result{}, err
	}
	if err := g.writeSitemap(manifest.BuiltAt); err != nil {
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
				if name := strings.TrimSpace(station.Address["municipio"]); name != "" {
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
		return fmt.Errorf("portalgen: page budget exceeded")
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
		return fmt.Errorf("portalgen: byte budget exceeded")
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
	if g.bytes+int64(len(data)) > g.opts.MaxBytes {
		return fmt.Errorf("portalgen: byte budget exceeded")
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

func (g *generator) addSitemapRoute(route string) {
	g.sitemapRoutes = append(g.sitemapRoutes, route)
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

func (g *generator) writeSitemap(builtAt string) error {
	if !g.indexable {
		return nil
	}
	routes := append([]string(nil), g.sitemapRoutes...)
	sort.Strings(routes)
	unique := routes[:0]
	for _, route := range routes {
		if len(unique) == 0 || unique[len(unique)-1] != route {
			unique = append(unique, route)
		}
	}
	if len(unique) == 0 || len(unique) > 50_000 {
		return fmt.Errorf("portalgen: invalid sitemap route count")
	}
	lastMod := ""
	if at, err := time.Parse(time.RFC3339, builtAt); err == nil {
		lastMod = at.UTC().Format("2006-01-02")
	}
	set := sitemapSet{XMLNS: "http://www.sitemaps.org/schemas/sitemap/0.9", URLs: make([]sitemapURL, 0, len(unique))}
	for _, route := range unique {
		if !strings.HasPrefix(route, "/") || !strings.HasSuffix(route, "/") {
			return fmt.Errorf("portalgen: invalid sitemap route")
		}
		set.URLs = append(set.URLs, sitemapURL{Loc: g.siteURL + route, LastMod: lastMod})
	}
	body, err := xml.MarshalIndent(set, "", "  ")
	if err != nil {
		return err
	}
	body = append([]byte(xml.Header), body...)
	body = append(body, '\n')
	if err := g.writeAsset("sitemap.xml", body); err != nil {
		return err
	}
	g.hasSitemap = true
	return nil
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
		return Manifest{}, fmt.Errorf("portalgen: byte budget exceeded")
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

func (g *generator) base(title, description, active, period, published, inputHash string, crumbs []crumb) pageBase {
	return pageBase{
		Title: title, Description: description, ActiveNav: active,
		Indexable: g.indexable, OfficialOnly: g.opts.OfficialOnly, Landing: g.opts.LandingURL,
		Period: period, PublishedAt: published,
		InputHash: inputHash, CSSURL: g.cssURL, ScriptURL: g.scriptURL, Breadcrumbs: crumbs,
	}
}

func (g *generator) generateConsultar(summaries []analytics.Summary, states map[string]int, searchStates []searchState, release, period, published, inputHash string) error {
	stateLinks := make([]stateLink, 0)
	for code, stations := range states {
		fuels := map[string]bool{}
		for _, summary := range summaries {
			if summary.Geography.Level == analytics.LevelState && summary.Geography.Code == code {
				fuels[summary.Fuel] = true
			}
		}
		stateLinks = append(stateLinks, stateLink{Code: code, Name: ufName(code), Stations: stations, Fuels: len(fuels)})
	}
	sort.Slice(stateLinks, func(i, j int) bool { return stateLinks[i].Code < stateLinks[j].Code })
	data := consultarPage{
		pageBase: g.base("Consultar preços de combustíveis", "Consulta estática de preços da pesquisa ANP, com período, fonte, amostra e status informado.", "consultar", period, published, inputHash, []crumb{{Label: "Consultar", Current: true}}),
		States:   stateLinks, National: summaryRows(summaries, analytics.LevelNational, "BR", false),
		SearchStates: searchStates, SearchRelease: release,
	}
	if err := g.writePage("consultar/index.html", "consultar", data); err != nil {
		return err
	}
	g.addSitemapRoute("/consultar/")
	return nil
}

func (g *generator) generatePrecos(summaries []analytics.Summary, rankings []analytics.Ranking, stations map[string]enterpriseexport.Station, states map[string]int, period, published, inputHash string) error {
	stateLinks := make([]stateLink, 0)
	for code, stations := range states {
		fuels := map[string]bool{}
		for _, summary := range summaries {
			if summary.Geography.Level == analytics.LevelState && summary.Geography.Code == code {
				fuels[summary.Fuel] = true
			}
		}
		stateLinks = append(stateLinks, stateLink{Code: code, Name: ufName(code), Stations: stations, Fuels: len(fuels)})
	}
	sort.Slice(stateLinks, func(i, j int) bool { return stateLinks[i].Code < stateLinks[j].Code })
	views, err := rankingViews(rankings, analytics.LevelNational, "", stations, true)
	if err != nil {
		return err
	}
	data := precosPage{
		pageBase: g.base("Preços por região", "Resumo derivado da pesquisa ANP por região, com amostra, mínimo, máximo, dispersão e vínculo com a metodologia.", "precos", period, published, inputHash, []crumb{{Label: "Preços", Current: true}}),
		National: summaryRows(summaries, analytics.LevelNational, "BR", false), Rankings: views, States: stateLinks,
	}
	if err := g.writePage("precos/index.html", "precos", data); err != nil {
		return err
	}
	g.addSitemapRoute("/precos/")
	return nil
}

func (g *generator) generateStates(summaries []analytics.Summary, rankings []analytics.Ranking, stations map[string]enterpriseexport.Station, municipalityStations map[string]int, municipalityState, municipalityName map[string]string, period, published, inputHash string) error {
	states := map[string]bool{}
	for _, summary := range summaries {
		if summary.Geography.Level == analytics.LevelState {
			states[summary.Geography.Code] = true
		}
	}
	for code := range states {
		uf := code
		if !isUF(uf) {
			return fmt.Errorf("portalgen: invalid state code")
		}
		stateSummaries := summaryRows(summaries, analytics.LevelState, uf, false)
		views, err := rankingViews(rankings, analytics.LevelState, uf, stations, true)
		if err != nil {
			return err
		}
		municipalities := map[string]bool{}
		for _, summary := range summaries {
			if summary.Geography.Level == analytics.LevelMunicipality {
				code := summary.Geography.Code
				if municipalityState[code] == uf {
					municipalities[code] = true
				}
			}
		}
		links := make([]municipalityLink, 0, len(municipalities))
		for code := range municipalities {
			stationsCount := municipalityStations[code]
			name := municipalityName[code]
			if name == "" {
				name = "Município " + code
			}
			links = append(links, municipalityLink{
				Code: code, Name: name, UF: uf,
				Href:     "/precos/uf/" + uf + "/municipio/" + code + "/",
				Stations: stationsCount, Insufficient: stationsCount < minMunicipality,
			})
		}
		sort.Slice(links, func(i, j int) bool { return links[i].Name < links[j].Name })
		data := statePage{
			pageBase: g.base("Preços em "+ufName(uf), "Preços da pesquisa ANP em "+ufName(uf)+", com amostra, período, resumo derivado e postos.", "precos", period, published, inputHash, []crumb{{Label: "Preços", Href: "/precos/"}, {Label: ufName(uf), Current: true}}),
			UF:       uf, Name: ufName(uf), Summaries: stateSummaries, Rankings: views, Municipalities: links,
		}
		if err := g.writePage("precos/uf/"+uf+"/index.html", "state", data); err != nil {
			return err
		}
		g.addSitemapRoute("/precos/uf/" + uf + "/")
	}
	return nil
}

func (g *generator) generateMunicipalities(summaries []analytics.Summary, observations []observation, municipalityStations map[string]int, municipalityState, municipalityName map[string]string, period, published, inputHash string) error {
	municipalities := map[string]bool{}
	for _, summary := range summaries {
		if summary.Geography.Level == analytics.LevelMunicipality {
			municipalities[summary.Geography.Code] = true
		}
	}
	for code := range municipalities {
		if !isIBGE(code) {
			return fmt.Errorf("portalgen: invalid municipality code")
		}
		uf := municipalityState[code]
		if uf == "" {
			continue
		}
		if !isUF(uf) {
			return fmt.Errorf("portalgen: invalid municipality state")
		}
		name := municipalityName[code]
		if name == "" {
			name = "Município " + code
		}
		insufficient := municipalityStations[code] < minMunicipality
		data := municipalityPage{
			pageBase: g.base("Preços em "+name, "Preços da pesquisa ANP em "+name+", com amostra, período e resumo derivado.", "precos", period, published, inputHash, []crumb{{Label: "Preços", Href: "/precos/"}, {Label: ufName(uf), Href: "/precos/uf/" + uf + "/"}, {Label: name, Current: true}}),
			Code:     code, Name: name, UF: uf,
			Summaries:    summaryRows(summaries, analytics.LevelMunicipality, code, true),
			Insufficient: insufficient, Population: municipalityStations[code],
		}
		items := make([]observation, 0)
		for _, item := range observations {
			if item.station.MunicipalityCode == code && item.station.State == uf {
				items = append(items, item)
			}
		}
		if insufficient {
			data.Rows = observationRows(items, 0, false)
		} else {
			data.Rankings = observationRankings(items)
		}
		if err := g.writePage("precos/uf/"+uf+"/municipio/"+code+"/index.html", "municipality", data); err != nil {
			return err
		}
		g.addSitemapRoute("/precos/uf/" + uf + "/municipio/" + code + "/")
	}
	return nil
}

func (g *generator) generateStations(snapshot enterpriseexport.Snapshot, stations map[string]enterpriseexport.Station, observations map[string][]observation, stateStations, municipalityStations map[string]int, municipalityName map[string]string, official *officialDataset, period, published, inputHash string) error {
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
	for _, station := range ordered {
		items := observations[station.StationID]
		obsView := make([]observationView, 0, len(items))
		historyKeys := map[string]struct{ fuel, unit string }{}
		for _, item := range items {
			obsView = append(obsView, observationView{
				Fuel: fuelLabel(item.price.FuelProduct), Unit: item.price.Unit,
				Condition: "À vista", Amount: item.price.AmountMilliBRL,
				Collected: dateBR(item.price.CollectedOn), Period: period,
			})
			historyKeys[item.price.FuelProduct+"\x00"+item.price.Unit] = struct{ fuel, unit string }{item.price.FuelProduct, item.price.Unit}
		}
		for _, price := range snapshot.Prices {
			if price.StationID != station.StationID {
				continue
			}
			unit, err := kernel.Product(price.FuelProduct).Unit()
			if err != nil || price.Unit != string(unit) {
				continue
			}
			historyKeys[price.FuelProduct+"\x00"+price.Unit] = struct{ fuel, unit string }{price.FuelProduct, price.Unit}
		}
		historyViews := make([]historyView, 0, len(historyKeys))
		for _, key := range historyKeys {
			history, err := analytics.StationHistory(snapshot.Prices, snapshot.Revisions, station.StationID, key.fuel, key.unit)
			if err != nil {
				return err
			}
			points := make([]historyPointView, 0, len(history.Points))
			hasObservation := false
			for _, point := range history.Points {
				points = append(points, historyPointView{
					Period:    periodLabel(point.SurveyStart, point.SurveyEnd),
					Collected: dateBR(point.CollectedOn), Amount: point.AmountMilliBRL, Missing: point.Missing,
				})
				if !point.Missing {
					hasObservation = true
				}
			}
			if hasObservation {
				historyViews = append(historyViews, historyView{Fuel: fuelLabel(key.fuel), Unit: key.unit, Points: points})
			}
		}
		sort.Slice(historyViews, func(i, j int) bool {
			if historyViews[i].Fuel != historyViews[j].Fuel {
				return historyViews[i].Fuel < historyViews[j].Fuel
			}
			return historyViews[i].Unit < historyViews[j].Unit
		})
		place := "Localização não informada"
		if station.MunicipalityCode != "" {
			name := municipalityName[station.MunicipalityCode]
			if name == "" {
				name = "Município " + station.MunicipalityCode
			}
			place = name + "/" + station.State
		} else if station.State != "" {
			place = ufName(station.State)
		}
		view := stationView{
			ID: station.StationID, Name: station.DisplayName, CNPJ: cnpjBR(station.CNPJ),
			Address: addressLine(station.Address), Status: statusLabel(station.OperationStatus),
			Location: locationLabel(station.LocationQuality), Place: place,
		}
		if station.State != "" && stateStations[station.State] > 0 {
			view.StateHref = "/precos/uf/" + station.State + "/"
		}
		if station.MunicipalityCode != "" && station.State != "" && municipalityStations[station.MunicipalityCode] > 0 {
			view.Municipality = place
			view.MunicipalityHref = "/precos/uf/" + station.State + "/municipio/" + station.MunicipalityCode + "/"
		}
		source := sourceView{}
		if latest := latestRevision(snapshot.Revisions); latest != nil {
			source = sourceView{
				URL: latest.SourceURL, Checksum: latest.SourceChecksum,
				ParserVersion: latest.ParserVersion, PublishedAt: publicationLabel(latest.PublishedAt),
				Period: periodLabel(latest.SurveyStart, latest.SurveyEnd),
			}
		}
		officialRecords, officialCount := officialStationViews(official, station.StationID, stations)
		data := stationPage{
			pageBase: g.base(station.DisplayName, "Preços datados da pesquisa ANP para "+station.DisplayName+", com fonte, histórico, lacunas, status informado e registros oficiais vinculados.", "consultar", period, published, inputHash, []crumb{{Label: "Consultar", Href: "/consultar/"}, {Label: station.DisplayName, Current: true}}),
			Station:  view, Observations: obsView, Histories: historyViews, Source: source,
			OfficialRecords: officialRecords, OfficialCount: officialCount, OfficialMore: officialCount > len(officialRecords),
		}
		if err := g.writePage("postos/"+station.StationID+"/index.html", "station", data); err != nil {
			return err
		}
		g.addSitemapRoute("/postos/" + station.StationID + "/")
	}
	return nil
}

func (g *generator) generateCuriosidades(cards []insightCard, period, published, inputHash string) error {
	data := curiosidadesPage{
		pageBase: g.base("Perguntas aprovadas sobre os dados", "Respostas determinísticas sobre amostra, preços e cobertura da pesquisa ANP, com fórmula e fonte.", "curiosidades", period, published, inputHash, []crumb{{Label: "Curiosidades", Current: true}}),
		Cards:    cards, DownloadsHref: "/dados/",
	}
	if err := g.writePage("curiosidades/index.html", "curiosidades", data); err != nil {
		return err
	}
	g.addSitemapRoute("/curiosidades/")
	return nil
}

func (g *generator) generateDownloads(files []downloadFile, manifest downloadFile, period, release, published, inputHash string) error {
	data := dadosPage{
		pageBase: g.base("Baixar dados públicos", "Downloads CSV e manifesto da pesquisa ANP mais recente, com amostra, período, fonte e limites.", "", period, published, inputHash, []crumb{{Label: "Dados para download", Current: true}}),
		Release:  release, Period: period, Files: files, Manifest: manifest, Notes: downloadNotes,
	}
	if err := g.writePage("dados/index.html", "dados", data); err != nil {
		return err
	}
	g.addSitemapRoute("/dados/")
	return nil
}

func (g *generator) generateStatic(period, published, inputHash string) error {
	metodologia := g.base("Metodologia e fontes", "Fontes, datas, amostra, cálculos derivados, correções, direitos e limites da publicação.", "metodologia", period, published, inputHash, []crumb{{Label: "Metodologia", Current: true}})
	if err := g.writePage("metodologia/index.html", "metodologia", metodologia); err != nil {
		return err
	}
	g.addSitemapRoute("/metodologia/")
	notFound := g.base("Página não encontrada", "A página ou o posto solicitado não existe neste portal estático.", "", period, published, inputHash, []crumb{{Label: "Página não encontrada", Current: true}})
	notFound.Indexable = false
	return g.writePage("404.html", "notfound", notFound)
}

func summaryRows(summaries []analytics.Summary, level, code string, markInsufficient bool) []summaryRow {
	rows := make([]summaryRow, 0)
	for _, summary := range summaries {
		if summary.Geography.Level != level || summary.Geography.Code != code {
			continue
		}
		rows = append(rows, summaryRow{
			Fuel: fuelLabel(summary.Fuel), Unit: summary.Unit,
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
				Position: entry.Position, Name: station.DisplayName, StationID: entry.StationID,
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
			Position: position, Name: item.station.DisplayName, StationID: item.station.StationID,
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
