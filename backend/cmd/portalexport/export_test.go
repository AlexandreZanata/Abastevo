package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	enterprisejobs "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/enterpriseexport/adapters/jobs"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/enterpriseexport/portalgen"
)

func TestConfigValidation(t *testing.T) {
	base := Config{DSN: "postgres://u:p@h/db", SnapshotDir: "snap", PortalOut: "out"}
	if err := base.validate(); err != nil {
		t.Fatalf("valid config refused: %v", err)
	}
	for name, mutate := range map[string]func(*Config){
		"missing DSN":       func(c *Config) { c.DSN = "" },
		"missing snapshot":  func(c *Config) { c.SnapshotDir = "" },
		"missing out":       func(c *Config) { c.PortalOut = "" },
		"indexable w/o URL": func(c *Config) { c.Indexable = true },
	} {
		cfg := base
		mutate(&cfg)
		if err := cfg.validate(); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

type fakeOfficialProvider struct {
	payload []byte
	pin     enterprisejobs.OfficialPin
	err     error
}

func (f fakeOfficialProvider) Pin(ctx context.Context) (*enterprisejobs.OfficialPin, error) {
	return &f.pin, f.err
}

func (f fakeOfficialProvider) Build(ctx context.Context, out io.Writer) (enterprisejobs.OfficialBuild, error) {
	if f.err != nil {
		return enterprisejobs.OfficialBuild{}, f.err
	}
	if _, err := out.Write(f.payload); err != nil {
		return enterprisejobs.OfficialBuild{}, err
	}
	sum := sha256.Sum256(f.payload)
	pin := f.pin
	pin.PayloadSHA256 = hex.EncodeToString(sum[:])
	manifest := map[string]any{
		"schema_version": "quality-official-export-v1", "public_field_policy": "fq-public-v1",
		"source": "sanctions", "stream": "sanction-v1", "edition": "2025",
		"revision_id": pin.RevisionID, "lease_id": pin.LeaseID, "published_at": "2026-10-24T21:55:16Z",
		"status": "published", "provenance_state": "VERIFIED",
		"source_url": "https://www.gov.br/anp/multas-aplicadas-2025.csv", "source_checksum": strings.Repeat("a", 64),
		"source_bytes": 450504, "source_period": "2025", "source_clock_precision": "MONTH", "source_clock_value": "2026-01",
		"counts":             map[string]any{"input": 1, "accepted": 1, "duplicates": 0, "quarantined": 0},
		"association_policy": "current-company-v1", "association_scope": "CURRENT_COMPANY_ONLY",
		"association_decision_day": "2026-10-20",
		"association_census":       map[string]any{"total": 1, "linked": 0, "unmatched": 1, "ambiguous": 0, "wrong_branch": 0, "succession_review": 0},
		"rows":                     1, "payload_sha256": pin.PayloadSHA256,
	}
	raw, err := json.Marshal(manifest)
	if err != nil {
		return enterprisejobs.OfficialBuild{}, err
	}
	return enterprisejobs.OfficialBuild{Manifest: raw, Pin: pin}, nil
}

func fakePin() enterprisejobs.OfficialPin {
	return enterprisejobs.OfficialPin{
		RevisionID: "00000000-0000-4000-8000-0000000000f1", LeaseID: strings.Repeat("b", 64),
		Edition: "2025", Rows: 1, Unmatched: 1,
	}
}

func fakePayload() []byte {
	return []byte(`{"native_key":"u1","subject_cnpj":"11222333000181","scope":"sanction","status":"Multa aplicada","evidence":"fines.csv:3","association_state":"unmatched","association_reason":"no current station","revision_id":"00000000-0000-4000-8000-0000000000f1","lease_id":"` + strings.Repeat("b", 64) + `","source_url":"https://www.gov.br/anp/multas-aplicadas-2025.csv","source_checksum":"` + strings.Repeat("a", 64) + `","proceeding_ref":"48000.2/2025-12","amount_minor":30000,"amount_text":"300,00","due_on":"2025-03-31","source_contract":"anp-sicom-cp850-9-v1","source_period":"2025","extraction_month":"2026-01","report_year":2025}` + "\n")
}

func TestWriteOfficialSnapshotOfficialOnly(t *testing.T) {
	ctx := context.Background()
	dir := filepath.Join(t.TempDir(), "snapshot")
	cfg := Config{SnapshotDir: dir, PortalOut: filepath.Join(t.TempDir(), "portal"), OfficialOnly: true}
	snapshot, pin, err := writeOfficialSnapshot(ctx, fakeOfficialProvider{payload: fakePayload(), pin: fakePin()}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot != dir || pin.Rows != 1 || pin.Unmatched != 1 {
		t.Fatalf("snapshot pin = %+v", pin)
	}
	result, err := portalgen.Generate(portalgen.Options{SnapshotDir: snapshot, OutputDir: cfg.PortalOut, OfficialOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if result.Pages != 4 {
		t.Fatalf("pages = %d, want index + 1 record + metodologia + 404", result.Pages)
	}
	if _, err := portalgen.Verify(cfg.PortalOut, portalgen.VerifyOptions{OfficialOnly: true}); err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"precos", "postos", "dados", "consultar", "curiosidades"} {
		if _, err := os.Stat(filepath.Join(cfg.PortalOut, forbidden)); !os.IsNotExist(err) {
			t.Fatalf("official-only candidate contains %s", forbidden)
		}
	}
}

func TestWriteOfficialSnapshotFailsClosed(t *testing.T) {
	ctx := context.Background()
	cfg := Config{SnapshotDir: filepath.Join(t.TempDir(), "snapshot"), PortalOut: "out", OfficialOnly: true}
	if _, _, err := writeOfficialSnapshot(ctx, fakeOfficialProvider{err: errors.New("no published revision")}, cfg); err == nil {
		t.Fatal("missing official lease accepted")
	}
}
