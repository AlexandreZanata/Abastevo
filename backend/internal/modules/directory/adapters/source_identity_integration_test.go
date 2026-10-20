//go:build integration

package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	dbmigrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
	"io/fs"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/application"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const sourceCNPJ = "11222333000181"

func sourceBindingFixture(t *testing.T) (*Repository, *SourceIdentityReader, application.SourceBinding) {
	t.Helper()
	repo := freshRepo(t)
	station, e := repo.ResolveCNPJ(context.Background(), sourceCNPJ, "Synthetic source station", nil)
	if e != nil {
		t.Fatal(e)
	}
	return repo, NewSourceIdentityReader(repo.pool), application.SourceBinding{CNPJ: sourceCNPJ, StationID: station.ID}
}
func TestSourceIdentityLookupAndHistoricalAmbiguity(t *testing.T) {
	repo, r, b := sourceBindingFixture(t)
	ctx := context.Background()
	rows, e := r.Exact(ctx, sourceCNPJ)
	if e != nil || len(rows) != 1 || !rows[0].Current || !rows[0].StationActive || rows[0].StationID != b.StationID {
		t.Fatal(rows, e)
	}
	if _, e = r.Exact(ctx, "invalid%identifier"); !errors.Is(e, ErrSourceIdentityUnavailable) {
		t.Fatal("invalid identifier accepted", e)
	}
	if e = repo.RetireIdentifier(ctx, "CNPJ", sourceCNPJ); e != nil {
		t.Fatal(e)
	}
	rows, e = r.Exact(ctx, sourceCNPJ)
	if e != nil || len(rows) != 1 || rows[0].Current {
		t.Fatal("retired history disappeared", rows, e)
	}
	next, e := repo.ResolveCNPJ(ctx, sourceCNPJ, "Synthetic successor", nil)
	if e != nil || next.ID == b.StationID {
		t.Fatal(next, e)
	}
	rows, e = r.Exact(ctx, sourceCNPJ)
	if e != nil || len(rows) != 2 {
		t.Fatal("historical ambiguity lost", rows, e)
	}
	tx, e := repo.pool.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(ctx)
	if e = r.Guard(ctx, tx, []application.SourceBinding{{CNPJ: sourceCNPJ, StationID: next.ID}}); !errors.Is(e, ErrSourceIdentityChanged) {
		t.Fatal("successor auto-bound", e)
	}
	unknown := "04218406000104"
	missing, e := r.Exact(ctx, unknown)
	if e != nil || len(missing) != 0 {
		t.Fatal(missing, e)
	}
	branch, e := r.OtherBranch(ctx, "11222333000262")
	if e != nil || !branch {
		t.Fatal("branch evidence missing", branch, e)
	}
}
func TestSourceEvidenceBatchMirrorsExactLookup(t *testing.T) {
	repo, r, b := sourceBindingFixture(t)
	ctx := context.Background()
	if out, e := r.Evidence(ctx, nil); e != nil || len(out) != 0 {
		t.Fatal("empty batch", out, e)
	}
	if _, e := r.Evidence(ctx, []string{sourceCNPJ, "invalid%identifier"}); !errors.Is(e, ErrSourceIdentityUnavailable) {
		t.Fatal("invalid batch identifier accepted", e)
	}
	rows, e := r.Evidence(ctx, []string{"04218406000104", sourceCNPJ})
	if e != nil || len(rows) != 1 || rows[0].CNPJ != sourceCNPJ || !rows[0].Current || !rows[0].StationActive || rows[0].StationID != b.StationID {
		t.Fatal(rows, e)
	}
	if e = repo.RetireIdentifier(ctx, "CNPJ", sourceCNPJ); e != nil {
		t.Fatal(e)
	}
	successor, e := repo.ResolveCNPJ(ctx, sourceCNPJ, "Synthetic successor", nil)
	if e != nil {
		t.Fatal(e)
	}
	rows, e = r.Evidence(ctx, []string{sourceCNPJ})
	if e != nil || len(rows) != 2 {
		t.Fatal("historical ambiguity lost", rows, e)
	}
	byStation := map[string]application.SourceEvidence{}
	for _, row := range rows {
		byStation[row.StationID] = row
	}
	if byStation[successor.ID].Current != true || byStation[successor.ID].StationActive != true || byStation[b.StationID].Current {
		t.Fatal("batch evidence diverged from exact semantics", rows)
	}
}

