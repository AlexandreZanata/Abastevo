//go:build integration

package adapters

import (
	"context"
	"encoding/json"
	"errors"
	dbmigrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	account "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
	"io/fs"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"

	dbquality "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/quality"
	identity "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/adapters"
	identitydomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/domain"
	moderation "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/moderation/adapters"
	moderationapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/moderation/application"
	moderationdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/moderation/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/domain"
	jobs "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func reviewCase(t *testing.T, s *ReportStore, id string, revision int32, n int) (string, string) {
	t.Helper()
	ctx := context.Background()
	tx, e := s.Pool.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	rid, _ := mustUUID(id)
	target, _ := mustUUID(reportUUID(n))
	target, e = dbquality.New(tx).EnsureReportReviewTarget(ctx, dbquality.EnsureReportReviewTargetParams{ID: target, ReportID: rid, ReportRevision: pgtype.Int4{Int32: revision, Valid: true}})
	if e != nil {
		t.Fatal(e)
	}
	c, _, e := moderationdomain.NewCase(moderationdomain.CaseParams{ID: reportUUID(n + 1), TargetType: moderationdomain.TargetQualityReport, TargetID: uuidString(target), Priority: "P2", Reason: "EXPERIENCE", OpenedAt: s.now()})
	if e != nil {
		t.Fatal(e)
	}
	cid, _, e := moderation.NewStore(s.Pool).OpenCaseTx(ctx, tx, c)
	if e != nil {
		t.Fatal(e)
	}
	if e = tx.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	return uuidString(target), cid
}
func reviewPorts(s *ReportStore) moderationapp.ActPorts {
	return moderationapp.ActPorts{Clock: s.now, NewID: jobs.NewUUIDv4, Store: moderation.NewStore(s.Pool), ApplyQualityReport: s.ApplyReviewTx, EnqueueJob: func(ctx context.Context, tx pgx.Tx, kind string, payload []byte, dedupe string) error {
		_, e := jobs.Enqueue(ctx, tx, kind, payload, dedupe, 5, time.Time{})
		return e
	}}
}
func actReview(ctx context.Context, p moderationapp.ActPorts, caseID, action, reason string) error {
	_, e := moderationapp.Act(ctx, p, moderationapp.Caller{OperatorID: "synthetic-restricted-operator"}, moderationapp.ActDTO{CaseID: caseID, Action: action, Reason: reason})
	return e
}

