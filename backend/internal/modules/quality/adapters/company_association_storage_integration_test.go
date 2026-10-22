//go:build integration

package adapters

import (
	"context"
	"errors"
	"fmt"
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
	qualityread "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters/read"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

func companyContextParams(rid pgtype.UUID, count int32) quality.RecordCompanyAssociationContextParams {
	day, _ := pgDate("2026-10-20")
	return quality.RecordCompanyAssociationContextParams{RevisionID: rid, PolicyVersion: domain.CompanyObservationPolicyVersion, AssociationScope: domain.CurrentCompanyOnly, DecisionDay: day, ObservationCount: count}
}
func TestCompanyAssociationStorageAcceptsOnlySameRevisionNativeFacts(t *testing.T) {
	p := freshPool(t)
	ctx := context.Background()
	q := quality.New(p)
	m := &idMaker{}
	rid := historyRevision(t, p, "sanctions", "sanction-v1", "2025", m)
	if _, e := q.RecordNativeFineClock(ctx, quality.RecordNativeFineClockParams{RevisionID: rid, ReportYear: 2025, ExtractionMonth: "2026-01"}); e != nil {
		t.Fatal(e)
	}
	event := nativeFineParams(t, uuidString(rid), m)
	if _, e := q.StageNativeFineObservation(ctx, event); e != nil {
		t.Fatal(e)
	}
	if _, e := q.RecordCompanyAssociationContext(ctx, companyContextParams(rid, 1)); e != nil {
		t.Fatal(e)
	}
	if _, e := q.StageCompanyAssociation(ctx, quality.StageCompanyAssociationParams{RevisionID: rid, RecordKey: event.NativeKey, ResolutionKind: "unmatched", Reason: domain.ReasonUnmatchedID}); e != nil {
		t.Fatal(e)
	}
	rows, e := q.CompanyAssociationDecisions(ctx, rid)
	if e != nil || len(rows) != 1 || rows[0].StationID.Valid || rows[0].ResolutionKind != "unmatched" {
		t.Fatal("native company decision lost", e)
	}
	c, e := q.CompanyAssociationContext(ctx, rid)
	if e != nil || c.ObservationCount != 1 || c.HistoricalBusinessWindowKnown || c.AssociationScope != domain.CurrentCompanyOnly || c.PolicyVersion != "current-company-v1" {
		t.Fatal("company-only source context lost", e)
	}
}

func companyStorageSource(t *testing.T, p *pgxpool.Pool, m *idMaker, period string, count int) (pgtype.UUID, []string) {
	t.Helper()
	ctx := context.Background()
	q := quality.New(p)
	rid := historyRevision(t, p, "sanctions", "sanction-v1", period, m)
	if period == "2025" {
		if _, e := q.RecordNativeFineClock(ctx, quality.RecordNativeFineClockParams{RevisionID: rid, ReportYear: 2025, ExtractionMonth: "2026-01"}); e != nil {
			t.Fatal(e)
		}
	} else {
		cp := historyContextParams(period, rid)
		cp.SourceRecordCount = int32(count)
		cp.MetadataCount = 0
		if _, e := q.RecordHistoricalFineContext(ctx, cp); e != nil {
			t.Fatal(e)
		}
	}
	keys := make([]string, count)
	for i := range keys {
		keys[i] = fmt.Sprintf("observation:%064x", i+1)
		if period == "2025" {
			a := nativeFineParams(t, uuidString(rid), m)
			a.NativeKey = keys[i]
			if _, e := q.StageNativeFineObservation(ctx, a); e != nil {
				t.Fatal(e)
			}
		} else {
			a := historyObservationParams(t, period, rid, m)
			a.NativeKey = keys[i]
			profile, _ := application.HistoricalFineProfile(period)
			a.EvidenceRef = fmt.Sprintf("%s:%d", profile.SourceFile, profile.HeaderRecord+i+1)
			if _, e := q.StageHistoricalFineObservation(ctx, a); e != nil {
				t.Fatal(e)
			}
		}
	}
	return rid, keys
}

func TestCompanyAssociationStorageHistoricalEmptyAndResolutionCensus(t *testing.T) {
	p := freshPool(t)
	ctx := context.Background()
	q := quality.New(p)
	m := &idMaker{}
	if _, e := p.Exec(ctx, `INSERT INTO directory_stations(id,display_name) VALUES($1,'Synthetic company station')`, reportStation); e != nil {
		t.Fatal(e)
	}
	for _, period := range []string{"2016-2019", "2020", "2021", "2022", "2023", "2024", "2025"} {
		for _, n := range []int{0, 5} {
			rid, keys := companyStorageSource(t, p, m, period, n)
			if _, e := q.RecordCompanyAssociationContext(ctx, companyContextParams(rid, int32(n))); e != nil {
				t.Fatal(period, n, e)
			}
			for i, key := range keys {
				kind := []string{"bound", "unmatched", "ambiguous", "wrong_branch", "succession_review"}[i]
				reason := []string{"exact current cnpj; historical business window unknown", domain.ReasonUnmatchedID, domain.ReasonAmbiguous, domain.ReasonWrongBranch, domain.ReasonSuccession}[i]
				station := pgtype.UUID{}
				if i == 0 {
					station, _ = mustUUID(reportStation)
				}
				if _, e := q.StageCompanyAssociation(ctx, quality.StageCompanyAssociationParams{RevisionID: rid, RecordKey: key, ResolutionKind: kind, Reason: reason, StationID: station}); e != nil {
					t.Fatal(period, kind, e)
				}
			}
			counts, e := q.CompanyAssociationCounts(ctx, rid)
			if e != nil || counts.Total != int64(n) || counts.Total != counts.BoundCount+counts.UnmatchedCount+counts.AmbiguousCount+counts.WrongBranchCount+counts.SuccessionCount {
				t.Fatal("association census lost", e)
			}
			if n == 5 && (counts.BoundCount != 1 || counts.UnmatchedCount != 1 || counts.AmbiguousCount != 1 || counts.WrongBranchCount != 1 || counts.SuccessionCount != 1) {
				t.Fatal("resolution census changed")
			}
			rows, e := q.CompanyAssociationDecisions(ctx, rid)
			if e != nil || len(rows) != n {
				t.Fatal("decisions lost", e)
			}
			for _, row := range rows {
				if row.PartyCnpj != "11222333000181" || row.StationID.Valid != (row.ResolutionKind == "bound") {
					t.Fatal("source identity or review misattributed")
				}
			}
		}
	}
}

func TestCompanyAssociationStorageRejectsInvalidContextsAndDecisions(t *testing.T) {
	p := freshPool(t)
	ctx := context.Background()
	q := quality.New(p)
	m := &idMaker{}
	rid, keys := companyStorageSource(t, p, m, "2025", 1)
	good := companyContextParams(rid, 1)
	for name, change := range map[string]func(*quality.RecordCompanyAssociationContextParams){
		"policy":                     func(a *quality.RecordCompanyAssociationContextParams) { a.PolicyVersion = "invented" },
		"scope":                      func(a *quality.RecordCompanyAssociationContextParams) { a.AssociationScope = "HISTORICAL_SITE" },
		"invented historical window": func(a *quality.RecordCompanyAssociationContextParams) { a.HistoricalBusinessWindowKnown = true },
		"missing day":                func(a *quality.RecordCompanyAssociationContextParams) { a.DecisionDay.Valid = false },
		"infinite day": func(a *quality.RecordCompanyAssociationContextParams) {
			a.DecisionDay.InfinityModifier = pgtype.Infinity
		},
		"out of calendar": func(a *quality.RecordCompanyAssociationContextParams) {
			a.DecisionDay.Time = time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)
		},
		"wrong census":    func(a *quality.RecordCompanyAssociationContextParams) { a.ObservationCount = 2 },
		"negative census": func(a *quality.RecordCompanyAssociationContextParams) { a.ObservationCount = -1 },
	} {
		t.Run(name, func(t *testing.T) {
			tx, e := p.Begin(ctx)
			if e != nil {
				t.Fatal(e)
			}
			defer tx.Rollback(ctx)
			a := good
			change(&a)
			if _, e := quality.New(tx).RecordCompanyAssociationContext(ctx, a); e == nil {
				t.Fatal("invalid company context staged")
			}
		})
	}
	params := quality.StageCompanyAssociationParams{RevisionID: rid, RecordKey: keys[0], ResolutionKind: "unmatched", Reason: domain.ReasonUnmatchedID}
	if _, e := q.StageCompanyAssociation(ctx, params); !errors.Is(e, pgx.ErrNoRows) {
		t.Fatal("missing company context allowed decision", e)
	}
	if _, e := q.RecordCompanyAssociationContext(ctx, good); e != nil {
		t.Fatal(e)
	}
	for name, change := range map[string]func(*quality.StageCompanyAssociationParams){
		"bound without station": func(a *quality.StageCompanyAssociationParams) {
			a.ResolutionKind = "bound"
			a.Reason = "exact current cnpj; historical business window unknown"
		},
		"review with station": func(a *quality.StageCompanyAssociationParams) { a.StationID, _ = mustUUID(reportStation) },
		"missing station FK": func(a *quality.StageCompanyAssociationParams) {
			a.ResolutionKind = "bound"
			a.Reason = "exact current cnpj; historical business window unknown"
			a.StationID, _ = mustUUID(reportStation)
		},
		"unknown kind":     func(a *quality.StageCompanyAssociationParams) { a.ResolutionKind = "APPROVED_FINAL_GUILT" },
		"wrong reason":     func(a *quality.StageCompanyAssociationParams) { a.Reason = "private unknown reason" },
		"wrong record":     func(a *quality.StageCompanyAssociationParams) { a.RecordKey = "observation:" + strings.Repeat("f", 64) },
		"foreign revision": func(a *quality.StageCompanyAssociationParams) { a.RevisionID, _ = mustUUID(reportStation) },
	} {
		t.Run(name, func(t *testing.T) {
			tx, e := p.Begin(ctx)
			if e != nil {
				t.Fatal(e)
			}
			defer tx.Rollback(ctx)
			a := params
			change(&a)
			if _, e := quality.New(tx).StageCompanyAssociation(ctx, a); e == nil {
				t.Fatal("invalid company decision staged")
			}
		})
	}
	if _, e := q.StageCompanyAssociation(ctx, params); e != nil {
		t.Fatal(e)
	}
	for _, status := range []string{"published", "withdrawn"} {
		blocked, blockKeys := companyStorageSource(t, p, m, "2024", 1)
		if _, e := q.RecordCompanyAssociationContext(ctx, companyContextParams(blocked, 1)); e != nil {
			t.Fatal(e)
		}
		if _, e := p.Exec(ctx, `UPDATE quality_revisions SET status=$2,published_at=now() WHERE id=$1`, blocked, status); e != nil {
			t.Fatal(e)
		}
		if _, e := q.StageCompanyAssociation(ctx, quality.StageCompanyAssociationParams{RevisionID: blocked, RecordKey: blockKeys[0], ResolutionKind: "unmatched", Reason: domain.ReasonUnmatchedID}); !errors.Is(e, pgx.ErrNoRows) {
			t.Fatal("non-staging decision accepted", e)
		}
		if _, e := q.RecordCompanyAssociationContext(ctx, companyContextParams(blocked, 1)); !errors.Is(e, pgx.ErrNoRows) {
			t.Fatal("non-staging context accepted", e)
		}
	}
	foreign := historyRevision(t, p, "pmqc", "pmqc-assay-v1", "2026-09", m)
	if _, e := q.RecordCompanyAssociationContext(ctx, companyContextParams(foreign, 0)); !errors.Is(e, pgx.ErrNoRows) {
		t.Fatal("company context entered foreign source", e)
	}
	legacy := historyRevision(t, p, "sanctions", "sanction-v1", "2015", m)
	if _, e := q.RecordCompanyAssociationContext(ctx, companyContextParams(legacy, 0)); !errors.Is(e, pgx.ErrNoRows) {
		t.Fatal("company context guessed absent native profile", e)
	}
}

