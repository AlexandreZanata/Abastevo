//go:build integration

package adapters

import (
	"context"
	"io/fs"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	dbmigrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	quality "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/quality"
	qualityread "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters/read"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
)

func TestSourceClockUpgradeReplayAndFailedCorrection(t *testing.T) {
	ctx := context.Background()
	prior := fstest.MapFS{}
	entries, e := fs.ReadDir(dbmigrations.Files, ".")
	if e != nil {
		t.Fatal(e)
	}
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() >= "000101" {
			continue
		}
		b, e := fs.ReadFile(dbmigrations.Files, entry.Name())
		if e != nil {
			t.Fatal(e)
		}
		prior[entry.Name()] = &fstest.MapFile{Data: b}
	}
	p := freshPoolWithMigrations(t, prior)
	in := sampleInput("b")
	m := &idMaker{}
	previous := seedLegacyAssayRevision(t, p, in, m)
	req := editionRequest{Source: in.Source, Stream: in.Stream, Edition: in.Edition, ManifestHash: in.ManifestHash, Counts: in.Counts, Resources: in.Resources, Links: in.Links}
	if _, e = p.Exec(ctx, "INSERT INTO quality_revision_provenance(revision_id,provenance_state,input,accepted,duplicates,quarantined,manifest_hash,publication_key) VALUES($1,'VERIFIED',$2,$3,$4,$5,$6,$7)", previous, in.Counts.Input, in.Counts.Accepted, in.Counts.Duplicates, in.Counts.Quarantined, in.ManifestHash, req.publicationKey()); e != nil {
		t.Fatal(e)
	}
	applied, e := migrate.Apply(ctx, p.Config().ConnConfig.ConnString(), dbmigrations.Files)
	if e != nil || !slices.Equal(applied, []string{"000101", "000102", "000103", "000104"}) {
		t.Fatal("clock upgrade", applied, e)
	}
	replay, e := NewPublisher(p).Publish(ctx, in, m.next)
	if e != nil || replay != previous {
		t.Fatal("date-only replay forked", replay, e)
	}
	var n int
	if e = p.QueryRow(ctx, "SELECT count(*) FROM quality_assays WHERE source_observed_at IS NULL").Scan(&n); e != nil || n != 2 {
		t.Fatal("legacy clocks fabricated", n, e)
	}
	in.ManifestHash = strings.Repeat("c", 64)
	in.Assays[0].SourceObservedAt = "2026-09-04 12:34:56"
	current, e := NewPublisher(p).Publish(ctx, in, m.next)
	if e != nil {
		t.Fatal(e)
	}
	detail, e := qualityread.NewReader(p).Sample(ctx, time.Now(), in.Source, in.Stream, in.Assays[0].SampleRef)
	if e != nil || len(detail.Assays) != 2 {
		t.Fatal("clock read", detail, e)
	}
	found := false
	for _, a := range detail.Assays {
		if a.SourceObservedAt == in.Assays[0].SourceObservedAt && a.ObservedOn == "2026-09-04" {
			found = true
		}
	}
	if !found {
		t.Fatal("source clock not retained")
	}
	in.ManifestHash = strings.Repeat("d", 64)
	in.Assays[0].SourceObservedAt = "2026-09-04 12:34:57"
	in.Assays[1].ResultText = strings.Repeat("x", 1025)
	if _, e = NewPublisher(p).Publish(ctx, in, m.next); e == nil {
		t.Fatal("failed correction accepted")
	}
	q := quality.New(p)
	rid, e := q.GetCurrentRevision(ctx, quality.GetCurrentRevisionParams{Source: in.Source, Stream: in.Stream})
	if e != nil || uuidString(rid) != current {
		t.Fatal("failed correction switched current", e)
	}
	rows, e := q.ListRevisionAssays(ctx, rid)
	if e != nil || len(rows) != 2 || rows[0].SourceObservedAt.String != "2026-09-04 12:34:56" {
		t.Fatal("frozen source clock changed", rows, e)
	}
	if _, e = p.Exec(ctx, "UPDATE quality_assays SET source_observed_at='2026-09-05 12:34:56' WHERE revision_id=$1", current); e == nil {
		t.Fatal("SQL accepted date/clock mismatch")
	}
	applied, e = migrate.Apply(ctx, p.Config().ConnConfig.ConnString(), dbmigrations.Files)
	if e != nil || len(applied) != 0 {
		t.Fatal("clock migration replay", applied, e)
	}
}
