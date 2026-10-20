package jobs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters/prepared"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/application"
	platformjobs "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/jackc/pgx/v5/pgxpool"
)

const unitFineURL = "https://www.gov.br/anp/pt-br/centrais-de-conteudo/dados-abertos/arquivos/mav/multas-aplicadas-2025.csv"

func fineJobPayload(t *testing.T, change func(*ImportPayload)) []byte {
	t.Helper()
	p := ImportPayload{Version: 1, Source: "sanctions", Stream: "sanction-v1", Edition: "2025", URL: unitFineURL, SHA256: strings.Repeat("a", 64)}
	if change != nil {
		change(&p)
	}
	b, e := json.Marshal(p)
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func TestFineImportPayloadAndPortsRefusedBeforeIO(t *testing.T) {
	for _, change := range []func(*ImportPayload){func(p *ImportPayload) { p.Source = "pmqc" }, func(p *ImportPayload) { p.Stream = "enforcement-v1" }, func(p *ImportPayload) { p.Member = "anything.csv" }, func(p *ImportPayload) { p.Edition = "2024" }, func(p *ImportPayload) { p.URL += "?private=secret" }, func(p *ImportPayload) { p.SHA256 = strings.Repeat("A", 64) }, func(p *ImportPayload) { p.Version = 2 }} {
		if _, e := decodeFinePayload(fineJobPayload(t, change)); e == nil || strings.Contains(e.Error(), "secret") {
			t.Fatal("unsafe fine payload accepted/leaked")
		}
	}
	raw := fineJobPayload(t, nil)
	for _, text := range []string{string(raw[:len(raw)-1]) + `,"version":1}`, string(raw[:len(raw)-1]) + `,"private":"secret"}`, strings.Repeat("x", 4097)} {
		if _, e := decodeFinePayload([]byte(text)); e == nil {
			t.Fatal("ambiguous fine payload accepted")
		}
	}
	if _, e := decodeFinePayload(raw); e != nil {
		t.Fatal("valid selection refused", e)
	}
	calls := 0
	im := FineFileImport{Fetch: func(context.Context, string) (FileArtifact, error) { calls++; return FileArtifact{}, nil }}
	if e := im.Handle(context.Background(), platformjobs.Job{Payload: raw}); e == nil || calls != 0 {
		t.Fatal("missing fine ports performed I/O")
	}
}
func TestFineImportPrepublicationFailuresCleanAndRedact(t *testing.T) {
	for _, period := range []string{"2025", "2024"} {
		t.Run(period, func(t *testing.T) { testFineImportPrepublicationFailuresCleanAndRedact(t, period) })
	}
}
func testFineImportPrepublicationFailuresCleanAndRedact(t *testing.T, period string) {
	for _, mode := range []string{"fetch", "prepare", "tamper", "cancel", "cleanup"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			file := filepath.Join(root, "source.csv")
			raw := []byte("private synthetic source\n")
			sum := sha256.Sum256(raw)
			hash := hex.EncodeToString(sum[:])
			closed := 0
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			im := FineFileImport{Pool: &pgxpool.Pool{}, Publish: &adapters.Publisher{}, Serial: func(ctx context.Context, f func(context.Context) error) error { return f(ctx) }}
			im.Fetch = func(context.Context, string) (FileArtifact, error) {
				if e := os.WriteFile(file, raw, 0600); e != nil {
					t.Fatal(e)
				}
				artifact := FileArtifact{Path: file, Identity: prepared.InputIdentity{SHA256: hash, Bytes: int64(len(raw))}, Close: func() error {
					closed++
					_ = os.Remove(file)
					if mode == "cleanup" {
						return errors.New("private cleanup body")
					}
					return nil
				}}
				if mode == "fetch" {
					return artifact, errors.New("private fetch body")
				}
				return artifact, nil
			}
			im.Prepare = func(context.Context, ImportPayload, FileArtifact) (prepared.FineBatch, error) {
				if mode == "prepare" {
					return prepared.FineBatch{}, errors.New("private prepare body")
				}
				if mode == "tamper" {
					_ = os.WriteFile(file, []byte("altered source"), 0600)
				}
				if mode == "cancel" {
					cancel()
				}
				return prepared.FineBatch{ReportYear: 2025, ExtractionMonth: "2026-01", ManifestHash: strings.Repeat("b", 64)}, nil
			}
			im.PrepareHistory = func(ctx context.Context, payload ImportPayload, f FileArtifact) (prepared.FineHistoryBatch, error) {
				_, e := im.Prepare(ctx, payload, f)
				return prepared.FineHistoryBatch{SourcePeriod: period, InputContract: "anp-sicom-utf8bom-2024-v1", SourceFile: "multas-aplicadas-2024.csv", HeaderRecord: 7, Clock: application.FineHistoryClock{Precision: "MONTH", Value: "2025-03"}, ManifestHash: strings.Repeat("b", 64)}, e
			}
			payload := fineJobPayload(t, func(p *ImportPayload) {
				p.SHA256 = hash
				p.Edition = period
				p.URL = strings.Replace(unitFineURL, "2025.csv", period+".csv", 1)
			})
			e := im.Handle(ctx, platformjobs.Job{Payload: payload})
			if e == nil || strings.Contains(e.Error(), "private") || closed != 1 {
				t.Fatal("fine failure escaped/redacted/cleanup contract", e, closed)
			}
		})
	}
}

func TestHistoricalFineRequiresItsOwnPreparationPortBeforeIO(t *testing.T) {
	calls := 0
	im := FineFileImport{Pool: &pgxpool.Pool{}, Publish: &adapters.Publisher{}, Serial: func(ctx context.Context, f func(context.Context) error) error { calls++; return f(ctx) }, Fetch: func(context.Context, string) (FileArtifact, error) { calls++; return FileArtifact{}, nil }, Prepare: func(context.Context, ImportPayload, FileArtifact) (prepared.FineBatch, error) {
		calls++
		return prepared.FineBatch{}, nil
	}}
	payload := fineJobPayload(t, func(p *ImportPayload) {
		p.Edition = "2024"
		p.URL = strings.Replace(unitFineURL, "2025.csv", "2024.csv", 1)
	})
	if _, e := decodeFinePayload(payload); e != nil {
		t.Fatal("verified historical payload refused", e)
	}
	if e := im.Handle(context.Background(), platformjobs.Job{Payload: payload}); e == nil || calls != 0 {
		t.Fatal("historical job used v1 port/performed I/O", e, calls)
	}
}