func TestDocumentFreeReviewSQLAppealStaleAndErasure(t *testing.T) {
	s, a, b, c := reportFixture(t)
	ctx := context.Background()
	if _, e := s.Execute(ctx, a, c); e != nil {
		t.Fatal(e)
	}
	target, cid := reviewCase(t, s, c.ReportID, 1, 501)
	queued, next, e := moderation.NewStore(s.Pool).QualityQueue(ctx, "")
	if e != nil || len(queued) != 1 || queued[0].TargetID != target || next != "" {
		t.Fatal("restricted queue", queued, next, e)
	}
	p := reviewPorts(s)
	if _, e := moderationapp.Act(ctx, p, moderationapp.Caller{}, moderationapp.ActDTO{CaseID: cid, Action: "RESOLVE", Reason: "CLAIM_RECORDED"}); !errors.Is(e, moderationdomain.ErrUnauthorized) {
		t.Fatal("anonymous operator", e)
	}
	if e := actReview(ctx, p, cid, "RESOLVE", "PURCHASE_VERIFIED"); !errors.Is(e, domain.ErrReportInvalid) {
		t.Fatal("false proof", e)
	}
	if e := actReview(ctx, p, cid, "RESOLVE", "CLAIM_RECORDED"); e != nil {
		t.Fatal(e)
	}
	r, e := s.Get(ctx, a, c.ReportID)
	if e != nil || r.Status != "REVIEWED" || r.Revision != 2 || r.EvidenceLevel != "USER_CLAIM" || r.ReviewReason != "CLAIM_RECORDED" {
		t.Fatal(r, e)
	}
	appeal := domain.ReportCommand{CommandID: reportUUID(510), ReportID: c.ReportID, Action: "APPEAL", ExpectedRevision: 2, Text: "Synthetic private appeal"}
	if _, e = s.Execute(ctx, b, appeal); !errors.Is(e, domain.ErrReportNotFound) {
		t.Fatal("appeal IDOR", e)
	}
	if _, e = s.Execute(ctx, a, appeal); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Execute(ctx, a, appeal); e != nil {
		t.Fatal("appeal replay", e)
	}
	target2, cid2 := reviewCase(t, s, c.ReportID, 3, 520)
	if target2 == target {
		t.Fatal("revision target reused")
	}
	tx, _ := s.Pool.Begin(ctx)
	e = s.ApplyReviewTx(ctx, tx, target, reportUUID(530), "INVALIDATE", "ABUSE", s.now())
	_ = tx.Rollback(ctx)
	if !errors.Is(e, domain.ErrReportConflict) {
		t.Fatal("old decision changed appeal", e)
	}
	view, e := s.InspectReview(ctx, target2)
	if e != nil || view.Stale || view.AppealText != appeal.Text {
		t.Fatal(view, e)
	}
	if e = actReview(ctx, p, cid2, "INVALIDATE", "PRIVACY"); e != nil {
		t.Fatal(e)
	}
	r, e = s.Get(ctx, a, c.ReportID)
	if e != nil || r.Text != "" || r.Status != "HIDDEN" || r.Revision != 4 {
		t.Fatal(r, e)
	}
	var count int
	_ = s.Pool.QueryRow(ctx, "SELECT count(*) FROM quality_report_appeals").Scan(&count)
	if count != 0 {
		t.Fatal("appeal retained after privacy review")
	}
	if _, e = s.Execute(ctx, a, domain.ReportCommand{CommandID: reportUUID(540), ReportID: c.ReportID, Action: "WITHDRAW", ExpectedRevision: 4}); e != nil {
		t.Fatal("hidden owner withdrawal", e)
	}
	if _, e = s.EraseAccountBatch(ctx, a.Owner); e != nil {
		t.Fatal(e)
	}
	for _, table := range []string{"quality_reports", "quality_report_appeals", "quality_report_review_targets"} {
		if e = s.Pool.QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&count); e != nil || count != 0 {
			t.Fatal(table, count, e)
		}
	}
	if e = s.Pool.QueryRow(ctx, "SELECT count(*) FROM quality_report_review_decisions").Scan(&count); e != nil || count != 2 {
		t.Fatal("minimal audit", count, e)
	}
	var raw []byte
	_ = s.Pool.QueryRow(ctx, "SELECT json_agg(d)::text FROM quality_report_review_decisions d").Scan(&raw)
	if !json.Valid(raw) {
		t.Fatal("audit invalid")
	}
}

func TestDocumentFreeReviewSQLAtomicFailureAndOwnerRace(t *testing.T) {
	s, a, _, c := reportFixture(t)
	ctx := context.Background()
	if _, e := s.Execute(ctx, a, c); e != nil {
		t.Fatal(e)
	}
	_, cid := reviewCase(t, s, c.ReportID, 1, 601)
	p := reviewPorts(s)
	p.EnqueueJob = func(context.Context, pgx.Tx, string, []byte, string) error {
		return errors.New("synthetic queue failure")
	}
	if e := actReview(ctx, p, cid, "INVALIDATE", "PRIVACY"); e == nil {
		t.Fatal("queue failure hidden")
	}
	r, _ := s.Get(ctx, a, c.ReportID)
	if r.Revision != 1 || r.Text != c.Text {
		t.Fatal("effect escaped rollback", r)
	}
	var n int
	_ = s.Pool.QueryRow(ctx, "SELECT count(*) FROM quality_report_review_decisions").Scan(&n)
	if n != 0 {
		t.Fatal("audit escaped rollback")
	}
	p = reviewPorts(s)
	edit := c
	edit.Action = "EDIT"
	edit.CommandID = reportUUID(610)
	edit.ExpectedRevision = 1
	edit.Text = "Concurrent synthetic edit"
	var wins atomic.Int32
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, e := s.Execute(ctx, a, edit)
		if e == nil {
			wins.Add(1)
		} else if !errors.Is(e, domain.ErrReportConflict) {
			t.Error(e)
		}
	}()
	go func() {
		defer wg.Done()
		e := actReview(ctx, p, cid, "RESOLVE", "CLAIM_RECORDED")
		if e == nil {
			wins.Add(1)
		} else if !errors.Is(e, domain.ErrReportConflict) {
			t.Error(e)
		}
	}()
	wg.Wait()
	if wins.Load() != 1 {
		t.Fatal("revision race", wins.Load())
	}
	r, _ = s.Get(ctx, a, c.ReportID)
	if r.Revision != 2 {
		t.Fatal("revision fork", r)
	}
}

