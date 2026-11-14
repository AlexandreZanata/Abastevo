package portalgen

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	directoryapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/enterpriseexport"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/enterpriseexport/analytics"
)

const seoSite = "https://abastevo.example"

// seoStation is one station of the SEO fixture: registry identity plus the
// survey weeks (1..10, 10 = current) in which it has official prices.
type seoStation struct {
	id, name, street, city, uf string
	lat, lon                   float64
	weeks                      []int
	fuels                      map[string]int64
}

func seoID(n int) string { return fmt.Sprintf("00000000-0000-4000-8000-%012d", 100+n) }

// seoFixture is a multi-city national snapshot with ALL-CAPS names, a
// duplicate name, stale and address-less stations, a city with only older
// (still recent) observations, coordinates and several fuels.
func seoFixture() ([]enterpriseexport.Station, []enterpriseexport.Revision, []enterpriseexport.Price) {
	gas := func(v int64) map[string]int64 { return map[string]int64{"GASOLINE_REGULAR": v} }
	defs := []seoStation{
		{seoID(1), "AUTO POSTO CUIABÁ LTDA", "AVENIDA CUIABÁ,", "5103403", "MT", -15.60, -56.10, []int{9, 10}, map[string]int64{"GASOLINE_REGULAR": 6199, "ETHANOL": 3999, "DIESEL_S10": 6499}},
		{seoID(2), "POSTO SHELL", "RUA A", "5103403", "MT", -15.61, -56.09, []int{10}, map[string]int64{"GASOLINE_REGULAR": 6299, "ETHANOL": 4099}},
		{seoID(3), "POSTO SHELL", "RUA B", "5103403", "MT", -15.59, -56.11, []int{10}, gas(6099)},
		{seoID(4), "POSTO DE COMBUSTIVEIS SÃO JOÃO", "RUA C", "5103403", "MT", -15.62, -56.08, []int{10}, map[string]int64{"GASOLINE_REGULAR": 6399, "ETHANOL": 4199}},
		{seoID(5), "COMERCIAL BR 163 E GNV ME", "RODOVIA BR 163", "5103403", "MT", -15.58, -56.12, []int{10}, map[string]int64{"GASOLINE_REGULAR": 6149, "CNG": 4590}},
		{seoID(6), "POSTO VELHO", "RUA D", "5103403", "MT", -15.60, -56.10, []int{1}, gas(5199)},
		{seoID(7), "POSTO SEM RUA", "", "5103403", "MT", 0, 0, []int{10}, gas(6250)},
		{seoID(8), "POSTO VÁRZEA UM", "AVENIDA DA FEB", "5108402", "MT", -15.65, -56.13, []int{10}, map[string]int64{"GASOLINE_REGULAR": 6159, "ETHANOL": 3899}},
		{seoID(9), "POSTO VÁRZEA DOIS", "AVENIDA DA FEB,", "5108402", "MT", -15.66, -56.14, []int{10}, gas(6259)},
		{seoID(10), "POSTO VÁRZEA TRÊS", "RUA DO PORTO", "5108402", "MT", -15.64, -56.12, []int{10}, gas(6359)},
		{seoID(11), "POSTO SORRISO", "AVENIDA BLUMENAU", "5107925", "MT", -12.54, -55.71, []int{10}, map[string]int64{"GASOLINE_REGULAR": 6499, "DIESEL_S500": 6299}},
		{seoID(12), "POSTO RONDON UM", "AVENIDA LIONS", "5107602", "MT", -16.47, -54.63, []int{6}, gas(5999)},
		{seoID(13), "POSTO RONDON DOIS", "AVENIDA BANDEIRANTES", "5107602", "MT", -16.46, -54.64, []int{6}, gas(6099)},
		{seoID(14), "POSTO PAULISTA UM", "AVENIDA PAULISTA", "3550308", "SP", -23.56, -46.65, []int{10}, map[string]int64{"GASOLINE_REGULAR": 5899, "ETHANOL": 3799}},
		{seoID(15), "POSTO PAULISTA DOIS", "RUA AUGUSTA", "3550308", "SP", -23.55, -46.66, []int{10}, gas(5999)},
		{seoID(16), "POSTO PAULISTA TRÊS", "RUA DA CONSOLAÇÃO", "3550308", "SP", -23.54, -46.65, []int{10}, gas(6099)},
		{seoID(17), "POSTO MINEIRO", "AVENIDA AFONSO PENA", "3106200", "MG", -19.92, -43.94, []int{10}, gas(6199)},
		{seoID(18), "POSTO SEM PESQUISA", "RUA E", "5107602", "MT", 0, 0, nil, nil},
	}
	revisions := make([]enterpriseexport.Revision, 0, 10)
	for week := 1; week <= 10; week++ {
		start := addDays("2026-08-09", 7*(week-1))
		revisions = append(revisions, enterpriseexport.Revision{
			RevisionID: fmt.Sprintf("00000000-0000-4000-8000-%012d", 900+week), SurveyStart: start, SurveyEnd: addDays(start, 6),
			PublishedAt: addDays(start, 7) + "T12:00:00Z", SourceURL: fmt.Sprintf("https://example.test/anp-%02d.xlsx", week),
			SourceChecksum: fmt.Sprintf("sha256:w%02d", week), ParserVersion: "anp-v1",
		})
	}
	stations := make([]enterpriseexport.Station, 0, len(defs))
	prices := []enterpriseexport.Price{}
	for i, def := range defs {
		address := map[string]string{"uf": def.uf, "number": fmt.Sprintf("%d", 10+i), "district": "CENTRO"}
		if def.street != "" {
			address["street"] = def.street
		}
		station := enterpriseexport.Station{
			StationID: def.id, DisplayName: def.name, CNPJ: "", State: def.uf, MunicipalityCode: def.city,
			OperationStatus: "active", LocationQuality: "unknown", Address: address,
		}
		if def.lat != 0 {
			station.Coordinates = &directoryapp.LatLon{Lat: def.lat, Lon: def.lon}
			station.LocationQuality = "reviewed"
		}
		stations = append(stations, station)
		for _, week := range def.weeks {
			revision := revisions[week-1]
			for _, fuel := range sortedKeys(def.fuels) {
				unit, _ := productUnit(fuel)
				prices = append(prices, enterpriseexport.Price{
					PriceID: fmt.Sprintf("p-%02d-%02d-%s", i, week, fuel), RevisionID: revision.RevisionID, StationID: def.id,
					FuelProduct: fuel, Unit: unit, AmountMilliBRL: def.fuels[fuel] - int64(10-week)*10, Currency: "BRL",
					CollectedOn: addDays(revision.SurveyStart, 3),
				})
			}
		}
	}
	return stations, revisions, prices
}

