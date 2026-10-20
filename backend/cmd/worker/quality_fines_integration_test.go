//go:build integration

package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	qualityqueries "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/quality"
	directory "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/adapters"
	qualityjobs "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters/jobs"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters/native"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters/prepared"
	qualityread "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters/read"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/domain"
	platformjobs "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
)

func TestNativeFineWorkerDiscoveryDispatchRustAndCorrection(t *testing.T) {
	for _, period := range []string{"2025", "2024", "2021", "2016-2019"} {
		t.Run(period, func(t *testing.T) { testFineWorkerDiscoveryDispatchRustAndCorrection(t, period) })
	}
}
func testFineWorkerDiscoveryDispatchRustAndCorrection(t *testing.T, period string) {
	binary := os.Getenv("ANPFUEL_TEST_SANCTIONS_PREPARER_PATH")
	if binary == "" {
		t.Fatal("ANPFUEL_TEST_SANCTIONS_PREPARER_PATH must name the trusted fine CLI")
	}
	p := freshNativeWorkerDB(t)
	ctx := context.Background()
	repo := directory.NewRepository(p)
	station, e := repo.ResolveCNPJ(ctx, "11222333000181", "Synthetic worker company", nil)
	if e != nil {
		t.Fatal(e)
	}
	root := t.TempDir()
	raw := []byte("ANP - Ag\x88ncia Nacional do Petroleo, G\xa0s Natural e Biocombust\xa1veis;;;;;;;;\n;;;;;;;;\nRELA\x80\xc7O DE MULTAS APLICADAS EM 2025  (Relat\xa2rio extra\xa1do em janeiro/2026);;;;;;;;\nFonte: Sistema de Controle de Multas - SICOM;;;;;;;;\n;;;;;;;;\n;;;;;;;;\nStatus Processo;Superintend\x88ncia;N\xa3mero do Processo;N\xa3mero do DUF;CNPJ/CPF;Raz\xc6o Social;Vencimento GRM;Valor da Multa;Valor Total Pago\nSource original;SYNTHETIC;000000000000001;SYNTHETIC/1;11.222.333/0001-81;Discarded synthetic name;28/02/2025;150000,00;180000,01\n")
	ref := native.FineRef{URL: "https://www.gov.br" + native.FineResourcePrefix + "multas-aplicadas-2025.csv", Period: "2025"}
	clock := "2026-01"
	if period != "2025" {
		raw, clock = workerHistoricalFineCSV(period)
		profile, _ := application.HistoricalFineProfile(period)
		ref = native.FineRef{URL: "https://www.gov.br" + native.FineResourcePrefix + profile.SourceFile, Period: period}
	}
	file := func() (qualityjobs.FileArtifact, error) {
		filename := filepath.Join(root, "download.csv")
		if e := os.WriteFile(filename, raw, 0600); e != nil {
			return qualityjobs.FileArtifact{}, e
		}
		sum := sha256.Sum256(raw)
		return qualityjobs.FileArtifact{Path: filename, Identity: prepared.InputIdentity{SHA256: hex.EncodeToString(sum[:]), Bytes: int64(len(raw))}, Close: func() error { return os.Remove(filename) }}, nil
	}
	d := nativeFineLiveDiscovery(p)
	// Replace network delivery only; page parser, file integrity, cursor/queue,
	// native Rust process, source lease, publication and readers are real.
	d.List = func(context.Context) ([]native.FineRef, error) {
		return native.ParseFineListingPage(native.FineListingPage, []byte(`<a href="`+ref.URL+`">synthetic label</a>`))
	}
	d.Inspect = func(ctx context.Context, r native.FineRef) (entry application.InventoryEntry, err error) {
		f, e := file()
		if e != nil {
			return entry, e
		}
		defer func() {
			if e := f.Close(); e != nil {
				entry = application.InventoryEntry{}
				err = e
			}
		}()
		return native.InspectFineFile(ctx, f.Path, r, f.Identity)
	}
	im := nativeFineImport(p, binary, os.Getenv("ANPFUEL_TEST_FINE_HISTORY_PREPARER_PATH"), root)
	im.Fetch = func(_ context.Context, url string) (qualityjobs.FileArtifact, error) {
		if url != ref.URL {
			t.Fatal("unexpected URL")
		}
		return file()
	}
	handlers := map[string]platformjobs.Handler{qualityjobs.NativeFineImportKind: im}
	for _, kind := range []string{"evidence-sweep", "account-erasure-reconcile", "privacy-training-reconcile", "privacy-training-sweep"} {
		handlers[kind] = dispatcherHandler(kind)
	}
	dispatchers, e := workerDispatchers(platformjobs.NewQueue(p), handlers, qualityjobs.NativeFineImportKind)
	if e != nil {
		t.Fatal(e)
	}
	importer := dispatchers[len(dispatchers)-1]
	if e = d.Handle(ctx, platformjobs.Job{}); e != nil {
		t.Fatal(e)
	}
	for range 2 {
		if e = d.Handle(ctx, platformjobs.Job{}); e != nil {
			t.Fatal(e)
		}
	}
	if found, e := importer.RunOnce(ctx); e != nil || !found {
		t.Fatal("native fine not dispatched", found, e)
	}
	if found, e := importer.RunOnce(ctx); e != nil || found {
		t.Fatal("duplicate jobs", found, e)
	}
	r := qualityread.NewReader(p)
	rows, _, lease, e := r.Sanctions(ctx, time.Now(), "sanctions", "sanction-v1", "", "", "", 10)
	if e != nil || len(rows) != 1 || rows[0].DueOn != "2025-02-28" || rows[0].Status != "Source original" || lease.SourceClockValue != clock {
		t.Fatal("native source facts lost", e)
	}
	first := lease.RevisionID
	if e = d.Handle(ctx, platformjobs.Job{}); e != nil {
		t.Fatal(e)
	}
	if found, e := importer.RunOnce(ctx); e != nil || found {
		t.Fatal("published stable content enqueued", found, e)
	}
	// A same-URL correction receives a different frozen hash and one new job.
	// Append one distinct source observation in the selected exact profile.
	lines := strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n")
	corrected := strings.Replace(lines[len(lines)-1], "Source original", "Source corrected", 1)
	raw = append(raw, []byte(corrected+"\n")...)
	if e = d.Handle(ctx, platformjobs.Job{}); e != nil {
		t.Fatal(e)
	}
	if found, e := importer.RunOnce(ctx); e != nil || !found {
		t.Fatal("correction not dispatched", found, e)
	}
	rows, _, lease, e = r.Sanctions(ctx, time.Now(), "sanctions", "sanction-v1", "", "", "", 10)
	if e != nil || len(rows) != 2 || lease.RevisionID == first {
		t.Fatal("correction lost", e)
	}
	old, _, _, e := r.AtRevision(first).Sanctions(ctx, time.Now(), "sanctions", "sanction-v1", "", "", "", 10)
	if e != nil || len(old) != 1 || old[0].Status != "Source original" {
		t.Fatal("frozen history changed", e)
	}
	var jobs, done, revisions, links int
	if e = p.QueryRow(ctx, `SELECT (SELECT count(*) FROM job_queue WHERE kind=$1),(SELECT count(*) FROM job_queue WHERE kind=$1 AND status='done' AND attempts=1),(SELECT count(*) FROM quality_revisions),(SELECT count(*) FROM quality_links)`, qualityjobs.NativeFineImportKind).Scan(&jobs, &done, &revisions, &links); e != nil || jobs != 2 || done != 2 || revisions != 2 || links != 0 {
		t.Fatal("queue/publication/identity mismatch", jobs, done, revisions, links, e)
	}
	var bound, companyRows, stationRows, identifiers int
	if e := p.QueryRow(ctx, `SELECT (SELECT count(*) FROM quality_company_associations WHERE resolution_kind='bound'),(SELECT count(*) FROM quality_company_associations),(SELECT count(*) FROM directory_stations),(SELECT count(*) FROM directory_identifiers)`).Scan(&bound, &companyRows, &stationRows, &identifiers); e != nil || bound != 3 || companyRows != 3 || stationRows != 1 || identifiers != 1 {
		t.Fatal("worker company scope/no-write mismatch", bound, companyRows, stationRows, identifiers, e)
	}
	var correct int
	if e := p.QueryRow(ctx, `SELECT count(*) FROM quality_company_associations WHERE station_id=$1`, station.ID).Scan(&correct); e != nil || correct != 3 {
		t.Fatal("worker declared wrong company station", e)
	}
	files, e := os.ReadDir(root)
	if e != nil || len(files) != 0 {
		t.Fatal("source files retained", e)
	}
}