func TestSourceBindingGuardRejectsRetirementAndStatus(t *testing.T) {
	for _, change := range []string{"retire", "status"} {
		t.Run(change, func(t *testing.T) {
			repo, r, b := sourceBindingFixture(t)
			ctx := context.Background()
			if change == "retire" {
				if e := repo.RetireIdentifier(ctx, "CNPJ", sourceCNPJ); e != nil {
					t.Fatal(e)
				}
			} else {
				if _, e := repo.pool.Exec(ctx, "UPDATE directory_stations SET status='suspended' WHERE id=$1", b.StationID); e != nil {
					t.Fatal(e)
				}
			}
			tx, e := repo.pool.Begin(ctx)
			if e != nil {
				t.Fatal(e)
			}
			defer tx.Rollback(ctx)
			if e = r.Guard(ctx, tx, []application.SourceBinding{b}); !errors.Is(e, ErrSourceIdentityChanged) {
				t.Fatal("stale source binding accepted", e)
			}
		})
	}
}
func TestSourceBindingGuardFencesConcurrentRetirementAndStatus(t *testing.T) {
	for _, change := range []string{"retire", "status"} {
		t.Run(change, func(t *testing.T) {
			repo, r, b := sourceBindingFixture(t)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			tx, e := repo.pool.Begin(ctx)
			if e != nil {
				t.Fatal(e)
			}
			defer tx.Rollback(context.Background())
			if e = r.Guard(ctx, tx, []application.SourceBinding{b, b}); e != nil {
				t.Fatal(e)
			}
			cfg := repo.pool.Config()
			cfg.ConnConfig.RuntimeParams["application_name"] = "directory-source-guard-writer"
			writers, e := pgxpool.NewWithConfig(ctx, cfg)
			if e != nil {
				t.Fatal(e)
			}
			defer writers.Close()
			done := make(chan error, 1)
			go func() {
				if change == "retire" {
					_, e := writers.Exec(ctx, "UPDATE directory_identifiers SET valid_to=now() WHERE normalized_value=$1 AND valid_to IS NULL", sourceCNPJ)
					done <- e
				} else {
					_, e := writers.Exec(ctx, "UPDATE directory_stations SET status='suspended' WHERE id=$1", b.StationID)
					done <- e
				}
			}()
			blocked := 0
			for ctx.Err() == nil && blocked == 0 {
				if e = repo.pool.QueryRow(ctx, "SELECT count(*) FROM pg_stat_activity WHERE datname=current_database() AND application_name='directory-source-guard-writer' AND wait_event_type='Lock'").Scan(&blocked); e != nil {
					break
				}
			}
			if blocked != 1 {
				t.Fatal("writer did not reach real row-lock barrier", blocked, e)
			}
			if e = tx.Commit(ctx); e != nil {
				t.Fatal(e)
			}
			if e = <-done; e != nil {
				t.Fatal(e)
			}
			stale, e := repo.pool.Begin(ctx)
			if e != nil {
				t.Fatal(e)
			}
			defer stale.Rollback(context.Background())
			if e = r.Guard(ctx, stale, []application.SourceBinding{b}); !errors.Is(e, ErrSourceIdentityChanged) {
				t.Fatal("post-transition replay bypassed guard", e)
			}
		})
	}
}

type sourceQueryTrace struct {
	sql  string
	args []any
}

