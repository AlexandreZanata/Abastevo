//go:build integration

package adapters

import (
	"context"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/domain"
)

// SEC03-D real-PostGIS intake health (AUD-02/04): reserved, accepted
// and rejected declared buckets plus the oldest incomplete
// reservation report exact aggregates with no content attached.

func TestSEC03DIntakeHealthOnRealSQL(t *testing.T) {
	s, _ := freshStore(t)
	ctx := context.Background()
	now := time.Now()
	y, m, d := now.UTC().Date()
	dayStart := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)

	reserve := func(id, client string, bytes int64) {
		t.Helper()
		sess, _, err := domain.NewSession(domain.Params{
			ID: id, ContributorRef: "tok-c1",
			ClientSessionID: client, MIME: "image/jpeg", DeclaredBytes: bytes,
			ClaimedSHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			QuarantineKey: "q/" + client, CreatedAt: now,
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := s.ReserveSession(ctx, sess, domain.ReserveBudget{
			Now: now, DayStart: dayStart, MaxCount: 100, MaxBytes: 1 << 40,
		}); err != nil {
			t.Fatal(err)
		}
	}
	jobs := [][]byte{}
	enqueue := enqueueStub(&jobs)

	// One live reservation (reserved bucket).
	reserve("e0000000-0000-4000-8000-000000003101", "health-live", 512<<10)
	// One rejected reservation (rejected bucket).
	reserve("e0000000-0000-4000-8000-000000003102", "health-rejected", 256<<10)
	if err := s.CompleteSession(ctx, "e0000000-0000-4000-8000-000000003102", enqueue); err != nil {
		t.Fatalf("complete rejected path: %v", err)
	}
	if err := s.RecordRejected(ctx, "e0000000-0000-4000-8000-000000003102", []string{"oversize"}); err != nil {
		t.Fatalf("reject: %v", err)
	}
	// One accepted reservation (accepted bucket).
	reserve("e0000000-0000-4000-8000-000000003103", "health-ready", 128<<10)
	if err := s.CompleteSession(ctx, "e0000000-0000-4000-8000-000000003103", enqueue); err != nil {
		t.Fatalf("complete ready path: %v", err)
	}
	if _, err := s.RecordVerified(ctx, "e0000000-0000-4000-8000-000000003103", testObject("e0000000-0000-4000-8000-000000003103")); err != nil {
		t.Fatalf("verify: %v", err)
	}

	rep, err := s.IntakeHealth(ctx, dayStart, now)
	if err != nil {
		t.Fatal(err)
	}
	if rep.ReservedSessions != 1 || rep.ReservedBytes != 512<<10 {
		t.Errorf("reserved = %+v, want 1 session / 512KiB", rep)
	}
	if rep.AcceptedToday != 1 || rep.AcceptedBytes != 128<<10 {
		t.Errorf("accepted = %+v, want 1 / 128KiB", rep)
	}
	if rep.RejectedToday != 1 || rep.RejectedBytes != 256<<10 {
		t.Errorf("rejected = %+v, want 1 / 256KiB", rep)
	}
	if rep.OldestIncomplete.IsZero() || rep.OldestIncomplete.After(now) {
		t.Errorf("oldest incomplete = %v, want a live stamp", rep.OldestIncomplete)
	}
}
