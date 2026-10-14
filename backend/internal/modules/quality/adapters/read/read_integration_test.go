//go:build integration

package read

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	dbmigrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	parentadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
)

func freshPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	adminDSN := os.Getenv("ANPFUEL_TEST_DATABASE_URL")
	if adminDSN == "" {
		adminDSN = "postgres://anpfuel:anpfuel@127.0.0.1:5434/anpfuel?sslmode=disable"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatalf("admin connect: %v", err)
	}
	defer admin.Close(ctx)
	name := fmt.Sprintf("quality_read_test_%d", time.Now().UnixNano())
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
		t.Fatal(err)
	}
	u.Path = "/" + name
	if _, err := migrate.Apply(ctx, u.String(), dbmigrations.Files); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err := pgxpool.New(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

var readIDs = 0

func nextID() (string, error) {
	readIDs++
	return fmt.Sprintf("dddddddd-4444-4444-8444-%012d", readIDs), nil
}

const readStation = "eeeeeeee-5555-4555-8555-000000000001"

func seedStation(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		"INSERT INTO directory_stations (id, display_name) VALUES ($1, 'Posto Leitura')", readStation); err != nil {
		t.Fatalf("seed station: %v", err)
	}
}

func seedAssays(t *testing.T, pool *pgxpool.Pool, hashChar string) string {
	t.Helper()
	rev, err := parentadapters.NewPublisher(pool).Publish(context.Background(), application.Input{
		Source: "pmqc", Stream: "pmqc-assay-v1", Edition: "2026-09",
		Resources: []application.Resource{{
			URL:    "https://www.gov.br/anp/pmqc_2026_09.zip",
			SHA256: strings.Repeat("a", 64), Bytes: 10,
		}},
		Counts:       domain.Counts{Input: 3, Accepted: 3},
		ManifestHash: strings.Repeat(hashChar, 64),
		Assays: []application.AssayInput{
			{SampleRef: "S-1", AssayRef: "ASPECTO", SourceCNPJ: "11222333000181", ObservedOn: "2026-09-04", Fuel: "ETHANOL", ResultText: "92,6", Unit: "%", Conformity: "CONFORMING", EvidenceRef: "f#S-1#ASPECTO", Fingerprint: strings.Repeat("c", 64)},
			{SampleRef: "S-1", AssayRef: "DESTILACAO", SourceCNPJ: "11222333000181", ObservedOn: "2026-09-04", Fuel: "ETHANOL", ResultText: "181,5", Unit: "°C", Conformity: "CONFORMING", EvidenceRef: "f#S-1#DESTILACAO", Fingerprint: strings.Repeat("d", 64)},
			{SampleRef: "S-2", AssayRef: "ASPECTO", SourceCNPJ: "11222333000181", ObservedOn: "2026-09-05", Fuel: "GASOLINE_REGULAR", ResultText: "turvo", Unit: "-", Conformity: "NONCONFORMING", EvidenceRef: "f#S-2#ASPECTO", Fingerprint: strings.Repeat("e", 64)},
		},
		Links: []application.LinkInput{
			{RecordKey: "pmqc-assay-v1:S-1", StationID: readStation, Automatic: true, Reason: "exact cnpj", Active: true},
			{RecordKey: "pmqc-assay-v1:S-2", StationID: readStation, Automatic: true, Reason: "exact cnpj", Active: true},
		},
	}, nextID)
	if err != nil {
		t.Fatalf("seed assays: %v", err)
	}
	return rev
}

func seedEvents(t *testing.T, pool *pgxpool.Pool) (string, string) {
	t.Helper()
	amount := int64(150000)
	enforcement, err := parentadapters.NewPublisher(pool).PublishEvents(context.Background(), application.EventsInput{
		Source: "enforcement", Stream: "enforcement-v1", Edition: "2026-09",
		Resources: []application.Resource{{
			URL: "https://www.gov.br/anp/acoes", SHA256: strings.Repeat("b", 64), Bytes: 9,
		}},
		Counts:       domain.Counts{Input: 1, Accepted: 1},
		ManifestHash: strings.Repeat("f", 64),
		Enforcement: []application.EnforcementInput{
			{NativeKey: "AUTO-1", Scope: "municipal", Status: "autuado", SubjectCNPJ: "11222333000181", OccurredOn: "2026-08-02", AsOf: "2026-09-30", EvidenceRef: "e#AUTO-1"},
		},
	}, nextID)
	if err != nil {
		t.Fatalf("seed enforcement: %v", err)
	}
	sanctions, err := parentadapters.NewPublisher(pool).PublishEvents(context.Background(), application.EventsInput{
		Source: "sanctions", Stream: "sanction-v1", Edition: "2025",
		Resources: []application.Resource{{
			URL: "https://www.gov.br/anp/multas", SHA256: strings.Repeat("c", 64), Bytes: 8,
		}},
		Counts:       domain.Counts{Input: 1, Accepted: 1},
		ManifestHash: strings.Repeat("d", 64),
		Sanctions: []application.SanctionInput{
			{NativeKey: "MULTA-1", ProceedingRef: "48000.1/2025-11", PartyCNPJ: "04218406000104", AmountMinor: &amount, Status: "aplicada", DecidedOn: "2025-07-15", AsOf: "2025-12-31", EvidenceRef: "e#MULTA-1"},
		},
	}, nextID)
	if err != nil {
		t.Fatalf("seed sanctions: %v", err)
	}
	return enforcement, sanctions
}

