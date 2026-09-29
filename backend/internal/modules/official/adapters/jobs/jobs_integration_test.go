//go:build integration

package jobs

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	dbmigrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	parentdirectory "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/adapters"
	officialadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/official/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/official/adapters/anp"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/official/adapters/source"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
)

func testDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("ANPFUEL_TEST_DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://anpfuel:anpfuel@127.0.0.1:5434/anpfuel?sslmode=disable"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("integration database unreachable at %s: %v (start it: docker compose -f infra/compose.dev.yml up -d db)", dsn, err)
	}
	defer conn.Close(ctx)
	return dsn
}

func freshPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	adminDSN := testDSN(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatalf("admin connect: %v", err)
	}
	defer admin.Close(ctx)
	name := fmt.Sprintf("jobshandler_test_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
		t.Fatalf("create database: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		admin, err := pgx.Connect(ctx, adminDSN)
		if err != nil {
			t.Errorf("admin connect for drop: %v", err)
			return
		}
		defer admin.Close(ctx)
		if _, err := admin.Exec(ctx, "DROP DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
			t.Errorf("drop database: %v", err)
		}
	})
	u, err := url.Parse(adminDSN)
	if err != nil {
		t.Fatalf("parse dsn: %v", err)
	}
	u.Path = "/" + name
	dsn := u.String()
	if _, err := migrate.Apply(ctx, dsn, dbmigrations.Files); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// fixtureWorkbook builds a minimal station sheet: headers plus two data
// rows with exact decimals and one numeric Excel date.
func fixtureWorkbook(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	add := func(name, body string) {
		f, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	add("[Content_Types].xml", `<?xml version="1.0"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/><Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/></Types>`)
	add("_rels/.rels", `<?xml version="1.0"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/></Relationships>`)
	add("xl/workbook.xml", `<?xml version="1.0"?><workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="DPC" sheetId="1" r:id="rId1"/></sheets></workbook>`)
	add("xl/_rels/workbook.xml.rels", `<?xml version="1.0"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/></Relationships>`)
	add("xl/worksheets/sheet1.xml", `<?xml version="1.0"?><worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>`+
		`<row r="8"><c r="A8" t="str"><v>CNPJ</v></c><c r="B8" t="str"><v>PRODUTO</v></c><c r="C8" t="str"><v>PRECO</v></c><c r="D8" t="str"><v>DATA</v></c></row>`+
		`<row r="9"><c r="A9" t="str"><v>04.218.406/0001-04</v></c><c r="B9" t="str"><v>GASOLINA COMUM</v></c><c r="C9" t="str"><v>5,999</v></c><c r="D9"><v>45658</v></c></row>`+
		`<row r="10"><c r="A10" t="str"><v>11.222.333/0001-81</v></c><c r="B10" t="str"><v>ETANOL</v></c><c r="C10" t="str"><v>3,799</v></c><c r="D10"><v>45658</v></c></row>`+
		`</sheetData></worksheet>`)
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// fixtureServer serves one weekly file plus 404s, with an ETag.
func fixtureServer(t *testing.T, week string, body []byte) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".xlsx") && strings.Contains(r.URL.Path, week) {
			w.Header().Set("ETag", `"w1"`)
			_, _ = w.Write(body)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	srv := httptest.NewTLSServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func loopbackEntries(t *testing.T, serverURL string) []source.Entry {
	t.Helper()
	u, err := url.Parse(serverURL)
	if err != nil {
		t.Fatal(err)
	}
	var port int
	_, _ = fmt.Sscanf(u.Port(), "%d", &port)
	return []source.Entry{
		{ID: "t", Host: u.Hostname(), PathPrefix: "/", FileGlob: "*.xlsx", Ports: []int{port}},
	}
}

// seedStations resolves the two fixture CNPJs.
func seedStations(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	repo := parentdirectory.NewRepository(pool)
	ctx := context.Background()
	for _, c := range []struct{ cnpj, name string }{
		{"04218406000104", "Posto A"},
		{"11222333000181", "Posto B"},
	} {
		if _, err := repo.ResolveCNPJ(ctx, c.cnpj, c.name, nil); err != nil {
			t.Fatalf("station %s: %v", c.cnpj, err)
		}
	}
}

func importHandler(t *testing.T, pool *pgxpool.Pool, allow []source.Entry) Import {
	t.Helper()
	fetcher := source.NewFetcher(allow)
	fetcher.Timeout = 10 * time.Second
	fetcher.InsecureTLS = true
	fetcher.IPAllow = func(netip.Addr) bool { return true }
	dirRepo := parentdirectory.NewRepository(pool)
	return Import{
		Fetch:  &fetcher,
		Parser: anp.Parser{Limits: anp.DefaultLimits()},
		Import: officialadapters.NewImporter(pool),
		Resolve: func(ctx context.Context, cnpj, display string, addr map[string]string) (string, error) {
			st, err := dirRepo.ResolveCNPJ(ctx, cnpj, display, addr)
			if err != nil {
				return "", err
			}
			return st.ID, nil
		},
	}
}

func TestDiscoveryEnqueuesChangedOnly(t *testing.T) {
	ctx := context.Background()
	body := fixtureWorkbook(t)
	// Week containing 2026-09-28 is 2026-09-27..2026-10-03 (Sun-Sat).
	srv := fixtureServer(t, "2026-09-27_2026-10-03", body)
	allow := loopbackEntries(t, srv.URL)
	fetcher := source.NewFetcher(allow)
	fetcher.Timeout = 10 * time.Second
	fetcher.InsecureTLS = true
	fetcher.IPAllow = func(netip.Addr) bool { return true }
	var enqueued []ImportPayload
	d := Discovery{
		Fetch:       &fetcher,
		ListingBase: srv.URL + "/arquivos-lpc/",
		Enqueue: func(_ context.Context, p ImportPayload, _ string) error {
			enqueued = append(enqueued, p)
			return nil
		},
		Now: func() time.Time { return time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC) },
	}
	if err := d.Handle(ctx, jobs.Job{}); err != nil {
		t.Fatalf("discover: %v", err)
	}
	// Current week serves both files; summary and previous weeks 404.
	if len(enqueued) != 2 {
		t.Fatalf("enqueued %d, want detail plus summary of the current week", len(enqueued))
	}
	kinds := map[string]bool{}
	for _, p := range enqueued {
		kinds[p.SourceURL] = true
		if p.ETag != `"w1"` || p.SurveyStart != "2026-09-27" || p.SurveyEnd != "2026-10-03" {
			t.Errorf("payload = %+v", p)
		}
	}
	if len(kinds) != 2 {
		t.Errorf("enqueued URLs = %v", kinds)
	}
	if got := WeekStart(time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)).Format("2006-01-02"); got != "2026-09-27" {
		t.Errorf("week start = %s", got)
	}
}

