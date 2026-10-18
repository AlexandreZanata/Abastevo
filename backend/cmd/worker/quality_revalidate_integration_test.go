//go:build integration

package main

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	directory "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/adapters"
	qualityjobs "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters/jobs"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters/prepared"
	qualityread "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters/read"
	platformjobs "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

const revalidateCNPJA = "11222333000181"
const revalidateCNPJB = "11222333000262"

func nativeWorkerMultiRowArchive(t *testing.T, rows ...string) []byte {
	t.Helper()
	header := "DataColeta;IdNumeric;GrupoProduto;Produto;RazaoSocialPosto;CnpjPosto;Distribuidora;Endereço;Complemento;Bairro;Município;Latitude;Longitude;Uf;RegiaoPolitica;Ensaio;Resultado;UnidadeEnsaio;Conforme\n"
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	f, e := w.Create("PMQC_2016_01.csv")
	if e != nil {
		t.Fatal(e)
	}
	body := header
	for _, row := range rows {
		body += row + "\n"
	}
	if _, e = f.Write([]byte(body)); e != nil {
		t.Fatal(e)
	}
	if e = w.Close(); e != nil {
		t.Fatal(e)
	}
	return b.Bytes()
}

func qualityImportJobs(t *testing.T, p *pgxpool.Pool) [][]byte {
	t.Helper()
	rows, e := p.Query(context.Background(), "SELECT payload::text FROM job_queue WHERE kind='quality-import' ORDER BY created_at,id")
	if e != nil {
		t.Fatal(e)
	}
	defer rows.Close()
	var out [][]byte
	for rows.Next() {
		var payload []byte
		if e = rows.Scan(&payload); e != nil {
			t.Fatal(e)
		}
		out = append(out, payload)
	}
	if e = rows.Err(); e != nil {
		t.Fatal(e)
	}
	return out
}

func testUUID(raw string) pgtype.UUID {
	var id pgtype.UUID
	decoded, err := hex.DecodeString(strings.ReplaceAll(raw, "-", ""))
	if err != nil || len(decoded) != 16 {
		panic("test uuid")
	}
	copy(id.Bytes[:], decoded)
	id.Valid = true
	return id
}

func revalidationCursor(t *testing.T, p *pgxpool.Pool) string {
	t.Helper()
	var edition pgtype.UUID
	if e := p.QueryRow(context.Background(), "SELECT cursor_edition FROM quality_revalidation_cursors WHERE kind='quality-revalidate'").Scan(&edition); e != nil {
		t.Fatal(e)
	}
	if !edition.Valid {
		return ""
	}
	hexed := hex.EncodeToString(edition.Bytes[:])
	return hexed[0:8] + "-" + hexed[8:12] + "-" + hexed[12:16] + "-" + hexed[16:20] + "-" + hexed[20:32]
}

