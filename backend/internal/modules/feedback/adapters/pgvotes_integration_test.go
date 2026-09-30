//go:build integration

package adapters

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/feedback/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/feedback/domain"
)

func voteSetup(t *testing.T) (*application.Service, *pgxpool.Pool, string, string, string) {
	t.Helper()
	svc, pool := freshService(t)
	svc.Comments = svc.Store.(application.CommentStore)
	svc.Votes = svc.Store.(application.VoteStore)
	station := testUUID(9001)
	author := seedAccount(t, pool, 1, "active")
	voter := seedAccount(t, pool, 2, "active")
	return svc, pool, station, author, voter
}

func voteTarget(t *testing.T, svc *application.Service, author, station string) string {
	t.Helper()
	top, err := svc.SubmitComment(context.Background(), author, station, "GASOLINE_REGULAR", "Thread")
	if err != nil {
		t.Fatalf("SubmitComment: %v", err)
	}
	return top.ID
}

func TestPGVoteHappyAndDenominator(t *testing.T) {
	svc, pool, station, author, voter := voteSetup(t)
	ctx := context.Background()
	target := voteTarget(t, svc, author, station)

	got, err := svc.Vote(ctx, voter, target, domain.VoteValid)
	if err != nil {
		t.Fatalf("Vote: %v", err)
	}
	if !got.Created || got.Tally.Valid != 1 {
		t.Errorf("first vote must create 1/0, got %+v", got)
	}
	if _, err := svc.Vote(ctx, author, target, domain.VoteValid); !errors.Is(err, domain.ErrSelfVote) {
		t.Errorf("self-vote must refuse, got %v", err)
	}
	// Shared golden denominator: 2 valid / 1 invalid → 6666bps.
	voter2 := seedAccount(t, pool, 3, "active")
	voter3 := seedAccount(t, pool, 4, "active")
	if _, err := svc.Vote(ctx, voter2, target, domain.VoteValid); err != nil {
		t.Fatal(err)
	}
	third, err := svc.Vote(ctx, voter3, target, domain.VoteInvalid)
	if err != nil {
		t.Fatal(err)
	}
	if third.Tally.Valid != 2 || third.Tally.Invalid != 1 {
		t.Fatalf("tally must be 2/1, got %+v", third.Tally)
	}
	ag, err := third.Tally.Agreement()
	if err != nil {
		t.Fatal(err)
	}
	if ag.BasisPoints != 6666 {
		t.Errorf("2/1 must be 6666bps like the fixture, got %+v", ag)
	}
}

func TestPGVoteChangeRemoveRevision(t *testing.T) {
	svc, _, station, author, voter := voteSetup(t)
	ctx := context.Background()
	target := voteTarget(t, svc, author, station)

	if _, err := svc.Vote(ctx, voter, target, domain.VoteValid); err != nil {
		t.Fatal(err)
	}
	changed, err := svc.Vote(ctx, voter, target, domain.VoteInvalid)
	if err != nil {
		t.Fatal(err)
	}
	if changed.Created || changed.Tally.Valid != 0 || changed.Tally.Invalid != 1 {
		t.Errorf("change must rewrite to 0/1, got %+v", changed)
	}
	if err := svc.RemoveVote(ctx, voter, target); err != nil {
		t.Fatalf("RemoveVote: %v", err)
	}
	if err := svc.RemoveVote(ctx, voter, target); err != nil {
		t.Errorf("absent removal must converge no-op, got %v", err)
	}
	tally, err := svc.Tally(ctx, target)
	if err != nil {
		t.Fatal(err)
	}
	ag, _ := tally.Agreement()
	if ag.HasVotes {
		t.Errorf("emptied tally must be null agreement, got %+v", tally)
	}
	// Comment edit moves the denominator: fresh votes bind rev 2.
	if _, err := svc.EditComment(ctx, author, target, "Edited", 1); err != nil {
		t.Fatal(err)
	}
	moved, err := svc.Vote(ctx, voter, target, domain.VoteValid)
	if err != nil {
		t.Fatal(err)
	}
	if moved.Vote.CommentRevision != 2 || moved.Tally.Valid != 1 {
		t.Errorf("rev-2 tally must be 1/0, got %+v", moved)
	}
}

func TestPGConcurrentVotesConverge(t *testing.T) {
	svc, pool, station, author, _ := voteSetup(t)
	ctx := context.Background()
	target := voteTarget(t, svc, author, station)

	const voters = 8
	accounts := make([]string, voters)
	for i := range accounts {
		accounts[i] = seedAccount(t, pool, 10+i, "active")
	}
	var wg sync.WaitGroup
	errs := make([]error, voters)
	for i := 0; i < voters; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = svc.Vote(ctx, accounts[i], target, domain.VoteValid)
		}(i)
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatalf("concurrent vote: %v", err)
		}
	}
	tally, err := svc.Tally(ctx, target)
	if err != nil {
		t.Fatal(err)
	}
	if tally.Valid != voters || tally.Invalid != 0 {
		t.Errorf("tally must be %d/0, got %+v", voters, tally)
	}
}

func TestPGVoteSuspendedRefuses(t *testing.T) {
	svc, pool, station, author, _ := voteSetup(t)
	ctx := context.Background()
	target := voteTarget(t, svc, author, station)
	susp := seedAccount(t, pool, 20, "suspended")

	if _, err := svc.Vote(ctx, susp, target, domain.VoteValid); err == nil {
		t.Error("suspended vote must refuse")
	}
}
