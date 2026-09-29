package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	domain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/moderation/domain"
)

type fakeActStore struct {
	cases   map[string]domain.Case
	actions []domain.Action
	jobs    []string
	failJob bool
}

func newFakeActStore(cases ...domain.Case) *fakeActStore {
	m := map[string]domain.Case{}
	for _, c := range cases {
		m[c.ID] = c
	}
	return &fakeActStore{cases: m}
}

func (f *fakeActStore) OpenCase(context.Context, domain.Case) (string, bool, error) {
	return "", false, errors.New("not used")
}

func (f *fakeActStore) Get(_ context.Context, id string) (domain.Case, error) {
	c, ok := f.cases[id]
	if !ok {
		return domain.Case{}, errors.New("adapters: unknown case")
	}
	return c, nil
}

func (f *fakeActStore) ListOpen(context.Context, int32, time.Time, string, int32) ([]domain.Case, error) {
	return nil, errors.New("not used")
}

func (f *fakeActStore) RecordAction(_ context.Context, a domain.Action, toStatus, kind string, _ []byte, _ string, enqueue func(ctx context.Context, tx pgx.Tx, kind string, payload []byte, dedupe string) error) (string, error) {
	c := f.cases[a.CaseID]
	next, err := domain.NextStatus(a.Action, c.Status)
	if err != nil {
		return "", err
	}
	if next != toStatus {
		return "", errors.New("status derivation drifted")
	}
	if f.failJob {
		return "", errors.New("enqueue down")
	}
	f.actions = append(f.actions, a)
	c.Status = toStatus
	f.cases[a.CaseID] = c
	if enqueue != nil {
		_ = enqueue(context.Background(), nil, kind, []byte(`{}`), "dedupe")
		f.jobs = append(f.jobs, kind)
	}
	return a.ID, nil
}

func actPorts(store *fakeActStore) ActPorts {
	return ActPorts{
		Clock: func() time.Time { return time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC) },
		NewID: func() (string, error) { return "a0000000-0000-4000-8000-000000000001", nil },
		Store: store,
		EnqueueJob: func(context.Context, pgx.Tx, string, []byte, string) error {
			return nil
		},
	}
}

func openTestCase(status, targetType, targetID string) domain.Case {
	c, _, err := domain.NewCase(domain.CaseParams{
		ID:         "c0000000-0000-4000-8000-000000000001",
		TargetType: targetType, TargetID: targetID,
		Priority: domain.PriorityP2, Reason: "r",
		OpenedAt: time.Date(2026, 9, 30, 11, 0, 0, 0, time.UTC),
	})
	if err != nil {
		panic(err)
	}
	c.Status = status
	return c
}

func TestActDeniesAnonymousOperator(t *testing.T) {
	store := newFakeActStore(openTestCase(domain.StatusOpen,
		domain.TargetObservation, "b0000000-0000-4000-8000-000000000001"))
	if _, err := Act(context.Background(), actPorts(store), Caller{}, ActDTO{
		CaseID: "c0000000-0000-4000-8000-000000000001",
		Action: domain.ActionReview, Reason: "triage",
	}); !errors.Is(err, domain.ErrUnauthorized) {
		t.Errorf("anonymous operator = %v", err)
	}
	if len(store.actions) != 0 || len(store.jobs) != 0 {
		t.Errorf("anonymous act reached the store: %+v %v", store.actions, store.jobs)
	}
}

func TestActRejectsMissingReason(t *testing.T) {
	store := newFakeActStore(openTestCase(domain.StatusOpen,
		domain.TargetObservation, "b0000000-0000-4000-8000-000000000001"))
	if _, err := Act(context.Background(), actPorts(store), Caller{OperatorID: "op-7"}, ActDTO{
		CaseID: "c0000000-0000-4000-8000-000000000001",
		Action: domain.ActionReview, Reason: "  ",
	}); !errors.Is(err, domain.ErrBadReason) {
		t.Errorf("empty reason = %v", err)
	}
	if len(store.actions) != 0 {
		t.Errorf("reasonless act reached the store")
	}
}

func TestActForbidsMismatchedTarget(t *testing.T) {
	store := newFakeActStore(openTestCase(domain.StatusOpen,
		domain.TargetObservation, "b0000000-0000-4000-8000-000000000001"))
	if _, err := Act(context.Background(), actPorts(store), Caller{OperatorID: "op-7"}, ActDTO{
		CaseID: "c0000000-0000-4000-8000-000000000001",
		Action: domain.ActionBlock, Reason: "block it",
	}); !errors.Is(err, domain.ErrActionForbidden) {
		t.Errorf("block on observation = %v", err)
	}
}

func TestActRefusesClosedCase(t *testing.T) {
	store := newFakeActStore(openTestCase(domain.StatusResolved,
		domain.TargetObservation, "b0000000-0000-4000-8000-000000000001"))
	if _, err := Act(context.Background(), actPorts(store), Caller{OperatorID: "op-7"}, ActDTO{
		CaseID: "c0000000-0000-4000-8000-000000000001",
		Action: domain.ActionDismiss, Reason: "late dismiss",
	}); !errors.Is(err, domain.ErrTerminalCase) {
		t.Errorf("closed case act = %v", err)
	}
}

func TestActRecordsAuditStatusAndJob(t *testing.T) {
	store := newFakeActStore(openTestCase(domain.StatusOpen,
		domain.TargetObservation, "b0000000-0000-4000-8000-000000000001"))
	res, err := Act(context.Background(), actPorts(store), Caller{OperatorID: "op-7"}, ActDTO{
		CaseID: "c0000000-0000-4000-8000-000000000001",
		Action: domain.ActionInvalidate, Reason: "confirmed fake price",
	})
	if err != nil {
		t.Fatalf("act = %v", err)
	}
	if res.ToStatus != domain.StatusResolved || res.JobKind != "community-consensus" || !res.Enqueued {
		t.Errorf("result = %+v", res)
	}
	if len(store.actions) != 1 || store.actions[0].ActorID != "op-7" {
		t.Errorf("audit = %+v", store.actions)
	}
	if store.cases[res.CaseID].Status != domain.StatusResolved {
		t.Errorf("case status = %q", store.cases[res.CaseID].Status)
	}
}

func TestActMapsBlockToTrustJob(t *testing.T) {
	store := newFakeActStore(openTestCase(domain.StatusInReview,
		domain.TargetContributor, "tok-abc"))
	res, err := Act(context.Background(), actPorts(store), Caller{OperatorID: "op-7"}, ActDTO{
		CaseID: "c0000000-0000-4000-8000-000000000001",
		Action: domain.ActionBlock, Reason: "reviewed Sybil ring",
	})
	if err != nil {
		t.Fatalf("act = %v", err)
	}
	if res.JobKind != "trust-affected-recompute" || res.ToStatus != domain.StatusResolved {
		t.Errorf("result = %+v", res)
	}
}

func TestActPropagatesEnqueueFailure(t *testing.T) {
	store := newFakeActStore(openTestCase(domain.StatusOpen,
		domain.TargetObservation, "b0000000-0000-4000-8000-000000000001"))
	store.failJob = true
	if _, err := Act(context.Background(), actPorts(store), Caller{OperatorID: "op-7"}, ActDTO{
		CaseID: "c0000000-0000-4000-8000-000000000001",
		Action: domain.ActionReview, Reason: "triage",
	}); err == nil {
		t.Error("enqueue failure accepted")
	}
}
