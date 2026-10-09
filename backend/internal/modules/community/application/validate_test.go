package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/community/domain"
)

// fakeValidateStore is an in-memory ValidateStore: decisions append with the
// same guards as the SQL store (known transition, exact sequence), and the
// consensus intent is captured instead of touching a queue.
type fakeValidateStore struct {
	obs       map[string]domain.Observation
	decisions map[string][]domain.Decision
	jobs      []string
	claimErr  error
}

func newFakeValidateStore() *fakeValidateStore {
	return &fakeValidateStore{
		obs:       map[string]domain.Observation{},
		decisions: map[string][]domain.Decision{},
	}
}

func (f *fakeValidateStore) Observation(_ context.Context, id string) (domain.Observation, error) {
	o, ok := f.obs[id]
	if !ok {
		return domain.Observation{}, errors.New("adapters: unknown observation")
	}
	return o, nil
}

func (f *fakeValidateStore) Decisions(_ context.Context, id string) ([]domain.Decision, error) {
	return append([]domain.Decision{}, f.decisions[id]...), nil
}

func currentState(ds []domain.Decision) string {
	s := domain.StateReceived
	for _, d := range ds {
		s = d.ToState
	}
	return s
}

func (f *fakeValidateStore) RecordDecision(_ context.Context, d domain.Decision) error {
	if f.claimErr != nil {
		return f.claimErr
	}
	if d.EventName() == "" {
		return domain.ErrBadTransition
	}
	cur := currentState(f.decisions[d.ObservationID])
	if d.FromState != cur {
		return domain.ErrBadTransition
	}
	if d.Sequence != int64(len(f.decisions[d.ObservationID])+1) {
		return domain.ErrBadTransition
	}
	f.decisions[d.ObservationID] = append(f.decisions[d.ObservationID], d)
	return nil
}

func (f *fakeValidateStore) RecordDecisionWithJob(_ context.Context, d domain.Decision, kind string, payload []byte, dedupe string, enqueue func(ctx context.Context, tx pgx.Tx, kind string, payload []byte, dedupe string) error) error {
	if err := f.RecordDecision(context.Background(), d); err != nil {
		return err
	}
	if kind != "" {
		f.jobs = append(f.jobs, kind+":"+dedupe)
		if enqueue != nil {
			_ = enqueue(context.Background(), nil, kind, payload, dedupe)
		}
	}
	return nil
}

func validateObs(stationID, evidenceID, supersedes string, received time.Time) domain.Observation {
	obs, _, err := domain.NewObservation(domain.Params{
		ID: "d6c74c23-63db-4c24-a2e5-408cb23bad27", ContributorRef: "tok-c1",
		ClientSubmissionID: "sub-1", StationID: stationID,
		Product: "GASOLINE_REGULAR", Unit: "L", AmountMilli: 5999,
		RawText: "5,999", ConditionKind: "STANDARD",
		EvidenceID: evidenceID, SupersedesID: supersedes,
		ClaimedCapturedAt: received.Add(-time.Hour), ReceivedAt: received,
	})
	if err != nil {
		panic(err)
	}
	return obs
}

func validateDeps(store *fakeValidateStore, now time.Time) ValidateDeps {
	return ValidateDeps{
		Clock:           func() time.Time { return now },
		StationExists:   func(context.Context, string) (bool, error) { return true, nil },
		StationLocation: func(context.Context, string) (string, bool, error) { return "reviewed", true, nil },
		Evidence:        func(context.Context, string) (EvidenceState, error) { return EvidenceState{}, nil },
		Trust:           func(context.Context, string) (bool, error) { return false, nil },
		Store:           store,
		EnqueueConsensus: func(context.Context, pgx.Tx, string, []byte, string) error {
			return nil
		},
	}
}

