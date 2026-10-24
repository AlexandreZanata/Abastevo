package portalgen

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/enterpriseexport"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/enterpriseexport/analytics"
)

const (
	stA = "00000000-0000-4000-8000-000000000001"
	stB = "00000000-0000-4000-8000-000000000002"
	stE = "00000000-0000-4000-8000-000000000003"
	stF = "00000000-0000-4000-8000-000000000004"
	stG = "00000000-0000-4000-8000-000000000005"
	stC = "00000000-0000-4000-8000-000000000006"
	stD = "00000000-0000-4000-8000-000000000007"
	r1  = "00000000-0000-4000-8000-000000000011"
	r2  = "00000000-0000-4000-8000-000000000012"
)

func fixture() ([]enterpriseexport.Station, []enterpriseexport.Revision, []enterpriseexport.Price) {
	address := map[string]string{"street": "Rua A", "number": "100", "district": "Centro", "municipio": "São Paulo", "uf": "SP", "postal_code": "01000-000"}
	stations := []enterpriseexport.Station{
		{StationID: stA, DisplayName: "Posto Alfa", CNPJ: "04218406000104", State: "SP", MunicipalityCode: "3550308", OperationStatus: "active", LocationQuality: "reviewed", Address: address},
		{StationID: stB, DisplayName: "Posto Beta", CNPJ: "11222333000181", State: "SP", MunicipalityCode: "3550308", OperationStatus: "active", LocationQuality: "unknown"},
		{StationID: stE, DisplayName: "Posto Gama", CNPJ: "11444777000161", State: "SP", MunicipalityCode: "3550308", OperationStatus: "suspended", LocationQuality: "unknown"},
		{StationID: stF, DisplayName: "Posto Delta", State: "SP", MunicipalityCode: "3550308", OperationStatus: "active", LocationQuality: "unknown"},
		{StationID: stG, DisplayName: "Posto Épsilon", State: "SP", MunicipalityCode: "3550308", OperationStatus: "active", LocationQuality: "unknown"},
		{StationID: stC, DisplayName: "Posto MG", State: "MG", MunicipalityCode: "3106200", OperationStatus: "active", LocationQuality: "unknown"},
		{StationID: stD, DisplayName: "Posto Sem Local", OperationStatus: "active", LocationQuality: "unknown"},
	}
	revisions := []enterpriseexport.Revision{
		{RevisionID: r1, SurveyStart: "2026-10-04", SurveyEnd: "2026-10-10", PublishedAt: "2026-10-11T12:00:00Z", SourceURL: "https://example.test/r1.xlsx", SourceChecksum: "sha256:r1", ParserVersion: "anp-v1"},
		{RevisionID: r2, SurveyStart: "2026-10-11", SurveyEnd: "2026-10-17", PublishedAt: "2026-10-18T12:00:00Z", SourceURL: "https://example.test/r2.xlsx", SourceChecksum: "sha256:r2", ParserVersion: "anp-v1"},
	}
	price := func(id, rev, station, fuel, unit, day string, amount int64) enterpriseexport.Price {
		return enterpriseexport.Price{PriceID: id, RevisionID: rev, StationID: station, FuelProduct: fuel, Unit: unit, AmountMilliBRL: amount, Currency: "BRL", CollectedOn: day}
	}
	prices := []enterpriseexport.Price{
		price("r1-a-gas", r1, stA, "GASOLINE_REGULAR", "L", "2026-10-08", 5899),
		price("r1-b-gas", r1, stB, "GASOLINE_REGULAR", "L", "2026-10-08", 5899),
		price("r1-e-gas", r1, stE, "GASOLINE_REGULAR", "L", "2026-10-08", 5899),
		price("r1-f-gas", r1, stF, "GASOLINE_REGULAR", "L", "2026-10-08", 5999),
		price("r1-g-gas", r1, stG, "GASOLINE_REGULAR", "L", "2026-10-08", 6099),
		price("r1-c-gas", r1, stC, "GASOLINE_REGULAR", "L", "2026-10-08", 6099),
		price("r1-d-gas", r1, stD, "GASOLINE_REGULAR", "L", "2026-10-08", 5799),
		price("r1-a-eth", r1, stA, "ETHANOL", "L", "2026-10-08", 3799),
		price("r1-e-eth", r1, stE, "ETHANOL", "L", "2026-10-08", 3899),
		price("r2-a-gas", r2, stA, "GASOLINE_REGULAR", "L", "2026-10-15", 6199),
		price("r2-b-gas", r2, stB, "GASOLINE_REGULAR", "L", "2026-10-15", 6199),
		price("r2-e-gas", r2, stE, "GASOLINE_REGULAR", "L", "2026-10-15", 6199),
		price("r2-f-gas", r2, stF, "GASOLINE_REGULAR", "L", "2026-10-15", 6299),
		price("r2-g-gas", r2, stG, "GASOLINE_REGULAR", "L", "2026-10-15", 6399),
		price("r2-c-gas", r2, stC, "GASOLINE_REGULAR", "L", "2026-10-15", 6299),
		price("r2-d-gas", r2, stD, "GASOLINE_REGULAR", "L", "2026-10-15", 6099),
		price("r2-e-eth", r2, stE, "ETHANOL", "L", "2026-10-15", 3999),
	}
	return stations, revisions, prices
}

func writeFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	stations, revisions, prices := fixture()
	manifest := enterpriseexport.Manifest{
		SchemaVersion: enterpriseexport.SchemaVersion, ContentVersion: enterpriseexport.ContentVersion,
		PublicFieldPolicy: enterpriseexport.PublicFieldPolicy,
		BuiltAt:           "2026-10-24T12:00:00Z", ContentSHA256: "sha256:fixture",
	}
	writeJSON(t, filepath.Join(dir, "manifest.json"), manifest)
	writeJSONL(t, filepath.Join(dir, "stations.jsonl"), stations)
	writeJSONL(t, filepath.Join(dir, "revisions.jsonl"), revisions)
	writeJSONL(t, filepath.Join(dir, "prices.jsonl"), prices)
	projection, err := analytics.Compute(analytics.Input{
		ContentSHA256: "sha256:fixture", Stations: stations, Revisions: revisions, Prices: prices,
	}, analytics.Options{})
	if err != nil {
		t.Fatal(err)
	}
	writeJSON(t, filepath.Join(dir, "analytics-v1.json"), projection)
	return dir
}

func writeJSON(t *testing.T, path string, value any) {
	t.Helper()
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(raw, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeJSONL(t *testing.T, path string, value any) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	encoder := json.NewEncoder(file)
	switch rows := value.(type) {
	case []enterpriseexport.Station:
		for _, row := range rows {
			if err := encoder.Encode(row); err != nil {
				t.Fatal(err)
			}
		}
	case []enterpriseexport.Revision:
		for _, row := range rows {
			if err := encoder.Encode(row); err != nil {
				t.Fatal(err)
			}
		}
	case []enterpriseexport.Price:
		for _, row := range rows {
			if err := encoder.Encode(row); err != nil {
				t.Fatal(err)
			}
		}
	default:
		t.Fatalf("unsupported fixture type")
	}
}

func readPortal(t *testing.T, dir, rel string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, rel))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestGenerateFixturePages(t *testing.T) {
	snapshot := writeFixture(t)
	out := filepath.Join(t.TempDir(), "portal")
	result, err := Generate(Options{SnapshotDir: snapshot, OutputDir: out})
	if err != nil {
		t.Fatal(err)
	}
	if result.Pages != 16 || result.Files != 22 {
		t.Fatalf("result = %+v, want 16 pages and 22 files", result)
	}
	for _, page := range []string{
		"consultar/index.html", "precos/index.html", "precos/uf/SP/index.html",
		"precos/uf/SP/municipio/3550308/index.html",
		"precos/uf/MG/municipio/3106200/index.html",
		"postos/" + stA + "/index.html", "postos/" + stD + "/index.html",
		"metodologia/index.html", "fiscalizacao/index.html", "404.html",
		"assets/portal.css", "robots.txt", "portal-manifest.json",
		"dados/busca/fixture/SP.json", "dados/busca/fixture/MG.json",
		"dados/busca/fixture/sem-uf.json",
	} {
		if _, err := os.Stat(filepath.Join(out, page)); err != nil {
			t.Errorf("missing %s: %v", page, err)
		}
	}
	consultar := readPortal(t, out, "consultar/index.html")
	for _, want := range []string{
		"São Paulo (SP)", "Minas Gerais (MG)", "5 postos", "1 posto",
		`data-portal-search`, `data-release="fixture"`, `data-shard-base="/dados/busca/"`,
		`<option value="SP">São Paulo</option>`, `<option value="sem-uf">Sem UF</option>`,
		`data-search-query`, `data-search-status`,
	} {
		if !strings.Contains(consultar, want) {
			t.Errorf("consultar missing %q", want)
		}
	}
	shardRaw, err := os.ReadFile(filepath.Join(out, "dados/busca/fixture/SP.json"))
	if err != nil {
		t.Fatal(err)
	}
	var sp searchShard
	if err := json.Unmarshal(shardRaw, &sp); err != nil {
		t.Fatal(err)
	}
	if sp.Schema != SearchSchema || sp.Release != "fixture" || sp.UF != "SP" || sp.Count != 5 || len(sp.Stations) != 5 {
		t.Errorf("SP shard = %+v", sp)
	}
	foundA, foundE, foundG := false, false, false
	for _, row := range sp.Stations {
		if row.ID == stG {
			foundG = true
			if row.SearchName != "posto epsilon" {
				t.Errorf("accent normalization missing: %+v", row)
			}
		}
		if row.ID == stA {
			foundA = true
			if row.SearchName != "posto alfa" || row.CNPJ != "04218406000104" ||
				row.Prices["GASOLINE_REGULAR"] != 6199 {
				t.Errorf("station A shard row = %+v", row)
			}
		}
		if row.ID == stE {
			foundE = true
			if !strings.Contains(row.StatusLabel, "Suspenso") {
				t.Errorf("suspended shard row = %+v", row)
			}
		}
	}
	if !foundA || !foundE || !foundG {
		t.Errorf("SP shard missing fixture stations")
	}
	if _, err := os.Stat(filepath.Join(out, "dados/busca/fixture/epsilon.json")); err == nil {
		t.Error("unexpected national/extra shard")
	}
	if _, err := os.Stat(filepath.Join(out, "dados/busca/fixture/all.json")); err == nil {
		t.Error("national shard must not be generated")
	}
	precos := readPortal(t, out, "precos/index.html")
	for _, want := range []string{"R$ 6,099", "R$ 6,399", "R$ 0,300", "Menores preços encontrados", "7 postos"} {
		if !strings.Contains(precos, want) {
			t.Errorf("precos missing %q", want)
		}
	}
	state := readPortal(t, out, "precos/uf/SP/index.html")
	for _, want := range []string{"R$ 6,199", "R$ 6,399", "São Paulo", "5 postos"} {
		if !strings.Contains(state, want) {
			t.Errorf("state missing %q", want)
		}
	}
	municipality := readPortal(t, out, "precos/uf/SP/municipio/3550308/index.html")
	for _, want := range []string{"5 postos", "Menores preços encontrados", "São Paulo"} {
		if !strings.Contains(municipality, want) {
			t.Errorf("municipality missing %q", want)
		}
	}
	insufficient := readPortal(t, out, "precos/uf/MG/municipio/3106200/index.html")
	if !strings.Contains(insufficient, "Amostra insuficiente") || !strings.Contains(insufficient, "sem ranking") {
		t.Errorf("insufficient municipality state missing")
	}
	station := readPortal(t, out, "postos/"+stA+"/index.html")
	for _, want := range []string{
		"Posto Alfa", "04.218.406/0001-04", "Ativo (informado pela fonte)",
		"R$ 6,199", "R$ 5,899", "04/10/2026 a 10/10/2026",
		"11/10/2026 a 17/10/2026", "Sem observação nesta pesquisa",
		"Identificador canônico: " + stA, "Fonte:</strong> ANP", "checksum: sha256:r2",
	} {
		if !strings.Contains(station, want) {
			t.Errorf("station missing %q", want)
		}
	}
	suspended := readPortal(t, out, "postos/"+stE+"/index.html")
	if !strings.Contains(suspended, "Suspenso (informado pela fonte)") ||
		!strings.Contains(suspended, "R$ 3,999") {
		t.Errorf("suspended station state missing")
	}
	unlocated := readPortal(t, out, "postos/"+stD+"/index.html")
	if !strings.Contains(unlocated, "Localização não informada") {
		t.Errorf("unlocated state missing")
	}
	metodologia := readPortal(t, out, "metodologia/index.html")
	for _, want := range []string{"precisão", "Fonte", "abastevo-metrics-v1", "Snapshot: sha256:fixture"} {
		if !strings.Contains(metodologia, want) {
			t.Errorf("metodologia missing %q", want)
		}
	}
	fiscalizacao := readPortal(t, out, "fiscalizacao/index.html")
	if !strings.Contains(fiscalizacao, "ainda não disponível") || strings.Contains(fiscalizacao, "<table") {
		t.Errorf("fiscalizacao gate failed")
	}
	robots := readPortal(t, out, "robots.txt")
	if !strings.Contains(robots, "Disallow: /") {
		t.Errorf("preview robots must disallow indexing")
	}
	for _, page := range []string{
		"consultar/index.html", "precos/index.html", "precos/uf/SP/index.html",
		"precos/uf/SP/municipio/3550308/index.html", "postos/" + stA + "/index.html",
	} {
		html := readPortal(t, out, page)
		if strings.Contains(html, "<script") || strings.Contains(html, "/v1/") ||
			strings.Contains(html, "fetch(") || strings.Contains(html, "mais barato") ||
			strings.Contains(html, "preço atual") {
			t.Errorf("%s contains a forbidden call/claim", page)
		}
	}
	var manifest Manifest
	if err := json.Unmarshal([]byte(readPortal(t, out, "portal-manifest.json")), &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.InputContentSHA256 != "sha256:fixture" || manifest.Pages != result.Pages ||
		manifest.ContentSHA256 != result.ContentSHA256 || len(manifest.Files) != 21 {
		t.Errorf("portal manifest = %+v", manifest)
	}
}

func TestGenerateIsDeterministic(t *testing.T) {
	snapshot := writeFixture(t)
	firstDir := filepath.Join(t.TempDir(), "first")
	secondDir := filepath.Join(t.TempDir(), "second")
	first, err := Generate(Options{SnapshotDir: snapshot, OutputDir: firstDir})
	if err != nil {
		t.Fatal(err)
	}
	second, err := Generate(Options{SnapshotDir: snapshot, OutputDir: secondDir})
	if err != nil {
		t.Fatal(err)
	}
	if first.ContentSHA256 != second.ContentSHA256 || first.Pages != second.Pages || first.Bytes != second.Bytes {
		t.Errorf("non-deterministic result: %+v vs %+v", first, second)
	}
	if readPortal(t, firstDir, "portal-manifest.json") != readPortal(t, secondDir, "portal-manifest.json") {
		t.Error("portal manifest differs between identical runs")
	}
	if readPortal(t, firstDir, "postos/"+stA+"/index.html") != readPortal(t, secondDir, "postos/"+stA+"/index.html") {
		t.Error("station page differs between identical runs")
	}
}

func TestGenerateRejectsStaleAnalyticsAndBudgets(t *testing.T) {
	t.Run("stale analytics", func(t *testing.T) {
		snapshot := writeFixture(t)
		path := filepath.Join(snapshot, "analytics-v1.json")
		var projection analytics.Projection
		if err := json.Unmarshal([]byte(readPortal(t, snapshot, "analytics-v1.json")), &projection); err != nil {
			t.Fatal(err)
		}
		projection.InputContentSHA256 = "sha256:other"
		writeJSON(t, path, projection)
		if _, err := Generate(Options{SnapshotDir: snapshot, OutputDir: filepath.Join(t.TempDir(), "out")}); err == nil {
			t.Fatal("stale analytics accepted")
		}
	})
	t.Run("page budget", func(t *testing.T) {
		snapshot := writeFixture(t)
		out := filepath.Join(t.TempDir(), "out")
		if _, err := Generate(Options{SnapshotDir: snapshot, OutputDir: out, MaxPages: 1}); err == nil {
			t.Fatal("page budget not enforced")
		}
		if _, err := os.Stat(out); !os.IsNotExist(err) {
			t.Error("failed generation left an output directory")
		}
	})
	t.Run("existing output", func(t *testing.T) {
		snapshot := writeFixture(t)
		out := filepath.Join(t.TempDir(), "out")
		if err := os.Mkdir(out, 0o700); err != nil {
			t.Fatal(err)
		}
		if _, err := Generate(Options{SnapshotDir: snapshot, OutputDir: out}); err == nil {
			t.Fatal("existing output accepted")
		}
	})
	t.Run("census divergence", func(t *testing.T) {
		snapshot := writeFixture(t)
		path := filepath.Join(snapshot, "prices.jsonl")
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		lines := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
		if err := os.WriteFile(path, []byte(strings.Join(lines[:len(lines)-1], "\n")+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := Generate(Options{SnapshotDir: snapshot, OutputDir: filepath.Join(t.TempDir(), "out")}); err == nil {
			t.Fatal("census divergence accepted")
		}
	})
	t.Run("unsafe station id", func(t *testing.T) {
		snapshot := writeFixture(t)
		stations, revisions, prices := fixture()
		stations[0].StationID = "../../escape"
		writeJSONL(t, filepath.Join(snapshot, "stations.jsonl"), stations)
		writeJSONL(t, filepath.Join(snapshot, "revisions.jsonl"), revisions)
		writeJSONL(t, filepath.Join(snapshot, "prices.jsonl"), prices)
		if _, err := Generate(Options{SnapshotDir: snapshot, OutputDir: filepath.Join(t.TempDir(), "out")}); err == nil {
			t.Fatal("unsafe station id accepted")
		}
	})
}

func TestGeneratedLinksResolve(t *testing.T) {
	snapshot := writeFixture(t)
	out := filepath.Join(t.TempDir(), "portal")
	if _, err := Generate(Options{SnapshotDir: snapshot, OutputDir: out}); err != nil {
		t.Fatal(err)
	}
	linkPattern := regexp.MustCompile(`(?:href|src)="(/[^"#?]*)"`)
	err := filepath.WalkDir(out, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".html") {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, match := range linkPattern.FindAllStringSubmatch(string(raw), -1) {
			target := match[1]
			full := filepath.Join(out, strings.TrimPrefix(target, "/"))
			if strings.HasSuffix(target, "/") {
				full = filepath.Join(full, "index.html")
			}
			if _, err := os.Stat(full); err != nil {
				t.Errorf("%s links to missing %s", filepath.Base(filepath.Dir(path)), target)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestSearchScriptIsEmbeddedAndReferenced(t *testing.T) {
	snapshot := writeFixture(t)
	out := filepath.Join(t.TempDir(), "portal")
	expected := "__portalSearch"
	script := filepath.Join(t.TempDir(), "search.js")
	if real := os.Getenv("ANPFUEL_TEST_REAL_SEARCH_JS"); real != "" {
		script = real
		expected = "setupPortalSearch"
	} else if err := os.WriteFile(script, []byte("globalThis.__portalSearch = true;\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Generate(Options{SnapshotDir: snapshot, OutputDir: out, SearchScriptPath: script}); err != nil {
		t.Fatal(err)
	}
	consultar := readPortal(t, out, "consultar/index.html")
	pattern := regexp.MustCompile(`<script type="module" src="(/assets/portal\.[a-f0-9]{8}\.js)" defer></script>`)
	match := pattern.FindStringSubmatch(consultar)
	if len(match) != 2 {
		t.Fatalf("search script reference missing")
	}
	asset, err := os.ReadFile(filepath.Join(out, strings.TrimPrefix(match[1], "/")))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(asset), expected) {
		t.Errorf("embedded search script content mismatch")
	}
	var manifest Manifest
	if err := json.Unmarshal([]byte(readPortal(t, out, "portal-manifest.json")), &manifest); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, file := range manifest.Files {
		if "/"+file.Path == match[1] {
			found = true
		}
	}
	if !found {
		t.Errorf("search script missing from portal manifest")
	}
}

func TestRegionalSearchShardIsBounded(t *testing.T) {
	stations := make([]enterpriseexport.Station, 0, 6000)
	for i := 0; i < 6000; i++ {
		uf := "SP"
		if i >= 2000 {
			uf = "MG"
		}
		if i >= 4000 {
			uf = ""
		}
		stations = append(stations, enterpriseexport.Station{
			StationID:        "00000000-0000-4000-8000-" + fmt.Sprintf("%012d", i),
			DisplayName:      fmt.Sprintf("Posto %05d", i),
			CNPJ:             fmt.Sprintf("%014d", i),
			State:            uf,
			MunicipalityCode: fmt.Sprintf("%07d", 3550308+i%100),
			Address:          map[string]string{"municipio": fmt.Sprintf("Cidade %02d", i%50)},
		})
	}
	shards, err := buildSearchShards(enterpriseexport.Snapshot{Stations: stations}, nil, "rel")
	if err != nil {
		t.Fatal(err)
	}
	var sp []byte
	all := make([]searchStation, 0, len(stations))
	for _, shard := range shards {
		if shard.UF == "SP" {
			sp = shard.Data
		}
		var decoded searchShard
		if err := json.Unmarshal(shard.Data, &decoded); err != nil {
			t.Fatal(err)
		}
		all = append(all, decoded.Stations...)
	}
	if len(sp) == 0 {
		t.Fatal("SP shard missing")
	}
	national, err := json.Marshal(searchShard{
		Schema: SearchSchema, Release: "rel", UF: "all", Count: len(all), Stations: all,
	})
	if err != nil {
		t.Fatal(err)
	}
	spGzip := gzipLen(sp)
	nationalGzip := gzipLen(national)
	t.Logf("regional SP bytes=%d gzip=%d; national bytes=%d gzip=%d",
		len(sp), spGzip, len(national), nationalGzip)
	if len(sp) >= len(national) || spGzip >= nationalGzip {
		t.Errorf("regional shard is not smaller than the national locator")
	}
	if spGzip > 250*1024 {
		t.Errorf("regional SP gzip shard exceeds 250 KiB: %d", spGzip)
	}
}

func gzipLen(data []byte) int {
	var buf bytes.Buffer
	writer := gzip.NewWriter(&buf)
	_, _ = writer.Write(data)
	_ = writer.Close()
	return buf.Len()
}