func TestCompanyAssociationStorageConcurrentConvergenceAndWithdrawalFences(t *testing.T) {
	p := freshPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	q := quality.New(p)
	m := &idMaker{}
	rid, keys := companyStorageSource(t, p, m, "2025", 1)
	for _, kind := range []string{"context", "decision"} {
		var winners atomic.Int32
		errs := make(chan error, 8)
		var wg sync.WaitGroup
		for range 8 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				var e error
				if kind == "context" {
					_, e = q.RecordCompanyAssociationContext(ctx, companyContextParams(rid, 1))
				} else {
					_, e = q.StageCompanyAssociation(ctx, quality.StageCompanyAssociationParams{RevisionID: rid, RecordKey: keys[0], ResolutionKind: "unmatched", Reason: domain.ReasonUnmatchedID})
				}
				if e == nil {
					winners.Add(1)
				} else if !errors.Is(e, pgx.ErrNoRows) {
					errs <- e
				}
			}()
		}
		wg.Wait()
		close(errs)
		for e := range errs {
			t.Fatal("company stage convergence", e)
		}
		if winners.Load() != 1 {
			t.Fatal("company stage did not converge", kind, winners.Load())
		}
	}
	for _, kind := range []string{"context", "decision"} {
		t.Run(kind, func(t *testing.T) {
			blocked, keys := companyStorageSource(t, p, m, "2024", 1)
			if kind == "decision" {
				if _, e := q.RecordCompanyAssociationContext(ctx, companyContextParams(blocked, 1)); e != nil {
					t.Fatal(e)
				}
			}
			locker, e := p.Begin(ctx)
			if e != nil {
				t.Fatal(e)
			}
			defer locker.Rollback(context.Background())
			if _, e := locker.Exec(ctx, `SELECT id FROM quality_revisions WHERE id=$1 FOR UPDATE`, blocked); e != nil {
				t.Fatal(e)
			}
			conn, e := p.Acquire(ctx)
			if e != nil {
				t.Fatal(e)
			}
			defer conn.Release()
			pid := int32(conn.Conn().PgConn().PID())
			done := make(chan error, 1)
			go func() {
				query := quality.New(conn)
				var e error
				if kind == "context" {
					_, e = query.RecordCompanyAssociationContext(ctx, companyContextParams(blocked, 1))
				} else {
					_, e = query.StageCompanyAssociation(ctx, quality.StageCompanyAssociationParams{RevisionID: blocked, RecordKey: keys[0], ResolutionKind: "unmatched", Reason: domain.ReasonUnmatchedID})
				}
				done <- e
			}()
			for {
				var n int
				if e := p.QueryRow(ctx, `SELECT cardinality(pg_blocking_pids($1))`, pid).Scan(&n); e != nil {
					t.Fatal(e)
				}
				if n > 0 {
					break
				}
				select {
				case e := <-done:
					t.Fatal("company stage escaped revision lock", e)
				default:
				}
			}
			if _, e := locker.Exec(ctx, `UPDATE quality_revisions SET status='withdrawn' WHERE id=$1`, blocked); e != nil {
				t.Fatal(e)
			}
			if e := locker.Commit(ctx); e != nil {
				t.Fatal(e)
			}
			select {
			case e := <-done:
				if !errors.Is(e, pgx.ErrNoRows) {
					t.Fatal("withdrawn revision gained company facts", e)
				}
			case <-ctx.Done():
				t.Fatal("withdrawal did not release company stage")
			}
			counts, e := q.CompanyAssociationCounts(ctx, blocked)
			if e != nil || counts.Total != 0 {
				t.Fatal("withdrawn company decisions persisted", e)
			}
			if kind == "context" {
				if _, e := q.CompanyAssociationContext(ctx, blocked); !errors.Is(e, pgx.ErrNoRows) {
					t.Fatal("withdrawn context persisted", e)
				}
			}
		})
	}
}

