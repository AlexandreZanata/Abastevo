//go:build integration

package adapters

import (
	"context"
	"errors"
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
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func nativeFineParams(t *testing.T, revision string, m *idMaker) quality.StageNativeFineObservationParams {
	t.Helper()
	id, e := m.next()
	if e != nil {
		t.Fatal(e)
	}
	rowID, _ := mustUUID(id)
	revisionID, _ := mustUUID(revision)
	due, _ := pgDate("2025-02-28")
	return quality.StageNativeFineObservationParams{ID: rowID, RevisionID: revisionID, NativeKey: "observation:" + strings.Repeat("a", 64), ProceedingRef: "000000000000001", PartyCnpj: "11222333000181", AmountMinor: 15000000, Status: strings.Repeat("long source label ", 8), EvidenceRef: "synthetic.csv:8", DufRef: "SYNTHETIC/1", Department: "SYNTHETIC", DueOn: due, AmountText: "150000,00", PaidText: "180000,01", PaidMinor: 18000001, ReportYear: 2025, ExtractionMonth: "2026-01"}
}
func legacyFineInput() application.EventsInput {
	in := sampleEventsInput("b")
	in.Source = "sanctions"
	in.Stream = "sanction-v1"
	in.Enforcement = nil
	in.Counts = domain.Counts{Input: 1, Accepted: 1}
	return in
}
func TestFineStorageUpgradeKeepsLegacySnapshotsAndPublication(t *testing.T) {
	ctx := context.Background()
	prior := fstest.MapFS{}
	entries, e := fs.ReadDir(dbmigrations.Files, ".")
	if e != nil {
		t.Fatal(e)
	}
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() >= "000106" {
			continue
		}
		raw, e := fs.ReadFile(dbmigrations.Files, entry.Name())
		if e != nil {
			t.Fatal(e)
		}
		prior[entry.Name()] = &fstest.MapFile{Data: raw}
	}
	p := freshPoolWithMigrations(t, prior)
	m := &idMaker{}
	in := legacyFineInput()
	rev, e := NewPublisher(p).PublishEvents(ctx, in, m.next)
	if e != nil {
		t.Fatal(e)
	}
	applied, e := migrate.Apply(ctx, p.Config().ConnConfig.ConnString(), dbmigrations.Files)
	if e != nil || !slices.Equal(applied, []string{"000106", "000107", "000108", "000109", "000110"}) {
		t.Fatal("native storage upgrade", applied, e)
	}
	if again, e := migrate.Apply(ctx, p.Config().ConnConfig.ConnString(), dbmigrations.Files); e != nil || len(again) != 0 {
		t.Fatal("migration replay", again, e)
	}
	replay, e := NewPublisher(p).PublishEvents(ctx, in, m.next)
	if e != nil || replay != rev {
		t.Fatal("legacy replay forked", e)
	}
	q := quality.New(p)
	rid, _ := mustUUID(rev)
	rows, e := q.ListRevisionSanctions(ctx, rid)
	if e != nil || len(rows) != 1 || !rows[0].AsOf.Valid || !rows[0].DecidedOn.Valid || rows[0].AmountMinor.Int64 != 150000 {
		t.Fatal("legacy values changed", e)
	}
	var contract string
	var nullable bool
	e = p.QueryRow(ctx, "SELECT source_contract, extraction_month IS NULL AND due_on IS NULL AND paid_minor IS NULL AND amount_text IS NULL AND paid_text IS NULL AND report_year IS NULL AND duf_ref IS NULL AND department IS NULL FROM quality_sanction_events WHERE revision_id=$1", rid).Scan(&contract, &nullable)
	if e != nil || contract != "sanction-prepared-8-v1" || !nullable {
		t.Fatal("legacy precision invented", e)
	}
	// Native staging can never mutate a published legacy snapshot.
	if _, e = q.StageNativeFineObservation(ctx, nativeFineParams(t, rev, m)); !errors.Is(e, pgx.ErrNoRows) {
		t.Fatal("published revision accepted native stage", e)
	}
	current, e := q.GetCurrentRevision(ctx, quality.GetCurrentRevisionParams{Source: in.Source, Stream: in.Stream})
	if e != nil || uuidString(current) != rev {
		t.Fatal("upgrade switched pointer", e)
	}
}
func TestNativeFineStorageConstraintsRollbackAndConcurrentIdempotency(t *testing.T) {
	p := freshPool(t)
	ctx := context.Background()
	m := &idMaker{}
	in := legacyFineInput()
	legacy, e := NewPublisher(p).PublishEvents(ctx, in, m.next)
	if e != nil {
		t.Fatal(e)
	}
	q := quality.New(p)
	edition, e := q.GetEditionByKey(ctx, quality.GetEditionByKeyParams{Source: in.Source, Stream: in.Stream, Edition: in.Edition})
	if e != nil {
		t.Fatal(e)
	}
	next, e := m.next()
	if e != nil {
		t.Fatal(e)
	}
	rid, _ := mustUUID(next)
	previous, _ := mustUUID(legacy)
	if _, e = q.CreateRevision(ctx, quality.CreateRevisionParams{ID: rid, EditionID: edition.ID, SupersedesRevisionID: previous}); e != nil {
		t.Fatal(e)
	}
	good := nativeFineParams(t, next, m)
	for name, mutate := range map[string]func(*quality.StageNativeFineObservationParams){
		"invalid month":          func(a *quality.StageNativeFineObservationParams) { a.ExtractionMonth = "2026-13" },
		"invented day precision": func(a *quality.StageNativeFineObservationParams) { a.ExtractionMonth = "2026-01-01" },
		"wrong extraction year":  func(a *quality.StageNativeFineObservationParams) { a.ExtractionMonth = "2024-12" },
		"wrong content identity": func(a *quality.StageNativeFineObservationParams) { a.NativeKey = "MULTA-GUESSED" },
		"money disagreement":     func(a *quality.StageNativeFineObservationParams) { a.AmountMinor++ },
		"paid disagreement":      func(a *quality.StageNativeFineObservationParams) { a.PaidMinor++ },
		"negative":               func(a *quality.StageNativeFineObservationParams) { a.AmountMinor = -1 },
		"malformed money":        func(a *quality.StageNativeFineObservationParams) { a.AmountText = "1e3" },
		"missing DUF":            func(a *quality.StageNativeFineObservationParams) { a.DufRef = "" },
		"infinite due date":      func(a *quality.StageNativeFineObservationParams) { a.DueOn.InfinityModifier = pgtype.Infinity },
		"out of contract calendar": func(a *quality.StageNativeFineObservationParams) {
			a.DueOn.Time = time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)
		},
		"missing due date":   func(a *quality.StageNativeFineObservationParams) { a.DueOn.Valid = false },
		"missing department": func(a *quality.StageNativeFineObservationParams) { a.Department = "" },
		"CPF shape":          func(a *quality.StageNativeFineObservationParams) { a.PartyCnpj = "12345678900" },
	} {
		t.Run(name, func(t *testing.T) {
			tx, e := p.Begin(ctx)
			if e != nil {
				t.Fatal(e)
			}
			defer tx.Rollback(ctx)
			a := good
			mutate(&a)
			if _, e = quality.New(tx).StageNativeFineObservation(ctx, a); e == nil {
				t.Fatal("invalid native observation staged")
			}
		})
	}
	var successes atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, e := q.StageNativeFineObservation(ctx, good)
			if e == nil {
				successes.Add(1)
			} else if !errors.Is(e, pgx.ErrNoRows) {
				t.Error("concurrent stage", e)
			}
		}()
	}
	wg.Wait()
	if successes.Load() != 1 {
		t.Fatal("native unique stage", successes.Load())
	}
	var asOfNull, decisionNull bool
	var month, due, rawAmount, rawPaid, status string
	var amount, paid int64
	e = p.QueryRow(ctx, "SELECT as_of IS NULL,decided_on IS NULL,extraction_month,due_on::text,amount_text,paid_text,amount_minor,paid_minor,status FROM quality_sanction_events WHERE revision_id=$1", rid).Scan(&asOfNull, &decisionNull, &month, &due, &rawAmount, &rawPaid, &amount, &paid, &status)
	if e != nil || !asOfNull || !decisionNull || month != "2026-01" || due != "2025-02-28" || rawAmount != "150000,00" || rawPaid != "180000,01" || amount != 15000000 || paid != 18000001 || status != good.Status {
		t.Fatal("native source facts lost or relabelled", e)
	}
	current, e := q.GetCurrentRevision(ctx, quality.GetCurrentRevisionParams{Source: in.Source, Stream: in.Stream})
	if e != nil || uuidString(current) != legacy {
		t.Fatal("staging switched pointer", e)
	}
	// A partial native revision remains staging and never modifies the frozen predecessor.
	var state string
	if e = p.QueryRow(ctx, "SELECT status FROM quality_revisions WHERE id=$1", rid).Scan(&state); e != nil || state != "staging" {
		t.Fatal("native revision visible prematurely", e)
	}
}

