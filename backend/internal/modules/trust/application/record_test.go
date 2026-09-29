package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trust/domain"
)

type fakeRecordStore struct {
	decisions []domain.Decision
	jobs      []string
	fail      error
}

func (f *fakeRecordStore) AppendDecision(_ context.Context, d domain.Decision, enqueue func(ctx context.Context, tx pgx.Tx, kind string, payload []byte, dedupe string) error) (string, error) {
	if f.fail != nil {
		return "", f.fail
	}
	f.decisions = append(f.decisions, d)
	if enqueue != nil {
		_ = enqueue(context.Background(), nil, "trust-affected-recompute", []byte(`{}`), "trust:"+d.ContributorRef)
		f.jobs = append(f.jobs, "trust:"+d.ContributorRef)
	}
	return d.ID, nil
}

func recordPorts(store *fakeRecordStore) Ports {
	return Ports{
		Clock: func() time.Time { return time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC) },
		NewID: func() (string, error) { return "t0000000-0000-4000-8000-000000000001", nil },
		Store: store,
		EnqueueJob: func(context.Context, pgx.Tx, string, []byte, string) error {
			return nil
		},
	}
}

func TestRecordPersistsVerdictAndJob(t *testing.T) {
	store := &fakeRecordStore{}
	decision, err := Record(context.Background(), recordPorts(store), RecordDTO{
		ContributorRef: "tok-c1", Tier: domain.TierBlocked,
		Reason: "reviewed Sybil ring", CaseRefs: []string{"case-9"},
	})
	if err != nil {
		t.Fatalf("record = %v", err)
	}
	if decision.Tier != domain.TierBlocked || decision.PolicyVersion != domain.PolicyV1 {
		t.Errorf("decision = %+v", decision)
	}
	if len(store.decisions) != 1 || len(store.jobs) != 1 || store.jobs[0] != "trust:tok-c1" {
		t.Errorf("stored = %+v jobs = %v", store.decisions, store.jobs)
	}
}

func TestRecordValidatesBeforeWork(t *testing.T) {
	store := &fakeRecordStore{}
	if _, err := Record(context.Background(), recordPorts(store), RecordDTO{
		ContributorRef: "tok-c1", Tier: domain.TierEstablished, Reason: "review",
	}); !errors.Is(err, domain.ErrCaseRequired) {
		t.Errorf("caseless established = %v", err)
	}
	if _, err := Record(context.Background(), recordPorts(store), RecordDTO{
		ContributorRef: "tok-c1", Tier: "GOLD", Reason: "review", CaseRefs: []string{"case-1"},
	}); !errors.Is(err, domain.ErrUnknownTier) {
		t.Errorf("unknown tier = %v", err)
	}
	if len(store.decisions) != 0 || len(store.jobs) != 0 {
		t.Errorf("invalid verdicts reached the store: %+v %v", store.decisions, store.jobs)
	}
}

func TestRecordPropagatesStoreFailure(t *testing.T) {
	store := &fakeRecordStore{fail: errors.New("db down")}
	if _, err := Record(context.Background(), recordPorts(store), RecordDTO{
		ContributorRef: "tok-c1", Tier: domain.TierNew, Reason: "intake",
	}); err == nil {
		t.Error("store failure accepted")
	}
}
