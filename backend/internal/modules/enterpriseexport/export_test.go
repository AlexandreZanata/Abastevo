package enterpriseexport

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	directoryapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/application"
	officialapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/official/application"
)

type fakeStations struct {
	rows  []directoryapp.ExportStation
	calls int
}

func (f *fakeStations) ExportPage(_ context.Context, afterID string, limit int) ([]directoryapp.ExportStation, string, error) {
	f.calls++
	start := 0
	if afterID != "" {
		start = len(f.rows)
		for i, row := range f.rows {
			if row.ID == afterID {
				start = i + 1
				break
			}
		}
	}
	end := start + limit
	if end > len(f.rows) {
		end = len(f.rows)
	}
	next := ""
	if end < len(f.rows) && end > start {
		next = f.rows[end-1].ID
	}
	return f.rows[start:end], next, nil
}

type fakePrices struct {
	revisions []officialapp.ExportRevision
	prices    map[string][]officialapp.ExportPrice
}

func (f *fakePrices) ExportRevisions(_ context.Context, _, _, afterID string, limit int) ([]officialapp.ExportRevision, string, error) {
	start := 0
	if afterID != "" {
		start = len(f.revisions)
		for i, row := range f.revisions {
			if row.ID == afterID {
				start = i + 1
				break
			}
		}
	}
	end := start + limit
	if end > len(f.revisions) {
		end = len(f.revisions)
	}
	next := ""
	if end < len(f.revisions) && end > start {
		next = f.revisions[end-1].ID
	}
	return f.revisions[start:end], next, nil
}

func (f *fakePrices) ExportPrices(_ context.Context, revisionID, afterID string, limit int) ([]officialapp.ExportPrice, string, error) {
	rows := f.prices[revisionID]
	start := 0
	if afterID != "" {
		start = len(rows)
		for i, row := range rows {
			if row.ID == afterID {
				start = i + 1
				break
			}
		}
	}
	end := start + limit
	if end > len(rows) {
		end = len(rows)
	}
	next := ""
	if end < len(rows) && end > start {
		next = rows[end-1].ID
	}
	return rows[start:end], next, nil
}

func fixtureSources() (*fakeStations, *fakePrices) {
	stations := &fakeStations{rows: []directoryapp.ExportStation{
		{ID: "00000000-0000-4000-8000-000000000001", DisplayName: "A", CNPJ: "04218406000104", Status: "active", LocationQuality: "reviewed", Coordinates: &directoryapp.LatLon{Lat: -15.8, Lon: -47.9}, Address: map[string]string{"street": "Rua A", "raw": "drop me"}, CurrentRevisionID: "00000000-0000-4000-8000-0000000000d1"},
		{ID: "00000000-0000-4000-8000-000000000002", DisplayName: "B", Status: "suspended", LocationQuality: "unknown"},
		{ID: "00000000-0000-4000-8000-000000000003", DisplayName: "C", Status: "revoked", LocationQuality: "unknown"},
	}}
	prices := &fakePrices{
		revisions: []officialapp.ExportRevision{
			{ID: "00000000-0000-4000-8000-000000000011", SurveyStart: "2026-10-04", SurveyEnd: "2026-10-10", PublishedAt: "2026-10-11T12:00:00Z", SourceURL: "https://example.test/w1.xlsx", SourceChecksum: "sha256:w1", ParserVersion: "anp-v1"},
			{ID: "00000000-0000-4000-8000-000000000012", SurveyStart: "2026-10-11", SurveyEnd: "2026-10-17", PublishedAt: "2026-10-18T12:00:00Z", SourceURL: "https://example.test/w2.xlsx", SourceChecksum: "sha256:w2", ParserVersion: "anp-v1", SupersedesRevisionID: "00000000-0000-4000-8000-000000000011"},
		},
		prices: map[string][]officialapp.ExportPrice{
			"00000000-0000-4000-8000-000000000011": {
				{ID: "00000000-0000-4000-8000-000000000021", RevisionID: "00000000-0000-4000-8000-000000000011", StationID: "00000000-0000-4000-8000-000000000001", FuelProduct: "GASOLINE_REGULAR", Unit: "L", AmountMilliBRL: 5999, CollectedOn: "2026-10-08"},
			},
			"00000000-0000-4000-8000-000000000012": {
				{ID: "00000000-0000-4000-8000-000000000022", RevisionID: "00000000-0000-4000-8000-000000000012", StationID: "00000000-0000-4000-8000-000000000001", FuelProduct: "GASOLINE_REGULAR", Unit: "L", AmountMilliBRL: 6099, CollectedOn: "2026-10-15"},
				{ID: "00000000-0000-4000-8000-000000000023", RevisionID: "00000000-0000-4000-8000-000000000012", StationID: "00000000-0000-4000-8000-000000000002", FuelProduct: "ETHANOL", Unit: "L", AmountMilliBRL: 3799, CollectedOn: "2026-10-15"},
			},
		},
	}
	return stations, prices
}

func buildFixture(t *testing.T, output string, now time.Time) Manifest {
	t.Helper()
	stations, prices := fixtureSources()
	builder := Builder{
		Stations: stations, Prices: prices, Now: func() time.Time { return now },
		PageSize: 2,
	}
	manifest, err := builder.Build(context.Background(), Options{
		FromSurvey: "2026-10-01", ToSurvey: "2026-10-31", OutputDir: output,
		DatabaseSnapshot: "test-snapshot",
	})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	return manifest
}