// TestNativeWorkerDirectoryRevalidation covers the durable post-publication
// invalidation path with the real Rust preparer, real Directory reads and
// real PostGIS: a bounded sweep detects a retired binding, enqueues one
// refresh, a converged dead job is replayed once with an audited reason, the
// fresh import publishes a correction while the historical revision stays
// reachable, repeated sweeps stay idempotent, and a later re-binding triggers
// a new refresh on the corrected revision.
func TestNativeWorkerDirectoryRevalidation(t *testing.T) {
	binary := os.Getenv("ANPFUEL_TEST_QUALITY_PREPARER_PATH")
	if binary == "" {
		t.Fatal("ANPFUEL_TEST_QUALITY_PREPARER_PATH must name the pinned Rust quality CLI")
	}
	p := freshNativeWorkerDB(t)
	ctx := context.Background()
	repo := directory.NewRepository(p)
	stationA, e := repo.ResolveCNPJ(ctx, revalidateCNPJA, "Synthetic A", nil)
	if e != nil {
		t.Fatal(e)
	}
	stationB, e := repo.ResolveCNPJ(ctx, revalidateCNPJB, "Synthetic B", nil)
	if e != nil {
		t.Fatal(e)
	}
	root := t.TempDir()
	raw := nativeWorkerMultiRowArchive(t,
		"2016-01-04 12:34:56;501;synthetic;ETANOL HIDRATADO COMUM;Synthetic A;11.222.333/0001-81;Synthetic;Street A;;Synthetic;Synthetic;;;MT;CENTRO-OESTE;Aspecto;LII;;Sim",
		"2016-01-05 08:00:00;502;synthetic;ETANOL HIDRATADO COMUM;Synthetic B;11.222.333/0002-62;Synthetic;Street B;;Synthetic;Synthetic;;;MT;CENTRO-OESTE;Aspecto;LII;;Sim",
	)
	sum := sha256.Sum256(raw)
	identity := prepared.InputIdentity{SHA256: hex.EncodeToString(sum[:]), Bytes: int64(len(raw))}
	payload := qualityjobs.ImportPayload{Version: 1, Source: "pmqc", Stream: "pmqc-assay-v1", Edition: "2016-01", URL: "https://www.gov.br" + qualityArchivePrefix + "pmqc_2016_01.zip", SHA256: identity.SHA256, Member: "PMQC_2016_01.csv"}
	encoded, e := json.Marshal(payload)
	if e != nil {
		t.Fatal(e)
	}
	im := nativeQualityImport(p, binary, root)
	im.Fetch = func(_ context.Context, url string) (qualityjobs.FileArtifact, error) {
		if url != payload.URL {
			t.Fatal("unexpected source URL")
		}
		filename := filepath.Join(root, "synthetic.zip")
		if e := os.WriteFile(filename, raw, 0600); e != nil {
			return qualityjobs.FileArtifact{}, e
		}
		return qualityjobs.FileArtifact{Path: filename, Identity: identity, Close: func() error { return os.Remove(filename) }}, nil
	}
	if e = im.Handle(ctx, platformjobs.Job{Payload: encoded}); e != nil {
		t.Fatal(e)
	}
	r := qualityread.NewReader(p)
	rows, _, firstLease, e := r.Assays(ctx, time.Now(), payload.Source, payload.Stream, stationB.ID, "", "", "", 100)
	if e != nil || len(rows) != 1 {
		t.Fatal("initial binding not published", rows, e)
	}

	reval := nativeQualityRevalidate(p)
	if e = repo.RetireIdentifier(ctx, "CNPJ", revalidateCNPJB); e != nil {
		t.Fatal(e)
	}
	if e = reval.Handle(ctx, platformjobs.Job{}); e != nil {
		t.Fatal(e)
	}
	jobs := qualityImportJobs(t, p)
	if len(jobs) != 1 {
		t.Fatal("retired binding did not enqueue exactly one refresh", jobs)
	}
	var refresh qualityjobs.ImportPayload
	if e = json.Unmarshal(jobs[0], &refresh); e != nil {
		t.Fatal(e)
	}
	if refresh.Edition != payload.Edition || refresh.SHA256 != payload.SHA256 || refresh.Member != payload.Member || refresh.URL != payload.URL {
		t.Fatal("refresh payload did not reuse frozen provenance", refresh)
	}

	// Repeating the sweep converges on the same queued job; after the job
	// exhausts its attempts, the sweep replays the dead job once with an
	// audited reason instead of forking duplicate work.
	if e = reval.Handle(ctx, platformjobs.Job{}); e != nil {
		t.Fatal(e)
	}
	if jobs = qualityImportJobs(t, p); len(jobs) != 1 {
		t.Fatal("repeated sweep forked refresh work", jobs)
	}
	queue := platformjobs.NewQueue(p)
	for i := 0; i < 3; i++ {
		claimed, cerr := queue.Claim(ctx, "quality-import", "revalidation-test", time.Minute)
		if cerr != nil {
			t.Fatal(cerr)
		}
		if _, ferr := queue.Fail(ctx, claimed.ID, claimed.LeaseToken, 0, "synthetic outage"); ferr != nil {
			t.Fatal(ferr)
		}
	}
	if e = reval.Handle(ctx, platformjobs.Job{}); e != nil {
		t.Fatal(e)
	}
	if jobs = qualityImportJobs(t, p); len(jobs) != 1 {
		t.Fatal("dead refresh forked", jobs)
	}
	var status, lastError string
	var attempts int
	if e = p.QueryRow(ctx, "SELECT status,attempts,last_error FROM job_queue WHERE kind='quality-import'").Scan(&status, &attempts, &lastError); e != nil {
		t.Fatal(e)
	}
	if status != "queued" || attempts != 0 || lastError == "" {
		t.Fatal("dead refresh not replayed with audit reason", status, attempts, lastError)
	}

	// Execute the queued refresh through the same native path: the retired
	// binding goes to review, the connected station keeps its fact, and the
	// superseded revision stays reachable with its frozen association.
	if e = im.Handle(ctx, platformjobs.Job{Payload: jobs[0]}); e != nil {
		t.Fatal(e)
	}
	rows, _, correctedLease, e := r.Assays(ctx, time.Now(), payload.Source, payload.Stream, stationB.ID, "", "", "", 100)
	if e != nil || len(rows) != 0 || correctedLease.RevisionID == firstLease.RevisionID {
		t.Fatal("retired identity remained bound", rows, correctedLease, e)
	}
	rows, _, _, e = r.Assays(ctx, time.Now(), payload.Source, payload.Stream, stationA.ID, "", "", "", 100)
	if e != nil || len(rows) != 1 {
		t.Fatal("unrelated binding lost", rows, e)
	}
	historical, _, _, e := r.AtRevision(firstLease.RevisionID).Assays(ctx, time.Now(), payload.Source, payload.Stream, stationB.ID, "", "", "", 100)
	if e != nil || len(historical) != 1 {
		t.Fatal("frozen historical association lost", historical, e)
	}
	if e = reval.Handle(ctx, platformjobs.Job{}); e != nil {
		t.Fatal(e)
	}
	if jobs = qualityImportJobs(t, p); len(jobs) != 1 {
		t.Fatal("consistent correction re-enqueued", jobs)
	}
	var stations, revisions int
	if e = p.QueryRow(ctx, "SELECT (SELECT count(*) FROM directory_stations),(SELECT count(*) FROM quality_revisions)").Scan(&stations, &revisions); e != nil || stations != 2 || revisions != 2 {
		t.Fatal("revalidation created identity or lost revisions", stations, revisions, e)
	}
}

