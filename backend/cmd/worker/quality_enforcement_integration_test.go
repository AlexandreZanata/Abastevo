//go:build integration

package main

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	qualityjobs "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters/jobs"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters/native"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters/prepared"
	qualityread "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters/read"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/application"
	platformjobs "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestEnforcementDisabledJobsKeepAttempts(t *testing.T) {
	p := freshNativeWorkerDB(t)
	ctx := context.Background()
	handlers := map[string]platformjobs.Handler{}
	for _, kind := range []string{"evidence-sweep", "account-erasure-reconcile", "privacy-training-reconcile", "privacy-training-sweep"} {
		handlers[kind] = dispatcherHandler(kind)
	}
	ds, e := workerDispatchers(platformjobs.NewQueue(p), handlers)
	if e != nil {
		t.Fatal(e)
	}
	for _, kind := range []string{"quality-enforcement-import", "quality-enforcement-discovery"} {
		if _, e := platformjobs.Enqueue(ctx, p, kind, []byte(`{"version":1}`), kind, 3, time.Time{}); e != nil {
			t.Fatal(e)
		}
	}
	if found, e := ds[0].RunOnce(ctx); e != nil || found {
		t.Fatal("disabled enforcement work was claimed", found, e)
	}
	var attempts int
	if e := p.QueryRow(ctx, "SELECT sum(attempts) FROM job_queue WHERE kind LIKE 'quality-enforcement-%'").Scan(&attempts); e != nil || attempts != 0 {
		t.Fatal("disabled attempt charged", attempts, e)
	}
}

