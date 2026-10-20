//go:build integration

package adapters

import (
	"context"
	"fmt"
	dbmigrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	quality "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/quality"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
	"github.com/jackc/pgx/v5/pgxpool"
	"io/fs"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"
)

func TestLateHistoricalPublicationDoesNotRegressCurrentOrWatermark(t *testing.T) {
	p := freshPool(t)
	ctx := context.Background()
	pub := NewPublisher(p)
	maker := &idMaker{}
	recent := sampleInput("b")
	current, e := pub.Publish(ctx, recent, maker.next)
	if e != nil {
		t.Fatal(e)
	}
	older := sampleInput("c")
	older.Edition = "2026-08"
	historical, e := pub.Publish(ctx, older, maker.next)
	if e != nil {
		t.Fatal(e)
	}
	q := quality.New(p)
	id, e := q.GetCurrentRevision(ctx, quality.GetCurrentRevisionParams{Source: "pmqc", Stream: "pmqc-assay-v1"})
	if e != nil || uuidString(id) != current {
		t.Fatal("late history regressed current", uuidString(id), e)
	}
	hid, _ := mustUUID(historical)
	state, e := q.GetRevision(ctx, hid)
	if e != nil || state.Status != "published" {
		t.Fatal("history not retained", state, e)
	}
	for _, edition := range []string{"2026-09", "2026-08"} {
		if e = q.SetWatermark(ctx, quality.SetWatermarkParams{Source: "pmqc", Stream: "pmqc-assay-v1", LastEdition: edition, LastHash: recent.ManifestHash}); e != nil {
			t.Fatal(e)
		}
	}
	mark, e := q.GetWatermark(ctx, quality.GetWatermarkParams{Source: "pmqc", Stream: "pmqc-assay-v1"})
	if e != nil || mark.LastEdition != "2026-09" {
		t.Fatal("late history regressed watermark", mark, e)
	}
}