func workerHistoricalFineCSV(period string) ([]byte, string) {
	agency := "ANP - Agência Nacional do Petroleo, Gás Natural e Biocombustíveis"
	header := "Status Processo;Superintendência;Número do Processo;Número do DUF;CNPJ/CPF;Razão Social;Vencimento;Valor da Multa;Valor Total Pago;"
	row := "Source original;SYNTHETIC;000000000000001;SYNTHETIC/1;11.222.333/0001-81;Discarded synthetic name;28/02/2025;150000,00;180000,01;SIM\n"
	clock := "2025-03"
	preamble := "\ufeff" + agency + ";;;;;;;;;\n;;;;;;;;;\nRELAÇÃO DE MULTAS APLICADAS EM 2024  (Relatório extraído em março/2025);;;;;;;;;\nFonte: Sistema de Controle de Multas - SICOM;;;;;;;;;\n;;;;;;;;;\n;;;;;;;;;\n"
	if period != "2024" {
		header = "Status Processo;Superintendência;Número do Processo;Auto de Infração;CNPJ/CPF;Razão Social;Data Transito Julgado;Vencimento;Valor  da Multa Aplicada;Valor Total Pago (*) (**) "
		row = "Source original;SYNTHETIC;000000000000001;SYNTHETIC/1;11.222.333/0001-81;Discarded synthetic name;20/02/2021;28/02/2025;150000,00;180000,01\n"
		preamble = strings.Replace(preamble, "2024  (Relatório extraído em março/2025)", "2021  (Relatório extraído em junho/2022)", 1)
		clock = "2022-06"
	}
	if period == "2016-2019" {
		preamble = "\ufeff" + agency + ";;;;Data: 06/02/2020;;;;;\nSICOM;;;;;;;;;\nFinanceiro (GRM);;;;;;;;;\n;;;;;;;;;\n"
		header = "Status Processo;Superintendência;Número do Processo;Auto de Infração;CNPJ/CPF;Razão Social;Data Transito Julgado;Vencimento ;Valor da Multa Aplicada;Valor Total Pago (*) (**)"
		clock = "2020-02-06"
	}
	return []byte(preamble + header + "\n" + row), clock
}

