//go:build integration

package adapters

import (
	"context"
	"errors"
	"fmt"
	accountadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/adapters"
	accountdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
	identityadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const reportStation = "11111111-aaaa-4111-8111-000000000001"

func reportUUID(n int) string { return fmt.Sprintf("22222222-aaaa-4222-8222-%012d", n) }
func seedReportOwner(t *testing.T, p *pgxpool.Pool, n int, now time.Time) ReportAccess {
	t.Helper()
	ctx := context.Background()
	owner, family, contributor, key := reportUUID(n), reportUUID(n+10), reportUUID(n+20), reportUUID(n+30)
	exec := func(q string, args ...any) {
		t.Helper()
		if _, e := p.Exec(ctx, q, args...); e != nil {
			t.Fatal(e)
		}
	}
	exec("INSERT INTO accounts(id,alias,status) VALUES($1,$2,'active')", owner, fmt.Sprintf("report-owner-%d", n))
	exec("INSERT INTO identity_contributors(id,status) VALUES($1,'active')", contributor)
	exec("INSERT INTO identity_keys(id,contributor_id,algorithm,public_jwk,fingerprint) VALUES($1,$2,'ecdsa-p256-sha512','{}',$3)", key, contributor, fmt.Sprintf("report-key-%d", n))
	exec("INSERT INTO account_session_families(id,account_id,refresh_salt,refresh_hash,access_salt,access_hash,access_expires,issued_at) VALUES($1,$2,'fixture','fixture','fixture',$3,$4,$5)", family, owner, accountdomain.SHA256Hasher{}.Hash("fixture", "fixture-token"), now.Add(time.Hour), now)
	exec("INSERT INTO account_contributor_bindings(account_id,contributor_id,key_fingerprint) VALUES($1,$2,'fixture')", owner, contributor)
	return ReportAccess{Owner: owner, Guard: func(ctx context.Context, tx pgx.Tx) error {
		if e := accountadapters.NewPGStore(p).RequireQualitySessionTx(ctx, tx, owner, family, "fixture-token", contributor, accountdomain.SHA256Hasher{}, now.Unix()); e != nil {
			return domain.ErrReportUnauthorized
		}
		if e := identityadapters.RequireQualitySigningKeyTx(ctx, tx, contributor, key); e != nil {
			return domain.ErrReportUnauthorized
		}
		return nil
	}}
}
func reportFixture(t *testing.T) (*ReportStore, ReportAccess, ReportAccess, domain.ReportCommand) {
	t.Helper()
	p := freshPool(t)
	now := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	if _, e := p.Exec(context.Background(), "INSERT INTO directory_stations(id,display_name) VALUES($1,'Synthetic report station')", reportStation); e != nil {
		t.Fatal(e)
	}
	s := &ReportStore{Pool: p, Now: func() time.Time { return now }, Station: func(_ context.Context, id string) (bool, error) { return id == reportStation, nil }}
	a := seedReportOwner(t, p, 1, now)
	b := seedReportOwner(t, p, 2, now)
	c := domain.ReportCommand{CommandID: reportUUID(101), ReportID: reportUUID(102), Action: "SUBMIT", Kind: "EXPERIENCE", StationID: reportStation, Fuel: "ETHANOL", OccurredOn: "2026-10-13", Symptom: "OTHER", Text: "Private synthetic experience."}
	return s, a, b, c
}

func TestReportSQLReplayOwnershipEditAndWithdrawal(t *testing.T) {
	s, a, b, c := reportFixture(t)
	ctx := context.Background()
	one, e := s.Execute(ctx, a, c)
	if e != nil {
		t.Fatal(e)
	}
	replay, e := s.Execute(ctx, a, c)
	if e != nil || one != replay {
		t.Fatal("replay", replay, e)
	}
	changed := c
	changed.Text = "Changed payload"
	if _, e = s.Execute(ctx, a, changed); !errors.Is(e, domain.ErrReportConflict) {
		t.Fatal("content reuse", e)
	}
	if _, e = s.Get(ctx, b, c.ReportID); !errors.Is(e, domain.ErrReportNotFound) {
		t.Fatal("IDOR", e)
	}
	changed = c
	changed.CommandID = reportUUID(103)
	changed.Action = "EDIT"
	changed.ExpectedRevision = 1
	if _, e = s.Execute(ctx, b, changed); !errors.Is(e, domain.ErrReportNotFound) {
		t.Fatal("foreign edit", e)
	}
	changed.Text = "Edited private claim"
	two, e := s.Execute(ctx, a, changed)
	if e != nil || two.Revision != 2 {
		t.Fatal(two, e)
	}
	changed.CommandID = reportUUID(104)
	if _, e = s.Execute(ctx, a, changed); !errors.Is(e, domain.ErrReportConflict) {
		t.Fatal("stale edit", e)
	}
	withdraw := domain.ReportCommand{CommandID: reportUUID(105), ReportID: c.ReportID, Action: "WITHDRAW", ExpectedRevision: 2}
	out, e := s.Execute(ctx, a, withdraw)
	if e != nil || out.Status != "WITHDRAWN" {
		t.Fatal(out, e)
	}
	got, e := s.Get(ctx, a, c.ReportID)
	if e != nil || got.Text != "" || got.EvidenceLevel != "USER_CLAIM" {
		t.Fatal("withdraw privacy", got, e)
	}
	var jobs, audit int
	_ = s.Pool.QueryRow(ctx, "SELECT count(*) FROM job_queue WHERE kind='quality-report-review'").Scan(&jobs)
	_ = s.Pool.QueryRow(ctx, "SELECT count(*) FROM quality_report_audit").Scan(&audit)
	if jobs != 2 || audit != 3 {
		t.Fatal("atomic intents/audit", jobs, audit)
	}
}

func TestReportSQLConcurrentReplayAndEdit(t *testing.T) {
	s, a, _, c := reportFixture(t)
	ctx := context.Background()
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, e := s.Execute(ctx, a, c); e != nil {
				t.Error(e)
			}
		}()
	}
	wg.Wait()
	var wins atomic.Int32
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			d := c
			d.Action = "EDIT"
			d.ExpectedRevision = 1
			d.CommandID = reportUUID(200 + i)
			d.Text = fmt.Sprint("Edit ", i)
			_, e := s.Execute(ctx, a, d)
			if e == nil {
				wins.Add(1)
			} else if !errors.Is(e, domain.ErrReportConflict) {
				t.Error(e)
			}
		}(i)
	}
	wg.Wait()
	if wins.Load() != 1 {
		t.Fatal("optimistic winners", wins.Load())
	}
}

