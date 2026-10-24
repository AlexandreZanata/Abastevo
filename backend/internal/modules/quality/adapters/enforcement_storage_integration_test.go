//go:build integration

package adapters

import (
	"context"
	"encoding/json"
	"io/fs"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"

	dbmigrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	quality "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/quality"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
	"github.com/jackc/pgx/v5/pgtype"
)

func enforcementInput() application.EnforcementSourceInput {
	r := application.EnforcementSourceRecord{SourceCNPJ: "11222333000181", SubjectKind: "company-shaped", UF: "DF", Municipality: "SYNTHETIC", Segment: "synthetic", DocumentDate: "2026-01-02", DocumentNumber: "DOC-1", Procedure: "Inspection", Result: strings.Repeat("Distinct result ", 8), EvidenceRef: "synthetic.xlsx#row:2"}
	r.Fingerprint = r.ContentHash()
	r.RecordKey = "enforcement-source:" + r.Fingerprint
	return application.EnforcementSourceInput{Context: application.EnforcementSourceContext{Profile: application.EnforcementSourceProfile, SourceFile: "synthetic.xlsx", DateSystem: "EXCEL_1900", FirstDocumentDay: r.DocumentDate, LastDocumentDay: r.DocumentDate}, Counts: domain.Counts{Input: 1, Accepted: 1}, ManifestHash: strings.Repeat("b", 64), Resources: []application.Resource{{URL: "https://www.gov.br/synthetic.xlsx", SHA256: strings.Repeat("a", 64), Bytes: 10}}, Records: []application.EnforcementSourceRecord{r}}
}
func enforcementContextArgs(rid pgtype.UUID) quality.RecordEnforcementSourceContextParams {
	in := enforcementInput()
	day, _ := pgDate(in.Context.FirstDocumentDay)
	return quality.RecordEnforcementSourceContextParams{RevisionID: rid, SourceProfile: in.Context.Profile, SourceFile: in.Context.SourceFile, DateSystem: in.Context.DateSystem, InputCount: 1, AcceptedCount: 1, FirstDocumentDay: day, LastDocumentDay: day}
}
func enforcementJSON(t *testing.T, m *idMaker, mutate func(map[string]any)) []byte {
	t.Helper()
	id, _ := m.next()
	r := enforcementInput().Records[0]
	row := map[string]any{"id": id, "native_key": r.RecordKey, "subject_cnpj": r.SourceCNPJ, "subject_kind": r.SubjectKind, "uf": r.UF, "municipality": r.Municipality, "segment": r.Segment, "document_day": r.DocumentDate, "document_number": r.DocumentNumber, "procedure": r.Procedure, "result": r.Result, "evidence_ref": r.EvidenceRef, "fingerprint": r.Fingerprint}
	if mutate != nil {
		mutate(row)
	}
	raw, e := json.Marshal([]any{row})
	if e != nil {
		t.Fatal(e)
	}
	return raw
}
func newEnforcementRevision(t *testing.T, q *quality.Queries, m *idMaker, source, stream string) pgtype.UUID {
	t.Helper()
	ctx := context.Background()
	ed, _ := m.next()
	eid, _ := mustUUID(ed)
	rev, _ := m.next()
	rid, _ := mustUUID(rev)
	if _, e := q.CreateEdition(ctx, quality.CreateEditionParams{ID: eid, Source: source, Stream: stream, Edition: ed[24:]}); e != nil {
		t.Fatal(e)
	}
	if _, e := q.CreateRevision(ctx, quality.CreateRevisionParams{ID: rid, EditionID: eid}); e != nil {
		t.Fatal(e)
	}
	return rid
}
func TestEnforcementStorageUpgradeRetainsLegacyHashAndReplay(t *testing.T) {
	ctx := context.Background()
	prior := fstest.MapFS{}
	entries, e := fs.ReadDir(dbmigrations.Files, ".")
	if e != nil {
		t.Fatal(e)
	}
	for _, v := range entries {
		if v.IsDir() || v.Name() >= "000111" {
			continue
		}
		b, e := fs.ReadFile(dbmigrations.Files, v.Name())
		if e != nil {
			t.Fatal(e)
		}
		prior[v.Name()] = &fstest.MapFile{Data: b}
	}
	p := freshPoolWithMigrations(t, prior)
	m := &idMaker{}
	in := sampleEventsInput("b")
	in.Source = "enforcement"
	in.Stream = "enforcement-v1"
	in.Sanctions = nil
	in.Counts = domain.Counts{Input: 1, Accepted: 1}
	rev, e := NewPublisher(p).PublishEvents(ctx, in, m.next)
	if e != nil {
		t.Fatal(e)
	}
	hash := func() string {
		var h string
		if e := p.QueryRow(ctx, "SELECT md5(string_agg(row_to_json(x)::text,'' ORDER BY id)) FROM quality_enforcement_events x WHERE revision_id=$1", rev).Scan(&h); e != nil {
			t.Fatal(e)
		}
		return h
	}
	before := hash()
	applied, e := migrate.Apply(ctx, p.Config().ConnConfig.ConnString(), dbmigrations.Files)
	if e != nil || !slices.Equal(applied, expectedMigrationsFrom(t, "000111")) {
		t.Fatal(applied, e)
	}
	if hash() != before {
		t.Fatal("legacy rows changed")
	}
	if a, e := migrate.Apply(ctx, p.Config().ConnConfig.ConnString(), dbmigrations.Files); e != nil || len(a) != 0 {
		t.Fatal(a, e)
	}
	if replay, e := NewPublisher(p).PublishEvents(ctx, in, m.next); e != nil || replay != rev || hash() != before {
		t.Fatal("legacy replay changed", e)
	}
	rid, _ := mustUUID(rev)
	if n, e := quality.New(p).RecordEnforcementSourceContext(ctx, enforcementContextArgs(rid)); e != nil || n != 0 {
		t.Fatal("published context mutation", n, e)
	}
}
func TestEnforcementStorageConstraintsRollbackAndConcurrency(t *testing.T) {
	p := freshPool(t)
	q := quality.New(p)
	m := &idMaker{}
	ctx := context.Background()
	rid := newEnforcementRevision(t, q, m, "enforcement", "enforcement-v1")
	args := enforcementContextArgs(rid)
	for name, change := range map[string]func(*quality.RecordEnforcementSourceContextParams){"census": func(a *quality.RecordEnforcementSourceContextParams) { a.InputCount++ }, "profile": func(a *quality.RecordEnforcementSourceContextParams) { a.SourceProfile = "unknown" }, "date system": func(a *quality.RecordEnforcementSourceContextParams) { a.DateSystem = "GUESS" }, "path": func(a *quality.RecordEnforcementSourceContextParams) { a.SourceFile = "../a.xlsx" }, "infinity": func(a *quality.RecordEnforcementSourceContextParams) {
		a.FirstDocumentDay.InfinityModifier = pgtype.Infinity
	}} {
		t.Run(name, func(t *testing.T) {
			a := args
			change(&a)
			if _, e := q.RecordEnforcementSourceContext(ctx, a); e == nil {
				t.Fatal("invalid context admitted")
			}
		})
	}
	if n, e := q.RecordEnforcementSourceContext(ctx, args); e != nil || n != 1 {
		t.Fatal(n, e)
	}
	for name, change := range map[string]func(map[string]any){"date": func(a map[string]any) { a["document_day"] = "2026-02-30" }, "range": func(a map[string]any) { a["document_day"] = "2025-01-01" }, "cpf": func(a map[string]any) { a["subject_cnpj"] = "12345678901" }, "missing result": func(a map[string]any) { delete(a, "result") }, "tamper": func(a map[string]any) { a["result"] = "forged" }, "oversize": func(a map[string]any) { a["result"] = strings.Repeat("x", 65537) }} {
		t.Run(name, func(t *testing.T) {
			raw := enforcementJSON(t, m, change)
			if _, e := q.StageEnforcementSourceRecords(ctx, quality.StageEnforcementSourceRecordsParams{RevisionID: rid, Records: raw}); e == nil {
				t.Fatal("invalid row admitted")
			}
		})
	}
	raw := enforcementJSON(t, m, nil)
	var wg sync.WaitGroup
	var success atomic.Int32
	for range 6 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if n, e := q.StageEnforcementSourceRecords(ctx, quality.StageEnforcementSourceRecordsParams{RevisionID: rid, Records: raw}); e == nil && n == 1 {
				success.Add(1)
			}
		}()
	}
	wg.Wait()
	if success.Load() != 1 {
		t.Fatal("writers did not converge", success.Load())
	}
	if n, e := q.CountEnforcementSourceRecords(ctx, rid); e != nil || n != 1 {
		t.Fatal(n, e)
	}
	// A successful early row and later invalid row share one transaction.
	tx, e := p.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	tq := quality.New(tx)
	next := newEnforcementRevision(t, tq, m, "enforcement", "enforcement-v1")
	if _, e = tq.RecordEnforcementSourceContext(ctx, enforcementContextArgs(next)); e != nil {
		t.Fatal(e)
	}
	if _, e = tq.StageEnforcementSourceRecords(ctx, quality.StageEnforcementSourceRecordsParams{RevisionID: next, Records: enforcementJSON(t, m, nil)}); e != nil {
		t.Fatal(e)
	}
	if _, e = tq.StageEnforcementSourceRecords(ctx, quality.StageEnforcementSourceRecordsParams{RevisionID: next, Records: enforcementJSON(t, m, func(r map[string]any) { r["uf"] = "BAD" })}); e == nil {
		t.Fatal("later failure missing")
	}
	_ = tx.Rollback(ctx)
	var n int
	if e = p.QueryRow(ctx, "SELECT count(*) FROM quality_revisions WHERE id=$1", next).Scan(&n); e != nil || n != 0 {
		t.Fatal("partial transaction retained", n, e)
	}
	foreign := newEnforcementRevision(t, q, m, "pmqc", "pmqc-assay-v1")
	if n, e := q.RecordEnforcementSourceContext(ctx, enforcementContextArgs(foreign)); e != nil || n != 0 {
		t.Fatal("foreign context admitted", n, e)
	}
}
func TestEnforcementStorageWithdrawalBarrier(t *testing.T) {
	p := freshPool(t)
	q := quality.New(p)
	m := &idMaker{}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	rid := newEnforcementRevision(t, q, m, "enforcement", "enforcement-v1")
	if _, e := q.RecordEnforcementSourceContext(ctx, enforcementContextArgs(rid)); e != nil {
		t.Fatal(e)
	}
	locker, e := p.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer locker.Rollback(context.Background())
	if _, e = locker.Exec(ctx, "SELECT id FROM quality_revisions WHERE id=$1 FOR UPDATE", rid); e != nil {
		t.Fatal(e)
	}
	conn, e := p.Acquire(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer conn.Release()
	pid := int32(conn.Conn().PgConn().PID())
	done := make(chan error, 1)
	raw := enforcementJSON(t, m, nil)
	go func() {
		n, e := quality.New(conn).StageEnforcementSourceRecords(ctx, quality.StageEnforcementSourceRecordsParams{RevisionID: rid, Records: raw})
		if e == nil && n != 0 {
			e = context.Canceled
		}
		done <- e
	}()
	for {
		var n int
		if e = p.QueryRow(ctx, "SELECT cardinality(pg_blocking_pids($1))", pid).Scan(&n); e != nil {
			t.Fatal(e)
		}
		if n > 0 {
			break
		}
		select {
		case e := <-done:
			t.Fatal("escaped revision lock", e)
		default:
		}
	}
	if _, e = locker.Exec(ctx, "UPDATE quality_revisions SET status='withdrawn' WHERE id=$1", rid); e != nil {
		t.Fatal(e)
	}
	if e = locker.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	if e = <-done; e != nil {
		t.Fatal(e)
	}
	if n, e := q.CountEnforcementSourceRecords(ctx, rid); e != nil || n != 0 {
		t.Fatal("withdrawn revision mutated", n, e)
	}
}