func (t *sourceQueryTrace) TraceQueryStart(ctx context.Context, _ *pgx.Conn, d pgx.TraceQueryStartData) context.Context {
	t.sql = d.SQL
	t.args = append([]any(nil), d.Args...)
	return ctx
}
func (*sourceQueryTrace) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}
func sourceHasSequentialIdentifierScan(value any) bool {
	switch v := value.(type) {
	case map[string]any:
		if v["Node Type"] == "Seq Scan" && v["Relation Name"] == "directory_identifiers" {
			return true
		}
		for _, child := range v {
			if sourceHasSequentialIdentifierScan(child) {
				return true
			}
		}
	case []any:
		for _, child := range v {
			if sourceHasSequentialIdentifierScan(child) {
				return true
			}
		}
	}
	return false
}
func TestSourceIdentityHistoricalIndexPlan(t *testing.T) {
	ctx := context.Background()
	prior := fstest.MapFS{}
	entries, e := fs.ReadDir(dbmigrations.Files, ".")
	if e != nil {
		t.Fatal(e)
	}
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() >= "000102" {
			continue
		}
		b, e := fs.ReadFile(dbmigrations.Files, entry.Name())
		if e != nil {
			t.Fatal(e)
		}
		prior[entry.Name()] = &fstest.MapFile{Data: b}
	}
	repo := freshRepo(t, prior)
	if _, e := repo.ResolveCNPJ(ctx, sourceCNPJ, "Synthetic indexed source station", nil); e != nil {
		t.Fatal(e)
	}
	if _, e := repo.pool.Exec(ctx, `INSERT INTO directory_identifiers(id,station_id,kind,normalized_value,valid_to)
 SELECT md5('source-index-retained-'||g)::uuid,s.id,'CNPJ',lpad(g::text,14,'0'),now()
 FROM generate_series(1,20000) g CROSS JOIN directory_stations s`); e != nil {
		t.Fatal(e)
	}
	if _, e := repo.pool.Exec(ctx, "ANALYZE directory_identifiers"); e != nil {
		t.Fatal(e)
	}
	trace := &sourceQueryTrace{}
	cfg := repo.pool.Config()
	cfg.ConnConfig.Tracer = trace
	p, e := pgxpool.NewWithConfig(ctx, cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer p.Close()
	rows, e := NewSourceIdentityReader(p).Exact(ctx, sourceCNPJ)
	if e != nil || len(rows) != 1 {
		t.Fatal(rows, e)
	}
	query, args := trace.sql, trace.args
	var raw []byte
	if e = repo.pool.QueryRow(ctx, "EXPLAIN (ANALYZE,BUFFERS,FORMAT JSON)\n"+query, args...).Scan(&raw); e != nil {
		t.Fatal(e)
	}
	var plan any
	if e = json.Unmarshal(raw, &plan); e != nil {
		t.Fatal(e)
	}
	if !sourceHasSequentialIdentifierScan(plan) {
		t.Fatal("baseline scan not reproduced", string(raw))
	}
	t.Log("Before migration102: generated lookup scans 20,001 retained rows")
	applied, e := migrate.Apply(ctx, repo.pool.Config().ConnConfig.ConnString(), dbmigrations.Files)
	if e != nil || (len(applied) == 0 || applied[0] != "000102") {
		t.Fatal("retained history index upgrade", applied, e)
	}
	applied, e = migrate.Apply(ctx, repo.pool.Config().ConnConfig.ConnString(), dbmigrations.Files)
	if e != nil || len(applied) != 0 {
		t.Fatal("index upgrade replay", applied, e)
	}
	if e = repo.pool.QueryRow(ctx, "EXPLAIN (ANALYZE,BUFFERS,FORMAT JSON)\n"+query, args...).Scan(&raw); e != nil {
		t.Fatal(e)
	}
	if e = json.Unmarshal(raw, &plan); e != nil {
		t.Fatal(e)
	}
	if sourceHasSequentialIdentifierScan(plan) {
		t.Fatal("historical identifier lookup scans retained universe", string(raw))
	}
	if !strings.Contains(string(raw), "directory_identifiers_source_history_idx") {
		t.Fatal("owned historical index absent", string(raw))
	}
	t.Logf("20001-row exact generated source identity plan: %s", raw)
	if _, e = NewSourceIdentityReader(p).OtherBranch(ctx, "11222333000262"); e != nil {
		t.Fatal(e)
	}
	query, args = trace.sql, trace.args
	if e = repo.pool.QueryRow(ctx, "EXPLAIN (ANALYZE,BUFFERS,FORMAT JSON)\n"+query, args...).Scan(&raw); e != nil {
		t.Fatal(e)
	}
	if e = json.Unmarshal(raw, &plan); e != nil {
		t.Fatal(e)
	}
	if sourceHasSequentialIdentifierScan(plan) || !strings.Contains(string(raw), "directory_identifiers_source_history_idx") {
		t.Fatal("branch lookup scans retained history", string(raw))
	}
	t.Logf("20001-row generated branch plan: %s", raw)
}

