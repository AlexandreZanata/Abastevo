package application

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/community/domain"
)

// BUC-CS02: an active free account marks helpful, changes/removes and
// recovers a lost response without a second vote; another account or
// device cannot act as owner. B-BR-CS01: self-votes refuse, stale
// revisions never transfer, tallies stay exact.

type fakeHelpfulnessStore struct {
	mu        sync.Mutex
	target    HelpfulnessTarget
	targetErr error
	votes     map[string]domain.StoredVote // contributor|observation|sequence
	bySub     map[string]string            // client submission -> key
	tally     domain.Tally
	casts     int
}

func helpfulKey(contrib, obs string, seq int64) string {
	return contrib + "|" + obs + "|" + strconv.FormatInt(seq, 10)
}

func newFakeHelpfulnessStore() *fakeHelpfulnessStore {
	return &fakeHelpfulnessStore{
		target: HelpfulnessTarget{
			ObservationID:  "obs-1",
			AuthorRef:      "tok-contrib-author",
			State:          domain.StateValidated,
			LatestSequence: 3,
		},
		votes: map[string]domain.StoredVote{},
		bySub: map[string]string{},
		tally: domain.Tally{ObservationID: "obs-1", DecisionSequence: 3},
	}
}

func (f *fakeHelpfulnessStore) Target(_ context.Context, id string) (HelpfulnessTarget, error) {
	if f.targetErr != nil {
		return HelpfulnessTarget{}, f.targetErr
	}
	if id != f.target.ObservationID {
		return HelpfulnessTarget{}, ErrTargetNotFound
	}
	return f.target, nil
}

func (f *fakeHelpfulnessStore) CastVote(_ context.Context, rec domain.StoredVote) (domain.StoredVote, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.casts++
	if key, ok := f.bySub[rec.ContributorRef+"|"+rec.ClientSubmissionID]; ok {
		return f.votes[key], false, nil
	}
	key := helpfulKey(rec.ContributorRef, rec.ObservationID, rec.DecisionSequence)
	if existing, ok := f.votes[key]; ok {
		existing.Choice = rec.Choice
		f.votes[key] = existing
		f.bySub[rec.ContributorRef+"|"+rec.ClientSubmissionID] = key
		f.recount()
		return existing, false, nil
	}
	f.votes[key] = rec
	f.bySub[rec.ContributorRef+"|"+rec.ClientSubmissionID] = key
	f.recount()
	return rec, true, nil
}

func (f *fakeHelpfulnessStore) RemoveVote(_ context.Context, contrib, obs string, seq int64) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	key := helpfulKey(contrib, obs, seq)
	if _, ok := f.votes[key]; !ok {
		return false, nil
	}
	delete(f.votes, key)
	f.recount()
	return true, nil
}

func (f *fakeHelpfulnessStore) Tally(_ context.Context, obs string, seq int64) (domain.Tally, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if obs != f.tally.ObservationID || seq != f.tally.DecisionSequence {
		return domain.Tally{}, false, nil
	}
	return f.tally, true, nil
}

func (f *fakeHelpfulnessStore) MyVote(_ context.Context, contrib, obs string, seq int64) (domain.Choice, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	v, ok := f.votes[helpfulKey(contrib, obs, seq)]
	if !ok || !v.Live() {
		return "", false, nil
	}
	return v.Choice, true, nil
}

func (f *fakeHelpfulnessStore) recount() {
	var h, n int64
	for _, v := range f.votes {
		if !v.Live() {
			continue
		}
		if v.Choice == domain.ChoiceHelpful {
			h++
		} else {
			n++
		}
	}
	f.tally.Helpful, f.tally.NotHelpful = h, n
}

func helpfulPorts(store *fakeHelpfulnessStore) HelpfulnessPorts {
	n := 0
	return HelpfulnessPorts{
		CheckQuota:   func(_ context.Context, _, _ string) (time.Duration, error) { return 0, nil },
		Clock:        func() time.Time { return time.Unix(1_700_000_000, 0).UTC() },
		NewID:        func() (string, error) { n++; return "vote-" + strconv.Itoa(n), nil },
		CheckAccount: func(_ context.Context, _ string) error { return nil },
		Store:        store,
	}
}

func helpfulCaller() Caller {
	return helpfulCallerAs("contrib-a")
}