func TestNativeFineStageRefusesForeignStreamAndRechecksBlockedRevision(t *testing.T) {
	p := freshPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	m := &idMaker{}
	q := quality.New(p)
	makeRevision := func(source, stream string) string {
		t.Helper()
		ed, _ := m.next()
		eid, _ := mustUUID(ed)
		rev, _ := m.next()
		rid, _ := mustUUID(rev)
		if _, e := q.CreateEdition(ctx, quality.CreateEditionParams{ID: eid, Source: source, Stream: stream, Edition: "synthetic"}); e != nil {
			t.Fatal(e)
		}
		if _, e := q.CreateRevision(ctx, quality.CreateRevisionParams{ID: rid, EditionID: eid}); e != nil {
			t.Fatal(e)
		}
		return rev
	}
	foreign := makeRevision("pmqc", "pmqc-assay-v1")
	if _, e := q.StageNativeFineObservation(ctx, nativeFineParams(t, foreign, m)); !errors.Is(e, pgx.ErrNoRows) {
		t.Fatal("native fine entered foreign source stream", e)
	}
	revision := makeRevision("sanctions", "sanction-v1")
	rid, _ := mustUUID(revision)
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
	result := make(chan error, 1)
	args := nativeFineParams(t, revision, m)
	go func() { _, err := quality.New(conn).StageNativeFineObservation(ctx, args); result <- err }()
	// Observe PostgreSQL's actual blocker instead of assuming a scheduling delay.
	for {
		var n int
		if e = p.QueryRow(ctx, "SELECT cardinality(pg_blocking_pids($1))", pid).Scan(&n); e != nil {
			t.Fatal("lock observation", e)
		}
		if n > 0 {
			break
		}
		select {
		case err := <-result:
			t.Fatal("staging escaped revision lock", err)
		default:
		}
	}
	if _, e = locker.Exec(ctx, "UPDATE quality_revisions SET status='withdrawn' WHERE id=$1", rid); e != nil {
		t.Fatal(e)
	}
	if e = locker.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	select {
	case e = <-result:
		if !errors.Is(e, pgx.ErrNoRows) {
			t.Fatal("stale staging state accepted", e)
		}
	case <-ctx.Done():
		t.Fatal("blocked native stage did not finish")
	}
	var rows int
	if e = p.QueryRow(ctx, "SELECT count(*) FROM quality_sanction_events WHERE revision_id=$1", rid).Scan(&rows); e != nil || rows != 0 {
		t.Fatal("withdrawn revision gained rows", e)
	}
}