// seoForumPostID numbers the fixture's editorial posts.
func seoForumPostID(n int) string { return fmt.Sprintf("f%07d-0000-4000-8000-%012d", n, 500+n) }

// seoForumFixture is the editorial web projection of the SEO fixture: a
// Cuiabá post with fuel, station and every body block kind, a statewide SP
// post, a Várzea Grande diesel post and a national post with hostile text.
func seoForumFixture() []enterpriseexport.ForumPost {
	return []enterpriseexport.ForumPost{
		{
			PostID: seoForumPostID(1), Category: "FUELS", CategoryName: "Combustíveis",
			CategoryDescription: "Preços, tributação, composição e qualidade de combustíveis.",
			Title:               "Etanol ou gasolina em Cuiabá? A regra dos 70%",
			Excerpt:             "Quando o etanol compensa mais que a gasolina comum em Cuiabá, com a conta passo a passo e os preços da pesquisa ANP.",
			Body:                "O etanol compensa quando custa até 70% da gasolina.\n\nComo fazer a conta\n\n- Divida o preço do etanol pelo da gasolina.\n- Até 0,70: etanol.\n\nVeja o preço de cada posto\nna página da cidade.",
			PublishedAt:         "2026-10-20T13:00:00Z", UpdatedAt: "2026-10-22T09:30:00Z",
			RegionUF: "MT", RegionMunicipality: "5103403", FuelProduct: "ETHANOL", StationID: seoID(1), SourceName: "Equipe Abastevo",
		},
		{
			PostID: seoForumPostID(2), Category: "ELECTRIC", CategoryName: "Eletromobilidade",
			CategoryDescription: "Recarga elétrica, conectores, tarifas e infraestrutura.",
			Title:               "Recarga elétrica nas rodovias paulistas",
			Body:                "Pontos de recarga rápida se espalham pelas rodovias de São Paulo.\n\nO que observar\n\n- Conector e potência.\n- Tarifa por kWh.",
			PublishedAt:         "2026-10-18T10:00:00Z", UpdatedAt: "2026-10-18T10:00:00Z",
			RegionUF: "SP", SourceName: "Equipe Abastevo",
		},
		{
			PostID: seoForumPostID(3), Category: "FUELS", CategoryName: "Combustíveis",
			CategoryDescription: "Preços, tributação, composição e qualidade de combustíveis.",
			Title:               "Diesel S10 em Várzea Grande",
			Body:                "O diesel S10 é o mais procurado por caminhoneiros na região metropolitana de Cuiabá.",
			PublishedAt:         "2026-10-15T08:00:00Z", UpdatedAt: "2026-10-15T08:00:00Z",
			RegionUF: "MT", RegionMunicipality: "5108402", FuelProduct: "DIESEL_S10", SourceName: "Equipe Abastevo",
		},
		{
			PostID: seoForumPostID(4), Category: "ABASTEVO_DATA", CategoryName: "Dados Abastevo",
			CategoryDescription: "Transparência, estatísticas de mercado e análises agregadas.",
			Title:               `Como lemos a pesquisa da ANP <script>alert("x")</script> & "aspas"`,
			Excerpt:             "Como o abastevo lê a pesquisa semanal da ANP: amostra, período e o que um preço publicado significa.",
			Body:                "Texto com <img src=x onerror=alert(1)> e javascript:alert(1) que fica como texto.\n\n</script><b>negrito?</b>",
			PublishedAt:         "2026-10-10T12:00:00Z", UpdatedAt: "2026-10-10T12:00:00Z", SourceName: "Equipe Abastevo",
		},
	}
}

