package application

import (
	"context"
	"errors"
	"testing"
	"time"

	domain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/moderation/domain"
)

type fakeCaseStore struct {
	cases    map[string]domain.Case
	byTarget map[string]string
	fail     error
}

func newFakeCaseStore() *fakeCaseStore {
	return &fakeCaseStore{cases: map[string]domain.Case{}, byTarget: map[string]string{}}
}

func (f *fakeCaseStore) OpenCase(_ context.Context, c domain.Case) (string, bool, error) {
	if f.fail != nil {
		return "", false, f.fail
	}
	key := c.TargetType + "|" + c.TargetID
	if id, ok := f.byTarget[key]; ok {
		return id, true, nil
	}
	f.cases[c.ID] = c
	f.byTarget[key] = c.ID
	return c.ID, false, nil
}

func (f *fakeCaseStore) ListOpen(_ context.Context, rank int32, at time.Time, id string, limit int32) ([]domain.Case, error) {
	if f.fail != nil {
		return nil, f.fail
	}
	// Priority-then-age order with stable ID tie-break, mirroring the SQL.
	all := make([]domain.Case, 0, len(f.cases))
	for _, c := range f.cases {
		all = append(all, c)
	}
	for i := 0; i < len(all); i++ {
		for j := i + 1; j < len(all); j++ {
			ri, rj := domain.PriorityRank(all[i].Priority), domain.PriorityRank(all[j].Priority)
			swap := ri > rj ||
				(ri == rj && (all[i].OpenedAt.After(all[j].OpenedAt) ||
					(all[i].OpenedAt.Equal(all[j].OpenedAt) && all[i].ID > all[j].ID)))
			if swap {
				all[i], all[j] = all[j], all[i]
			}
		}
	}
	// Apply the keyset cursor.
	out := all[:0:0]
	started := rank == -1 && id == ""
	for _, c := range all {
		if !started {
			r := int32(domain.PriorityRank(c.Priority))
			if r > rank || (r == rank && (c.OpenedAt.After(at) ||
				(c.OpenedAt.Equal(at) && c.ID > id))) {
				started = true
			} else {
				continue
			}
		}
		out = append(out, c)
		if int32(len(out)) >= limit {
			break
		}
	}
	if out == nil {
		return []domain.Case{}, nil
	}
	return out, nil
}

func queuePorts(store *fakeCaseStore) Ports {
	base := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	n := 0
	return Ports{
		Clock: func() time.Time { return base.Add(time.Duration(n) * time.Second) },
		NewID: func() (string, error) {
			n++
			return "c0000000-0000-4000-8000-0000000000" + string(rune('0'+n)), nil
		},
		Store: store,
	}
}

func TestOpenPersistsCase(t *testing.T) {
	store := newFakeCaseStore()
	res, err := Open(context.Background(), queuePorts(store), OpenDTO{
		TargetType: domain.TargetObservation,
		TargetID:   "b0000000-0000-4000-8000-000000000001",
		Reason:     "repeated conflicting reports",
	})
	if err != nil {
		t.Fatalf("open = %v", err)
	}
	if res.Replayed {
		t.Errorf("first open replayed")
	}
	stored, ok := store.cases[res.CaseID]
	if !ok || stored.Priority != domain.PriorityP2 || stored.Status != domain.StatusOpen {
		t.Errorf("stored = %+v", stored)
	}
}

func TestOpenDefaultsContributorPriority(t *testing.T) {
	store := newFakeCaseStore()
	res, err := Open(context.Background(), queuePorts(store), OpenDTO{
		TargetType: domain.TargetContributor, TargetID: "tok-abc",
		Reason: "suspected Sybil ring",
	})
	if err != nil {
		t.Fatalf("open = %v", err)
	}
	if store.cases[res.CaseID].Priority != domain.PriorityP1 {
		t.Errorf("contributor priority = %q", store.cases[res.CaseID].Priority)
	}
}

func TestOpenValidatesBeforeWork(t *testing.T) {
	store := newFakeCaseStore()
	if _, err := Open(context.Background(), queuePorts(store), OpenDTO{
		TargetType: "STATION", TargetID: "x", Reason: "r",
	}); !errors.Is(err, domain.ErrUnknownTarget) {
		t.Errorf("unknown target = %v", err)
	}
	if _, err := Open(context.Background(), queuePorts(store), OpenDTO{
		TargetType: domain.TargetObservation, TargetID: "not-a-uuid", Reason: "r",
	}); !errors.Is(err, domain.ErrInvalidCase) {
		t.Errorf("bad target id = %v", err)
	}
	if len(store.cases) != 0 {
		t.Errorf("invalid cases reached the store: %+v", store.cases)
	}
}

func TestOpenConvergesDuplicateTarget(t *testing.T) {
	store := newFakeCaseStore()
	p := queuePorts(store)
	first, err := Open(context.Background(), p, OpenDTO{
		TargetType: domain.TargetDispute,
		TargetID:   "d0000000-0000-4000-8000-000000000001",
		Reason:     "first report",
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := Open(context.Background(), p, OpenDTO{
		TargetType: domain.TargetDispute,
		TargetID:   "d0000000-0000-4000-8000-000000000001",
		Reason:     "second report, same target",
	})
	if err != nil {
		t.Fatal(err)
	}
	if second.CaseID != first.CaseID || !second.Replayed {
		t.Errorf("duplicate did not converge: %+v vs %+v", first, second)
	}
	if len(store.cases) != 1 {
		t.Errorf("duplicate flooded the queue: %d cases", len(store.cases))
	}
}

func TestListOrdersByPriorityThenAge(t *testing.T) {
	store := newFakeCaseStore()
	base := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	mustOpen := func(target, priority string, at time.Time) {
		c, _, err := domain.NewCase(domain.CaseParams{
			ID: "c-" + target[len(target)-4:] + priority, TargetType: domain.TargetObservation,
			TargetID: target, Priority: priority, Reason: "r", OpenedAt: at,
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := store.OpenCase(context.Background(), c); err != nil {
			t.Fatal(err)
		}
	}
	// P3 oldest still sorts after P1 newest: priority dominates age.
	mustOpen("b0000000-0000-4000-8000-000000000001", domain.PriorityP3, base)
	mustOpen("b0000000-0000-4000-8000-000000000002", domain.PriorityP1, base.Add(time.Hour))
	mustOpen("b0000000-0000-4000-8000-000000000003", domain.PriorityP2, base.Add(-time.Hour))
	res, err := List(context.Background(), Ports{Store: store}, ListDTO{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Cases) != 3 {
		t.Fatalf("cases = %d", len(res.Cases))
	}
	if res.Cases[0].Priority != domain.PriorityP1 || res.Cases[1].Priority != domain.PriorityP2 || res.Cases[2].Priority != domain.PriorityP3 {
		t.Errorf("queue order = %v", res.Cases)
	}
	// Cursor walk returns the next page only.
	next, err := List(context.Background(), Ports{Store: store}, ListDTO{
		Limit: 10, HasCursor: true, Rank: res.NextRank, At: res.Cases[0].OpenedAt, ID: res.Cases[0].ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = next
	second, err := List(context.Background(), Ports{Store: store}, ListDTO{
		Limit: 1, HasCursor: true,
		Rank: int32(domain.PriorityRank(res.Cases[0].Priority)),
		At:   res.Cases[0].OpenedAt, ID: res.Cases[0].ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Cases) != 1 || second.Cases[0].ID != res.Cases[1].ID {
		t.Errorf("cursor page = %+v, want %v", second.Cases, res.Cases[1].ID)
	}
}