// TestNativeWorkerRevalidationCursorProgress proves the bounded scan advances
// durably across sweeps instead of re-checking the same first edition. Two
// synthetic published editions with explicit ordered identifiers make the
// batch boundary deterministic.
func TestNativeWorkerRevalidationCursorProgress(t *testing.T) {
	p := freshNativeWorkerDB(t)
	ctx := context.Background()
	repo := directory.NewRepository(p)
	stationA, e := repo.ResolveCNPJ(ctx, revalidateCNPJA, "Synthetic A", nil)
	if e != nil {
		t.Fatal(e)
	}
	stationB, e := repo.ResolveCNPJ(ctx, revalidateCNPJB, "Synthetic B", nil)
	if e != nil {
		t.Fatal(e)
	}
	editionA := testUUID("00000000-0000-0000-0000-000000000001")
	editionB := testUUID("00000000-0000-0000-0000-000000000002")
	revisionA := testUUID("10000000-0000-0000-0000-000000000001")
	revisionB := testUUID("10000000-0000-0000-0000-000000000002")
	pageHash := strings.Repeat("a", 64)
	resource := func(edition, revision pgtype.UUID, label string) {
		t.Helper()
		if _, e := p.Exec(ctx, "INSERT INTO quality_editions(id,source,stream,edition) VALUES($1,'pmqc','pmqc-assay-v1',$2)", edition, label); e != nil {
			t.Fatal(e)
		}
		if _, e := p.Exec(ctx, "INSERT INTO quality_revisions(id,edition_id,status) VALUES($1,$2,'published')", revision, edition); e != nil {
			t.Fatal(e)
		}
		if _, e := p.Exec(ctx, `INSERT INTO quality_revision_resources(revision_id,url,sha256,bytes,member_name,member_sha256,member_bytes)
		 VALUES($1,'https://www.gov.br/anp/pt-br/centrais-de-conteudo/dados-abertos/arquivos/pmqc/pmqc_2016_01.zip',$2,10,'PMQC_2016_01.csv',$2,10)`, revision, pageHash); e != nil {
			t.Fatal(e)
		}
	}
	resource(editionA, revisionA, "2016-01")
	resource(editionB, revisionB, "2016-02")
	record := func(revision pgtype.UUID, station string, cnpj, recordKey string) {
		t.Helper()
		assayID, _ := platformjobs.NewUUIDv4()
		linkID, _ := platformjobs.NewUUIDv4()
		if _, e := p.Exec(ctx, `INSERT INTO quality_assays(id,revision_id,sample_ref,assay_ref,source_cnpj,observed_on,fuel,result_text,unit,conformity,evidence_ref,fingerprint)
		 VALUES($1,$2,$3,'a1',$4,'2016-01-04','ETHANOL','LII','','CONFORMING','e1',$5)`, testUUID(assayID), revision, strings.TrimPrefix(recordKey, "pmqc-assay-v1:"), cnpj, strings.Repeat("b", 64)); e != nil {
			t.Fatal(e)
		}
		if _, e := p.Exec(ctx, "INSERT INTO quality_links(id,revision_id,record_key,station_id,automatic,reason,active) VALUES($1,$2,$3,$4,true,'exact current cnpj',true)", testUUID(linkID), revision, recordKey, testUUID(station)); e != nil {
			t.Fatal(e)
		}
	}
	record(revisionA, stationA.ID, revalidateCNPJA, "pmqc-assay-v1:rec1")
	record(revisionB, stationB.ID, revalidateCNPJB, "pmqc-assay-v1:rec2")
	if e = repo.RetireIdentifier(ctx, "CNPJ", revalidateCNPJB); e != nil {
		t.Fatal(e)
	}

	reval := nativeQualityRevalidate(p)
	reval.Batch = 1
	if e = reval.Handle(ctx, platformjobs.Job{}); e != nil {
		t.Fatal(e)
	}
	if jobs := qualityImportJobs(t, p); len(jobs) != 0 {
		t.Fatal("sweep skipped its cursor window", jobs)
	}
	if got := revalidationCursor(t, p); got != "00000000-0000-0000-0000-000000000001" {
		t.Fatal("cursor did not advance durably", got)
	}
	if e = reval.Handle(ctx, platformjobs.Job{}); e != nil {
		t.Fatal(e)
	}
	jobs := qualityImportJobs(t, p)
	if len(jobs) != 1 {
		t.Fatal("second sweep did not reach the divergent edition", jobs)
	}
	var refresh qualityjobs.ImportPayload
	if e = json.Unmarshal(jobs[0], &refresh); e != nil || refresh.Edition != "2016-02" {
		t.Fatal("wrong edition refreshed", refresh, e)
	}
	if got := revalidationCursor(t, p); got != "" {
		t.Fatal("completed sweep did not wrap", got)
	}
}
