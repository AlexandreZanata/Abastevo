package application

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/domain"
	officialapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/official/application"
)

type fakeBulletinReader struct {
	sets map[string]officialapp.BulletinSet
	err  error
}

func (f *fakeBulletinReader) ReadWeeklyBulletins(ctx context.Context, q officialapp.BulletinQuery) (officialapp.BulletinSet, error) {
	if f.err != nil {
		return officialapp.BulletinSet{}, f.err
	}
	set, ok := f.sets[q.Fuel+"|"+q.Unit]
	if !ok {
		return officialapp.BulletinSet{}, officialapp.ErrBulletinUnavailable
	}
	return set, nil
}

type fakeBulletinStore struct {
	source *domain.Source
	groups map[string]*BulletinGroup
	posts  map[string]*domain.Post
	keys   map[string]string
}

func newFakeBulletinStore() *fakeBulletinStore {
	return &fakeBulletinStore{
		source: &domain.Source{
			ID: "00000000-0000-0000-0000-000000000001", Code: BulletinSourceCode,
			RightsStatus: domain.RightsApproved, PublicationEnabled: true,
		},
		groups: map[string]*BulletinGroup{},
		posts:  map[string]*domain.Post{},
		keys:   map[string]string{},
	}
}

func (f *fakeBulletinStore) GetSourceByCode(ctx context.Context, code string) (*domain.Source, error) {
	if f.source == nil {
		return nil, domain.ErrMissingSource
	}
	return f.source, nil
}

func (f *fakeBulletinStore) FindLatestBulletinGroup(ctx context.Context, groupKey string) (*BulletinGroup, error) {
	return f.groups[groupKey], nil
}

func (f *fakeBulletinStore) CreateBulletinDraft(ctx context.Context, post *domain.Post) (bool, error) {
	if existing, ok := f.keys[*post.BulletinKey]; ok {
		_ = existing
		return false, nil
	}
	cp := *post
	f.posts[post.ID] = &cp
	f.keys[*post.BulletinKey] = post.ID
	f.groups[*post.BulletinGroupKey] = &BulletinGroup{PostID: post.ID, PublicationState: post.PublicationState}
	return true, nil
}

type fixedBulletinClock struct{ now time.Time }

func (c fixedBulletinClock) Now() time.Time { return c.now }

type sequenceIDs struct{ next int }

func (s *sequenceIDs) NewID() string {
	s.next++
	return fmt.Sprintf("00000000-0000-4000-8000-%012d", s.next)
}

func officialBulletinSet(currentRevision, currentPublished string, currentSum int64) officialapp.BulletinSet {
	revision := func(id, start, end, published string) officialapp.BulletinRevision {
		return officialapp.BulletinRevision{
			RevisionID: id, SurveyStart: start, SurveyEnd: end, PublishedAt: published,
			SourceURL: "https://www.gov.br/anp/synthetic.xlsx", SourceChecksum: "sha256:" + id,
			ParserVersion: "synthetic-v1",
		}
	}
	sample := func(sum int64) officialapp.BulletinSample {
		return officialapp.BulletinSample{
			UF: "SP", SumMilliBrl: sum, ObservationCount: 100,
			MeanNumerator: sum / 100, MeanDenominator: 1, Mean: fmt.Sprintf("%d", sum/100),
			Coverage: officialapp.BulletinCoverage{
				SourceObservations: 105, SampledStations: 100, SourceStations: 120,
			},
		}
	}
	return officialapp.BulletinSet{
		MethodologyVersion: officialapp.BulletinMethodologyVersion,
		Query: officialapp.BulletinQuery{
			MethodologyVersion: officialapp.BulletinMethodologyVersion,
			Fuel:               "GASOLINE_REGULAR", Unit: "L",
		},
		CurrentRevision:  revision(currentRevision, "2026-10-26", "2026-11-01", currentPublished),
		PreviousRevision: revision("22222222-2222-4222-8222-222222222222", "2026-10-19", "2026-10-25", "2026-10-27T12:00:00Z"),
		Pairs: []officialapp.BulletinPair{{
			UF: "SP", Fuel: "GASOLINE_REGULAR", Unit: "L",
			Current: sample(currentSum), Previous: sample(590000),
		}},
	}
}

