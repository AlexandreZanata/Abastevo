//go:build integration

package adapters

import (
	"context"
	quality "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/quality"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/application"
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
		if e = q.AddResource(ctx, quality.AddResourceParams{EditionID: eid, Url: r.URL, Sha256: r.SHA256, Bytes: int64(r.Bytes)}); e != nil {
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
		if _, e = q.StageAssay(ctx, quality.StageAssayParams{ID: id, RevisionID: rid, SampleRef: a.SampleRef, AssayRef: a.AssayRef, SourceCnpj: a.SourceCNPJ, ObservedOn: date, Fuel: a.Fuel, ResultText: a.ResultText, Unit: a.Unit, Conformity: a.Conformity, EvidenceRef: a.EvidenceRef, Fingerprint: a.Fingerprint}); e != nil {
			t.Fatal(e)
		}
	}
	if e = q.SetRevisionPublished(ctx, rid); e != nil {
		t.Fatal(e)
	}
	if e = q.SetCurrentRevision(ctx, quality.SetCurrentRevisionParams{Source: in.Source, Stream: in.Stream, RevisionID: rid}); e != nil {
		t.Fatal(e)
	}
	return uuidString(rid)
}
