package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/community/domain"
)

// fakeRecompute exposes every loader input explicitly: anchors,
// confirmations, tiers, photos and the moderation flag, plus the
// persisted projection for assertions.
type fakeRecompute struct {
	obs        map[string]domain.Observation
	anchors    []domain.Observation
	confs      map[string][]ConfirmationVote
	tiers      map[string]string
	photos     map[string]PhotoInfo
	moderation bool
	saved      []savedProjection
	obsErr     error
}

type savedProjection struct {
	key    domain.PriceKey
	result domain.Result
	rep    string
	cutoff time.Time
	next   *time.Time
}

func newFakeRecompute() *fakeRecompute {
	return &fakeRecompute{
		obs: map[string]domain.Observation{}, confs: map[string][]ConfirmationVote{},
		tiers: map[string]string{}, photos: map[string]PhotoInfo{},
	}
}

func (f *fakeRecompute) Observation(_ context.Context, id string) (domain.Observation, error) {
	if f.obsErr != nil {
		return domain.Observation{}, f.obsErr
	}
	o, ok := f.obs[id]
	if !ok {
		return domain.Observation{}, errors.New("adapters: unknown observation")
	}
	return o, nil
}

func (f *fakeRecompute) Anchors(_ context.Context, _ domain.PriceKey, _ time.Time) ([]domain.Observation, error) {
	return append([]domain.Observation{}, f.anchors...), nil
}

func (f *fakeRecompute) Confirmations(_ context.Context, anchorIDs []string) ([]ConfirmationVote, error) {
	var out []ConfirmationVote
	for _, id := range anchorIDs {
		out = append(out, f.confs[id]...)
	}
	return out, nil
}

func (f *fakeRecompute) Tiers(_ context.Context, refs []string) (map[string]string, error) {
	out := map[string]string{}
	for _, r := range refs {
		if t, ok := f.tiers[r]; ok {
			out[r] = t
		} else {
			out[r] = "NEW"
		}
	}
	return out, nil
}

func (f *fakeRecompute) Photo(_ context.Context, anchor domain.Observation) (PhotoInfo, error) {
	if p, ok := f.photos[anchor.EvidenceID]; ok {
		return p, nil
	}
	return PhotoInfo{}, nil
}

func (f *fakeRecompute) ModerationOpen(_ context.Context, _ domain.PriceKey) (bool, error) {
	return f.moderation, nil
}

func (f *fakeRecompute) SaveProjection(_ context.Context, key domain.PriceKey, result domain.Result, rep string, _ time.Time, cutoff time.Time, next *time.Time, _ string) error {
	f.saved = append(f.saved, savedProjection{key: key, result: result, rep: rep, cutoff: cutoff, next: next})
	return nil
}

func recomputePorts(f *fakeRecompute, now time.Time) RecomputePorts {
	return RecomputePorts{
		Clock:         func() time.Time { return now },
		Store:         f,
		Anchors:       f.Anchors,
		Confirmations: f.Confirmations,
		Tiers:         f.Tiers,
		Photo:         f.Photo,
		Moderation:    f.ModerationOpen,
	}
}

func recomputeObs(id, ref string, received time.Time) domain.Observation {
	obs, _, err := domain.NewObservation(domain.Params{
		ID: id, ContributorRef: ref,
		ClientSubmissionID: "sub-" + id[len(id)-2:], StationID: "d6c74c23-63db-4c24-a2e5-408cb23bad26",
		Product: "GASOLINE_REGULAR", Unit: "L", AmountMilli: 5999,
		RawText: "5,999", ConditionKind: "STANDARD",
		ClaimedCapturedAt: received.Add(-time.Hour), ReceivedAt: received,
	})
	if err != nil {
		panic(err)
	}
	return obs
}

