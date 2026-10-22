//go:build integration

package read

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestEnforcementFilteredPageUsesIndexWithUnrelatedRows(t *testing.T) {
	p := freshPool(t)
	ctx := context.Background()
	rev := "aaaaaaaa-1111-4111-8111-000000000001"
	ed := "bbbbbbbb-2222-4222-8222-000000000001"
	if _, e := p.Exec(ctx, `INSERT INTO quality_editions(id,source,stream,edition) VALUES($1,'enforcement','enforcement-v1','test')`, ed); e != nil {
		t.Fatal(e)
	}
	if _, e := p.Exec(ctx, `INSERT INTO quality_revisions(id,edition_id,status,published_at) VALUES($1,$2,'published',now())`, rev, ed); e != nil {
		t.Fatal(e)
	}
	if _, e := p.Exec(ctx, `INSERT INTO quality_enforcement_source_revisions(revision_id,source_profile,source_file,date_system,input_count,accepted_count,duplicate_count,first_document_day,last_document_day) VALUES($1,'anp-enforcement-xlsx-2019-v1','synthetic.xlsx','EXCEL_1900',1,1,0,'2026-10-04','2026-10-04')`, rev); e != nil {
		t.Fatal(e)
	}
	fingerprint := `encode(sha256(convert_to(concat_ws(chr(31),'11222333000181','DF','BRASILIA','Revenda de Combustíveis','2026-10-04','DOC-1','Inspection','Result'),'UTF8')),'hex')`
	if _, e := p.Exec(ctx, `INSERT INTO quality_enforcement_source_records(id,revision_id,native_key,subject_cnpj,subject_kind,uf,municipality,segment,document_day,document_number,procedure,result,evidence_ref,fingerprint,first_document_day,last_document_day) VALUES(md5('mex23-one')::uuid,$1,'enforcement-source:'||`+fingerprint+`,'11222333000181','company-shaped','DF','BRASILIA','Revenda de Combustíveis','2026-10-04','DOC-1','Inspection','Result','synthetic.xlsx#row:2',`+fingerprint+`,'2026-10-04','2026-10-04')`, rev); e != nil {
		t.Fatal(e)
	}
	foreign := "bbbbbbbb-3333-4333-8333-000000000019"
	if _, e := p.Exec(ctx, `INSERT INTO quality_revisions(id,edition_id,status,published_at) SELECT $2,edition_id,'superseded',now() FROM quality_revisions WHERE id=$1`, rev, foreign); e != nil {
		t.Fatal(e)
	}
	if _, e := p.Exec(ctx, `INSERT INTO quality_enforcement_source_revisions SELECT $2,source_profile,source_file,date_system,input_count,accepted_count,duplicate_count,first_document_day,last_document_day FROM quality_enforcement_source_revisions WHERE revision_id=$1`, rev, foreign); e != nil {
		t.Fatal(e)
	}
	if _, e := p.Exec(ctx, `INSERT INTO quality_enforcement_source_records SELECT md5('mex23-row-'||g)::uuid,$2,'enforcement-source:'||encode(sha256(convert_to(concat_ws(chr(31),coalesce(subject_cnpj,''),uf,municipality,segment,to_char(document_day,'YYYY-MM-DD'),'DOC-'||g,procedure,result),'UTF8')),'hex'),subject_cnpj,subject_kind,uf,municipality,segment,document_day,'DOC-'||g,procedure,result,'synthetic.xlsx#row:'||g,encode(sha256(convert_to(concat_ws(chr(31),coalesce(subject_cnpj,''),uf,municipality,segment,to_char(document_day,'YYYY-MM-DD'),'DOC-'||g,procedure,result),'UTF8')),'hex'),first_document_day,last_document_day FROM quality_enforcement_source_records v CROSS JOIN generate_series(1,20000) g WHERE v.revision_id=$1`, rev, foreign); e != nil {
		t.Fatal(e)
	}
	if _, e := p.Exec(ctx, `ANALYZE quality_enforcement_source_records`); e != nil {
		t.Fatal(e)
	}
	var plan string
	q := `EXPLAIN (FORMAT JSON) SELECT id FROM quality_enforcement_source_records WHERE revision_id=$1 AND uf=$2 ORDER BY native_key,id::text LIMIT 101`
	if e := p.QueryRow(ctx, q, rev, "DF").Scan(&plan); e != nil {
		t.Fatal(e)
	}
	var decoded []map[string]any
	if e := json.Unmarshal([]byte(plan), &decoded); e != nil {
		t.Fatal(e)
	}
	text := strings.ToUpper(plan)
	if !strings.Contains(text, "INDEX") || strings.Contains(text, "SEQ SCAN") {
		t.Fatal("facet query lost its index", plan)
	}
}
