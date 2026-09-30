package application

import (
	"context"
	"errors"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/feedback/domain"
)

func testVoteService(gate AccountGate) (*Service, *MemStore) {
	store := NewMemStore()
	var n int
	return &Service{
		Clock:         &fakeClock{now: 1_700_000_000},
		Store:         store,
		Comments:      store,
		Votes:         store,
		CheckAccount:  gate,
		StationExists: knownStations,
		IDGen: func() (string, error) {
			n++
			return "vote-id", nil
		},
	}, store
}

func voteComment(t *testing.T, svc *Service, author, text string) string {
	t.Helper()
	top, err := svc.SubmitComment(context.Background(), author, "station-1", "GASOLINE_REGULAR", text)
	if err != nil {
		t.Fatalf("SubmitComment: %v", err)
	}
	return top.ID
}

func TestVoteHappyAndTallyExact(t *testing.T) {
	svc, _ := testVoteService(allowAll)
	ctx := context.Background()
	target := voteComment(t, svc, "author-1", "Thread")

	got, err := svc.Vote(ctx, "voter-1", target, domain.VoteValid)
	if err != nil {
		t.Fatalf("Vote: %v", err)
	}
	if !got.Created || got.Vote.Choice != domain.VoteValid || got.Vote.CommentRevision != 1 {
		t.Errorf("first vote must create on revision 1: %+v", got.Vote)
	}
	if got.Tally.Valid != 1 || got.Tally.Invalid != 0 {
		t.Errorf("tally must be 1/0, got %+v", got.Tally)
	}
	ag, err := got.Tally.Agreement()
	if err != nil {
		t.Fatal(err)
	}
	if !ag.HasVotes || ag.BasisPoints != 10000 {
		t.Errorf("unanimous agreement must be 10000, got %+v", ag)
	}

	second, err := svc.Vote(ctx, "voter-2", target, domain.VoteInvalid)
	if err != nil {
		t.Fatal(err)
	}
	if second.Tally.Valid != 1 || second.Tally.Invalid != 1 {
		t.Errorf("tally must be 1/1, got %+v", second.Tally)
	}
	ag, _ = second.Tally.Agreement()
	if ag.BasisPoints != 5000 {
		t.Errorf("1/1 must be 5000bps, got %+v", ag)
	}
}

func TestVoteSelfChangeRemove(t *testing.T) {
	svc, _ := testVoteService(allowAll)
	ctx := context.Background()
	target := voteComment(t, svc, "author-1", "Thread")

	if _, err := svc.Vote(ctx, "author-1", target, domain.VoteValid); !errors.Is(err, domain.ErrSelfVote) {
		t.Errorf("self-vote must refuse, got %v", err)
	}
	if _, err := svc.Vote(ctx, "voter-1", target, "MAYBE"); !errors.Is(err, domain.ErrVoteChoiceInvalid) {
		t.Errorf("bad choice must refuse, got %v", err)
	}
	first, err := svc.Vote(ctx, "voter-1", target, domain.VoteValid)
	if err != nil {
		t.Fatal(err)
	}
	again, err := svc.Vote(ctx, "voter-1", target, domain.VoteValid)
	if err != nil {
		t.Fatal(err)
	}
	if again.Created || again.Vote.ID != first.Vote.ID {
		t.Errorf("equal choice must converge, got %+v", again.Vote)
	}
	changed, err := svc.Vote(ctx, "voter-1", target, domain.VoteInvalid)
	if err != nil {
		t.Fatal(err)
	}
	if changed.Created || changed.Vote.Choice != domain.VoteInvalid {
		t.Errorf("changed choice must rewrite, got %+v", changed.Vote)
	}
	if changed.Tally.Valid != 0 || changed.Tally.Invalid != 1 {
		t.Errorf("tally must be 0/1, got %+v", changed.Tally)
	}
	if err := svc.RemoveVote(ctx, "voter-1", target); err != nil {
		t.Fatalf("RemoveVote: %v", err)
	}
	if err := svc.RemoveVote(ctx, "voter-1", target); err != nil {
		t.Errorf("absent removal must converge no-op, got %v", err)
	}
	tally, err := svc.Tally(ctx, target)
	if err != nil {
		t.Fatal(err)
	}
	if tally.Valid != 0 || tally.Invalid != 0 {
		t.Errorf("tally must be empty, got %+v", tally)
	}
	ag, _ := tally.Agreement()
	if ag.HasVotes {
		t.Errorf("empty tally must be null agreement, got %+v", ag)
	}
}

func TestVoteRevisionMoveOnEdit(t *testing.T) {
	svc, _ := testVoteService(allowAll)
	ctx := context.Background()
	target := voteComment(t, svc, "author-1", "Original")

	if _, err := svc.Vote(ctx, "voter-1", target, domain.VoteValid); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.EditComment(ctx, "author-1", target, "Edited", 1); err != nil {
		t.Fatal(err)
	}
	// Old-revision votes stay audit: the new denominator starts empty
	// and fresh votes bind revision 2.
	fresh, err := svc.Tally(ctx, target)
	if err != nil {
		t.Fatal(err)
	}
	if fresh.Revision != 2 || fresh.Valid != 0 {
		t.Errorf("new revision tally must start empty, got %+v", fresh)
	}
	moved, err := svc.Vote(ctx, "voter-1", target, domain.VoteInvalid)
	if err != nil {
		t.Fatal(err)
	}
	if moved.Vote.CommentRevision != 2 {
		t.Errorf("vote must bind the current revision, got %+v", moved.Vote)
	}
	if moved.Tally.Invalid != 1 {
		t.Errorf("revision-2 tally must be 0/1, got %+v", moved.Tally)
	}
}

func TestVoteGateAndTarget(t *testing.T) {
	ctx := context.Background()
	denied := errors.New("feedback: suspended test")
	svc, _ := testVoteService(func(context.Context, string) error { return denied })
	if _, err := svc.Vote(ctx, "voter-1", "missing", domain.VoteValid); !errors.Is(err, domain.ErrAuthorForbidden) {
		t.Errorf("gate failure must map to author-forbidden, got %v", err)
	}
	if _, err := svc.Vote(ctx, "voter-1", "missing", "MAYBE"); !errors.Is(err, domain.ErrVoteChoiceInvalid) {
		t.Errorf("choice validation precedes target reads, got %v", err)
	}
	svc, _ = testVoteService(allowAll)
	if _, err := svc.Vote(ctx, "voter-1", "missing", domain.VoteValid); !errors.Is(err, domain.ErrCommentNotFound) {
		t.Errorf("missing target must be not-found, got %v", err)
	}
	if _, err := svc.Tally(ctx, "missing"); !errors.Is(err, domain.ErrCommentNotFound) {
		t.Errorf("missing tally must be not-found, got %v", err)
	}
	bare := &Service{Clock: &fakeClock{now: 1_700_000_000}, Store: NewMemStore(), Comments: NewMemStore(), Votes: NewMemStore()}
	if _, err := bare.Vote(ctx, "voter-1", "x", domain.VoteValid); !errors.Is(err, domain.ErrGateRequired) {
		t.Errorf("missing ports must fail closed, got %v", err)
	}
}
