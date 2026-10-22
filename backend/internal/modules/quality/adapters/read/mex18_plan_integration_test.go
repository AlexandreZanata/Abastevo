//go:build integration

package read

import (
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"strings"
	"testing"
	"time"
)

type scopedPlanTrace struct {
	query string
	args  []any
}

func (t *scopedPlanTrace) TraceQueryStart(ctx context.Context, _ *pgx.Conn, d pgx.TraceQueryStartData) context.Context {
	if strings.Contains(d.SQL, "-- name: CompanySanctionPage") || strings.Contains(d.SQL, "-- name: CompanySanctionByID") {
		t.query = d.SQL
		t.args = append([]any(nil), d.Args...)
	}
	return ctx
}
func (*scopedPlanTrace) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func TestMEX18ExactScopedQueryPlansWithTwentyThousandUnrelatedRows(t *testing.T) {
	p, r, _, in, pub, station, other := companyReadFixture(t)
	ctx := context.Background()
	now := time.Now()
	rev, e := pub.PublishNativeFines(ctx, in, companyReadIDMaker())
	if e != nil {
		t.Fatal(e)
	}
	foreign := "bbbbbbbb-3333-4333-8333-000000000018"
	if _, e = p.Exec(ctx, `INSERT INTO quality_revisions(id,edition_id,status,published_at) SELECT $2,edition_id,'superseded',now() FROM quality_revisions WHERE id=$1`, rev, foreign); e != nil {
		t.Fatal(e)
	}
	if _, e = p.Exec(ctx, `INSERT INTO quality_sanction_events SELECT (jsonb_populate_record(NULL::quality_sanction_events,to_jsonb(v)||jsonb_build_object('id',md5('mex18-event-'||g)::uuid,'revision_id',$2::uuid,'native_key','observation:'||encode(sha256(convert_to('mex18-key-'||g,'UTF8')),'hex')))).* FROM quality_sanction_events v CROSS JOIN generate_series(1,20000) g WHERE v.revision_id=$1 AND v.id=(SELECT min(id::text)::uuid FROM quality_sanction_events WHERE revision_id=$1)`, rev, foreign); e != nil {
		t.Fatal(e)
	}
	if _, e = p.Exec(ctx, `INSERT INTO quality_company_association_revisions SELECT $2,policy_version,association_scope,decision_day,historical_business_window_known,20000 FROM quality_company_association_revisions WHERE revision_id=$1`, rev, foreign); e != nil {
		t.Fatal(e)
	}
	if _, e = p.Exec(ctx, `INSERT INTO quality_company_associations SELECT revision_id,native_key,'bound','exact current cnpj; historical business window unknown',$2 FROM quality_sanction_events WHERE revision_id=$1`, foreign, other); e != nil {
		t.Fatal(e)
	}
	for _, table := range []string{"quality_sanction_events", "quality_company_associations", "quality_revisions", "quality_company_association_revisions"} {
		if _, e = p.Exec(ctx, "ANALYZE "+table); e != nil {
			t.Fatal(e)
		}
	}
	trace := &scopedPlanTrace{}
	cfg := p.Config()
	cfg.ConnConfig.Tracer = trace
	observed, e := pgxpool.NewWithConfig(ctx, cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer observed.Close()
	reader := NewReader(observed).WithCompanyIdentity(r.companyProbe, r.companyGuard)
	lease, e := reader.LeaseBlock(ctx, now, "sanctions", "sanction-v1")
	if e != nil {
		t.Fatal(e)
	}
	scope, e := reader.ResolveCompanyScope(ctx, now, lease, station)
	if e != nil {
		t.Fatal(e)
	}
	page, _, e := reader.CompanySanctionsAtScope(ctx, lease, scope, "", "", "", 10)
	if e != nil || len(page) != 2 {
		t.Fatal("scoped page count", e)
	}
	for _, mode := range []string{"page", "detail"} {
		if mode == "detail" {
			if _, e = reader.CompanySanctionAtScope(ctx, lease, scope, page[0].ID); e != nil {
				t.Fatal(e)
			}
		}
		var plan []byte
		if e = p.QueryRow(ctx, "EXPLAIN (ANALYZE,BUFFERS,FORMAT JSON) "+trace.query, trace.args...).Scan(&plan); e != nil {
			t.Fatal(e)
		}
		if !strings.Contains(string(plan), "Index Scan") && !strings.Contains(string(plan), "Index Only Scan") {
			t.Fatal("no indexed scoped access")
		}
		var parsed []map[string]any
		if e := json.Unmarshal(plan, &parsed); e != nil {
			t.Fatal(e)
		}
		var check func(map[string]any)
		check = func(n map[string]any) {
			if n["Relation Name"] == "quality_company_associations" || n["Relation Name"] == "quality_sanction_events" {
				if n["Node Type"] == "Seq Scan" {
					t.Fatal("scoped query scanned unrelated event/association corpus")
				}
				if rows, ok := n["Actual Rows"].(float64); ok && rows > 4 {
					t.Fatal("scoped query touched unrelated rows")
				}
			}
			if children, ok := n["Plans"].([]any); ok {
				for _, child := range children {
					check(child.(map[string]any))
				}
			}
		}
		check(parsed[0]["Plan"].(map[string]any))
		t.Logf("%s exact scoped generated query, 20004 event rows: %s", mode, plan)
	}
}