func TestReportSQLFailedIntentRollsBack(t *testing.T) {
	s, a, _, c := reportFixture(t)
	ctx := context.Background()
	s.Enqueue = func(context.Context, pgx.Tx, []byte, string) error { return errors.New("synthetic queue failure") }
	if _, e := s.Execute(ctx, a, c); e == nil {
		t.Fatal("queue failure accepted")
	}
	for _, table := range []string{"quality_reports", "quality_report_commands", "quality_report_audit"} {
		var n int
		if e := s.Pool.QueryRow(ctx, "SELECT count(*) FROM "+pgx.Identifier{table}.Sanitize()).Scan(&n); e != nil || n != 0 {
			t.Fatal("rollback", table, n, e)
		}
	}
	s.Enqueue = nil
	if _, e := s.Execute(ctx, a, c); e != nil {
		t.Fatal("retry", e)
	}
}

func TestReportSQLForgedAndStaleOfficialTarget(t *testing.T) {
	s, a, _, c := reportFixture(t)
	ctx := context.Background()
	input := sampleInput("b")
	input.Links[0].StationID = reportStation
	input.Assays[0].Fuel = "ETHANOL"
	input.Assays[1].Fuel = "ETHANOL"
	m := &idMaker{}
	rid, e := NewPublisher(s.Pool).Publish(ctx, input, m.next)
	if e != nil {
		t.Fatal(e)
	}
	c.Kind = "LINK_REVIEW"
	c.RevisionID = rid
	c.SampleRef = "S-1"
	wrong := c
	wrong.Fuel = "DIESEL_S10"
	if _, e = s.Execute(ctx, a, wrong); !errors.Is(e, domain.ErrReportConflict) {
		t.Fatal("fuel forgery", e)
	}
	wrong = c
	wrong.SampleRef = "forged"
	if _, e = s.Execute(ctx, a, wrong); !errors.Is(e, domain.ErrReportConflict) {
		t.Fatal("sample forgery", e)
	}
	if _, e = s.Execute(ctx, a, c); e != nil {
		t.Fatal("valid target", e)
	}
	input.ManifestHash = fmt.Sprintf("%064d", 5)
	input.Resources[0].SHA256 = fmt.Sprintf("%064d", 6)
	if _, e = NewPublisher(s.Pool).Publish(ctx, input, m.next); e != nil {
		t.Fatal(e)
	}
	c.CommandID = reportUUID(107)
	c.ReportID = reportUUID(108)
	if _, e = s.Execute(ctx, a, c); !errors.Is(e, domain.ErrReportConflict) {
		t.Fatal("stale target", e)
	}
}

