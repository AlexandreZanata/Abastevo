package application

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/community/domain"
)

// fakeVoteStore mimics the atomic SQL contract: natural-key convergence,
// divergent-payload conflicts, pair-duplicate refusals and open-triple
// report dedup, with one consensus job per accepted write.
type fakeVoteStore struct {
	obs         map[string]domain.Observation
	decisions   map[string][]domain.Decision
	confirmByID map[string]domain.Confirmation
	disputes    map[string]domain.Dispute
	jobs        []string
}

func newFakeVoteStore() *fakeVoteStore {
	return &fakeVoteStore{
		obs:         map[string]domain.Observation{},
		decisions:   map[string][]domain.Decision{},
		confirmByID: map[string]domain.Confirmation{},
		disputes:    map[string]domain.Dispute{},
	}
}

func validatedObs(author string, received time.Time) domain.Observation {
	obs, _, err := domain.NewObservation(domain.Params{
		ID: "d6c74c23-63db-4c24-a2e5-408cb23bad27", ContributorRef: author,
		ClientSubmissionID: "sub-1", StationID: "d6c74c23-63db-4c24-a2e5-408cb23bad26",
		Product: "GASOLINE_REGULAR", Unit: "L", AmountMilli: 5999,
		RawText: "5,999", ConditionKind: "STANDARD",
		ClaimedCapturedAt: received.Add(-time.Hour), ReceivedAt: received,
	})
	if err != nil {
		panic(err)
	}
	return obs
}

func validatedDecisions(obs domain.Observation, at time.Time) []domain.Decision {
	claim, err := domain.ClaimValidation(obs, domain.StateReceived, "job-1", domain.ActorWorker, at, 1)
	if err != nil {
		panic(err)
	}
	admit, err := domain.Admit(obs, domain.StateValidating, domain.ActorWorker, at, 2)
	if err != nil {
		panic(err)
	}
	return []domain.Decision{claim, admit}
}

func (f *fakeVoteStore) Observation(_ context.Context, id string) (domain.Observation, error) {
	o, ok := f.obs[id]
	if !ok {
		return domain.Observation{}, errors.New("adapters: unknown observation")
	}
	return o, nil
}

func (f *fakeVoteStore) Decisions(_ context.Context, id string) ([]domain.Decision, error) {
	return append([]domain.Decision{}, f.decisions[id]...), nil
}

func (f *fakeVoteStore) Confirm(_ context.Context, c domain.Confirmation, enqueue func(ctx context.Context, tx pgx.Tx, kind string, payload []byte, dedupe string) error) (string, bool, error) {
	for _, prev := range f.confirmByID {
		if prev.ContributorRef == c.ContributorRef && prev.ClientSubmissionID == c.ClientSubmissionID {
			if prev.ObservationID != c.ObservationID {
				return "", false, domain.ErrConflict
			}
			return prev.ID, true, nil
		}
		if prev.ObservationID == c.ObservationID && prev.ContributorRef == c.ContributorRef {
			return "", false, domain.ErrAlreadyConfirmed
		}
	}
	f.confirmByID[c.ID] = c
	if enqueue != nil {
		_ = enqueue(context.Background(), nil, "community-consensus", []byte(`{}`), "consensus:"+c.ObservationID)
		f.jobs = append(f.jobs, "consensus:"+c.ObservationID)
	}
	return c.ID, false, nil
}

func (f *fakeVoteStore) Report(_ context.Context, d domain.Dispute, enqueue func(ctx context.Context, tx pgx.Tx, kind string, payload []byte, dedupe string) error) (string, bool, error) {
	for _, prev := range f.disputes {
		if prev.ContributorRef == d.ContributorRef && prev.ClientSubmissionID == d.ClientSubmissionID {
			if prev.TargetObservationID != d.TargetObservationID || prev.Reason != d.Reason {
				return "", false, domain.ErrConflict
			}
			return prev.ID, true, nil
		}
		if prev.ContributorRef == d.ContributorRef && prev.TargetObservationID == d.TargetObservationID &&
			prev.Reason == d.Reason && prev.Status == domain.DisputeOpen {
			return prev.ID, true, nil
		}
	}
	f.disputes[d.ID] = d
	if enqueue != nil {
		_ = enqueue(context.Background(), nil, "community-consensus", []byte(`{}`), "consensus:"+d.TargetObservationID)
		f.jobs = append(f.jobs, "consensus:"+d.TargetObservationID)
	}
	return d.ID, false, nil
}