// writeForumSection writes posts as the snapshot's forum section and binds
// them in the manifest, as the export builder does.
func writeForumSection(t *testing.T, dir string, posts []enterpriseexport.ForumPost) {
	t.Helper()
	var manifest enterpriseexport.Manifest
	raw, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	manifest.Sources.Forum = &enterpriseexport.ForumPin{Kind: enterpriseexport.ForumKind, Posts: len(posts)}
	manifest.Counts.ForumPosts = len(posts)
	writeJSON(t, filepath.Join(dir, "manifest.json"), manifest)
	writeJSONL(t, filepath.Join(dir, enterpriseexport.ForumFile), posts)
}

func writeSEOFixture(t *testing.T) string {
	t.Helper()
	dir := writeSEOFixtureWithoutForum(t)
	writeForumSection(t, dir, seoForumFixture())
	return dir
}

func writeSEOFixtureWithoutForum(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	stations, revisions, prices := seoFixture()
	manifest := enterpriseexport.Manifest{
		SchemaVersion: enterpriseexport.SchemaVersion, ContentVersion: enterpriseexport.ContentVersion,
		PublicFieldPolicy: enterpriseexport.PublicFieldPolicy,
		BuiltAt:           "2026-10-24T12:00:00Z", ContentSHA256: "sha256:seo-fixture",
	}
	writeJSON(t, filepath.Join(dir, "manifest.json"), manifest)
	writeJSONL(t, filepath.Join(dir, "stations.jsonl"), stations)
	writeJSONL(t, filepath.Join(dir, "revisions.jsonl"), revisions)
	writeJSONL(t, filepath.Join(dir, "prices.jsonl"), prices)
	projection, err := analytics.Compute(analytics.Input{
		ContentSHA256: "sha256:seo-fixture", Stations: stations, Revisions: revisions, Prices: prices,
	}, analytics.Options{})
	if err != nil {
		t.Fatal(err)
	}
	writeJSON(t, filepath.Join(dir, "analytics-v1.json"), projection)
	return dir
}

