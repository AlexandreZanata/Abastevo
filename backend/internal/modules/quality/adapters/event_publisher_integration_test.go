//go:build integration

package adapters

import (
	"context"
	"strings"
	"testing"

	quality "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/quality"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/domain"
)

func sampleEventsInput(hashChar string) application.EventsInput {
	amount := int64(150000)
	return application.EventsInput{
		Source: "enforcement", Stream: "enforcement-v1", Edition: "2026-09",
		Resources: []application.Resource{{
			URL:    "https://www.gov.br/anp/acoes-de-fiscalizacao",
			SHA256: strings.Repeat("a", 64), Bytes: 9001,
		}},
		Counts:       domain.Counts{Input: 2, Accepted: 2},
		ManifestHash: strings.Repeat(hashChar, 64),
		Periods: []application.PeriodInput{
			{Label: "2026-08", Start: "2026-08-01", End: "2026-08-31"},
			{Label: "2026-09", Start: "2026-09-01", End: "2026-09-30"},
		},
		Enforcement: []application.EnforcementInput{
			{NativeKey: "AUTO-1", Scope: "municipal", Status: "autuado", SubjectCNPJ: "11222333000181", OccurredOn: "2026-08-02", AsOf: "2026-09-30", EvidenceRef: "e#AUTO-1"},
		},
		Sanctions: []application.SanctionInput{
			{NativeKey: "MULTA-1", ProceedingRef: "48000.001234/2026-11", PartyCNPJ: "04218406000104", AmountMinor: &amount, Status: "aplicada", DecidedOn: "2026-07-15", AsOf: "2026-09-30", EvidenceRef: "e#MULTA-1"},
		},
	}
}

func TestPublishEventsAndScopedLease(t *testing.T) {
	pool := freshPool(t)
	ctx := context.Background()
	pub := NewPublisher(pool)
	maker := &idMaker{}

	rev, err := pub.PublishEvents(ctx, sampleEventsInput("b"), maker.next)
	if err != nil {
		t.Fatalf("publish events: %v", err)
	}
	q := quality.New(pool)
	current, err := q.GetCurrentRevision(ctx, quality.GetCurrentRevisionParams{
		Source: "enforcement", Stream: "enforcement-v1",
	})
	if err != nil {
		t.Fatalf("scoped lease read: %v", err)
	}
	if uuidString(current) != rev {
		t.Fatalf("enforcement lease = %s, want %s", uuidString(current), rev)
	}
	// The assay stream lease stays independent: nothing published there.
	if _, err := q.GetCurrentRevision(ctx, quality.GetCurrentRevisionParams{
		Source: "pmqc", Stream: "pmqc-assay-v1",
	}); err == nil {
		t.Fatal("assay lease moved by an enforcement publication")
	}
	revID, _ := mustUUID(rev)
	enforcement, err := q.ListRevisionEnforcement(ctx, revID)
	if err != nil || len(enforcement) != 1 || enforcement[0].Scope != "municipal" {
		t.Fatalf("enforcement rows = %+v %v", enforcement, err)
	}
	if enforcement[0].Status != "autuado" {
		t.Errorf("procedural status rewritten: %q", enforcement[0].Status)
	}
	sanctions, err := q.ListRevisionSanctions(ctx, revID)
	if err != nil || len(sanctions) != 1 {
		t.Fatalf("sanction rows = %+v %v", sanctions, err)
	}
	if !sanctions[0].AmountMinor.Valid || sanctions[0].AmountMinor.Int64 != 150000 {
		t.Errorf("sanction amount lost: %+v", sanctions[0].AmountMinor)
	}
	edition, err := q.GetEditionByKey(ctx, quality.GetEditionByKeyParams{
		Source: "enforcement", Stream: "enforcement-v1", Edition: "2026-09",
	})
	if err != nil {
		t.Fatal(err)
	}
	periods, err := q.ListEditionPeriods(ctx, edition.ID)
	if err != nil || len(periods) != 2 || periods[0].PeriodLabel != "2026-08" {
		t.Fatalf("period claims = %+v %v", periods, err)
	}
}

func TestPublishEventsReplayStable(t *testing.T) {
	pool := freshPool(t)
	ctx := context.Background()
	pub := NewPublisher(pool)
	maker := &idMaker{}

	first, err := pub.PublishEvents(ctx, sampleEventsInput("b"), maker.next)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	second, err := pub.PublishEvents(ctx, sampleEventsInput("b"), maker.next)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if first != second {
		t.Fatalf("event replay returned %s, want stable %s", second, first)
	}
}

func TestPublishEventsPartialFailureRollback(t *testing.T) {
	pool := freshPool(t)
	ctx := context.Background()
	pub := NewPublisher(pool)
	maker := &idMaker{}

	// Empty evidence reference passes application validation but violates
	// the ledger CHECK inside the transaction: the whole publication must
	// roll back, leaving no edition, manifest or revision behind.
	bad := sampleEventsInput("b")
	bad.Enforcement[0].EvidenceRef = ""
	if _, err := pub.PublishEvents(ctx, bad, maker.next); err == nil {
		t.Fatal("empty evidence reference published")
	}
	if _, err := quality.New(pool).GetEditionByKey(ctx, quality.GetEditionByKeyParams{
		Source: "enforcement", Stream: "enforcement-v1", Edition: "2026-09",
	}); err == nil {
		t.Fatal("failed event publication left a visible edition")
	}
}

func TestPublishEventsCorrectionSupersedes(t *testing.T) {
	pool := freshPool(t)
	ctx := context.Background()
	pub := NewPublisher(pool)
	maker := &idMaker{}

	rev1, err := pub.PublishEvents(ctx, sampleEventsInput("b"), maker.next)
	if err != nil {
		t.Fatalf("publish rev1: %v", err)
	}
	corrected := sampleEventsInput("e")
	corrected.Enforcement[0].Status = "interditado"
	rev2, err := pub.PublishEvents(ctx, corrected, maker.next)
	if err != nil {
		t.Fatalf("publish correction: %v", err)
	}
	if rev1 == rev2 {
		t.Fatal("event correction reused the revision id")
	}
	q := quality.New(pool)
	rev1ID, _ := mustUUID(rev1)
	got1, err := q.GetRevision(ctx, rev1ID)
	if err != nil {
		t.Fatal(err)
	}
	if got1.Status != "superseded" {
		t.Errorf("rev1 status = %s, want superseded", got1.Status)
	}
	rows, err := q.ListRevisionEnforcement(ctx, rev1ID)
	if err != nil || len(rows) != 1 || rows[0].Status != "autuado" {
		t.Fatalf("superseded event history lost: %+v %v", rows, err)
	}
}
