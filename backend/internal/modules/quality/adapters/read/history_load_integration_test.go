//go:build integration

package read

import (
	"context"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"strings"
	"testing"
)

type historyQueryTrace struct {
	SQL  string
	Args []any
}

func (t *historyQueryTrace) TraceQueryStart(ctx context.Context, _ *pgx.Conn, d pgx.TraceQueryStartData) context.Context {
	t.SQL = d.SQL
	t.Args = append([]any(nil), d.Args...)
	return ctx
}
func (*historyQueryTrace) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func TestHistoricalCatalogBoundedLoadAndActualQueryPlan(t *testing.T) {
	p := freshPool(t)
	seedStation(t, p)
	id := seedAssays(t, p, "a")
	ctx := context.Background()
	// Synthetic retained metadata only; never a claim of 5,000 official editions.
	if _, e := p.Exec(ctx, `INSERT INTO quality_revisions(id,edition_id,status,published_at)
 SELECT md5('synthetic-history-'||g)::uuid,r.edition_id,'superseded',now()
 FROM quality_revisions r CROSS JOIN generate_series(1,5000) g WHERE r.id=$1`, id); e != nil {
		t.Fatal(e)
	}
	if _, e := p.Exec(ctx, `INSERT INTO quality_revision_provenance(revision_id,provenance_state)
 SELECT r.id,'LEGACY_UNVERIFIED' FROM quality_revisions r WHERE NOT EXISTS(SELECT 1 FROM quality_revision_provenance p WHERE p.revision_id=r.id)`); e != nil {
		t.Fatal(e)
	}
	for _, table := range []string{"quality_revisions", "quality_editions", "quality_revision_provenance"} {
		if _, e := p.Exec(ctx, "ANALYZE "+table); e != nil {
			t.Fatal(e)
		}
	}
	trace := &historyQueryTrace{}
	cfg := p.Config()
	cfg.ConnConfig.Tracer = trace
	observed, e := pgxpool.NewWithConfig(ctx, cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer observed.Close()
	rows, next, e := NewReader(observed).Revisions(ctx, "pmqc", "pmqc-assay-v1", "", 100)
	if e != nil || len(rows) != 100 || next == "" {
		t.Fatal("unbounded catalog", len(rows), next, e)
	}
	// Explain the exact generated query and bound arguments used above, rather
	// than a simplified test-only query. Plans contain only synthetic metadata.
	query, args := trace.SQL, trace.Args
	var plan []byte
	if e = p.QueryRow(ctx, "EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON)\n"+query, args...).Scan(&plan); e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(string(plan), "Index Scan") && !strings.Contains(string(plan), "Index Only Scan") {
		t.Fatal("catalog has no indexed access", string(plan))
	}
	t.Logf("5001-revision generated catalog plan: %s", plan)
	if _, _, e = NewReader(observed).Revisions(ctx, "pmqc", "pmqc-assay-v1", "", 101); e == nil {
		t.Fatal("catalog limit bypass")
	}
}
