//go:build integration

package adapters

import (
	"context"
	"errors"
	"io/fs"
	"reflect"
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
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

func historyRevision(t *testing.T, p *pgxpool.Pool, source, stream, period string, m *idMaker) pgtype.UUID {
	t.Helper()
	ctx := context.Background()
	q := quality.New(p)
	ed, e := q.GetEditionByKey(ctx, quality.GetEditionByKeyParams{Source: source, Stream: stream, Edition: period})
	if errors.Is(e, pgx.ErrNoRows) {
		id, _ := m.next()
		eid, _ := mustUUID(id)
		created, createErr := q.CreateEdition(ctx, quality.CreateEditionParams{ID: eid, Source: source, Stream: stream, Edition: period})
		ed.ID = created
		e = createErr
	}
	if e != nil {
		t.Fatal(e)
	}
	id, _ := m.next()
	rid, _ := mustUUID(id)
	if _, e = q.CreateRevision(ctx, quality.CreateRevisionParams{ID: rid, EditionID: ed.ID}); e != nil {
		t.Fatal(e)
	}
	return rid
}
func historyContextParams(period string, rid pgtype.UUID) quality.RecordHistoricalFineContextParams {
	p, _ := application.HistoricalFineProfile(period)
	clock := "2024-06"
	if period == "2016-2019" {
		clock = "2020-02-06"
	}
	metadata := int32(0)
	if period == "2020" || period == "2021" {
		metadata = 1
	}
	return quality.RecordHistoricalFineContextParams{RevisionID: rid, InputContract: p.Contract, SourcePeriod: period, SourceFile: p.SourceFile, HeaderRecord: int32(p.HeaderRecord), SourceClockPrecision: p.ClockPrecision, SourceClockValue: clock, SourceRecordCount: 4, MetadataCount: metadata}
}
func historyObservationParams(t *testing.T, period string, rid pgtype.UUID, m *idMaker) quality.StageHistoricalFineObservationParams {
	t.Helper()
	profile, _ := application.HistoricalFineProfile(period)
	id, _ := m.next()
	rowID, _ := mustUUID(id)
	due, _ := pgDate("2020-02-28")
	text := func(s string) pgtype.Text { return pgtype.Text{String: s, Valid: true} }
	paid, rawPaid := pgtype.Int8{Int64: 0, Valid: true}, " R$ 0,00 "
	if period == "2023" {
		paid = pgtype.Int8{}
		rawPaid = " R$ -   "
	}
	return quality.StageHistoricalFineObservationParams{ID: rowID, RevisionID: rid, NativeKey: "observation:" + strings.Repeat("a", 64), ProceedingRef: "000000000000001", PartyCnpj: "11222333000181", AmountMinor: 123456, Status: "Unknown source status", EvidenceRef: profile.SourceFile + ":8", ReferenceLabel: text(profile.ReferenceLabel), ReferenceRef: text("SYNTHETIC/1"), UnlabelledField: text(""), Department: text(""), DueOn: due, AmountText: text(" R$ 1.234,56 "), PaidText: text(rawPaid), PaidMinor: paid}
}
func TestHistoricalFineStorageUpgradeKeepsLegacyNativeValuesAndPointers(t *testing.T) {
	prior := fstest.MapFS{}
	entries, e := fs.ReadDir(dbmigrations.Files, ".")
	if e != nil {
		t.Fatal(e)
	}
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() >= "000108" {
			continue
		}
		raw, e := fs.ReadFile(dbmigrations.Files, entry.Name())
		if e != nil {
			t.Fatal(e)
		}
		prior[entry.Name()] = &fstest.MapFile{Data: raw}
	}
	p := freshPoolWithMigrations(t, prior)
	ctx := context.Background()
	m := &idMaker{}
	publisher := NewPublisher(p)
	legacy := legacyFineInput()
	old, e := publisher.PublishEvents(ctx, legacy, m.next)
	if e != nil {
		t.Fatal(e)
	}
	native := nativeFineInput("c")
	current, e := publisher.PublishNativeFines(ctx, native, m.next)
	if e != nil {
		t.Fatal(e)
	}
	snapshot := func() []string {
		var rows, pointers, revisions, clocks string
		e := p.QueryRow(ctx, `SELECT
 md5(coalesce((SELECT jsonb_agg(to_jsonb(e)-ARRAY['history_revision_id','reference_label','reference_ref','source_transit_on','unlabelled_field'] ORDER BY id)::text FROM quality_sanction_events e),'[]')),
 md5(coalesce((SELECT jsonb_agg(to_jsonb(c) ORDER BY source,stream)::text FROM quality_current_revisions c),'[]')),
 md5(coalesce((SELECT jsonb_agg(to_jsonb(r) ORDER BY id)::text FROM quality_revisions r),'[]')),
 md5(coalesce((SELECT jsonb_agg(to_jsonb(n) ORDER BY revision_id)::text FROM quality_native_fine_revisions n),'[]'))`).Scan(&rows, &pointers, &revisions, &clocks)
		if e != nil {
			t.Fatal(e)
		}
		return []string{rows, pointers, revisions, clocks}
	}
	before := snapshot()
	applied, e := migrate.Apply(ctx, p.Config().ConnConfig.ConnString(), dbmigrations.Files)
	if e != nil || !slices.Equal(applied, []string{"000108", "000109", "000110", "000111"}) {
		t.Fatal("historical storage upgrade", applied, e)
	}
	if again, e := migrate.Apply(ctx, p.Config().ConnConfig.ConnString(), dbmigrations.Files); e != nil || len(again) != 0 {
		t.Fatal("upgrade replay", again, e)
	}
	if !reflect.DeepEqual(before, snapshot()) {
		t.Fatal("upgrade mutated existing facts/pointers")
	}
	if replay, e := publisher.PublishEvents(ctx, legacy, m.next); e != nil || replay != old {
		t.Fatal("legacy replay forked", e)
	}
	if replay, e := publisher.PublishNativeFines(ctx, native, m.next); e != nil || replay != current {
		t.Fatal("2025 replay forked", e)
	}
	var changed, contexts int
	if e = p.QueryRow(ctx, `SELECT (SELECT count(*) FROM quality_sanction_events WHERE history_revision_id IS NOT NULL OR reference_label IS NOT NULL OR reference_ref IS NOT NULL OR source_transit_on IS NOT NULL OR unlabelled_field IS NOT NULL),(SELECT count(*) FROM quality_historical_fine_revisions)`).Scan(&changed, &contexts); e != nil || changed != 0 || contexts != 0 {
		t.Fatal("upgrade invented history facts", changed, contexts, e)
	}
}
func TestHistoricalFineStorageProfilesMoneyClocksAndNullSemantics(t *testing.T) {
	p := freshPool(t)
	q := quality.New(p)
	ctx := context.Background()
	m := &idMaker{}
	for _, period := range []string{"2016-2019", "2020", "2021", "2022", "2023", "2024"} {
		t.Run(period, func(t *testing.T) {
			rid := historyRevision(t, p, "sanctions", "sanction-v1", period, m)
			args := historyContextParams(period, rid)
			if _, e := q.RecordHistoricalFineContext(ctx, args); e != nil {
				t.Fatal("context stage", e)
			}
			good := historyObservationParams(t, period, rid, m)
			// Same Unicode whitespace accepted by Rust/Go must preserve raw currency text.
			good.AmountText.String = "\u00a0R$\u00a01.234,56\u00a0"
			if _, e := q.StageHistoricalFineObservation(ctx, good); e != nil {
				t.Fatal("native facts stage", e)
			}
			source, e := q.HistoricalFineContext(ctx, rid)
			if e != nil || source.SourceClockPrecision != args.SourceClockPrecision || source.SourceClockValue != args.SourceClockValue || source.SourcePeriod != period {
				t.Fatal("source clock lost", e)
			}
			var department, label, amountText string
			var paidNil, oldNull bool
			if e = p.QueryRow(ctx, `SELECT department,reference_label,amount_text,paid_minor IS NULL,as_of IS NULL AND decided_on IS NULL AND report_year IS NULL AND extraction_month IS NULL AND duf_ref IS NULL FROM quality_sanction_events WHERE revision_id=$1`, rid).Scan(&department, &label, &amountText, &paidNil, &oldNull); e != nil || department != "" || label != good.ReferenceLabel.String || amountText != good.AmountText.String || paidNil != (period == "2023") || !oldNull {
				t.Fatal("source fields relabelled", e)
			}
			if period == "2020" || period == "2021" {
				note := quality.StageHistoricalFineNoteParams{RevisionID: rid, EvidenceRef: args.SourceFile + ":11", Kind: "BLANK_SEPARATOR"}
				if _, e := q.StageHistoricalFineNote(ctx, note); e != nil {
					t.Fatal(e)
				}
				notes, e := q.HistoricalFineNotes(ctx, rid)
				if e != nil || len(notes) != 1 {
					t.Fatal("note source context lost", e)
				}
			}
		})
	}
	for _, raw := range []string{"v1,00", "1.2345,00", "01,00", "R$ -", "-1,00", "1e3"} {
		var missing bool
		if e := p.QueryRow(ctx, "SELECT quality_history_money_minor($1) IS NULL", raw).Scan(&missing); e != nil || !missing {
			t.Fatal("malformed exact money accepted", e)
		}
	}
	var pointers int
	if e := p.QueryRow(ctx, "SELECT count(*) FROM quality_current_revisions").Scan(&pointers); e != nil || pointers != 0 {
		t.Fatal("staging became public", e)
	}
}
func TestHistoricalFineStorageRejectsInvalidContextsAndObservations(t *testing.T) {
	p := freshPool(t)
	ctx := context.Background()
	q := quality.New(p)
	m := &idMaker{}
	rid := historyRevision(t, p, "sanctions", "sanction-v1", "2023", m)
	base := historyContextParams("2023", rid)
	for name, change := range map[string]func(*quality.RecordHistoricalFineContextParams){
		"unknown contract":      func(a *quality.RecordHistoricalFineContextParams) { a.InputContract = "future" },
		"wrong source filename": func(a *quality.RecordHistoricalFineContextParams) { a.SourceFile = "invented.csv" },
		"wrong header":          func(a *quality.RecordHistoricalFineContextParams) { a.HeaderRecord++ },
		"invented exact day": func(a *quality.RecordHistoricalFineContextParams) {
			a.SourceClockPrecision = "DAY"
			a.SourceClockValue = "2024-06-01"
		},
		"invalid month":          func(a *quality.RecordHistoricalFineContextParams) { a.SourceClockValue = "2024-13" },
		"backwards source clock": func(a *quality.RecordHistoricalFineContextParams) { a.SourceClockValue = "2022-12" },
		"unsupported metadata":   func(a *quality.RecordHistoricalFineContextParams) { a.MetadataCount = 1 },
		"negative records":       func(a *quality.RecordHistoricalFineContextParams) { a.SourceRecordCount = -1 },
		"over budget":            func(a *quality.RecordHistoricalFineContextParams) { a.SourceRecordCount = 500001 },
	} {
		t.Run(name, func(t *testing.T) {
			tx, e := p.Begin(ctx)
			if e != nil {
				t.Fatal(e)
			}
			defer tx.Rollback(ctx)
			a := base
			change(&a)
			if _, e = quality.New(tx).RecordHistoricalFineContext(ctx, a); e == nil {
				t.Fatal("invalid source context accepted")
			}
		})
	}
	if _, e := q.RecordHistoricalFineContext(ctx, base); e != nil {
		t.Fatal(e)
	}
	good := historyObservationParams(t, "2023", rid, m)
	for name, change := range map[string]func(*quality.StageHistoricalFineObservationParams){
		"CPF":                       func(a *quality.StageHistoricalFineObservationParams) { a.PartyCnpj = "12345678900" },
		"unknown fingerprint":       func(a *quality.StageHistoricalFineObservationParams) { a.NativeKey = "guessed" },
		"money mismatch":            func(a *quality.StageHistoricalFineObservationParams) { a.AmountMinor++ },
		"negative amount":           func(a *quality.StageHistoricalFineObservationParams) { a.AmountMinor = -1 },
		"malformed money":           func(a *quality.StageHistoricalFineObservationParams) { a.AmountText.String = "v1,00" },
		"money overflow":            func(a *quality.StageHistoricalFineObservationParams) { a.AmountText.String = "92233720368547758,08" },
		"unknown paid coerced zero": func(a *quality.StageHistoricalFineObservationParams) { a.PaidMinor = pgtype.Int8{Valid: true} },
		"known paid erased":         func(a *quality.StageHistoricalFineObservationParams) { a.PaidText.String = "R$ 0,00" },
		"unknown paid plain dash":   func(a *quality.StageHistoricalFineObservationParams) { a.PaidText.String = "-" },
		"wrong AI label":            func(a *quality.StageHistoricalFineObservationParams) { a.ReferenceLabel.String = "Auto de Infração" },
		"missing label":             func(a *quality.StageHistoricalFineObservationParams) { a.ReferenceLabel.Valid = false },
		"missing reference":         func(a *quality.StageHistoricalFineObservationParams) { a.ReferenceRef.Valid = false },
		"missing department":        func(a *quality.StageHistoricalFineObservationParams) { a.Department.Valid = false },
		"invented transit":          func(a *quality.StageHistoricalFineObservationParams) { a.SourceTransitOn = a.DueOn },
		"infinite due":              func(a *quality.StageHistoricalFineObservationParams) { a.DueOn.InfinityModifier = pgtype.Infinity },
		"missing due":               func(a *quality.StageHistoricalFineObservationParams) { a.DueOn.Valid = false },
		"out of range due": func(a *quality.StageHistoricalFineObservationParams) {
			a.DueOn.Time = time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)
		},
		"extra unlabelled":   func(a *quality.StageHistoricalFineObservationParams) { a.UnlabelledField.String = "SIM" },
		"missing unlabelled": func(a *quality.StageHistoricalFineObservationParams) { a.UnlabelledField.Valid = false },
		"unicode separator":  func(a *quality.StageHistoricalFineObservationParams) { a.Status = "source\u2028separator" },
		"retained control":   func(a *quality.StageHistoricalFineObservationParams) { a.Status = "source\ncontrol" },
	} {
		t.Run(name, func(t *testing.T) {
			tx, e := p.Begin(ctx)
			if e != nil {
				t.Fatal(e)
			}
			defer tx.Rollback(ctx)
			a := good
			change(&a)
			if _, e = quality.New(tx).StageHistoricalFineObservation(ctx, a); e == nil {
				t.Fatal("invalid historical fact staged")
			}
		})
	}
	if _, e := q.StageHistoricalFineObservation(ctx, good); e != nil {
		t.Fatal("valid nullable paid refused", e)
	}
	if _, e := q.StageHistoricalFineNote(ctx, quality.StageHistoricalFineNoteParams{RevisionID: rid, EvidenceRef: "synthetic:1", Kind: "PUBLIC_PAYMENT_NOTE", NoteText: "private"}); e == nil {
		t.Fatal("private/foreign note accepted")
	}
}