func TestBuildManifestAndFiles(t *testing.T) {
	parent := t.TempDir()
	output := filepath.Join(parent, "snapshot")
	manifest := buildFixture(t, output, time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC))

	if manifest.Counts != (Counts{Stations: 3, Revisions: 2, Prices: 3}) {
		t.Fatalf("counts = %+v", manifest.Counts)
	}
	if manifest.Sources.Directory.StationStatuses["active"] != 1 ||
		manifest.Sources.Directory.StationStatuses["suspended"] != 1 ||
		manifest.Sources.Directory.StationStatuses["revoked"] != 1 {
		t.Errorf("status counts = %+v", manifest.Sources.Directory.StationStatuses)
	}
	if len(manifest.Sources.Official) != 2 || manifest.Sources.Official[0].SurveyStart != "2026-10-04" {
		t.Errorf("revision pins = %+v", manifest.Sources.Official)
	}
	if !strings.HasPrefix(manifest.ContentSHA256, "sha256:") || len(manifest.Files) != 3 {
		t.Errorf("manifest identity = %+v", manifest)
	}
	for _, file := range manifest.Files {
		if file.Rows == 0 || file.SHA256 == "" {
			t.Errorf("file digest = %+v", file)
		}
	}
	stations, err := os.ReadFile(filepath.Join(output, "stations.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(stations), `"street":"Rua A"`) {
		t.Error("allowlisted address missing")
	}
	if strings.Contains(string(stations), "drop me") || strings.Contains(string(stations), `"raw"`) {
		t.Error("internal address field exported")
	}
	prices, err := os.ReadFile(filepath.Join(output, "prices.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(prices), `"currency":"BRL"`) ||
		!strings.Contains(string(prices), `"amount_milli_brl":6099`) {
		t.Error("price payload missing exact fields")
	}
	var decoded Manifest
	raw, err := os.ReadFile(filepath.Join(output, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &decoded); err != nil || decoded.ContentSHA256 != manifest.ContentSHA256 {
		t.Errorf("manifest round trip = %+v, %v", decoded, err)
	}
}

func TestBuildIsDeterministicAndBuiltAtIsOperational(t *testing.T) {
	parent := t.TempDir()
	a := buildFixture(t, filepath.Join(parent, "a"), time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC))
	b := buildFixture(t, filepath.Join(parent, "b"), time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC))
	if a.ContentSHA256 != b.ContentSHA256 {
		t.Errorf("content hash changed with built_at: %s vs %s", a.ContentSHA256, b.ContentSHA256)
	}
	if a.BuiltAt == b.BuiltAt {
		t.Error("built_at did not change")
	}
	for i := range a.Files {
		if a.Files[i] != b.Files[i] {
			t.Errorf("file digest changed: %+v vs %+v", a.Files[i], b.Files[i])
		}
	}
}

func TestBuildBudgetAndExistingOutput(t *testing.T) {
	parent := t.TempDir()
	stations, prices := fixtureSources()
	builder := Builder{
		Stations: stations, Prices: prices, PageSize: 2, MaxStations: 1,
	}
	err := func() error {
		_, err := builder.Build(context.Background(), Options{
			FromSurvey: "2026-10-01", ToSurvey: "2026-10-31",
			OutputDir: filepath.Join(parent, "over-budget"),
		})
		return err
	}()
	if err == nil || !strings.Contains(err.Error(), "budget") {
		t.Fatalf("budget error = %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(parent, "over-budget")); !os.IsNotExist(statErr) {
		t.Error("failed build left an output directory")
	}

	existing := filepath.Join(parent, "existing")
	if err := os.Mkdir(existing, 0o700); err != nil {
		t.Fatal(err)
	}
	_, err = builder.Build(context.Background(), Options{
		FromSurvey: "2026-10-01", ToSurvey: "2026-10-31", OutputDir: existing,
	})
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("existing output error = %v", err)
	}
}

func TestBuildChangesContentHashWhenDataChanges(t *testing.T) {
	parent := t.TempDir()
	first := buildFixture(t, filepath.Join(parent, "first"), time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC))
	stations, prices := fixtureSources()
	stations.rows = stations.rows[:2]
	builder := Builder{Stations: stations, Prices: prices, PageSize: 2}
	second, err := builder.Build(context.Background(), Options{
		FromSurvey: "2026-10-01", ToSurvey: "2026-10-31",
		OutputDir: filepath.Join(parent, "second"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if first.ContentSHA256 == second.ContentSHA256 {
		t.Error("content hash ignored a station removal")
	}
}

func TestBuildRejectsCursorThatDoesNotAdvance(t *testing.T) {
	parent := t.TempDir()
	_, prices := fixtureSources()
	builder := Builder{
		Stations: stuckStations{}, Prices: prices, PageSize: 2,
	}
	_, err := builder.Build(context.Background(), Options{
		FromSurvey: "2026-10-01", ToSurvey: "2026-10-31",
		OutputDir: filepath.Join(parent, "stuck"),
	})
	if err == nil || !strings.Contains(err.Error(), "cursor") {
		t.Fatalf("stuck cursor error = %v", err)
	}
}

type stuckStations struct{}

func (stuckStations) ExportPage(context.Context, string, int) ([]directoryapp.ExportStation, string, error) {
	return []directoryapp.ExportStation{{ID: "00000000-0000-4000-8000-000000000001", Status: "active"}}, "same", nil
}

func TestContentHashIgnoresOperationalPageSize(t *testing.T) {
	parent := t.TempDir()
	a := buildFixture(t, filepath.Join(parent, "a"), time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC))
	stations, prices := fixtureSources()
	builder := Builder{Stations: stations, Prices: prices, PageSize: 1}
	b, err := builder.Build(context.Background(), Options{
		FromSurvey: "2026-10-01", ToSurvey: "2026-10-31",
		OutputDir: filepath.Join(parent, "b"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if a.ContentSHA256 != b.ContentSHA256 {
		t.Errorf("page size changed content hash: %s vs %s", a.ContentSHA256, b.ContentSHA256)
	}
}