func TestSummaryDenominators(t *testing.T) {
	pool := freshPool(t)
	seedStation(t, pool)
	seedAssays(t, pool, "b")
	r := NewReader(pool)
	now := time.Now()

	cells, lease, err := r.Summary(context.Background(), now, "pmqc", "pmqc-assay-v1", readStation, "")
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if len(cells) != 2 {
		t.Fatalf("cells = %+v, want ETHANOL/CONFORMING + GASOLINE_REGULAR/NONCONFORMING", cells)
	}
	if cells[0].Fuel != "ETHANOL" || cells[0].Count != 2 || cells[1].Conformity != "NONCONFORMING" {
		t.Errorf("cells = %+v", cells)
	}
	if !lease.HasPublished || lease.Edition != "2026-09" || lease.Stale {
		t.Errorf("lease = %+v, want published fresh 2026-09", lease)
	}
	if lease.Counts.Accepted != 3 || !strings.Contains(lease.CoverageNote, "2026-09") {
		t.Errorf("lease counts/note = %+v", lease)
	}
	filtered, _, err := r.Summary(context.Background(), now, "pmqc", "pmqc-assay-v1", readStation, "ETHANOL")
	if err != nil || len(filtered) != 1 {
		t.Fatalf("fuel filter = %+v %v", filtered, err)
	}
	// Unknown station: empty denominators with the live lease, never an error.
	empty, _, err := r.Summary(context.Background(), now, "pmqc", "pmqc-assay-v1",
		"ffffffff-6666-4666-8666-000000000002", "")
	if err != nil || len(empty) != 0 {
		t.Fatalf("unknown station = %+v %v, want empty", empty, err)
	}
}

func TestAssayPagesTraverseFrozenEdition(t *testing.T) {
	pool := freshPool(t)
	seedStation(t, pool)
	seedAssays(t, pool, "b")
	r := NewReader(pool)
	now := time.Now()
	ctx := context.Background()

	page1, next, _, err := r.Assays(ctx, now, "pmqc", "pmqc-assay-v1", readStation, "", "", "", 2)
	if err != nil || len(page1) != 2 || next == "" {
		t.Fatalf("page1 = %+v next %q %v", page1, next, err)
	}
	observed, id, _ := strings.Cut(next, "|")
	page2, next2, _, err := r.Assays(ctx, now, "pmqc", "pmqc-assay-v1", readStation, "", observed, id, 2)
	if err != nil || len(page2) != 1 || next2 != "" {
		t.Fatalf("page2 = %+v next %q %v", page2, next2, err)
	}
	if page2[0].SampleRef != "S-2" {
		t.Errorf("traversal order broke: %+v", page2)
	}
}

func TestSampleDetail(t *testing.T) {
	pool := freshPool(t)
	seedStation(t, pool)
	seedAssays(t, pool, "b")
	r := NewReader(pool)

	detail, err := r.Sample(context.Background(), time.Now(), "pmqc", "pmqc-assay-v1", "S-1")
	if err != nil {
		t.Fatalf("sample: %v", err)
	}
	if len(detail.Assays) != 2 || len(detail.Links) != 1 || !detail.Links[0].Active {
		t.Fatalf("detail = %+v", detail)
	}
	if detail.Links[0].StationID != readStation {
		t.Errorf("link station = %q", detail.Links[0].StationID)
	}
	// Unknown sample: empty rows with the live lease, never an error.
	missing, err := r.Sample(context.Background(), time.Now(), "pmqc", "pmqc-assay-v1", "S-9")
	if err != nil || len(missing.Assays) != 0 || !missing.Lease.HasPublished {
		t.Fatalf("unknown sample = %+v %v", missing, err)
	}
}