func TestCompanyAssociationStorageFailedDecisionRollsBackWholeStaging(t *testing.T) {
	p := freshPool(t)
	ctx := context.Background()
	m := &idMaker{}
	rid := historyRevision(t, p, "sanctions", "sanction-v1", "2025", m)
	tx, e := p.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(ctx)
	q := quality.New(tx)
	if _, e := q.RecordNativeFineClock(ctx, quality.RecordNativeFineClockParams{RevisionID: rid, ReportYear: 2025, ExtractionMonth: "2026-01"}); e != nil {
		t.Fatal(e)
	}
	a := nativeFineParams(t, uuidString(rid), m)
	b := nativeFineParams(t, uuidString(rid), m)
	b.NativeKey = "observation:" + strings.Repeat("b", 64)
	for _, row := range []quality.StageNativeFineObservationParams{a, b} {
		if _, e := q.StageNativeFineObservation(ctx, row); e != nil {
			t.Fatal(e)
		}
	}
	if _, e := q.RecordCompanyAssociationContext(ctx, companyContextParams(rid, 2)); e != nil {
		t.Fatal(e)
	}
	if _, e := q.StageCompanyAssociation(ctx, quality.StageCompanyAssociationParams{RevisionID: rid, RecordKey: a.NativeKey, ResolutionKind: "unmatched", Reason: domain.ReasonUnmatchedID}); e != nil {
		t.Fatal(e)
	}
	if _, e := q.StageCompanyAssociation(ctx, quality.StageCompanyAssociationParams{RevisionID: rid, RecordKey: b.NativeKey, ResolutionKind: "bound", Reason: "exact current cnpj; historical business window unknown"}); e == nil {
		t.Fatal("bad later decision accepted")
	}
	if e := tx.Rollback(ctx); e != nil {
		t.Fatal(e)
	}
	var n int
	if e := p.QueryRow(ctx, `SELECT (SELECT count(*) FROM quality_native_fine_revisions WHERE revision_id=$1)+(SELECT count(*) FROM quality_sanction_events WHERE revision_id=$1)+(SELECT count(*) FROM quality_company_association_revisions WHERE revision_id=$1)+(SELECT count(*) FROM quality_company_associations WHERE revision_id=$1)`, rid).Scan(&n); e != nil || n != 0 {
		t.Fatal("failed company batch partially staged", n, e)
	}
}

