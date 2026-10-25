//go:build integration

package jobs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	quality "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/quality"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters/native"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters/prepared"
	qualityread "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters/read"
	platformjobs "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/jackc/pgx/v5"
)

func nativeFineCSV(status string) []byte {
	preamble := "ANP - Ag\x88ncia Nacional do Petroleo, G\xa0s Natural e Biocombust\xa1veis;;;;;;;;\n;;;;;;;;\nRELA\x80\xc7O DE MULTAS APLICADAS EM 2025  (Relat\xa2rio extra\xa1do em janeiro/2026);;;;;;;;\nFonte: Sistema de Controle de Multas - SICOM;;;;;;;;\n;;;;;;;;\n;;;;;;;;\nStatus Processo;Superintend\x88ncia;N\xa3mero do Processo;N\xa3mero do DUF;CNPJ/CPF;Raz\xc6o Social;Vencimento GRM;Valor da Multa;Valor Total Pago\n"
	row := status + ";SYNTHETIC;000000000000001;SYNTHETIC/1;11.222.333/0001-81;Discarded synthetic company;28/02/2025;150000,00;180000,01\n"
	cpf := strings.Replace(row, "11.222.333/0001-81", "000.000.000-00", 1)
	return []byte(preamble + row + row + cpf)
}
func nativeFineJob(t *testing.T, raw []byte) platformjobs.Job {
	t.Helper()
	sum := sha256.Sum256(raw)
	payload := fineJobPayload(t, func(p *ImportPayload) { p.SHA256 = hex.EncodeToString(sum[:]) })
	return platformjobs.Job{Payload: payload}
}
func TestNativeFineFileRustPostGISRecoveryCorrectionAndCleanup(t *testing.T) {
	for _, period := range []string{"2025", "2024"} {
		t.Run(period, func(t *testing.T) { testFineFileRustPostGISRecoveryCorrectionAndCleanup(t, period) })
	}
}
func testFineFileRustPostGISRecoveryCorrectionAndCleanup(t *testing.T, period string) {
	binary := os.Getenv("ANPFUEL_TEST_SANCTIONS_PREPARER_PATH")
	if binary == "" {
		t.Fatal("ANPFUEL_TEST_SANCTIONS_PREPARER_PATH must name the trusted Rust fine CLI")
	}
	p := freshJobsDB(t)
	ctx := context.Background()
	root := t.TempDir()
	csv := nativeFineCSV
	if period == "2024" {
		csv = historicalFineCSV
	}
	makeJob := func(raw []byte) platformjobs.Job {
		sum := sha256.Sum256(raw)
		return platformjobs.Job{Payload: fineJobPayload(t, func(p *ImportPayload) {
			p.SHA256 = hex.EncodeToString(sum[:])
			p.Edition = period
			p.URL = "https://www.gov.br" + native.FineResourcePrefix + "multas-aplicadas-" + period + ".csv"
		})}
	}
	raw := csv("Source original")
	original := append([]byte{}, raw...)
	job := makeJob(raw)
	im := FineFileImport{Pool: p, Publish: adapters.NewPublisher(p), Serial: adapters.NativeSourceSerial(p)}
	hookCalls := 0
	hookRevisions := map[string]int{}
	im.AfterPublish = func(_ context.Context, revisionID string) error {
		if revisionID == "" {
			t.Fatal("empty after-publish revision")
		}
		hookCalls++
		hookRevisions[revisionID]++
		return nil
	}
	cleanupFailure := false
	closed := 0
	im.Fetch = func(context.Context, string) (FileArtifact, error) {
		filename := filepath.Join(root, "download.csv")
		if e := os.WriteFile(filename, raw, 0600); e != nil {
			return FileArtifact{}, e
		}
		sum := sha256.Sum256(raw)
		return FileArtifact{Path: filename, Identity: prepared.InputIdentity{SHA256: hex.EncodeToString(sum[:]), Bytes: int64(len(raw))}, Close: func() error {
			closed++
			e := os.Remove(filename)
			if e == nil && cleanupFailure {
				return errors.New("synthetic cleanup refusal")
			}
			return e
		}}, nil
	}
	im.Prepare = func(ctx context.Context, payload ImportPayload, file FileArtifact) (prepared.FineBatch, error) {
		return (native.FineRunner{Binary: binary, WorkspaceRoot: root}).Prepare(ctx, native.FineRequest{Path: file.Path, URL: payload.URL, Edition: payload.Edition, Identity: file.Identity})
	}
	im.PrepareHistory = func(ctx context.Context, payload ImportPayload, file FileArtifact) (prepared.FineHistoryBatch, error) {
		return (native.FineHistoryRunner{Binary: os.Getenv("ANPFUEL_TEST_FINE_HISTORY_PREPARER_PATH"), WorkspaceRoot: root}).Prepare(ctx, native.FineRequest{Path: file.Path, URL: payload.URL, Edition: payload.Edition, Identity: file.Identity})
	}
	// Cleanup is confirmed before publication, even when the typed native parse succeeded.
	cleanupFailure = true
	if e := im.Handle(ctx, job); e == nil {
		t.Fatal("cleanup failure acknowledged")
	}
	var n int
	if e := p.QueryRow(ctx, "SELECT count(*) FROM quality_revisions").Scan(&n); e != nil || n != 0 {
		t.Fatal("cleanup failure partially published", n, e)
	}
	cleanupFailure = false
	// A post-publication watermark failure must replay the same frozen revision.
	if _, e := p.Exec(ctx, `CREATE FUNCTION synthetic_fine_watermark_failure() RETURNS trigger LANGUAGE plpgsql AS $test$ BEGIN RAISE EXCEPTION 'synthetic failure'; END $test$;
 CREATE TRIGGER synthetic_fine_watermark_failure BEFORE INSERT OR UPDATE ON quality_watermarks FOR EACH ROW EXECUTE FUNCTION synthetic_fine_watermark_failure()`); e != nil {
		t.Fatal(e)
	}
	if e := im.Handle(ctx, job); e == nil || strings.Contains(e.Error(), "synthetic") {
		t.Fatal("watermark failure acknowledged/leaked", e)
	}
	reader := qualityread.NewReader(p)
	rows, _, lease, e := reader.Sanctions(ctx, time.Now(), "sanctions", "sanction-v1", "", "", "", 10)
	if e != nil || len(rows) != 1 || lease.Counts.Input != 3 || lease.Counts.Accepted != 1 || lease.Counts.Duplicates != 1 || lease.Counts.Quarantined != 1 || rows[0].DueOn != "2025-02-28" || rows[0].Status != "Source original" || (period == "2024" && (lease.SourceClockValue != "2025-03" || rows[0].UnlabelledField != "SIM")) {
		t.Fatal("actual Rust/PostGIS source mismatch", e)
	}
	first := lease.RevisionID
	if _, e = quality.New(p).GetWatermark(ctx, quality.GetWatermarkParams{Source: "sanctions", Stream: "sanction-v1"}); !errors.Is(e, pgx.ErrNoRows) {
		t.Fatal("failed watermark advanced", e)
	}
	if _, e = p.Exec(ctx, "DROP TRIGGER synthetic_fine_watermark_failure ON quality_watermarks; DROP FUNCTION synthetic_fine_watermark_failure()"); e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 2; i++ {
		if e = im.Handle(ctx, job); e != nil {
			t.Fatal("watermark recovery/replay", e)
		}
	}
	_, _, replayed, e := reader.Sanctions(ctx, time.Now(), "sanctions", "sanction-v1", "", "", "", 10)
	if e != nil || replayed.RevisionID != first {
		t.Fatal("replayed fine import forked", e)
	}
	raw = csv("Source correction")
	correction := makeJob(raw)
	if e = im.Handle(ctx, correction); e != nil {
		t.Fatal("native correction import", e)
	}
	_, _, latest, e := reader.Sanctions(ctx, time.Now(), "sanctions", "sanction-v1", "", "", "", 10)
	if e != nil || latest.RevisionID == first {
		t.Fatal("correction did not publish", e)
	}
	if e = im.Handle(ctx, job); e == nil {
		t.Fatal("old planned hash accepted changed bytes")
	}
	raw = original
	if e = im.Handle(ctx, job); e != nil {
		t.Fatal("superseded source replay should acknowledge safely", e)
	}
	_, _, current, e := reader.Sanctions(ctx, time.Now(), "sanctions", "sanction-v1", "", "", "", 10)
	if e != nil || current.RevisionID != latest.RevisionID {
		t.Fatal("old native source resurrected", e)
	}
	old, _, _, e := reader.AtRevision(first).Sanctions(ctx, time.Now(), "sanctions", "sanction-v1", "", "", "", 10)
	if e != nil || len(old) != 1 || old[0].Status != "Source original" {
		t.Fatal("original native history lost", e)
	}
	mark, e := quality.New(p).GetWatermark(ctx, quality.GetWatermarkParams{Source: "sanctions", Stream: "sanction-v1"})
	if e != nil || mark.LastEdition != period || mark.LastHash != latest.ManifestHash {
		t.Fatal("watermark drift", e)
	}
	raw = []byte("private invalid source body\n")
	if e = im.Handle(ctx, makeJob(raw)); e == nil || strings.Contains(e.Error(), "private invalid source") {
		t.Fatal("malformed native source accepted/leaked", e)
	}
	children, e := os.ReadDir(root)
	if e != nil || len(children) != 0 || closed != 8 {
		t.Fatal("owned native file cleanup", len(children), closed, e)
	}
	if e = p.QueryRow(ctx, "SELECT count(*) FROM quality_revisions").Scan(&n); e != nil || n != 2 {
		t.Fatal("failed/replayed import forked history", n, e)
	}
	if e = p.QueryRow(ctx, "SELECT count(*) FROM quality_links").Scan(&n); e != nil || n != 0 {
		t.Fatal("raw native import invented station association", n, e)
	}
	if hookCalls < 2 || len(hookRevisions) != 2 {
		t.Fatalf("after-publish hooks = %d/%v, want original + correction revisions", hookCalls, hookRevisions)
	}
}

func historicalFineCSV(status string) []byte {
	preamble := "\ufeffANP - Agência Nacional do Petroleo, Gás Natural e Biocombustíveis;;;;;;;;;\n;;;;;;;;;\nRELAÇÃO DE MULTAS APLICADAS EM 2024  (Relatório extraído em março/2025);;;;;;;;;\nFonte: Sistema de Controle de Multas - SICOM;;;;;;;;;\n;;;;;;;;;\n;;;;;;;;;\nStatus Processo;Superintendência;Número do Processo;Número do DUF;CNPJ/CPF;Razão Social;Vencimento;Valor da Multa;Valor Total Pago;\n"
	row := status + ";SYNTHETIC;000000000000001;SYNTHETIC/1;11.222.333/0001-81;Discarded synthetic company;28/02/2025;150000,00;180000,01;SIM\n"
	cpf := strings.Replace(row, "11.222.333/0001-81", "000.000.000-00", 1)
	return []byte(preamble + row + row + cpf)
}