func TestBulletinGeneratorCreatesPendingDraftAndConverges(t *testing.T) {
	store := newFakeBulletinStore()
	reader := &fakeBulletinReader{sets: map[string]officialapp.BulletinSet{
		"GASOLINE_REGULAR|L": officialBulletinSet("11111111-1111-4111-8111-111111111111", "2026-11-02T12:00:00Z", 601800),
	}}
	ids := &sequenceIDs{}
	gen := BulletinGenerator{
		Official: reader, Store: store,
		Clock: fixedBulletinClock{now: time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)},
		IDs:   ids,
	}
	report, err := gen.Generate(context.Background())
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if report.Created != 1 || len(store.posts) != 1 {
		t.Fatalf("report %+v posts %d", report, len(store.posts))
	}
	var post *domain.Post
	for _, p := range store.posts {
		post = p
	}
	if post.Kind != domain.KindBulletin || post.Category != domain.CategoryAbastevoData ||
		post.PublicationState != domain.StatePendingReview || post.SourceID != store.source.ID {
		t.Fatalf("draft shape %+v", post)
	}
	if post.BulletinKey == nil || *post.BulletinKey == "" || post.BulletinGroupKey == nil ||
		post.BulletinProvenance == nil || post.BulletinProvenance.Fuel != "GASOLINE_REGULAR" ||
		post.BulletinProvenance.Current.SumMilliBrl != 601800 {
		t.Fatalf("provenance %+v", post.BulletinProvenance)
	}
	if post.FuelProduct != nil || post.StationID != nil || post.Region == nil || post.Region.UF != "SP" {
		t.Fatalf("subject fields %+v", post)
	}

	again, err := gen.Generate(context.Background())
	if err != nil {
		t.Fatalf("regenerate: %v", err)
	}
	if again.Created != 0 || again.Existing != 1 || len(store.posts) != 1 {
		t.Fatalf("restart not convergent: %+v posts %d", again, len(store.posts))
	}
}

func TestBulletinGeneratorLinksCorrection(t *testing.T) {
	store := newFakeBulletinStore()
	reader := &fakeBulletinReader{sets: map[string]officialapp.BulletinSet{
		"GASOLINE_REGULAR|L": officialBulletinSet("11111111-1111-4111-8111-111111111111", "2026-11-02T12:00:00Z", 601800),
	}}
	ids := &sequenceIDs{}
	gen := BulletinGenerator{
		Official: reader, Store: store,
		Clock: fixedBulletinClock{now: time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)},
		IDs:   ids,
	}
	if _, err := gen.Generate(context.Background()); err != nil {
		t.Fatal(err)
	}
	var originalID string
	for id := range store.posts {
		originalID = id
	}
	reader.sets["GASOLINE_REGULAR|L"] = officialBulletinSet(
		"33333333-3333-4333-8333-333333333333", "2026-11-04T12:00:00Z", 602000,
	)
	report, err := gen.Generate(context.Background())
	if err != nil {
		t.Fatalf("correction: %v", err)
	}
	if report.Created != 1 || len(store.posts) != 2 {
		t.Fatalf("correction report %+v posts %d", report, len(store.posts))
	}
	var corrected *domain.Post
	for id, p := range store.posts {
		if id != originalID {
			corrected = p
		}
	}
	if corrected == nil || corrected.SupersedesPostID == nil || *corrected.SupersedesPostID != originalID {
		t.Fatalf("correction link %+v", corrected)
	}
	if corrected.BulletinKey == nil || *corrected.BulletinKey == *store.posts[originalID].BulletinKey {
		t.Fatal("correction key was not distinct")
	}
}

func TestBulletinGeneratorNoInputAndMissingSource(t *testing.T) {
	store := newFakeBulletinStore()
	gen := BulletinGenerator{
		Official: &fakeBulletinReader{},
		Store:    store,
		Clock:    fixedBulletinClock{now: time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)},
		IDs:      &sequenceIDs{},
	}
	report, err := gen.Generate(context.Background())
	if err != nil || report.Created != 0 || report.Existing != 0 || len(store.posts) != 0 {
		t.Fatalf("no-input run: %+v %v posts %d", report, err, len(store.posts))
	}

	store.source = nil
	if _, err := gen.Generate(context.Background()); !errors.Is(err, domain.ErrMissingSource) {
		t.Fatalf("expected missing source, got %v", err)
	}

	store.source = &domain.Source{ID: "source", Code: BulletinSourceCode, RightsStatus: domain.RightsPending, PublicationEnabled: true}
	if _, err := gen.Generate(context.Background()); !errors.Is(err, domain.ErrMissingRights) {
		t.Fatalf("expected missing rights, got %v", err)
	}
}

func TestBulletinGeneratorPropagatesAggregateFailure(t *testing.T) {
	store := newFakeBulletinStore()
	gen := BulletinGenerator{
		Official: &fakeBulletinReader{err: officialapp.ErrBulletinOverflow},
		Store:    store,
		Clock:    fixedBulletinClock{now: time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)},
		IDs:      &sequenceIDs{},
	}
	if _, err := gen.Generate(context.Background()); !errors.Is(err, officialapp.ErrBulletinOverflow) {
		t.Fatalf("expected overflow, got %v", err)
	}
	if len(store.posts) != 0 {
		t.Fatal("failed aggregate must not create a post")
	}
}