func TestCompanyAssociationStorageUpgradePreservesAllFrozenSourcesAndPointers(t *testing.T) {
	prior := fstest.MapFS{}
	entries, e := fs.ReadDir(dbmigrations.Files, ".")
	if e != nil {
		t.Fatal(e)
	}
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() >= "000109" {
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
	pub := NewPublisher(p)
	if _, e := pub.Publish(ctx, sampleInput("b"), m.next); e != nil {
		t.Fatal(e)
	}
	legacy, e := pub.PublishEvents(ctx, legacyFineInput(), m.next)
	if e != nil {
		t.Fatal(e)
	}
	native, e := pub.PublishNativeFines(ctx, nativeFineInput("c"), m.next)
	if e != nil {
		t.Fatal(e)
	}
	historical, e := pub.PublishHistoricalFines(ctx, historicalFineInput("2021", "d"), m.next)
	if e != nil {
		t.Fatal(e)
	}
	tables := []string{"quality_editions", "quality_revisions", "quality_resources", "quality_manifests", "quality_periods", "quality_revision_periods", "quality_revision_provenance", "quality_revision_resources", "quality_assays", "quality_links", "quality_sanction_events", "quality_current_revisions", "quality_native_fine_revisions", "quality_historical_fine_revisions", "quality_historical_fine_notes"}
	snapshot := func() map[string]string {
		result := map[string]string{}
		for _, table := range tables {
			var hash string
			if e := p.QueryRow(ctx, `SELECT md5(coalesce(jsonb_agg(to_jsonb(v) ORDER BY to_jsonb(v)::text)::text,'[]')) FROM `+pgx.Identifier{table}.Sanitize()+` v`).Scan(&hash); e != nil {
				t.Fatal("upgrade snapshot", table, e)
			}
			result[table] = hash
		}
		return result
	}
	before := snapshot()
	applied, e := migrate.Apply(ctx, p.Config().ConnConfig.ConnString(), dbmigrations.Files)
	if e != nil || !slices.Equal(applied, []string{"000109", "000110"}) {
		t.Fatal("company scope upgrade", applied, e)
	}
	if !reflect.DeepEqual(before, snapshot()) {
		t.Fatal("company upgrade changed frozen source data")
	}
	applied, e = migrate.Apply(ctx, p.Config().ConnConfig.ConnString(), dbmigrations.Files)
	if e != nil || len(applied) != 0 {
		t.Fatal("company migration replay", e)
	}
	var n int
	if e := p.QueryRow(ctx, `SELECT (SELECT count(*) FROM quality_company_association_revisions)+(SELECT count(*) FROM quality_company_associations)`).Scan(&n); e != nil || n != 0 {
		t.Fatal("company upgrade invented associations", n, e)
	}
	for _, revision := range []string{legacy, native, historical} {
		if _, e := qualityread.NewReader(p).AtRevision(revision).RevisionLease(ctx, time.Now(), "sanctions", "sanction-v1", revision); e != nil {
			t.Fatal("upgrade lost frozen lease", e)
		}
	}
	if replay, e := pub.PublishNativeFines(ctx, nativeFineInput("c"), m.next); e != nil || replay != native {
		t.Fatal("upgrade changed prior publication identity", e)
	}
}