func TestValidateMetadataOnlyAdmitsWithConsensusIntent(t *testing.T) {
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	store := newFakeValidateStore()
	obs := validateObs("d6c74c23-63db-4c24-a2e5-408cb23bad26", "", "", now)
	store.obs[obs.ID] = obs
	deps := validateDeps(store, now)

	state, err := Validate(context.Background(), deps, obs.ID, "job-1")
	if err != nil {
		t.Fatalf("validate = %v", err)
	}
	if state != domain.StateValidated {
		t.Fatalf("state = %q, want VALIDATED", state)
	}
	ds := store.decisions[obs.ID]
	if len(ds) != 2 || ds[0].ToState != domain.StateValidating || ds[1].ToState != domain.StateValidated {
		t.Fatalf("decisions = %+v", ds)
	}
	if len(store.jobs) != 1 {
		t.Fatalf("downstream jobs = %v, want exactly the consensus intent", store.jobs)
	}
}

func TestValidateMissingStationRejects(t *testing.T) {
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	store := newFakeValidateStore()
	obs := validateObs("d6c74c23-63db-4c24-a2e5-408cb23bad26", "", "", now)
	store.obs[obs.ID] = obs
	deps := validateDeps(store, now)
	deps.StationExists = func(context.Context, string) (bool, error) { return false, nil }

	state, err := Validate(context.Background(), deps, obs.ID, "job-1")
	if err != nil {
		t.Fatalf("validate = %v", err)
	}
	if state != domain.StateRejected {
		t.Fatalf("state = %q, want REJECTED", state)
	}
	ds := store.decisions[obs.ID]
	if len(ds) != 2 || ds[1].ReasonCodes[0] != ReasonInvalidStation {
		t.Fatalf("decisions = %+v", ds)
	}
	if len(store.jobs) != 0 {
		t.Fatalf("rejected observation enqueued consensus: %v", store.jobs)
	}
}

func TestValidateUnknownLocationStillAdmits(t *testing.T) {
	// B-BR-014: an unverified coordinate records UNKNOWN but never blocks
	// admissibility and never uses a centroid as precise.
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	store := newFakeValidateStore()
	obs := validateObs("d6c74c23-63db-4c24-a2e5-408cb23bad26", "", "", now)
	store.obs[obs.ID] = obs
	deps := validateDeps(store, now)
	deps.StationLocation = func(context.Context, string) (string, bool, error) {
		return "unknown", false, nil
	}

	state, err := Validate(context.Background(), deps, obs.ID, "job-1")
	if err != nil {
		t.Fatalf("validate = %v", err)
	}
	if state != domain.StateValidated {
		t.Fatalf("state = %q, want VALIDATED despite unknown location", state)
	}
}

func TestValidateMissingEvidencePendsThenTimesOut(t *testing.T) {
	received := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	store := newFakeValidateStore()
	obs := validateObs("d6c74c23-63db-4c24-a2e5-408cb23bad26", "e0000000-0000-4000-8000-000000000001", "", received)
	store.obs[obs.ID] = obs
	deps := validateDeps(store, received.Add(time.Hour))
	deps.Evidence = func(context.Context, string) (EvidenceState, error) {
		return EvidenceState{Found: false}, nil
	}

	if _, err := Validate(context.Background(), deps, obs.ID, "job-1"); !errors.Is(err, ErrPending) {
		t.Fatalf("missing evidence = %v, want pending", err)
	}
	if got := currentState(store.decisions[obs.ID]); got != domain.StateValidating {
		t.Fatalf("state = %q, want VALIDATING while evidence pends", got)
	}

	// After the 24h policy deadline the same photo-dependent submission
	// rejects instead of waiting forever.
	deps.Clock = func() time.Time { return received.Add(25 * time.Hour) }
	state, err := Validate(context.Background(), deps, obs.ID, "job-2")
	if err != nil {
		t.Fatalf("timeout validate = %v", err)
	}
	if state != domain.StateRejected {
		t.Fatalf("state = %q, want REJECTED after evidence timeout", state)
	}
	last := store.decisions[obs.ID][len(store.decisions[obs.ID])-1]
	if len(last.ReasonCodes) == 0 || last.ReasonCodes[0] != ReasonEvidenceTimeout {
		t.Fatalf("reasons = %+v", last.ReasonCodes)
	}
}

