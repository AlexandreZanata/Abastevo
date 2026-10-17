//go:build integration

package main

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	dbmigrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	directory "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/adapters"
	qualityjobs "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters/jobs"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters/prepared"
	qualityread "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters/read"
	platformjobs "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func freshNativeWorkerDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("ANPFUEL_TEST_DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://anpfuel:anpfuel@127.0.0.1:5434/anpfuel?sslmode=disable"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	admin, e := pgx.Connect(ctx, dsn)
	if e != nil {
		t.Fatal(e)
	}
	defer admin.Close(ctx)
	name := fmt.Sprintf("quality_native_worker_%d", time.Now().UnixNano())
	if _, e = admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		conn, e := pgx.Connect(ctx, dsn)
		if e != nil {
			t.Error(e)
			return
		}
		defer conn.Close(ctx)
		if _, e = conn.Exec(ctx, "DROP DATABASE "+pgx.Identifier{name}.Sanitize()); e != nil {
			t.Error(e)
		}
	})
	u, e := url.Parse(dsn)
	if e != nil {
		t.Fatal(e)
	}
	u.Path = "/" + name
	if _, e = migrate.Apply(ctx, u.String(), dbmigrations.Files); e != nil {
		t.Fatal(e)
	}
	p, e := pgxpool.New(ctx, u.String())
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(p.Close)
	return p
}
func nativeWorkerArchive(t *testing.T) []byte {
	t.Helper()
	header := "DataColeta;IdNumeric;GrupoProduto;Produto;RazaoSocialPosto;CnpjPosto;Distribuidora;Endereço;Complemento;Bairro;Município;Latitude;Longitude;Uf;RegiaoPolitica;Ensaio;Resultado;UnidadeEnsaio;Conforme\n"
	row := "2016-01-04 12:34:56;501;synthetic;ETANOL HIDRATADO COMUM;Synthetic;11.222.333/0001-81;Synthetic;Synthetic street;;Synthetic;Synthetic;;;MT;CENTRO-OESTE;Aspecto;LII;;Sim\n"
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	f, e := w.Create("PMQC_2016_01.csv")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.Write([]byte(header + row)); e != nil {
		t.Fatal(e)
	}
	if e = w.Close(); e != nil {
		t.Fatal(e)
	}
	return b.Bytes()
}
func TestNativeWorkerRustDirectoryPublicationAndRetirement(t *testing.T) {
	binary := os.Getenv("ANPFUEL_TEST_QUALITY_PREPARER_PATH")
	if binary == "" {
		t.Fatal("ANPFUEL_TEST_QUALITY_PREPARER_PATH must name the pinned Rust quality CLI")
	}
	p := freshNativeWorkerDB(t)
	ctx := context.Background()
	repo := directory.NewRepository(p)
	station, e := repo.ResolveCNPJ(ctx, "11222333000181", "Synthetic current station", nil)
	if e != nil {
		t.Fatal(e)
	}
	root := t.TempDir()
	raw := nativeWorkerArchive(t)
	sum := sha256.Sum256(raw)
	identity := prepared.InputIdentity{SHA256: hex.EncodeToString(sum[:]), Bytes: int64(len(raw))}
	payload := qualityjobs.ImportPayload{Version: 1, Source: "pmqc", Stream: "pmqc-assay-v1", Edition: "2016-01", URL: "https://www.gov.br" + qualityArchivePrefix + "pmqc_2016_01.zip", SHA256: identity.SHA256, Member: "PMQC_2016_01.csv"}
	encoded, e := json.Marshal(payload)
	if e != nil {
		t.Fatal(e)
	}
	job := platformjobs.Job{Payload: encoded}
	im := nativeQualityImport(p, binary, root)
	// Substitute only offline network delivery. Real Rust, Directory reads/live
	// guard, PostgreSQL source lease/publication and frozen reader remain composed.
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
	for i := 0; i < 2; i++ {
		if e = im.Handle(ctx, job); e != nil {
			t.Fatal(e)
		}
	}
	r := qualityread.NewReader(p)
	rows, _, lease, e := r.Assays(ctx, time.Now(), payload.Source, payload.Stream, station.ID, "", "", "", 100)
	// Lease freshness uses publication time; historical sampling clocks remain
	// independent of both that clock and Directory creation time.
	if e != nil || len(rows) != 1 || rows[0].ObservedOn != "2016-01-04" || rows[0].SourceObservedAt != "2016-01-04 12:34:56" || lease.Stale {
		t.Fatal("dated official identity lost to operational timestamp", rows, lease, e)
	}
	old := lease.RevisionID
	prepare := im.Prepare
	retire := true
	im.Prepare = func(ctx context.Context, payload qualityjobs.ImportPayload, file qualityjobs.FileArtifact) (qualityjobs.PreparedBatch, error) {
		batch, e := prepare(ctx, payload, file)
		if e == nil && retire {
			retire = false
			e = repo.RetireIdentifier(ctx, "CNPJ", "11222333000181")
		}
		return batch, e
	}
	if e = im.Handle(ctx, job); e == nil {
		t.Fatal("Directory change crossed prepare/publication guard")
	}
	rows, _, lease, e = r.Assays(ctx, time.Now(), payload.Source, payload.Stream, station.ID, "", "", "", 100)
	if e != nil || len(rows) != 1 || lease.RevisionID != old {
		t.Fatal("failed guard touched published source", rows, lease, e)
	}
	if e = im.Handle(ctx, job); e != nil {
		t.Fatal("fresh retry failed to keep retired identity in review", e)
	}
	rows, _, lease, e = r.Assays(ctx, time.Now(), payload.Source, payload.Stream, station.ID, "", "", "", 100)
	if e != nil || len(rows) != 0 || lease.RevisionID == old {
		t.Fatal("retired identity auto-bound", rows, lease, e)
	}
	historical, _, _, e := r.AtRevision(old).Assays(ctx, time.Now(), payload.Source, payload.Stream, station.ID, "", "", "", 100)
	if e != nil || len(historical) != 1 {
		t.Fatal("historical source facts lost", historical, e)
	}
	var stations, revisions int
	if e = p.QueryRow(ctx, "SELECT (SELECT count(*) FROM directory_stations),(SELECT count(*) FROM quality_revisions)").Scan(&stations, &revisions); e != nil || stations != 1 || revisions != 2 {
		t.Fatal("source created identity or duplicate edition", stations, revisions, e)
	}
	files, e := os.ReadDir(root)
	if e != nil || len(files) != 0 {
		t.Fatal("native workspaces retained", files, e)
	}
}
func TestNativeWorkerDisabledQueueDoesNotChargeAttempts(t *testing.T) {
	p := freshNativeWorkerDB(t)
	ctx := context.Background()
	q := platformjobs.NewQueue(p)
	handlers := map[string]platformjobs.Handler{}
	for _, kind := range []string{"evidence-sweep", "account-erasure-reconcile", "privacy-training-reconcile", "privacy-training-sweep", "ordinary", "quality-import", "quality-discovery"} {
		handlers[kind] = dispatcherHandler(kind)
	}
	ids := map[string]string{}
	for _, kind := range []string{"quality-import", "quality-discovery", "evidence-sweep", "ordinary"} {
		id, e := platformjobs.Enqueue(ctx, p, kind, []byte(`{"version":1}`), kind, 3, time.Time{})
		if e != nil {
			t.Fatal(e)
		}
		ids[kind] = id
	}
	dispatchers, e := workerDispatchers(q, handlers)
	if e != nil {
		t.Fatal(e)
	}
	for _, d := range dispatchers {
		if _, e = d.RunOnce(ctx); e != nil {
			t.Fatal(e)
		}
	}
	for kind, id := range ids {
		var status string
		var attempts int
		if e = p.QueryRow(ctx, "SELECT status,attempts FROM job_queue WHERE id=$1", id).Scan(&status, &attempts); e != nil {
			t.Fatal(e)
		}
		if kind == "quality-import" || kind == "quality-discovery" {
			if status != "queued" || attempts != 0 {
				t.Fatal("disabled native job charged", kind, status, attempts)
			}
		} else if status != "done" || attempts != 1 {
			t.Fatal("source work blocked ordinary/safety slot", kind, status, attempts)
		}
	}
	enabled, e := workerDispatchers(q, handlers, "quality-import")
	if e != nil {
		t.Fatal(e)
	}
	if found, e := enabled[len(enabled)-1].RunOnce(ctx); e != nil || !found {
		t.Fatal("enabled source slot did not claim", found, e)
	}
	var attempts int
	if e = p.QueryRow(ctx, "SELECT attempts FROM job_queue WHERE id=$1", ids["quality-discovery"]).Scan(&attempts); e != nil || attempts != 0 {
		t.Fatal("reserved discovery charged without enable", attempts, e)
	}
}
