//go:build integration

package adapters

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"strings"
	"sync"
	"testing"
	"time"

	quality "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/quality"
	qualityread "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters/read"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/domain"
)

func nativeFineInput(ch string) application.NativeFineInput {
	a := application.FineObservation{ObservationKey: "observation:" + strings.Repeat("c", 64), SourceCNPJ: "11222333000181", ProceedingRef: "000000000000001", DUFRef: "SYNTHETIC/1", Department: "SYNTHETIC", Status: "SOURCE only", DueOn: "2025-02-28", AmountText: "150000,00", AmountMinor: 15000000, PaidText: "180000,01", PaidMinor: 18000001, ReportYear: 2025, ExtractionMonth: "2026-01", EvidenceRef: "synthetic.csv:8"}
	a.ObservationKey = a.ObservationIdentity()
	b := a
	b.DUFRef = "SYNTHETIC/2"
	b.ObservationKey = b.ObservationIdentity()
	b.EvidenceRef = "synthetic.csv:9"
	return application.NativeFineInput{Edition: "2025", Clock: application.NativeFineClock{ReportYear: 2025, ExtractionMonth: "2026-01"}, Resources: []application.Resource{{URL: "https://www.gov.br/anp/fines-2025.csv", SHA256: strings.Repeat("a", 64), Bytes: 100}}, Counts: domain.Counts{Input: 2, Accepted: 2}, ManifestHash: strings.Repeat(ch, 64), Observations: []application.FineObservation{a, b}}
}
func TestNativeFinePublicationReplayCorrectionRollbackAndHistoricalClocks(t *testing.T) {
	p := freshPool(t)
	ctx := context.Background()
	m := &idMaker{}
	pub := NewPublisher(p)
	in := nativeFineInput("b")
	first, e := pub.PublishNativeFines(ctx, in, m.next)
	if e != nil {
		t.Fatal(e)
	}
	if replay, e := pub.PublishNativeFines(ctx, in, m.next); e != nil || replay != first {
		t.Fatal("native replay forked", e)
	}
	denied := pub.WithGuard(func(context.Context, pgx.Tx) error { return errors.New("guard denied") })
	if _, e = denied.PublishNativeFines(ctx, in, m.next); e == nil {
		t.Fatal("native replay bypassed live guard")
	}
	read := qualityread.NewReader(p)
	now := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	rows, next, lease, e := read.Sanctions(ctx, now, "sanctions", domain.StreamSanction, "", "", "", 1)
	if e != nil || len(rows) != 1 || next == "" || !lease.Stale || lease.SourceExtractionMonth != "2026-01" || lease.SourceClockPrecision != "MONTH" {
		t.Fatal("native month lease/page", e)
	}
	row := rows[0]
	if row.AsOf != "" || row.DecidedOn != "" || row.DueOn != "2025-02-28" || row.IdentityKind != "CONTENT_OBSERVATION" || row.ClockPrecision != "MONTH" || row.ExtractionMonth != "2026-01" || row.PaidMinor == nil || *row.PaidMinor != 18000001 || row.AmountText != "150000,00" || row.PaidText != "180000,01" || (row.DUFRef != "SYNTHETIC/1" && row.DUFRef != "SYNTHETIC/2") {
		t.Fatal("source facts relabelled or lost")
	}
	corrected := nativeFineInput("e")
	corrected.Observations[0].Status = "source correction"
	corrected.Observations[0].ObservationKey = corrected.Observations[0].ObservationIdentity()
	second, e := pub.PublishNativeFines(ctx, corrected, m.next)
	if e != nil || second == first {
		t.Fatal("native correction", e)
	}
	old, _, oldLease, e := read.AtRevision(first).Sanctions(ctx, now, "sanctions", domain.StreamSanction, "", "", "", 10)
	if e != nil || len(old) != 2 || old[0].Status != "SOURCE only" || oldLease.Status != "superseded" || oldLease.SourceExtractionMonth != "2026-01" {
		t.Fatal("frozen native history changed", e)
	}
	bad := nativeFineInput("a")
	calls := 0
	_, e = pub.PublishNativeFines(ctx, bad, func() (string, error) {
		calls++
		if calls == 4 {
			return "", errors.New("synthetic second-stage failure")
		}
		return m.next()
	})
	if e == nil {
		t.Fatal("partial native publication committed")
	}
	q := quality.New(p)
	current, e := q.GetCurrentRevision(ctx, quality.GetCurrentRevisionParams{Source: "sanctions", Stream: domain.StreamSanction})
	if e != nil || uuidString(current) != second {
		t.Fatal("failed native correction switched pointer", e)
	}
	var clocks, staging int
	if e = p.QueryRow(ctx, "SELECT count(*) FROM quality_native_fine_revisions").Scan(&clocks); e != nil || clocks != 2 {
		t.Fatal("partial source metadata leaked", e)
	}
	if e = p.QueryRow(ctx, "SELECT count(*) FROM quality_revisions WHERE status='staging'").Scan(&staging); e != nil || staging != 0 {
		t.Fatal("partial native revision leaked", e)
	}
}
func TestNativeFineConcurrentConvergenceAndEmptyRevisionClock(t *testing.T) {
	p := freshPool(t)
	ctx := context.Background()
	m := &idMaker{}
	pub := NewPublisher(p)
	in := nativeFineInput("b")
	results := make(chan string, 8)
	errs := make(chan error, 8)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); rev, e := pub.PublishNativeFines(ctx, in, m.next); results <- rev; errs <- e }()
	}
	wg.Wait()
	close(results)
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal("concurrent native publication", e)
		}
	}
	first := ""
	for rev := range results {
		if first == "" {
			first = rev
		}
		if rev != first {
			t.Fatal("native publication fork")
		}
	}
	empty := nativeFineInput("e")
	empty.Observations = nil
	empty.Counts = domain.Counts{}
	empty.Clock.ExtractionMonth = "2026-02"
	rev, e := pub.PublishNativeFines(ctx, empty, m.next)
	if e != nil {
		t.Fatal("empty native publication", e)
	}
	rows, _, lease, e := qualityread.NewReader(p).Sanctions(ctx, time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC), "sanctions", domain.StreamSanction, "", "", "", 10)
	if e != nil || len(rows) != 0 || !lease.HasPublished || !lease.Stale || lease.RevisionID != rev || lease.SourceExtractionMonth != "2026-02" {
		t.Fatal("empty source metadata lost", e)
	}
}
