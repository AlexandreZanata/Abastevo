//go:build integration

package adapters

import (
	"context"
	"errors"
	quality "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/quality"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/application"
	"github.com/jackc/pgx/v5/pgxpool"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestRevisionProvenanceSurvivesCorrection(t *testing.T) {
	p := freshPool(t)
	ctx := context.Background()
	m := &idMaker{}
	pub := NewPublisher(p)
	input := sampleInput("b")
	old, e := pub.Publish(ctx, input, m.next)
	if e != nil {
		t.Fatal(e)
	}
	input.Counts.Input = 3
	input.Counts.Quarantined = 1
	input.ManifestHash = strings.Repeat("c", 64)
	input.Resources[0].SHA256 = strings.Repeat("d", 64)
	current, e := pub.Publish(ctx, input, m.next)
	if e != nil {
		t.Fatal(e)
	}
	for _, tc := range []struct {
		id             string
		n              int
		hash, resource string
	}{{old, 2, strings.Repeat("b", 64), strings.Repeat("a", 64)}, {current, 3, strings.Repeat("c", 64), strings.Repeat("d", 64)}} {
		var n int
		var hash, resource string
		if e = p.QueryRow(ctx, "SELECT input,manifest_hash FROM quality_revision_provenance WHERE revision_id=$1", tc.id).Scan(&n, &hash); e != nil {
			t.Fatal(e)
		}
		if e = p.QueryRow(ctx, "SELECT sha256 FROM quality_revision_resources WHERE revision_id=$1", tc.id).Scan(&resource); e != nil {
			t.Fatal(e)
		}
		if n != tc.n || hash != tc.hash || resource != tc.resource {
			t.Fatal("revision provenance changed", tc.id, n, hash, resource)
		}
	}
	var registered int
	if e = p.QueryRow(ctx, "SELECT count(*) FROM quality_resources WHERE sha256=$1", strings.Repeat("d", 64)).Scan(&registered); e != nil || registered != 1 {
		t.Fatal("corrected source undiscoverable", registered, e)
	}
}

func TestRevisionProvenanceConcurrentCorrectionConverges(t *testing.T) {
	base := freshPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cfg := base.Config()
	cfg.MaxConns = 16
	cfg.ConnConfig.RuntimeParams["application_name"] = "quality-provenance-race"
	p, e := pgxpool.NewWithConfig(ctx, cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer p.Close()
	m := &idMaker{}
	pub := NewPublisher(p)
	input := sampleInput("b")
	if _, e := pub.Publish(ctx, input, m.next); e != nil {
		t.Fatal(e)
	}
	holder, e := p.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer holder.Rollback(context.Background())
	if _, e = holder.Exec(ctx, "SELECT edition_id FROM quality_manifests FOR UPDATE"); e != nil {
		t.Fatal(e)
	}
	input.ManifestHash = strings.Repeat("c", 64)
	input.Assays[0].ResultText = "Corrected"
	var wg sync.WaitGroup
	out := make(chan string, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id, e := pub.Publish(ctx, input, m.next)
			if e != nil {
				t.Error(e)
				return
			}
			out <- id
		}()
	}
	// All writers must reach a real SQL lock before releasing the first one;
	// this reproduces stale pre-lock idempotency reads instead of trusting timing.
	blocked := 0
	for ctx.Err() == nil && blocked < 8 {
		if e = base.QueryRow(ctx, "SELECT count(*) FROM pg_stat_activity WHERE datname=current_database() AND application_name='quality-provenance-race' AND wait_event_type='Lock'").Scan(&blocked); e != nil {
			break
		}
	}
	if e = holder.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	wg.Wait()
	close(out)
	if blocked != 8 {
		t.Fatal("writers did not reach lock barrier", blocked)
	}
	seen := map[string]bool{}
	for id := range out {
		seen[id] = true
	}
	if len(seen) != 1 {
		t.Fatal("correction replay forked", seen)
	}
	var n int
	if e := p.QueryRow(ctx, "SELECT count(*) FROM quality_revisions").Scan(&n); e != nil || n != 2 {
		t.Fatal("extra revisions", n, e)
	}
}

func TestRevisionProvenancePeriodSnapshots(t *testing.T) {
	p := freshPool(t)
	ctx := context.Background()
	m := &idMaker{}
	pub := NewPublisher(p)
	rev1, e := pub.PublishEvents(ctx, sampleEventsInput("b"), m.next)
	if e != nil {
		t.Fatal(e)
	}
	corrected := sampleEventsInput("e")
	corrected.Enforcement[0].Status = "interditado"
	corrected.Periods = []application.PeriodInput{{Label: "2026-10", Start: "2026-10-01", End: "2026-10-14"}}
	corrected.Resources[0].SHA256 = strings.Repeat("f", 64)
	rev2, e := pub.PublishEvents(ctx, corrected, m.next)
	if e != nil {
		t.Fatal(e)
	}
	q := quality.New(p)

	for _, tc := range []struct {
		id, hash, resource string
		periods            []application.PeriodInput
	}{
		{rev1, strings.Repeat("b", 64), strings.Repeat("a", 64), sampleEventsInput("b").Periods},
		{rev2, strings.Repeat("e", 64), strings.Repeat("f", 64), corrected.Periods},
	} {
		rid, e := mustUUID(tc.id)
		if e != nil {
			t.Fatal(e)
		}
		periods, e := q.ListRevisionPeriods(ctx, rid)
		if e != nil || len(periods) != len(tc.periods) {
			t.Fatal("period snapshot lost", tc.id, periods, e)
		}
		for i, want := range tc.periods {
			got := periods[i]
			if got.PeriodLabel != want.Label || got.PeriodStart.Time.Format("2006-01-02") != want.Start || got.PeriodEnd.Time.Format("2006-01-02") != want.End {
				t.Fatal("period overwritten", tc.id, got, want)
			}
		}
		prov, e := q.GetRevisionProvenance(ctx, rid)
		if e != nil || prov.ProvenanceState != "VERIFIED" || !prov.ManifestHash.Valid || prov.ManifestHash.String != tc.hash {
			t.Fatal("provenance snapshot wrong", tc.id, prov, e)
		}
		var resource string
		if e = p.QueryRow(ctx, "SELECT sha256 FROM quality_revision_resources WHERE revision_id=$1", tc.id).Scan(&resource); e != nil || resource != tc.resource {
			t.Fatal("resource snapshot wrong", tc.id, resource, e)
		}
	}
}

func TestRevisionProvenanceRejectsSupersededAndBindsRawResources(t *testing.T) {
	p := freshPool(t)
	ctx := context.Background()
	m := &idMaker{}
	pub := NewPublisher(p)
	original := sampleInput("b")
	old, e := pub.Publish(ctx, original, m.next)
	if e != nil {
		t.Fatal(e)
	}
	corrected := sampleInput("b")
	corrected.Resources[0].SHA256 = strings.Repeat("f", 64)
	current, e := pub.Publish(ctx, corrected, m.next)
	if e != nil || old == current {
		t.Fatal("raw resource change ignored", current, e)
	}
	if _, e = pub.Publish(ctx, original, m.next); !errors.Is(e, application.ErrSupersededPublication) {
		t.Fatal("historical input resurrected", e)
	}
	var actual string
	if e = p.QueryRow(ctx, "SELECT revision_id::text FROM quality_current_revisions WHERE source='pmqc'").Scan(&actual); e != nil || actual != current {
		t.Fatal("stale retry replaced current", actual, e)
	}
}