func TestConcurrentHistoricalPublicationKeepsNewestEdition(t *testing.T) {
	base := freshPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cfg := base.Config()
	cfg.MaxConns = 16
	cfg.ConnConfig.RuntimeParams["application_name"] = "quality-backfill-race"
	p, e := pgxpool.NewWithConfig(ctx, cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer p.Close()
	pub := NewPublisher(p)
	m := &idMaker{}
	initial := sampleInput("a")
	initial.Edition = "2026-01"
	if _, e := pub.Publish(ctx, initial, m.next); e != nil {
		t.Fatal(e)
	}
	holder, e := base.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer holder.Rollback(context.Background())
	if _, e = holder.Exec(ctx, "SELECT revision_id FROM quality_current_revisions FOR UPDATE"); e != nil {
		t.Fatal(e)
	}
	var wg sync.WaitGroup
	for i := 2; i <= 9; i++ {
		wg.Add(1)
		go func(month int) {
			defer wg.Done()
			in := sampleInput("b")
			in.Edition = fmt.Sprintf("2026-%02d", month)
			if _, e := pub.Publish(ctx, in, m.next); e != nil {
				t.Error(e)
			}
		}(i)
	}
	blocked := 0
	for ctx.Err() == nil && blocked < 8 {
		if e = base.QueryRow(ctx, "SELECT count(*) FROM pg_stat_activity WHERE datname=current_database() AND application_name='quality-backfill-race' AND wait_event_type='Lock'").Scan(&blocked); e != nil {
			break
		}
	}
	if e = holder.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	wg.Wait()
	if blocked != 8 {
		t.Fatal("missing real SQL barrier", blocked)
	}
	var edition string
	if e = base.QueryRow(ctx, "SELECT e.edition FROM quality_current_revisions c JOIN quality_revisions r ON r.id=c.revision_id JOIN quality_editions e ON e.id=r.edition_id").Scan(&edition); e != nil || edition != "2026-09" {
		t.Fatal("concurrent current regression", edition, e)
	}
}

func TestFrozenMonthlyMembersAndPMQCPeriods(t *testing.T) {
	p := freshPool(t)
	ctx := context.Background()
	pub := NewPublisher(p)
	m := &idMaker{}
	in := sampleInput("b")
	in.Resources[0].Member = "2026/PMQC_2026_09.csv"
	in.Resources[0].MemberSHA256 = strings.Repeat("c", 64)
	in.Resources[0].MemberBytes = 12
	in.Periods = []application.PeriodInput{{Label: "2026-09", Start: "2026-09-01", End: "2026-09-30"}}
	old, e := pub.Publish(ctx, in, m.next)
	if e != nil {
		t.Fatal(e)
	}
	in.Resources[0].MemberSHA256 = strings.Repeat("d", 64)
	in.Resources[0].MemberBytes = 13
	current, e := pub.Publish(ctx, in, m.next)
	if e != nil || old == current {
		t.Fatal("changed member failed correction", e)
	}
	q := quality.New(p)
	for _, tc := range []struct {
		id, hash string
		n        int64
	}{{old, strings.Repeat("c", 64), 12}, {current, strings.Repeat("d", 64), 13}} {
		id, _ := mustUUID(tc.id)
		r, e := q.ReadRevisionResources(ctx, id)
		if e != nil || len(r) != 1 || r[0].MemberName != in.Resources[0].Member || r[0].MemberSha256.String != tc.hash || r[0].MemberBytes.Int64 != tc.n {
			t.Fatal("member snapshot lost", r, e)
		}
		periods, e := q.ListRevisionPeriods(ctx, id)
		if e != nil || len(periods) != 1 || periods[0].PeriodLabel != "2026-09" {
			t.Fatal("PMQC period lost", periods, e)
		}
	}
	replay, e := pub.Publish(ctx, in, m.next)
	if e != nil || replay != current {
		t.Fatal("member replay forked", e)
	}
}

func TestMemberProvenanceUpgradePreservesPublishedReplay(t *testing.T) {
	ctx := context.Background()
	prior := fstest.MapFS{}
	entries, e := fs.ReadDir(dbmigrations.Files, ".")
	if e != nil {
		t.Fatal(e)
	}
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() >= "000100" {
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
	in := sampleInput("b")
	id := seedLegacyAssayRevision(t, p, in, m)
	req := editionRequest{Source: in.Source, Stream: in.Stream, Edition: in.Edition, ManifestHash: in.ManifestHash, Counts: in.Counts, Resources: in.Resources, Links: in.Links}
	if _, e = p.Exec(ctx, "INSERT INTO quality_revision_provenance(revision_id,provenance_state,input,accepted,duplicates,quarantined,manifest_hash,publication_key) VALUES($1,'VERIFIED',$2,$3,$4,$5,$6,$7)", id, in.Counts.Input, in.Counts.Accepted, in.Counts.Duplicates, in.Counts.Quarantined, in.ManifestHash, req.publicationKey()); e != nil {
		t.Fatal(e)
	}
	r := in.Resources[0]
	if _, e = p.Exec(ctx, "INSERT INTO quality_revision_resources(revision_id,url,sha256,bytes) VALUES($1,$2,$3,$4)", id, r.URL, r.SHA256, int64(r.Bytes)); e != nil {
		t.Fatal(e)
	}
	applied, e := migrate.Apply(ctx, p.Config().ConnConfig.ConnString(), dbmigrations.Files)
	if e != nil || !slices.Equal(applied, []string{"000100", "000101", "000102", "000103", "000104", "000105", "000106", "000107", "000108", "000109"}) {
		t.Fatal("upgrade", applied, e)
	}
	replay, e := NewPublisher(p).Publish(ctx, in, m.next)
	if e != nil || replay != id {
		t.Fatal("pre-upgrade replay forked", replay, e)
	}
	rid, _ := mustUUID(id)
	resources, e := quality.New(p).ReadRevisionResources(ctx, rid)
	if e != nil || len(resources) != 1 || resources[0].MemberName != "" || resources[0].MemberSha256.Valid || resources[0].MemberBytes.Valid {
		t.Fatal("upgrade invented member provenance", resources, e)
	}
	applied, e = migrate.Apply(ctx, p.Config().ConnConfig.ConnString(), dbmigrations.Files)
	if e != nil || len(applied) != 0 {
		t.Fatal("migration replay", applied, e)
	}
	var edition string
	if e = p.QueryRow(ctx, "SELECT edition FROM quality_current_revisions").Scan(&edition); e != nil || edition != in.Edition {
		t.Fatal("current edition backfill", edition, e)
	}
}