func TestDocumentFreeReportSQLTransactionalBudget(t *testing.T) {
	s, a, _, c := reportFixture(t)
	ctx := context.Background()
	policy := identitydomain.DefaultQuotaPolicy()
	policy.Ops[identitydomain.OperationWrite] = identitydomain.OperationQuota{Limit: 2, Window: time.Hour}
	limiter := identity.NewLimiter(s.Pool, policy)
	s.ConsumeBudget = func(ctx context.Context, tx pgx.Tx, owner string) error {
		_, e := limiter.CheckTx(ctx, tx, "synthetic-account:"+owner, identitydomain.OperationWrite)
		return e
	}
	s.Enqueue = func(context.Context, pgx.Tx, []byte, string) error { return errors.New("synthetic job failure") }
	if _, e := s.Execute(ctx, a, c); e == nil {
		t.Fatal("job failure hidden")
	}
	var n int
	_ = s.Pool.QueryRow(ctx, "SELECT count(*) FROM identity_rate_windows").Scan(&n)
	if n != 0 {
		t.Fatal("failed command charged quota", n)
	}
	s.Enqueue = nil
	if _, e := s.Execute(ctx, a, c); e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 4; i++ {
		if _, e := s.Execute(ctx, a, c); e != nil {
			t.Fatal("replay charged", e)
		}
	}
	edit := c
	edit.Action = "EDIT"
	edit.CommandID = reportUUID(701)
	edit.ExpectedRevision = 1
	if _, e := s.Execute(ctx, a, edit); e != nil {
		t.Fatal(e)
	}
	edit.CommandID = reportUUID(702)
	edit.ExpectedRevision = 2
	if _, e := s.Execute(ctx, a, edit); !errors.Is(e, identitydomain.ErrQuotaExceeded) {
		t.Fatal("quota not enforced", e)
	}
	if _, e := s.Execute(ctx, a, domain.ReportCommand{CommandID: reportUUID(703), ReportID: c.ReportID, Action: "WITHDRAW", ExpectedRevision: 2}); e != nil {
		t.Fatal("quota blocks rights", e)
	}
}

