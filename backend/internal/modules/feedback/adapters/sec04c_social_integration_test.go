//go:build integration

package adapters

import (
	"context"
	"errors"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/feedback/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/feedback/domain"
)

// SEC04-C social attribution on real PostGIS (AUD-05, B-BR-F02…F06):
// erasing one author tombstones their rows and rebuilds aggregates
// without deleting other authors' replies/history; training consent
// and inventory stay untouched (separate consent-withdrawal path).

func TestSEC04CSocialErasureOnRealSQL(t *testing.T) {
	svc, pool := freshService(t)
	svc.Comments = svc.Store.(application.CommentStore)
	svc.Votes = svc.Store.(application.VoteStore)
	ctx := context.Background()
	acc1 := seedAccount(t, pool, 11, "active")
	acc2 := seedAccount(t, pool, 12, "active")
	station := testUUID(9001)

	if _, err := svc.Rate(ctx, acc1, station, "GASOLINE_REGULAR", 5); err != nil {
		t.Fatalf("rate A: %v", err)
	}
	if _, err := svc.Rate(ctx, acc2, station, "GASOLINE_REGULAR", 3); err != nil {
		t.Fatalf("rate B: %v", err)
	}
	top, err := svc.SubmitComment(ctx, acc1, station, "GASOLINE_REGULAR", "voz do autor A")
	if err != nil {
		t.Fatalf("submit A: %v", err)
	}
	reply, err := svc.Reply(ctx, acc2, station, "GASOLINE_REGULAR", top.ID, "resposta do autor B")
	if err != nil {
		t.Fatalf("reply B: %v", err)
	}
	if _, err := svc.Vote(ctx, acc2, top.ID, domain.VoteValid); err != nil {
		t.Fatalf("vote B: %v", err)
	}
	var consentsBefore, derivativesBefore int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM privacy_training_consents`).Scan(&consentsBefore); err != nil {
		t.Fatalf("consent baseline: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM privacy_training_derivatives`).Scan(&derivativesBefore); err != nil {
		t.Fatalf("derivative baseline: %v", err)
	}

	report, err := svc.EraseAccount(ctx, acc1)
	if err != nil {
		t.Fatalf("EraseAccount: %v", err)
	}
	if report.Comments != 1 || report.Ratings != 1 {
		t.Fatalf("report = %+v, want A rows only", report)
	}
	// A vanishes from public reads; B's reply stays readable.
	if _, err := svc.ViewComment(ctx, top.ID); !errors.Is(err, domain.ErrCommentNotFound) {
		t.Fatalf("erased view = %v, want not-found", err)
	}
	got, err := svc.ViewComment(ctx, reply.ID)
	if err != nil || got.Text != "resposta do autor B" {
		t.Fatalf("surviving reply = %+v, %v", got, err)
	}
	// Aggregates rebuild to B only.
	stats, found, err := svc.Store.Stats(ctx, station, "GASOLINE_REGULAR")
	if err != nil || !found || stats.Count != 1 || stats.Sum != 3 {
		t.Fatalf("stats = %+v found=%v err=%v, want 1/3", stats, found, err)
	}
	// B's vote on the erased comment stays (denominator survives).
	tally, found, err := svc.Votes.Tally(ctx, top.ID, top.Revision)
	if err != nil || !found || tally.Valid != 1 {
		t.Fatalf("tally = %+v found=%v err=%v, want surviving vote", tally, found, err)
	}
	// Training inventory untouched: consent withdrawal rides its own
	// revocation path, never operational erasure.
	var consentsAfter, derivativesAfter int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM privacy_training_consents`).Scan(&consentsAfter); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM privacy_training_derivatives`).Scan(&derivativesAfter); err != nil {
		t.Fatal(err)
	}
	if consentsAfter != consentsBefore || derivativesAfter != derivativesBefore {
		t.Fatalf("training touched by social erasure: consents %d->%d derivatives %d->%d",
			consentsBefore, consentsAfter, derivativesBefore, derivativesAfter)
	}
	// Second run converges with zero new work.
	second, err := svc.EraseAccount(ctx, acc1)
	if err != nil {
		t.Fatalf("re-erase: %v", err)
	}
	if second.Comments != 0 || second.Ratings != 0 {
		t.Fatalf("re-erase = %+v, want zeros", second)
	}
}