func votePorts(store *fakeVoteStore, now time.Time) VotePorts {
	var seq atomic.Int64
	return VotePorts{
		Clock: func() time.Time { return now },
		NewID: func() (string, error) {
			return fmt.Sprintf("c6c74c23-63db-4c24-a2e5-408cb23bad%02d", seq.Add(1)), nil
		},
		CheckQuota: func(context.Context, string, string) (time.Duration, error) {
			return 0, nil
		},
		EnqueueJob: func(context.Context, pgx.Tx, string, []byte, string) error { return nil },
		Store:      store,
	}
}

func voteCaller() Caller {
	return Caller{ContributorID: "c2", Fingerprint: "fp:y", KeyID: "k2", Token: "tok-c2"}
}

func TestConfirmHappyPathEnqueuesRecompute(t *testing.T) {
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	store := newFakeVoteStore()
	obs := validatedObs("tok-c1", now.Add(-time.Hour))
	store.obs[obs.ID] = obs
	store.decisions[obs.ID] = validatedDecisions(obs, now.Add(-30*time.Minute))
	res, err := Confirm(context.Background(), votePorts(store, now), voteCaller(), ConfirmDTO{
		ObservationID: obs.ID, ClientSubmissionID: "cfm-1",
	})
	if err != nil {
		t.Fatalf("confirm = %v", err)
	}
	if res.Replayed || res.ConfirmationID == "" {
		t.Errorf("result = %+v", res)
	}
	if len(store.jobs) != 1 || store.jobs[0] != "consensus:"+obs.ID {
		t.Errorf("jobs = %v, want exactly the recompute intent", store.jobs)
	}
	again, err := Confirm(context.Background(), votePorts(store, now), voteCaller(), ConfirmDTO{
		ObservationID: obs.ID, ClientSubmissionID: "cfm-1",
	})
	if err != nil || !again.Replayed || again.ConfirmationID != res.ConfirmationID {
		t.Fatalf("retry = %+v, %v", again, err)
	}
	if len(store.jobs) != 1 {
		t.Errorf("retry enqueued again: %v", store.jobs)
	}
}

func TestConfirmRefusesSelfDuplicatesAndBadTargets(t *testing.T) {
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	store := newFakeVoteStore()
	obs := validatedObs("tok-c1", now.Add(-time.Hour))
	store.obs[obs.ID] = obs
	store.decisions[obs.ID] = validatedDecisions(obs, now.Add(-30*time.Minute))
	ports := votePorts(store, now)

	self := voteCaller()
	self.ContributorID, self.Token = "c1", "tok-c1"
	if _, err := Confirm(context.Background(), ports, self, ConfirmDTO{ObservationID: obs.ID, ClientSubmissionID: "cfm-1"}); !errors.Is(err, domain.ErrSelfConfirmation) {
		t.Errorf("self-confirm = %v", err)
	}
	if _, err := Confirm(context.Background(), ports, voteCaller(), ConfirmDTO{ObservationID: obs.ID, ClientSubmissionID: "cfm-1"}); err != nil {
		t.Fatal(err)
	}
	// Same pair, fresh client key: still one vote, never amplified.
	if _, err := Confirm(context.Background(), ports, voteCaller(), ConfirmDTO{ObservationID: obs.ID, ClientSubmissionID: "cfm-2"}); !errors.Is(err, domain.ErrAlreadyConfirmed) {
		t.Errorf("duplicate support = %v", err)
	}
	// Same client key, divergent target: idempotency conflict.
	other := validatedObs("tok-c1", now.Add(-time.Hour))
	other.ID = "d6c74c23-63db-4c24-a2e5-408cb23bad28"
	other.ClientSubmissionID = "sub-2"
	store.obs[other.ID] = other
	store.decisions[other.ID] = validatedDecisions(other, now.Add(-30*time.Minute))
	if _, err := Confirm(context.Background(), ports, voteCaller(), ConfirmDTO{ObservationID: other.ID, ClientSubmissionID: "cfm-1"}); !errors.Is(err, ErrConflict) {
		t.Errorf("divergent retry = %v", err)
	}
	// Missing target shares the not-found shape.
	if _, err := Confirm(context.Background(), ports, voteCaller(), ConfirmDTO{ObservationID: "missing", ClientSubmissionID: "cfm-9"}); !errors.Is(err, ErrTargetNotFound) {
		t.Errorf("missing target = %v", err)
	}
}