func TestImportEndToEndWithoutDuplicates(t *testing.T) {
	pool := freshPool(t)
	ctx := context.Background()
	seedStations(t, pool)
	body := fixtureWorkbook(t)
	srv := fixtureServer(t, "2026-09-27_2026-10-03", body)
	allow := loopbackEntries(t, srv.URL)
	imp := importHandler(t, pool, allow)
	fileURL := srv.URL + "/arquivos-lpc/2026/revendas_lpc_2026-09-27_2026-10-03.xlsx"
	payload := ImportPayload{Version: 1, SourceURL: fileURL, ETag: `"w1"`,
		SurveyStart: "2026-09-27", SurveyEnd: "2026-10-03"}
	raw, _ := json.Marshal(payload)
	if err := imp.Handle(ctx, jobs.Job{Payload: raw}); err != nil {
		t.Fatalf("import: %v", err)
	}
	var revisions, prices int64
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM official_revisions WHERE status = 'published'").Scan(&revisions); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM official_station_prices").Scan(&prices); err != nil {
		t.Fatal(err)
	}
	if revisions != 1 || prices != 2 {
		t.Errorf("revisions=%d prices=%d, want 1/2", revisions, prices)
	}
	var pointer string
	if err := pool.QueryRow(ctx, "SELECT revision_id::text FROM official_current_revisions").Scan(&pointer); err != nil {
		t.Fatalf("pointer: %v", err)
	}
	// Re-running the same file is a checksum no-op: still one revision.
	if err := imp.Handle(ctx, jobs.Job{Payload: raw}); err != nil {
		t.Fatalf("re-import: %v", err)
	}
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM official_revisions").Scan(&revisions); err != nil {
		t.Fatal(err)
	}
	if revisions != 1 {
		t.Errorf("revisions = %d after re-import, want 1", revisions)
	}
}

func TestImportBadPayloadFailsSafely(t *testing.T) {
	pool := freshPool(t)
	imp := importHandler(t, pool, loopbackEntries(t, "https://127.0.0.1:1/"))
	for _, raw := range [][]byte{
		[]byte(`not json`),
		[]byte(`{"version":2}`),
		[]byte(`{"version":1}`),
	} {
		if err := imp.Handle(context.Background(), jobs.Job{Payload: raw}); err == nil {
			t.Errorf("bad payload accepted: %s", raw)
		}
	}
}
