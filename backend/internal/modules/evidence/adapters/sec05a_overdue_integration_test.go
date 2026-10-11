//go:build integration

package adapters

import (
	"context"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/application"
)

// SEC05-A real-PostGIS overdue originals (AUD-06, B-BR-M04): a 48 h
// quarantine original without a deletion marker reports overdue and
// trips enforcement; fresh and marked-deleted originals stay silent.

func TestSEC05AOverdueOriginalsOnRealSQL(t *testing.T) {
	s, _ := freshStore(t)
	ctx := context.Background()
	now := time.Now()
	old := now.Add(-48 * time.Hour)
	if _, _, err := s.ReserveSession(ctx, testSessionAt("e0000000-0000-4000-8000-000000005001", "upl-old", old), openBudget()); err != nil {
		t.Fatalf("reserve old: %v", err)
	}
	if _, _, err := s.ReserveSession(ctx, testSession("e0000000-0000-4000-8000-000000005002", "upl-fresh"), openBudget()); err != nil {
		t.Fatalf("reserve fresh: %v", err)
	}

	rows, err := s.OverdueOriginals(ctx, now.Add(-application.CopyRetention), 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].QuarantineKey == "" {
		t.Fatalf("overdue originals = %+v, want exactly the 48 h row", rows)
	}

	rep, err := application.AuditOverdue(ctx, s, now, 100)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Originals != 1 || !rep.Unhealthy() {
		t.Fatalf("audit = %+v, want tripped on originals", rep)
	}

	// Marking the deletion silences the original half again.
	if err := s.MarkQuarantineDeleted(ctx, "e0000000-0000-4000-8000-000000005001"); err != nil {
		t.Fatal(err)
	}
	rep, err = application.AuditOverdue(ctx, s, now, 100)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Originals != 0 || rep.Unhealthy() {
		t.Fatalf("audit after mark = %+v, want healthy zero", rep)
	}
}