func workerEnforcementWorkbook(result string) []byte {
	index := map[string]int{}
	var shared strings.Builder
	shared.WriteString(`<sst xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">`)
	position := func(value string) int {
		if i, ok := index[value]; ok {
			return i
		}
		i := len(index)
		index[value] = i
		shared.WriteString("<si><t>" + value + "</t></si>")
		return i
	}
	cell := func(letter string, number int, value string, numeric bool) string {
		if numeric {
			return fmt.Sprintf(`<c r="%s%d"><v>%s</v></c>`, letter, number, value)
		}
		return fmt.Sprintf(`<c r="%s%d" t="s"><v>%d</v></c>`, letter, number, position(value))
	}
	letters := []string{"A", "B", "C", "D", "E", "F", "G", "H", "I", "J", "K"}
	var sheet strings.Builder
	sheet.WriteString(`<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>`)
	rows := [][]string{
		{"UF", "Município", "Bairro", "ENDEREÇO", "CNPJ/CPF", "Agente Econômico", "Segmento Fiscalizado", "DATA DO DF", "Número do Documento", "Procedimento de Fiscalização", "Resultado"},
		{"DF", "BRASILIA", "Centro Sintético", "Rua Sintética 1", "36754752000195", "Empresa Sintética Ltda", "Revenda de Combustíveis", "43467", "712508", "Procedimento sintético", result},
		{"SP", "CAMPINAS", "Centro Sintético", "Rua Sintética 2", "12345678901", "Pessoa Sintética", "Revenda de Combustíveis", "43468", "712509", "Procedimento sintético", "Resultado curto"},
	}
	for i, row := range rows {
		number := i + 1
		sheet.WriteString(fmt.Sprintf(`<row r="%d">`, number))
		for j, value := range row {
			numeric := i > 0 && j == 7
			sheet.WriteString(cell(letters[j], number, value, numeric))
		}
		sheet.WriteString(`</row>`)
	}
	sheet.WriteString(`</sheetData></worksheet>`)
	shared.WriteString(`</sst>`)
	workbook := `<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><workbookPr defaultThemeVersion="202300"/><sheets><sheet name="v_dados_abertos_dfs" sheetId="1" r:id="rId1"/></sheets></workbook>`
	rels := `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/></Relationships>`
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for _, member := range []struct{ name, body string }{
		{"xl/workbook.xml", workbook},
		{"xl/_rels/workbook.xml.rels", rels},
		{"xl/sharedStrings.xml", shared.String()},
		{"xl/worksheets/sheet1.xml", sheet.String()},
	} {
		f, err := w.Create(member.name)
		if err != nil {
			panic(err)
		}
		if _, err := io.WriteString(f, member.body); err != nil {
			panic(err)
		}
	}
	if err := w.Close(); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

func TestEnforcementWorkerNativeDiscoveryCorrectionAckLossCleanupAndDeadBudget(t *testing.T) {
	binary := os.Getenv("ANPFUEL_TEST_ENFORCEMENT_PREPARER_PATH")
	if binary == "" {
		t.Fatal("trusted enforcement CLI required")
	}
	p := freshNativeWorkerDB(t)
	ctx := context.Background()
	root := t.TempDir()
	raw := workerEnforcementWorkbook("Original source result")
	file := func() (qualityjobs.FileArtifact, error) {
		path := filepath.Join(root, "download.xlsx")
		if e := os.WriteFile(path, raw, 0600); e != nil {
			return qualityjobs.FileArtifact{}, e
		}
		sum := sha256.Sum256(raw)
		return qualityjobs.FileArtifact{Path: path, Identity: prepared.InputIdentity{SHA256: hex.EncodeToString(sum[:]), Bytes: int64(len(raw))}, Close: func() error { return os.Remove(path) }}, nil
	}
	d := nativeEnforcementLiveDiscovery(p)
	d.List = func(context.Context) error {
		return native.ParseEnforcementListing(native.EnforcementListingPage, []byte(`<a href="`+native.EnforcementResourceURL+`">source</a>`))
	}
	d.Inspect = func(ctx context.Context) (application.InventoryEntry, error) {
		f, e := file()
		if e != nil {
			return application.InventoryEntry{}, e
		}
		defer f.Close()
		return native.InspectEnforcementFile(ctx, f.Path, f.Identity)
	}
	im := nativeEnforcementImport(p, binary, root)
	im.Fetch = func(context.Context, string) (qualityjobs.FileArtifact, error) { return file() }
	handlers := map[string]platformjobs.Handler{qualityjobs.EnforcementImportKind: im}
	for _, kind := range []string{"evidence-sweep", "account-erasure-reconcile", "privacy-training-reconcile", "privacy-training-sweep"} {
		handlers[kind] = dispatcherHandler(kind)
	}
	ds, e := workerDispatchers(platformjobs.NewQueue(p), handlers, qualityjobs.EnforcementImportKind)
	if e != nil {
		t.Fatal(e)
	}
	dispatch := ds[len(ds)-1]
	dispatch.RetryDelay = time.Nanosecond
	for range 3 {
		if e = d.Handle(ctx, platformjobs.Job{}); e != nil {
			t.Fatal(e)
		}
	}
	if found, e := dispatch.RunOnce(ctx); e != nil || !found {
		t.Fatal("source not dispatched", found, e)
	}
	reader := qualityread.NewReader(p)
	rows, _, lease, e := reader.Enforcement(ctx, time.Now(), "enforcement", "enforcement-v1", "", "", "", "", 100)
	if e != nil || len(rows) != 2 || !lease.Stale {
		var reason string
		_ = p.QueryRow(ctx, "SELECT last_error FROM job_queue WHERE kind=$1", qualityjobs.EnforcementImportKind).Scan(&reason)
		t.Fatal("native source facts lost", len(rows), reason, e)
	}
	first := lease.RevisionID
	var payload []byte
	if e = p.QueryRow(ctx, "SELECT payload FROM job_queue WHERE kind=$1", qualityjobs.EnforcementImportKind).Scan(&payload); e != nil {
		t.Fatal(e)
	}
	if e = im.Handle(ctx, platformjobs.Job{Payload: payload}); e != nil {
		t.Fatal("ack replay", e)
	}
	// A correction commits but its durable acknowledgement fails. A retry must
	// replay that exact publication, never resurrect the predecessor or fork it.
	raw = workerEnforcementWorkbook("Corrected source result")
	if _, e = p.Exec(ctx, `CREATE FUNCTION refuse_enforcement_ack() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'synthetic acknowledgement loss'; END $$; CREATE TRIGGER refuse_ack BEFORE INSERT OR UPDATE ON quality_watermarks FOR EACH ROW EXECUTE FUNCTION refuse_enforcement_ack()`); e != nil {
		t.Fatal(e)
	}
	if e = d.Handle(ctx, platformjobs.Job{}); e != nil {
		t.Fatal(e)
	}
	if found, e := dispatch.RunOnce(ctx); e != nil || !found {
		t.Fatal("correction not dispatched", e)
	}
	_, _, corrected, e := reader.Enforcement(ctx, time.Now(), "enforcement", "enforcement-v1", "", "", "", "", 100)
	if e != nil || corrected.RevisionID == first {
		t.Fatal("correction did not commit before ack loss", e)
	}
	if _, e = p.Exec(ctx, "DROP TRIGGER refuse_ack ON quality_watermarks"); e != nil {
		t.Fatal(e)
	}
	if found, e := dispatch.RunOnce(ctx); e != nil || !found {
		t.Fatal("ack retry missing", e)
	}
	var revisions int
	if e = p.QueryRow(ctx, "SELECT count(*) FROM quality_enforcement_source_revisions").Scan(&revisions); e != nil || revisions != 2 {
		t.Fatal("ack retry forked", revisions, e)
	}
	old, _, _, e := reader.AtRevision(first).Enforcement(ctx, time.Now(), "enforcement", "enforcement-v1", "", "", "", "", 100)
	if e != nil || len(old) != 2 {
		t.Fatal("frozen source changed", e)
	}
	// Close returning nil is insufficient when the raw file still exists.
	raw = workerEnforcementWorkbook("Must never publish")
	sum := sha256.Sum256(raw)
	jobraw, _ := json.Marshal(qualityjobs.ImportPayload{Version: 1, Source: "enforcement", Stream: "enforcement-v1", Edition: application.EnforcementSourceEdition, URL: native.EnforcementResourceURL, SHA256: hex.EncodeToString(sum[:])})
	refused := im
	refused.Fetch = func(context.Context, string) (qualityjobs.FileArtifact, error) {
		f, e := file()
		f.Close = func() error { return nil }
		return f, e
	}
	if e = refused.Handle(ctx, platformjobs.Job{Payload: jobraw}); e == nil || !strings.Contains(e.Error(), "retained") {
		t.Fatal("raw retention published", e)
	}
	if e = p.QueryRow(ctx, "SELECT count(*) FROM quality_enforcement_source_revisions").Scan(&revisions); e != nil || revisions != 2 {
		t.Fatal("cleanup failure published", e)
	}
	_ = os.Remove(filepath.Join(root, "download.xlsx"))
	if e = d.Handle(ctx, platformjobs.Job{}); e != nil {
		t.Fatal(e)
	}
	queue := platformjobs.NewQueue(p)
	for range 3 {
		job, e := queue.Claim(ctx, qualityjobs.EnforcementImportKind, "bounded-test", time.Minute)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = queue.Fail(ctx, job.ID, job.LeaseToken, 0, "synthetic outage"); e != nil {
			t.Fatal(e)
		}
	}
	for range 3 {
		if e = d.Handle(ctx, platformjobs.Job{}); e != nil {
			t.Fatal(e)
		}
	}
	var attempts int
	var last string
	if e = p.QueryRow(ctx, "SELECT attempts,last_error FROM job_queue WHERE kind=$1 AND status='dead'", qualityjobs.EnforcementImportKind).Scan(&attempts, &last); e != nil || attempts != 3 || last != "synthetic outage" {
		t.Fatal("discovery renewed dead budget", attempts, e)
	}
	entries, e := os.ReadDir(root)
	if e != nil || len(entries) != 0 {
		t.Fatal("native workspace retained", e)
	}
	// A denied serial lease prevents both fetch and processing.
	refused = im
	refused.Serial = func(context.Context, func(context.Context) error) error { return errors.New("lease denied") }
	if e = refused.Handle(ctx, platformjobs.Job{Payload: jobraw}); e == nil {
		t.Fatal("source lease bypassed")
	}
}
