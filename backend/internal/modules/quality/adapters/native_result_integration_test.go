//go:build integration

package adapters

import (
	"context"
	dbmigrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	quality "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/quality"
	qualityread "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters/read"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
	"io/fs"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

func TestNativePMQCEmptyResultIsRetained(t *testing.T) {
	p := freshPool(t)
	input := sampleInput("b")
	input.Assays[0].ResultText = ""
	m := &idMaker{}
	rid, e := NewPublisher(p).Publish(context.Background(), input, m.next)
	if e != nil {
		t.Fatal("source-empty assay lost", e)
	}
	var n int
	if e = p.QueryRow(context.Background(), "SELECT count(*) FROM quality_assays WHERE revision_id=$1 AND result_text=''", rid).Scan(&n); e != nil || n != 1 {
		t.Fatal(n, e)
	}
}

func TestNativePMQCResultExpansionUpgradeReplayAndRollback(t *testing.T) {
	ctx := context.Background()
	prior := fstest.MapFS{}
	var expected []string
	entries, e := fs.ReadDir(dbmigrations.Files, ".")
	if e != nil {
		t.Fatal(e)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		if entry.Name() >= "000097" {
			expected = append(expected, entry.Name()[:6])
			continue
		}
		b, e := fs.ReadFile(dbmigrations.Files, entry.Name())
		if e != nil {
			t.Fatal(e)
		}
		prior[entry.Name()] = &fstest.MapFile{Data: b}
	}
	p := freshPoolWithMigrations(t, prior)
	m := &idMaker{}
	input := sampleInput("b")
	previous := seedLegacyAssayRevision(t, p, input, m)
	dsn := p.Config().ConnConfig.ConnString()
	if e = migrate.Check(ctx, p, dbmigrations.Files); e == nil {
		t.Fatal("old chain marked compatible")
	}
	applied, e := migrate.Apply(ctx, dsn, dbmigrations.Files)
	if e != nil || !slices.Equal(applied, expected) {
		t.Fatal(applied, e)
	}
	applied, e = migrate.Apply(ctx, dsn, dbmigrations.Files)
	if e != nil || len(applied) != 0 {
		t.Fatal("migration replay", applied, e)
	}
	if e = migrate.Check(ctx, p, dbmigrations.Files); e != nil {
		t.Fatal(e)
	}
	legacyID, e := mustUUID(previous)
	if e != nil {
		t.Fatal(e)
	}
	legacyProv, e := quality.New(p).GetRevisionProvenance(ctx, legacyID)
	if e != nil || legacyProv.ProvenanceState != "LEGACY_UNVERIFIED" {
		t.Fatal("legacy provenance fabricated", legacyProv, e)
	}
	if legacyProv.ManifestHash.Valid || legacyProv.PublicationKey.Valid || legacyProv.Input.Valid || legacyProv.Accepted.Valid || legacyProv.Duplicates.Valid || legacyProv.Quarantined.Valid {
		t.Fatal("legacy provenance invented a hash")
	}
	legacyLease, e := qualityread.NewReader(p).LeaseBlock(ctx, time.Now(), "pmqc", "pmqc-assay-v1")
	if e != nil || legacyLease.HasPublished || !legacyLease.Stale {
		t.Fatal("unverified legacy served as available lease", legacyLease, e)
	}
	var original string
	if e = p.QueryRow(ctx, "SELECT result_text FROM quality_assays WHERE revision_id=$1 AND assay_ref='ASPECTO'", previous).Scan(&original); e != nil || original != "Límpido" {
		t.Fatal("upgrade changed history", original, e)
	}
	input.ManifestHash = strings.Repeat("c", 64)
	input.Assays[0].ResultText = ""
	current, e := NewPublisher(p).Publish(ctx, input, m.next)
	if e != nil {
		t.Fatal(e)
	}
	input.ManifestHash = strings.Repeat("d", 64)
	input.Assays[0].ResultText = strings.Repeat("x", 1025)
	if _, e = NewPublisher(p).Publish(ctx, input, m.next); e == nil {
		t.Fatal("resource ceiling weakened")
	}
	var actual string
	if e = p.QueryRow(ctx, "SELECT revision_id::text FROM quality_current_revisions WHERE source='pmqc'").Scan(&actual); e != nil || actual != current {
		t.Fatal("failed publication replaced current", actual, e)
	}
	currentID, e := mustUUID(current)
	if e != nil {
		t.Fatal(e)
	}
	currentProv, e := quality.New(p).GetRevisionProvenance(ctx, currentID)
	if e != nil || currentProv.ProvenanceState != "VERIFIED" || !currentProv.ManifestHash.Valid || currentProv.ManifestHash.String != strings.Repeat("c", 64) {
		t.Fatal("failed correction touched frozen provenance", currentProv, e)
	}
}