func helpfulCallerAs(ref string) Caller {
	return Caller{ContributorID: ref, Fingerprint: "fp", KeyID: "k", Token: "tok-" + ref}
}

func helpfulDTO() CastDTO {
	return CastDTO{
		ObservationID: "obs-1", DecisionSequence: 3,
		Choice: domain.ChoiceHelpful, ClientSubmissionID: "sub-1",
	}
}

func TestCastHelpfulCreatesVoteWithExactTally(t *testing.T) {
	store := newFakeHelpfulnessStore()
	res, err := Cast(context.Background(), helpfulPorts(store), helpfulCaller(), helpfulDTO())
	if err != nil {
		t.Fatalf("cast must succeed: %v", err)
	}
	if !res.Created || res.Replayed {
		t.Fatalf("first cast must be created, not replayed: %+v", res)
	}
	if res.Tally.Helpful != 1 || res.Tally.NotHelpful != 0 {
		t.Fatalf("tally must be 1/0: %+v", res.Tally)
	}
	if res.MyChoice != domain.ChoiceHelpful {
		t.Fatalf("owner overlay must echo the choice: %+v", res)
	}
	if !res.Share.HasVotes || res.Share.BasisPoints != 10000 {
		t.Fatalf("unanimous share must be 10000bp: %+v", res.Share)
	}
}

func TestCastRefusesSelfVoteBeforeMutation(t *testing.T) {
	store := newFakeHelpfulnessStore()
	caller := helpfulCallerAs("contrib-author")
	if _, err := Cast(context.Background(), helpfulPorts(store), caller, helpfulDTO()); err != domain.ErrSelfVote {
		t.Fatalf("self-vote must refuse with ErrSelfVote, got %v", err)
	}
	if store.casts != 0 {
		t.Fatal("self-vote must never reach the store")
	}
}

func TestCastRefusesInactiveAccount(t *testing.T) {
	store := newFakeHelpfulnessStore()
	ports := helpfulPorts(store)
	ports.CheckAccount = func(_ context.Context, _ string) error { return errors.New("suspended") }
	if _, err := Cast(context.Background(), ports, helpfulCaller(), helpfulDTO()); err != ErrAccountBlocked {
		t.Fatalf("inactive account must map onto ErrAccountBlocked, got %v", err)
	}
	if store.casts != 0 {
		t.Fatal("blocked account must never reach the store")
	}
}

func TestCastRefusesUnknownAndIneligibleTargets(t *testing.T) {
	store := newFakeHelpfulnessStore()
	dto := helpfulDTO()
	dto.ObservationID = "obs-missing"
	if _, err := Cast(context.Background(), helpfulPorts(store), helpfulCaller(), dto); err != ErrTargetNotFound {
		t.Fatalf("unknown target must 404, got %v", err)
	}
	for _, state := range []struct {
		name    string
		mutate  func(*HelpfulnessTarget)
		wantErr error
	}{
		{"unvalidated", func(t *HelpfulnessTarget) { t.State = domain.StateReceived }, ErrIneligibleTarget},
		{"rejected", func(t *HelpfulnessTarget) { t.State = domain.StateRejected }, ErrIneligibleTarget},
		{"hidden", func(t *HelpfulnessTarget) { t.Hidden = true }, ErrIneligibleTarget},
		{"deleted", func(t *HelpfulnessTarget) { t.Deleted = true }, ErrIneligibleTarget},
	} {
		store := newFakeHelpfulnessStore()
		state.mutate(&store.target)
		if _, err := Cast(context.Background(), helpfulPorts(store), helpfulCaller(), helpfulDTO()); err != state.wantErr {
			t.Fatalf("%s target must refuse with %v, got %v", state.name, state.wantErr, err)
		}
	}
}

func TestCastRefusesStaleSequence(t *testing.T) {
	store := newFakeHelpfulnessStore()
	dto := helpfulDTO()
	dto.DecisionSequence = 2 // target moved on to 3
	if _, err := Cast(context.Background(), helpfulPorts(store), helpfulCaller(), dto); err != ErrStaleSequence {
		t.Fatalf("stale revision must refuse with ErrStaleSequence, got %v", err)
	}
	if store.casts != 0 {
		t.Fatal("stale vote must never reach the store")
	}
}