func TestValidateEvidenceOwnerMismatchRejects(t *testing.T) {
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	store := newFakeValidateStore()
	obs := validateObs("d6c74c23-63db-4c24-a2e5-408cb23bad26", "e0000000-0000-4000-8000-000000000001", "", now)
	store.obs[obs.ID] = obs
	deps := validateDeps(store, now)
	deps.Evidence = func(context.Context, string) (EvidenceState, error) {
		return EvidenceState{Found: true, Ready: true, OwnerRef: "tok-other"}, nil
	}

	state, err := Validate(context.Background(), deps, obs.ID, "job-1")
	if err != nil {
		t.Fatalf("validate = %v", err)
	}
	if state != domain.StateRejected {
		t.Fatalf("state = %q, want REJECTED", state)
	}
}

func TestValidateTransientFailureKeepsValidating(t *testing.T) {
	// Unavailable signals never count as verified: a port error retries
	// without terminal progress and without regressing state.
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	store := newFakeValidateStore()
	obs := validateObs("d6c74c23-63db-4c24-a2e5-408cb23bad26", "", "", now)
	store.obs[obs.ID] = obs
	deps := validateDeps(store, now)
	deps.StationExists = func(context.Context, string) (bool, error) {
		return false, errors.New("directory down")
	}

	if _, err := Validate(context.Background(), deps, obs.ID, "job-1"); err == nil || errors.Is(err, ErrPending) {
		t.Fatalf("transient = %v, want a retryable error", err)
	}
	if got := currentState(store.decisions[obs.ID]); got != domain.StateValidating {
		t.Fatalf("state = %q, want VALIDATING after transient", got)
	}
}

func TestValidateTerminalIsIdempotent(t *testing.T) {
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	store := newFakeValidateStore()
	obs := validateObs("d6c74c23-63db-4c24-a2e5-408cb23bad26", "", "", now)
	store.obs[obs.ID] = obs
	deps := validateDeps(store, now)

	if _, err := Validate(context.Background(), deps, obs.ID, "job-1"); err != nil {
		t.Fatal(err)
	}
	n := len(store.decisions[obs.ID])
	state, err := Validate(context.Background(), deps, obs.ID, "job-2")
	if err != nil || state != domain.StateValidated {
		t.Fatalf("replay = %q %v", state, err)
	}
	if len(store.decisions[obs.ID]) != n {
		t.Fatalf("terminal replay appended decisions")
	}
}

func TestValidateBlockedContributorRejects(t *testing.T) {
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	store := newFakeValidateStore()
	obs := validateObs("d6c74c23-63db-4c24-a2e5-408cb23bad26", "", "", now)
	store.obs[obs.ID] = obs
	deps := validateDeps(store, now)
	deps.Trust = func(context.Context, string) (bool, error) { return true, nil }

	state, err := Validate(context.Background(), deps, obs.ID, "job-1")
	if err != nil {
		t.Fatalf("validate = %v", err)
	}
	if state != domain.StateRejected {
		t.Fatalf("state = %q, want REJECTED for blocked contributor", state)
	}
}