func TestReportSQLAuthorizationRemainsCurrent(t *testing.T) {
	s, a, _, c := reportFixture(t)
	ctx := context.Background()
	if _, e := s.Execute(ctx, ReportAccess{Owner: a.Owner}, c); !errors.Is(e, domain.ErrReportUnauthorized) {
		t.Fatal("missing guard", e)
	}
	mutations := []struct {
		name, q string
		arg     string
		restore string
	}{
		{"suspended", "UPDATE accounts SET status='suspended' WHERE id=$1", a.Owner, "UPDATE accounts SET status='active' WHERE id=$1"},
		{"revoked family", "UPDATE account_session_families SET revoked_at=now() WHERE id=$1", reportUUID(11), "UPDATE account_session_families SET revoked_at=NULL WHERE id=$1"},
		{"rotated token", "UPDATE account_session_families SET access_hash='changed' WHERE id=$1", reportUUID(11), ""},
		{"revoked key", "UPDATE identity_keys SET revoked_at=now() WHERE id=$1", reportUUID(31), "UPDATE identity_keys SET revoked_at=NULL WHERE id=$1"},
	}
	for _, tc := range mutations {
		t.Run(tc.name, func(t *testing.T) {
			if _, e := s.Pool.Exec(ctx, tc.q, tc.arg); e != nil {
				t.Fatal(e)
			}
			if _, e := s.Execute(ctx, a, c); !errors.Is(e, domain.ErrReportUnauthorized) {
				t.Fatal("authorization", e)
			}
			if tc.restore != "" {
				if _, e := s.Pool.Exec(ctx, tc.restore, tc.arg); e != nil {
					t.Fatal(e)
				}
			} else {
				_, e := s.Pool.Exec(ctx, "UPDATE account_session_families SET access_hash=$1 WHERE id=$2", accountdomain.SHA256Hasher{}.Hash("fixture", "fixture-token"), tc.arg)
				if e != nil {
					t.Fatal(e)
				}
			}
		})
	}
	if _, e := s.Pool.Exec(ctx, "DELETE FROM account_contributor_bindings WHERE account_id=$1", a.Owner); e != nil {
		t.Fatal(e)
	}
	if _, e := s.Execute(ctx, a, c); !errors.Is(e, domain.ErrReportUnauthorized) {
		t.Fatal("unbound", e)
	}
}

func TestReportSQLRevocationCannotCrossGuardedWrite(t *testing.T) {
	s, a, _, c := reportFixture(t)
	ctx := context.Background()
	entered, release := make(chan struct{}), make(chan struct{})
	guard := a.Guard
	a.Guard = func(ctx context.Context, tx pgx.Tx) error {
		if e := guard(ctx, tx); e != nil {
			return e
		}
		close(entered)
		<-release
		return nil
	}
	done := make(chan error, 1)
	go func() { _, e := s.Execute(ctx, a, c); done <- e }()
	<-entered
	lockCtx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
	_, e := s.Pool.Exec(lockCtx, "UPDATE accounts SET status='deleted' WHERE id=$1", a.Owner)
	cancel()
	close(release)
	if e == nil {
		t.Fatal("deletion crossed guarded write")
	}
	if e = <-done; e != nil {
		t.Fatal(e)
	}
	if _, e = s.Pool.Exec(ctx, "UPDATE accounts SET status='deleted' WHERE id=$1", a.Owner); e != nil {
		t.Fatal(e)
	}
	a.Guard = guard
	if _, e = s.Execute(ctx, a, c); !errors.Is(e, domain.ErrReportUnauthorized) {
		t.Fatal("replay after deletion", e)
	}
	if done, e := s.EraseAccountBatch(ctx, a.Owner); e != nil || !done {
		t.Fatal("erase", done, e)
	}
	var n int
	_ = s.Pool.QueryRow(ctx, "SELECT count(*) FROM quality_reports").Scan(&n)
	if n != 0 {
		t.Fatal("private reports survived", n)
	}
	if done, e := s.EraseAccountBatch(ctx, a.Owner); e != nil || !done {
		t.Fatal("erase replay", done, e)
	}
}
