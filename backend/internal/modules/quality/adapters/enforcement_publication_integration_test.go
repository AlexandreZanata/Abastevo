//go:build integration

package adapters

import (
	"context"
	"errors"
	"fmt"
	quality "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/quality"
	qualityread "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters/read"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestEnforcementNativePublishedFactsAreReadable(t *testing.T) {
	p := freshPool(t)
	ctx := context.Background()
	m := &idMaker{}
	in := enforcementInput()
	req := editionRequest{Source: "enforcement", Stream: "enforcement-v1", Edition: "2019+", Resources: in.Resources, Counts: in.Counts, ManifestHash: in.ManifestHash}
	_, _, e := NewPublisher(p).publishTx(ctx, req, m.next, func(q *quality.Queries, _, rid pgtype.UUID) error {
		if _, e := q.RecordEnforcementSourceContext(ctx, enforcementContextArgs(rid)); e != nil {
			return e
		}
		_, e := q.StageEnforcementSourceRecords(ctx, quality.StageEnforcementSourceRecordsParams{RevisionID: rid, Records: enforcementJSON(t, m, nil)})
		return e
	})
	if e != nil {
		t.Fatal(e)
	}
	rows, _, lease, e := qualityread.NewReader(p).Enforcement(ctx, time.Now(), "enforcement", "enforcement-v1", "", "", "", "", 100)
	if e != nil || len(rows) != 1 || lease.SourceInputContract != "anp-enforcement-xlsx-2019-v1" {
		t.Fatal("published native facts lost", len(rows), e)
	}
}

func TestEnforcementPublicationReplayCorrectionRollbackAndConcurrentWriters(t *testing.T) {
	p := freshPool(t)
	ctx := context.Background()
	m := &idMaker{}
	pub := NewPublisher(p)
	in := enforcementInput()
	//129 rows cross a chunk boundary; the source explicitly has distinct results.
	for i := 1; i < 129; i++ {
		r := in.Records[0]
		r.Result = fmt.Sprintf("Distinct result %d", i)
		r.EvidenceRef = fmt.Sprintf("synthetic.xlsx#row:%d", i+2)
		r.Fingerprint = r.ContentHash()
		r.RecordKey = "enforcement-source:" + r.Fingerprint
		in.Records = append(in.Records, r)
	}
	in.Counts.Input = 129
	in.Counts.Accepted = 129
	var wg sync.WaitGroup
	results := make(chan string, 4)
	errs := make(chan error, 4)
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, e := pub.PublishEnforcementSource(ctx, in, m.next)
			results <- r
			errs <- e
		}()
	}
	wg.Wait()
	close(results)
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	first := ""
	for r := range results {
		if first == "" {
			first = r
		}
		if r != first {
			t.Fatal("publication fork")
		}
	}
	if r, e := pub.PublishEnforcementSource(ctx, in, m.next); e != nil || r != first {
		t.Fatal("replay fork", e)
	}
	if _, e := pub.WithGuard(func(context.Context, pgx.Tx) error { return errors.New("denied") }).PublishEnforcementSource(ctx, in, m.next); e == nil {
		t.Fatal("replay bypassed guard")
	}
	reader := qualityread.NewReader(p)
	rows, next, lease, e := reader.Enforcement(ctx, time.Now(), "enforcement", "enforcement-v1", "", "", "", "", 100)
	if e != nil || len(rows) != 100 || next == "" || !lease.Stale || lease.SourceClockPrecision != "UNAVAILABLE" || rows[0].AsOf != "" || rows[0].OccurredOn != "" || rows[0].DocumentDay != "2026-01-02" {
		t.Fatal("source semantics lost", e)
	}
	key, id, _ := strings.Cut(next, "|")
	tail, last, _, e := reader.EnforcementAtLease(ctx, lease, "", "", key, id, 100)
	if e != nil || len(tail) != 29 || last != "" {
		t.Fatal("traversal lost facts", e)
	}
	corrected := in
	corrected.Records = append([]application.EnforcementSourceRecord(nil), in.Records...)
	corrected.ManifestHash = strings.Repeat("e", 64)
	corrected.Records[0].Result = "Corrected source result"
	corrected.Records[0].Fingerprint = corrected.Records[0].ContentHash()
	corrected.Records[0].RecordKey = "enforcement-source:" + corrected.Records[0].Fingerprint
	second, e := pub.PublishEnforcementSource(ctx, corrected, m.next)
	if e != nil || second == first {
		t.Fatal("correction failed", e)
	}
	frozen, _, old, e := reader.AtRevision(first).Enforcement(ctx, time.Now(), "enforcement", "enforcement-v1", "", "", "", "", 100)
	if e != nil || len(frozen) != 100 || old.Status != "superseded" {
		t.Fatal("frozen revision lost", e)
	}
	fail := in
	fail.ManifestHash = strings.Repeat("d", 64)
	calls := 0
	if _, e := pub.PublishEnforcementSource(ctx, fail, func() (string, error) {
		calls++
		if calls == 131 {
			return "", errors.New("second chunk failed")
		}
		return m.next()
	}); e == nil {
		t.Fatal("late chunk failure committed")
	}
	var current string
	if e := p.QueryRow(ctx, "SELECT revision_id::text FROM quality_current_revisions WHERE source='enforcement'").Scan(&current); e != nil || current != second {
		t.Fatal("pointer switched on failure", e)
	}
	var n int
	if e := p.QueryRow(ctx, "SELECT count(*) FROM quality_enforcement_source_revisions").Scan(&n); e != nil || n != 2 {
		t.Fatal("failed metadata retained", e)
	}
	if _, e := pub.PublishEnforcementSource(ctx, in, m.next); !errors.Is(e, application.ErrSupersededPublication) {
		t.Fatal("old replay resurrected", e)
	}
	empty := enforcementInput()
	empty.Records = nil
	empty.Counts = domain.Counts{}
	empty.Context.FirstDocumentDay = ""
	empty.Context.LastDocumentDay = ""
	if _, e := pub.PublishEnforcementSource(ctx, empty, m.next); e != nil {
		t.Fatal("empty context publication", e)
	}
}