func TestSourceIdentityLookupOutageIsRedacted(t *testing.T) {
	repo, r, _ := sourceBindingFixture(t)
	repo.pool.Close()
	if _, e := r.Exact(context.Background(), sourceCNPJ); !errors.Is(e, ErrSourceIdentityUnavailable) {
		t.Fatal("closed lookup not contained", e)
	}
	if _, e := r.OtherBranch(context.Background(), sourceCNPJ); !errors.Is(e, ErrSourceIdentityUnavailable) {
		t.Fatal("closed branch lookup not contained", e)
	}
}

func syntheticSourceCNPJ(n int) string {
	raw := fmt.Sprintf("%012d", 100000000000+n)
	for _, weights := range [][]int{{5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2}, {6, 5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2}} {
		sum := 0
		for i, w := range weights {
			sum += int(raw[i]-'0') * w
		}
		digit := 11 - sum%11
		if digit >= 10 {
			digit = 0
		}
		raw += strconv.Itoa(digit)
	}
	return raw
}
func TestSourceBindingGuardSpansBoundedChunks(t *testing.T) {
	repo, r, b := sourceBindingFixture(t)
	ctx := context.Background()
	bindings := make([]application.SourceBinding, 0, 514)
	for i := 0; i < 514; i++ {
		cnpj := syntheticSourceCNPJ(i)
		if _, e := repo.pool.Exec(ctx, "INSERT INTO directory_identifiers(id,station_id,kind,normalized_value) VALUES(md5($1)::uuid,$2,'CNPJ',$1)", cnpj, b.StationID); e != nil {
			t.Fatal(e)
		}
		bindings = append(bindings, application.SourceBinding{CNPJ: cnpj, StationID: b.StationID})
	}
	tx, e := repo.pool.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(ctx)
	if e = r.Guard(ctx, tx, bindings); e != nil {
		t.Fatal("multi-chunk guard", e)
	}
	if e = tx.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	if e = repo.RetireIdentifier(ctx, "CNPJ", bindings[len(bindings)-1].CNPJ); e != nil {
		t.Fatal(e)
	}
	stale, e := repo.pool.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer stale.Rollback(ctx)
	if e = r.Guard(ctx, stale, bindings); !errors.Is(e, ErrSourceIdentityChanged) {
		t.Fatal("later-chunk mismatch accepted", e)
	}
}

func TestSourceOtherBranchesBatchMatchesExactAndPreservesWholeIdentifiers(t *testing.T) {
	repo, r, _ := sourceBindingFixture(t)
	ctx := context.Background()
	if _, e := repo.ResolveCNPJ(ctx, "04218406000104", "Synthetic zero-prefixed company", nil); e != nil {
		t.Fatal(e)
	}
	if _, e := repo.ResolveCNPJ(ctx, "12ABC34501DE35", "Synthetic alphanumeric company", nil); e != nil {
		t.Fatal(e)
	}
	branch := func(base string) string {
		for n := 0; n < 100; n++ {
			value := fmt.Sprintf("%s%02d", base, n)
			if normalizedSourceCNPJ(value) {
				return value
			}
		}
		t.Fatal("synthetic branch checksum failed")
		return ""
	}
	zeroBranch, alphaBranch := branch("042184060002"), branch("12ABC34501DF")
	input := []string{"11222333000262", sourceCNPJ, "04218406000104", "12ABC34501DE35", "11222333000262", zeroBranch, alphaBranch}
	result, e := r.OtherBranches(ctx, input)
	if e != nil {
		t.Fatal(e)
	}
	expected := []string{}
	seen := map[string]bool{}
	for _, cnpj := range input {
		found, e := r.OtherBranch(ctx, cnpj)
		if e != nil {
			t.Fatal(e)
		}
		if found && !seen[cnpj] {
			expected = append(expected, cnpj)
			seen[cnpj] = true
		}
	}
	slices.Sort(expected)
	if !slices.Equal(result, expected) || len(result) != 3 {
		t.Fatal("batch foreign-branch semantics changed", result, e)
	}
	if out, e := r.OtherBranches(ctx, nil); e != nil || len(out) != 0 {
		t.Fatal("empty foreign-branch query", e)
	}
	// Invalid input must be rejected before touching even a deliberately absent pool.
	if _, e := NewSourceIdentityReader(nil).OtherBranches(ctx, []string{sourceCNPJ, "invalid%identifier"}); !errors.Is(e, ErrSourceIdentityUnavailable) {
		t.Fatal("bad batch reached storage", e)
	}
	repo.pool.Close()
	if _, e := r.OtherBranches(ctx, []string{sourceCNPJ}); !errors.Is(e, ErrSourceIdentityUnavailable) {
		t.Fatal("closed branch batch leaked/accepted", e)
	}
}