func TestHistoricalFineStorageConcurrentStagesAndWithdrawalFence(t *testing.T) {
	p := freshPool(t)
	q := quality.New(p)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	m := &idMaker{}
	rid := historyRevision(t, p, "sanctions", "sanction-v1", "2020", m)
	cp := historyContextParams("2020", rid)
	good := historyObservationParams(t, "2020", rid, m)
	for _, run := range []func() error{
		func() error { _, e := q.RecordHistoricalFineContext(ctx, cp); return e },
		func() error { _, e := q.StageHistoricalFineObservation(ctx, good); return e },
		func() error {
			_, e := q.StageHistoricalFineNote(ctx, quality.StageHistoricalFineNoteParams{RevisionID: rid, EvidenceRef: cp.SourceFile + ":11", Kind: "BLANK_SEPARATOR"})
			return e
		},
	} {
		var successes atomic.Int32
		var wg sync.WaitGroup
		for range 8 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if e := run(); e == nil {
					successes.Add(1)
				} else if !errors.Is(e, pgx.ErrNoRows) {
					t.Error("concurrent stage failed", e)
				}
			}()
		}
		wg.Wait()
		if successes.Load() != 1 {
			t.Fatal("unique stage did not converge", successes.Load())
		}
	}
	for _, kind := range []string{"context", "observation", "note"} {
		t.Run(kind, func(t *testing.T) {
			blockedRID := historyRevision(t, p, "sanctions", "sanction-v1", "2020", m)
			contextParams := historyContextParams("2020", blockedRID)
			if kind != "context" {
				if _, e := q.RecordHistoricalFineContext(ctx, contextParams); e != nil {
					t.Fatal(e)
				}
			}
			locker, e := p.Begin(ctx)
			if e != nil {
				t.Fatal(e)
			}
			defer locker.Rollback(context.Background())
			if _, e = locker.Exec(ctx, "SELECT id FROM quality_revisions WHERE id=$1 FOR UPDATE", blockedRID); e != nil {
				t.Fatal(e)
			}
			conn, e := p.Acquire(ctx)
			if e != nil {
				t.Fatal(e)
			}
			defer conn.Release()
			pid := int32(conn.Conn().PgConn().PID())
			done := make(chan error, 1)
			observation := historyObservationParams(t, "2020", blockedRID, m)
			go func() {
				query := quality.New(conn)
				var e error
				switch kind {
				case "context":
					_, e = query.RecordHistoricalFineContext(ctx, contextParams)
				case "observation":
					_, e = query.StageHistoricalFineObservation(ctx, observation)
				default:
					_, e = query.StageHistoricalFineNote(ctx, quality.StageHistoricalFineNoteParams{RevisionID: blockedRID, EvidenceRef: contextParams.SourceFile + ":11", Kind: "BLANK_SEPARATOR"})
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
					t.Fatal("stage escaped revision lock", e)
				default:
				}
			}
			if _, e = locker.Exec(ctx, "UPDATE quality_revisions SET status='withdrawn' WHERE id=$1", blockedRID); e != nil {
				t.Fatal(e)
			}
			if e = locker.Commit(ctx); e != nil {
				t.Fatal(e)
			}
			select {
			case e := <-done:
				if !errors.Is(e, pgx.ErrNoRows) {
					t.Fatal("withdrawn revision gained staged facts", e)
				}
			case <-ctx.Done():
				t.Fatal("withdrawal did not release stage")
			}
			var facts int
			if e = p.QueryRow(ctx, `SELECT (SELECT count(*) FROM quality_sanction_events WHERE revision_id=$1)+(SELECT count(*) FROM quality_historical_fine_notes WHERE revision_id=$1)`, blockedRID).Scan(&facts); e != nil || facts != 0 {
				t.Fatal("withdrawn stage wrote facts", facts, e)
			}
		})
	}
	foreign := historyRevision(t, p, "pmqc", "pmqc-assay-v1", "2020", m)
	if _, e := q.RecordHistoricalFineContext(ctx, historyContextParams("2020", foreign)); !errors.Is(e, pgx.ErrNoRows) {
		t.Fatal("foreign stream accepted history context", e)
	}
	if _, e := q.StageHistoricalFineObservation(ctx, historyObservationParams(t, "2020", foreign, m)); !errors.Is(e, pgx.ErrNoRows) {
		t.Fatal("foreign stream accepted facts", e)
	}
	if _, e := q.StageHistoricalFineNote(ctx, quality.StageHistoricalFineNoteParams{RevisionID: foreign, EvidenceRef: "synthetic:1", Kind: "BLANK_SEPARATOR"}); !errors.Is(e, pgx.ErrNoRows) {
		t.Fatal("foreign stream accepted note", e)
	}
}