func TestChangeChoiceRewritesInPlace(t *testing.T) {
	store := newFakeHelpfulnessStore()
	ports := helpfulPorts(store)
	caller := helpfulCaller()
	first, err := Cast(context.Background(), ports, caller, helpfulDTO())
	if err != nil {
		t.Fatalf("first cast must succeed: %v", err)
	}
	flip := helpfulDTO()
	flip.Choice = domain.ChoiceNotHelpful
	flip.ClientSubmissionID = "sub-2"
	second, err := Cast(context.Background(), ports, caller, flip)
	if err != nil {
		t.Fatalf("change must succeed: %v", err)
	}
	if second.Created {
		t.Fatal("change must rewrite, not insert a second vote")
	}
	if second.Vote.ID != first.Vote.ID {
		t.Fatal("change must keep one vote identity per account+target+revision")
	}
	if second.Tally.Helpful != 0 || second.Tally.NotHelpful != 1 {
		t.Fatalf("tally must be 0/1 after change: %+v", second.Tally)
	}
}

func TestRetryWithSameSubmissionConverges(t *testing.T) {
	store := newFakeHelpfulnessStore()
	ports := helpfulPorts(store)
	caller := helpfulCaller()
	first, err := Cast(context.Background(), ports, caller, helpfulDTO())
	if err != nil {
		t.Fatalf("first cast must succeed: %v", err)
	}
	second, err := Cast(context.Background(), ports, caller, helpfulDTO())
	if err != nil {
		t.Fatalf("lost-response retry must succeed: %v", err)
	}
	if second.Created || !second.Replayed || second.Vote.ID != first.Vote.ID {
		t.Fatalf("retry must replay the same vote: %+v", second)
	}
	if second.Tally.Helpful != 1 {
		t.Fatalf("retry must not inflate totals: %+v", second.Tally)
	}
}

func TestRemoveIsIdempotentAndClearsOverlay(t *testing.T) {
	store := newFakeHelpfulnessStore()
	ports := helpfulPorts(store)
	caller := helpfulCaller()
	if _, err := Cast(context.Background(), ports, caller, helpfulDTO()); err != nil {
		t.Fatalf("cast must succeed: %v", err)
	}
	if err := Remove(context.Background(), ports, caller, "obs-1", 3); err != nil {
		t.Fatalf("remove must succeed: %v", err)
	}
	if err := Remove(context.Background(), ports, caller, "obs-1", 3); err != nil {
		t.Fatalf("second remove must converge as no-op success: %v", err)
	}
	got, err := TallyFor(context.Background(), ports, caller, "obs-1", 3)
	if err != nil {
		t.Fatalf("tally must succeed: %v", err)
	}
	if got.Share.HasVotes {
		t.Fatalf("removed vote must leave zero totals: %+v", got)
	}
	if got.MyChoice != nil {
		t.Fatalf("removed vote must clear the owner overlay: %+v", got)
	}
}

func TestInvalidChoiceNeverMutates(t *testing.T) {
	store := newFakeHelpfulnessStore()
	dto := helpfulDTO()
	dto.Choice = "VALID"
	if _, err := Cast(context.Background(), helpfulPorts(store), helpfulCaller(), dto); err != domain.ErrVoteChoiceInvalid {
		t.Fatalf("VALID must refuse as helpfulness choice, got %v", err)
	}
	if store.casts != 0 {
		t.Fatal("invalid choice must never reach the store")
	}
}

func TestCastQuotaDenialFailsBeforeMutation(t *testing.T) {
	store := newFakeHelpfulnessStore()
	ports := helpfulPorts(store)
	ports.CheckQuota = func(_ context.Context, subject, operation string) (time.Duration, error) {
		if subject != "fp" || operation != "write" {
			t.Errorf("quota subject = %q op = %q, want fp/write", subject, operation)
		}
		return 90 * time.Second, &QuotaDeniedError{RetryAfter: 90 * time.Second}
	}
	_, err := Cast(context.Background(), ports, helpfulCaller(), helpfulDTO())
	var denied *QuotaDeniedError
	if !errors.As(err, &denied) || denied.RetryAfter != 90*time.Second {
		t.Fatalf("quota denial must surface with retry delay, got %v", err)
	}
	if store.casts != 0 {
		t.Fatal("denied vote must never reach the store")
	}
}

