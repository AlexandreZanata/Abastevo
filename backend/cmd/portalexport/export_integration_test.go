//go:build integration

package main

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	directory "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/adapters"
	directoryapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/application"
	parentadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	dbmigrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
)

func portalExportPool(t *testing.T) *pgxpool.Pool {
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
	name := fmt.Sprintf("portalexport_test_%d", time.Now().UnixNano())
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

var portalExportIDs = 0

func portalExportNextID() (string, error) {
	portalExportIDs++
	return fmt.Sprintf("eeeeeeee-5555-4555-8555-%012d", portalExportIDs), nil
}

func portalExportObservation(status, cnpj, duf, evidence string) application.FineObservation {
	row := application.FineObservation{
		SourceCNPJ: cnpj, ProceedingRef: "48000.1/2025-11", DUFRef: duf,
		Department: "SYNTHETIC", Status: status, DueOn: "2025-02-28",
		AmountText: "150000,00", AmountMinor: 15000000, PaidText: "0,00",
		ReportYear: 2025, ExtractionMonth: "2026-01", EvidenceRef: evidence,
	}
	row.ObservationKey = row.ObservationIdentity()
	return row
}

func portalExportPublish(t *testing.T, pool *pgxpool.Pool, stationID, status string) string {
	t.Helper()
	rows := []application.FineObservation{
		portalExportObservation(status, "11222333000181", "SYNTHETIC/A", "synthetic.csv:2"),
		portalExportObservation(status, "11222333000262", "SYNTHETIC/B", "synthetic.csv:3"),
	}
	decisions := []domain.CompanyObservationDecision{
		{RecordKey: rows[0].ObservationKey, DecidedAt: "2026-10-20", PolicyVersion: domain.CompanyObservationPolicyVersion, Kind: domain.ResolutionBound, StationID: stationID, Scope: domain.CurrentCompanyOnly, Reason: "exact current cnpj; historical business window unknown"},
		{RecordKey: rows[1].ObservationKey, DecidedAt: "2026-10-20", PolicyVersion: domain.CompanyObservationPolicyVersion, Kind: domain.ResolutionUnmatched, Reason: domain.ReasonUnmatchedID},
	}
	in := application.NativeFineInput{
		Edition: "2025", Clock: application.NativeFineClock{ReportYear: 2025, ExtractionMonth: "2026-01"},
		Resources: []application.Resource{{
			URL:    "https://www.gov.br/anp/pt-br/centrais-de-conteudo/dados-abertos/arquivos/mav/multas-aplicadas-2025.csv",
			SHA256: strings.Repeat("a", 64), Bytes: 100,
		}},
		Counts:       domain.Counts{Input: 2, Accepted: 2},
		ManifestHash: strings.Repeat("b", 64), Observations: rows,
		CompanyAssociations: &application.CompanyAssociationInput{
			PolicyVersion: domain.CompanyObservationPolicyVersion, DecisionDay: "2026-10-20", Decisions: decisions,
		},
	}
	revision, err := parentadapters.NewPublisher(pool).WithCompanyGuard(func(ctx context.Context, tx pgx.Tx, bindings []application.CompanyBinding) error {
		expected := make([]directoryapp.SourceBinding, 0, len(bindings))
		for _, b := range bindings {
			expected = append(expected, directoryapp.SourceBinding{CNPJ: b.CNPJ, StationID: b.StationID})
		}
		return directory.NewSourceIdentityReader(pool).Guard(ctx, tx, expected)
	}).PublishNativeFines(context.Background(), in, portalExportNextID)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	return revision
}

func TestPortalExportOfficialOnlyPostGIS(t *testing.T) {
	pool := portalExportPool(t)
	ctx := context.Background()
	station, err := directory.NewRepository(pool).ResolveCNPJ(ctx, "11222333000181", "Synthetic export station", nil)
	if err != nil {
		t.Fatalf("resolve station: %v", err)
	}
	portalExportPublish(t, pool, station.ID, "Multa aplicada")
	newCfg := func() Config {
		return Config{
			SnapshotDir: filepath.Join(t.TempDir(), "snapshot"),
			PortalOut:   filepath.Join(t.TempDir(), "portal"), OfficialOnly: true,
		}
	}
	first, err := runExportTx(ctx, pool, newCfg())
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if first.Edition != "2025" || first.Rows != 2 || first.Linked != 1 || first.Unmatched != 1 {
		t.Fatalf("result = %+v", first)
	}
	// No-op rerun converges on the identical candidate.
	again, err := runExportTx(ctx, pool, newCfg())
	if err != nil {
		t.Fatalf("export rerun: %v", err)
	}
	if again.ContentSHA256 != first.ContentSHA256 {
		t.Fatal("unchanged official lease rebuilt a different candidate")
	}
	// A corrected revision produces a second, distinct version.
	portalExportPublish(t, pool, station.ID, "Multa aplicada corrigida")
	second, err := runExportTx(ctx, pool, newCfg())
	if err != nil {
		t.Fatalf("export after correction: %v", err)
	}
	if second.ContentSHA256 == first.ContentSHA256 {
		t.Fatal("corrected official revision reused the old candidate")
	}
}

func TestPortalExportNoPublishedRevisionPostGIS(t *testing.T) {
	pool := portalExportPool(t)
	cfg := Config{
		SnapshotDir: filepath.Join(t.TempDir(), "snapshot"),
		PortalOut:   filepath.Join(t.TempDir(), "portal"), OfficialOnly: true,
	}
	if _, err := runExportTx(context.Background(), pool, cfg); err == nil {
		t.Fatal("export accepted an empty official stream")
	} else {
		t.Logf("empty stream refused: %v", err)
	}
}