func TestDocumentFreeReviewUpgradeLegacyUnbound(t *testing.T) {
	ctx := context.Background()
	prior := fstest.MapFS{}
	entries, e := fs.ReadDir(dbmigrations.Files, ".")
	if e != nil {
		t.Fatal(e)
	}
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() >= "000099" {
			continue
		}
		b, e := fs.ReadFile(dbmigrations.Files, entry.Name())
		if e != nil {
			t.Fatal(e)
		}
		prior[entry.Name()] = &fstest.MapFile{Data: b}
	}
	p := freshPoolWithMigrations(t, prior)
	for _, query := range []string{
		"INSERT INTO accounts(id,alias,status) VALUES('" + reportUUID(1) + "','synthetic-legacy-review','active')",
		"INSERT INTO directory_stations(id,display_name) VALUES('" + reportStation + "','Synthetic legacy station')",
		"INSERT INTO quality_reports(id,account_id,kind,station_id,fuel,occurred_on,symptom,body,revision,status,created_at,updated_at) VALUES('" + reportUUID(102) + "','" + reportUUID(1) + "','EXPERIENCE','" + reportStation + "','ETHANOL','2026-10-13','OTHER','Legacy private claim',7,'PENDING_REVIEW',now(),now())",
	} {
		if _, e = p.Exec(ctx, query); e != nil {
			t.Fatal(e)
		}
	}
	m := moderation.NewStore(p)
	c, _, e := moderationdomain.NewCase(moderationdomain.CaseParams{ID: reportUUID(801), TargetID: reportUUID(102), TargetType: moderationdomain.TargetQualityReport, Reason: "EXPERIENCE", Priority: "P2", OpenedAt: time.Now()})
	if e != nil {
		t.Fatal(e)
	}
	if _, _, e = m.OpenCase(ctx, c); e != nil {
		t.Fatal(e)
	}
	applied, e := migrate.Apply(ctx, p.Config().ConnConfig.ConnString(), dbmigrations.Files)
	if e != nil || !slices.Equal(applied, []string{"000099", "000100", "000101"}) {
		t.Fatal(applied, e)
	}
	applied, e = migrate.Apply(ctx, p.Config().ConnConfig.ConnString(), dbmigrations.Files)
	if e != nil || len(applied) != 0 {
		t.Fatal("replay", applied, e)
	}
	s := &ReportStore{Pool: p}
	view, e := s.InspectReview(ctx, reportUUID(102))
	if e != nil || !view.Stale || view.BoundRevision != 0 || view.Current.Revision != 7 {
		t.Fatal("legacy revision invented", view, e)
	}
	ports := reviewPorts(s)
	if e = actReview(ctx, ports, c.ID, "RESOLVE", "CLAIM_RECORDED"); !errors.Is(e, domain.ErrReportConflict) {
		t.Fatal("legacy decision applied", e)
	}
	if e = actReview(ctx, ports, c.ID, "DISMISS", "SUPERSEDED"); e != nil {
		t.Fatal(e)
	}
	var rev int
	var status string
	if e = p.QueryRow(ctx, "SELECT revision,status FROM quality_reports").Scan(&rev, &status); e != nil || rev != 7 || status != "PENDING_REVIEW" {
		t.Fatal("legacy report changed", rev, status, e)
	}
	var nullable bool
	if e = p.QueryRow(ctx, "SELECT report_revision IS NULL FROM quality_report_review_decisions").Scan(&nullable); e != nil || !nullable {
		t.Fatal("invented decision provenance", e)
	}
}

func TestDocumentFreeGroupSQLPrivateAccountsAndOfficialIsolation(t *testing.T) {
	s, a, b, c := reportFixture(t)
	ctx := context.Background()
	maker := &idMaker{}
	for history, conformity := range []string{"CONFORMING", "NONCONFORMING"} {
		input := sampleInput("b")
		if conformity == "NONCONFORMING" {
			input = sampleInput("c")
		}
		input.Assays[0].Conformity = conformity
		if _, e := NewPublisher(s.Pool).Publish(ctx, input, maker.next); e != nil {
			t.Fatal(e)
		}
		var before string
		if e := s.Pool.QueryRow(ctx, "SELECT md5(string_agg(row_to_json(a)::text,',' ORDER BY a.id)) FROM quality_assays a").Scan(&before); e != nil {
			t.Fatal(e)
		}
		for i, owner := range []ReportAccess{a, a, b} {
			command := c
			command.CommandID = reportUUID(900 + i*10 + history*1000)
			command.ReportID = reportUUID(901 + i*10 + history*1000)
			if _, e := s.Execute(ctx, owner, command); e != nil {
				t.Fatal(e)
			}
			_, cid := reviewCase(t, s, command.ReportID, 1, 950+i*10+history*1000)
			if e := actReview(ctx, reviewPorts(s), cid, "RESOLVE", "CLAIM_RECORDED"); e != nil {
				t.Fatal(e)
			}
		}
		group, e := s.GroupReview(ctx, reportStation, "ETHANOL", "2026-10-13", "", account.ActiveQualityOwnersTx)
		if e != nil || group.Signal.ReviewedAccounts != 2 || group.Signal.Publication != "REVIEW_ONLY" || group.Signal.Independence != "UNPROVEN" {
			t.Fatal(group, e)
		}
		if _, e = s.Pool.Exec(ctx, "UPDATE accounts SET status='suspended' WHERE id=$1", b.Owner); e != nil {
			t.Fatal(e)
		}
		group, e = s.GroupReview(ctx, reportStation, "ETHANOL", "2026-10-13", "", account.ActiveQualityOwnersTx)
		if e != nil || group.Signal.ReviewedAccounts != 1 || group.Signal.DuplicateAccountClaims != 1 || group.Signal.Excluded != 1 {
			t.Fatal("group eligibility", group, e)
		}
		var after string
		if e = s.Pool.QueryRow(ctx, "SELECT md5(string_agg(row_to_json(a)::text,',' ORDER BY a.id)) FROM quality_assays a").Scan(&after); e != nil || after != before {
			t.Fatal("moderation changed official facts", e)
		}
		// Reset only synthetic report fixtures between the two independent histories.
		for _, owner := range []ReportAccess{a, b} {
			if _, e = s.EraseAccountBatch(ctx, owner.Owner); e != nil {
				t.Fatal(e)
			}
		}
		if _, e = s.Pool.Exec(ctx, "UPDATE accounts SET status='active' WHERE id=$1", b.Owner); e != nil {
			t.Fatal(e)
		}
	}
}