func TestSameAccountAcrossDeviceKeysConverges(t *testing.T) {
	store := newFakeHelpfulnessStore()
	ports := helpfulPorts(store)
	// One account, two device keys: the stable attribution token binds
	// both to a single vote (tablet/kiosk rule: devices never mint
	// separate votes for one account).
	firstKey := Caller{ContributorID: "contrib-a", Fingerprint: "fp-phone", KeyID: "k1", Token: "tok-contrib-a"}
	secondKey := Caller{ContributorID: "contrib-a", Fingerprint: "fp-tablet", KeyID: "k2", Token: "tok-contrib-a"}
	first, err := Cast(context.Background(), ports, firstKey, helpfulDTO())
	if err != nil {
		t.Fatalf("first key cast must succeed: %v", err)
	}
	second, err := Cast(context.Background(), ports, secondKey, helpfulDTO())
	if err != nil {
		t.Fatalf("second key cast must succeed: %v", err)
	}
	if second.Created || second.Vote.ID != first.Vote.ID {
		t.Fatalf("second device key must converge onto one vote: %+v", second)
	}
	if second.Tally.Helpful != 1 {
		t.Fatalf("one account holds one vote: %+v", second.Tally)
	}
}

func TestSharedDeviceKeepsAccountsSeparate(t *testing.T) {
	store := newFakeHelpfulnessStore()
	ports := helpfulPorts(store)
	// One shared fingerprint, two accounts: votes stay separate even
	// though the write quota subject is shared.
	a := Caller{ContributorID: "contrib-a", Fingerprint: "fp-kiosk", KeyID: "ka", Token: "tok-contrib-a"}
	b := Caller{ContributorID: "contrib-b", Fingerprint: "fp-kiosk", KeyID: "kb", Token: "tok-contrib-b"}
	dto := helpfulDTO()
	if _, err := Cast(context.Background(), ports, a, dto); err != nil {
		t.Fatalf("account A must succeed: %v", err)
	}
	dto.ClientSubmissionID = "sub-b"
	res, err := Cast(context.Background(), ports, b, dto)
	if err != nil {
		t.Fatalf("account B must succeed: %v", err)
	}
	if !res.Created || res.Tally.Helpful != 2 {
		t.Fatalf("kiosk accounts hold separate votes: %+v", res)
	}
}

type stubReviewLookup struct {
	open bool
	err  error
	seen []string
}

func (s *stubReviewLookup) OpenReview(_ context.Context, targetType, targetID string) (bool, error) {
	s.seen = append(s.seen, targetType+"|"+targetID)
	return s.open, s.err
}

func TestTallyAnnotatesOpenReview(t *testing.T) {
	store := newFakeHelpfulnessStore()
	ports := helpfulPorts(store)
	lookup := &stubReviewLookup{open: true}
	ports.Reviews = lookup
	got, err := TallyFor(context.Background(), ports, helpfulCaller(), "obs-1", 3)
	if err != nil {
		t.Fatalf("tally must succeed: %v", err)
	}
	if !got.UnderReview {
		t.Fatal("open review must annotate the tally")
	}
	if len(lookup.seen) != 1 || lookup.seen[0] != ReviewTargetContribution+"|obs-1" {
		t.Fatalf("lookup saw %v, want pinned CONTRIBUTION spelling", lookup.seen)
	}
}

func TestTallyCleanWithoutReview(t *testing.T) {
	store := newFakeHelpfulnessStore()
	ports := helpfulPorts(store)
	ports.Reviews = &stubReviewLookup{open: false}
	got, err := TallyFor(context.Background(), ports, helpfulCaller(), "obs-1", 3)
	if err != nil || got.UnderReview {
		t.Fatalf("clean tally = %+v %v", got, err)
	}
	// Nil lookup keeps older wiring working with no annotation.
	ports.Reviews = nil
	got, err = TallyFor(context.Background(), ports, helpfulCaller(), "obs-1", 3)
	if err != nil || got.UnderReview {
		t.Fatalf("nil lookup tally = %+v %v", got, err)
	}
}

func TestTallyReviewLookupFailureClosesRead(t *testing.T) {
	store := newFakeHelpfulnessStore()
	ports := helpfulPorts(store)
	ports.Reviews = &stubReviewLookup{err: errors.New("queue down")}
	if _, err := TallyFor(context.Background(), ports, helpfulCaller(), "obs-1", 3); err != ErrReviewUnavailable {
		t.Fatalf("lookup failure = %v, want ErrReviewUnavailable", err)
	}
}
