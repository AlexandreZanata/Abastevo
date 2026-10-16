//go:build integration

package adapters

import (
	"context"
	quality "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/quality"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/application"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"testing"
)

// Seed the previous runtime's actual tables; the new publisher must never run
// against a chain that its compiled readiness guard correctly rejects.
func seedLegacyAssayRevision(t *testing.T, p *pgxpool.Pool, in application.Input, m *idMaker) string {
	t.Helper()
	ctx := context.Background()
	q := quality.New(p)
	eid, e := nextID(m.next)
	if e != nil {
		t.Fatal(e)
	}
	rid, e := nextID(m.next)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = q.CreateEdition(ctx, quality.CreateEditionParams{ID: eid, Source: in.Source, Stream: in.Stream, Edition: in.Edition}); e != nil {
		t.Fatal(e)
	}
	for _, r := range in.Resources {
		if e = legacyAddResource(ctx, p, eid, r); e != nil {
			t.Fatal(e)
		}
	}
	if _, e = q.RecordManifest(ctx, quality.RecordManifestParams{EditionID: eid, Input: int64(in.Counts.Input), Accepted: int64(in.Counts.Accepted), Duplicates: int64(in.Counts.Duplicates), Quarantined: int64(in.Counts.Quarantined), ManifestHash: in.ManifestHash}); e != nil {
		t.Fatal(e)
	}
	if _, e = q.CreateRevision(ctx, quality.CreateRevisionParams{ID: rid, EditionID: eid}); e != nil {
		t.Fatal(e)
	}
	for _, a := range in.Assays {
		id, e := nextID(m.next)
		if e != nil {
			t.Fatal(e)
		}
		date, e := pgDate(a.ObservedOn)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = legacyStageAssay(ctx, p, quality.StageAssayParams{ID: id, RevisionID: rid, SampleRef: a.SampleRef, AssayRef: a.AssayRef, SourceCnpj: a.SourceCNPJ, ObservedOn: date, Fuel: a.Fuel, ResultText: a.ResultText, Unit: a.Unit, Conformity: a.Conformity, EvidenceRef: a.EvidenceRef, Fingerprint: a.Fingerprint}); e != nil {
			t.Fatal(e)
		}
	}
	if e = q.SetRevisionPublished(ctx, rid); e != nil {
		t.Fatal(e)
	}
	if e = legacySetCurrent(ctx, p, in, rid); e != nil {
		t.Fatal(e)
	}
	return uuidString(rid)
}

func legacyAddResource(ctx context.Context, p *pgxpool.Pool, eid pgtype.UUID, r application.Resource) error {
	_, err := p.Exec(ctx, "INSERT INTO quality_resources(edition_id,url,sha256,bytes) VALUES($1,$2,$3,$4)", eid, r.URL, r.SHA256, int64(r.Bytes))
	return err
}
func legacySetCurrent(ctx context.Context, p *pgxpool.Pool, in application.Input, rid pgtype.UUID) error {
	var hasEdition bool
	if e := p.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema='public' AND table_name='quality_current_revisions' AND column_name='edition')").Scan(&hasEdition); e != nil {
		return e
	}
	if hasEdition {
		_, e := p.Exec(ctx, "INSERT INTO quality_current_revisions(source,stream,revision_id,edition) VALUES($1,$2,$3,$4)", in.Source, in.Stream, rid, in.Edition)
		return e
	}
	_, err := p.Exec(ctx, "INSERT INTO quality_current_revisions(source,stream,revision_id) VALUES($1,$2,$3)", in.Source, in.Stream, rid)
	return err
}

func legacyStageAssay(ctx context.Context, p *pgxpool.Pool, a quality.StageAssayParams) (pgtype.UUID, error) {
	var id pgtype.UUID
	err := p.QueryRow(ctx, "INSERT INTO quality_assays(id,revision_id,sample_ref,assay_ref,source_cnpj,observed_on,fuel,result_text,unit,conformity,evidence_ref,fingerprint) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12) RETURNING id", a.ID, a.RevisionID, a.SampleRef, a.AssayRef, a.SourceCnpj, a.ObservedOn, a.Fuel, a.ResultText, a.Unit, a.Conformity, a.EvidenceRef, a.Fingerprint).Scan(&id)
	return id, err
}