func TestDocumentFreeReviewSQLConcurrentTriage(t *testing.T) {
	s, a, _, c := reportFixture(t)
	ctx := context.Background()
	if _, e := s.Execute(ctx, a, c); e != nil {
		t.Fatal(e)
	}
	_, cid := reviewCase(t, s, c.ReportID, 1, 1200)
	p := reviewPorts(s)
	var wg sync.WaitGroup
	var accepted atomic.Int32
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			e := actReview(ctx, p, cid, "REVIEW", "TRIAGE")
			if e == nil {
				accepted.Add(1)
			} else if !errors.Is(e, domain.ErrReportConflict) && !errors.Is(e, moderationdomain.ErrBadTransition) {
				t.Error(e)
			}
		}()
	}
	wg.Wait()
	if accepted.Load() != 1 {
		t.Fatal("triage race", accepted.Load())
	}
	var n int
	_ = s.Pool.QueryRow(ctx, "SELECT count(*) FROM quality_report_review_decisions").Scan(&n)
	if n != 1 {
		t.Fatal("duplicate triage audit", n)
	}
	r, e := s.Get(ctx, a, c.ReportID)
	if e != nil || r.Revision != 1 || r.Status != "PENDING_REVIEW" {
		t.Fatal("triage altered claim", r, e)
	}
}

func TestDocumentFreeAppealSQLConcurrentConverges(t *testing.T) {
	s, a, _, c := reportFixture(t)
	ctx := context.Background()
	if _, e := s.Execute(ctx, a, c); e != nil {
		t.Fatal(e)
	}
	_, cid := reviewCase(t, s, c.ReportID, 1, 1300)
	if e := actReview(ctx, reviewPorts(s), cid, "RESOLVE", "CLAIM_RECORDED"); e != nil {
		t.Fatal(e)
	}
	var wg sync.WaitGroup
	var accepted atomic.Int32
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, e := s.Execute(ctx, a, domain.ReportCommand{CommandID: reportUUID(1400 + i), ReportID: c.ReportID, Action: "APPEAL", ExpectedRevision: 2, Text: "Synthetic concurrent appeal"})
			if e == nil {
				accepted.Add(1)
			} else if !errors.Is(e, domain.ErrReportConflict) {
				t.Error(e)
			}
		}(i)
	}
	wg.Wait()
	if accepted.Load() != 1 {
		t.Fatal("appeal fork", accepted.Load())
	}
	var n int
	_ = s.Pool.QueryRow(ctx, "SELECT count(*) FROM quality_report_appeals").Scan(&n)
	if n != 1 {
		t.Fatal("appeal text duplicated", n)
	}
	r, e := s.Get(ctx, a, c.ReportID)
	if e != nil || r.Revision != 3 || r.Status != "PENDING_REVIEW" || r.Text != c.Text {
		t.Fatal(r, e)
	}
}
