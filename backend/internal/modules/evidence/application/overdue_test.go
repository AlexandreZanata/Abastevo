package application

import (
	"context"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/domain"
)

// SEC05-A overdue originals (AUD-06, B-BR-M04): an overdue quarantine
// original with a failed deletion trips the audit exactly like an
// overdue final — the final-only report healthy-zero is gone.

func TestAuditOverdueTripsOnOriginalsAlone(t *testing.T) {
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	old := now.Add(-48 * time.Hour)
	store := &fakeSweepStore{
		overdueOriginals: []domain.SessionRef{
			{ID: "s-old", QuarantineKey: "q/old", Status: domain.StateIssued, CreatedAt: old},
		},
	}
	rep, err := AuditOverdue(context.Background(), store, now, 100)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Originals != 1 || !rep.OldestOriginal.Equal(old) {
		t.Fatalf("originals = %+v, want 1 at %v", rep, old)
	}
	if rep.Count != 0 {
		t.Fatalf("finals = %d, want zero (originals-only ledger)", rep.Count)
	}
	if !rep.Unhealthy() {
		t.Fatal("originals-only ledger must trip the breaker")
	}
}

func TestAuditOverdueHealthyZeroNeedsBothNamespaces(t *testing.T) {
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	rep, err := AuditOverdue(context.Background(), &fakeSweepStore{}, now, 100)
	if err != nil {
		t.Fatal(err)
	}
	if rep != (OverdueReport{}) {
		t.Fatalf("empty ledger = %+v, want healthy zero", rep)
	}
	if rep.Unhealthy() {
		t.Fatal("empty ledger must not trip the breaker")
	}
}

func TestAuditOverdueRefusesBadBatch(t *testing.T) {
	if _, err := AuditOverdue(context.Background(), &fakeSweepStore{}, time.Now(), 0); err == nil {
		t.Fatal("zero batch accepted")
	}
}