type sourceBranchBatchTrace struct {
	calls, maxIDs int
	firstSQL      string
	firstArgs     []any
}

func (t *sourceBranchBatchTrace) TraceQueryStart(ctx context.Context, _ *pgx.Conn, d pgx.TraceQueryStartData) context.Context {
	if strings.Contains(d.SQL, "-- name: SourceOtherBranchesBatch") {
		t.calls++
		if len(d.Args) == 1 {
			if ids, ok := d.Args[0].([]string); ok {
				t.maxIDs = max(t.maxIDs, len(ids))
			}
		}
		if t.calls == 1 {
			t.firstSQL = d.SQL
			t.firstArgs = append([]any(nil), d.Args...)
		}
	}
	return ctx
}
func (*sourceBranchBatchTrace) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func TestSourceOtherBranchesBatchChunksAndUsesRetainedIndex(t *testing.T) {
	repo, _, b := sourceBindingFixture(t)
	ctx := context.Background()
	if _, e := repo.pool.Exec(ctx, `INSERT INTO directory_identifiers(id,station_id,kind,normalized_value,valid_to)
 SELECT md5('company-branch-retained-'||g)::uuid,$1,'CNPJ',lpad(g::text,14,'0'),now() FROM generate_series(1,20000) g`, b.StationID); e != nil {
		t.Fatal(e)
	}
	if _, e := repo.pool.Exec(ctx, `ANALYZE directory_identifiers`); e != nil {
		t.Fatal(e)
	}
	trace := &sourceBranchBatchTrace{}
	cfg := repo.pool.Config()
	cfg.ConnConfig.Tracer = trace
	p, e := pgxpool.NewWithConfig(ctx, cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer p.Close()
	input := make([]string, 0, 516)
	for i := 0; i < 513; i++ {
		input = append(input, syntheticSourceCNPJ(i))
	}
	input = append(input, "11222333000262", input[0], input[1])
	rows, e := NewSourceIdentityReader(p).OtherBranches(ctx, input)
	if e != nil || len(rows) != 1 || rows[0] != "11222333000262" || trace.calls != 2 || trace.maxIDs != 512 {
		t.Fatal("branch batch budget or denominator changed", trace.calls, trace.maxIDs, e)
	}
	var raw []byte
	if e := repo.pool.QueryRow(ctx, "EXPLAIN (ANALYZE,BUFFERS,FORMAT JSON)\n"+trace.firstSQL, trace.firstArgs...).Scan(&raw); e != nil {
		t.Fatal(e)
	}
	var plan any
	if e := json.Unmarshal(raw, &plan); e != nil {
		t.Fatal(e)
	}
	if sourceHasSequentialIdentifierScan(plan) || !strings.Contains(string(raw), "directory_identifiers_source_history_idx") {
		t.Fatal("correlated branch batch scans retained universe")
	}
	t.Log("PASS:514 distinct full CNPJs use512+2 queries;512-prefix EXPLAIN uses existing historical index across20001 retained identities")
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, e := NewSourceIdentityReader(nil).OtherBranches(cancelled, []string{sourceCNPJ}); !errors.Is(e, ErrSourceIdentityUnavailable) {
		t.Fatal("cancelled batch accessed storage", e)
	}
}
