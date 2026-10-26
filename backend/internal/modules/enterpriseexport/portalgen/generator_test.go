package portalgen

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
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
	stH = "00000000-0000-4000-8000-000000000008"
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
	return writeFixtureWith(t, nil)
}

func writeFixtureWith(t *testing.T, mutate func(*[]enterpriseexport.Station, *[]enterpriseexport.Price)) string {
	t.Helper()
	dir := t.TempDir()
	stations, revisions, prices := fixture()
	if mutate != nil {
		mutate(&stations, &prices)
	}
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
	if result.Pages != 18 || result.Files != 28 {
		t.Fatalf("result = %+v, want 18 pages and 28 files", result)
	}
	for _, page := range []string{
		"consultar/index.html", "precos/index.html", "precos/uf/SP/index.html",
		"precos/uf/SP/municipio/3550308/index.html",
		"precos/uf/MG/municipio/3106200/index.html",
		"postos/" + stA + "/index.html", "postos/" + stD + "/index.html",
		"metodologia/index.html", "fiscalizacao/index.html", "404.html",
		"curiosidades/index.html", "dados/index.html",
		"robots.txt", "portal-manifest.json",
		"dados/busca/fixture/SP.json", "dados/busca/fixture/MG.json",
		"dados/busca/fixture/sem-uf.json",
		"dados/fixture/estacoes.csv", "dados/fixture/precos.csv",
		"dados/fixture/resumos.csv", "dados/fixture/manifest.json",
	} {
		if _, err := os.Stat(filepath.Join(out, page)); err != nil {
			t.Errorf("missing %s: %v", page, err)
		}
	}
	cssFiles, err := filepath.Glob(filepath.Join(out, "assets", "portal.*.css"))
	if err != nil || len(cssFiles) != 1 {
		t.Fatalf("fingerprinted CSS = %v %v", cssFiles, err)
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
	curiosidades := readPortal(t, out, "curiosidades/index.html")
	for _, want := range []string{
		"Perguntas aprovadas sobre os dados", "Quantos postos tinham preço",
		"A pesquisa de 11/10/2026 a 17/10/2026 tem 7 postos",
		"aumentou R$ 0,300", "ficou igual", "1 de 2 municípios",
		"Nenhuma observação da revisão foi descartada",
	} {
		if !strings.Contains(curiosidades, want) {
			t.Errorf("curiosidades missing %q", want)
		}
	}
	if strings.Contains(curiosidades, "mais barato") || strings.Contains(curiosidades, "preço atual") {
		t.Errorf("curiosidades contains a prohibited claim")
	}
	dados := readPortal(t, out, "dados/index.html")
	for _, want := range []string{
		"Baixar dados públicos", "/dados/fixture/estacoes.csv",
		"/dados/fixture/precos.csv", "/dados/fixture/resumos.csv",
		"/dados/fixture/manifest.json", "UTF-8", "direitos",
	} {
		if !strings.Contains(dados, want) {
			t.Errorf("dados missing %q", want)
		}
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
		"metodologia/index.html", "404.html",
	} {
		html := readPortal(t, out, page)
		if !strings.Contains(html, "/assets/portal.") || !strings.Contains(html, ".css") {
			t.Errorf("%s lacks the shared stylesheet", page)
		}
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
		manifest.ContentSHA256 != result.ContentSHA256 || len(manifest.Files) != 27 {
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
	checkPortalLinks(t, out)
}

func checkPortalLinks(t *testing.T, out string) {
	t.Helper()
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
	for _, page := range []string{"consultar/index.html", "metodologia/index.html", "404.html"} {
		if !strings.Contains(readPortal(t, out, page), match[1]) {
			t.Errorf("%s does not reference the search script", page)
		}
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
	if int64(spGzip) > DefaultMaxShardGzip {
		t.Errorf("regional SP gzip shard exceeds the shard budget: %d", spGzip)
	}
}

func gzipLen(data []byte) int {
	var buf bytes.Buffer
	writer := gzip.NewWriter(&buf)
	_, _ = writer.Write(data)
	_ = writer.Close()
	return buf.Len()
}

func TestDownloadsCensusAndCSVSafety(t *testing.T) {
	snapshot := writeFixture(t)
	out := filepath.Join(t.TempDir(), "portal")
	if _, err := Generate(Options{SnapshotDir: snapshot, OutputDir: out}); err != nil {
		t.Fatal(err)
	}
	parse := func(name string) [][]string {
		raw, err := os.ReadFile(filepath.Join(out, "dados/fixture", name))
		if err != nil {
			t.Fatal(err)
		}
		rows, err := csv.NewReader(strings.NewReader(string(raw))).ReadAll()
		if err != nil {
			t.Fatal(err)
		}
		return rows
	}
	stations := parse("estacoes.csv")
	prices := parse("precos.csv")
	summaries := parse("resumos.csv")
	if len(stations) != 8 || len(prices) != 9 || len(summaries) != 9 {
		t.Errorf("download census = %d/%d/%d, want 8/9/9 including headers", len(stations), len(prices), len(summaries))
	}
	var manifest downloadManifest
	if err := json.Unmarshal([]byte(readPortal(t, out, "dados/fixture/manifest.json")), &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Schema != DownloadsSchema || manifest.InputContentSHA256 != "sha256:fixture" ||
		manifest.Release != "fixture" || len(manifest.Files) != 3 {
		t.Errorf("download manifest = %+v", manifest)
	}
	wantRows := map[string]int{"estacoes.csv": 7, "precos.csv": 8, "resumos.csv": 8}
	for _, file := range manifest.Files {
		if file.Rows != wantRows[file.Name] || file.SHA256 == "" || file.Bytes == 0 {
			t.Errorf("download file census = %+v", file)
		}
	}
	for _, value := range []string{"=cmd", "+1", "-2", "@x", "\tfoo", "a\rb", "a\nb", "normal"} {
		got := csvSafe(value)
		if strings.ContainsAny(got, "\r\n") {
			t.Errorf("csvSafe left newline in %q", got)
		}
		if strings.Contains("=+-@\t", string(value[0])) && !strings.HasPrefix(got, "'") {
			t.Errorf("csvSafe did not neutralize %q -> %q", value, got)
		}
	}
}

func TestInsightsExplainSampleChange(t *testing.T) {
	snapshot := writeFixtureWith(t, func(stations *[]enterpriseexport.Station, prices *[]enterpriseexport.Price) {
		*stations = append(*stations, enterpriseexport.Station{
			StationID: stH, DisplayName: "Posto Eta", CNPJ: "11444777000161",
			State: "SP", MunicipalityCode: "3550308", OperationStatus: "active", LocationQuality: "unknown",
		})
		for i := range *prices {
			if (*prices)[i].RevisionID == r2 && (*prices)[i].StationID == stD {
				(*prices)[i].StationID = stH
			}
		}
	})
	out := filepath.Join(t.TempDir(), "portal")
	if _, err := Generate(Options{SnapshotDir: snapshot, OutputDir: out}); err != nil {
		t.Fatal(err)
	}
	page := readPortal(t, out, "curiosidades/index.html")
	if !strings.Contains(page, "A amostra de postos mudou") ||
		!strings.Contains(page, "não calculamos variação") {
		t.Errorf("sample-change explanation missing")
	}
	if strings.Contains(page, "A amostra de postos foi a mesma") {
		t.Errorf("sample change was compared as if unchanged")
	}
}

func writeOfficialFixture(t *testing.T, dir string) {
	t.Helper()
	revision := "00000000-0000-4000-8000-0000000000f1"
	lease := strings.Repeat("b", 64)
	checksum := strings.Repeat("a", 64)
	sourceURL := "https://www.gov.br/anp/pt-br/centrais-de-conteudo/dados-abertos/arquivos/mav/multas-aplicadas-2025.csv"
	amount := func(value int64) *int64 { return &value }
	year := func(value int32) *int32 { return &value }
	records := []officialRecord{
		{
			NativeKey: "f1", SubjectCNPJ: "04218406000104", Scope: "sanction",
			Status: "Multa aplicada", Evidence: "fines.csv:2", StationID: stA,
			AssociationState: "linked", AssociationReason: "exact current cnpj; historical business window unknown",
			RevisionID: revision, LeaseID: lease, SourceURL: sourceURL, SourceChecksum: checksum,
			ProceedingRef: "48000.1/2025-11", AmountMinor: amount(150000), AmountText: "1.500,00",
			DecidedOn: "2025-01-15", DueOn: "2025-02-28", AsOf: "2026-01-31",
			SourceContract: "anp-sicom-cp850-9-v1", SourcePeriod: "2025",
			ExtractionMonth: "2026-01", ReportYear: year(2025),
		},
		{
			NativeKey: "f2", SubjectCNPJ: "11222333000181", Scope: "sanction",
			Status: "Multa aplicada", Evidence: "fines.csv:3",
			AssociationState: "unmatched", AssociationReason: "no current station",
			RevisionID: revision, LeaseID: lease, SourceURL: sourceURL, SourceChecksum: checksum,
			ProceedingRef: "48000.2/2025-12", AmountMinor: amount(30000), AmountText: "300,00",
			DecidedOn: "2025-02-10", DueOn: "2025-03-31", AsOf: "2026-01-31",
			SourceContract: "anp-sicom-cp850-9-v1", SourcePeriod: "2025",
			ExtractionMonth: "2026-01", ReportYear: year(2025),
		},
		{
			NativeKey: "f3", SubjectCNPJ: "11444777000161", Scope: "sanction",
			Status: "Multa aplicada", Evidence: "fines.csv:4",
			AssociationState: "ambiguous", AssociationReason: "multiple current stations",
			RevisionID: revision, LeaseID: lease, SourceURL: sourceURL, SourceChecksum: checksum,
			ProceedingRef: "48000.3/2025-13", AmountMinor: amount(5000), AmountText: "50,00",
			DecidedOn: "2025-03-12", DueOn: "2025-04-30", AsOf: "2026-01-31",
			SourceContract: "anp-sicom-cp850-9-v1", SourcePeriod: "2025",
			ExtractionMonth: "2026-01", ReportYear: year(2025),
		},
	}
	var payload bytes.Buffer
	encoder := json.NewEncoder(&payload)
	encoder.SetEscapeHTML(false)
	for _, record := range records {
		if err := encoder.Encode(record); err != nil {
			t.Fatal(err)
		}
	}
	manifest := officialManifest{
		SchemaVersion: officialSchema, PublicFieldPolicy: "fq-public-v1",
		Source: "sanctions", Stream: "sanction-v1", Edition: "2025",
		RevisionID: revision, LeaseID: lease, PublishedAt: "2026-10-24T21:55:16Z",
		Status: "published", ProvenanceState: "VERIFIED",
		SourceURL: sourceURL, SourceChecksum: checksum, SourceBytes: 450504,
		SourcePeriod: "2025", SourceClockPrecision: "MONTH", SourceClockValue: "2026-01",
		Counts:            officialCounts{Input: 3, Accepted: 3, Duplicates: 0, Quarantined: 0},
		AssociationPolicy: "current-company-v1", AssociationScope: "CURRENT_COMPANY_ONLY",
		AssociationDecisionDay: "2026-10-20",
		AssociationCensus:      officialCensus{Total: 3, Linked: 1, Unmatched: 1, Ambiguous: 1},
		Rows:                   3, PayloadSHA256: hex.EncodeToString(func() []byte { sum := sha256.Sum256(payload.Bytes()); return sum[:] }()),
	}
	if err := os.WriteFile(filepath.Join(dir, "official-records.jsonl"), payload.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	writeJSON(t, filepath.Join(dir, "official-records-manifest.json"), manifest)
}

func TestOfficialRecordsPages(t *testing.T) {
	snapshot := writeFixture(t)
	writeOfficialFixture(t, snapshot)
	out := filepath.Join(t.TempDir(), "portal")
	result, err := Generate(Options{SnapshotDir: snapshot, OutputDir: out})
	if err != nil {
		t.Fatal(err)
	}
	if result.Pages != 21 || result.Files != 31 {
		t.Fatalf("result = %+v, want 21 pages and 31 files", result)
	}
	index := readPortal(t, out, "fiscalizacao/index.html")
	indexLower := strings.ToLower(index)
	for _, want := range []string{
		"Fiscalização — multas aplicadas", "Edição 2025", "1",
		"não prova adulteração",
		"não é certidão negativa", "48000.1/2025-11", "48000.2/2025-12",
		"48000.3/2025-13", "R$ 1.500,00",
	} {
		if !strings.Contains(index, want) {
			t.Errorf("fiscalizacao index missing %q", want)
		}
	}
	for _, want := range []string{"sem vínculo", "ambíguos"} {
		if !strings.Contains(indexLower, want) {
			t.Errorf("fiscalizacao index missing %q", want)
		}
	}
	linked := readPortal(t, out, "fiscalizacao/registro/"+officialRecordID("f1")+"/index.html")
	for _, want := range []string{
		"Posto Alfa", "/postos/" + stA + "/", "Vínculo de empresa atual",
		"não é histórico de propriedade", "R$ 1.500,00",
		"não é a data da ocorrência",
	} {
		if !strings.Contains(linked, want) {
			t.Errorf("linked record missing %q", want)
		}
	}
	unmatched := readPortal(t, out, "fiscalizacao/registro/"+officialRecordID("f2")+"/index.html")
	if !strings.Contains(unmatched, "Sem vínculo de posto") || strings.Contains(unmatched, "/postos/") {
		t.Errorf("unmatched record leaked a station")
	}
	ambiguous := readPortal(t, out, "fiscalizacao/registro/"+officialRecordID("f3")+"/index.html")
	if !strings.Contains(ambiguous, "Vínculo ambíguo") || strings.Contains(ambiguous, "/postos/") {
		t.Errorf("ambiguous record leaked a station")
	}
	station := readPortal(t, out, "postos/"+stA+"/index.html")
	if !strings.Contains(station, "Registros oficiais vinculados") || !strings.Contains(station, "48000.1/2025-11") {
		t.Errorf("linked station section missing")
	}
	stationB := readPortal(t, out, "postos/"+stB+"/index.html")
	if strings.Contains(stationB, "Registros oficiais vinculados") {
		t.Errorf("unmatched station section must be absent")
	}
	var manifest Manifest
	if err := json.Unmarshal([]byte(readPortal(t, out, "portal-manifest.json")), &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.OfficialRecords == nil || manifest.OfficialRecords.Rows != 3 ||
		manifest.OfficialRecords.RevisionID != "00000000-0000-4000-8000-0000000000f1" ||
		manifest.InputContentSHA256 == "sha256:fixture" {
		t.Errorf("portal manifest official pin = %+v", manifest.OfficialRecords)
	}
	if strings.Contains(index, "CPF") && !strings.Contains(index, "Nenhum CPF") && !strings.Contains(index, "não") {
		t.Errorf("unexpected CPF claim")
	}
	checkPortalLinks(t, out)
}

func TestOfficialIntegrityFailures(t *testing.T) {
	t.Run("payload hash", func(t *testing.T) {
		snapshot := writeFixture(t)
		writeOfficialFixture(t, snapshot)
		var manifest officialManifest
		if err := json.Unmarshal([]byte(readPortal(t, snapshot, "official-records-manifest.json")), &manifest); err != nil {
			t.Fatal(err)
		}
		manifest.PayloadSHA256 = strings.Repeat("0", 64)
		writeJSON(t, filepath.Join(snapshot, "official-records-manifest.json"), manifest)
		if _, err := Generate(Options{SnapshotDir: snapshot, OutputDir: filepath.Join(t.TempDir(), "out")}); err == nil {
			t.Fatal("payload mismatch accepted")
		}
	})
	t.Run("linked station missing", func(t *testing.T) {
		snapshot := writeFixture(t)
		writeOfficialFixture(t, snapshot)
		raw, err := os.ReadFile(filepath.Join(snapshot, "official-records.jsonl"))
		if err != nil {
			t.Fatal(err)
		}
		raw = bytes.Replace(raw, []byte(stA), []byte("00000000-0000-4000-8000-0000000000ff"), 1)
		var manifest officialManifest
		if err := json.Unmarshal([]byte(readPortal(t, snapshot, "official-records-manifest.json")), &manifest); err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(raw)
		manifest.PayloadSHA256 = hex.EncodeToString(sum[:])
		if err := os.WriteFile(filepath.Join(snapshot, "official-records.jsonl"), raw, 0o600); err != nil {
			t.Fatal(err)
		}
		writeJSON(t, filepath.Join(snapshot, "official-records-manifest.json"), manifest)
		if _, err := Generate(Options{SnapshotDir: snapshot, OutputDir: filepath.Join(t.TempDir(), "out")}); err == nil {
			t.Fatal("linked station outside snapshot accepted")
		}
	})
}

func TestRealOfficialSnapshotPages(t *testing.T) {
	dir := os.Getenv("ANPFUEL_TEST_REAL_OFFICIAL_DIR")
	if dir == "" {
		t.Skip("ANPFUEL_TEST_REAL_OFFICIAL_DIR is not set")
	}
	snapshot := writeFixture(t)
	payload, err := os.ReadFile(filepath.Join(dir, "official-records-2025.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	manifestRaw, err := os.ReadFile(filepath.Join(dir, "official-records-2025.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(snapshot, "official-records.jsonl"), payload, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(snapshot, "official-records-manifest.json"), manifestRaw, 0o600); err != nil {
		t.Fatal(err)
	}
	var manifest officialManifest
	if err := json.Unmarshal(manifestRaw, &manifest); err != nil {
		t.Fatal(err)
	}
	out := os.Getenv("ANPFUEL_TEST_PORTAL_OUT")
	if out == "" {
		out = filepath.Join(t.TempDir(), "portal-real-official")
	}
	indexable := os.Getenv("ANPFUEL_TEST_PORTAL_INDEXABLE") == "1"
	siteURL := os.Getenv("ANPFUEL_TEST_PORTAL_SITE_URL")
	if siteURL == "" {
		siteURL = "https://portal.example"
	}
	searchJS := os.Getenv("ANPFUEL_TEST_PORTAL_SEARCH_JS")
	result, err := Generate(Options{SnapshotDir: snapshot, OutputDir: out, Indexable: indexable, SiteURL: siteURL, SearchScriptPath: searchJS})
	if err != nil {
		t.Fatal(err)
	}
	pages := (manifest.Rows + officialPageSize - 1) / officialPageSize
	wantPages := 18 + (pages - 1) + manifest.Rows
	wantFiles := 28 + (pages - 1) + manifest.Rows
	if indexable {
		wantFiles++
	}
	if searchJS != "" {
		wantFiles++
	}
	if result.Pages != wantPages || result.Files != wantFiles {
		t.Fatalf("real official result = %+v, want %d pages/%d files", result, wantPages, wantFiles)
	}
	t.Logf("real FQ-07 2025 portal: rows=%d pages=%d files=%d bytes=%d content=%s",
		manifest.Rows, result.Pages, result.Files, result.Bytes, result.ContentSHA256)
	verified, err := Verify(out, VerifyOptions{SiteURL: siteURL})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("real FQ-07 2025 portal verified: pages=%d files=%d bytes=%d html_gzip=%d shard_gzip=%d initial_gzip=%d path=%s",
		verified.Pages, verified.Files, verified.Bytes, verified.LargestHTMLGzipBytes,
		verified.LargestShardGzipBytes, verified.InitialViewGzipBytes, out)
	index := readPortal(t, out, "fiscalizacao/index.html")
	if !strings.Contains(index, "Fiscalização — multas aplicadas") || !strings.Contains(index, "Edição 2025") {
		t.Errorf("real official index missing identity")
	}
	firstLine := strings.SplitN(string(payload), "\n", 2)[0]
	var first officialRecord
	if err := json.Unmarshal([]byte(firstLine), &first); err != nil {
		t.Fatal(err)
	}
	recordPage := readPortal(t, out, "fiscalizacao/registro/"+officialRecordID(first.NativeKey)+"/index.html")
	if !strings.Contains(recordPage, cnpjBR(first.SubjectCNPJ)) || !strings.Contains(recordPage, "não é certidão negativa") {
		t.Errorf("real official record page missing fields")
	}
	checkPortalLinks(t, out)
}

func TestGenerateOfficialOnly(t *testing.T) {
	snapshot := writeFixture(t)
	writeOfficialFixture(t, snapshot)
	out := filepath.Join(t.TempDir(), "official-only")
	result, err := Generate(Options{SnapshotDir: snapshot, OutputDir: out, OfficialOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	// fiscalizacao index + 3 records + metodologia + 404 = 6 pages;
	// 6 HTML + 1 CSS + robots + manifest = 9 files.
	if result.Pages != 6 || result.Files != 9 {
		t.Fatalf("result = %+v, want 6 pages and 9 files", result)
	}
	for _, page := range []string{
		"fiscalizacao/index.html",
		"fiscalizacao/registro/" + officialRecordID("f1") + "/index.html",
		"fiscalizacao/registro/" + officialRecordID("f2") + "/index.html",
		"fiscalizacao/registro/" + officialRecordID("f3") + "/index.html",
		"metodologia/index.html", "404.html",
		"robots.txt", "portal-manifest.json",
	} {
		if _, err := os.Stat(filepath.Join(out, page)); err != nil {
			t.Errorf("missing %s: %v", page, err)
		}
	}
	for _, dir := range []string{"precos", "postos", "dados", "consultar", "curiosidades"} {
		if _, err := os.Stat(filepath.Join(out, dir)); !os.IsNotExist(err) {
			t.Errorf("official-only artifact must not contain %s", dir)
		}
	}
	manifestRaw, err := os.ReadFile(filepath.Join(out, "portal-manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest Manifest
	if err := json.Unmarshal(manifestRaw, &manifest); err != nil {
		t.Fatal(err)
	}
	if !manifest.OfficialOnly || manifest.OfficialRecords == nil {
		t.Fatalf("manifest = %+v, want official-only with official pin", manifest)
	}
	index := readPortal(t, out, "fiscalizacao/index.html")
	for _, want := range []string{"Fiscalização — multas aplicadas", "Edição 2025", `href="/fiscalizacao/"`, `href="/metodologia/"`} {
		if !strings.Contains(index, want) {
			t.Errorf("official-only index missing %q", want)
		}
	}
	for _, forbidden := range []string{`href="/consultar/"`, `href="/precos/"`, `href="/dados/"`, `href="/curiosidades/"`, `href="/postos/`} {
		if strings.Contains(index, forbidden) {
			t.Errorf("official-only index links a price route: %q", forbidden)
		}
	}
	verified, err := Verify(out, VerifyOptions{OfficialOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if verified.Pages != 6 || verified.SitemapURLs != 0 {
		t.Fatalf("verified = %+v, want 6 pages and no sitemap", verified)
	}
	linked := readPortal(t, out, "fiscalizacao/registro/"+officialRecordID("f1")+"/index.html")
	if !strings.Contains(linked, "Vinculado a posto") {
		t.Error("official-only linked record lost its association state")
	}
	if strings.Contains(linked, `href="/postos/`) {
		t.Error("official-only linked record links a station page")
	}
	if _, err := Verify(out, VerifyOptions{}); err == nil || !strings.Contains(err.Error(), "official-only mode mismatch") {
		t.Fatalf("full-mode verify accepted an official-only artifact: %v", err)
	}
}

func TestGenerateOfficialOnlyRequiresOfficialDataset(t *testing.T) {
	snapshot := writeFixture(t)
	out := filepath.Join(t.TempDir(), "official-only-empty")
	if _, err := Generate(Options{SnapshotDir: snapshot, OutputDir: out, OfficialOnly: true}); err == nil {
		t.Fatal("official-only generation accepted a snapshot without official records")
	}
}

func TestOfficialInsightsAdmittedFactsOnly(t *testing.T) {
	snapshot := writeFixture(t)
	writeOfficialFixture(t, snapshot)
	out := filepath.Join(t.TempDir(), "portal")
	if _, err := Generate(Options{SnapshotDir: snapshot, OutputDir: out}); err != nil {
		t.Fatal(err)
	}
	page := readPortal(t, out, "curiosidades/index.html")
	for _, want := range []string{
		"Quantos registros oficiais há nesta edição",
		"A edição 2025 publica 3 registros: 1 vinculados, 1 sem vínculo e 1 com vínculo ambíguo.",
		"O que cobre a fiscalização publicada",
		"O que significa o vínculo de um registro",
		"Multa não prova adulteração, culpa ou definitividade",
		"Vínculo não prova operação histórica no local",
		"Decisão de associação da revisão sobre o CNPJ integral",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("curiosidades missing %q", want)
		}
	}
	for _, forbidden := range []string{
		"pior posto", "campeã de multas", "ranking de postos", "culpada", "sonegador",
	} {
		if strings.Contains(strings.ToLower(page), forbidden) {
			t.Errorf("official insights contain a prohibited verdict: %q", forbidden)
		}
	}
}

func TestLandingURLSharedChrome(t *testing.T) {
	snapshot := writeFixture(t)
	out := filepath.Join(t.TempDir(), "with-landing")
	if _, err := Generate(Options{SnapshotDir: snapshot, OutputDir: out, LandingURL: "https://abastevo.example/"}); err != nil {
		t.Fatal(err)
	}
	page := readPortal(t, out, "consultar/index.html")
	for _, want := range []string{
		`href="https://abastevo.example/guias/"`, `href="https://abastevo.example/como-usar/"`,
		`href="https://abastevo.example/privacidade/"`, `<a class="brand" href="https://abastevo.example/"`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("landing chrome missing %s", want)
		}
	}
	checkPortalLinks(t, out)
	if _, err := Verify(out, VerifyOptions{}); err != nil {
		t.Fatal(err)
	}

	alone := filepath.Join(t.TempDir(), "self-contained")
	if _, err := Generate(Options{SnapshotDir: snapshot, OutputDir: alone}); err != nil {
		t.Fatal(err)
	}
	if page := readPortal(t, alone, "consultar/index.html"); strings.Contains(page, "/guias/") || strings.Contains(page, "/privacidade/") {
		t.Error("self-contained portal links to landing pages")
	}
	checkPortalLinks(t, alone)

	for _, bad := range []string{"javascript:alert(1)", "https://abastevo.example/sub", "https://u:p@abastevo.example", "ftp://abastevo.example", "https://abastevo.example/?q=1"} {
		_, err := Generate(Options{SnapshotDir: snapshot, OutputDir: filepath.Join(t.TempDir(), "bad"), LandingURL: bad})
		if err == nil || !strings.Contains(err.Error(), "invalid landing URL") {
			t.Errorf("LandingURL %q accepted: %v", bad, err)
		}
	}
}

func TestGenerationBudgetErrorsAreTyped(t *testing.T) {
	out := filepath.Join(t.TempDir(), "portal")
	_, err := Generate(Options{SnapshotDir: writeFixture(t), OutputDir: out, MaxPages: 1})
	var budget *BudgetError
	if !errors.As(err, &budget) || err.Error() != "portalgen: page budget exceeded" {
		t.Fatalf("page budget breach not typed: %v", err)
	}
}

func TestDefaultBudgetsFitMeasuredNationalScale(t *testing.T) {
	// Real staging edition 2026-10-26: 52,578 pages (48,750 stations) and a
	// 461,000-byte gzip SP shard (8,381 stations).
	if defaultMaxPages < 52_578*3/2 {
		t.Fatalf("page budget %d lacks headroom over the measured national portal", defaultMaxPages)
	}
	if DefaultMaxShardGzip < 461_000 {
		t.Fatalf("shard budget %d is below the measured real SP shard", DefaultMaxShardGzip)
	}
}