func TestValidateBindsEvidenceOnce(t *testing.T) {
	// P05-T04: the photo path claims its object exactly once; reuse
	// across observations rejects with a stable reason and no consensus
	// job, while transient claim failures retry in VALIDATING.
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	newPhotoObs := func(store *fakeValidateStore) {
		obs := validateObs("d6c74c23-63db-4c24-a2e5-408cb23bad26", "e0000000-0000-4000-8000-000000000001", "", now)
		store.obs[obs.ID] = obs
	}
	readyDeps := func(store *fakeValidateStore) ValidateDeps {
		deps := validateDeps(store, now)
		deps.Evidence = func(context.Context, string) (EvidenceState, error) {
			return EvidenceState{Found: true, Ready: true, OwnerRef: "tok-c1"}, nil
		}
		return deps
	}

	claimed := map[string]string{}
	claim := func(_ context.Context, evidenceID, observationID, _ string) error {
		if owner, ok := claimed[evidenceID]; ok && owner != observationID {
			return ErrEvidenceInUse
		}
		claimed[evidenceID] = observationID
		return nil
	}

	store := newFakeValidateStore()
	newPhotoObs(store)
	deps := readyDeps(store)
	deps.ClaimEvidence = claim
	state, err := Validate(context.Background(), deps, "d6c74c23-63db-4c24-a2e5-408cb23bad27", "job-1")
	if err != nil || state != domain.StateValidated {
		t.Fatalf("first bind = %q, %v", state, err)
	}
	if len(store.jobs) != 1 {
		t.Fatalf("jobs = %v, want the consensus intent", store.jobs)
	}

	// Same evidence, same observation (late retry): the terminal state
	// converges without re-claiming or duplicating work. Same-pair
	// claim replay itself is proven in the evidence store suite.
	replayed, err := Validate(context.Background(), deps, "d6c74c23-63db-4c24-a2e5-408cb23bad27", "job-2")
	if err != nil || replayed != domain.StateValidated {
		t.Fatalf("replay = %q, %v", replayed, err)
	}

	// A transient claim failure retries without terminal progress.
	flaky := newFakeValidateStore()
	newPhotoObs(flaky)
	fdeps := readyDeps(flaky)
	fdeps.ClaimEvidence = func(context.Context, string, string, string) error {
		return errors.New("evidence store down")
	}
	if _, err := Validate(context.Background(), fdeps, "d6c74c23-63db-4c24-a2e5-408cb23bad27", "job-1"); err == nil {
		t.Fatal("transient claim accepted")
	}
	if got := currentState(flaky.decisions["d6c74c23-63db-4c24-a2e5-408cb23bad27"]); got != domain.StateValidating {
		t.Fatalf("state = %q, want VALIDATING after transient", got)
	}
}

func TestValidateRejectsReusedEvidence(t *testing.T) {
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	store := newFakeValidateStore()
	obs := validateObs("d6c74c23-63db-4c24-a2e5-408cb23bad26", "e0000000-0000-4000-8000-000000000001", "", now)
	store.obs[obs.ID] = obs
	deps := validateDeps(store, now)
	deps.Evidence = func(context.Context, string) (EvidenceState, error) {
		return EvidenceState{Found: true, Ready: true, OwnerRef: "tok-c1"}, nil
	}
	deps.ClaimEvidence = func(context.Context, string, string, string) error { return ErrEvidenceInUse }
	state, err := Validate(context.Background(), deps, obs.ID, "job-1")
	if err != nil {
		t.Fatalf("validate = %v", err)
	}
	if state != domain.StateRejected {
		t.Fatalf("state = %q, want REJECTED for reused evidence", state)
	}
	ds := store.decisions[obs.ID]
	last := ds[len(ds)-1]
	if len(last.ReasonCodes) == 0 || last.ReasonCodes[0] != ReasonEvidenceReused {
		t.Fatalf("reasons = %+v", last.ReasonCodes)
	}
	if len(store.jobs) != 0 {
		t.Fatalf("rejected observation enqueued consensus: %v", store.jobs)
	}
}

func TestValidateSharedPhotoUsesCaptureLaneAndExpiredReadyRejects(t *testing.T) {
	now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	for _, expired := range []bool{false, true} {
		store := newFakeValidateStore()
		obs := validateObs("station", "photo", "", now)
		obs.PhotoCaptureID = "capture"
		store.obs[obs.ID] = obs
		deps := validateDeps(store, now)
		deadline := now.Add(time.Hour)
		if expired {
			deadline = now
		}
		deps.Evidence = func(context.Context, string) (EvidenceState, error) {
			return EvidenceState{Found: true, Ready: true, OwnerRef: obs.ContributorRef, ExpiresAt: deadline}, nil
		}
		deps.ClaimEvidence = func(context.Context, string, string, string) error {
			t.Fatal("shared photo entered legacy claim lane")
			return nil
		}
		claims := 0
		deps.ClaimPhotoEvidence = func(_ context.Context, got domain.Observation) error {
			claims++
			if got.PhotoCaptureID != "capture" {
				t.Fatal("lost capture")
			}
			return nil
		}
		state, err := Validate(context.Background(), deps, obs.ID, "job")
		if expired {
			if err != nil || state != domain.StateRejected || claims != 0 {
				t.Fatalf("expired state=%s err=%v claims=%d", state, err, claims)
			}
		} else if err != nil || state != domain.StateValidated || claims != 1 {
			t.Fatalf("shared state=%s err=%v claims=%d", state, err, claims)
		}
	}
}