func TestFineWorkerCompanyRetirementCleanupAndAcknowledgmentRecovery(t *testing.T) {
	p := freshNativeWorkerDB(t)
	ctx := context.Background()
	root := t.TempDir()
	repo := directory.NewRepository(p)
	if _, e := repo.ResolveCNPJ(ctx, "11222333000181", "Synthetic retirement worker", nil); e != nil {
		t.Fatal(e)
	}
	raw, _ := workerHistoricalFineCSV("2024")
	im := nativeFineImport(p, os.Getenv("ANPFUEL_TEST_SANCTIONS_PREPARER_PATH"), os.Getenv("ANPFUEL_TEST_FINE_HISTORY_PREPARER_PATH"), root)
	url := "https://www.gov.br" + native.FineResourcePrefix + "multas-aplicadas-2024.csv"
	im.Fetch = func(context.Context, string) (qualityjobs.FileArtifact, error) {
		path := filepath.Join(root, "private-download")
		if e := os.WriteFile(path, raw, 0600); e != nil {
			return qualityjobs.FileArtifact{}, e
		}
		sum := sha256.Sum256(raw)
		return qualityjobs.FileArtifact{Path: path, Identity: prepared.InputIdentity{SHA256: hex.EncodeToString(sum[:]), Bytes: int64(len(raw))}, Close: func() error { return os.Remove(path) }}, nil
	}
	job := func() platformjobs.Job {
		sum := sha256.Sum256(raw)
		payload, e := json.Marshal(qualityjobs.ImportPayload{Version: 1, Source: "sanctions", Stream: "sanction-v1", Edition: "2024", URL: url, SHA256: hex.EncodeToString(sum[:])})
		if e != nil {
			t.Fatal(e)
		}
		return platformjobs.Job{Payload: payload}
	}
	reader := qualityread.NewReader(p)
	q := qualityqueries.New(p)
	current := func() string {
		_, _, lease, e := reader.Sanctions(ctx, time.Now(), "sanctions", "sanction-v1", "", "", "", 10)
		if e != nil {
			t.Fatal(e)
		}
		return lease.RevisionID
	}
	mark := func() string {
		m, e := q.GetWatermark(ctx, qualityqueries.GetWatermarkParams{Source: "sanctions", Stream: "sanction-v1"})
		if e != nil {
			t.Fatal(e)
		}
		return m.LastHash
	}
	clean := func() {
		files, e := os.ReadDir(root)
		if e != nil || len(files) != 0 {
			t.Fatal("worker retained private source on failure/replay", e)
		}
	}
	if e := im.Handle(ctx, job()); e != nil {
		t.Fatal(e)
	}
	first, firstMark := current(), mark()
	clean()
	originalPlanner := im.Associate
	retire := true
	im.Associate = func(ctx context.Context, records []domain.CompanyObservationRecord) (*application.CompanyAssociationInput, error) {
		plan, e := originalPlanner(ctx, records)
		if e == nil && retire {
			retire = false
			e = repo.RetireIdentifier(ctx, "CNPJ", "11222333000181")
		}
		return plan, e
	}
	raw = []byte(strings.Replace(string(raw), "Source original", "Source corrected", 1))
	correction := job()
	if e := im.Handle(ctx, correction); e == nil || strings.Contains(e.Error(), "11222333000181") {
		t.Fatal("retired prepared company published/leaked identity", e)
	}
	clean()
	if current() != first || mark() != firstMark {
		t.Fatal("guard refusal advanced pointer/watermark")
	}
	if e := im.Handle(ctx, correction); e != nil {
		t.Fatal("fresh review correction failed", e)
	}
	second := current()
	if second == first {
		t.Fatal("retirement review did not correct source association")
	}
	clean()
	var currentBound int
	if e := p.QueryRow(ctx, `SELECT count(*) FROM quality_company_associations WHERE revision_id=$1 AND station_id IS NOT NULL`, second).Scan(&currentBound); e != nil || currentBound != 0 {
		t.Fatal("retired source still assigned company station", e)
	}
	if _, e := p.Exec(ctx, `CREATE FUNCTION synthetic_company_worker_watermark() RETURNS trigger LANGUAGE plpgsql AS $fail$ BEGIN RAISE EXCEPTION 'synthetic failure'; END $fail$; CREATE TRIGGER synthetic_company_worker_watermark BEFORE INSERT OR UPDATE ON quality_watermarks FOR EACH ROW EXECUTE FUNCTION synthetic_company_worker_watermark()`); e != nil {
		t.Fatal(e)
	}
	previousMark := mark()
	raw = []byte(strings.Replace(string(raw), "Source corrected", "Source recovery", 1))
	recovery := job()
	if e := im.Handle(ctx, recovery); e == nil {
		t.Fatal("company watermark failure acknowledged")
	}
	frozen := current()
	if frozen == second || mark() != previousMark {
		t.Fatal("watermark failure changed wrong state")
	}
	clean()
	if _, e := p.Exec(ctx, `DROP TRIGGER synthetic_company_worker_watermark ON quality_watermarks; DROP FUNCTION synthetic_company_worker_watermark()`); e != nil {
		t.Fatal(e)
	}
	for range 2 {
		if e := im.Handle(ctx, recovery); e != nil {
			t.Fatal("company recovery/replay", e)
		}
	}
	if current() != frozen || mark() == previousMark {
		t.Fatal("company replay forked or failed acknowledgment")
	}
	clean()
	var revisions, contexts, decisions, stations, identifiers, datedLinks int
	if e := p.QueryRow(ctx, `SELECT (SELECT count(*) FROM quality_revisions),(SELECT count(*) FROM quality_company_association_revisions),(SELECT count(*) FROM quality_company_associations),(SELECT count(*) FROM directory_stations),(SELECT count(*) FROM directory_identifiers),(SELECT count(*) FROM quality_links)`).Scan(&revisions, &contexts, &decisions, &stations, &identifiers, &datedLinks); e != nil || revisions != 3 || contexts != 3 || decisions != 3 || stations != 1 || identifiers != 1 || datedLinks != 0 {
		t.Fatal("worker company recovery/census/no-write mismatch", revisions, contexts, decisions, stations, identifiers, datedLinks, e)
	}
}