func TestConfirmRequiresEligibleTarget(t *testing.T) {
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	ports := votePorts(newFakeVoteStore(), now)

	received := validatedObs("tok-c1", now.Add(-time.Hour))
	ports.Store.(*fakeVoteStore).obs[received.ID] = received
	if _, err := Confirm(context.Background(), ports, voteCaller(), ConfirmDTO{ObservationID: received.ID, ClientSubmissionID: "cfm-1"}); !errors.Is(err, ErrIneligibleTarget) {
		t.Errorf("unvalidated target = %v", err)
	}

	stale := validatedObs("tok-c1", now.Add(-50*time.Hour))
	ports.Store.(*fakeVoteStore).obs[stale.ID] = stale
	ports.Store.(*fakeVoteStore).decisions[stale.ID] = validatedDecisions(stale, now.Add(-49*time.Hour))
	if _, err := Confirm(context.Background(), ports, voteCaller(), ConfirmDTO{ObservationID: stale.ID, ClientSubmissionID: "cfm-1"}); !errors.Is(err, ErrIneligibleTarget) {
		t.Errorf("stale target = %v", err)
	}
}

func TestDisputeDedupesOpenReports(t *testing.T) {
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	store := newFakeVoteStore()
	obs := validatedObs("tok-c1", now.Add(-time.Hour))
	store.obs[obs.ID] = obs
	store.decisions[obs.ID] = validatedDecisions(obs, now.Add(-30*time.Minute))
	ports := votePorts(store, now)
	intent := DisputeDTO{
		TargetObservationID: obs.ID, ClientSubmissionID: "dsp-1",
		Reason: domain.ReasonWrongProduct,
	}
	first, err := Dispute(context.Background(), ports, voteCaller(), intent)
	if err != nil {
		t.Fatalf("dispute = %v", err)
	}
	if first.State != domain.DisputeOpen || first.Replayed {
		t.Errorf("result = %+v", first)
	}
	if len(store.jobs) != 1 {
		t.Errorf("jobs = %v, want the recompute intent", store.jobs)
	}
	// Active repeated report converges without error or extra work.
	second, err := Dispute(context.Background(), ports, voteCaller(), intent)
	if err != nil || !second.Replayed || second.DisputeID != first.DisputeID {
		t.Fatalf("repeat = %+v, %v", second, err)
	}
	if len(store.jobs) != 1 {
		t.Errorf("repeat enqueued again: %v", store.jobs)
	}
	// A different reason is a different report.
	other := intent
	other.ClientSubmissionID = "dsp-2"
	other.Reason = domain.ReasonEvidenceMismatch
	third, err := Dispute(context.Background(), ports, voteCaller(), other)
	if err != nil || third.Replayed || third.DisputeID == first.DisputeID {
		t.Fatalf("other reason = %+v, %v", third, err)
	}
}

func TestDisputeRejectsBadTargets(t *testing.T) {
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	store := newFakeVoteStore()
	ports := votePorts(store, now)
	intent := DisputeDTO{TargetObservationID: "missing", ClientSubmissionID: "dsp-1", Reason: domain.ReasonOther}
	if _, err := Dispute(context.Background(), ports, voteCaller(), intent); !errors.Is(err, ErrTargetNotFound) {
		t.Errorf("missing target = %v", err)
	}
	// Rejected facts take no reports: disputing them is noise.
	obs := validatedObs("tok-c1", now.Add(-time.Hour))
	store.obs[obs.ID] = obs
	claim, _ := domain.ClaimValidation(obs, domain.StateReceived, "job-1", domain.ActorWorker, now, 1)
	reject, _ := domain.Reject(obs, domain.StateValidating, domain.ActorWorker, []string{"invalid-station"}, now, 2)
	store.decisions[obs.ID] = []domain.Decision{claim, reject}
	intent.TargetObservationID = obs.ID
	if _, err := Dispute(context.Background(), ports, voteCaller(), intent); !errors.Is(err, ErrIneligibleTarget) {
		t.Errorf("rejected target = %v", err)
	}
	// Unknown reasons and self-replacements fail before any write.
	intent.Reason = "NOPE"
	if _, err := Dispute(context.Background(), ports, voteCaller(), intent); !errors.Is(err, domain.ErrUnknownReason) {
		t.Errorf("unknown reason = %v", err)
	}
}

func TestVotesCheckQuotaFirst(t *testing.T) {
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	ports := votePorts(newFakeVoteStore(), now)
	ports.CheckQuota = func(context.Context, string, string) (time.Duration, error) {
		return 0, &QuotaDeniedError{RetryAfter: time.Minute}
	}
	if _, err := Confirm(context.Background(), ports, voteCaller(), ConfirmDTO{ObservationID: "x", ClientSubmissionID: "y"}); !errors.Is(err, ErrQuotaDenied) {
		t.Errorf("confirm quota = %v", err)
	}
	if _, err := Dispute(context.Background(), ports, voteCaller(), DisputeDTO{TargetObservationID: "x", ClientSubmissionID: "y", Reason: domain.ReasonOther}); !errors.Is(err, ErrQuotaDenied) {
		t.Errorf("dispute quota = %v", err)
	}
}