func TestRecomputePersistsPriceProjection(t *testing.T) {
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	f := newFakeRecompute()
	a1 := recomputeObs("d6c74c23-63db-4c24-a2e5-408cb23bad27", "tok-c1", now.Add(-time.Hour))
	a2 := recomputeObs("d6c74c23-63db-4c24-a2e5-408cb23bad28", "tok-c2", now.Add(-2*time.Hour))
	f.obs[a1.ID] = a1
	f.obs[a2.ID] = a2
	f.anchors = []domain.Observation{a1, a2}
	f.confs[a1.ID] = []ConfirmationVote{{ID: "k1", ObservationID: a1.ID, ContributorRef: "tok-c3", ReceivedAt: now.Add(-30 * time.Minute)}}
	f.tiers["tok-c1"] = "ESTABLISHED"

	res, err := Recompute(context.Background(), recomputePorts(f, now), a1.ID)
	if err != nil {
		t.Fatalf("recompute = %v", err)
	}
	// Established anchor plus independent confirmation: MEDIUM needs a
	// validated photo, so a photo-less pair stays an honest LOW.
	if res.Verdict != domain.VerdictPrice || res.AmountMilli != 5999 {
		t.Fatalf("result = %+v", res)
	}
	if len(f.saved) != 1 {
		t.Fatalf("saved = %+v", f.saved)
	}
	saved := f.saved[0]
	if saved.key.StationID == "" || saved.rep == "" {
		t.Errorf("saved key/representative = %+v", saved)
	}
	if saved.next == nil || !saved.next.After(now) {
		t.Errorf("next recompute = %v, want a future boundary", saved.next)
	}
	if len(saved.result.WinnerObservationIDs) != 2 {
		t.Errorf("audit anchors = %v", saved.result.WinnerObservationIDs)
	}
}

func TestRecomputeSkipsConfirmationOnIneligibleAnchor(t *testing.T) {
	// A confirmation affirming a stale anchor cannot revive it: the
	// confirmation drops with the anchor instead of reaching Compute.
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	f := newFakeRecompute()
	fresh := recomputeObs("d6c74c23-63db-4c24-a2e5-408cb23bad27", "tok-c1", now.Add(-time.Hour))
	stale := recomputeObs("d6c74c23-63db-4c24-a2e5-408cb23bad28", "tok-c2", now.Add(-50*time.Hour))
	f.obs[fresh.ID] = fresh
	f.obs[stale.ID] = stale
	f.anchors = []domain.Observation{fresh, stale}
	f.confs[stale.ID] = []ConfirmationVote{{ID: "k9", ObservationID: stale.ID, ContributorRef: "tok-c3", ReceivedAt: now.Add(-30 * time.Minute)}}
	res, err := Recompute(context.Background(), recomputePorts(f, now), fresh.ID)
	if err != nil {
		t.Fatalf("recompute = %v", err)
	}
	if res.Verdict != domain.VerdictPrice || res.Confirmations != 0 {
		t.Errorf("reviving confirmation counted: %+v", res)
	}
}

func TestRecomputePropagatesUnavailable(t *testing.T) {
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	f := newFakeRecompute()
	f.obsErr = errors.New("db down")
	if _, err := Recompute(context.Background(), recomputePorts(f, now), "missing"); err == nil {
		t.Error("missing trigger accepted")
	}
	if len(f.saved) != 0 {
		t.Errorf("failed run persisted: %+v", f.saved)
	}
}

func TestNextRecomputeBoundaries(t *testing.T) {
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	mk := func(received time.Time) domain.Vote {
		return domain.Vote{
			VoterRef: "c1", ObservationID: "o1", EventID: "e1", AuthorRef: "c1",
			AmountMilli: 5999, ReceivedAt: received, AnchorReceivedAt: received,
			TrustTier: "NEW",
		}
	}
	fresh := domain.Result{
		Verdict: domain.VerdictPrice, AmountMilli: 5999,
		ExpiresAt: now.Add(47 * time.Hour),
	}
	// A 1h-old vote crosses 6h before anything else: next wake is 17:00.
	next := NextRecomputeAt([]domain.Vote{mk(now.Add(-time.Hour))}, fresh, now)
	if next == nil || !next.Equal(now.Add(5*time.Hour)) {
		t.Errorf("next = %v, want +5h boundary", next)
	}
	// Stale projections schedule nothing: writes retrigger instead.
	stale := fresh
	stale.Freshness = domain.FreshStale
	if next := NextRecomputeAt([]domain.Vote{mk(now.Add(-50 * time.Hour))}, stale, now); next != nil {
		t.Errorf("stale next = %v, want no wake", next)
	}
}