func TestHistoricalFineStorageInvalidDayNotesLocatorsAndAtomicFailure(t *testing.T) {
	p := freshPool(t)
	ctx := context.Background()
	q := quality.New(p)
	m := &idMaker{}
	group := historyRevision(t, p, "sanctions", "sanction-v1", "2016-2019", m)
	for _, day := range []string{"2020-02-30", "2020-13-01", "infinity", "10000-01-01", "2019-12-31"} {
		tx, e := p.Begin(ctx)
		if e != nil {
			t.Fatal(e)
		}
		a := historyContextParams("2016-2019", group)
		a.SourceClockValue = day
		_, e = quality.New(tx).RecordHistoricalFineContext(ctx, a)
		_ = tx.Rollback(ctx)
		if e == nil {
			t.Fatal("invalid source day accepted")
		}
	}
	rid := historyRevision(t, p, "sanctions", "sanction-v1", "2020", m)
	contextParams := historyContextParams("2020", rid)
	if _, e := q.RecordHistoricalFineContext(ctx, contextParams); e != nil {
		t.Fatal(e)
	}
	for _, locator := range []string{"foreign.csv:8", contextParams.SourceFile + ":7", contextParams.SourceFile + ":12", contextParams.SourceFile + ":08", contextParams.SourceFile + ":1e3"} {
		args := historyObservationParams(t, "2020", rid, m)
		args.EvidenceRef = locator
		if _, e := q.StageHistoricalFineObservation(ctx, args); !errors.Is(e, pgx.ErrNoRows) {
			t.Fatal("unverified observation locator accepted", e)
		}
		if _, e := q.StageHistoricalFineNote(ctx, quality.StageHistoricalFineNoteParams{RevisionID: rid, EvidenceRef: locator, Kind: "BLANK_SEPARATOR"}); !errors.Is(e, pgx.ErrNoRows) {
			t.Fatal("unverified note locator accepted", e)
		}
	}
	for _, note := range []quality.StageHistoricalFineNoteParams{{RevisionID: rid, EvidenceRef: contextParams.SourceFile + ":11", Kind: "BLANK_SEPARATOR", NoteText: "private"}, {RevisionID: rid, EvidenceRef: contextParams.SourceFile + ":11", Kind: "PUBLIC_PAYMENT_NOTE", NoteText: "private"}, {RevisionID: rid, EvidenceRef: contextParams.SourceFile + ":11", Kind: "future"}} {
		if _, e := q.StageHistoricalFineNote(ctx, note); e == nil {
			t.Fatal("unreviewed private source note accepted")
		}
	}
	// Context and a valid observation must both roll back on a later note failure.
	rollbackRID := historyRevision(t, p, "sanctions", "sanction-v1", "2020", m)
	tx, e := p.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	tq := quality.New(tx)
	if _, e = tq.RecordHistoricalFineContext(ctx, historyContextParams("2020", rollbackRID)); e != nil {
		t.Fatal(e)
	}
	if _, e = tq.StageHistoricalFineObservation(ctx, historyObservationParams(t, "2020", rollbackRID, m)); e != nil {
		t.Fatal(e)
	}
	if _, e = tq.StageHistoricalFineNote(ctx, quality.StageHistoricalFineNoteParams{RevisionID: rollbackRID, EvidenceRef: contextParams.SourceFile + ":11", Kind: "PUBLIC_PAYMENT_NOTE", NoteText: "private"}); e == nil {
		t.Fatal("late invalid note accepted")
	}
	if e = tx.Rollback(ctx); e != nil {
		t.Fatal(e)
	}
	var remaining int
	if e = p.QueryRow(ctx, `SELECT (SELECT count(*) FROM quality_historical_fine_revisions WHERE revision_id=$1)+(SELECT count(*) FROM quality_sanction_events WHERE revision_id=$1)+(SELECT count(*) FROM quality_historical_fine_notes WHERE revision_id=$1)`, rollbackRID).Scan(&remaining); e != nil || remaining != 0 {
		t.Fatal("partial historical source survived rollback", remaining, e)
	}
}
func TestNativeFineGeneratedIDCollisionRefusesWithoutPartialPublication(t *testing.T) {
	p := freshPool(t)
	ctx := context.Background()
	m := &idMaker{}
	pub := NewPublisher(p)
	first, e := pub.PublishNativeFines(ctx, nativeFineInput("b"), m.next)
	if e != nil {
		t.Fatal(e)
	}
	var reused pgtype.UUID
	if e = p.QueryRow(ctx, "SELECT id FROM quality_sanction_events LIMIT 1").Scan(&reused); e != nil {
		t.Fatal(e)
	}
	calls := 0
	next := func() (string, error) {
		calls++
		if calls == 3 {
			return uuidString(reused), nil
		}
		return m.next()
	}
	changed := nativeFineInput("e")
	changed.Observations[0].Status = "source correction"
	changed.Observations[0].ObservationKey = changed.Observations[0].ObservationIdentity()
	if _, e = pub.PublishNativeFines(ctx, changed, next); e == nil {
		t.Fatal("foreign generated ID collision accepted")
	}
	current, e := quality.New(p).GetCurrentRevision(ctx, quality.GetCurrentRevisionParams{Source: "sanctions", Stream: "sanction-v1"})
	if e != nil || uuidString(current) != first {
		t.Fatal("failed ID collision switched pointer", e)
	}
	var revisions, clocks int
	if e = p.QueryRow(ctx, "SELECT (SELECT count(*) FROM quality_revisions),(SELECT count(*) FROM quality_native_fine_revisions)").Scan(&revisions, &clocks); e != nil || revisions != 1 || clocks != 1 {
		t.Fatal("ID collision left a partial revision/context", revisions, clocks, e)
	}
}