func TestEventPagesAndDetail(t *testing.T) {
	pool := freshPool(t)
	seedEvents(t, pool)
	r := NewReader(pool)
	now := time.Now()
	ctx := context.Background()

	actions, next, lease, err := r.Enforcement(ctx, now, "enforcement", "enforcement-v1", "", "", "", "", 10)
	if err != nil || len(actions) != 1 || next != "" {
		t.Fatalf("actions = %+v next %q %v", actions, next, err)
	}
	if actions[0].NativeKey != "AUTO-1" || actions[0].Scope != "municipal" || !lease.HasPublished {
		t.Errorf("action = %+v", actions[0])
	}
	one, _, err := r.EnforcementOne(ctx, now, "enforcement", "enforcement-v1", actions[0].ID)
	if err != nil || one.NativeKey != "AUTO-1" {
		t.Fatalf("action detail = %+v %v", one, err)
	}
	sanctions, _, _, err := r.Sanctions(ctx, now, "sanctions", "sanction-v1", "", "", "", 10)
	if err != nil || len(sanctions) != 1 || sanctions[0].AmountMinor == nil || *sanctions[0].AmountMinor != 150000 {
		t.Fatalf("sanctions = %+v %v", sanctions, err)
	}
	if _, _, err := r.SanctionOne(ctx, now, "sanctions", "sanction-v1", "aaaaaaaa-1111-4111-8111-000000000099"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown id -> %v, want ErrNotFound", err)
	}
}

func TestNoLeaseServesUnavailable(t *testing.T) {
	pool := freshPool(t)
	r := NewReader(pool)
	now := time.Now()

	cells, lease, err := r.Summary(context.Background(), now, "pmqc", "pmqc-assay-v1", readStation, "")
	if err != nil || len(cells) != 0 || lease.HasPublished || !lease.Stale {
		t.Fatalf("empty lease = %+v %+v %v", cells, lease, err)
	}
	rows, next, _, err := r.Assays(context.Background(), now, "pmqc", "pmqc-assay-v1", readStation, "", "", "", 10)
	if err != nil || len(rows) != 0 || next != "" {
		t.Fatalf("empty page = %+v %q %v", rows, next, err)
	}
}

func TestConcurrentRevisionKeepsFrozenReads(t *testing.T) {
	pool := freshPool(t)
	seedStation(t, pool)
	rev1 := seedAssays(t, pool, "b")
	r := NewReader(pool)
	now := time.Now()
	ctx := context.Background()

	before, _, err := r.Summary(ctx, now, "pmqc", "pmqc-assay-v1", readStation, "")
	if err != nil || len(before) != 2 {
		t.Fatalf("rev1 summary = %+v %v", before, err)
	}
	rev2 := seedAssays(t, pool, "e")
	if rev1 == rev2 {
		t.Fatal("correction reused the revision")
	}
	after, lease, err := r.Summary(ctx, now, "pmqc", "pmqc-assay-v1", readStation, "")
	if err != nil {
		t.Fatal(err)
	}
	if lease.RevisionID != rev2 {
		t.Errorf("lease did not move to the correction")
	}
	if len(after) != 2 {
		t.Errorf("rev2 summary mixed revisions: %+v", after)
	}
}

func TestWireCarriesNoIdentifiers(t *testing.T) {
	pool := freshPool(t)
	seedStation(t, pool)
	seedAssays(t, pool, "b")
	seedEvents(t, pool)
	r := NewReader(pool)
	ctx := context.Background()
	now := time.Now()

	assays, _, _, err := r.Assays(ctx, now, "pmqc", "pmqc-assay-v1", readStation, "", "", "", 10)
	if err != nil {
		t.Fatal(err)
	}
	actions, _, _, err := r.Enforcement(ctx, now, "enforcement", "enforcement-v1", "", "", "", "", 10)
	if err != nil {
		t.Fatal(err)
	}
	sanctions, _, _, err := r.Sanctions(ctx, now, "sanctions", "sanction-v1", "", "", "", 10)
	if err != nil {
		t.Fatal(err)
	}
	marshal := func(v any) string {
		raw, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		return strings.ToLower(string(raw))
	}
	for _, body := range []string{marshal(assays), marshal(actions), marshal(sanctions)} {
		if strings.Contains(body, "cnpj") {
			t.Errorf("source identifier leaked: %s", body)
		}
	}
}