// TestWriteSEOFixtureSnapshot exports the SEO fixture snapshot (with the
// official slice) for a local `go run ./cmd/portalgen` sample build:
// ANPFUEL_TEST_SEO_SNAPSHOT_OUT=/tmp/seo-snapshot go test -run TestWriteSEOFixtureSnapshot.
func TestWriteSEOFixtureSnapshot(t *testing.T) {
	target := os.Getenv("ANPFUEL_TEST_SEO_SNAPSHOT_OUT")
	if target == "" {
		t.Skip("ANPFUEL_TEST_SEO_SNAPSHOT_OUT is not set")
	}
	snapshot := writeSEOFixture(t)
	writeOfficialFixture(t, snapshot)
	retargetOfficialFixture(t, snapshot)
	if err := os.MkdirAll(target, 0o700); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		raw, err := os.ReadFile(filepath.Join(snapshot, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(target, entry.Name()), raw, 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

// generateSEOPortal builds the SEO fixture as the production worker does:
// indexable, with the landing on the portal's own origin.
func generateSEOPortal(t *testing.T, indexable bool) string {
	t.Helper()
	snapshot := writeSEOFixture(t)
	writeOfficialFixture(t, snapshot)
	retargetOfficialFixture(t, snapshot)
	out := filepath.Join(t.TempDir(), "portal")
	if _, err := Generate(Options{SnapshotDir: snapshot, OutputDir: out, Indexable: indexable, SiteURL: seoSite, LandingURL: seoSite}); err != nil {
		t.Fatal(err)
	}
	return out
}

// retargetOfficialFixture links the official fixture to an SEO station.
func retargetOfficialFixture(t *testing.T, dir string) {
	t.Helper()
	payload := readPortal(t, dir, "official-records.jsonl")
	payload = strings.ReplaceAll(payload, stA, seoID(1))
	if err := os.WriteFile(filepath.Join(dir, "official-records.jsonl"), []byte(payload), 0o600); err != nil {
		t.Fatal(err)
	}
	manifest := readPortal(t, dir, "official-records-manifest.json")
	sum := digestBytes([]byte(payload))
	manifest = regexp.MustCompile(`"payload_sha256": "[0-9a-f]{64}"`).ReplaceAllString(manifest, `"payload_sha256": "`+strings.TrimPrefix(sum, "sha256:")+`"`)
	if err := os.WriteFile(filepath.Join(dir, "official-records-manifest.json"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
}

type htmlHead struct {
	Robots, Title, Description, Canonical string
}

var (
	headTitle       = regexp.MustCompile(`<title>([^<]*)</title>`)
	headDescription = regexp.MustCompile(`<meta name="description" content="([^"]*)">`)
	headCanonical   = regexp.MustCompile(`<link rel="canonical" href="([^"]*)">`)
)

func parseHead(body string) htmlHead {
	pick := func(re *regexp.Regexp) string {
		if match := re.FindStringSubmatch(body); match != nil {
			return match[1]
		}
		return ""
	}
	return htmlHead{Robots: pick(metaRobots), Title: pick(headTitle), Description: pick(headDescription), Canonical: pick(headCanonical)}
}

// portalPages maps every generated HTML page (relative path) to its body.
func portalPages(t *testing.T, out string) map[string]string {
	t.Helper()
	pages := map[string]string{}
	err := filepath.WalkDir(out, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".html") {
			return err
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(out, path)
		pages[filepath.ToSlash(rel)] = string(raw)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return pages
}
